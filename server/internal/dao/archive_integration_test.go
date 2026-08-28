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
	"testing"
	"time"

	"file-share-manager/server/internal/model"
)

func TestArchiveCompletionUpdatesAllSnapshotsMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.FileVersion{}, &model.Share{}, &model.ShareItem{}, &model.BatchDownloadJob{}, &model.BatchDownloadItem{}, &model.ChangeLog{}); err != nil {
		t.Fatal(err)
	}
	createdAt := time.Now().Add(-400 * 24 * time.Hour)
	version := model.FileVersion{WorkspaceID: 7, NodeID: 11, VersionNo: 1, StorageKey: "objects/7/test", StorageClass: "standard", Size: 4, SHA256: "hash", CreatedAt: createdAt}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	share := model.Share{WorkspaceID: 7, SourceNodeID: 11, PublicID: "share-public", TokenHash: "share-token", Name: "test", RootType: "file", RootName: "test", ExpiresAt: time.Now().Add(time.Hour), Status: "active", CreatedBy: 1}
	if err := db.Create(&share).Error; err != nil {
		t.Fatal(err)
	}
	shareItem := model.ShareItem{ShareID: share.ID, PublicID: "item-public", Name: "test", VersionNo: 1, StorageKey: version.StorageKey, StorageClass: "standard", Size: 4, SHA256: "hash", ScanStatus: "clean"}
	if err := db.Create(&shareItem).Error; err != nil {
		t.Fatal(err)
	}
	job := model.BatchDownloadJob{ID: "job-archive", WorkspaceID: 7, CreatedBy: 1, Name: "test", Status: "queued", TotalFiles: 1, TotalBytes: 4}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	batchItem := model.BatchDownloadItem{JobID: job.ID, NodeID: 11, VersionID: version.ID, RelativePath: "test", StorageKey: version.StorageKey, StorageClass: "standard", Size: 4, SHA256: "hash"}
	if err := db.Create(&batchItem).Error; err != nil {
		t.Fatal(err)
	}
	dao := &ArchiveDAO{db: db}
	candidates, err := dao.ListCandidates(time.Now().Add(-365*24*time.Hour), 10)
	if err != nil || len(candidates) != 1 {
		t.Fatalf("candidates = %#v, error = %v", candidates, err)
	}
	if err := dao.Complete(candidates[0], time.Now().Add(-365*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	for table, id := range map[string]any{"file_versions": version.ID, "share_items": shareItem.ID, "batch_download_items": batchItem.ID} {
		var storageClass string
		if err := db.Table(table).Select("storage_class").Where("id = ?", id).Scan(&storageClass).Error; err != nil {
			t.Fatal(err)
		}
		if storageClass != "archive" {
			t.Fatalf("%s storage_class = %q", table, storageClass)
		}
	}
	assertTableCount(t, db, &model.ChangeLog{}, 1)
}

func TestArchiveCompletionRejectsRecentAccessMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.FileVersion{}, &model.Share{}, &model.ShareItem{}, &model.BatchDownloadJob{}, &model.BatchDownloadItem{}, &model.ChangeLog{}); err != nil {
		t.Fatal(err)
	}
	accessedAt := time.Now()
	version := model.FileVersion{WorkspaceID: 7, NodeID: 11, VersionNo: 1, StorageKey: "objects/7/recent", StorageClass: "standard", Size: 4, SHA256: "hash", CreatedAt: time.Now().Add(-400 * 24 * time.Hour), LastAccessedAt: &accessedAt}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	err := (&ArchiveDAO{db: db}).Complete(ArchiveCandidate{WorkspaceID: 7, StorageKey: version.StorageKey, Size: 4, SHA256: "hash"}, time.Now().Add(-365*24*time.Hour))
	if !errors.Is(err, ErrArchiveCandidateChanged) {
		t.Fatalf("Complete() error = %v", err)
	}
}
