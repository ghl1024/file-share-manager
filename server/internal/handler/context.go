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
	"strconv"
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/auditcontext"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/trace"
)

func rememberRecentNode(store *dao.CollaborationDAO, actor authorization.Actor, nodeID uint) {
	if store == nil || nodeID == 0 {
		return
	}
	if err := store.TouchRecent(actor.WorkspaceID, actor.UserID, nodeID); err != nil {
		logger.Warn("recent_node_access_failed", "workspace_id", actor.WorkspaceID, "user_id", actor.UserID, "node_id", nodeID, "error", err)
	}
}

func recordDataAuthorization(c *gin.Context, allowed bool, permission, targetType string, targetID uint) {
	recordAuthorization(c, allowed, permission, "acl", targetType, targetID)
}

func recordWorkspaceAuthorization(c *gin.Context, allowed bool, permission string, workspaceID uint) {
	recordAuthorization(c, allowed, permission, "workspace", "workspace", workspaceID)
}

func recordAuthorization(c *gin.Context, allowed bool, permission, scope, targetType string, targetID uint) {
	decision := "denied"
	if allowed {
		decision = "allowed"
	}
	auditcontext.RecordAuthorization(c, auditcontext.AuthorizationCheck{
		Decision: decision, Permission: permission, Scope: scope,
		TargetType: targetType, TargetID: strconv.FormatUint(uint64(targetID), 10),
	})
}

func actorFromContext(c *gin.Context) (authorization.Actor, bool) {
	userValue, userExists := c.Get("user_id")
	userID, userValid := userValue.(uint)
	workspaceValue, workspaceExists := c.Get("workspace_id")
	workspaceID, workspaceValid := workspaceValue.(uint)
	if !userExists || !userValid || !workspaceExists || !workspaceValid {
		response.Forbidden(c, "缺少工作空间权限上下文")
		return authorization.Actor{}, false
	}
	isSuperAdmin, _ := c.Get("is_super_admin")
	workspaceRole, _ := c.Get("workspace_role")
	return authorization.Actor{
		UserID:        userID,
		WorkspaceID:   workspaceID,
		IsSuperAdmin:  isSuperAdmin == true,
		WorkspaceRole: stringValue(workspaceRole),
	}, true
}

func normalizeNodeName(name string) (string, string, bool) {
	displayName := strings.TrimSpace(name)
	if displayName == "" || displayName == "." || displayName == ".." || strings.ContainsAny(displayName, "/\\\x00") {
		return "", "", false
	}
	return displayName, strings.ToLower(displayName), true
}

func stringValue(value any) string {
	result, _ := value.(string)
	return result
}

// newBusinessAuditEvent builds the immutable request metadata shared by
// mutation handlers. The DAO fills BeforeJSON/AfterJSON and writes the event
// through the same transaction as the business change.
func newBusinessAuditEvent(c *gin.Context, userID uint, workspaceID *uint, action, targetType, targetID, targetName string) *model.OperationLog {
	path := c.FullPath()
	if path == "" {
		path = c.Request.URL.Path
	}
	requestID := stringValueFromContext(c, "request_id")
	traceID := ""
	spanContext := trace.SpanFromContext(c.Request.Context()).SpanContext()
	if spanContext.IsValid() {
		traceID = spanContext.TraceID().String()
	}
	event := &model.OperationLog{
		UserID: userID, Username: stringValueFromContext(c, "username"), WorkspaceID: workspaceID,
		Method: c.Request.Method, Path: path, Action: action,
		Category: model.AuditCategoryOperation, Severity: model.AuditSeverityInfo,
		Result: model.AuditResultSuccess, Status: 200, IP: c.ClientIP(),
		RequestID: requestID, TraceID: traceID, UserAgent: c.Request.UserAgent(),
		TargetType: targetType, TargetID: targetID, TargetName: targetName,
	}
	if workspaceID != nil {
		actorWorkspaceID := *workspaceID
		event.ActorWorkspaceID = &actorWorkspaceID
		event.TargetWorkspaceID = &actorWorkspaceID
	}
	return event
}

func stringValueFromContext(c *gin.Context, key string) string {
	value, _ := c.Get(key)
	return stringValue(value)
}
