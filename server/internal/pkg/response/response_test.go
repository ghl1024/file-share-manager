/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package response

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestInternalErrorDoesNotExposeCause(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	InternalError(c, "查询失败", errors.New("mysql password=secret"))

	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", recorder.Code)
	}
	if len(c.Errors) != 1 {
		t.Fatalf("expected cause to be recorded, got %d errors", len(c.Errors))
	}
	var body Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != http.StatusInternalServerError || body.Message != "查询失败" {
		t.Fatalf("unexpected response: %+v", body)
	}
	if recorder.Body.String() == "mysql password=secret" {
		t.Fatal("internal cause leaked into response")
	}
}

func TestSuccessWithPageNormalizesNilList(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	SuccessWithPage[string](c, nil, 0, 1, 10)

	var body struct {
		Code int `json:"code"`
		Data struct {
			List []string `json:"list"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != CodeSuccess || body.Data.List == nil {
		t.Fatalf("unexpected response: %+v", body)
	}
}

func TestSemanticErrorsUseHTTPStatusCodes(t *testing.T) {
	tests := []struct {
		name       string
		write      func(*gin.Context)
		statusCode int
		bodyCode   int
	}{
		{name: "not found", write: func(c *gin.Context) { NotFound(c, "missing") }, statusCode: http.StatusNotFound, bodyCode: http.StatusNotFound},
		{name: "conflict", write: func(c *gin.Context) { Conflict(c, "conflict") }, statusCode: http.StatusConflict, bodyCode: http.StatusConflict},
		{name: "business", write: func(c *gin.Context) { BusinessError(c, CodeInvalidCredentials, "invalid") }, statusCode: http.StatusOK, bodyCode: CodeInvalidCredentials},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			test.write(c)

			var body Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if recorder.Code != test.statusCode || body.Code != test.bodyCode {
				t.Fatalf("status=%d code=%d, want status=%d code=%d", recorder.Code, body.Code, test.statusCode, test.bodyCode)
			}
		})
	}
}
