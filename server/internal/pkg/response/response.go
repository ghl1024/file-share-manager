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
	"net/http"

	"github.com/gin-gonic/gin"
)

// Response 统一响应结构
type Response struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
	Details interface{} `json:"details,omitempty"`
}

const (
	CodeSuccess            = 0
	CodeInvalidCredentials = 1001
	CodeUserDisabled       = 1002
)

// Success 成功响应
func Success(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: "success",
		Data:    data,
	})
}

// SuccessWithMessage 成功响应带消息
func SuccessWithMessage(c *gin.Context, message string, data interface{}) {
	c.JSON(http.StatusOK, Response{
		Code:    CodeSuccess,
		Message: message,
		Data:    data,
	})
}

// Error is kept for compatibility with older internal callers.
// Deprecated: use semantic helpers such as NotFound, Conflict, or InternalError.
func Error(c *gin.Context, code int, message string) {
	status := code
	if status >= 1000 && status < 2000 {
		status = http.StatusOK
	} else if status < 400 || status > 599 {
		status = http.StatusInternalServerError
	}
	writeError(c, status, code, message, nil)
}

// Unauthorized 未授权响应
func Unauthorized(c *gin.Context, message string) {
	writeError(c, http.StatusUnauthorized, http.StatusUnauthorized, message, nil)
}

// Forbidden 禁止访问响应
func Forbidden(c *gin.Context, message string) {
	writeError(c, http.StatusForbidden, http.StatusForbidden, message, nil)
}

// BadRequest 请求错误响应
func BadRequest(c *gin.Context, message string) {
	writeError(c, http.StatusBadRequest, http.StatusBadRequest, message, nil)
}

// BadRequestWithDetails returns safe structured validation details.
func BadRequestWithDetails(c *gin.Context, message string, details interface{}) {
	writeError(c, http.StatusBadRequest, http.StatusBadRequest, message, details)
}

// NotFound returns a standard resource-not-found response.
func NotFound(c *gin.Context, message string) {
	writeError(c, http.StatusNotFound, http.StatusNotFound, message, nil)
}

// Conflict returns a standard state-conflict response.
func Conflict(c *gin.Context, message string) {
	writeError(c, http.StatusConflict, http.StatusConflict, message, nil)
}

// Gone indicates a resource that existed but is no longer available.
func Gone(c *gin.Context, message string) {
	writeError(c, http.StatusGone, http.StatusGone, message, nil)
}

// PayloadTooLarge indicates that a valid resource exceeds the operation limit.
func PayloadTooLarge(c *gin.Context, message string) {
	writeError(c, http.StatusRequestEntityTooLarge, http.StatusRequestEntityTooLarge, message, nil)
}

// UnsupportedMediaType indicates content that the requested renderer will not process.
func UnsupportedMediaType(c *gin.Context, message string) {
	writeError(c, http.StatusUnsupportedMediaType, http.StatusUnsupportedMediaType, message, nil)
}

// TooManyRequests returns a standard rate-limit response.
func TooManyRequests(c *gin.Context, message string) {
	writeError(c, http.StatusTooManyRequests, http.StatusTooManyRequests, message, nil)
}

// BadGateway returns a standard upstream-dependency response.
func BadGateway(c *gin.Context, message string) {
	writeError(c, http.StatusBadGateway, http.StatusBadGateway, message, nil)
}

// ServiceUnavailable returns a standard unavailable-dependency response.
func ServiceUnavailable(c *gin.Context, message string) {
	writeError(c, http.StatusServiceUnavailable, http.StatusServiceUnavailable, message, nil)
}

// BusinessError keeps an explicitly versioned business code while using HTTP
// 200 for compatibility with existing authentication clients.
func BusinessError(c *gin.Context, code int, message string) {
	writeError(c, http.StatusOK, code, message, nil)
}

// InternalError 内部错误响应
func InternalError(c *gin.Context, message string, causes ...error) {
	for _, cause := range causes {
		if cause != nil {
			_ = c.Error(cause)
		}
	}
	writeError(c, http.StatusInternalServerError, http.StatusInternalServerError, message, nil)
}

// InternalErrorFrom records an internal cause without exposing its contents
// to the client.
func InternalErrorFrom(c *gin.Context, cause error) {
	InternalError(c, "服务器内部错误", cause)
}

func writeError(c *gin.Context, status, code int, message string, details interface{}) {
	c.JSON(status, Response{Code: code, Message: message, Details: details})
}

type pageResult[T any] struct {
	List     []T   `json:"list"`
	Total    int64 `json:"total"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
}

func newPageResult[T any](list []T, total int64, page, pageSize int) pageResult[T] {
	if list == nil {
		list = []T{}
	}
	return pageResult[T]{List: list, Total: total, Page: page, PageSize: pageSize}
}

// SuccessWithPage returns the standard pagination envelope.
func SuccessWithPage[T any](c *gin.Context, list []T, total int64, page, pageSize int) {
	Success(c, newPageResult(list, total, page, pageSize))
}
