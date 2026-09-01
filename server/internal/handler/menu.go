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
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var menuCodePattern = regexp.MustCompile(`^[a-z][a-z0-9:_-]{1,63}$`)

type MenuHandler struct {
	menus *dao.MenuDAO
}

func NewMenuHandler() *MenuHandler {
	return &MenuHandler{menus: dao.NewMenuDAO()}
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/system/menus.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/menus [get]
func (h *MenuHandler) List(c *gin.Context) {
	menus, err := h.menus.ListTree()
	if err != nil {
		response.InternalError(c, "查询菜单失败", err)
		return
	}
	response.Success(c, menus)
}

// @Summary Get
// @Description Handles GET /api/fileshare/v1/management/system/menus/{id}.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/menus/{id} [get]
func (h *MenuHandler) Get(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	menu, err := h.menus.Get(id)
	if err != nil {
		response.InternalError(c, "查询菜单失败", err)
		return
	}
	if menu == nil {
		response.NotFound(c, "菜单不存在")
		return
	}
	response.Success(c, menu)
}

type menuRequest struct {
	Code        string   `json:"code" binding:"required,max=64"`
	ParentID    uint     `json:"parent_id"`
	Name        string   `json:"name" binding:"required,max=64"`
	Path        string   `json:"path" binding:"max=255"`
	Component   string   `json:"component" binding:"max=255"`
	Type        int8     `json:"type" binding:"oneof=0 1 2"`
	Icon        string   `json:"icon" binding:"max=64"`
	SortOrder   int      `json:"sort_order"`
	Hidden      bool     `json:"hidden"`
	Status      int      `json:"status" binding:"oneof=0 1"`
	Permissions []string `json:"permissions" binding:"max=50,dive,max=96"`
}

// @Summary Create
// @Description Handles POST /api/fileshare/v1/management/system/menus.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/menus [post]
func (h *MenuHandler) Create(c *gin.Context) {
	var req menuRequest
	if !request.BindJSON(c, &req) {
		return
	}
	menu, ok := buildMenuFromRequest(req, true)
	if !ok {
		response.BadRequest(c, menuValidationMessage(req))
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.menus.CreateWithAudit(menu, req.Permissions,
		newBusinessAuditEvent(c, actorID.(uint), nil, "menu:create", "menu", "0", menu.Name)); err != nil {
		response.BadRequest(c, menuErrorMessage(err, "创建菜单失败"))
		return
	}
	response.SuccessWithMessage(c, "创建成功", menu)
}

// @Summary Update
// @Description Handles PUT /api/fileshare/v1/management/system/menus/{id}.
// @Tags System management
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
// @Router /management/system/menus/{id} [put]
func (h *MenuHandler) Update(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	var req menuRequest
	if !request.BindJSON(c, &req) {
		return
	}
	menu, ok := buildMenuFromRequest(req, false)
	if !ok {
		response.BadRequest(c, menuValidationMessage(req))
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.menus.UpdateWithAudit(id, *menu, req.Permissions,
		newBusinessAuditEvent(c, actorID.(uint), nil, "menu:update", "menu", "0", menu.Name)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "菜单不存在")
			return
		}
		response.BadRequest(c, menuErrorMessage(err, "更新菜单失败"))
		return
	}
	response.Success(c, gin.H{"id": id})
}

// @Summary Delete
// @Description Handles DELETE /api/fileshare/v1/management/system/menus/{id}.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/menus/{id} [delete]
func (h *MenuHandler) Delete(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	actorID, _ := c.Get("user_id")
	if err := h.menus.DeleteWithAudit(id,
		newBusinessAuditEvent(c, actorID.(uint), nil, "menu:delete", "menu", "0", "")); err != nil {
		switch {
		case errors.Is(err, dao.ErrMenuHasChildren):
			response.Conflict(c, "请先删除子菜单或按钮")
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "菜单不存在")
		default:
			response.InternalError(c, "删除菜单失败", err)
		}
		return
	}
	response.SuccessWithMessage(c, "删除成功", nil)
}

func buildMenuFromRequest(req menuRequest, includeCode bool) (*model.Menu, bool) {
	code := strings.ToLower(strings.TrimSpace(req.Code))
	if includeCode && !menuCodePattern.MatchString(code) {
		return nil, false
	}
	name := strings.TrimSpace(req.Name)
	if name == "" || name == "..." || name == "…" {
		return nil, false
	}
	if req.Status != 0 && req.Status != 1 {
		req.Status = 1
	}
	menu := &model.Menu{
		ParentID:  req.ParentID,
		Name:      name,
		Path:      strings.TrimSpace(req.Path),
		Component: strings.TrimSpace(req.Component),
		Type:      req.Type,
		Icon:      strings.TrimSpace(req.Icon),
		SortOrder: req.SortOrder,
		Hidden:    req.Hidden,
		Status:    req.Status,
	}
	if includeCode {
		menu.Code = code
	}
	return menu, true
}

func menuValidationMessage(req menuRequest) string {
	name := strings.TrimSpace(req.Name)
	if name == "" || name == "..." || name == "…" {
		return "菜单名称不能为空或使用占位符"
	}
	return "菜单编码格式不合法"
}

func menuErrorMessage(err error, fallback string) string {
	if err == nil {
		return fallback
	}
	message := err.Error()
	switch {
	case strings.Contains(message, "Duplicate"):
		return "菜单编码已存在"
	case strings.Contains(message, "不存在的权限点"):
		return "菜单关联了不存在的权限点"
	case strings.Contains(message, "父级菜单"):
		return message
	default:
		return fallback
	}
}
