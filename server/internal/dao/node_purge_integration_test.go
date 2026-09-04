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
	"slices"
	"testing"
	"time"

	"file-share-manager/server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestPurgeExpiredTrashDefersLiveSnapshotsMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	migrateNodePurgeModels(t, db)
	now := time.Now()
	fixture := createNodePurgeFixture(t, db, now)

	activeShare := model.Share{
		WorkspaceID: fixture.workspace.ID, SourceNodeID: fixture.root.ID, PublicID: uuid.NewString(), TokenHash: uuid.NewString(),
		Name: "有效分享", RootType: "folder", RootName: fixture.root.Name, ExpiresAt: now.Add(time.Hour), Status: "active", CreatedBy: fixture.user.ID,
	}
	if err := db.Create(&activeShare).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ShareItem{
		ShareID: activeShare.ID, PublicID: uuid.NewString(), Name: fixture.child.Name, VersionNo: 1,
		StorageKey: fixture.version.StorageKey, Size: fixture.version.Size, SHA256: fixture.version.SHA256, ScanStatus: "clean",
	}).Error; err != nil {
		t.Fatal(err)
	}
	batch := model.BatchDownloadJob{
		ID: uuid.NewString(), WorkspaceID: fixture.workspace.ID, CreatedBy: fixture.user.ID, Name: "有效批量下载",
		Status: "completed", TotalFiles: 1, TotalBytes: fixture.version.Size, ExpiresAt: timePointer(now.Add(time.Hour)),
	}
	if err := db.Create(&batch).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BatchDownloadItem{
		JobID: batch.ID, NodeID: fixture.child.ID, VersionID: fixture.version.ID, RelativePath: fixture.child.Name,
		StorageKey: fixture.version.StorageKey, Size: fixture.version.Size, SHA256: fixture.version.SHA256,
	}).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (&NodeDAO{db: db}).PurgeExpiredTrash(now.Add(-30*24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Versions) != 0 || len(result.UploadIDs) != 0 || len(result.BatchArchiveIDs) != 0 {
		t.Fatalf("live snapshots unexpectedly produced purge result: %#v", result)
	}
	assertTableCount(t, db, &model.Node{}, 2)
	assertTableCount(t, db, &model.FileVersion{}, 1)
}

