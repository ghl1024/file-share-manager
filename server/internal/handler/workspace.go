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
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type WorkspaceHandler struct {
	wsDAO   *dao.WorkspaceDAO
	userDAO *dao.UserDAO
}

func NewWorkspaceHandler() *WorkspaceHandler {
	return &WorkspaceHandler{
		wsDAO:   dao.NewWorkspaceDAO(),
		userDAO: dao.NewUserDAO(),
	}
}

// CreateWorkspace 超管创建工作空间
func (h *WorkspaceHandler) CreateWorkspace(c *gin.Context) {
	var req struct {
		Name       string `json:"name" binding:"required,max=128"`
		Code       string `json:"code" binding:"required,max=64"`
		QuotaBytes *int64 `json:"quota_bytes"` // null 为不限制
		OwnerID    *uint  `json:"owner_id"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.QuotaBytes != nil && *req.QuotaBytes < 0 {
		response.BadRequest(c, "quota_bytes 不能为负数")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`).MatchString(req.Code) {
		response.BadRequest(c, "工作空间代号必须以小写字母开头，仅包含小写字母、数字和连字符")
		return
	}
	actorID, _ := c.Get("user_id")
	ownerID := actorID.(uint)
	if req.OwnerID != nil {
		ownerID = *req.OwnerID
	}
	owner, err := h.userDAO.GetByID(ownerID)
	if err != nil {
		response.InternalError(c, "Database error", err)
		return
	}
	if owner == nil || owner.Status != 1 {
		response.BadRequest(c, "指定的工作空间管理员不存在或已停用")
		return
	}

	// Code 重复性校验
	existing, err := h.wsDAO.GetByCode(req.Code)
	if err != nil {
		response.InternalError(c, "Database error", err)
		return
	}
	if existing != nil {
		response.BadRequest(c, "Workspace Code 已存在")
		return
	}

	ws := &model.Workspace{
		UUID:       uuid.New().String(),
		Name:       req.Name,
		Code:       req.Code,
		QuotaBytes: req.QuotaBytes,
		Status:     1,
		CreatedBy:  actorID.(uint),
	}

	if err := h.wsDAO.CreateWorkspaceWithAudit(ws, ownerID, actorID.(uint),
		newBusinessAuditEvent(c, actorID.(uint), nil, "workspace:create", "workspace", "0", ws.Name)); err != nil {
		logger.Error("Failed to create workspace", "error", err)
		response.InternalError(c, "Failed to create workspace", err)
		return
	}

	response.Success(c, gin.H{"id": ws.ID, "name": ws.Name, "code": ws.Code})
}

// ListWorkspaces 获取工作空间列表
func (h *WorkspaceHandler) ListWorkspaces(c *gin.Context) {
	userID, _ := c.Get("user_id")
	isSuperAdmin, _ := c.Get("is_super_admin")

	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	workspaces, err := h.wsDAO.ListPageForUser(userID.(uint), isSuperAdmin.(bool), page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "Database error", err)
		return
	}

	response.SuccessWithPage(c, workspaces.List, workspaces.Total, workspaces.Page, workspaces.PageSize)
}

func (h *WorkspaceHandler) UpdateQuota(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间配额")
		return
	}
	quotaBytes, ok := bindQuota(c)
	if !ok {
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.wsDAO.UpdateQuotaWithAudit(workspaceID, quotaBytes,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "workspace:quota_update", "workspace", fmt.Sprint(workspaceID), "")); err != nil {
		h.handleQuotaUpdateError(c, err, "工作空间不存在")
		return
	}
	response.Success(c, gin.H{"workspace_id": workspaceID, "quota_bytes": quotaBytes})
}

func (h *WorkspaceHandler) UpdateMemberQuota(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	userID, err := request.ParseUintParam(c, "uid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该成员配额")
		return
	}
	quotaBytes, ok := bindQuota(c)
	if !ok {
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.wsDAO.UpdateMemberQuotaWithAudit(workspaceID, userID, quotaBytes,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "workspace:member_quota_update", "workspace_membership", fmt.Sprint(userID), "")); err != nil {
		h.handleQuotaUpdateError(c, err, "工作空间成员不存在")
		return
	}
	response.Success(c, gin.H{"workspace_id": workspaceID, "user_id": userID, "quota_bytes": quotaBytes})
}

