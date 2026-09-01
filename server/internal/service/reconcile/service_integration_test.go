/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package reconcile

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/testsql"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestQuarantineLifecycleAndReferenceProtectionMySQL(t *testing.T) {
	db := openReconcileTestDB(t)
	if err := db.AutoMigrate(
		&model.FileVersion{}, &model.Share{}, &model.ShareItem{},
		&model.BatchDownloadJob{}, &model.BatchDownloadItem{},
		&model.StorageQuarantine{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}
	rootPath := t.TempDir()
	stagingPath := t.TempDir()
	previousConfig := config.GetConfig()
	previousDB := database.DB
	config.SetTestConfig(&config.Config{
		Storage:   config.StorageConfig{RootPath: rootPath, StagingPath: stagingPath, Mode: "local"},
		Lifecycle: config.LifecycleConfig{QuarantineRetentionDays: 7, ReconcileBatchSize: 10},
	})
	database.DB = db
	t.Cleanup(func() {
		config.SetTestConfig(previousConfig)
		database.DB = previousDB
	})

	const workspaceID uint = 17
	purgeKey := "objects/17/purge-me"
	writeReconcileObject(t, rootPath, purgeKey, "purge")
	report, err := QuarantineWorkspaceOrphans(context.Background(), workspaceID, 3, []string{purgeKey})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.QuarantinedObjects) != 1 || len(report.QuarantineRecords) != 1 {
		t.Fatalf("quarantine report = %#v", report)
	}
	makeQuarantineDue(t, db, purgeKey)
	purgeReport, err := PurgeDueQuarantines(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(purgeReport.Purged) != 1 || purgeReport.Purged[0] != purgeKey {
		t.Fatalf("purge report = %#v", purgeReport)
	}
	assertQuarantineStatus(t, db, purgeKey, model.StorageQuarantineStatusPurged, 0)
	if _, err := os.Stat(filepath.Join(rootPath, filepath.FromSlash(purgeKey))); !os.IsNotExist(err) {
		t.Fatalf("purged primary object stat error = %v", err)
	}

	restoreKey := "objects/17/restore-me"
	writeReconcileObject(t, rootPath, restoreKey, "restore")
	if _, err := QuarantineWorkspaceOrphans(context.Background(), workspaceID, 3, []string{restoreKey}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.FileVersion{
		WorkspaceID: workspaceID, NodeID: 99, VersionNo: 1, StorageKey: restoreKey,
		StorageClass: "standard", Size: 7, SHA256: strings.Repeat("b", 64), ScanStatus: "clean",
	}).Error; err != nil {
		t.Fatal(err)
	}
	makeQuarantineDue(t, db, restoreKey)
	purgeReport, err = PurgeDueQuarantines(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(purgeReport.Restored) != 1 || purgeReport.Restored[0] != restoreKey {
		t.Fatalf("reference-protected report = %#v", purgeReport)
	}
	assertQuarantineStatus(t, db, restoreKey, model.StorageQuarantineStatusRestored, 0)
	content, err := os.ReadFile(filepath.Join(rootPath, filepath.FromSlash(restoreKey)))
	if err != nil || string(content) != "restore" {
		t.Fatalf("restored object content = %q, error = %v", content, err)
	}

	retryKey := "objects/17/retry-me"
	writeReconcileObject(t, rootPath, retryKey, "retry")
	if _, err := QuarantineWorkspaceOrphans(context.Background(), workspaceID, 3, []string{retryKey}); err != nil {
		t.Fatal(err)
	}
	var retryRecord model.StorageQuarantine
	if err := db.Where("storage_key = ?", retryKey).First(&retryRecord).Error; err != nil {
		t.Fatal(err)
	}
	validQuarantineKey := retryRecord.QuarantineKey
	if err := db.Model(&retryRecord).Updates(map[string]any{
		"quarantine_key": "quarantine/18/cross-workspace", "purge_after": time.Now().Add(-time.Minute),
	}).Error; err != nil {
		t.Fatal(err)
	}
	purgeReport, err = PurgeDueQuarantines(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(purgeReport.Failed) != 1 {
		t.Fatalf("retry failure report = %#v", purgeReport)
	}
	assertQuarantineStatus(t, db, retryKey, model.StorageQuarantineStatusQuarantined, 1)
	if err := db.Model(&retryRecord).Update("quarantine_key", validQuarantineKey).Error; err != nil {
		t.Fatal(err)
	}
	purgeReport, err = PurgeDueQuarantines(context.Background(), time.Now(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(purgeReport.Purged) != 1 || purgeReport.Purged[0] != retryKey {
		t.Fatalf("retry recovery report = %#v", purgeReport)
	}
	assertQuarantineStatus(t, db, retryKey, model.StorageQuarantineStatusPurged, 1)

	var changeCount int64
	if err := db.Model(&model.ChangeLog{}).Where("entity_type = ?", "storage_object").Count(&changeCount).Error; err != nil {
		t.Fatal(err)
	}
	if changeCount != 6 {
		t.Fatalf("storage-object change log count = %d, want 6", changeCount)
	}
}

func writeReconcileObject(t *testing.T, rootPath, storageKey, content string) {
	t.Helper()
	path := filepath.Join(rootPath, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}

func makeQuarantineDue(t *testing.T, db *gorm.DB, storageKey string) {
	t.Helper()
	if err := db.Model(&model.StorageQuarantine{}).Where("storage_key = ?", storageKey).
		Update("purge_after", time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
}

func assertQuarantineStatus(t *testing.T, db *gorm.DB, storageKey, status string, retryCount int) {
	t.Helper()
	var record model.StorageQuarantine
	if err := db.Where("storage_key = ?", storageKey).First(&record).Error; err != nil {
		t.Fatal(err)
	}
	if record.Status != status || record.RetryCount != retryCount {
		t.Fatalf("quarantine %s status = %q retry_count = %d, want %q/%d", storageKey, record.Status, record.RetryCount, status, retryCount)
	}
}

func openReconcileTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run storage reconciliation tests")
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
	databaseName := "fs_reconcile_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	databaseIdentifier, err := testsql.Identifier(databaseName)
	if err != nil {
		t.Fatalf("quote temporary reconciliation database: %v", err)
	}
	if err := adminDB.Exec("CREATE DATABASE " + databaseIdentifier + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create temporary reconciliation database: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + databaseIdentifier).Error
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	testConfig := *adminConfig
	testConfig.DBName = databaseName
	testDB, err := gorm.Open(mysql.Open(testConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect temporary reconciliation database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := testDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return testDB
}
