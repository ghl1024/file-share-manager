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
)

// Role assignments are workspace-scoped data. Keep this integration test
// behind the existing temporary-MySQL DSN so it exercises the same transaction
// and unique-index behavior as production without changing the dev database.
func TestAssignUserRolesEnforcesWorkspaceMembershipAndRoleScopeMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(
		&model.User{}, &model.Role{}, &model.UserRole{}, &model.WorkspaceMembership{}, &model.ChangeLog{},
	); err != nil {
		t.Fatal(err)
	}

	user := model.User{Username: "role-boundary", RealName: "Role Boundary", Status: 1, AuthVersion: 1}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	workspaceRole := model.Role{WorkspaceID: 1, Code: "workspace-role", Name: "Workspace role", Status: 1}
	otherWorkspaceRole := model.Role{WorkspaceID: 2, Code: "other-role", Name: "Other role", Status: 1}
	if err := db.Create(&workspaceRole).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&otherWorkspaceRole).Error; err != nil {
		t.Fatal(err)
	}

	roles := &RoleDAO{db: db}
	if err := roles.AssignUserRoles(1, user.ID, []uint{workspaceRole.ID}); !errors.Is(err, ErrUserNotWorkspaceMember) {
		t.Fatalf("non-member assignment error = %v, want %v", err, ErrUserNotWorkspaceMember)
	}

	membership := model.WorkspaceMembership{WorkspaceID: 1, UserID: user.ID, Role: "member"}
	if err := db.Create(&membership).Error; err != nil {
		t.Fatal(err)
	}
	if err := roles.AssignUserRoles(1, user.ID, []uint{otherWorkspaceRole.ID}); err == nil {
		t.Fatal("cross-workspace role assignment unexpectedly succeeded")
	}
	var bindings []model.UserRole
	if err := db.Where("user_id = ?", user.ID).Find(&bindings).Error; err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 0 {
		t.Fatalf("cross-workspace rejection left %d role bindings", len(bindings))
	}

	if err := roles.AssignUserRoles(1, user.ID, []uint{workspaceRole.ID}); err != nil {
		t.Fatalf("valid member assignment failed: %v", err)
	}
	if err := db.Where("workspace_id = ? AND user_id = ? AND role_id = ?", 1, user.ID, workspaceRole.ID).First(&model.UserRole{}).Error; err != nil {
		t.Fatalf("valid role binding not persisted: %v", err)
	}
	var updated model.User
	if err := db.First(&updated, user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.AuthVersion != 2 {
		t.Fatalf("auth_version = %d, want 2", updated.AuthVersion)
	}
}
