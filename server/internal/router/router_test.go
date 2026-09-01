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
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"

	"file-share-manager/server/internal/config"

	"github.com/gin-gonic/gin"
)

var ginPathParameterPattern = regexp.MustCompile(`:([A-Za-z0-9_]+)`)

func TestRegisterRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	RegisterRoutesWithConfig(engine, nil)
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

func TestSwaggerDocumentsEveryAPIRoute(t *testing.T) {
	content, err := os.ReadFile("../../docs/swagger.json")
	if err != nil {
		t.Fatalf("read generated Swagger document: %v", err)
	}
	var document struct {
		BasePath string                                `json:"basePath"`
		Paths    map[string]map[string]json.RawMessage `json:"paths"`
	}
	if err := json.Unmarshal(content, &document); err != nil {
		t.Fatalf("parse generated Swagger document: %v", err)
	}

	engine := gin.New()
	RegisterRoutesWithConfig(engine, nil)
	actualOperations := 0
	for _, route := range engine.Routes() {
		if !strings.HasPrefix(route.Path, "/api/fileshare/v1/") {
			continue
		}
		actualOperations++
		path := ginPathParameterPattern.ReplaceAllString(route.Path, `{$1}`)
		documentedPath := strings.TrimPrefix(path, strings.TrimRight(document.BasePath, "/"))
		if documentedPath == "" {
			documentedPath = "/"
		}
		operation, exists := document.Paths[documentedPath][strings.ToLower(route.Method)]
		if !exists {
			t.Errorf("Swagger is missing %s %s (document path %s)", route.Method, path, documentedPath)
			continue
		}
		if requiresBrowserRequestHeader(route.Method, route.Path) && !bytes.Contains(operation, []byte(`"X-Requested-With"`)) {
			t.Errorf("Swagger is missing the X-Requested-With parameter for %s %s", route.Method, path)
		}
	}
	documentedOperations := 0
	for _, operations := range document.Paths {
		documentedOperations += len(operations)
	}
	if documentedOperations != actualOperations {
		t.Fatalf("Swagger operations = %d, registered API routes = %d", documentedOperations, actualOperations)
	}
}

func requiresBrowserRequestHeader(method, path string) bool {
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return false
	}
	return path != "/api/fileshare/v1/auth/login" && !strings.HasPrefix(path, "/api/fileshare/v1/share/")
}

func TestRegisterRoutesWithConfigRestrictsSwagger(t *testing.T) {
	for _, test := range []struct {
		name    string
		mode    string
		enabled bool
		want    bool
	}{
		{name: "debug disabled", mode: "debug"},
		{name: "debug explicitly enabled", mode: "debug", enabled: true, want: true},
		{name: "release remains disabled", mode: "release", enabled: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			RegisterRoutesWithConfig(engine, &config.Config{Server: config.ServerConfig{Mode: test.mode, EnableSwagger: test.enabled}})
			found := false
			for _, route := range engine.Routes() {
				found = found || route.Path == "/swagger/*any"
			}
			if found != test.want {
				t.Fatalf("Swagger route present = %v, want %v", found, test.want)
			}
		})
	}
}

func TestSwaggerUIIsServedWhenExplicitlyEnabledInDebugMode(t *testing.T) {
	engine := gin.New()
	RegisterRoutesWithConfig(engine, &config.Config{Server: config.ServerConfig{Mode: "debug", EnableSwagger: true}})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)
	engine.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /swagger/index.html status = %d, want 200", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("GET /swagger/index.html Content-Type = %q", contentType)
	}
	if !strings.Contains(recorder.Body.String(), `<div id="swagger-ui"></div>`) {
		t.Fatal("GET /swagger/index.html did not return Swagger UI")
	}
}
