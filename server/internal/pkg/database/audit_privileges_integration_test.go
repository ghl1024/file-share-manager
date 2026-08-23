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
	"fmt"
	"os"
	"strings"
	"testing"

	"file-share-manager/server/internal/model"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestAuditPrivilegeIsolationMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run MySQL audit privilege tests")
	}
	adminConfig, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse test MySQL DSN: %v", err)
	}
	adminConfig.ParseTime = true
	adminConfig.DBName = ""
	adminDB, err := gorm.Open(mysql.Open(adminConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect test MySQL: %v", err)
	}

	suffix := strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	databaseName := "fs_audit_" + suffix
	businessUser := "fs_app_" + suffix
	archiveUser := "fs_arc_" + suffix
	businessPassword := "App" + suffix + "!9"
	archivePassword := "Arc" + suffix + "!9"
	t.Cleanup(func() {
		_ = adminDB.Exec(fmt.Sprintf("DROP USER IF EXISTS `%s`@'%%'", businessUser)).Error
		_ = adminDB.Exec(fmt.Sprintf("DROP USER IF EXISTS `%s`@'%%'", archiveUser)).Error
		_ = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName)).Error
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", databaseName)).Error; err != nil {
		t.Fatalf("create temporary audit database: %v", err)
	}
	schemaConfig := *adminConfig
	schemaConfig.DBName = databaseName
	schemaDB, err := gorm.Open(mysql.Open(schemaConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect temporary audit database: %v", err)
	}
	if err := schemaDB.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatalf("migrate temporary audit database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := schemaDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	statements := []string{
		fmt.Sprintf("CREATE USER `%s`@'%%' IDENTIFIED BY '%s'", businessUser, businessPassword),
		fmt.Sprintf("CREATE USER `%s`@'%%' IDENTIFIED BY '%s'", archiveUser, archivePassword),
		fmt.Sprintf("GRANT SELECT, INSERT ON `%s`.`operation_logs` TO `%s`@'%%'", databaseName, businessUser),
		fmt.Sprintf("GRANT SELECT, INSERT, UPDATE ON `%s`.`audit_streams` TO `%s`@'%%'", databaseName, businessUser),
		fmt.Sprintf("GRANT SELECT ON `%s`.`audit_archives` TO `%s`@'%%'", databaseName, businessUser),
		fmt.Sprintf("GRANT SELECT, DELETE ON `%s`.`operation_logs` TO `%s`@'%%'", databaseName, archiveUser),
		fmt.Sprintf("GRANT SELECT, INSERT, UPDATE ON `%s`.`audit_archives` TO `%s`@'%%'", databaseName, archiveUser),
	}
	for _, statement := range statements {
		if err := adminDB.Exec(statement).Error; err != nil {
			t.Fatalf("apply temporary audit grant: %v", err)
		}
	}

	businessDB := openRestrictedTestDB(t, adminConfig, databaseName, businessUser, businessPassword)
	archiveDB := openRestrictedTestDB(t, adminConfig, databaseName, archiveUser, archivePassword)
	if err := VerifyAuditPrivilegeIsolation(businessDB, archiveDB); err != nil {
		t.Fatalf("VerifyAuditPrivilegeIsolation() error = %v", err)
	}
	if err := VerifyAuditPrivilegeIsolation(schemaDB, archiveDB); err == nil || !strings.Contains(err.Error(), "must not have UPDATE operation_logs") {
		t.Fatalf("broad business privilege error = %v", err)
	}

	if err := schemaDB.Exec("INSERT INTO operation_logs (stream_key, stream_seq, actor_type, user_id, username, method, path, status, created_at) VALUES ('permission-test', 1, 'system', 0, 'system', 'GET', '/permission-test', 200, NOW())").Error; err != nil {
		t.Fatal(err)
	}
	if result := archiveDB.Exec("DELETE FROM operation_logs WHERE stream_key = 'permission-test'"); result.Error != nil || result.RowsAffected != 1 {
		t.Fatalf("archive account delete result = rows %d, error %v", result.RowsAffected, result.Error)
	}
}

func openRestrictedTestDB(t *testing.T, base *mysqldriver.Config, databaseName, user, password string) *gorm.DB {
	t.Helper()
	cfg := *base
	cfg.DBName = databaseName
	cfg.User = user
	cfg.Passwd = password
	cfg.ParseTime = true
	db, err := gorm.Open(mysql.Open(cfg.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect restricted MySQL account %s: %v", user, err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
