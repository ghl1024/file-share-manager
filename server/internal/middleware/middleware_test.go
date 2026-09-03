/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/auditcontext"

	"github.com/gin-gonic/gin"
)

func TestSessionUserIsCurrent(t *testing.T) {
	tests := []struct {
		name        string
		user        *model.User
		authVersion uint64
		want        bool
	}{
		{name: "current active session", user: &model.User{Status: 1, AuthVersion: 4}, authVersion: 4, want: true},
		{name: "permission change invalidates old session", user: &model.User{Status: 1, AuthVersion: 5}, authVersion: 4},
		{name: "disabled user rejected", user: &model.User{Status: 0, AuthVersion: 4}, authVersion: 4},
		{name: "deleted user rejected"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sessionUserIsCurrent(test.user, test.authVersion); got != test.want {
				t.Fatalf("sessionUserIsCurrent() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestClientIPUsesOnlyConfiguredTrustedProxies(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		trusted    []string
		remoteAddr string
		forwarded  string
		want       string
	}{
		{name: "untrusted proxy header is ignored", trusted: []string{"10.0.0.0/8"}, remoteAddr: "192.0.2.10:1234", forwarded: "203.0.113.7", want: "192.0.2.10"},
		{name: "trusted proxy header is accepted", trusted: []string{"10.0.0.0/8"}, remoteAddr: "10.0.0.8:1234", forwarded: "203.0.113.7", want: "203.0.113.7"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			if err := engine.SetTrustedProxies(test.trusted); err != nil {
				t.Fatal(err)
			}
			engine.GET("/ip", func(c *gin.Context) { c.String(http.StatusOK, c.ClientIP()) })
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/ip", nil)
			request.RemoteAddr = test.remoteAddr
			request.Header.Set("X-Forwarded-For", test.forwarded)
			engine.ServeHTTP(recorder, request)
			if got := recorder.Body.String(); got != test.want {
				t.Fatalf("ClientIP() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSecurityHeadersMiddlewareAllowsSwaggerAssetsOnlyOnSwaggerPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(SecurityHeadersMiddleware())
	engine.GET("/api/example", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	engine.GET("/swagger/index.html", func(c *gin.Context) { c.Status(http.StatusOK) })

	for _, test := range []struct {
		path            string
		wantScriptSelf  bool
		wantDefaultNone bool
	}{
		{path: "/api/example", wantDefaultNone: true},
		{path: "/swagger/index.html", wantScriptSelf: true},
	} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		engine.ServeHTTP(recorder, request)
		policy := recorder.Header().Get("Content-Security-Policy")
		if strings.Contains(policy, "script-src 'self'") != test.wantScriptSelf {
			t.Errorf("%s Content-Security-Policy = %q", test.path, policy)
		}
		if strings.Contains(policy, "default-src 'none'") != test.wantDefaultNone {
			t.Errorf("%s Content-Security-Policy = %q", test.path, policy)
		}
	}
}

func TestBuildRequestAuditEntryKeepsBusinessForbiddenWhenAuthorizationAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/fileshare/v1/management/example", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	setAuthorizationDecision(context, "allowed", "session:use", "authentication")
	setAuthorizationDecision(context, "allowed", "file:upload", "permission")

	entry := buildRequestAuditEntry(context, http.StatusForbidden, 3, "")
	if entry == nil || entry.Action == "permission:denied" || entry.ReasonCode == model.AuditReasonPermissionDenied {
		t.Fatalf("allowed authorization chain was misclassified: %+v", entry)
	}
}

func TestSafeRequestPathRedactsShareToken(t *testing.T) {
	raw := "/api/fileshare/v1/share/Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA/download"
	got := safeRequestPath(raw)
	if got != "/api/fileshare/v1/share/:token/download" {
		t.Fatalf("safeRequestPath() = %q", got)
	}
	if got == raw {
		t.Fatal("safeRequestPath did not redact public share token")
	}
}

func TestHasDedicatedDownloadAudit(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{method: "GET", path: "/api/fileshare/v1/management/files/28/download", want: true},
		{method: "GET", path: "/api/fileshare/v1/management/files/28/download/", want: true},
		{method: "POST", path: "/api/fileshare/v1/management/files/28/download"},
		{method: "GET", path: "/api/fileshare/v1/management/batch-downloads/1/download"},
		{method: "GET", path: "/api/fileshare/v1/management/files/28"},
	}
	for _, test := range tests {
		if got := hasDedicatedDownloadAudit(test.method, test.path); got != test.want {
			t.Errorf("hasDedicatedDownloadAudit(%q, %q) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func TestShouldRecordRequestAudit(t *testing.T) {
	tests := []struct {
		name     string
		method   string
		path     string
		status   int
		decision string
		want     bool
	}{
		{name: "successful read is omitted", method: http.MethodGet, path: "/api/fileshare/v1/management/folders/roots", status: http.StatusOK},
		{name: "allowed read is recorded", method: http.MethodGet, path: "/api/fileshare/v1/management/folders/roots", status: http.StatusOK, decision: "allowed", want: true},
		{name: "denied read is recorded", method: http.MethodGet, path: "/api/fileshare/v1/management/folders/roots", status: http.StatusForbidden, decision: "denied", want: true},
		{name: "concealed denied read is recorded", method: http.MethodGet, path: "/api/fileshare/v1/management/folders/roots", status: http.StatusNotFound, decision: "denied", want: true},
		{name: "business forbidden read without authorization decision is omitted", method: http.MethodGet, path: "/api/fileshare/v1/management/example", status: http.StatusForbidden},
		{name: "write is recorded", method: http.MethodPost, path: "/api/fileshare/v1/management/folders", status: http.StatusOK, want: true},
		{name: "dedicated download remains dedicated on denial", method: http.MethodGet, path: "/api/fileshare/v1/management/files/5/download", status: http.StatusForbidden, decision: "denied"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(test.method, test.path, nil)
			if test.decision != "" {
				setAuthorizationDecision(context, test.decision, "file:list", "permission")
			}
			if got := shouldRecordRequestAudit(context, test.status); got != test.want {
				t.Fatalf("shouldRecordRequestAudit() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestBuildRequestAuditEntryClassifiesPermissionAllowed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/folders/roots", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	context.Set("workspace_id", uint(11))
	setAuthorizationDecision(context, "allowed", "file:list", "permission")

	entry := buildRequestAuditEntry(context, http.StatusOK, 5, "")
	if entry == nil {
		t.Fatal("buildRequestAuditEntry() returned nil")
	}
	if entry.Action != "permission:allowed" || entry.Category != model.AuditCategoryAccess ||
		entry.Result != model.AuditResultSuccess || entry.ReasonCode != "" {
		t.Fatalf("unexpected permission allowed classification: %+v", entry)
	}
}

func TestBuildRequestAuditEntryClassifiesCrossWorkspaceReadAsHighRisk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/nodes/9/detail", nil)
	context.Set("user_id", uint(1))
	context.Set("username", "admin")
	context.Set("workspace_id", uint(22))
	sourceWorkspaceID := uint(11)
	auditcontext.RecordCrossWorkspaceAccess(context, auditcontext.CrossWorkspaceAccess{
		SourceWorkspaceID: &sourceWorkspaceID,
		TargetWorkspaceID: 22,
		Reason:            "security investigation",
	})
	setAuthorizationDecisionForTarget(context, "allowed", "node:read", "acl", "file", "9")

	entry := buildRequestAuditEntry(context, http.StatusOK, 5, "")
	if entry == nil || entry.Action != "super_admin_cross_workspace_read" ||
		entry.Category != model.AuditCategorySecurity || entry.Severity != model.AuditSeverityHigh ||
		entry.Result != model.AuditResultSuccess {
		t.Fatalf("unexpected cross-workspace audit classification: %+v", entry)
	}
	if entry.SourceWorkspaceID == nil || *entry.SourceWorkspaceID != sourceWorkspaceID ||
		entry.TargetWorkspaceID == nil || *entry.TargetWorkspaceID != 22 {
		t.Fatalf("unexpected cross-workspace identity: %+v", entry)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(entry.Details), &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["operation_reason"] != "security investigation" || details["cross_workspace_access"] != true {
		t.Fatalf("unexpected cross-workspace details: %#v", details)
	}
}

func TestBuildRequestAuditEntryClassifiesFailedCrossWorkspaceReadAsHighRisk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/search", nil)
	context.Set("user_id", uint(1))
	context.Set("username", "admin")
	auditcontext.RecordCrossWorkspaceAccess(context, auditcontext.CrossWorkspaceAccess{
		TargetWorkspaceID: 22,
		Reason:            "incident review",
	})
	setAuthorizationDecision(context, "allowed", "file:list", "permission")

	entry := buildRequestAuditEntry(context, http.StatusInternalServerError, 8, "database unavailable")
	if entry == nil || entry.Action != "super_admin_cross_workspace_read" ||
		entry.Severity != model.AuditSeverityHigh || entry.Result != model.AuditResultFailure {
		t.Fatalf("unexpected failed cross-workspace audit classification: %+v", entry)
	}
}

func TestBuildRequestAuditEntryKeepsDeniedCrossWorkspaceReadHighRisk(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/folders/9/children", nil)
	context.Set("user_id", uint(1))
	context.Set("username", "admin")
	auditcontext.RecordCrossWorkspaceAccess(context, auditcontext.CrossWorkspaceAccess{
		TargetWorkspaceID: 22,
		Reason:            "incident review",
	})
	setAuthorizationDecisionForTarget(context, "denied", "node:read", "acl", "folder", "9")

	entry := buildRequestAuditEntry(context, http.StatusForbidden, 5, "")
	if entry == nil || entry.Action != "super_admin_cross_workspace_read" ||
		entry.Severity != model.AuditSeverityHigh || entry.Result != model.AuditResultDenied ||
		entry.ReasonCode != model.AuditReasonPermissionDenied {
		t.Fatalf("unexpected denied cross-workspace audit classification: %+v", entry)
	}
}

func TestBuildRequestAuditEntryDoesNotPromoteCrossWorkspaceManagementRead(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/audit/logs", nil)
	context.Set("user_id", uint(1))
	context.Set("username", "admin")
	auditcontext.RecordCrossWorkspaceAccess(context, auditcontext.CrossWorkspaceAccess{
		TargetWorkspaceID: 22,
		Reason:            "incident review",
	})
	setAuthorizationDecision(context, "allowed", "audit:list", "permission")

	entry := buildRequestAuditEntry(context, http.StatusOK, 4, "")
	if entry == nil || entry.Action != "permission:allowed" || entry.Severity == model.AuditSeverityHigh {
		t.Fatalf("management read was incorrectly promoted: %+v", entry)
	}
}

func TestBuildRequestAuditEntryKeepsLastConcreteTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/folders/roots", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	setAuthorizationDecisionForTarget(context, "allowed", "workspace:access", "workspace", "workspace", "11")
	setAuthorizationDecision(context, "allowed", "file:list", "permission")

	entry := buildRequestAuditEntry(context, http.StatusOK, 5, "")
	if entry == nil || entry.TargetType != "workspace" || entry.TargetID != "11" {
		t.Fatalf("concrete authorization target was lost: %+v", entry)
	}
}

func TestBuildRequestAuditEntryClassifiesPermissionDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/folders/roots", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	context.Set("workspace_id", uint(11))
	context.Set("request_id", "request-1")
	setAuthorizationDecision(context, "denied", "file:list", "permission")

	entry := buildRequestAuditEntry(context, http.StatusForbidden, 9, "")
	if entry == nil {
		t.Fatal("buildRequestAuditEntry() returned nil")
	}
	if entry.Action != "permission:denied" || entry.Category != model.AuditCategorySecurity ||
		entry.Result != model.AuditResultDenied || entry.ReasonCode != model.AuditReasonPermissionDenied {
		t.Fatalf("unexpected permission denial classification: %+v", entry)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(entry.Details), &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["permission"] != "file:list" || details["decision"] != "denied" || details["scope"] != "permission" {
		t.Fatalf("unexpected permission denial details: %#v", details)
	}
}

func TestBuildRequestAuditEntryDoesNotMisclassifyBusinessForbidden(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/api/fileshare/v1/management/example", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	setAuthorizationDecision(context, "allowed", "file:upload", "permission")

	entry := buildRequestAuditEntry(context, http.StatusForbidden, 3, "")
	if entry == nil {
		t.Fatal("buildRequestAuditEntry() returned nil")
	}
	if entry.Action == "permission:denied" || entry.ReasonCode == model.AuditReasonPermissionDenied {
		t.Fatalf("business 403 was misclassified as a permission denial: %+v", entry)
	}
}

func TestBuildRequestAuditEntryClassifiesConcealedPermissionDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/nodes/9/detail", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	auditcontext.RecordAuthorization(context, auditcontext.AuthorizationCheck{
		Decision: "denied", Permission: "node:read", Scope: "acl", TargetType: "file", TargetID: "9",
	})

	entry := buildRequestAuditEntry(context, http.StatusNotFound, 3, "")
	if entry == nil || entry.Action != "permission:denied" || entry.Result != model.AuditResultDenied {
		t.Fatalf("concealed permission denial was not classified: %+v", entry)
	}
}

func TestBuildRequestAuditEntryTreatsFinalForbiddenAsDataPermissionDenial(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/fileshare/v1/management/folders/9/children", nil)
	context.Set("user_id", uint(7))
	context.Set("username", "member")
	setAuthorizationDecision(context, "allowed", "file:list", "permission")
	auditcontext.RecordAuthorization(context, auditcontext.AuthorizationCheck{
		Decision: "denied", Permission: "node:read", Scope: "acl", TargetType: "folder", TargetID: "9",
	})

	entry := buildRequestAuditEntry(context, http.StatusForbidden, 4, "")
	if entry == nil || entry.Action != "permission:denied" || entry.ReasonCode != model.AuditReasonPermissionDenied {
		t.Fatalf("final 403 was not classified as a data permission denial: %+v", entry)
	}
	if entry.TargetType != "folder" || entry.TargetID != "9" {
		t.Fatalf("permission target was not promoted to searchable fields: %+v", entry)
	}
	var details map[string]any
	if err := json.Unmarshal([]byte(entry.Details), &details); err != nil {
		t.Fatalf("unmarshal details: %v", err)
	}
	if details["decision"] != "denied" || details["permission"] != "node:read" {
		t.Fatalf("unexpected layered authorization details: %#v", details)
	}
	checks, ok := details["checks"].([]any)
	if !ok || len(checks) < 2 {
		t.Fatalf("authorization check chain is incomplete: %#v", details["checks"])
	}
}
