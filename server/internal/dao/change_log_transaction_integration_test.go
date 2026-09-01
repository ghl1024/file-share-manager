/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/testsql"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	errInjectedChangeLog = errors.New("injected change log failure")
	errInjectedBusiness  = errors.New("injected business mutation failure")
)

func TestChangeLogFailureRollsBackNodeMutationMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.Node{}, &model.NodeClosure{}, &model.ChangeLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_change_log", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (model.ChangeLog{}).TableName() {
			tx.AddError(errInjectedChangeLog)
		}
	}); err != nil {
		t.Fatal(err)
	}

	node := &model.Node{WorkspaceID: 41, Name: "rollback", NormalizedName: "rollback", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: 1, UpdatedBy: 1}
	err := (&NodeDAO{db: db}).CreateNode(node)
	if !errors.Is(err, errInjectedChangeLog) {
		t.Fatalf("CreateNode() error = %v", err)
	}
	assertTableCount(t, db, &model.Node{}, 0)
	assertTableCount(t, db, &model.NodeClosure{}, 0)
}

func TestBusinessFailureAfterChangeLogRollsBackUploadMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.Workspace{}, &model.WorkspaceMembership{}, &model.UploadSession{},
		&model.Node{}, &model.NodeClosure{}, &model.FileVersion{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}
	workspace := model.Workspace{UUID: uuid.NewString(), Name: "transaction test", Code: "transaction-test", Status: 1, ReservedBytes: 10, CreatedBy: 1}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	session := model.UploadSession{
		ID: "upload-rollback", WorkspaceID: workspace.ID, DisplayName: "rollback.txt", TotalSize: 10,
		ChunkSize: 10, TotalChunks: 1, ReceivedChunks: "[0]", Status: "uploading",
		ExpiresAt: time.Now().Add(time.Hour), CreatedBy: 1,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("test:fail_workspace_update", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (model.Workspace{}).TableName() {
			tx.AddError(errInjectedBusiness)
		}
	}); err != nil {
		t.Fatal(err)
	}

	node := &model.Node{Name: "rollback.txt", NormalizedName: "rollback.txt", Type: "file", InheritMode: "inherit", Status: "active", CreatedBy: 1, UpdatedBy: 1}
	version := &model.FileVersion{StorageKey: "41/rollback", Size: 10, SHA256: strings.Repeat("a", 64), Extension: ".txt", ScanStatus: "clean", CreatedBy: 1}
	err := (&UploadDAO{db: db}).FinalizeSession(session.ID, workspace.ID, 1, version, node)
	if !errors.Is(err, errInjectedBusiness) {
		t.Fatalf("FinalizeSession() error = %v", err)
	}
	assertTableCount(t, db, &model.Node{}, 0)
	assertTableCount(t, db, &model.NodeClosure{}, 0)
	assertTableCount(t, db, &model.FileVersion{}, 0)
	assertTableCount(t, db, &model.ChangeLog{}, 0)

	var persisted model.UploadSession
	if err := db.First(&persisted, "id = ?", session.ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.Status != "uploading" {
		t.Fatalf("upload status = %q, want uploading", persisted.Status)
	}
}

func openTransactionTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run MySQL transaction failure tests")
	}
	adminConfig, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse test MySQL DSN: %v", err)
	}
	// Integration tests read GORM time fields directly. Callers commonly pass
	// a minimal root DSN without parseTime, so make the test connection
	// deterministic instead of requiring every shell command to repeat it.
	adminConfig.ParseTime = true
	adminConfig.DBName = ""
	adminDB, err := gorm.Open(mysql.Open(adminConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect test MySQL: %v", err)
	}
	databaseName := "fileshare_tx_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:16]
	databaseIdentifier, err := testsql.Identifier(databaseName)
	if err != nil {
		t.Fatalf("quote temporary transaction database: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + databaseIdentifier + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create temporary test database: %v", err)
	}

	testConfig := *adminConfig
	testConfig.DBName = databaseName
	db, err := gorm.Open(mysql.Open(testConfig.FormatDSN()), &gorm.Config{
		Logger:                                   logger.Default.LogMode(logger.Silent),
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		_ = adminDB.Exec("DROP DATABASE " + databaseIdentifier).Error
		t.Fatalf("connect temporary test database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
		if dropErr := adminDB.Exec("DROP DATABASE IF EXISTS " + databaseIdentifier).Error; dropErr != nil {
			t.Errorf("drop temporary test database: %v", dropErr)
		}
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func assertTableCount(t *testing.T, db *gorm.DB, value any, expected int64) {
	t.Helper()
	var count int64
	if err := db.Model(value).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("%T count = %d, want %d", value, count, expected)
	}
}
