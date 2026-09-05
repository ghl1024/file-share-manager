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
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/go-playground/validator/v10"
)

// FieldError is the safe, transport-level representation of one binding
// violation. It intentionally omits the raw decoder error.
type FieldError struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// BindJSON decodes and validates a JSON request body. It writes the standard
// 400 response itself so handlers cannot drift in binding error semantics.
func BindJSON(c *gin.Context, destination any) bool {
	if err := c.ShouldBindJSON(destination); err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			response.PayloadTooLarge(c, "请求体超过系统限制")
		} else if details := bindingDetails(err, destination); len(details) > 0 {
			response.BadRequestWithDetails(c, "请求参数校验失败", details)
		} else if errors.Is(err, io.EOF) {
			response.BadRequest(c, "请求体不能为空")
		} else {
			response.BadRequest(c, "请求体格式错误")
		}
		return false
	}
	return true
}

func bindingDetails(err error, destination any) []FieldError {
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		details := make([]FieldError, 0, len(validationErrors))
		for _, field := range validationErrors {
			details = append(details, FieldError{Field: jsonFieldName(destination, field.Field()), Rule: field.Tag()})
		}
		return details
	}

	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		return []FieldError{{Field: jsonFieldName(destination, typeError.Field), Rule: "type"}}
	}
	return nil
}

func jsonFieldName(destination any, fieldName string) string {
	typ := reflect.TypeOf(destination)
	if typ == nil {
		return fieldName
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	field, ok := typ.FieldByName(fieldName)
	if !ok {
		return fieldName
	}
	name := strings.Split(field.Tag.Get("json"), ",")[0]
	if name == "" || name == "-" {
		return fieldName
	}
	return name
}

// ParseInt64Param parses a positive int64 path parameter. Handlers can map the
// returned error to a consistent 400 response before querying the DAO.
func ParseInt64Param(c *gin.Context, name string) (int64, error) {
	raw := strings.TrimSpace(c.Param(name))
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return value, nil
}

// ParseUintParam parses a positive uint path parameter.
func ParseUintParam(c *gin.Context, name string) (uint, error) {
	raw := strings.TrimSpace(c.Param(name))
	value, err := strconv.ParseUint(raw, 10, 0)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return uint(value), nil
}

// ParseUintParamAllowZero parses a non-negative uint path parameter. Upload
// part indexes are zero-based, so they are valid even though resource IDs are
// required to be positive.
func ParseUintParamAllowZero(c *gin.Context, name string) (uint, error) {
	raw := strings.TrimSpace(c.Param(name))
	value, err := strconv.ParseUint(raw, 10, 0)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return uint(value), nil
}
