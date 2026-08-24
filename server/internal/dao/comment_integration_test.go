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
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
)

func TestCommentTransactionAndActivityMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Node{}, &model.FileVersion{}, &model.Share{},
		&model.NodeComment{}, &model.NodeCommentMention{}, &model.ChangeLog{},
		&model.OperationLog{}, &model.AuditStream{},
	); err != nil {
		t.Fatal(err)
	}
	author := model.User{Username: "comment-author", RealName: "评论作者", Status: 1}
	mentioned := model.User{Username: "comment-mentioned", RealName: "被提及成员", Status: 1}
	if err := db.Create(&[]*model.User{&author, &mentioned}).Error; err != nil {
		t.Fatal(err)
	}
	node := model.Node{WorkspaceID: 7, Name: "协作文档.txt", NormalizedName: "协作文档.txt", Type: "file", Status: "active", CreatedBy: author.ID, UpdatedBy: author.ID}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	version := model.FileVersion{WorkspaceID: 7, NodeID: node.ID, VersionNo: 1, StorageKey: "test/object", Size: 10, SHA256: strings.Repeat("a", 64), CreatedBy: author.ID, CreatedAt: time.Now().Add(-time.Minute)}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}

	comments := &CommentDAO{db: db}
	comment := &model.NodeComment{WorkspaceID: 7, NodeID: node.ID, AuthorID: author.ID, Content: "请 @comment-mentioned 查看", Revision: 1}
	if err := comments.Create(comment, []uint{mentioned.ID}, commentTestAudit(author, "comment:create")); err != nil {
		t.Fatal(err)
	}
	page, err := comments.ListPage(7, node.ID, 1, 20)
	if err != nil || page.Total != 1 || len(page.List) != 1 || page.List[0].AuthorRealName != author.RealName {
		t.Fatalf("ListPage() = %#v, %v", page, err)
	}
	mentions, err := comments.ListMentions([]uint{comment.ID})
	if err != nil || len(mentions[comment.ID]) != 1 || mentions[comment.ID][0].UserID != mentioned.ID {
		t.Fatalf("ListMentions() = %#v, %v", mentions, err)
	}
	if _, err := comments.Update(7, node.ID, comment.ID, author.ID, 9, "冲突", nil, commentTestAudit(author, "comment:update")); !errors.Is(err, ErrCommentConflict) {
		t.Fatalf("revision conflict error = %v", err)
	}
	updated, err := comments.Update(7, node.ID, comment.ID, author.ID, 1, "已更新", nil, commentTestAudit(author, "comment:update"))
	if err != nil || updated == nil || updated.Revision != 2 {
		t.Fatalf("Update() = %#v, %v", updated, err)
	}
	if _, err := comments.Update(7, node.ID, comment.ID, mentioned.ID, 2, "越权", nil, commentTestAudit(mentioned, "comment:update")); !errors.Is(err, ErrCommentForbidden) {
		t.Fatalf("non-author update error = %v", err)
	}
	activity, err := comments.ListActivity(7, node.ID, 1, 20)
	if err != nil || activity.Total != 3 || len(activity.List) != 3 {
		t.Fatalf("ListActivity() = %#v, %v", activity, err)
	}
	deleted, err := comments.Delete(7, node.ID, comment.ID, mentioned.ID, true, commentTestAudit(mentioned, "comment:delete"))
	if err != nil || deleted == nil {
		t.Fatalf("Delete() = %#v, %v", deleted, err)
	}
	assertTableCount(t, db, &model.NodeComment{}, 0)
	assertTableCount(t, db, &model.NodeCommentMention{}, 0)
	assertTableCount(t, db, &model.ChangeLog{}, 3)
	assertTableCount(t, db, &model.OperationLog{}, 3)
	var createAudit model.OperationLog
	if err := db.Where("action = ?", "comment:create").First(&createAudit).Error; err != nil {
		t.Fatal(err)
	}
	if createAudit.AfterJSON == nil || strings.Contains(*createAudit.AfterJSON, comment.Content) {
		t.Fatal("comment audit must contain a safe summary without comment content")
	}
}

func commentTestAudit(user model.User, action string) *model.OperationLog {
	workspaceID := uint(7)
	return &model.OperationLog{
		UserID: user.ID, Username: user.Username, WorkspaceID: &workspaceID,
		Method: "POST", Path: "/nodes/:id/comments", Action: action, Status: 200,
		Details: "{}", CreatedAt: time.Now(),
	}
}
