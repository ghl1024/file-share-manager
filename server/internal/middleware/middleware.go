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
	"encoding/json"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/auditcontext"
	"file-share-manager/server/internal/pkg/jwt"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// AuthMiddleware JWT认证中间件
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			cookieToken, cookieErr := c.Cookie(jwt.SessionCookieName)
			if cookieErr != nil || cookieToken == "" {
				response.Unauthorized(c, "未登录，请先登录")
				c.Abort()
				return
			}
			if c.Request.Method != "GET" && c.Request.Method != "HEAD" && c.Request.Method != "OPTIONS" &&
				c.GetHeader("X-Requested-With") != "XMLHttpRequest" {
				response.Forbidden(c, "缺少浏览器请求校验头")
				c.Abort()
				return
			}
			authHeader = "Bearer " + cookieToken
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		if tokenString == authHeader {
			response.Unauthorized(c, "Token格式错误")
			c.Abort()
			return
		}

		claims, err := jwt.ParseToken(tokenString)
		if err != nil {
			response.Unauthorized(c, "Token无效或已过期")
			c.Abort()
			return
		}
		user, err := dao.NewUserDAO().GetByID(claims.UserID)
		if err != nil || !sessionUserIsCurrent(user, claims.AuthVersion) {
			response.Unauthorized(c, "登录状态已失效，请重新登录")
			c.Abort()
			return
		}

		c.Set("user_id", user.ID)
		c.Set("username", user.Username)
		c.Set("is_super_admin", user.IsSuperAdmin)
		if claims.ExpiresAt != nil {
			c.Set("session_expires_at", claims.ExpiresAt.Time)
		}
		if claims.WorkspaceID != nil {
			c.Set("workspace_id", *claims.WorkspaceID)
		}
		if claims.SourceWorkspaceID != nil {
			c.Set("source_workspace_id", *claims.SourceWorkspaceID)
		}
		c.Set("cross_workspace_access", claims.CrossWorkspaceAccess)
		c.Set("cross_workspace_reason", strings.TrimSpace(claims.CrossWorkspaceReason))
		setAuthorizationDecision(c, "allowed", "session:use", "authentication")
		c.Next()
	}
}

func sessionUserIsCurrent(user *model.User, authVersion uint64) bool {
	return user != nil && user.Status == 1 && user.AuthVersion == authVersion
}

// WorkspaceContextMiddleware accepts only a workspace selected through the
// signed session and revalidates membership on every request.
func WorkspaceContextMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		value, exists := c.Get("workspace_id")
		workspaceID, valid := value.(uint)
		if !exists || !valid || workspaceID == 0 {
			response.BadRequest(c, "请先选择工作空间")
			c.Abort()
			return
		}
		workspace, err := dao.NewWorkspaceDAO().GetByID(workspaceID)
		if err != nil {
			response.InternalError(c, "工作空间校验失败", err)
			c.Abort()
			return
		}
		if workspace == nil || workspace.Status != 1 {
			setAuthorizationDecisionForTarget(c, "denied", "workspace:access", "workspace", "workspace", fmt.Sprint(workspaceID))
			response.Forbidden(c, "工作空间不存在或已停用")
			c.Abort()
			return
		}

		isSuperAdmin, _ := c.Get("is_super_admin")
		if allowed, ok := isSuperAdmin.(bool); ok && allowed {
			membership, err := dao.NewWorkspaceDAO().GetMembership(workspaceID, userIDFromContext(c))
			if err != nil {
				response.InternalError(c, "工作空间成员校验失败", err)
				c.Abort()
				return
			}
			if membership == nil {
				crossAccessValue, _ := c.Get("cross_workspace_access")
				crossAccess, _ := crossAccessValue.(bool)
				reasonValue, _ := c.Get("cross_workspace_reason")
				reason, _ := reasonValue.(string)
				reason = strings.TrimSpace(reason)
				if !crossAccess || len([]rune(reason)) < 5 {
					setAuthorizationDecisionForTarget(c, "denied", "workspace:cross_access", "workspace", "workspace", fmt.Sprint(workspaceID))
					response.Forbidden(c, "请重新选择工作空间并填写跨空间访问原因")
					c.Abort()
					return
				}
				var sourceWorkspaceID *uint
				if value, exists := c.Get("source_workspace_id"); exists {
					if source, valid := value.(uint); valid && source > 0 {
						sourceWorkspaceID = &source
					}
				}
				auditcontext.RecordCrossWorkspaceAccess(c, auditcontext.CrossWorkspaceAccess{
					SourceWorkspaceID: sourceWorkspaceID, TargetWorkspaceID: workspaceID, Reason: reason,
				})
			}
			c.Set("workspace_role", "super_admin")
			setAuthorizationDecisionForTarget(c, "allowed", "workspace:access", "workspace", "workspace", fmt.Sprint(workspaceID))
			c.Next()
			return
		}
		userValue, exists := c.Get("user_id")
		userID, valid := userValue.(uint)
		if !exists || !valid {
			response.Unauthorized(c, "未登录")
			c.Abort()
			return
		}
		membership, err := dao.NewWorkspaceDAO().GetMembership(workspaceID, userID)
		if err != nil {
			response.InternalError(c, "工作空间成员校验失败", err)
			c.Abort()
			return
		}
		if membership == nil {
			setAuthorizationDecisionForTarget(c, "denied", "workspace:access", "workspace", "workspace", fmt.Sprint(workspaceID))
			response.Forbidden(c, "已失去该工作空间的访问权限")
			c.Abort()
			return
		}
		c.Set("workspace_role", membership.Role)
		c.Set("workspace_membership_id", membership.ID)
		setAuthorizationDecisionForTarget(c, "allowed", "workspace:access", "workspace", "workspace", fmt.Sprint(workspaceID))
		c.Next()
	}
}

