/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditcontext

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthorizationChecksPreserveOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordAuthorization(context, AuthorizationCheck{Decision: "allowed", Permission: "file:list", Scope: "permission"})
	RecordAuthorization(context, AuthorizationCheck{Decision: "denied", Permission: "node:read", Scope: "acl", TargetID: "9"})

	checks := AuthorizationChecks(context)
	if len(checks) != 2 || checks[0].Permission != "file:list" || checks[1].Permission != "node:read" {
		t.Fatalf("AuthorizationChecks() = %#v", checks)
	}
	last, ok := LastAuthorizationCheck(context)
	if !ok || last.Decision != "denied" || last.TargetID != "9" {
		t.Fatalf("LastAuthorizationCheck() = %#v, %v", last, ok)
	}
	target, ok := LastAuthorizationTargetCheck(context)
	if !ok || target.TargetID != "9" {
		t.Fatalf("LastAuthorizationTargetCheck() = %#v, %v", target, ok)
	}
}

func TestLastAuthorizationTargetCheckSkipsTargetlessChecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordAuthorization(context, AuthorizationCheck{Decision: "allowed", Permission: "workspace:access", Scope: "workspace", TargetType: "workspace", TargetID: "11"})
	RecordAuthorization(context, AuthorizationCheck{Decision: "allowed", Permission: "file:list", Scope: "permission"})

	target, ok := LastAuthorizationTargetCheck(context)
	if !ok || target.TargetType != "workspace" || target.TargetID != "11" {
		t.Fatalf("LastAuthorizationTargetCheck() = %#v, %v", target, ok)
	}
}
