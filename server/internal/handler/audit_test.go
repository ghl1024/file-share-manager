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
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAuditFilters(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name       string
		query      string
		wantOK     bool
		wantStatus int
	}{
		{name: "valid filters", query: "?category=security&severity=warning&result=denied&actor_type=external_share&target_type=file&target_id=17&ip=127.0.0.1&request_id=req-1&method=patch&status=403&from=2026-08-12T00:00:00Z&to=2026-08-13T00:00:00Z", wantOK: true},
		{name: "invalid category", query: "?category=unknown", wantStatus: 400},
		{name: "invalid severity", query: "?severity=critical", wantStatus: 400},
		{name: "invalid result", query: "?result=ok", wantStatus: 400},
		{name: "invalid actor type", query: "?actor_type=robot", wantStatus: 400},
		{name: "invalid method", query: "?method=TRACE", wantStatus: 400},
		{name: "invalid status", query: "?status=700", wantStatus: 400},
		{name: "invalid time", query: "?from=not-a-time", wantStatus: 400},
		{name: "reversed range", query: "?from=2026-08-13T00:00:00Z&to=2026-08-12T00:00:00Z", wantStatus: 400},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest("GET", "/audit/events"+test.query, nil)
			_, ok := parseAuditFilters(context)
			if ok != test.wantOK {
				t.Fatalf("ok = %v, want %v", ok, test.wantOK)
			}
			if test.wantStatus != 0 && recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
		})
	}
}
