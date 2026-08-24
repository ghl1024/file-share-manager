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

func TestParseNodeSearchFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name        string
		query       string
		wantMessage bool
		check       func(t *testing.T, keyword, extension, sort string, minSize, maxSize *int64)
	}{
		{
			name:  "full metadata filter",
			query: "keyword=%E5%9B%9E%E5%BD%92&type=file&extension=PDF&created_by=Admin&updated_from=2026-08-01&updated_to=2026-08-14&min_size=1024&max_size=4096&sort=size_desc",
			check: func(t *testing.T, keyword, extension, sort string, minSize, maxSize *int64) {
				t.Helper()
				if keyword != "回归" || extension != ".pdf" || sort != "size_desc" {
					t.Fatalf("unexpected normalized filter: keyword=%q extension=%q sort=%q", keyword, extension, sort)
				}
				if minSize == nil || *minSize != 1024 || maxSize == nil || *maxSize != 4096 {
					t.Fatalf("unexpected size filter: min=%v max=%v", minSize, maxSize)
				}
			},
		},
		{name: "empty", query: "", wantMessage: true},
		{name: "folder with file metadata", query: "type=folder&extension=zip", wantMessage: true},
		{name: "invalid time range", query: "created_from=2026-08-14&created_to=2026-08-01", wantMessage: true},
		{name: "invalid sort", query: "keyword=test&sort=random", wantMessage: true},
		{name: "invalid extension", query: "extension=pdf%25", wantMessage: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = httptest.NewRequest("GET", "/search?"+test.query, nil)
			filter, message := parseNodeSearchFilter(context)
			if (message != "") != test.wantMessage {
				t.Fatalf("validation message = %q", message)
			}
			if test.check != nil {
				test.check(t, filter.Keyword, filter.Extension, filter.Sort, filter.MinSize, filter.MaxSize)
			}
		})
	}
}

func TestParseSearchDateUsesExclusiveUpperBound(t *testing.T) {
	start, message := parseSearchDate("2026-08-14", false)
	if message != "" {
		t.Fatal(message)
	}
	end, message := parseSearchDate("2026-08-14", true)
	if message != "" {
		t.Fatal(message)
	}
	if end.Sub(*start).Hours() != 24 {
		t.Fatalf("upper bound must include the selected day: start=%v end=%v", start, end)
	}
}
