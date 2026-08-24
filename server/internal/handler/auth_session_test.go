/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package handler

import (
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBuildSessionMenus(t *testing.T) {
	tests := []struct {
		name          string
		isSuperAdmin  bool
		hasWorkspace  bool
		permissions   map[string]bool
		expectedPaths []string
	}{
		{
			name:          "member without selected workspace",
			expectedPaths: []string{"/dashboard", "/workspaces"},
		},
		{
			name:         "member sees only authorized workspace modules",
			hasWorkspace: true,
			permissions: map[string]bool{
				"file:list":  true,
				"audit:list": true,
			},
			expectedPaths: []string{"/dashboard", "/workspaces", "/files", "/audit/history"},
		},
		{
			name:         "member sees authorized system children under system parent",
			hasWorkspace: true,
			permissions: map[string]bool{
				"workspace:role:manage": true,
				"backup:manage":         true,
			},
			expectedPaths: []string{"/dashboard", "/workspaces", "/system", "/system/role", "/system/backups"},
		},
		{
			name:          "super admin sees platform and workspace modules",
			isSuperAdmin:  true,
			hasWorkspace:  true,
			expectedPaths: []string{"/dashboard", "/workspaces", "/files", "/shares", "/audit/history", "/system", "/system/user", "/system/menu", "/system/ldap", "/system/clamav", "/system/backup-storage", "/system/role", "/system/backups"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			menus := buildSessionMenus(test.isSuperAdmin, test.hasWorkspace, test.permissions)
			if paths := menuPaths(menus); !reflect.DeepEqual(paths, test.expectedPaths) {
				t.Fatalf("paths = %#v, want %#v", paths, test.expectedPaths)
			}
		})
	}
}

func TestSessionWorkspaceAccess(t *testing.T) {
	context, _ := gin.CreateTestContext(nil)
	context.Set("workspace_id", uint(12))
	context.Set("source_workspace_id", uint(3))
	context.Set("cross_workspace_access", true)
	context.Set("cross_workspace_reason", "  incident review  ")

	access := sessionWorkspaceAccess(context)
	if access.WorkspaceID == nil || *access.WorkspaceID != 12 {
		t.Fatalf("workspace id = %#v", access.WorkspaceID)
	}
	if access.SourceWorkspaceID == nil || *access.SourceWorkspaceID != 3 {
		t.Fatalf("source workspace id = %#v", access.SourceWorkspaceID)
	}
	if !access.CrossWorkspaceAccess || access.CrossWorkspaceReason != "incident review" {
		t.Fatalf("cross-workspace access = %#v", access)
	}
}

func menuPaths(menus []gin.H) []string {
	paths := make([]string, 0, len(menus))
	for _, menu := range menus {
		paths = append(paths, menu["path"].(string))
		children, _ := menu["children"].([]gin.H)
		if len(children) > 0 {
			paths = append(paths, menuPaths(children)...)
		}
	}
	return paths
}
