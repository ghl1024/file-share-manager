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
	"file-share-manager/server/internal/pkg/ldapdn"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type groupMutationRequest struct {
	Name        string `json:"name" binding:"required,max=128"`
	Description string `json:"description" binding:"max=1000"`
}

type GroupHandler struct {
	groupDAO *dao.GroupDAO
	wsDAO    *dao.WorkspaceDAO
	acls     *dao.ACLDAO
}

type groupDirectoryItem struct {
	ID            uint      `json:"id"`
	Name          string    `json:"name"`
	Description   string    `json:"description"`
	Source        string    `json:"source"`
	DirectoryPath []string  `json:"directory_path"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func NewGroupHandler() *GroupHandler {
	return &GroupHandler{
		groupDAO: dao.NewGroupDAO(),
		wsDAO:    dao.NewWorkspaceDAO(),
		acls:     dao.NewACLDAO(),
	}
}

// CreateGroup 在工作空间内创建组
// @Summary Create Group
// @Description Handles POST /api/fileshare/v1/management/workspaces/{wid}/groups.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param wid path string true "wid"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups [post]
func (h *GroupHandler) CreateGroup(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间组")
		return
	}

	var req groupMutationRequest
	if !request.BindJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	if req.Name == "" {
		response.BadRequest(c, "用户组名称不能为空")
		return
	}
	existing, err := h.groupDAO.GetGroupByName(workspaceID, req.Name)
	if err != nil {
		response.InternalError(c, "查询用户组失败", err)
		return
	}
	if existing != nil {
		response.Conflict(c, "用户组名称已存在")
		return
	}
	actorID, _ := c.Get("user_id")

	group := &model.UserGroup{
		WorkspaceID: workspaceID,
		Name:        req.Name,
		Description: req.Description,
		Source:      "local",
		CreatedBy:   actorID.(uint),
	}

	if err := h.groupDAO.CreateGroupWithAudit(group,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "group:create", "user_group", "0", group.Name)); err != nil {
		logger.Error("create_user_group_failed", "error", err)
		response.InternalError(c, "创建用户组失败", err)
		return
	}

	response.SuccessWithMessage(c, "创建成功", gin.H{"id": group.ID, "name": group.Name})
}

// @Summary Update Group
// @Description Handles PUT /api/fileshare/v1/management/workspaces/{wid}/groups/{gid}.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param gid path string true "gid"
// @Param wid path string true "wid"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/{gid} [put]
func (h *GroupHandler) UpdateGroup(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupID, err := request.ParseUintParam(c, "gid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间组")
		return
	}
	group, err := h.groupDAO.GetGroupByID(workspaceID, groupID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	if group == nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	if dao.IsManagedGroupSource(group.Source) {
		response.Conflict(c, "LDAP 用户组由目录同步管理，不能手工编辑")
		return
	}
	var req groupMutationRequest
	if !request.BindJSON(c, &req) {
		return
	}
	req.Name, req.Description = strings.TrimSpace(req.Name), strings.TrimSpace(req.Description)
	if req.Name == "" {
		response.BadRequest(c, "用户组名称不能为空")
		return
	}
	if existing, lookupErr := h.groupDAO.GetGroupByName(workspaceID, req.Name); lookupErr != nil {
		response.InternalError(c, "查询用户组失败", lookupErr)
		return
	} else if existing != nil && existing.ID != groupID {
		response.Conflict(c, "用户组名称已存在")
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.groupDAO.UpdateLocalGroupWithAudit(workspaceID, groupID, req.Name, req.Description,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "group:update", "user_group", strconv.FormatUint(uint64(groupID), 10), group.Name)); err != nil {
		if errors.Is(err, dao.ErrManagedUserGroup) {
			response.Conflict(c, "LDAP 用户组由目录同步管理，不能手工编辑")
			return
		}
		response.InternalError(c, "更新用户组失败", err)
		return
	}
	response.SuccessWithMessage(c, "保存成功", nil)
}

// ListGroups 列出工作空间内的组
// @Summary List Groups
// @Description Handles GET /api/fileshare/v1/management/workspaces/{wid}/groups.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param wid path string true "wid"
// @Param keyword query string false "keyword"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups [get]
func (h *GroupHandler) ListGroups(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	// 只要是该空间的成员即可查询列表
	if !checkWorkspaceMember(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权访问该工作空间")
		return
	}

	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	groups, err := h.groupDAO.ListPageByWorkspace(workspaceID, page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "查询用户组失败", err)
		return
	}

	response.SuccessWithPage(c, groupDirectoryItems(groups.List), groups.Total, groups.Page, groups.PageSize)
}

// ListDirectory returns the complete group option set used by the ACL tree.
// Group DNs are reduced to organizational labels and are not exposed.
// @Summary List Directory
// @Description Handles GET /api/fileshare/v1/management/workspaces/{wid}/groups/directory.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param wid path string true "wid"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/directory [get]
func (h *GroupHandler) ListDirectory(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceMember(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权访问该工作空间")
		return
	}

	groups, err := h.groupDAO.ListGroupsByWorkspace(workspaceID)
	if err != nil {
		response.InternalError(c, "查询用户组目录失败", err)
		return
	}
	response.Success(c, groupDirectoryItems(groups))
}

func groupDirectoryItems(groups []model.UserGroup) []groupDirectoryItem {
	items := make([]groupDirectoryItem, 0, len(groups))
	for _, group := range groups {
		path := []string{}
		if strings.EqualFold(group.Source, "ldap") {
			path = ldapdn.OrganizationalPath(group.LDAPDN)
		}
		items = append(items, groupDirectoryItem{
			ID: group.ID, Name: group.Name, Description: group.Description,
			Source: group.Source, DirectoryPath: path,
			CreatedAt: group.CreatedAt, UpdatedAt: group.UpdatedAt,
		})
	}
	return items
}

// @Summary Delete Group
// @Description Handles DELETE /api/fileshare/v1/management/workspaces/{wid}/groups/{gid}.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param gid path string true "gid"
// @Param wid path string true "wid"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/{gid} [delete]
func (h *GroupHandler) DeleteGroup(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupID, err := request.ParseUintParam(c, "gid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理该工作空间组")
		return
	}
	group, err := h.groupDAO.GetGroupByID(workspaceID, groupID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	if group == nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	if dao.IsManagedGroupSource(group.Source) {
		response.Conflict(c, "LDAP 用户组由目录同步管理，不能手工删除")
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.groupDAO.DeleteGroupWithAudit(workspaceID, groupID,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "group:delete", "user_group", strconv.FormatUint(uint64(groupID), 10), group.Name)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "用户组不存在")
			return
		}
		if errors.Is(err, dao.ErrManagedUserGroup) {
			response.Conflict(c, "LDAP 用户组由目录同步管理，不能手工删除")
			return
		}
		if errors.Is(err, dao.ErrGroupIsSoleDirectoryAdmin) {
			response.Conflict(c, "该用户组是一个或多个目录的唯一管理员，请先调整目录权限")
			return
		}
		response.InternalError(c, "删除用户组失败", err)
		return
	}
	response.SuccessWithMessage(c, "删除成功", nil)
}

// @Summary List Members
// @Description Handles GET /api/fileshare/v1/management/workspaces/{wid}/groups/{gid}/members.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param gid path string true "gid"
// @Param wid path string true "wid"
// @Param keyword query string false "keyword"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/{gid}/members [get]
func (h *GroupHandler) ListMembers(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupID, err := request.ParseUintParam(c, "gid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !checkWorkspaceMember(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权访问该工作空间")
		return
	}
	group, err := h.groupDAO.GetGroupByID(workspaceID, groupID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	if group == nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	members, err := h.groupDAO.ListMembersPage(workspaceID, groupID, page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "读取用户组成员失败", err)
		return
	}
	response.SuccessWithPage(c, members.List, members.Total, members.Page, members.PageSize)
}

// @Summary Remove Member
// @Description Handles DELETE /api/fileshare/v1/management/workspaces/{wid}/groups/{gid}/members/{uid}.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param gid path string true "gid"
// @Param uid path string true "uid"
// @Param wid path string true "wid"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/{gid}/members/{uid} [delete]
func (h *GroupHandler) RemoveMember(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	groupID, err := request.ParseUintParam(c, "gid")
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
		response.Forbidden(c, "无权管理组内成员")
		return
	}
	group, err := h.groupDAO.GetGroupByID(workspaceID, groupID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	if group == nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	if dao.IsManagedGroupSource(group.Source) {
		response.Conflict(c, "LDAP 用户组成员由目录同步管理，不能手工移除")
		return
	}
	wasMember, err := h.groupDAO.IsUserInGroup(workspaceID, groupID, userID)
	if err != nil {
		response.InternalError(c, "检查用户组成员失败", err)
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.groupDAO.RemoveGroupMemberWithAudit(workspaceID, groupID, userID,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "group:member_remove", "user_group", strconv.FormatUint(uint64(groupID), 10), group.Name)); err != nil {
		if errors.Is(err, dao.ErrManagedUserGroup) {
			response.Conflict(c, "LDAP 用户组成员由目录同步管理，不能手工移除")
			return
		}
		response.InternalError(c, "移除用户组成员失败", err)
		return
	}
	if wasMember {
		h.publishGroupAccessNotification(c.Request.Context(), workspaceID, groupID, userID, group.Name, false)
	}
	response.SuccessWithMessage(c, "移除成功", nil)
}

// AddMemberToGroup 将用户添加到组内
// @Summary Add Member To Group
// @Description Handles POST /api/fileshare/v1/management/workspaces/{wid}/groups/{gid}/members.
// @Tags Workspaces
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param gid path string true "gid"
// @Param wid path string true "wid"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/workspaces/{wid}/groups/{gid}/members [post]
func (h *GroupHandler) AddMemberToGroup(c *gin.Context) {
	workspaceID, err := request.ParseUintParam(c, "wid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	groupID, err := request.ParseUintParam(c, "gid")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	if !checkWorkspaceAdmin(c, h.wsDAO, workspaceID) {
		response.Forbidden(c, "无权管理组内成员")
		return
	}
	group, err := h.groupDAO.GetGroupByID(workspaceID, groupID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	if group == nil {
		response.NotFound(c, "用户组不存在")
		return
	}
	if dao.IsManagedGroupSource(group.Source) {
		response.Conflict(c, "LDAP 用户组成员由目录同步管理，不能手工添加")
		return
	}

	var req struct {
		UserID uint `json:"user_id" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	membership, err := h.wsDAO.GetMembership(workspaceID, req.UserID)
	if err != nil {
		response.InternalError(c, "查询工作空间成员失败", err)
		return
	}
	if membership == nil {
		response.BadRequest(c, "只能将工作空间成员加入用户组")
		return
	}
	alreadyMember, err := h.groupDAO.IsUserInGroup(workspaceID, groupID, req.UserID)
	if err != nil {
		response.InternalError(c, "检查用户组成员失败", err)
		return
	}

	member := &model.UserGroupMember{
		GroupID: groupID,
		UserID:  req.UserID,
	}

	actorID, _ := c.Get("user_id")
	if err := h.groupDAO.AddGroupMemberWithAudit(member,
		newBusinessAuditEvent(c, actorID.(uint), &workspaceID, "group:member_add", "user_group", strconv.FormatUint(uint64(groupID), 10), group.Name)); err != nil {
		if errors.Is(err, dao.ErrManagedUserGroup) {
			response.Conflict(c, "LDAP 用户组成员由目录同步管理，不能手工添加")
			return
		}
		logger.Error("add_user_group_member_failed", "error", err)
		response.InternalError(c, "添加用户组成员失败", err)
		return
	}
	if !alreadyMember {
		h.publishGroupAccessNotification(c.Request.Context(), workspaceID, groupID, req.UserID, group.Name, true)
	}

	response.SuccessWithMessage(c, "成员添加成功", nil)
}

// checkWorkspaceMember 检查是否是工作空间成员（或超管）
func checkWorkspaceMember(c *gin.Context, wsDAO *dao.WorkspaceDAO, workspaceID uint) bool {
	isSuperAdmin, _ := c.Get("is_super_admin")
	if allowed, ok := isSuperAdmin.(bool); ok && allowed {
		recordWorkspaceAuthorization(c, true, "workspace:access", workspaceID)
		return true
	}
	userID, _ := c.Get("user_id")
	uid, ok := userID.(uint)
	if !ok {
		recordWorkspaceAuthorization(c, false, "workspace:access", workspaceID)
		return false
	}
	m, err := wsDAO.GetMembership(workspaceID, uid)
	if err != nil || m == nil {
		recordWorkspaceAuthorization(c, false, "workspace:access", workspaceID)
		return false
	}
	recordWorkspaceAuthorization(c, true, "workspace:access", workspaceID)
	return true
}