// PermissionMiddleware 权限校验中间件
func PermissionMiddleware(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		_, exists := c.Get("user_id")
		if !exists {
			response.Unauthorized(c, "未登录")
			c.Abort()
			return
		}

		isSuperAdmin, _ := c.Get("is_super_admin")
		if b, ok := isSuperAdmin.(bool); ok && b {
			setAuthorizationDecision(c, "allowed", permission, "permission")
			c.Next()
			return
		}
		workspaceRole, _ := c.Get("workspace_role")
		if role, ok := workspaceRole.(string); ok && role == "workspace_admin" {
			setAuthorizationDecision(c, "allowed", permission, "permission")
			c.Next()
			return
		}

		workspaceValue, workspaceExists := c.Get("workspace_id")
		workspaceID, workspaceValid := workspaceValue.(uint)
		userValue, userExists := c.Get("user_id")
		userID, userValid := userValue.(uint)
		if !workspaceExists || !workspaceValid || !userExists || !userValid {
			setAuthorizationDecision(c, "denied", permission, "permission")
			response.Forbidden(c, "缺少工作空间权限上下文")
			c.Abort()
			return
		}
		allowed, err := dao.NewPermissionDAO().UserHasPermission(workspaceID, userID, permission)
		if err != nil {
			response.InternalError(c, "权限校验失败", err)
			c.Abort()
			return
		}
		if !allowed {
			setAuthorizationDecision(c, "denied", permission, "permission")
			response.Forbidden(c, "缺少权限: "+permission)
			c.Abort()
			return
		}
		setAuthorizationDecision(c, "allowed", permission, "permission")
		c.Next()
	}
}

func SuperAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		isSuperAdmin, _ := c.Get("is_super_admin")
		if allowed, ok := isSuperAdmin.(bool); !ok || !allowed {
			setAuthorizationDecision(c, "denied", "super_admin", "system")
			response.Forbidden(c, "仅超级管理员可执行此操作")
			c.Abort()
			return
		}
		setAuthorizationDecision(c, "allowed", "super_admin", "system")
		c.Next()
	}
}

// CORSMiddleware 跨域中间件
func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		cfg := config.GetConfig()

		// 确定允许的 Origin
		allowedOrigin := ""
		if cfg != nil && cfg.Server.WebURL != "" {
			if origin == cfg.Server.WebURL {
				allowedOrigin = origin
			}
		}

		if allowedOrigin != "" {
			c.Header("Access-Control-Allow-Origin", allowedOrigin)
		}
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Requested-With")
		c.Header("Access-Control-Expose-Headers", "Content-Length")
		if origin != "" && allowedOrigin == origin {
			c.Header("Access-Control-Allow-Credentials", "true")
		}

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func SecurityHeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		contentSecurityPolicy := "default-src 'none'; frame-ancestors 'none'; base-uri 'none'"
		if strings.HasPrefix(c.Request.URL.Path, "/swagger/") {
			contentSecurityPolicy = "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'"
		}
		c.Header("Content-Security-Policy", contentSecurityPolicy)
		if c.Request.TLS != nil || strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https") {
			c.Header("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		c.Next()
	}
}

