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
	"regexp"
	"strconv"
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var roleCodePattern = regexp.MustCompile(`^[a-z][a-z0-9:_-]{2,63}$`)

type RoleHandler struct {
	roles      *dao.RoleDAO
	workspaces *dao.WorkspaceDAO
}

func NewRoleHandler() *RoleHandler {
	return &RoleHandler{roles: dao.NewRoleDAO(), workspaces: dao.NewWorkspaceDAO()}
}

func (h *RoleHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	roles, err := h.roles.ListPage(actor.WorkspaceID, page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "读取角色失败", err)
		return
	}
	response.SuccessWithPage(c, roles.List, roles.Total, roles.Page, roles.PageSize)
}

func (h *RoleHandler) ListPermissions(c *gin.Context) {
	permissions, err := h.roles.ListPermissions()
	if err != nil {
		response.InternalError(c, "读取权限点失败", err)
		return
	}
	response.Success(c, permissions)
}

func (h *RoleHandler) Get(c *gin.Context) {
	actor, roleID, ok := roleContext(c)
	if !ok {
		return
	}
	role, permissions, err := h.roles.GetWithPermissions(actor.WorkspaceID, roleID)
	if err != nil {
		response.InternalError(c, "读取角色失败", err)
		return
	}
	if role == nil {
		response.NotFound(c, "角色不存在")
		return
	}
	codes := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		codes = append(codes, permission.Code)
	}
	response.Success(c, gin.H{"role": role, "permissions": codes})
}

func (h *RoleHandler) Create(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Code        string `json:"code" binding:"required,max=64"`
		Name        string `json:"name" binding:"required,max=64"`
		Description string `json:"description" binding:"max=1000"`
		SortOrder   int    `json:"sort_order"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	req.Name = strings.TrimSpace(req.Name)
	if !roleCodePattern.MatchString(req.Code) {
		response.BadRequest(c, "角色编码格式不合法")
		return
	}
	role := &model.Role{
		WorkspaceID: actor.WorkspaceID, Code: req.Code, Name: req.Name,
		Description: req.Description, SortOrder: req.SortOrder, Status: 1, CreatedBy: actor.UserID,
	}
	workspaceID := actor.WorkspaceID
	if err := h.roles.CreateWithAudit(role, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "role:create", "role", "0", role.Name)); err != nil {
		response.Conflict(c, "角色编码已存在")
		return
	}
	response.Success(c, role)
}

func (h *RoleHandler) Update(c *gin.Context) {
	actor, roleID, ok := roleContext(c)
	if !ok {
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required,max=64"`
		Description string `json:"description" binding:"max=1000"`
		SortOrder   int    `json:"sort_order"`
		Status      int    `json:"status" binding:"oneof=0 1"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	workspaceID := actor.WorkspaceID
	err := h.roles.UpdateWithAudit(actor.WorkspaceID, roleID, map[string]any{
		"name": strings.TrimSpace(req.Name), "description": req.Description,
		"sort_order": req.SortOrder, "status": req.Status,
	}, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "role:update", "role", strconv.FormatUint(uint64(roleID), 10), req.Name))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "角色不存在")
		return
	}
	if err != nil {
		response.InternalError(c, "更新角色失败", err)
		return
	}
	response.Success(c, gin.H{"id": roleID})
}

func (h *RoleHandler) Delete(c *gin.Context) {
	actor, roleID, ok := roleContext(c)
	if !ok {
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.roles.DeleteWithAudit(actor.WorkspaceID, roleID,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "role:delete", "role", strconv.FormatUint(uint64(roleID), 10), "")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		response.InternalError(c, "删除角色失败", err)
		return
	}
	response.SuccessWithMessage(c, "删除成功", nil)
}

func (h *RoleHandler) ReplacePermissions(c *gin.Context) {
	actor, roleID, ok := roleContext(c)
	if !ok {
		return
	}
	var req struct {
		Permissions []string `json:"permissions" binding:"max=100,dive,required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if hasDuplicates(req.Permissions) {
		response.BadRequest(c, "权限点不能重复")
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.roles.ReplacePermissionsWithAudit(actor.WorkspaceID, roleID, req.Permissions,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "role:permissions_replace", "role", strconv.FormatUint(uint64(roleID), 10), "")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "角色不存在")
			return
		}
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"id": roleID, "permissions": req.Permissions})
}

func (h *RoleHandler) AssignUserRoles(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	userID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	membership, err := h.workspaces.GetMembership(actor.WorkspaceID, userID)
	if err != nil {
		response.InternalError(c, "检查工作空间成员失败", err)
		return
	}
	if membership == nil {
		response.BadRequest(c, "用户不是当前工作空间成员")
		return
	}
	var req struct {
		RoleIDs []uint `json:"role_ids" binding:"max=100,dive,gt=0"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if hasDuplicateUint(req.RoleIDs) {
		response.BadRequest(c, "角色不能重复")
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.roles.AssignUserRolesWithAudit(actor.WorkspaceID, userID, req.RoleIDs,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "user:roles_replace", "user", strconv.FormatUint(uint64(userID), 10), "")); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	response.Success(c, gin.H{"user_id": userID, "role_ids": req.RoleIDs})
}

func (h *RoleHandler) BatchAssignUserRoles(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		UserIDs []uint `json:"user_ids" binding:"required,max=200,dive,gt=0"`
		RoleIDs []uint `json:"role_ids" binding:"required,max=100,dive,gt=0"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if len(req.UserIDs) == 0 || len(req.RoleIDs) == 0 || hasDuplicateUint(req.UserIDs) || hasDuplicateUint(req.RoleIDs) {
		response.BadRequest(c, "用户和角色不能为空且不能重复")
		return
	}
	for _, userID := range req.UserIDs {
		membership, err := h.workspaces.GetMembership(actor.WorkspaceID, userID)
		if err != nil {
			response.InternalError(c, "检查工作空间成员失败", err)
			return
		}
		if membership == nil {
			response.BadRequest(c, "只能为当前工作空间成员分配角色")
			return
		}
		workspaceID := actor.WorkspaceID
		if err := h.roles.AssignUserRolesWithAudit(actor.WorkspaceID, userID, req.RoleIDs,
			newBusinessAuditEvent(c, actor.UserID, &workspaceID, "user:roles_replace", "user", strconv.FormatUint(uint64(userID), 10), "")); err != nil {
			response.BadRequest(c, err.Error())
			return
		}
	}
	response.Success(c, gin.H{"user_ids": req.UserIDs, "role_ids": req.RoleIDs})
}

func roleContext(c *gin.Context) (authorization.Actor, uint, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, 0, false
	}
	roleID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return authorization.Actor{}, 0, false
	}
	return actor, roleID, true
}

func hasDuplicates(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func hasDuplicateUint(values []uint) bool {
	seen := make(map[uint]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}
