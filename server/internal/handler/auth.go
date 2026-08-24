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
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/jwt"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/pkg/security"
	ldapservice "file-share-manager/server/internal/service/ldap"

	"github.com/gin-gonic/gin"
)

type AuthHandler struct {
	userDAO       *dao.UserDAO
	workspaceDAO  *dao.WorkspaceDAO
	permissionDAO *dao.PermissionDAO
	menuDAO       *dao.MenuDAO
	ldapConfigDAO *dao.LDAPConfigDAO
	ldap          *ldapservice.Service
}

func NewAuthHandler() *AuthHandler {
	return &AuthHandler{
		userDAO:       dao.NewUserDAO(),
		workspaceDAO:  dao.NewWorkspaceDAO(),
		permissionDAO: dao.NewPermissionDAO(),
		menuDAO:       dao.NewMenuDAO(),
		ldapConfigDAO: dao.NewLDAPConfigDAO(),
		ldap:          ldapservice.NewService(),
	}
}

// ListUsers returns the standard paginated envelope. Workspace-scoped roles
// are managed separately after a workspace is selected.
func (h *AuthHandler) ListUsers(c *gin.Context) {
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	if keyword == "" {
		keyword = strings.TrimSpace(c.Query("username"))
	}
	users, err := h.userDAO.ListPage(page, pageSize, keyword)
	if err != nil {
		response.InternalError(c, "查询用户失败", err)
		return
	}

	type userListItem struct {
		model.User
		WorkspaceMember bool                  `json:"workspace_member"`
		Roles           []dao.UserRoleSummary `json:"roles"`
	}
	items := make([]userListItem, 0, len(users.List))
	memberMap := map[uint]bool{}
	roleMap := map[uint][]dao.UserRoleSummary{}
	if value, exists := c.Get("workspace_id"); exists {
		if workspaceID, valid := value.(uint); valid && workspaceID > 0 {
			userIDs := make([]uint, 0, len(users.List))
			for _, user := range users.List {
				userIDs = append(userIDs, user.ID)
			}
			memberMap, roleMap, err = h.userDAO.WorkspaceMetadata(workspaceID, userIDs)
			if err != nil {
				response.InternalError(c, "查询用户工作空间角色失败", err)
				return
			}
		}
	}
	for _, user := range users.List {
		roles := roleMap[user.ID]
		if roles == nil {
			roles = []dao.UserRoleSummary{}
		}
		items = append(items, userListItem{User: user, WorkspaceMember: memberMap[user.ID], Roles: roles})
	}
	response.SuccessWithPage(c, items, users.Total, users.Page, users.PageSize)
}

func (h *AuthHandler) GetUser(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	user, err := h.userDAO.GetByID(id)
	if err != nil {
		response.InternalError(c, "查询用户失败", err)
		return
	}
	if user == nil {
		response.NotFound(c, "用户不存在")
		return
	}
	response.Success(c, user)
}

// CreateUser is an administrator-only account creation endpoint.
func (h *AuthHandler) CreateUser(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required,min=3,max=64"`
		Password string `json:"password" binding:"required"`
		RealName string `json:"real_name" binding:"required,max=64"`
		Email    string `json:"email" binding:"omitempty,email"`
		Phone    string `json:"phone" binding:"omitempty"`
	}

	if !request.BindJSON(c, &req) {
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	req.RealName = strings.TrimSpace(req.RealName)

	if err := security.ValidatePassword(req.Password); err != nil {
		response.BadRequest(c, err.Error())
		return
	}

	existingUser, err := h.userDAO.GetByUsername(req.Username)
	if err != nil {
		logger.Error("Failed to query user", "error", err)
		response.InternalError(c, "Database error", err)
		return
	}
	if existingUser != nil {
		response.Conflict(c, "用户名已存在")
		return
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		logger.Error("Failed to hash password", "error", err)
		response.InternalError(c, "Security error", err)
		return
	}

	user := &model.User{
		Username:     req.Username,
		PasswordHash: hash,
		RealName:     req.RealName,
		Email:        req.Email,
		Phone:        req.Phone,
		Status:       1, // 默认启用
		Source:       "local",
	}

	actorID, _ := c.Get("user_id")
	if err := h.userDAO.CreateWithAudit(user, newBusinessAuditEvent(c, actorID.(uint), nil, "user:create", "user", "0", user.Username)); err != nil {
		logger.Error("Failed to create user", "error", err)
		response.InternalError(c, "Failed to create user", err)
		return
	}

	response.Success(c, gin.H{"id": user.ID, "username": user.Username})
}

