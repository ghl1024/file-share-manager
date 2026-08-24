/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package router

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutes(engine)
	if len(engine.Routes()) == 0 {
		t.Fatal("expected API routes to be registered")
	}

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, expected := range []string{
		"GET /api/fileshare/v1/auth/profile",
		"PUT /api/fileshare/v1/auth/profile",
		"PUT /api/fileshare/v1/auth/password",
		"GET /api/fileshare/v1/management/dashboard/stats",
		"GET /api/fileshare/v1/management/workspaces/:wid/available-users",
		"GET /api/fileshare/v1/management/workspaces/:wid/groups/directory",
		"PUT /api/fileshare/v1/management/workspaces/:wid",
		"PUT /api/fileshare/v1/management/workspaces/:wid/members/:uid/quota",
		"PUT /api/fileshare/v1/management/users/batch/roles",
		"GET /api/fileshare/v1/management/favorites",
		"GET /api/fileshare/v1/management/collaboration/shared-with-me",
		"GET /api/fileshare/v1/management/collaboration/recent",
		"GET /api/fileshare/v1/management/notifications",
		"GET /api/fileshare/v1/management/notifications/unread-count",
		"GET /api/fileshare/v1/management/notifications/preferences",
		"PUT /api/fileshare/v1/management/notifications/preferences",
		"PUT /api/fileshare/v1/management/notifications/read-all",
		"PUT /api/fileshare/v1/management/notifications/:id/read",
		"POST /api/fileshare/v1/management/notifications/:id/open",
		"GET /api/fileshare/v1/management/shares/:id",
		"PUT /api/fileshare/v1/management/nodes/:id/favorite",
		"PUT /api/fileshare/v1/management/nodes/batch/favorite",
		"POST /api/fileshare/v1/management/nodes/batch/move",
		"POST /api/fileshare/v1/management/nodes/batch/trash",
		"POST /api/fileshare/v1/management/nodes/batch/restore",
		"GET /api/fileshare/v1/management/nodes/:id/activity",
		"GET /api/fileshare/v1/management/nodes/:id/comments",
		"GET /api/fileshare/v1/management/nodes/:id/mention-candidates",
		"POST /api/fileshare/v1/management/nodes/:id/comments",
		"PUT /api/fileshare/v1/management/nodes/:id/comments/:comment_id",
		"DELETE /api/fileshare/v1/management/nodes/:id/comments/:comment_id",
		"GET /api/fileshare/v1/management/system/configs",
		"GET /api/fileshare/v1/management/system/permissions",
		"GET /api/fileshare/v1/management/system/menus",
		"POST /api/fileshare/v1/management/system/menus",
		"PUT /api/fileshare/v1/management/system/menus/:id",
		"DELETE /api/fileshare/v1/management/system/menus/:id",
		"GET /api/fileshare/v1/management/system/ldap",
		"POST /api/fileshare/v1/management/system/ldap",
		"POST /api/fileshare/v1/management/system/ldap/test",
		"POST /api/fileshare/v1/management/system/ldap/sync",
		"GET /api/fileshare/v1/management/system/ldap/history",
		"GET /api/fileshare/v1/management/system/clamav/health",
		"GET /api/fileshare/v1/management/system/storage/health",
		"GET /api/fileshare/v1/management/backups",
		"GET /api/fileshare/v1/management/backups/health",
		"GET /api/fileshare/v1/management/backup-restore-drills",
		"POST /api/fileshare/v1/management/backups/baseline",
		"POST /api/fileshare/v1/management/backups/incremental",
		"POST /api/fileshare/v1/management/backups/compact",
		"POST /api/fileshare/v1/management/backups/:id/retry",
		"POST /api/fileshare/v1/management/backups/:id/restore-drill",
		"POST /api/fileshare/v1/management/backups/:id/verify",
		"POST /api/fileshare/v1/management/backups/:id/restore",
		"POST /api/fileshare/v1/management/backups/:id/restore-workspace",
		"GET /api/fileshare/v1/management/storage/reconcile",
		"POST /api/fileshare/v1/management/storage/reconcile/quarantine",
		"POST /api/fileshare/v1/management/files/:id/versions/:version/rescan",
		"GET /api/fileshare/v1/management/files/:id/preview",
		"GET /api/fileshare/v1/management/files/:id/preview/content",
		"GET /api/fileshare/v1/management/audit/events",
		"GET /api/fileshare/v1/management/audit/security-events",
		"GET /api/fileshare/v1/management/audit/policy",
		"GET /api/fileshare/v1/management/audit/exports",
		"POST /api/fileshare/v1/management/audit/exports",
		"GET /api/fileshare/v1/management/audit/exports/:id/download",
		"GET /api/fileshare/v1/management/audit/archives",
		"POST /api/fileshare/v1/management/audit/archives/run",
	} {
		if !routes[expected] {
			t.Errorf("missing route %s", expected)
		}
	}
}
