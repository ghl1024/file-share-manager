/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequestSizeLimitMiddleware limits the size of the request body
func RequestSizeLimitMiddleware(limit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}

// GlobalRequestSizeLimitMiddleware applies the normal limit before any body is
// parsed while allowing the explicitly bounded upload endpoints a larger body.
func GlobalRequestSizeLimitMiddleware(defaultLimit, uploadLimit int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := defaultLimit
		if strings.Contains(c.Request.URL.Path, "/uploads/") {
			limit = uploadLimit
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit)
		c.Next()
	}
}