// LoggerMiddleware 自定义日志中间件
func LoggerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := strings.TrimSpace(c.GetHeader("X-Request-ID"))
		if requestID == "" || len(requestID) > 128 {
			requestID = uuid.NewString()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-ID", requestID)

		c.Next()
		status := c.Writer.Status()
		route := c.FullPath()
		if route == "" {
			route = safeRequestPath(c.Request.URL.Path)
		}
		safePath := safeRequestPath(c.Request.URL.Path)
		fields := []zap.Field{
			zap.String("req_id", requestID),
			zap.String("method", c.Request.Method),
			zap.String("route", route),
			zap.String("path", safePath),
			zap.Int("status", status),
			zap.Int64("duration_ms", time.Since(start).Milliseconds()),
			zap.String("client_ip", c.ClientIP()),
			zap.Int("bytes", c.Writer.Size()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("error", c.Errors.String()))
		}
		requestLogger := logger.FromContext(c.Request.Context())
		if status >= 500 {
			trace.SpanFromContext(c.Request.Context()).SetStatus(codes.Error, fmt.Sprintf("HTTP %d", status))
			requestLogger.Error("http_request", fields...)
		} else if status >= 400 {
			requestLogger.Warn("http_request", fields...)
		} else if c.Request.URL.Path == "/health" || c.Request.URL.Path == "/healthz" || c.Request.URL.Path == "/readyz" {
			requestLogger.Debug("http_request", fields...)
		} else {
			requestLogger.Info("http_request", fields...)
		}
	}
}

// RecoveryMiddleware 恢复中间件
func RecoveryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				requestID, _ := c.Get("request_id")
				span := trace.SpanFromContext(c.Request.Context())
				panicErr := fmt.Errorf("panic: %v", err)
				span.RecordError(panicErr)
				span.SetStatus(codes.Error, "request panic")
				logger.FromContext(c.Request.Context()).Error("http_panic",
					zap.Any("req_id", requestID),
					zap.String("method", c.Request.Method),
					zap.String("path", safeRequestPath(c.Request.URL.Path)),
					zap.Any("panic", err),
					zap.ByteString("stack", debug.Stack()))
				response.InternalError(c, "Internal Server Error", panicErr)
			}
		}()
		c.Next()
	}
}

// AuditLogMiddleware 操作日志中间件
func AuditLogMiddleware() gin.HandlerFunc {
	startAuditWorker()

	return func(c *gin.Context) {
		start := time.Now()

		c.Next()

		latency := time.Since(start).Milliseconds()
		statusCode := c.Writer.Status()
		if !shouldRecordRequestAudit(c, statusCode) {
			return
		}
		errorMessage := c.Errors.String()

		log := buildRequestAuditEntry(c, statusCode, latency, errorMessage)
		if log == nil {
			return
		}
		if log.Action == "super_admin_cross_workspace_read" {
			if err := dao.NewOperationLogDAO().Create(log); err != nil {
				logger.Error("cross_workspace_audit_write_failed", "error", err, "request_id", log.RequestID, "user_id", log.UserID)
			}
			return
		}

		select {
		case auditQueue <- log:
		default:
			logger.Warn("audit_queue_full", "method", log.Method, "path", log.Path, "user_id", log.UserID)
		}
	}
}

func shouldRecordRequestAudit(c *gin.Context, status int) bool {
	method := c.Request.Method
	if hasDedicatedDownloadAudit(method, c.Request.URL.Path) {
		return false
	}
	if authorizationWasDenied(c) {
		return true
	}
	if method == http.MethodGet || method == http.MethodHead {
		return authorizationDecisionExists(c)
	}
	return method != http.MethodOptions
}

func buildRequestAuditEntry(c *gin.Context, statusCode int, latency int64, errorMessage string) *model.OperationLog {
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	uid, _ := userID.(uint)
	uname, _ := username.(string)
	if uid == 0 && uname == "" {
		return nil
	}
	action := c.FullPath()
	details := authorizationAuditDetails(c, statusCode)
	entry := &model.OperationLog{
		UserID:       uid,
		Username:     uname,
		Method:       c.Request.Method,
		Path:         safeRequestPath(c.Request.URL.Path),
		Action:       action,
		Status:       statusCode,
		IP:           c.ClientIP(),
		Latency:      latency,
		Details:      details,
		RequestID:    requestIDFromContext(c),
		UserAgent:    c.Request.UserAgent(),
		TraceID:      traceIDFromContext(c),
		ErrorMessage: errorMessage,
		CreatedAt:    time.Now(),
	}
	if last, exists := auditcontext.LastAuthorizationTargetCheck(c); exists {
		entry.TargetType = last.TargetType
		entry.TargetID = last.TargetID
	}
	crossWorkspaceRead := false
	if access, exists := auditcontext.CrossWorkspaceAccessFrom(c); exists {
		entry.ActorWorkspaceID = access.SourceWorkspaceID
		entry.SourceWorkspaceID = access.SourceWorkspaceID
		entry.TargetWorkspaceID = &access.TargetWorkspaceID
		crossWorkspaceRead = isCrossWorkspaceReadRequest(c)
	}
	if crossWorkspaceRead {
		entry.Action = "super_admin_cross_workspace_read"
		entry.Category = model.AuditCategorySecurity
		entry.Severity = model.AuditSeverityHigh
		switch {
		case authorizationWasDenied(c):
			entry.Result = model.AuditResultDenied
			entry.ReasonCode = model.AuditReasonPermissionDenied
		case statusCode >= http.StatusBadRequest:
			entry.Result = model.AuditResultFailure
		default:
			entry.Result = model.AuditResultSuccess
		}
	} else if authorizationWasDenied(c) {
		entry.Action = "permission:denied"
		entry.Category = model.AuditCategorySecurity
		entry.Severity = model.AuditSeverityWarning
		entry.Result = model.AuditResultDenied
		entry.ReasonCode = model.AuditReasonPermissionDenied
	} else if (c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead) && authorizationWasAllowed(c) {
		entry.Action = "permission:allowed"
		entry.Category = model.AuditCategoryAccess
		entry.Severity = model.AuditSeverityInfo
		entry.Result = model.AuditResultSuccess
	}
	if workspaceValue, exists := c.Get("workspace_id"); exists {
		if workspaceID, valid := workspaceValue.(uint); valid && workspaceID > 0 {
			entry.WorkspaceID = &workspaceID
		}
	}
	return entry
}

