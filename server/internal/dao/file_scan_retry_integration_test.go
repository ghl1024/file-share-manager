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
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
)

func TestFileScanRetryQueueMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.FileVersion{}, &model.ChangeLog{}); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 14, 15, 0, 0, 0, time.Local)
	past := now.Add(-time.Minute)
	stale := now.Add(-10 * time.Minute)
	versions := []model.FileVersion{
		{WorkspaceID: 1, NodeID: 11, VersionNo: 1, StorageKey: "objects/1/retry", StorageClass: "standard", Size: 1, SHA256: strings.Repeat("a", 64), ScanStatus: "scan_error", ScanNextRetryAt: &past},
		{WorkspaceID: 1, NodeID: 12, VersionNo: 1, StorageKey: "objects/1/exhausted", StorageClass: "standard", Size: 1, SHA256: strings.Repeat("b", 64), ScanStatus: "scan_error", ScanRetryCount: 3},
		{WorkspaceID: 1, NodeID: 13, VersionNo: 1, StorageKey: "objects/1/stale", StorageClass: "standard", Size: 1, SHA256: strings.Repeat("c", 64), ScanStatus: "pending_scan", ScanRetryCount: 1, ScanLastAttemptAt: &stale},
	}
	if err := db.Create(&versions).Error; err != nil {
		t.Fatal(err)
	}
	files := &FileDAO{db: db}

	candidates, err := files.ListScanRetryCandidates(now, 3, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 1 || candidates[0].ID != versions[0].ID {
		t.Fatalf("candidates = %#v", candidates)
	}
	claimed, err := files.ClaimScanRetry(versions[0].ID, 0, now)
	if err != nil || !claimed {
		t.Fatalf("ClaimScanRetry() claimed = %v, err = %v", claimed, err)
	}
	claimed, err = files.ClaimScanRetry(versions[0].ID, 0, now)
	if err != nil || claimed {
		t.Fatalf("duplicate ClaimScanRetry() claimed = %v, err = %v", claimed, err)
	}
	next := now.Add(5 * time.Minute)
	completed, err := files.CompleteScanRetry(versions[0].ID, "scan_error", "timeout", &next)
	if err != nil || !completed {
		t.Fatalf("CompleteScanRetry() completed = %v, err = %v", completed, err)
	}

	recovered, err := files.RequeueStaleScanRetries(now.Add(-5*time.Minute), now)
	if err != nil || recovered != 1 {
		t.Fatalf("RequeueStaleScanRetries() recovered = %d, err = %v", recovered, err)
	}
	summary, err := files.ScanRetrySummary(3)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Retryable != 2 || summary.Pending != 0 || summary.Exhausted != 1 || summary.NextRetryAt == nil || !summary.NextRetryAt.Equal(now) {
		t.Fatalf("summary = %#v", summary)
	}
	var persisted model.FileVersion
	if err := db.First(&persisted, versions[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if persisted.ScanStatus != "scan_error" || persisted.ScanRetryCount != 1 || persisted.ScanLastAttemptAt == nil || persisted.ScanNextRetryAt == nil || !persisted.ScanNextRetryAt.Equal(next) {
		t.Fatalf("persisted retry = %#v", persisted)
	}
	var changeCount int64
	if err := db.Model(&model.ChangeLog{}).Where("entity_type = ? AND entity_id = ?", "file_version", versions[0].ID).Count(&changeCount).Error; err != nil {
		t.Fatal(err)
	}
	if changeCount != 2 {
		t.Fatalf("scan retry change log count = %d, want 2", changeCount)
	}
}
