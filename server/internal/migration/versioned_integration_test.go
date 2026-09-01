/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package migration

import (
	"os"
	"strings"
	"testing"

	"file-share-manager/server/internal/pkg/testsql"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestVersionedMigrationLifecycleMySQL(t *testing.T) {
	db := openVersionedMigrationTestDB(t)
	if err := VerifyCurrent(db); err == nil || !strings.Contains(err.Error(), "ledger is missing") {
		t.Fatalf("VerifyCurrent() before migration error = %v", err)
	}
	first, err := RunVersioned(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Applied) != 1 || first.Applied[0] != CurrentVersion || len(first.Skipped) != 0 {
		t.Fatalf("first migration report = %#v", first)
	}
	second, err := RunVersioned(db)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Applied) != 0 || len(second.Skipped) != 1 || second.Skipped[0] != CurrentVersion {
		t.Fatalf("second migration report = %#v", second)
	}
	if err := VerifyCurrent(db); err != nil {
		t.Fatalf("VerifyCurrent() error = %v", err)
	}
	var receiptCount int64
	if err := db.Model(&SchemaMigration{}).Count(&receiptCount).Error; err != nil || receiptCount != 1 {
		t.Fatalf("receipt count = %d, error = %v", receiptCount, err)
	}
	if err := db.Model(&SchemaMigration{}).Where("version = ?", CurrentVersion).Update("checksum", strings.Repeat("0", 64)).Error; err != nil {
		t.Fatal(err)
	}
	if err := VerifyCurrent(db); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("VerifyCurrent() after tampering error = %v", err)
	}
	if _, err := RunVersioned(db); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("RunVersioned() after tampering error = %v", err)
	}
}

func openVersionedMigrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run versioned migration tests")
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
	databaseName := "fs_migration_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	databaseIdentifier, err := testsql.Identifier(databaseName)
	if err != nil {
		t.Fatalf("quote temporary migration database: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + databaseIdentifier + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create temporary migration database: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + databaseIdentifier).Error
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	testConfig := *adminConfig
	testConfig.DBName = databaseName
	db, err := gorm.Open(mysql.Open(testConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect temporary migration database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