func (h *AuthHandler) UpdateUser(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req struct {
		RealName string `json:"real_name" binding:"required,max=64"`
		Email    string `json:"email" binding:"omitempty,email"`
		Phone    string `json:"phone" binding:"omitempty,max=32"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	target, err := h.userDAO.GetByID(id)
	if err != nil {
		response.InternalError(c, "查询用户失败", err)
		return
	}
	if target == nil {
		response.NotFound(c, "用户不存在")
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.userDAO.UpdateFieldsWithAudit(id, map[string]any{
		"real_name": strings.TrimSpace(req.RealName), "email": strings.TrimSpace(req.Email), "phone": strings.TrimSpace(req.Phone),
	}, newBusinessAuditEvent(c, actorID.(uint), nil, "user:update", "user", fmt.Sprint(id), target.Username)); err != nil {
		response.InternalError(c, "更新用户失败", err)
		return
	}
	response.SuccessWithMessage(c, "更新成功", nil)
}

func (h *AuthHandler) UpdateUserStatus(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req struct {
		Status int `json:"status"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.Status != 0 && req.Status != 1 {
		response.BadRequest(c, "status 只能是 0 或 1")
		return
	}
	target, err := h.userDAO.GetByID(id)
	if err != nil || target == nil {
		response.NotFound(c, "用户不存在")
		return
	}
	if target.IsSuperAdmin {
		response.Forbidden(c, "超级管理员状态不能通过此接口修改")
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.userDAO.UpdateStatusWithAudit(id, req.Status,
		newBusinessAuditEvent(c, actorID.(uint), nil, "user:status_update", "user", fmt.Sprint(id), target.Username)); err != nil {
		response.InternalError(c, "更新状态失败", err)
		return
	}
	response.SuccessWithMessage(c, "更新成功", nil)
}

func (h *AuthHandler) DeleteUser(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	target, err := h.userDAO.GetByID(id)
	if err != nil || target == nil {
		response.NotFound(c, "用户不存在")
		return
	}
	if target.IsSuperAdmin {
		response.Forbidden(c, "超级管理员不能通过此接口删除")
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.userDAO.DeleteWithAudit(id,
		newBusinessAuditEvent(c, actorID.(uint), nil, "user:delete", "user", fmt.Sprint(id), target.Username)); err != nil {
		response.InternalError(c, "删除用户失败", err)
		return
	}
	response.SuccessWithMessage(c, "删除成功", nil)
}

func (h *AuthHandler) ResetUserPassword(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if err := security.ValidatePassword(req.Password); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	target, err := h.userDAO.GetByID(id)
	if err != nil || target == nil {
		response.NotFound(c, "用户不存在")
		return
	}
	if target.IsSuperAdmin {
		actor, _ := c.Get("user_id")
		if actor.(uint) != target.ID {
			response.Forbidden(c, "超级管理员密码只能由本人或运维命令修改")
			return
		}
	}
	actorID, _ := c.Get("user_id")
	if err := h.userDAO.UpdatePasswordWithAudit(id, req.Password,
		newBusinessAuditEvent(c, actorID.(uint), nil, "user:password_reset", "user", fmt.Sprint(id), target.Username)); err != nil {
		response.InternalError(c, "重置密码失败", err)
		return
	}
	response.SuccessWithMessage(c, "重置成功", nil)
}

// Login 用户登录
func (h *AuthHandler) Login(c *gin.Context) {
	var req struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}

	if !request.BindJSON(c, &req) {
		return
	}

	user, err := h.userDAO.GetByUsername(strings.TrimSpace(req.Username))
	if err != nil {
		logger.Error("Failed to query user", "error", err)
		response.InternalError(c, "Database error", err)
		return
	}
	ldapAuthenticated := false
	if user == nil || user.Source == "ldap" {
		ldapCfg, ldapCfgErr := h.ldapConfigDAO.RuntimeConfig()
		if ldapCfgErr != nil {
			response.InternalError(c, "查询 LDAP 配置失败", ldapCfgErr)
			return
		}
		if ldapCfg.Enabled() {
			identity, ldapErr := h.ldap.Authenticate(c.Request.Context(), ldapCfg, req.Username, req.Password)
			if ldapErr == nil {
				ldapAuthenticated = true
				if user == nil {
					hash, hashErr := ldapservice.NewLDAPPasswordHash()
					if hashErr != nil {
						response.InternalError(c, "创建 LDAP 用户失败", hashErr)
						return
					}
					user = &model.User{Username: identity.Username, PasswordHash: hash, RealName: identity.RealName, Email: identity.Email, Source: "ldap", Status: 1}
					if err := h.userDAO.Create(user); err != nil {
						response.InternalError(c, "创建 LDAP 用户失败", err)
						return
					}
				} else if err := h.userDAO.UpdateFields(user.ID, map[string]any{"real_name": identity.RealName, "email": identity.Email}); err != nil {
					response.InternalError(c, "同步 LDAP 用户资料失败", err)
					return
				}
			} else if user == nil {
				response.Unauthorized(c, "用户名或密码错误")
				return
			}
		}
	}
	if user == nil || user.Status != 1 || (user.Source == "ldap" && !ldapAuthenticated) || (user.Source != "ldap" && !security.CheckPasswordHash(req.Password, user.PasswordHash)) {
		response.Unauthorized(c, "用户名或密码错误，或账号已被禁用")
		return
	}

	session, err := h.sessionPayload(user, nil)
	if err != nil {
		logger.Error("Failed to load user session", "error", err)
		response.InternalError(c, "Failed to load user session", err)
		return
	}

	token, expiresAt, err := jwt.GenerateToken(user.ID, user.Username, user.IsSuperAdmin, nil, user.AuthVersion)
	if err != nil {
		logger.Error("Failed to generate token", "error", err)
		response.InternalError(c, "Failed to generate token", err)
		return
	}

	// 登录态统一由 HttpOnly Cookie 承载。
	jwt.SetSessionCookie(c, token, int(expiresAt.Sub(jwt.Now()).Seconds()))

	session["expires_at"] = expiresAt
	response.Success(c, session)
}

// Logout 注销登录
func (h *AuthHandler) Logout(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "未登录")
		return
	}
	if err := h.userDAO.IncrementAuthVersion(userID.(uint)); err != nil {
		response.InternalError(c, "注销失败", err)
		return
	}
	jwt.ClearSessionCookie(c)
	response.Success(c, "注销成功")
}

