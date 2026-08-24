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
	"unicode"
	"unicode/utf8"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NodeHandler struct {
	nodeDAO          *dao.NodeDAO
	fileDAO          *dao.FileDAO
	favoriteDAO      *dao.FavoriteDAO
	collaborationDAO *dao.CollaborationDAO
	authz            *authorization.Service
}

func NewNodeHandler() *NodeHandler {
	return &NodeHandler{
		nodeDAO:          dao.NewNodeDAO(),
		fileDAO:          dao.NewFileDAO(),
		favoriteDAO:      dao.NewFavoriteDAO(),
		collaborationDAO: dao.NewCollaborationDAO(),
		authz:            authorization.NewService(),
	}
}

func (h *NodeHandler) CreateFolder(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Name     string `json:"name" binding:"required,max=255"`
		ParentID *uint  `json:"parent_id"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	displayName, normalizedName, valid := normalizeNodeName(req.Name)
	if !valid {
		response.BadRequest(c, "目录名称不合法")
		return
	}
	if req.ParentID == nil {
		allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
		recordDataAuthorization(c, allowed, "node:create_root", "workspace", actor.WorkspaceID)
		if !allowed {
			response.Forbidden(c, "只有工作空间管理员可以创建一级目录")
			return
		}
	} else {
		parent, err := h.nodeDAO.GetByID(actor.WorkspaceID, *req.ParentID)
		if err != nil {
			response.InternalError(c, "读取父目录失败", err)
			return
		}
		if parent == nil || parent.Type != "folder" || parent.Status != "active" {
			response.NotFound(c, "父目录不存在")
			return
		}
		allowed, err := h.authz.CanWrite(actor, parent.ID)
		if err != nil {
			h.handleAuthorizationError(c, err)
			return
		}
		recordDataAuthorization(c, allowed, "node:write", "folder", parent.ID)
		if !allowed {
			response.Forbidden(c, "无权在该目录下创建内容")
			return
		}
	}
	exists, err := h.nodeDAO.NameExists(actor.WorkspaceID, req.ParentID, normalizedName, nil)
	if err != nil {
		response.InternalError(c, "检查目录名称失败", err)
		return
	}
	if exists {
		response.Conflict(c, "同级目录中已存在同名项目")
		return
	}

	node := &model.Node{
		WorkspaceID: actor.WorkspaceID, ParentID: req.ParentID, Name: displayName,
		NormalizedName: normalizedName, Type: "folder", InheritMode: "inherit",
		Status: "active", CreatedBy: actor.UserID, UpdatedBy: actor.UserID,
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodeDAO.CreateNodeWithAudit(node, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:create", "node", "0", node.Name)); err != nil {
		logger.Error("create_folder_failed", "error", err, "workspace_id", actor.WorkspaceID)
		response.InternalError(c, "创建目录失败", err)
		return
	}
	response.Success(c, node)
}

func (h *NodeHandler) ListRoots(c *gin.Context) {
	h.listChildren(c, nil)
}

func (h *NodeHandler) ListChildren(c *gin.Context) {
	parentID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	h.listChildren(c, &parentID)
}

func (h *NodeHandler) Rename(c *gin.Context) {
	actor, node, ok := h.authorizeWrite(c, "active")
	if !ok {
		return
	}
	var req struct {
		Name string `json:"name" binding:"required,max=255"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	name, normalizedName, valid := normalizeNodeName(req.Name)
	if !valid {
		response.BadRequest(c, "名称不合法")
		return
	}
	exists, err := h.nodeDAO.NameExists(actor.WorkspaceID, node.ParentID, normalizedName, &node.ID)
	if err != nil {
		response.InternalError(c, "检查同名项目失败", err)
		return
	}
	if exists {
		response.Conflict(c, "同级目录中已存在同名项目")
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodeDAO.RenameWithAudit(actor.WorkspaceID, node.ID, actor.UserID, name, normalizedName, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:rename", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name)); err != nil {
		response.InternalError(c, "重命名失败", err)
		return
	}
	response.Success(c, gin.H{"id": node.ID, "name": name})
}

func (h *NodeHandler) Move(c *gin.Context) {
	actor, node, ok := h.authorizeWrite(c, "active")
	if !ok {
		return
	}
	var req struct {
		ParentID *uint `json:"parent_id"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.ParentID == nil {
		allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
		recordDataAuthorization(c, allowed, "node:move_to_root", "workspace", actor.WorkspaceID)
		if !allowed {
			response.Forbidden(c, "只有工作空间管理员可以移动到根目录")
			return
		}
	} else {
		parent, err := h.nodeDAO.GetByID(actor.WorkspaceID, *req.ParentID)
		if err != nil {
			response.InternalError(c, "读取目标目录失败", err)
			return
		}
		if parent == nil || parent.Type != "folder" || parent.Status != "active" {
			response.NotFound(c, "目标目录不存在")
			return
		}
		allowed, err := h.authz.CanWrite(actor, parent.ID)
		if err != nil {
			h.handleAuthorizationError(c, err)
			return
		}
		recordDataAuthorization(c, allowed, "node:write", "folder", parent.ID)
		if !allowed {
			response.Forbidden(c, "无权移动到目标目录")
			return
		}
	}
	exists, err := h.nodeDAO.NameExists(actor.WorkspaceID, req.ParentID, node.NormalizedName, &node.ID)
	if err != nil {
		response.InternalError(c, "检查目标目录名称失败", err)
		return
	}
	if exists {
		response.Conflict(c, "目标目录中已存在同名项目")
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodeDAO.MoveWithAudit(actor.WorkspaceID, node.ID, actor.UserID, req.ParentID, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:move", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name)); err != nil {
		switch {
		case errors.Is(err, dao.ErrInvalidMove):
			response.BadRequest(c, "目录不能移动到自身或其后代目录")
		case errors.Is(err, dao.ErrNodeState):
			response.Conflict(c, "节点状态不允许移动")
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "节点或目标目录不存在")
		default:
			response.InternalError(c, "移动节点失败", err)
		}
		return
	}
	response.Success(c, gin.H{"id": node.ID, "parent_id": req.ParentID})
}

func (h *NodeHandler) Trash(c *gin.Context) {
	actor, node, ok := h.authorizeWrite(c, "active")
	if !ok {
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodeDAO.TrashSubtreeWithAudit(actor.WorkspaceID, node.ID, actor.UserID, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:trash", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name)); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "节点不存在")
			return
		}
		response.InternalError(c, "移入回收站失败", err)
		return
	}
	response.Success(c, gin.H{"id": node.ID, "status": "trashed"})
}

func (h *NodeHandler) Restore(c *gin.Context) {
	actor, node, ok := h.authorizeWrite(c, "trashed")
	if !ok {
		return
	}
	workspaceID := actor.WorkspaceID
	if err := h.nodeDAO.RestoreSubtreeWithAudit(actor.WorkspaceID, node.ID, actor.UserID, newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:restore", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name)); err != nil {
		switch {
		case errors.Is(err, gorm.ErrDuplicatedKey):
			response.Conflict(c, "原目录中已存在同名项目，请先重命名冲突项目")
		case errors.Is(err, dao.ErrNodeState):
			response.Conflict(c, "父目录尚未恢复或节点状态不允许恢复")
		case errors.Is(err, gorm.ErrRecordNotFound):
			response.NotFound(c, "节点或原父目录不存在")
		default:
			response.InternalError(c, "恢复节点失败", err)
		}
		return
	}
	response.Success(c, gin.H{"id": node.ID, "status": "active"})
}

func (h *NodeHandler) ListTrash(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	nodes, err := h.nodeDAO.ListTrashRoots(actor.WorkspaceID)
	if err != nil {
		response.InternalError(c, "读取回收站失败", err)
		return
	}
	visible := make([]model.Node, 0, len(nodes))
	for i := range nodes {
		allowed, authErr := h.authz.CanRestore(actor, nodes[i].ID)
		if authErr != nil {
			h.handleAuthorizationError(c, authErr)
			return
		}
		if allowed {
			visible = append(visible, nodes[i])
		}
	}
	if err := h.decorateStorage(actor.WorkspaceID, visible); err != nil {
		response.InternalError(c, "读取文件存储状态失败", err)
		return
	}
	h.respondPage(c, visible)
}

func (h *NodeHandler) Detail(c *gin.Context) {
	actor, node, ok := h.authorizeRead(c)
	if !ok {
		return
	}
	result := gin.H{"node": node}
	access, err := h.authz.AccessSummary(actor, node.ID)
	if err != nil {
		response.InternalError(c, "读取权限来源失败", err)
		return
	}
	result["access"] = access
	ancestors, err := h.nodeDAO.ListAncestors(actor.WorkspaceID, node.ID)
	if err != nil {
		response.InternalError(c, "读取文件位置失败", err)
		return
	}
	if completeAncestorPath(node, ancestors) {
		ancestorIDs := make([]uint, 0, len(ancestors))
		for _, ancestor := range ancestors {
			ancestorIDs = append(ancestorIDs, ancestor.ID)
		}
		readable, readErr := h.authz.ReadableNodeIDs(actor, ancestorIDs)
		if readErr != nil {
			response.InternalError(c, "校验文件位置权限失败", readErr)
			return
		}
		breadcrumbs := make([]gin.H, 0, len(ancestors))
		locationVisible := true
		for _, ancestor := range ancestors {
			if ancestor.Type != "folder" || !readable[ancestor.ID] {
				locationVisible = false
				break
			}
			breadcrumbs = append(breadcrumbs, gin.H{"id": ancestor.ID, "name": ancestor.Name})
		}
		if locationVisible {
			result["location"] = gin.H{"node_id": node.ID, "node_name": node.Name, "node_type": node.Type, "breadcrumbs": breadcrumbs}
		}
	}
	if node.Type == "file" {
		versions, err := h.fileDAO.ListVersions(actor.WorkspaceID, node.ID)
		if err != nil {
			response.InternalError(c, "读取文件版本失败", err)
			return
		}
		result["versions"] = versions
	}
	rememberRecentNode(h.collaborationDAO, actor, node.ID)
	response.Success(c, result)
}

func (h *NodeHandler) Search(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	filter, validationMessage := parseNodeSearchFilter(c)
	if validationMessage != "" {
		response.BadRequest(c, validationMessage)
		return
	}
	nodes, err := h.nodeDAO.SearchActive(actor.WorkspaceID, filter)
	if err != nil {
		response.InternalError(c, "搜索失败", err)
		return
	}
	nodeIDs := make([]uint, 0, len(nodes))
	for _, node := range nodes {
		nodeIDs = append(nodeIDs, node.ID)
	}
	readable, err := h.authz.ReadableNodeIDs(actor, nodeIDs)
	if err != nil {
		h.handleAuthorizationError(c, err)
		return
	}
	visible := make([]model.Node, 0, len(nodes))
	for i := range nodes {
		if readable[nodes[i].ID] {
			visible = append(visible, nodes[i])
		}
	}
	if err := h.decorateFavorites(actor, visible); err != nil {
		response.InternalError(c, "读取收藏状态失败", err)
		return
	}
	if err := h.decorateStorage(actor.WorkspaceID, visible); err != nil {
		response.InternalError(c, "读取文件存储状态失败", err)
		return
	}
	h.respondSearchPage(c, visible)
}

func parseNodeSearchFilter(c *gin.Context) (dao.NodeSearchFilter, string) {
	filter := dao.NodeSearchFilter{
		Keyword:   strings.ToLower(strings.TrimSpace(c.Query("keyword"))),
		NodeType:  strings.ToLower(strings.TrimSpace(c.Query("type"))),
		Extension: strings.ToLower(strings.TrimSpace(c.Query("extension"))),
		CreatedBy: strings.ToLower(strings.TrimSpace(c.Query("created_by"))),
		UpdatedBy: strings.ToLower(strings.TrimSpace(c.Query("updated_by"))),
		Sort:      strings.ToLower(strings.TrimSpace(c.DefaultQuery("sort", "relevance"))),
	}
	if utf8.RuneCountInString(filter.Keyword) > 255 {
		return filter, "keyword 长度不能超过 255 个字符"
	}
	if filter.NodeType != "" && filter.NodeType != "file" && filter.NodeType != "folder" {
		return filter, "type 只能是 file 或 folder"
	}
	if filter.Extension != "" {
		filter.Extension = strings.TrimPrefix(filter.Extension, ".")
		if filter.Extension == "" || utf8.RuneCountInString(filter.Extension) > 31 || !validSearchExtension(filter.Extension) {
			return filter, "extension 格式不合法"
		}
		filter.Extension = "." + filter.Extension
	}
	if utf8.RuneCountInString(filter.CreatedBy) > 255 || utf8.RuneCountInString(filter.UpdatedBy) > 255 {
		return filter, "创建人或修改人长度不能超过 255 个字符"
	}
	var message string
	if filter.CreatedFrom, message = parseSearchDate(c.Query("created_from"), false); message != "" {
		return filter, "created_from " + message
	}
	if filter.CreatedTo, message = parseSearchDate(c.Query("created_to"), true); message != "" {
		return filter, "created_to " + message
	}
	if filter.UpdatedFrom, message = parseSearchDate(c.Query("updated_from"), false); message != "" {
		return filter, "updated_from " + message
	}
	if filter.UpdatedTo, message = parseSearchDate(c.Query("updated_to"), true); message != "" {
		return filter, "updated_to " + message
	}
	if filter.CreatedFrom != nil && filter.CreatedTo != nil && !filter.CreatedFrom.Before(*filter.CreatedTo) {
		return filter, "创建时间范围不合法"
	}
	if filter.UpdatedFrom != nil && filter.UpdatedTo != nil && !filter.UpdatedFrom.Before(*filter.UpdatedTo) {
		return filter, "修改时间范围不合法"
	}
	if filter.MinSize, message = parseSearchSize(c.Query("min_size")); message != "" {
		return filter, "min_size " + message
	}
	if filter.MaxSize, message = parseSearchSize(c.Query("max_size")); message != "" {
		return filter, "max_size " + message
	}
	if filter.MinSize != nil && filter.MaxSize != nil && *filter.MinSize > *filter.MaxSize {
		return filter, "文件大小范围不合法"
	}
	if filter.NodeType == "folder" && (filter.Extension != "" || filter.MinSize != nil || filter.MaxSize != nil) {
		return filter, "目录类型不能同时筛选扩展名或文件大小"
	}
	allowedSorts := map[string]bool{"relevance": true, "updated_desc": true, "created_desc": true, "name_asc": true, "size_asc": true, "size_desc": true}
	if !allowedSorts[filter.Sort] {
		return filter, "sort 参数不合法"
	}
	if filter.Keyword == "" && filter.NodeType == "" && filter.Extension == "" && filter.CreatedBy == "" && filter.UpdatedBy == "" &&
		filter.CreatedFrom == nil && filter.CreatedTo == nil && filter.UpdatedFrom == nil && filter.UpdatedTo == nil && filter.MinSize == nil && filter.MaxSize == nil {
		return filter, "请至少输入一个搜索条件"
	}
	return filter, ""
}

func validSearchExtension(value string) bool {
	for _, char := range value {
		if !unicode.IsLetter(char) && !unicode.IsDigit(char) && char != '.' && char != '_' && char != '-' {
			return false
		}
	}
	return true
}

func parseSearchDate(raw string, upperBound bool) (*time.Time, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}
	parsed, err := time.ParseInLocation("2006-01-02", raw, time.Local)
	if err != nil {
		return nil, "必须使用 YYYY-MM-DD 格式"
	}
	if upperBound {
		parsed = parsed.AddDate(0, 0, 1)
	}
	return &parsed, ""
}

func parseSearchSize(raw string) (*int64, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, ""
	}
	value, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || value < 0 {
		return nil, "必须是非负整数"
	}
	return &value, ""
}

func (h *NodeHandler) decorateStorage(workspaceID uint, nodes []model.Node) error {
	fileIDs := make([]uint, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == "file" {
			fileIDs = append(fileIDs, node.ID)
		}
	}
	versions, err := h.fileDAO.ActiveStorageByNodeIDs(workspaceID, fileIDs)
	if err != nil {
		return err
	}
	for index := range nodes {
		if version, ok := versions[nodes[index].ID]; ok {
			nodes[index].StorageClass = normalizedVersionStorageClass(version.StorageClass)
			nodes[index].ArchiveError = version.ArchiveError
			nodes[index].LastAccessedAt = version.LastAccessedAt
		}
	}
	return nil
}

func (h *NodeHandler) ListFavorites(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	nodes, err := h.favoriteDAO.ListActiveNodes(actor.WorkspaceID, actor.UserID, 5000)
	if err != nil {
		response.InternalError(c, "读取收藏失败", err)
		return
	}
	visible := make([]model.Node, 0, len(nodes))
	for i := range nodes {
		allowed, authErr := h.authz.CanRead(actor, nodes[i].ID)
		if authErr != nil {
			h.handleAuthorizationError(c, authErr)
			return
		}
		if allowed {
			nodes[i].IsFavorite = true
			visible = append(visible, nodes[i])
		}
	}
	if err := h.decorateStorage(actor.WorkspaceID, visible); err != nil {
		response.InternalError(c, "读取文件存储状态失败", err)
		return
	}
	h.respondPage(c, visible)
}

func (h *NodeHandler) SetFavorite(c *gin.Context) {
	actor, node, ok := h.authorizeRead(c)
	if !ok {
		return
	}
	var req struct {
		Favorite *bool `json:"favorite"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.Favorite == nil {
		response.BadRequest(c, "favorite 必须是布尔值")
		return
	}
	if err := h.favoriteDAO.Set(actor.WorkspaceID, actor.UserID, node.ID, *req.Favorite); err != nil {
		response.InternalError(c, "更新收藏失败", err)
		return
	}
	node.IsFavorite = *req.Favorite
	response.Success(c, gin.H{"node_id": node.ID, "favorite": *req.Favorite})
}

func (h *NodeHandler) FolderTree(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	folders, err := h.nodeDAO.ListActiveFolders(actor.WorkspaceID)
	if err != nil {
		response.InternalError(c, "读取目录树失败", err)
		return
	}
	writable := make([]model.Node, 0, len(folders))
	for i := range folders {
		allowed, authErr := h.authz.CanWrite(actor, folders[i].ID)
		if authErr != nil {
			h.handleAuthorizationError(c, authErr)
			return
		}
		if allowed {
			writable = append(writable, folders[i])
		}
	}
	response.Success(c, writable)
}

func (h *NodeHandler) listChildren(c *gin.Context, parentID *uint) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	if parentID != nil {
		parent, err := h.nodeDAO.GetByID(actor.WorkspaceID, *parentID)
		if err != nil {
			response.InternalError(c, "读取目录失败", err)
			return
		}
		if parent == nil || parent.Type != "folder" || parent.Status != "active" {
			response.NotFound(c, "目录不存在")
			return
		}
		allowed, err := h.authz.CanRead(actor, parent.ID)
		if err != nil {
			h.handleAuthorizationError(c, err)
			return
		}
		recordDataAuthorization(c, allowed, "node:read", "folder", parent.ID)
		if !allowed {
			response.Forbidden(c, "无权读取该目录")
			return
		}
	}
	nodes, err := h.nodeDAO.ListChildren(actor.WorkspaceID, parentID)
	if err != nil {
		response.InternalError(c, "读取目录内容失败", err)
		return
	}
	visible := make([]model.Node, 0, len(nodes))
	for i := range nodes {
		allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
		if !allowed {
			allowed, err = h.authz.CanRead(actor, nodes[i].ID)
			if err != nil {
				h.handleAuthorizationError(c, err)
				return
			}
		}
		if allowed {
			visible = append(visible, nodes[i])
		}
	}
	if err := h.decorateFavorites(actor, visible); err != nil {
		response.InternalError(c, "读取收藏状态失败", err)
		return
	}
	if err := h.decorateStorage(actor.WorkspaceID, visible); err != nil {
		response.InternalError(c, "读取文件存储状态失败", err)
		return
	}
	if parentID != nil {
		rememberRecentNode(h.collaborationDAO, actor, *parentID)
	}
	h.respondPage(c, visible)
}

func (h *NodeHandler) decorateFavorites(actor authorization.Actor, nodes []model.Node) error {
	if len(nodes) == 0 {
		return nil
	}
	nodeIDs, err := h.favoriteDAO.ListNodeIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return err
	}
	favoriteIDs := make(map[uint]struct{}, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		favoriteIDs[nodeID] = struct{}{}
	}
	for i := range nodes {
		_, nodes[i].IsFavorite = favoriteIDs[nodes[i].ID]
	}
	return nil
}

