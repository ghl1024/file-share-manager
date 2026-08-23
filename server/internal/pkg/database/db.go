/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/pkg/logger"

	mysqldriver "github.com/go-sql-driver/mysql"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var (
	DB             *gorm.DB
	AuditArchiveDB *gorm.DB
)

type zapGormLogger struct {
	LogLevel                  gormlogger.LogLevel
	SlowThreshold             time.Duration
	IgnoreRecordNotFoundError bool
}

func (l *zapGormLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	newLogger := *l
	newLogger.LogLevel = level
	return &newLogger
}

func (l *zapGormLogger) Info(ctx context.Context, s string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Info {
		logger.FromContext(ctx).Sugar().Infof(s, args...)
	}
}

func (l *zapGormLogger) Warn(ctx context.Context, s string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Warn {
		logger.FromContext(ctx).Sugar().Warnf(s, args...)
	}
}

func (l *zapGormLogger) Error(ctx context.Context, s string, args ...interface{}) {
	if l.LogLevel >= gormlogger.Error {
		logger.FromContext(ctx).Sugar().Errorf(s, args...)
	}
}

func (l *zapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.LogLevel <= gormlogger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	if err != nil && (!l.IgnoreRecordNotFoundError || err != gorm.ErrRecordNotFound) {
		if l.LogLevel >= gormlogger.Error {
			logger.FromContext(ctx).Error("db_trace", zap.Error(err), zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
		}
	} else if l.SlowThreshold != 0 && elapsed > l.SlowThreshold {
		if l.LogLevel >= gormlogger.Warn {
			logger.FromContext(ctx).Warn("db_slow_query", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
		}
	} else {
		if l.LogLevel >= gormlogger.Info {
			logger.FromContext(ctx).Debug("db_query", zap.Duration("elapsed", elapsed), zap.Int64("rows", rows), zap.String("sql", sql))
		}
	}
}

// InitDB initializes the MySQL database connection
func InitDB() error {
	cfg := config.GetConfig()
	db, err := openConnection(connectionConfig{
		host: cfg.Database.Host, port: cfg.Database.Port, user: cfg.Database.User, password: cfg.Database.Password,
		dbName: cfg.Database.DBName, maxIdleConns: cfg.Database.MaxIdleConns, maxOpenConns: cfg.Database.MaxOpenConns,
	})
	if err != nil {
		return fmt.Errorf("initialize business database connection: %w", err)
	}
	DB = db
	logger.Info("business_database_connection_initialized")
	return nil
}

// InitAuditArchiveDB opens and validates the privileged connection used by the
// archive worker. It remains nil while audit archiving is disabled.
func InitAuditArchiveDB() error {
	cfg := config.GetConfig()
	AuditArchiveDB = nil
	if cfg == nil || !cfg.Audit.ArchiveEnabled {
		return nil
	}
	db, err := openConnection(connectionConfig{
		host: cfg.AuditDatabase.Host, port: cfg.AuditDatabase.Port, user: cfg.AuditDatabase.User, password: cfg.AuditDatabase.Password,
		dbName: cfg.AuditDatabase.DBName, maxIdleConns: cfg.AuditDatabase.MaxIdleConns, maxOpenConns: cfg.AuditDatabase.MaxOpenConns,
	})
	if err != nil {
		return fmt.Errorf("initialize audit archive database connection: %w", err)
	}
	if err := VerifyAuditPrivilegeIsolation(DB, db); err != nil {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		return err
	}
	AuditArchiveDB = db
	logger.Info("audit_archive_database_connection_initialized")
	return nil
}

type connectionConfig struct {
	host         string
	port         int
	user         string
	password     string
	dbName       string
	maxIdleConns int
	maxOpenConns int
}

func openConnection(cfg connectionConfig) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", cfg.user, cfg.password, cfg.host, cfg.port, cfg.dbName)

	gormLogger := &zapGormLogger{
		LogLevel:                  gormlogger.Warn,
		SlowThreshold:             200 * time.Millisecond,
		IgnoreRecordNotFoundError: true,
	}
	if runtimeConfig := config.GetConfig(); runtimeConfig != nil && runtimeConfig.Database.LogQueries {
		gormLogger.LogLevel = gormlogger.Info
	}

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger: gormLogger,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	sqlDB.SetMaxIdleConns(cfg.maxIdleConns)
	sqlDB.SetMaxOpenConns(cfg.maxOpenConns)
	sqlDB.SetConnMaxLifetime(time.Hour)
	return db, nil
}

// VerifyAuditPrivilegeIsolation fails closed before an archive worker starts.
// No-op DML statements exercise effective privileges, including inherited
// schema grants and active roles, instead of trying to parse SHOW GRANTS.
func VerifyAuditPrivilegeIsolation(businessDB, archiveDB *gorm.DB) error {
	if businessDB == nil || archiveDB == nil {
		return errors.New("audit privilege isolation requires both database connections")
	}
	businessUser, err := currentUser(businessDB)
	if err != nil {
		return fmt.Errorf("read business database identity: %w", err)
	}
	archiveUser, err := currentUser(archiveDB)
	if err != nil {
		return fmt.Errorf("read audit archive database identity: %w", err)
	}
	if businessUser == archiveUser {
		return fmt.Errorf("audit archive database must use a different MySQL account from business database (%s)", businessUser)
	}
	businessProbeDB := businessDB.Session(&gorm.Session{Logger: gormlogger.Discard})
	archiveProbeDB := archiveDB.Session(&gorm.Session{Logger: gormlogger.Discard})
	for _, probe := range []struct {
		name  string
		query string
	}{
		{name: "SELECT operation_logs", query: "SELECT id FROM operation_logs LIMIT 0"},
		{name: "INSERT operation_logs", query: "INSERT INTO operation_logs (stream_key, stream_seq, actor_type, user_id, username, method, path, status, created_at) SELECT 'permission-probe', 0, 'system', 0, 'system', 'GET', '/permission-probe', 200, NOW() FROM DUAL WHERE FALSE"},
		{name: "SELECT audit_streams", query: "SELECT stream_key FROM audit_streams LIMIT 0"},
		{name: "INSERT audit_streams", query: "INSERT INTO audit_streams (stream_key, next_seq, last_hash, updated_at) SELECT 'permission-probe', 1, '', NOW() FROM DUAL WHERE FALSE"},
		{name: "UPDATE audit_streams", query: "UPDATE audit_streams SET stream_key = stream_key WHERE 1 = 0"},
		{name: "SELECT audit_archives", query: "SELECT id FROM audit_archives LIMIT 0"},
	} {
		if err := businessProbeDB.Exec(probe.query).Error; err != nil {
			return fmt.Errorf("business database account %s requires %s privilege: %w", businessUser, probe.name, err)
		}
	}
	for _, probe := range []struct {
		name  string
		query string
	}{
		{name: "UPDATE operation_logs", query: "UPDATE operation_logs SET id = id WHERE 1 = 0"},
		{name: "DELETE operation_logs", query: "DELETE FROM operation_logs WHERE 1 = 0"},
		{name: "INSERT audit_archives", query: "INSERT INTO audit_archives (id, stream_key, status, from_seq, to_seq, event_count, last_hash, events_sha256, manifest_hash, object_key, created_at, updated_at) SELECT 'permission-probe', 'global', 'queued', 1, 1, 1, '', '', '', '', NOW(), NOW() FROM DUAL WHERE FALSE"},
		{name: "UPDATE audit_archives", query: "UPDATE audit_archives SET id = id WHERE 1 = 0"},
		{name: "DELETE audit_archives", query: "DELETE FROM audit_archives WHERE 1 = 0"},
	} {
		err := businessProbeDB.Exec(probe.query).Error
		if err == nil {
			return fmt.Errorf("business database account %s must not have %s privilege", businessUser, probe.name)
		}
		if !isPermissionDenied(err) {
			return fmt.Errorf("verify business database %s denial: %w", probe.name, err)
		}
	}
	for _, probe := range []struct {
		name  string
		query string
	}{
		{name: "SELECT operation_logs", query: "SELECT id FROM operation_logs LIMIT 0"},
		{name: "DELETE operation_logs", query: "DELETE FROM operation_logs WHERE 1 = 0"},
		{name: "SELECT audit_archives", query: "SELECT id FROM audit_archives LIMIT 0"},
		{name: "INSERT audit_archives", query: "INSERT INTO audit_archives (id, stream_key, status, from_seq, to_seq, event_count, last_hash, events_sha256, manifest_hash, object_key, created_at, updated_at) SELECT 'permission-probe', 'global', 'queued', 1, 1, 1, '', '', '', '', NOW(), NOW() FROM DUAL WHERE FALSE"},
		{name: "UPDATE audit_archives", query: "UPDATE audit_archives SET id = id WHERE 1 = 0"},
	} {
		if err := archiveProbeDB.Exec(probe.query).Error; err != nil {
			return fmt.Errorf("audit archive account %s requires %s privilege: %w", archiveUser, probe.name, err)
		}
	}
	for _, probe := range []struct {
		name  string
		query string
	}{
		{name: "INSERT operation_logs", query: "INSERT INTO operation_logs (stream_key, stream_seq, actor_type, user_id, username, method, path, status, created_at) SELECT 'permission-probe', 0, 'system', 0, 'system', 'GET', '/permission-probe', 200, NOW() FROM DUAL WHERE FALSE"},
		{name: "UPDATE operation_logs", query: "UPDATE operation_logs SET id = id WHERE 1 = 0"},
	} {
		err := archiveProbeDB.Exec(probe.query).Error
		if err == nil {
			return fmt.Errorf("audit archive account %s must not have %s privilege", archiveUser, probe.name)
		}
		if !isPermissionDenied(err) {
			return fmt.Errorf("verify audit archive database %s denial: %w", probe.name, err)
		}
	}
	return nil
}

func currentUser(db *gorm.DB) (string, error) {
	var user string
	err := db.Raw("SELECT CURRENT_USER()").Scan(&user).Error
	return user, err
}

func isPermissionDenied(err error) bool {
	var mysqlErr *mysqldriver.MySQLError
	return errors.As(err, &mysqlErr) && (mysqlErr.Number == 1142 || mysqlErr.Number == 1143)
}