// Profile 获取当前登录用户信息
func (h *AuthHandler) Profile(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	workspaces, err := h.userDAO.ListProfileWorkspaces(user.ID, user.IsSuperAdmin)
	if err != nil {
		response.InternalError(c, "查询个人工作空间失败", err)
		return
	}
	payload := gin.H{
		"id":             user.ID,
		"username":       user.Username,
		"real_name":      user.RealName,
		"email":          user.Email,
		"phone":          user.Phone,
		"source":         user.Source,
		"status":         user.Status,
		"is_super_admin": user.IsSuperAdmin,
		"created_at":     user.CreatedAt,
		"updated_at":     user.UpdatedAt,
		"workspaces":     workspaces,
	}
	if expiresAt, exists := c.Get("session_expires_at"); exists {
		payload["session_expires_at"] = expiresAt
	}
	response.Success(c, payload)
}

func (h *AuthHandler) UpdateProfile(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	if user.Source == "ldap" {
		response.Conflict(c, "LDAP 账号资料由目录服务统一维护，不能在此修改")
		return
	}
	var req struct {
		RealName string `json:"real_name" binding:"required,max=64"`
		Email    string `json:"email" binding:"omitempty,email,max=128"`
		Phone    string `json:"phone" binding:"omitempty,max=32"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	realName := strings.TrimSpace(req.RealName)
	if realName == "" {
		response.BadRequest(c, "姓名不能为空")
		return
	}
	actorID := user.ID
	if err := h.userDAO.UpdateFieldsWithAudit(user.ID, map[string]any{
		"real_name": realName,
		"email":     strings.TrimSpace(req.Email),
		"phone":     strings.TrimSpace(req.Phone),
	}, newBusinessAuditEvent(c, actorID, nil, "user:profile_update", "user", fmt.Sprint(user.ID), user.Username)); err != nil {
		response.InternalError(c, "更新个人资料失败", err)
		return
	}
	updated, err := h.userDAO.GetByID(user.ID)
	if err != nil || updated == nil {
		response.InternalError(c, "读取更新后的个人资料失败", err)
		return
	}
	response.SuccessWithMessage(c, "个人资料已更新", gin.H{
		"id": updated.ID, "username": updated.Username, "real_name": updated.RealName,
		"email": updated.Email, "phone": updated.Phone, "source": updated.Source,
		"is_super_admin": updated.IsSuperAdmin,
	})
}

func (h *AuthHandler) ChangePassword(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	if user.Source == "ldap" {
		response.Conflict(c, "LDAP 账号密码由目录服务统一维护，不能在此修改")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password" binding:"required,max=128"`
		NewPassword     string `json:"new_password" binding:"required,max=128"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if err := security.ValidatePassword(req.NewPassword); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	updated, err := h.userDAO.ChangeOwnPasswordWithAudit(user.ID, req.CurrentPassword, req.NewPassword,
		newBusinessAuditEvent(c, user.ID, nil, "user:password_change", "user", fmt.Sprint(user.ID), user.Username))
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrInvalidCurrentPassword):
			response.BadRequest(c, "当前密码错误")
		case errors.Is(err, dao.ErrPasswordUnchanged):
			response.Conflict(c, "新密码不能与当前密码相同")
		case errors.Is(err, dao.ErrLDAPAccountReadOnly):
			response.Conflict(c, "LDAP 账号密码由目录服务统一维护")
		default:
			response.InternalError(c, "修改密码失败", err)
		}
		return
	}

	access := sessionWorkspaceAccess(c)
	token, expiresAt, err := jwt.GenerateTokenWithWorkspaceAccess(updated.ID, updated.Username, updated.IsSuperAdmin, access, updated.AuthVersion)
	if err != nil {
		response.InternalError(c, "刷新当前登录状态失败", err)
		return
	}
	jwt.SetSessionCookie(c, token, int(expiresAt.Sub(jwt.Now()).Seconds()))
	session, err := h.sessionPayload(updated, access.WorkspaceID)
	if err != nil {
		response.InternalError(c, "刷新当前登录状态失败", err)
		return
	}
	session["expires_at"] = expiresAt
	session["current_workspace_cross_access"] = access.CrossWorkspaceAccess
	if access.CrossWorkspaceAccess {
		session["source_workspace_id"] = access.SourceWorkspaceID
		session["cross_workspace_reason"] = access.CrossWorkspaceReason
	}
	response.SuccessWithMessage(c, "密码已修改，其他登录会话已失效", session)
}

func (h *AuthHandler) Session(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	var workspaceID *uint
	if value, exists := c.Get("workspace_id"); exists {
		if selected, valid := value.(uint); valid {
			workspaceID = &selected
		}
	}
	session, err := h.sessionPayload(user, workspaceID)
	if err != nil {
		response.InternalError(c, "Failed to load user session", err)
		return
	}
	if expiresAt, exists := c.Get("session_expires_at"); exists {
		session["expires_at"] = expiresAt
	}
	crossWorkspaceAccess, _ := c.Get("cross_workspace_access")
	if crossWorkspaceAccess == true {
		session["current_workspace_cross_access"] = true
		if sourceWorkspaceID, exists := c.Get("source_workspace_id"); exists {
			session["source_workspace_id"] = sourceWorkspaceID
		}
		if reason, exists := c.Get("cross_workspace_reason"); exists {
			session["cross_workspace_reason"] = strings.TrimSpace(fmt.Sprint(reason))
		}
	}
	response.Success(c, session)
}

func (h *AuthHandler) Workspaces(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	workspaces, err := h.workspaceDAO.ListWorkspacesForUser(user.ID, user.IsSuperAdmin)
	if err != nil {
		response.InternalError(c, "Failed to load workspaces", err)
		return
	}
	response.Success(c, workspaces)
}

func (h *AuthHandler) SwitchWorkspace(c *gin.Context) {
	user, ok := h.currentUser(c)
	if !ok {
		return
	}
	var req struct {
		WorkspaceID uint   `json:"workspace_id" binding:"required"`
		Reason      string `json:"reason" binding:"max=500"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	workspace, err := h.workspaceDAO.GetByID(req.WorkspaceID)
	if err != nil {
		response.InternalError(c, "Failed to load workspace", err)
		return
	}
	if workspace == nil || workspace.Status != 1 {
		recordWorkspaceAuthorization(c, false, "workspace:switch", req.WorkspaceID)
		response.NotFound(c, "工作空间不存在或已停用")
		return
	}
	membership, err := h.workspaceDAO.GetMembership(req.WorkspaceID, user.ID)
	if err != nil {
		response.InternalError(c, "Failed to verify workspace membership", err)
		return
	}
	if !user.IsSuperAdmin && membership == nil {
		recordWorkspaceAuthorization(c, false, "workspace:switch", req.WorkspaceID)
		response.Forbidden(c, "无权访问该工作空间")
		return
	}
	reason := strings.TrimSpace(req.Reason)
	crossWorkspaceAccess := user.IsSuperAdmin && membership == nil
	if crossWorkspaceAccess && len([]rune(reason)) < 5 {
		response.BadRequest(c, "跨空间访问原因至少需要 5 个字符")
		return
	}
	recordWorkspaceAuthorization(c, true, "workspace:switch", req.WorkspaceID)

	var sourceWorkspaceID *uint
	if crossWorkspaceAccess {
		if value, exists := c.Get("workspace_id"); exists {
			if source, valid := value.(uint); valid && source > 0 && source != req.WorkspaceID {
				sourceWorkspaceID = &source
			}
		}
	}
	token, expiresAt, err := jwt.GenerateTokenWithWorkspaceAccess(user.ID, user.Username, user.IsSuperAdmin, jwt.WorkspaceAccess{
		WorkspaceID: &req.WorkspaceID, SourceWorkspaceID: sourceWorkspaceID,
		CrossWorkspaceAccess: crossWorkspaceAccess, CrossWorkspaceReason: reason,
	}, user.AuthVersion)
	if err != nil {
		response.InternalError(c, "Failed to switch workspace", err)
		return
	}
	jwt.SetSessionCookie(c, token, int(expiresAt.Sub(jwt.Now()).Seconds()))
	session, err := h.sessionPayload(user, &req.WorkspaceID)
	if err != nil {
		response.InternalError(c, "Failed to load user session", err)
		return
	}
	session["expires_at"] = expiresAt
	session["current_workspace_cross_access"] = crossWorkspaceAccess
	if crossWorkspaceAccess {
		session["source_workspace_id"] = sourceWorkspaceID
		session["cross_workspace_reason"] = reason
	}
	response.Success(c, session)
}

func (h *AuthHandler) currentUser(c *gin.Context) (*model.User, bool) {
	userID, exists := c.Get("user_id")
	if !exists {
		response.Unauthorized(c, "未登录")
		return nil, false
	}
	user, err := h.userDAO.GetByID(userID.(uint))
	if err != nil || user == nil {
		response.Unauthorized(c, "用户不存在")
		return nil, false
	}
	return user, true
}

func (h *AuthHandler) sessionPayload(user *model.User, requestedWorkspaceID *uint) (gin.H, error) {
	workspaces, err := h.workspaceDAO.ListWorkspacesForUser(user.ID, user.IsSuperAdmin)
	if err != nil {
		return nil, err
	}

	var currentWorkspaceID *uint
	if requestedWorkspaceID != nil {
		for _, workspace := range workspaces {
			if workspace.ID == *requestedWorkspaceID {
				selected := workspace.ID
				currentWorkspaceID = &selected
				break
			}
		}
	}

	permissions := make([]string, 0)
	if user.IsSuperAdmin {
		permissions = []string{"*"}
	} else if currentWorkspaceID != nil {
		membership, err := h.workspaceDAO.GetMembership(*currentWorkspaceID, user.ID)
		if err != nil {
			return nil, err
		}
		if membership != nil && membership.Role == "workspace_admin" {
			permissions = make([]string, 0, len(dao.BuiltinPermissions))
			for _, definition := range dao.BuiltinPermissions {
				permissions = append(permissions, definition.Code)
			}
		} else {
			permissions, err = h.permissionDAO.ListUserPermissionCodes(*currentWorkspaceID, user.ID)
			if err != nil {
				return nil, err
			}
		}
	}

	permissionSet := make(map[string]bool, len(permissions))
	for _, permission := range permissions {
		permissionSet[permission] = true
	}
	dbMenus, err := h.menuDAO.ListTreeForAccess(user.IsSuperAdmin, currentWorkspaceID != nil, permissionSet)
	if err != nil {
		return nil, err
	}
	var menus any = dbMenus
	if len(dbMenus) == 0 {
		menus = buildSessionMenus(user.IsSuperAdmin, currentWorkspaceID != nil, permissionSet)
	}
	return gin.H{
		"user": gin.H{
			"id":             user.ID,
			"username":       user.Username,
			"real_name":      user.RealName,
			"email":          user.Email,
			"phone":          user.Phone,
			"source":         user.Source,
			"is_super_admin": user.IsSuperAdmin,
		},
		"menus":                menus,
		"permissions":          permissions,
		"workspaces":           workspaces,
		"current_workspace_id": currentWorkspaceID,
	}, nil
}

func sessionWorkspaceAccess(c *gin.Context) jwt.WorkspaceAccess {
	access := jwt.WorkspaceAccess{}
	if value, exists := c.Get("workspace_id"); exists {
		if workspaceID, valid := value.(uint); valid && workspaceID > 0 {
			access.WorkspaceID = &workspaceID
		}
	}
	if value, exists := c.Get("source_workspace_id"); exists {
		if workspaceID, valid := value.(uint); valid && workspaceID > 0 {
			access.SourceWorkspaceID = &workspaceID
		}
	}
	if value, exists := c.Get("cross_workspace_access"); exists {
		access.CrossWorkspaceAccess, _ = value.(bool)
	}
	access.CrossWorkspaceReason = strings.TrimSpace(stringValueFromContext(c, "cross_workspace_reason"))
	return access
}

func buildSessionMenus(isSuperAdmin, hasWorkspace bool, permissionSet map[string]bool) []gin.H {
	menus := []gin.H{
		{"id": "dashboard", "name": "工作台", "path": "/dashboard", "icon": "DataBoard", "type": 1},
		{"id": "workspaces", "name": "工作空间", "path": "/workspaces", "icon": "FolderOpened", "type": 1},
	}
	systemChildren := make([]gin.H, 0, 6)
	if isSuperAdmin {
		systemChildren = append(systemChildren,
			gin.H{"id": "users", "name": "用户管理", "path": "/system/user", "icon": "User", "type": 1},
			gin.H{"id": "system-menu", "name": "菜单权限", "path": "/system/menu", "icon": "Menu", "type": 1},
			gin.H{"id": "system-ldap", "name": "LDAP", "path": "/system/ldap", "icon": "Connection", "type": 1},
			gin.H{"id": "system-clamav", "name": "ClamAV", "path": "/system/clamav", "icon": "Monitor", "type": 1},
			gin.H{"id": "system-backup-storage", "name": "备份存储", "path": "/system/backup-storage", "icon": "Collection", "type": 1},
		)
	}
	if hasWorkspace {
		if isSuperAdmin || permissionSet["file:list"] {
			menus = append(menus, gin.H{"id": "files", "name": "文件目录", "path": "/files", "icon": "Folder", "type": 1})
		}
		if isSuperAdmin || permissionSet["file:share:create"] {
			menus = append(menus, gin.H{"id": "shares", "name": "外链分享", "path": "/shares", "icon": "Share", "type": 1})
		}
		if isSuperAdmin || permissionSet["workspace:role:manage"] {
			systemChildren = append(systemChildren, gin.H{"id": "roles", "name": "角色管理", "path": "/system/role", "icon": "Lock", "type": 1})
		}
		if isSuperAdmin || permissionSet["audit:list"] {
			menus = append(menus, gin.H{"id": "audit", "name": "审计中心", "path": "/audit/history", "icon": "Document", "type": 1})
		}
		if isSuperAdmin || permissionSet["backup:manage"] {
			systemChildren = append(systemChildren, gin.H{"id": "backups", "name": "备份恢复", "path": "/system/backups", "icon": "Collection", "type": 1})
		}
	}
	if len(systemChildren) > 0 {
		menus = append(menus, gin.H{"id": "system", "name": "系统管理", "path": "/system", "icon": "Setting", "type": 1, "children": systemChildren})
	}
	return menus
}
