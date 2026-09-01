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
	"fmt"
	"strconv"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ACLHandler struct {
	nodes      *dao.NodeDAO
	acls       *dao.ACLDAO
	groups     *dao.GroupDAO
	workspaces *dao.WorkspaceDAO
	authz      *authorization.Service
}

type aclInput struct {
	SubjectType       string `json:"subject_type" binding:"required"`
	SubjectID         uint   `json:"subject_id" binding:"required"`
	Effect            string `json:"effect" binding:"required"`
	AccessLevel       string `json:"access_level" binding:"required"`
	InheritToChildren bool   `json:"inherit_to_children"`
}

func NewACLHandler() *ACLHandler {
	return &ACLHandler{
		nodes: dao.NewNodeDAO(), acls: dao.NewACLDAO(), groups: dao.NewGroupDAO(),
		workspaces: dao.NewWorkspaceDAO(), authz: authorization.NewService(),
	}
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/folders/{id}/acl.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/folders/{id}/acl [get]
func (h *ACLHandler) List(c *gin.Context) {
	actor, nodeID, ok := h.authorize(c)
	if !ok {
		return
	}
	entries, err := h.acls.ListDirectPermissions(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取目录权限失败", err)
		return
	}
	response.Success(c, entries)
}

// @Summary Replace
// @Description Handles PUT /api/fileshare/v1/management/folders/{id}/acl.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/folders/{id}/acl [put]
func (h *ACLHandler) Replace(c *gin.Context) {
	actor, nodeID, ok := h.authorize(c)
	if !ok {
		return
	}
	var req struct {
		Entries []aclInput `json:"entries" binding:"required,min=1,dive"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	entries, err := h.validateEntries(actor.WorkspaceID, req.Entries)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	before, err := h.acls.ListDirectPermissions(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取原目录权限失败", err)
		return
	}
	node, err := h.nodes.GetByID(actor.WorkspaceID, nodeID)
	if err != nil || node == nil {
		response.InternalError(c, "读取目录信息失败", err)
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.acls.ReplaceDirectPermissionsWithAudit(actor.WorkspaceID, nodeID, actor.UserID, entries,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "acl:replace", "folder", strconv.FormatUint(uint64(nodeID), 10), "")); err != nil {
		response.InternalError(c, "更新目录权限失败", err)
		return
	}
	h.publishACLChangeNotifications(c.Request.Context(), actor.WorkspaceID, nodeID, node.Name, before, entries)
	response.Success(c, entries)
}

// @Summary Set Inheritance
// @Description Handles PUT /api/fileshare/v1/management/folders/{id}/inheritance.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/folders/{id}/inheritance [put]
func (h *ACLHandler) SetInheritance(c *gin.Context) {
	actor, nodeID, ok := h.authorize(c)
	if !ok {
		return
	}
	var req struct {
		Mode string `json:"mode" binding:"required,oneof=inherit break"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.Mode == "break" {
		hasAdmin, err := h.acls.HasDirectAllowAdmin(actor.WorkspaceID, nodeID)
		if err != nil {
			response.InternalError(c, "检查目录管理员失败", err)
			return
		}
		if !hasAdmin && !actor.IsSuperAdmin && actor.WorkspaceRole != "workspace_admin" {
			response.Conflict(c, "中断继承前必须为目录保留至少一名直接管理员")
			return
		}
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodes.UpdateInheritModeWithAudit(actor.WorkspaceID, nodeID, req.Mode,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "acl:inheritance_update", "folder", strconv.FormatUint(uint64(nodeID), 10), "")); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "目录不存在")
			return
		}
		response.InternalError(c, "更新继承设置失败", err)
		return
	}
	response.Success(c, gin.H{"inherit_mode": req.Mode})
}

func (h *ACLHandler) authorize(c *gin.Context) (authorization.Actor, uint, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, 0, false
	}
	nodeID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return authorization.Actor{}, 0, false
	}
	node, err := h.nodes.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取目录失败", err)
		return authorization.Actor{}, 0, false
	}
	if node == nil || node.Type != "folder" || node.Status != "active" {
		response.NotFound(c, "目录不存在")
		return authorization.Actor{}, 0, false
	}
	allowed, err := h.authz.CanManageACL(actor, nodeID)
	if err != nil {
		response.InternalError(c, "目录权限校验失败", err)
		return authorization.Actor{}, 0, false
	}
	recordDataAuthorization(c, allowed, "node:manage_acl", "folder", nodeID)
	if !allowed {
		response.Forbidden(c, "无权管理该目录权限")
		return authorization.Actor{}, 0, false
	}
	return actor, nodeID, true
}

func (h *ACLHandler) validateEntries(workspaceID uint, inputs []aclInput) ([]model.NodeACL, error) {
	entries := make([]model.NodeACL, 0, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	hasAdmin := false
	for _, input := range inputs {
		if input.SubjectType != "user" && input.SubjectType != "group" {
			return nil, errors.New("subject_type 必须是 user 或 group")
		}
		if input.Effect != "allow" && input.Effect != "deny" {
			return nil, errors.New("effect 必须是 allow 或 deny")
		}
		if input.AccessLevel != authorization.AccessRead && input.AccessLevel != authorization.AccessReadWrite && input.AccessLevel != authorization.AccessAdmin {
			return nil, errors.New("access_level 必须是 read、read_write 或 admin")
		}
		key := fmt.Sprintf("%s:%d", input.SubjectType, input.SubjectID)
		if _, exists := seen[key]; exists {
			return nil, errors.New("同一授权主体不能重复出现")
		}
		seen[key] = struct{}{}
		if input.SubjectType == "user" {
			membership, err := h.workspaces.GetMembership(workspaceID, input.SubjectID)
			if err != nil {
				return nil, err
			}
			if membership == nil {
				return nil, fmt.Errorf("用户 %d 不是当前工作空间成员", input.SubjectID)
			}
		} else {
			group, err := h.groups.GetGroupByID(workspaceID, input.SubjectID)
			if err != nil {
				return nil, err
			}
			if group == nil {
				return nil, fmt.Errorf("用户组 %d 不属于当前工作空间", input.SubjectID)
			}
		}
		if input.Effect == "allow" && input.AccessLevel == authorization.AccessAdmin {
			hasAdmin = true
		}
		entries = append(entries, model.NodeACL{
			SubjectType: input.SubjectType, SubjectID: input.SubjectID, Effect: input.Effect,
			AccessLevel: input.AccessLevel, InheritToChildren: input.InheritToChildren,
		})
	}
	if !hasAdmin {
		return nil, errors.New("目录必须至少保留一名直接管理员")
	}
	return entries, nil
}
