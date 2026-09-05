/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package request

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseInt64Param(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	id, err := ParseInt64Param(c, "id")
	if err != nil || id != 42 {
		t.Fatalf("expected 42, got %d (%v)", id, err)
	}
}

func TestParseUintParamRejectsInvalidValue(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "id", Value: "0"}}

	if _, err := ParseUintParam(c, "id"); err == nil {
		t.Fatal("expected zero id to be rejected")
	}
}

func TestParseUintParamAllowZero(t *testing.T) {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Params = gin.Params{{Key: "part_no", Value: "0"}}

	partNo, err := ParseUintParamAllowZero(c, "part_no")
	if err != nil || partNo != 0 {
		t.Fatalf("expected zero part index, got %d (%v)", partNo, err)
	}
}

func TestBindJSONWritesStructuredValidationError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/items", nil)

	type payload struct {
		DisplayName string `json:"display_name" binding:"required"`
	}
	if BindJSON(c, &payload{}) {
		t.Fatal("expected empty body to fail")
	}
	var body struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Details []FieldError `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 400 || body.Message != "请求体不能为空" || len(body.Details) != 0 {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestBindJSONReturnsValidationDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/items", strings.NewReader(`{"display_name":""}`))
	c.Request.Header.Set("Content-Type", "application/json")

	type payload struct {
		DisplayName string `json:"display_name" binding:"required"`
	}
	if BindJSON(c, &payload{}) {
		t.Fatal("expected validation to fail")
	}
	var body struct {
		Code    int          `json:"code"`
		Message string       `json:"message"`
		Details []FieldError `json:"details"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 400 || body.Message != "请求参数校验失败" || len(body.Details) != 1 || body.Details[0].Field != "display_name" {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestBindJSONMapsMaxBytesErrorToPayloadTooLarge(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/items", strings.NewReader(`{"name":"large"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Request.Body = http.MaxBytesReader(recorder, c.Request.Body, 4)

	if BindJSON(c, &struct {
		Name string `json:"name"`
	}{}) {
		t.Fatal("expected max bytes body to fail")
	}
	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusRequestEntityTooLarge)
	}
	var body struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != http.StatusRequestEntityTooLarge || body.Message != "请求体超过系统限制" {
		t.Fatalf("unexpected response: %+v", body)
	}
}
