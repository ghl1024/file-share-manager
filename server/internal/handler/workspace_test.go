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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestBindQuota(t *testing.T) {
	gin.SetMode(gin.TestMode)
	quota := int64(1048576)
	tests := []struct {
		name      string
		body      string
		wantOK    bool
		wantQuota *int64
	}{
		{name: "unlimited", body: `{"quota_bytes":null}`, wantOK: true},
		{name: "limited", body: `{"quota_bytes":1048576}`, wantOK: true, wantQuota: &quota},
		{name: "missing", body: `{}`, wantOK: false},
		{name: "negative", body: `{"quota_bytes":-1}`, wantOK: false},
		{name: "fraction", body: `{"quota_bytes":1.5}`, wantOK: false},
		{name: "string", body: `{"quota_bytes":"1024"}`, wantOK: false},
		{name: "malformed", body: `{"quota_bytes":`, wantOK: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			context, _ := gin.CreateTestContext(recorder)
			context.Request = httptest.NewRequest(http.MethodPut, "/quota", strings.NewReader(test.body))
			context.Request.Header.Set("Content-Type", "application/json")

			gotQuota, ok := bindQuota(context)
			if ok != test.wantOK {
				t.Fatalf("bindQuota() ok = %v, want %v", ok, test.wantOK)
			}
			if !test.wantOK {
				if recorder.Code != http.StatusBadRequest {
					t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
				}
				return
			}
			if test.wantQuota == nil {
				if gotQuota != nil {
					t.Fatalf("quota = %d, want nil", *gotQuota)
				}
				return
			}
			if gotQuota == nil || *gotQuota != *test.wantQuota {
				t.Fatalf("quota = %v, want %d", gotQuota, *test.wantQuota)
			}
		})
	}
}