func setAuthorizationDecision(c *gin.Context, decision, permission, scope string) {
	setAuthorizationDecisionForTarget(c, decision, permission, scope, "", "")
}

func setAuthorizationDecisionForTarget(c *gin.Context, decision, permission, scope, targetType, targetID string) {
	auditcontext.RecordAuthorization(c, auditcontext.AuthorizationCheck{
		Decision: decision, Permission: permission, Scope: scope, TargetType: targetType, TargetID: targetID,
	})
}

func authorizationAuditDetails(c *gin.Context, status int) string {
	if !authorizationDecisionExists(c) {
		return "{}"
	}
	last, _ := auditcontext.LastAuthorizationCheck(c)
	payload := map[string]any{
		"decision":   last.Decision,
		"permission": last.Permission,
		"scope":      last.Scope,
		"checks":     auditcontext.AuthorizationChecks(c),
	}
	if access, exists := auditcontext.CrossWorkspaceAccessFrom(c); exists {
		payload["cross_workspace_access"] = true
		payload["source_workspace_id"] = access.SourceWorkspaceID
		payload["target_workspace_id"] = access.TargetWorkspaceID
		payload["operation_reason"] = access.Reason
	}
	if authorizationWasDenied(c) {
		if last.Decision == "allowed" {
			payload["functional_decision"] = "allowed"
		}
		payload["decision"] = "denied"
		payload["reason"] = model.AuditReasonPermissionDenied
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func userIDFromContext(c *gin.Context) uint {
	value, _ := c.Get("user_id")
	userID, _ := value.(uint)
	return userID
}

func isCrossWorkspaceReadRequest(c *gin.Context) bool {
	if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
		return false
	}
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	for _, prefix := range []string{
		"/api/fileshare/v1/management/folders",
		"/api/fileshare/v1/management/nodes",
		"/api/fileshare/v1/management/files",
		"/api/fileshare/v1/management/favorites",
		"/api/fileshare/v1/management/trash",
		"/api/fileshare/v1/management/search",
		"/api/fileshare/v1/management/batch-downloads",
	} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func authorizationWasDenied(c *gin.Context) bool {
	last, exists := auditcontext.LastAuthorizationCheck(c)
	return exists && last.Decision == "denied"
}

func authorizationWasAllowed(c *gin.Context) bool {
	last, exists := auditcontext.LastAuthorizationCheck(c)
	return exists && last.Decision == "allowed"
}

func authorizationDecisionExists(c *gin.Context) bool {
	return authorizationWasAllowed(c) || authorizationWasDenied(c)
}

func hasDedicatedDownloadAudit(method, path string) bool {
	if method != http.MethodGet {
		return false
	}
	path = strings.TrimSuffix(path, "/")
	return strings.Contains(path, "/management/files/") && strings.HasSuffix(path, "/download")
}

func safeRequestPath(path string) string {
	return logger.SanitizeText(path)
}

func requestIDFromContext(c *gin.Context) string {
	value, _ := c.Get("request_id")
	requestID, _ := value.(string)
	return requestID
}

func traceIDFromContext(c *gin.Context) string {
	span := trace.SpanFromContext(c.Request.Context())
	if !span.SpanContext().IsValid() {
		return ""
	}
	return span.SpanContext().TraceID().String()
}

var (
	auditQueue = make(chan *model.OperationLog, 2048)
	auditOnce  sync.Once
)

func startAuditWorker() {
	auditOnce.Do(func() {
		logDAO := dao.NewOperationLogDAO()
		go func() {
			for entry := range auditQueue {
				if err := logDAO.Create(entry); err != nil {
					logger.Error("save_audit_log", "error", err, "path", entry.Path)
				}
			}
		}()
	})
}
