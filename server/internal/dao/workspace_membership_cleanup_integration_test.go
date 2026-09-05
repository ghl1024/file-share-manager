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

	"file-share-manager/server/internal/model"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestRemoveMemberRevokesWorkspaceAuthorizationsMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Workspace{}, &model.WorkspaceMembership{},
		&model.Role{}, &model.UserRole{}, &model.UserGroup{}, &model.UserGroupMember{},
		&model.Node{}, &model.NodeACL{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}

	user := model.User{Username: "remove-member", PasswordHash: "unused", RealName: "移除成员", Status: 1, AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	workspaceOne := model.Workspace{UUID: uuid.NewString(), Name: "空间一", Code: "workspace-one", Status: 1, CreatedBy: user.ID}
	workspaceTwo := model.Workspace{UUID: uuid.NewString(), Name: "空间二", Code: "workspace-two", Status: 1, CreatedBy: user.ID}
	if err := db.Create(&workspaceOne).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&workspaceTwo).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.WorkspaceMembership{
		{WorkspaceID: workspaceOne.ID, UserID: user.ID, Role: "member"},
		{WorkspaceID: workspaceTwo.ID, UserID: user.ID, Role: "member"},
	}).Error; err != nil {
		t.Fatal(err)
	}
	roles := []model.Role{
		{WorkspaceID: workspaceOne.ID, Code: "role-one", Name: "空间一角色", Status: 1},
		{WorkspaceID: workspaceTwo.ID, Code: "role-two", Name: "空间二角色", Status: 1},
	}
	if err := db.Create(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{
		{WorkspaceID: workspaceOne.ID, UserID: user.ID, RoleID: roles[0].ID},
		{WorkspaceID: workspaceTwo.ID, UserID: user.ID, RoleID: roles[1].ID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	groups := []model.UserGroup{
		{WorkspaceID: workspaceOne.ID, Name: "空间一组", Source: "local"},
		{WorkspaceID: workspaceTwo.ID, Name: "空间二组", Source: "local"},
	}
	if err := db.Create(&groups).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserGroupMember{
		{GroupID: groups[0].ID, UserID: user.ID},
		{GroupID: groups[1].ID, UserID: user.ID},
	}).Error; err != nil {
		t.Fatal(err)
	}
	nodes := []model.Node{
		{WorkspaceID: workspaceOne.ID, Name: "空间一目录", NormalizedName: "空间一目录", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: user.ID, UpdatedBy: user.ID},
		{WorkspaceID: workspaceTwo.ID, Name: "空间二目录", NormalizedName: "空间二目录", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: user.ID, UpdatedBy: user.ID},
	}
	if err := db.Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.NodeACL{
		{WorkspaceID: workspaceOne.ID, NodeID: nodes[0].ID, SubjectType: "user", SubjectID: user.ID, Effect: "allow", AccessLevel: "read"},
		{WorkspaceID: workspaceTwo.ID, NodeID: nodes[1].ID, SubjectType: "user", SubjectID: user.ID, Effect: "allow", AccessLevel: "read"},
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := (&WorkspaceDAO{db: db}).RemoveMember(workspaceOne.ID, user.ID); err != nil {
		t.Fatalf("RemoveMember() error = %v", err)
	}
	assertWorkspaceMemberRows(t, db, workspaceOne.ID, user.ID, 0)
	assertWorkspaceMemberRows(t, db, workspaceTwo.ID, user.ID, 1)
	assertScopedCount(t, db, &model.UserRole{}, "workspace_id = ? AND user_id = ?", workspaceOne.ID, user.ID, 0)
	assertScopedCount(t, db, &model.UserRole{}, "workspace_id = ? AND user_id = ?", workspaceTwo.ID, user.ID, 1)
	assertScopedCount(t, db, &model.UserGroupMember{}, "group_id = ? AND user_id = ?", groups[0].ID, user.ID, 0)
	assertScopedCount(t, db, &model.UserGroupMember{}, "group_id = ? AND user_id = ?", groups[1].ID, user.ID, 1)
	assertScopedCount(t, db, &model.NodeACL{}, "workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceOne.ID, "user", user.ID, 0)
	assertScopedCount(t, db, &model.NodeACL{}, "workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceTwo.ID, "user", user.ID, 1)

	var updated model.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.AuthVersion != 2 {
		t.Fatalf("auth_version = %d, want 2", updated.AuthVersion)
	}

	// Simulate a legacy database where membership was removed before the
	// authorization cleanup was introduced. Re-joining must discard that stale
	// state instead of restoring permissions.
	if err := db.Create(&model.UserRole{WorkspaceID: workspaceOne.ID, UserID: user.ID, RoleID: roles[0].ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.UserGroupMember{GroupID: groups[0].ID, UserID: user.ID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.NodeACL{WorkspaceID: workspaceOne.ID, NodeID: nodes[0].ID, SubjectType: "user", SubjectID: user.ID, Effect: "allow", AccessLevel: "read"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&WorkspaceDAO{db: db}).EnsureMember(workspaceOne.ID, user.ID, user.ID); err != nil {
		t.Fatal(err)
	}
	assertWorkspaceMemberRows(t, db, workspaceOne.ID, user.ID, 1)
	assertScopedCount(t, db, &model.UserRole{}, "workspace_id = ? AND user_id = ?", workspaceOne.ID, user.ID, 0)
	assertScopedCount(t, db, &model.UserGroupMember{}, "group_id = ? AND user_id = ?", groups[0].ID, user.ID, 0)
	assertScopedCount(t, db, &model.NodeACL{}, "workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceOne.ID, "user", user.ID, 0)
}

func TestRemoveMemberAuthorizationCleanupRollsBackWithChangeLogFailureMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Workspace{}, &model.WorkspaceMembership{},
		&model.Role{}, &model.UserRole{}, &model.UserGroup{}, &model.UserGroupMember{},
		&model.Node{}, &model.NodeACL{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}
	user := model.User{Username: "remove-member-rollback", PasswordHash: "unused", RealName: "回滚成员", Status: 1, AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	workspace := model.Workspace{UUID: uuid.NewString(), Name: "回滚空间", Code: "rollback-workspace", Status: 1, CreatedBy: user.ID}
	if err := db.Create(&workspace).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&model.WorkspaceMembership{WorkspaceID: workspace.ID, UserID: user.ID, Role: "member"}).Error; err != nil {
		t.Fatal(err)
	}
	role := model.Role{WorkspaceID: workspace.ID, Code: "rollback-role", Name: "回滚角色", Status: 1}
	group := model.UserGroup{WorkspaceID: workspace.ID, Name: "回滚组", Source: "local"}
	node := model.Node{WorkspaceID: workspace.ID, Name: "回滚目录", NormalizedName: "回滚目录", Type: "folder", InheritMode: "inherit", Status: "active", CreatedBy: user.ID, UpdatedBy: user.ID}
	if err := db.Create(&role).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserRole{{WorkspaceID: workspace.ID, UserID: user.ID, RoleID: role.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.UserGroupMember{{GroupID: group.ID, UserID: user.ID}}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&[]model.NodeACL{{WorkspaceID: workspace.ID, NodeID: node.ID, SubjectType: "user", SubjectID: user.ID, Effect: "allow", AccessLevel: "read"}}).Error; err != nil {
		t.Fatal(err)
	}

	failure := errors.New("injected member cleanup change log failure")
	if err := db.Callback().Create().Before("gorm:create").Register("test:fail_member_cleanup_change_log", func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == (model.ChangeLog{}).TableName() {
			tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err := (&WorkspaceDAO{db: db}).RemoveMember(workspace.ID, user.ID)
	if !errors.Is(err, failure) {
		t.Fatalf("RemoveMember() error = %v, want %v", err, failure)
	}
	assertWorkspaceMemberRows(t, db, workspace.ID, user.ID, 1)
	assertScopedCount(t, db, &model.UserRole{}, "workspace_id = ? AND user_id = ?", workspace.ID, user.ID, 1)
	assertScopedCount(t, db, &model.UserGroupMember{}, "group_id = ? AND user_id = ?", group.ID, user.ID, 1)
	assertScopedCount(t, db, &model.NodeACL{}, "workspace_id = ? AND subject_type = ? AND subject_id = ?", workspace.ID, "user", user.ID, 1)
	var unchanged model.User
	if err := db.First(&unchanged, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.AuthVersion != 1 {
		t.Fatalf("auth_version after rollback = %d, want 1", unchanged.AuthVersion)
	}
}

func assertWorkspaceMemberRows(t *testing.T, db *gorm.DB, workspaceID, userID uint, expected int64) {
	t.Helper()
	assertScopedCount(t, db, &model.WorkspaceMembership{}, "workspace_id = ? AND user_id = ?", workspaceID, userID, expected)
}

func assertScopedCount(t *testing.T, db *gorm.DB, value any, where string, args ...any) {
	t.Helper()
	var expected int64
	switch value := args[len(args)-1].(type) {
	case int:
		expected = int64(value)
	case int64:
		expected = value
	default:
		t.Fatalf("expected count must be an integer, got %T", value)
	}
	args = args[:len(args)-1]
	var count int64
	if err := db.Model(value).Where(where, args...).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != expected {
		t.Fatalf("%T count = %d, want %d", value, count, expected)
	}
}
