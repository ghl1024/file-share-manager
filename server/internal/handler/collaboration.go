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
	"strings"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/ldapdn"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
)

type CollaborationHandler struct {
	collaboration *dao.CollaborationDAO
	files         *dao.FileDAO
	favorites     *dao.FavoriteDAO
	authz         *authorization.Service
}

type permissionSourceResponse struct {
	Type              string    `json:"type"`
	ID                uint      `json:"id"`
	Name              string    `json:"name"`
	DirectorySource   string    `json:"directory_source"`
	DirectoryPath     []string  `json:"directory_path"`
	GrantedLevel      string    `json:"granted_level"`
	InheritToChildren bool      `json:"inherit_to_children"`
	SharedAt          time.Time `json:"shared_at"`
}

type sharedNodeResponse struct {
	model.Node
	EffectiveAccessLevel string                     `json:"effective_access_level"`
	PermissionSources    []permissionSourceResponse `json:"permission_sources"`
	SharedAt             time.Time                  `json:"shared_at"`
}

type recentNodeResponse struct {
	model.Node
	EffectiveAccessLevel string    `json:"effective_access_level"`
	RecentAccessedAt     time.Time `json:"recent_accessed_at"`
	AccessCount          uint64    `json:"access_count"`
}

func NewCollaborationHandler() *CollaborationHandler {
	return &CollaborationHandler{
		collaboration: dao.NewCollaborationDAO(), files: dao.NewFileDAO(),
		favorites: dao.NewFavoriteDAO(), authz: authorization.NewService(),
	}
}

// @Summary List Shared With Me
// @Description Handles GET /api/fileshare/v1/management/collaboration/shared-with-me.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "keyword"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/collaboration/shared-with-me [get]
func (h *CollaborationHandler) ListSharedWithMe(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	groupIDs, err := dao.NewGroupDAO().ListUserGroupIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		response.InternalError(c, "读取用户组失败", err)
		return
	}
	grants, err := h.collaboration.ListDirectSharedGrants(actor.WorkspaceID, actor.UserID, groupIDs)
	if err != nil {
		response.InternalError(c, "读取共享目录失败", err)
		return
	}
	items, nodeIDs := mergeSharedGrants(grants)
	levels, err := h.authz.NodeAccessLevels(actor, nodeIDs)
	if err != nil {
		response.InternalError(c, "校验共享目录权限失败", err)
		return
	}
	visible := make([]sharedNodeResponse, 0, len(items))
	for _, item := range items {
		level := levels[item.ID]
		if level == "" {
			continue
		}
		item.EffectiveAccessLevel = level
		visible = append(visible, item)
	}
	if err := h.decorateSharedNodes(actor, visible); err != nil {
		response.InternalError(c, "读取共享文件状态失败", err)
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	visible = filterSharedNodes(visible, keyword)
	respondSlicePage(c, visible, page, pageSize)
}

// @Summary List Recent
// @Description Handles GET /api/fileshare/v1/management/collaboration/recent.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param keyword query string false "keyword"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/collaboration/recent [get]
func (h *CollaborationHandler) ListRecent(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	recent, err := h.collaboration.ListRecentNodes(actor.WorkspaceID, actor.UserID)
	if err != nil {
		response.InternalError(c, "读取最近使用失败", err)
		return
	}
	nodeIDs := make([]uint, 0, len(recent))
	for _, item := range recent {
		nodeIDs = append(nodeIDs, item.ID)
	}
	levels, err := h.authz.NodeAccessLevels(actor, nodeIDs)
	if err != nil {
		response.InternalError(c, "校验最近使用权限失败", err)
		return
	}
	items := make([]recentNodeResponse, 0, len(recent))
	for _, item := range recent {
		if level := levels[item.ID]; level != "" {
			items = append(items, recentNodeResponse{Node: item.Node, EffectiveAccessLevel: level, RecentAccessedAt: item.RecentAccessedAt, AccessCount: item.AccessCount})
		}
	}
	if err := h.decorateRecentNodes(actor, items); err != nil {
		response.InternalError(c, "读取最近文件状态失败", err)
		return
	}
	page, pageSize, keyword := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	items = filterRecentNodes(items, keyword)
	respondSlicePage(c, items, page, pageSize)
}