// AddMember 向工作空间添加成员或修改其配额/角色
func (h *WorkspaceHandler) AddMember(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	var req struct {
		UserID     uint   `json:"user_id" binding:"required"`
		Role       string `json:"role" binding:"required"` // workspace_admin or member
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.QuotaBytes != nil && *req.QuotaBytes < 0 {
		response.BadRequest(c, "quota_bytes 不能为负数")
		return
	}
	if req.Role != "workspace_admin" && req.Role != "member" {
		response.BadRequest(c, "role 必须是 workspace_admin 或 member")
		return
	}

	// 权限检查：当前调用者必须是该空间的 workspace_admin 或者是 超管
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间成员")
		return
	}
	targetUser, err := h.userDAO.GetByID(req.UserID)
	if err != nil {
		response.InternalError(c, "Database error", err)
		return
	}
	if targetUser == nil || targetUser.Status != 1 {
		response.BadRequest(c, "用户不存在或已停用")
		return
	}
	actorID, _ := c.Get("user_id")

	membership := &model.WorkspaceMembership{
		WorkspaceID: workspaceID,
		UserID:      req.UserID,
		Role:        req.Role,
		QuotaBytes:  req.QuotaBytes,
		CreatedBy:   actorID.(uint),
	}

	if err := h.wsDAO.UpsertMemberWithAudit(membership,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "workspace:member_upsert", "workspace_membership", fmt.Sprint(req.UserID), targetUser.Username)); err != nil {
		if errors.Is(err, dao.ErrQuotaBelowUsage) {
			response.Conflict(c, "配额不能低于当前已用容量和上传预留容量之和")
			return
		}
		if errors.Is(err, dao.ErrInvalidWorkspaceRole) {
			response.BadRequest(c, "role 必须是 workspace_admin 或 member")
			return
		}
		if errors.Is(err, dao.ErrLastWorkspaceAdmin) {
			response.Conflict(c, "工作空间至少需要保留一名管理员")
			return
		}
		logger.Error("Failed to upsert member", "error", err)
		response.InternalError(c, "Failed to update member", err)
		return
	}

	response.Success(c, "添加/更新成功")
}

func (h *WorkspaceHandler) ListMembers(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceMember(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权访问该工作空间")
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	members, err := h.wsDAO.ListMembersPage(workspaceID, page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "读取工作空间成员失败", err)
		return
	}
	response.SuccessWithPage(c, members.List, members.Total, members.Page, members.PageSize)
}

func (h *WorkspaceHandler) ListAvailableUsers(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间成员")
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	users, err := h.userDAO.ListActivePage(page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "查询可选用户失败", err)
		return
	}
	items := make([]gin.H, 0, len(users.List))
	for _, user := range users.List {
		items = append(items, gin.H{
			"id": user.ID, "username": user.Username, "real_name": user.RealName, "email": user.Email,
		})
	}
	response.SuccessWithPage(c, items, users.Total, users.Page, users.PageSize)
}

func (h *WorkspaceHandler) RemoveMember(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	userID, err := request.ParseUintParam(c, "uid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间成员")
		return
	}
	target, err := h.userDAO.GetByID(userID)
	if err != nil {
		response.InternalError(c, "读取工作空间成员失败", err)
		return
	} else if target != nil && target.IsSuperAdmin {
		isSuperAdmin, _ := c.Get("is_super_admin")
		if allowed, ok := isSuperAdmin.(bool); !ok || !allowed {
			response.Forbidden(c, "超级管理员成员只能由超级管理员管理")
			return
		}
	}
	actorID, _ := c.Get("user_id")
	targetName := ""
	if target != nil {
		targetName = target.Username
	}
	if err := h.wsDAO.RemoveMemberWithAudit(workspaceID, userID,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "workspace:member_remove", "workspace_membership", fmt.Sprint(userID), targetName)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "工作空间成员不存在")
			return
		}
		if strings.Contains(err.Error(), "至少需要") {
			response.Conflict(c, err.Error())
			return
		}
		response.InternalError(c, "移除工作空间成员失败", err)
		return
	}
	response.SuccessWithMessage(c, "移除成功", nil)
}

func bindQuota(c *gin.Context) (*int64, bool) {
	var req map[string]json.RawMessage
	if !request.BindJSON(c, &req) {
		return nil, false
	}
	raw, exists := req["quota_bytes"]
	if !exists {
		response.BadRequest(c, "quota_bytes 字段不能为空")
		return nil, false
	}
	var quotaBytes *int64
	if err := json.Unmarshal(raw, &quotaBytes); err != nil {
		response.BadRequest(c, "quota_bytes 必须是整数或 null")
		return nil, false
	}
	if quotaBytes != nil && *quotaBytes < 0 {
		response.BadRequest(c, "quota_bytes 不能为负数")
		return nil, false
	}
	return quotaBytes, true
}

func (h *WorkspaceHandler) handleQuotaUpdateError(c *gin.Context, err error, notFoundMessage string) {
	switch {
	case errors.Is(err, dao.ErrInvalidQuota):
		response.BadRequest(c, "quota_bytes 不能为负数")
	case errors.Is(err, dao.ErrQuotaBelowUsage):
		response.Conflict(c, "配额不能低于当前已用容量和上传预留容量之和")
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, notFoundMessage)
	default:
		response.InternalError(c, "更新配额失败", err)
	}
}

// 提取的鉴权辅助函数
func checkWorkspaceAdmin(c *gin.Context, wsDAO *dao.WorkspaceDAO, workspaceID uint) bool {
	isSuperAdmin, _ := c.Get("is_super_admin")
	if allowed, ok := isSuperAdmin.(bool); ok && allowed {
		recordWorkspaceAuthorization(c, true, "workspace:manage", workspaceID)
		return true
	}
	userID, _ := c.Get("user_id")
	uid, ok := userID.(uint)
	if !ok {
		recordWorkspaceAuthorization(c, false, "workspace:manage", workspaceID)
		return false
	}
	m, err := wsDAO.GetMembership(workspaceID, uid)
	if err != nil || m == nil {
		recordWorkspaceAuthorization(c, false, "workspace:manage", workspaceID)
		return false
	}
	allowed := m.Role == "workspace_admin"
	recordWorkspaceAuthorization(c, allowed, "workspace:manage", workspaceID)
	return allowed
}
