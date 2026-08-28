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

func TestCalculateHashChangesWithPreviousHash(t *testing.T) {
	workspaceID := uint(7)
	log := &model.OperationLog{
		UserID: 1, Username: "admin", WorkspaceID: &workspaceID,
		Method: "POST", Path: "/api/fileshare/v1/management/folders", Action: "file:create",
		Status: 200, IP: "127.0.0.1", Latency: 12, RequestID: "req-1",
		CreatedAt: time.Unix(100, 0),
	}
	first := calculateHash(log, nil)
	secondPrevious := first
	second := calculateHash(log, &secondPrevious)
	if first == second {
		t.Fatal("expected previous hash to change the digest")
	}
	if len(first) != 64 || len(second) != 64 {
		t.Fatalf("hash lengths = %d and %d, want 64", len(first), len(second))
	}
}

func TestCalculateHashSurvivesMySQLMillisecondRoundTrip(t *testing.T) {
	workspaceID := uint(7)
	original := &model.OperationLog{
		UserID: 1, Username: "admin", WorkspaceID: &workspaceID,
		Method: "POST", Path: "/api/fileshare/v1/management/folders", Action: "file:create",
		Status: 200, IP: "127.0.0.1", Latency: 12, RequestID: "req-1",
		CreatedAt: time.Unix(100, 987654321),
	}
	persisted := *original
	persisted.CreatedAt = original.CreatedAt.Truncate(time.Millisecond)

	if got, want := calculateHash(original, nil), calculateHash(&persisted, nil); got != want {
		t.Fatalf("hash changed after datetime(3) round trip: got %s want %s", got, want)
	}
}

func TestNormalizeAuditTimestampFillsAndTruncates(t *testing.T) {
	if normalized := normalizeAuditTimestamp(time.Time{}); normalized.IsZero() {
		t.Fatal("zero timestamp was not initialized")
	}
	value := time.Unix(100, 987654321)
	if got, want := normalizeAuditTimestamp(value), time.Unix(100, 987000000); !got.Equal(want) {
		t.Fatalf("normalized timestamp = %s, want %s", got, want)
	}
}

func TestCalculateHashCoversAuditMetadata(t *testing.T) {
	workspaceID := uint(7)
	log := &model.OperationLog{
		UserID: 1, Username: "admin", WorkspaceID: &workspaceID,
		Method: "POST", Path: "/folders", Action: "file:create", Category: "operation",
		Severity: "info", Result: "success", Status: 200, IP: "127.0.0.1",
		Details: "{}", RequestID: "req-1", UserAgent: "test-agent", CreatedAt: time.Unix(100, 0),
	}
	original := calculateHash(log, nil)

	mutations := []struct {
		name   string
		mutate func(*model.OperationLog)
	}{
		{name: "username", mutate: func(value *model.OperationLog) { value.Username = "other" }},
		{name: "result", mutate: func(value *model.OperationLog) { value.Result = "failure" }},
		{name: "details", mutate: func(value *model.OperationLog) { value.Details = `{"changed":true}` }},
		{name: "user agent", mutate: func(value *model.OperationLog) { value.UserAgent = "other-agent" }},
		{name: "error", mutate: func(value *model.OperationLog) { value.ErrorMessage = "changed" }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			changed := *log
			mutation.mutate(&changed)
			if calculateHash(&changed, nil) == original {
				t.Fatal("metadata mutation did not change hash")
			}
		})
	}
}

func TestCalculateHashNormalizesJSONDetails(t *testing.T) {
	workspaceID := uint(7)
	first := &model.OperationLog{
		UserID: 1, Username: "admin", WorkspaceID: &workspaceID,
		Method: "POST", Path: "/shares", Action: "share:create", Category: "operation",
		Severity: "info", Result: "success", Status: 200, Details: `{"share_id":1,"item_id":"item"}`,
		CreatedAt: time.Unix(100, 0),
	}
	second := *first
	second.Details = `{ "item_id": "item", "share_id": 1 }`

	if got, want := calculateHash(first, nil), calculateHash(&second, nil); got != want {
		t.Fatalf("hash changed after JSON storage normalization: got %s want %s", got, want)
	}
}

func TestAuditActorType(t *testing.T) {
	tests := []struct {
		name string
		log  model.OperationLog
		want string
	}{
		{name: "user", log: model.OperationLog{UserID: 1, Username: "admin"}, want: "user"},
		{name: "system", log: model.OperationLog{Username: "scheduler"}, want: "system"},
		{name: "external share", log: model.OperationLog{Username: "external_share"}, want: "external_share"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := auditActorType(&test.log); got != test.want {
				t.Fatalf("auditActorType() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestAuditResultAndReasonCode(t *testing.T) {
	if got := auditResult(403); got != model.AuditResultDenied {
		t.Fatalf("auditResult(403) = %q", got)
	}
	if got := auditReasonCode(403, `{"reason":"unsafe_scan_status"}`); got != model.AuditReasonUnsafeScanStatus {
		t.Fatalf("explicit audit reason = %q", got)
	}
	if got := auditReasonCode(403, `{}`); got != model.AuditReasonPermissionDenied {
		t.Fatalf("fallback audit reason = %q", got)
	}
	if got := auditReasonCode(200, `{}`); got != "" {
		t.Fatalf("successful audit reason = %q", got)
	}
}

func TestAuditCategorySeparatesShareManagementAndAccess(t *testing.T) {
	tests := map[string]string{
		"share:create":          model.AuditCategoryOperation,
		"share:revoke":          model.AuditCategoryOperation,
		"share:access":          model.AuditCategoryAccess,
		"share:download_start":  model.AuditCategoryAccess,
		"share:password_failed": model.AuditCategorySecurity,
		"share:download_denied": model.AuditCategorySecurity,
		"file:preview_complete": model.AuditCategoryAccess,
	}
	for action, want := range tests {
		if got := auditCategory(action); got != want {
			t.Errorf("auditCategory(%q) = %q, want %q", action, got, want)
		}
	}
}

func TestSanitizeAuditEventRedactsStructuredSecrets(t *testing.T) {
	log := &model.OperationLog{
		Path:         "/api/fileshare/v1/share/Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA/download",
		ErrorMessage: "upstream password=plain-secret",
		Details:      `{"password":"plain-secret","permission":"session:use","action":"share:password_failed","nested":{"token_hash":"token-digest","password_configured":true},"items":[{"secret_key":"object-secret"}],"message":"Authorization: Bearer raw-jwt"}`,
	}
	before := `{"credential":"ldap-secret","password_changed":true}`
	log.BeforeJSON = &before

	sanitizeAuditEvent(log)
	combined := log.Path + log.ErrorMessage + log.Details + *log.BeforeJSON
	for _, secret := range []string{"Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA", "plain-secret", "token-digest", "object-secret", "raw-jwt", "ldap-secret"} {
		if strings.Contains(combined, secret) {
			t.Fatalf("audit event retained secret %q: %s", secret, combined)
		}
	}
	for _, expected := range []string{"/share/:token/download", `"permission":"session:use"`, `"action":"share:password_failed"`, `"password_configured":true`, `"password_changed":true`, "[REDACTED]"} {
		if !strings.Contains(combined, expected) {
			t.Fatalf("audit event missing %q after sanitization: %s", expected, combined)
		}
	}
}

func TestSanitizeAuditPayloadProtectsNonJSONText(t *testing.T) {
	got := sanitizeAuditPayload("request failed: token=raw-token")
	if strings.Contains(got, "raw-token") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("sanitizeAuditPayload() = %q", got)
	}
}
