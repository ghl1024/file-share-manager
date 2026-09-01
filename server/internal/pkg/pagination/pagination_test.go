/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package pagination

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseGinContextWithOptionsNormalizesValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/items?page=0&page_size=999&keyword=files", nil)

	page, pageSize, keyword := ParseGinContextWithOptions(c, Options{
		DefaultPage:     1,
		DefaultPageSize: 20,
		MaxPageSize:     100,
	})
	if page != 1 || pageSize != 20 || keyword != "files" {
		t.Fatalf("unexpected pagination values: page=%d pageSize=%d keyword=%q", page, pageSize, keyword)
	}
}

func TestParseGinContextWithOptionsUsesDefaultsForInvalidInput(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/items?page=bad&page_size=-1", nil)

	page, pageSize, _ := ParseGinContextWithOptions(c, Options{
		DefaultPage:     2,
		DefaultPageSize: 25,
		MaxPageSize:     200,
	})
	if page != 2 || pageSize != 25 {
		t.Fatalf("unexpected defaults: page=%d pageSize=%d", page, pageSize)
	}
}

func TestParseGinContextWithOptionsCapsInvalidDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/items", nil)

	_, pageSize, _ := ParseGinContextWithOptions(c, Options{
		DefaultPageSize: 200,
		MaxPageSize:     100,
	})
	if pageSize != 100 {
		t.Fatalf("expected capped default page size, got %d", pageSize)
	}
}
