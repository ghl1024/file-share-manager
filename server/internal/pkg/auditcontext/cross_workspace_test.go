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

func TestCrossWorkspaceAccessRoundTrip(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	source := uint(3)
	RecordCrossWorkspaceAccess(context, CrossWorkspaceAccess{
		SourceWorkspaceID: &source, TargetWorkspaceID: 9, Reason: "incident investigation",
	})

	access, ok := CrossWorkspaceAccessFrom(context)
	if !ok || access.SourceWorkspaceID == nil || *access.SourceWorkspaceID != source ||
		access.TargetWorkspaceID != 9 || access.Reason != "incident investigation" {
		t.Fatalf("CrossWorkspaceAccessFrom() = %#v, %v", access, ok)
	}
}

func TestCrossWorkspaceAccessRejectsIncompleteContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	context, _ := gin.CreateTestContext(httptest.NewRecorder())
	RecordCrossWorkspaceAccess(context, CrossWorkspaceAccess{TargetWorkspaceID: 9})
	if _, ok := CrossWorkspaceAccessFrom(context); ok {
		t.Fatal("incomplete cross-workspace context should not be valid")
	}
}