func TestPurgeExpiredTrashRemovesNodeOwnedDataMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	migrateNodePurgeModels(t, db)
	now := time.Now()
	fixture := createNodePurgeFixture(t, db, now)

	if err := db.Create(&model.NodeACL{WorkspaceID: fixture.workspace.ID, NodeID: fixture.child.ID, SubjectType: "user", SubjectID: fixture.user.ID, Effect: "allow", AccessLevel: "read"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.Favorite{WorkspaceID: fixture.workspace.ID, UserID: fixture.user.ID, NodeID: fixture.child.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.RecentNodeAccess{WorkspaceID: fixture.workspace.ID, UserID: fixture.user.ID, NodeID: fixture.child.ID, LastAccessedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	comment := model.NodeComment{WorkspaceID: fixture.workspace.ID, NodeID: fixture.child.ID, AuthorID: fixture.user.ID, Content: "待清理评论", Revision: 1}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeCommentMention{CommentID: comment.ID, UserID: fixture.user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	expiredShare := model.Share{
		WorkspaceID: fixture.workspace.ID, SourceNodeID: fixture.root.ID, PublicID: uuid.NewString(), TokenHash: uuid.NewString(),
		Name: "过期分享", RootType: "folder", RootName: fixture.root.Name, ExpiresAt: now.Add(-time.Hour), Status: "expired", CreatedBy: fixture.user.ID,
	}
	if err := db.Create(&expiredShare).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.ShareItem{
		ShareID: expiredShare.ID, PublicID: uuid.NewString(), Name: fixture.child.Name, VersionNo: 1,
		StorageKey: fixture.version.StorageKey, Size: fixture.version.Size, SHA256: fixture.version.SHA256, ScanStatus: "clean",
	}).Error; err != nil {
		t.Fatal(err)
	}
	expiredBatch := model.BatchDownloadJob{
		ID: uuid.NewString(), WorkspaceID: fixture.workspace.ID, CreatedBy: fixture.user.ID, Name: "过期批量下载",
		Status: "expired", TotalFiles: 1, TotalBytes: fixture.version.Size, ExpiresAt: timePointer(now.Add(-time.Hour)),
	}
	if err := db.Create(&expiredBatch).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.BatchDownloadItem{
		JobID: expiredBatch.ID, NodeID: fixture.child.ID, VersionID: fixture.version.ID, RelativePath: fixture.child.Name,
		StorageKey: fixture.version.StorageKey, Size: fixture.version.Size, SHA256: fixture.version.SHA256,
	}).Error; err != nil {
		t.Fatal(err)
	}
	upload := model.UploadSession{
		ID: uuid.NewString(), WorkspaceID: fixture.workspace.ID, NodeID: &fixture.child.ID, TargetParentID: &fixture.root.ID,
		DisplayName: fixture.child.Name, TotalSize: 5, ChunkSize: 5, TotalChunks: 1, ReceivedChunks: "[]",
		Status: "uploading", ExpiresAt: now.Add(time.Hour), CreatedBy: fixture.user.ID,
	}
	if err := db.Create(&upload).Error; err != nil {
		t.Fatal(err)
	}

	result, err := (&NodeDAO{db: db}).PurgeExpiredTrash(now.Add(-30*24*time.Hour), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Versions) != 1 || result.Versions[0].ID != fixture.version.ID {
		t.Fatalf("purged versions = %#v", result.Versions)
	}
	if !slices.Equal(result.UploadIDs, []string{upload.ID}) {
		t.Fatalf("purged upload IDs = %#v, want %q", result.UploadIDs, upload.ID)
	}
	if !slices.Equal(result.BatchArchiveIDs, []string{expiredBatch.ID}) {
		t.Fatalf("purged batch archive IDs = %#v, want %q", result.BatchArchiveIDs, expiredBatch.ID)
	}
	for _, table := range []any{
		&model.Node{}, &model.NodeClosure{}, &model.FileVersion{}, &model.NodeACL{}, &model.Favorite{},
		&model.RecentNodeAccess{}, &model.NodeComment{}, &model.NodeCommentMention{}, &model.Share{},
		&model.ShareItem{}, &model.BatchDownloadItem{}, &model.UploadSession{},
	} {
		assertTableCount(t, db, table, 0)
	}
	assertTableCount(t, db, &model.BatchDownloadJob{}, 1)
	var batchAfter model.BatchDownloadJob
	if err := db.First(&batchAfter, "id = ?", expiredBatch.ID).Error; err != nil {
		t.Fatal(err)
	}
	if batchAfter.Status != "expired" || batchAfter.ArchiveSize != 0 {
		t.Fatalf("batch after purge = %#v", batchAfter)
	}
	var workspaceAfter model.Workspace
	if err := db.First(&workspaceAfter, fixture.workspace.ID).Error; err != nil {
		t.Fatal(err)
	}
	if workspaceAfter.UsedBytes != 0 || workspaceAfter.ReservedBytes != 0 {
		t.Fatalf("workspace usage after purge = used %d, reserved %d", workspaceAfter.UsedBytes, workspaceAfter.ReservedBytes)
	}
	var membershipAfter model.WorkspaceMembership
	if err := db.Where("workspace_id = ? AND user_id = ?", fixture.workspace.ID, fixture.user.ID).First(&membershipAfter).Error; err != nil {
		t.Fatal(err)
	}
	if membershipAfter.UsedBytes != 0 || membershipAfter.ReservedBytes != 0 {
		t.Fatalf("member usage after purge = used %d, reserved %d", membershipAfter.UsedBytes, membershipAfter.ReservedBytes)
	}
}

func TestPurgeExpiredTrashRollsBackRelationsOnChangeLogFailureMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	migrateNodePurgeModels(t, db)
	now := time.Now()
	fixture := createNodePurgeFixture(t, db, now)
	if err := db.Create(&model.NodeACL{WorkspaceID: fixture.workspace.ID, NodeID: fixture.child.ID, SubjectType: "user", SubjectID: fixture.user.ID, Effect: "allow", AccessLevel: "read"}).Error; err != nil {
		t.Fatal(err)
	}
	comment := model.NodeComment{WorkspaceID: fixture.workspace.ID, NodeID: fixture.child.ID, AuthorID: fixture.user.ID, Content: "回滚评论", Revision: 1}
	if err := db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeCommentMention{CommentID: comment.ID, UserID: fixture.user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected node purge change log failure")
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_node_purge_change_log", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (model.ChangeLog{}).TableName() {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	_, err := (&NodeDAO{db: db}).PurgeExpiredTrash(now.Add(-30*24*time.Hour), now)
	if !errors.Is(err, failure) {
		t.Fatalf("PurgeExpiredTrash() error = %v, want %v", err, failure)
	}
	assertTableCount(t, db, &model.Node{}, 2)
	assertTableCount(t, db, &model.NodeClosure{}, 3)
	assertTableCount(t, db, &model.FileVersion{}, 1)
	assertTableCount(t, db, &model.NodeACL{}, 1)
	assertTableCount(t, db, &model.NodeComment{}, 1)
	assertTableCount(t, db, &model.NodeCommentMention{}, 1)
}

type nodePurgeFixture struct {
	user      model.User
	workspace model.Workspace
	root      model.Node
	child     model.Node
	version   model.FileVersion
}

func migrateNodePurgeModels(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.AutoMigrate(
		&model.User{}, &model.Workspace{}, &model.WorkspaceMembership{},
		&model.Node{}, &model.NodeClosure{}, &model.NodeACL{}, &model.FileVersion{}, &model.UploadSession{},
		&model.Favorite{}, &model.RecentNodeAccess{}, &model.NodeComment{}, &model.NodeCommentMention{},
		&model.Share{}, &model.ShareItem{}, &model.BatchDownloadJob{}, &model.BatchDownloadItem{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}
}

func createNodePurgeFixture(t *testing.T, db *gorm.DB, now time.Time) nodePurgeFixture {
	t.Helper()
	user := model.User{Username: "node-purge-user", PasswordHash: "unused", RealName: "节点清理用户", Status: 1, AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.Workspace{
		UUID: uuid.NewString(), Name: "节点清理空间", Code: "node-purge", Status: 1,
		UsedBytes: 10, ReservedBytes: 5, CreatedBy: user.ID,
	}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkspaceMembership{
		WorkspaceID: workspace.ID, UserID: user.ID, Role: "workspace_admin", UsedBytes: 10, ReservedBytes: 5,
	}).Error; err != nil {
		t.Fatal(err)
	}
	trashedAt := now.Add(-31 * 24 * time.Hour)
	root := model.Node{
		WorkspaceID: workspace.ID, Name: "待清理目录", NormalizedName: "待清理目录", Type: "folder",
		InheritMode: "inherit", Status: "trashed", TrashedAt: &trashedAt, CreatedBy: user.ID, UpdatedBy: user.ID,
	}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	child := model.Node{
		WorkspaceID: workspace.ID, ParentID: &root.ID, Name: "待清理文件.txt", NormalizedName: "待清理文件.txt", Type: "file",
		InheritMode: "inherit", Status: "trashed", TrashedAt: &trashedAt, CreatedBy: user.ID, UpdatedBy: user.ID,
	}
	if err := db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.NodeClosure{
		{AncestorID: root.ID, DescendantID: root.ID, Depth: 0},
		{AncestorID: root.ID, DescendantID: child.ID, Depth: 1},
		{AncestorID: child.ID, DescendantID: child.ID, Depth: 0},
	}).Error; err != nil {
		t.Fatal(err)
	}
	version := model.FileVersion{
		WorkspaceID: workspace.ID, NodeID: child.ID, VersionNo: 1, StorageKey: "objects/node-purge/content",
		Size: 10, SHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ScanStatus: "clean", CreatedBy: user.ID,
	}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&child).Update("active_version", version.ID).Error; err != nil {
		t.Fatal(err)
	}
	return nodePurgeFixture{user: user, workspace: workspace, root: root, child: child, version: version}
}

func timePointer(value time.Time) *time.Time { return &value }