func (h *NodeHandler) respondPage(c *gin.Context, nodes []model.Node) {
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	if keyword != "" {
		prefix := strings.ToLower(strings.TrimSpace(keyword))
		filtered := nodes[:0]
		for _, node := range nodes {
			if strings.HasPrefix(strings.ToLower(node.Name), prefix) {
				filtered = append(filtered, node)
			}
		}
		nodes = filtered
	}
	total := int64(len(nodes))
	start := (page - 1) * pageSize
	if start > len(nodes) {
		start = len(nodes)
	}
	end := start + pageSize
	if end > len(nodes) {
		end = len(nodes)
	}
	response.SuccessWithPage(c, nodes[start:end], total, page, pageSize)
}

func (h *NodeHandler) respondSearchPage(c *gin.Context, nodes []model.Node) {
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	total := int64(len(nodes))
	start := (page - 1) * pageSize
	if start > len(nodes) {
		start = len(nodes)
	}
	end := start + pageSize
	if end > len(nodes) {
		end = len(nodes)
	}
	response.SuccessWithPage(c, nodes[start:end], total, page, pageSize)
}

func (h *NodeHandler) authorizeRead(c *gin.Context) (authorization.Actor, *model.Node, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, false
	}
	nodeID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return authorization.Actor{}, nil, false
	}
	node, err := h.nodeDAO.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取节点失败", err)
		return authorization.Actor{}, nil, false
	}
	if node == nil || node.Status != "active" {
		response.NotFound(c, "节点不存在")
		return authorization.Actor{}, nil, false
	}
	allowed, err := h.authz.CanRead(actor, node.ID)
	if err != nil {
		h.handleAuthorizationError(c, err)
		return authorization.Actor{}, nil, false
	}
	recordDataAuthorization(c, allowed, "node:read", node.Type, node.ID)
	if !allowed {
		response.Forbidden(c, "无权读取该节点")
		return authorization.Actor{}, nil, false
	}
	return actor, node, true
}

