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

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
)

const dashboardSummaryLimit = 5

type DashboardHandler struct {
	dashboard     *dao.DashboardDAO
	collaboration *dao.CollaborationDAO
	favorites     *dao.FavoriteDAO
	permissions   *dao.PermissionDAO
	authz         *authorization.Service
}

func NewDashboardHandler() *DashboardHandler {
	return &DashboardHandler{
		dashboard: dao.NewDashboardDAO(), collaboration: dao.NewCollaborationDAO(),
		favorites: dao.NewFavoriteDAO(), permissions: dao.NewPermissionDAO(), authz: authorization.NewService(),
	}
}

// @Summary Stats
// @Description Handles GET /api/fileshare/v1/management/dashboard/stats.
// @Tags Dashboard
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param scope query string false "scope"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/dashboard/stats [get]
func (h *DashboardHandler) Stats(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	scope := strings.TrimSpace(c.DefaultQuery("scope", dao.DashboardScopeMine))
	if scope != dao.DashboardScopeMine && scope != dao.DashboardScopeWorkspace {
		response.BadRequest(c, "工作台视角必须是 mine 或 workspace")
		return
	}
	canViewWorkspace := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
	if scope == dao.DashboardScopeWorkspace && !canViewWorkspace {
		response.Forbidden(c, "只有工作空间管理员可以查看全空间统计")
		return
	}
	canBrowseFiles := canViewWorkspace
	if !canBrowseFiles {
		var err error
		canBrowseFiles, err = h.permissions.UserHasPermission(actor.WorkspaceID, actor.UserID, "file:list")
		if err != nil {
			response.InternalError(c, "校验文件浏览权限失败", err)
			return
		}
	}
	stats, err := h.dashboard.Stats(actor.WorkspaceID, actor.UserID, actor.IsSuperAdmin, canViewWorkspace, scope)
	if err != nil {
		response.InternalError(c, "读取工作台统计失败", err)
		return
	}
	stats.CanBrowseFiles = canBrowseFiles
	if canBrowseFiles {
		if scope == dao.DashboardScopeWorkspace {
			stats.FileCount, stats.FolderCount, err = h.dashboard.CountActiveNodes(actor.WorkspaceID)
		} else {
			stats.FileCount, stats.FolderCount, err = h.readableNodeCounts(actor)
		}
		if err != nil {
			response.InternalError(c, "统计可访问文件失败", err)
			return
		}
		if err := h.decoratePersonalSummary(actor, stats); err != nil {
			response.InternalError(c, "读取个人工作台摘要失败", err)
			return
		}
	}
	response.Success(c, stats)
}

func (h *DashboardHandler) readableNodeCounts(actor authorization.Actor) (int64, int64, error) {
	var fileCount, folderCount int64
	var afterID uint
	for {
		candidates, err := h.dashboard.ListActiveNodeCandidates(actor.WorkspaceID, afterID, 1000)
		if err != nil {
			return 0, 0, err
		}
		if len(candidates) == 0 {
			return fileCount, folderCount, nil
		}
		ids := make([]uint, 0, len(candidates))
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		readable, err := h.authz.ReadableNodeIDs(actor, ids)
		if err != nil {
			return 0, 0, err
		}
		for _, candidate := range candidates {
			if !readable[candidate.ID] {
				continue
			}
			if candidate.Type == "file" {
				fileCount++
			} else if candidate.Type == "folder" {
				folderCount++
			}
		}
		afterID = candidates[len(candidates)-1].ID
		if len(candidates) < 1000 {
			return fileCount, folderCount, nil
		}
	}
}

func (h *DashboardHandler) decoratePersonalSummary(actor authorization.Actor, stats *dao.DashboardStats) error {
	recent, err := h.collaboration.ListRecentNodes(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return err
	}
	recentNodes := make([]model.Node, 0, len(recent))
	for _, item := range recent {
		recentNodes = append(recentNodes, item.Node)
	}
	recentReadable, err := h.readableNodeMap(actor, recentNodes)
	if err != nil {
		return err
	}
	for _, item := range recent {
		if !recentReadable[item.ID] || len(stats.RecentNodes) >= dashboardSummaryLimit {
			continue
		}
		activityAt := item.RecentAccessedAt
		stats.RecentNodes = append(stats.RecentNodes, dao.DashboardNodeSummary{
			ID: item.ID, Name: item.Name, Type: item.Type, UpdatedAt: item.UpdatedAt, ActivityAt: &activityAt,
		})
	}

	favorites, err := h.favorites.ListActiveNodes(actor.WorkspaceID, actor.UserID, 5000)
	if err != nil {
		return err
	}
	favoriteReadable, err := h.readableNodeMap(actor, favorites)
	if err != nil {
		return err
	}
	for _, item := range favorites {
		if !favoriteReadable[item.ID] {
			continue
		}
		stats.FavoriteCount++
		if len(stats.FavoriteNodes) < dashboardSummaryLimit {
			stats.FavoriteNodes = append(stats.FavoriteNodes, dao.DashboardNodeSummary{
				ID: item.ID, Name: item.Name, Type: item.Type, UpdatedAt: item.UpdatedAt,
			})
		}
	}
	return nil
}

func (h *DashboardHandler) readableNodeMap(actor authorization.Actor, nodes []model.Node) (map[uint]bool, error) {
	ids := make([]uint, 0, len(nodes))
	for _, node := range nodes {
		ids = append(ids, node.ID)
	}
	return h.authz.ReadableNodeIDs(actor, ids)
}