func mergeSharedGrants(grants []dao.DirectSharedGrant) ([]sharedNodeResponse, []uint) {
	items := make([]sharedNodeResponse, 0, len(grants))
	nodeIDs := make([]uint, 0, len(grants))
	byNode := make(map[uint]int, len(grants))
	for _, grant := range grants {
		source := permissionSourceResponse{
			Type: grant.SourceType, ID: grant.SourceID, Name: grant.SourceName,
			DirectorySource: grant.SourceDirectory, DirectoryPath: []string{},
			GrantedLevel: grant.GrantedLevel, InheritToChildren: grant.InheritChildren, SharedAt: grant.SharedAt,
		}
		if grant.SourceType == "group" && grant.SourceDirectory == "ldap" {
			source.DirectoryPath = ldapdn.OrganizationalPath(grant.SourceLDAPDN)
		}
		if index, exists := byNode[grant.ID]; exists {
			items[index].PermissionSources = append(items[index].PermissionSources, source)
			continue
		}
		byNode[grant.ID] = len(items)
		nodeIDs = append(nodeIDs, grant.ID)
		items = append(items, sharedNodeResponse{
			Node: grant.Node, SharedAt: grant.SharedAt,
			PermissionSources: []permissionSourceResponse{source},
		})
	}
	return items, nodeIDs
}

func filterSharedNodes(items []sharedNodeResponse, keyword string) []sharedNodeResponse {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return items
	}
	result := make([]sharedNodeResponse, 0, len(items))
	for _, item := range items {
		matched := strings.Contains(strings.ToLower(item.Name), keyword)
		for _, source := range item.PermissionSources {
			matched = matched || strings.Contains(strings.ToLower(source.Name), keyword)
		}
		if matched {
			result = append(result, item)
		}
	}
	return result
}

func filterRecentNodes(items []recentNodeResponse, keyword string) []recentNodeResponse {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return items
	}
	result := make([]recentNodeResponse, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name), keyword) {
			result = append(result, item)
		}
	}
	return result
}

func respondSlicePage[T any](c *gin.Context, items []T, page, pageSize int) {
	total := int64(len(items))
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	response.SuccessWithPage(c, items[start:end], total, page, pageSize)
}

func (h *CollaborationHandler) decorateSharedNodes(actor authorization.Actor, items []sharedNodeResponse) error {
	nodes := make([]model.Node, len(items))
	for i := range items {
		nodes[i] = items[i].Node
	}
	if err := h.decorateNodes(actor, nodes); err != nil {
		return err
	}
	for i := range items {
		items[i].Node = nodes[i]
	}
	return nil
}

func (h *CollaborationHandler) decorateRecentNodes(actor authorization.Actor, items []recentNodeResponse) error {
	nodes := make([]model.Node, len(items))
	for i := range items {
		nodes[i] = items[i].Node
	}
	if err := h.decorateNodes(actor, nodes); err != nil {
		return err
	}
	for i := range items {
		items[i].Node = nodes[i]
	}
	return nil
}

func (h *CollaborationHandler) decorateNodes(actor authorization.Actor, nodes []model.Node) error {
	fileIDs := make([]uint, 0, len(nodes))
	for _, node := range nodes {
		if node.Type == "file" {
			fileIDs = append(fileIDs, node.ID)
		}
	}
	versions, err := h.files.ActiveStorageByNodeIDs(actor.WorkspaceID, fileIDs)
	if err != nil {
		return err
	}
	favoriteIDs, err := h.favorites.ListNodeIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return err
	}
	favorites := make(map[uint]struct{}, len(favoriteIDs))
	for _, nodeID := range favoriteIDs {
		favorites[nodeID] = struct{}{}
	}
	for i := range nodes {
		_, nodes[i].IsFavorite = favorites[nodes[i].ID]
		if version, exists := versions[nodes[i].ID]; exists {
			nodes[i].StorageClass = normalizedVersionStorageClass(version.StorageClass)
			nodes[i].ArchiveError = version.ArchiveError
		}
	}
	return nil
}