func (h *NodeHandler) authorizeWrite(c *gin.Context, expectedStatus string) (authorization.Actor, *model.Node, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, false
	}
	nodeID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return authorization.Actor{}, nil, false
	}
	node, err := h.nodeDAO.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取节点失败", err)
		return authorization.Actor{}, nil, false
	}
	if node == nil || node.Status != expectedStatus {
		response.NotFound(c, "节点不存在")
		return authorization.Actor{}, nil, false
	}
	var allowed bool
	if expectedStatus == "trashed" {
		allowed, err = h.authz.CanRestore(actor, node.ID)
	} else {
		allowed, err = h.authz.CanWrite(actor, node.ID)
	}
	if err != nil {
		h.handleAuthorizationError(c, err)
		return authorization.Actor{}, nil, false
	}
	recordDataAuthorization(c, allowed, "node:write", node.Type, node.ID)
	if !allowed {
		response.Forbidden(c, "无权修改该节点")
		return authorization.Actor{}, nil, false
	}
	return actor, node, true
}

func (h *NodeHandler) handleAuthorizationError(c *gin.Context, err error) {
	if errors.Is(err, authorization.ErrNodeNotFound) {
		response.NotFound(c, "目录或文件不存在")
		return
	}
	response.InternalError(c, "目录权限校验失败", err)
}
