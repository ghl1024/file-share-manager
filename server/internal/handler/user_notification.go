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
	"errors"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/notification"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UserNotificationHandler struct {
	notifications *dao.NotificationDAO
	workspaces    *dao.WorkspaceDAO
	authz         *authorization.Service
	batches       *dao.BatchDownloadDAO
	shares        *dao.ShareDAO
}

func NewUserNotificationHandler() *UserNotificationHandler {
	return &UserNotificationHandler{
		notifications: dao.NewNotificationDAO(), workspaces: dao.NewWorkspaceDAO(),
		authz: authorization.NewService(), batches: dao.NewBatchDownloadDAO(), shares: dao.NewShareDAO(),
	}
}

func (h *UserNotificationHandler) List(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	category := strings.TrimSpace(c.Query("category"))
	if category != "" && !notification.ValidUserCategory(category) {
		response.BadRequest(c, "通知分类无效")
		return
	}
	unreadOnly := false
	if raw := strings.TrimSpace(c.Query("unread_only")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "unread_only 必须是布尔值")
			return
		}
		unreadOnly = value
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	result, err := h.notifications.ListUserNotifications(userID, page, pageSize, dao.UserNotificationFilter{Category: category, UnreadOnly: unreadOnly})
	if err != nil {
		response.InternalError(c, "读取站内通知失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

func (h *UserNotificationHandler) UnreadCount(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	count, err := h.notifications.CountUnreadUserNotifications(userID)
	if err != nil {
		response.InternalError(c, "读取未读通知数失败", err)
		return
	}
	response.Success(c, gin.H{"unread_count": count})
}

func (h *UserNotificationHandler) MarkRead(c *gin.Context) {
	userID, id, ok := notificationIdentity(c)
	if !ok {
		return
	}
	item, err := h.notifications.GetUserNotification(userID, id)
	if err != nil {
		response.InternalError(c, "读取站内通知失败", err)
		return
	}
	if item == nil {
		response.NotFound(c, "通知不存在")
		return
	}
	if err := h.notifications.MarkUserNotificationRead(userID, id, time.Now()); err != nil {
		response.InternalError(c, "更新通知状态失败", err)
		return
	}
	response.Success(c, gin.H{"id": id, "is_read": true})
}

func (h *UserNotificationHandler) MarkAllRead(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	count, err := h.notifications.MarkAllUserNotificationsRead(userID, time.Now())
	if err != nil {
		response.InternalError(c, "批量更新通知状态失败", err)
		return
	}
	response.Success(c, gin.H{"updated_count": count})
}

func (h *UserNotificationHandler) Preferences(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	preference, err := h.notifications.GetUserNotificationPreference(userID)
	if err != nil {
		response.InternalError(c, "读取通知偏好失败", err)
		return
	}
	response.Success(c, preference)
}

func (h *UserNotificationHandler) SavePreferences(c *gin.Context) {
	userID, ok := currentUserID(c)
	if !ok {
		return
	}
	var req struct {
		CollaborationEnabled *bool `json:"collaboration_enabled" binding:"required"`
		TaskEnabled          *bool `json:"task_enabled" binding:"required"`
		SecurityEnabled      *bool `json:"security_enabled" binding:"required"`
		ShareEnabled         *bool `json:"share_enabled" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	preference := &model.UserNotificationPreference{
		UserID: userID, CollaborationEnabled: *req.CollaborationEnabled, TaskEnabled: *req.TaskEnabled,
		SecurityEnabled: *req.SecurityEnabled, ShareEnabled: *req.ShareEnabled, UpdatedAt: time.Now(),
	}
	if err := h.notifications.SaveUserNotificationPreference(preference); err != nil {
		response.InternalError(c, "保存通知偏好失败", err)
		return
	}
	response.Success(c, preference)
}

// Open re-authorizes the target before returning a route. A stale notification
// is still marked read, but never becomes a capability to the old target.
func (h *UserNotificationHandler) Open(c *gin.Context) {
	userID, id, ok := notificationIdentity(c)
	if !ok {
		return
	}
	item, err := h.notifications.GetUserNotification(userID, id)
	if err != nil {
		response.InternalError(c, "读取站内通知失败", err)
		return
	}
	if item == nil {
		response.NotFound(c, "通知不存在")
		return
	}
	if err := h.notifications.MarkUserNotificationRead(userID, id, time.Now()); err != nil {
		response.InternalError(c, "更新通知状态失败", err)
		return
	}
	path, workspaceID, err := h.resolveTarget(userID, item)
	if err != nil {
		if errors.Is(err, errNotificationTargetUnavailable) {
			response.Gone(c, "相关内容已不可访问")
			return
		}
		response.InternalError(c, "校验通知目标失败", err)
		return
	}
	response.Success(c, gin.H{"path": path, "workspace_id": workspaceID})
}

var errNotificationTargetUnavailable = errors.New("notification target is unavailable")

func (h *UserNotificationHandler) resolveTarget(userID uint, item *model.UserNotification) (string, *uint, error) {
	if item == nil || item.WorkspaceID == nil {
		return "/dashboard", nil, nil
	}
	workspaceID := *item.WorkspaceID
	membership, err := h.workspaces.GetMembership(workspaceID, userID)
	if err != nil {
		return "", nil, err
	}
	if membership == nil {
		return "", nil, errNotificationTargetUnavailable
	}
	actor := authorization.Actor{UserID: userID, WorkspaceID: workspaceID, WorkspaceRole: membership.Role}
	switch item.TargetType {
	case "node":
		nodeID, parseErr := strconv.ParseUint(item.TargetID, 10, 0)
		if parseErr != nil || nodeID == 0 {
			return "", nil, errNotificationTargetUnavailable
		}
		allowed, authErr := h.authz.CanRead(actor, uint(nodeID))
		if authErr != nil && !errors.Is(authErr, authorization.ErrNodeNotFound) {
			return "", nil, authErr
		}
		if !allowed {
			return "", nil, errNotificationTargetUnavailable
		}
		return "/files?node_id=" + strconv.FormatUint(nodeID, 10) + "&panel=collaboration", &workspaceID, nil
	case "batch_download":
		job, loadErr := h.batches.GetForOwner(workspaceID, userID, item.TargetID)
		if loadErr != nil {
			return "", nil, loadErr
		}
		if job == nil {
			return "", nil, errNotificationTargetUnavailable
		}
		return "/files", &workspaceID, nil
	case "share":
		shareID, parseErr := strconv.ParseUint(item.TargetID, 10, 0)
		if parseErr != nil || shareID == 0 {
			return "", nil, errNotificationTargetUnavailable
		}
		share, loadErr := h.shares.GetByID(workspaceID, uint(shareID))
		if loadErr != nil {
			return "", nil, loadErr
		}
		if share == nil || share.CreatedBy != userID {
			return "", nil, errNotificationTargetUnavailable
		}
		return "/shares", &workspaceID, nil
	default:
		return "/dashboard", &workspaceID, nil
	}
}

func currentUserID(c *gin.Context) (uint, bool) {
	value, exists := c.Get("user_id")
	userID, valid := value.(uint)
	if !exists || !valid || userID == 0 {
		response.Unauthorized(c, "登录状态无效")
		return 0, false
	}
	return userID, true
}

func notificationIdentity(c *gin.Context) (uint, string, bool) {
	userID, ok := currentUserID(c)
	if !ok {
		return 0, "", false
	}
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(c, "通知 ID 格式错误")
		return 0, "", false
	}
	return userID, id, true
}
