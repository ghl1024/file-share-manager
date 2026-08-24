/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"errors"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
)

const (
	DashboardScopeMine      = "mine"
	DashboardScopeWorkspace = "workspace"
)

type DashboardDAO struct {
	db *gorm.DB
}

type DashboardNodeCandidate struct {
	ID   uint
	Type string
}

type DashboardNodeSummary struct {
	ID         uint       `json:"id"`
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	UpdatedAt  time.Time  `json:"updated_at"`
	ActivityAt *time.Time `json:"activity_at,omitempty"`
}

type DashboardTaskSummary struct {
	UploadInProgress   int64 `json:"upload_in_progress"`
	UploadFailed       int64 `json:"upload_failed"`
	DownloadInProgress int64 `json:"download_in_progress"`
	DownloadReady      int64 `json:"download_ready"`
	DownloadFailed     int64 `json:"download_failed"`
}

type DashboardStats struct {
	ViewScope          string                 `json:"view_scope"`
	CanViewWorkspace   bool                   `json:"can_view_workspace"`
	CanBrowseFiles     bool                   `json:"can_browse_files"`
	WorkspaceCount     int64                  `json:"workspace_count"`
	FileCount          int64                  `json:"file_count"`
	FolderCount        int64                  `json:"folder_count"`
	ActiveUserCount    int64                  `json:"active_user_count"`
	ActiveShareCount   int64                  `json:"active_share_count"`
	MyActiveShareCount int64                  `json:"my_active_share_count"`
	FavoriteCount      int64                  `json:"favorite_count"`
	UsedBytes          int64                  `json:"used_bytes"`
	ReservedBytes      int64                  `json:"reserved_bytes"`
	QuotaBytes         *int64                 `json:"quota_bytes"`
	QuotaSource        string                 `json:"quota_source"`
	Tasks              DashboardTaskSummary   `json:"tasks"`
	RecentNodes        []DashboardNodeSummary `json:"recent_nodes"`
	FavoriteNodes      []DashboardNodeSummary `json:"favorite_nodes"`
}

func NewDashboardDAO() *DashboardDAO {
	return &DashboardDAO{db: database.DB}
}

func (dao *DashboardDAO) Stats(workspaceID, userID uint, isSuperAdmin, canViewWorkspace bool, scope string) (*DashboardStats, error) {
	stats := &DashboardStats{
		ViewScope: scope, CanViewWorkspace: canViewWorkspace,
		RecentNodes: []DashboardNodeSummary{}, FavoriteNodes: []DashboardNodeSummary{},
	}
	workspaceQuery := dao.db.Model(&model.Workspace{}).Where("status = ?", 1)
	if !isSuperAdmin {
		workspaceQuery = workspaceQuery.
			Joins("JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id").
			Where("workspace_memberships.user_id = ?", userID)
	}
	if err := workspaceQuery.Distinct("workspaces.id").Count(&stats.WorkspaceCount).Error; err != nil {
		return nil, err
	}

	var workspace model.Workspace
	if err := dao.db.Select("id", "used_bytes", "reserved_bytes", "quota_bytes").First(&workspace, workspaceID).Error; err != nil {
		return nil, err
	}
	if scope == DashboardScopeWorkspace {
		stats.UsedBytes = workspace.UsedBytes
		stats.ReservedBytes = workspace.ReservedBytes
		stats.QuotaBytes = workspace.QuotaBytes
		stats.QuotaSource = "workspace"
		if err := dao.db.Model(&model.WorkspaceMembership{}).
			Joins("JOIN users ON users.id = workspace_memberships.user_id AND users.status = ?", 1).
			Where("workspace_memberships.workspace_id = ?", workspaceID).
			Count(&stats.ActiveUserCount).Error; err != nil {
			return nil, err
		}
	} else {
		var membership model.WorkspaceMembership
		err := dao.db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&membership).Error
		if err != nil && !(isSuperAdmin && errors.Is(err, gorm.ErrRecordNotFound)) {
			return nil, err
		}
		if err == nil {
			stats.UsedBytes = membership.UsedBytes
			stats.ReservedBytes = membership.ReservedBytes
			stats.QuotaBytes = membership.QuotaBytes
			if membership.QuotaBytes != nil {
				stats.QuotaSource = "personal"
			} else if workspace.QuotaBytes != nil {
				stats.QuotaSource = "workspace_shared"
			} else {
				stats.QuotaSource = "unlimited"
			}
		} else {
			stats.QuotaSource = "unavailable"
		}
	}

	now := time.Now()
	activeShares := dao.db.Model(&model.Share{}).
		Where("workspace_id = ? AND status = ? AND expires_at > ?", workspaceID, "active", now).
		Where("max_downloads IS NULL OR download_count < max_downloads")
	if scope == DashboardScopeMine {
		activeShares = activeShares.Where("created_by = ?", userID)
	}
	if err := activeShares.Count(&stats.ActiveShareCount).Error; err != nil {
		return nil, err
	}
	if err := dao.db.Model(&model.Share{}).
		Where("workspace_id = ? AND created_by = ? AND status = ? AND expires_at > ?", workspaceID, userID, "active", now).
		Where("max_downloads IS NULL OR download_count < max_downloads").
		Count(&stats.MyActiveShareCount).Error; err != nil {
		return nil, err
	}
	if err := dao.loadTaskSummary(workspaceID, userID, now, &stats.Tasks); err != nil {
		return nil, err
	}
	return stats, nil
}

func (dao *DashboardDAO) CountActiveNodes(workspaceID uint) (files, folders int64, err error) {
	if err = dao.db.Model(&model.Node{}).
		Where("workspace_id = ? AND status = ? AND type = ?", workspaceID, "active", "file").
		Count(&files).Error; err != nil {
		return 0, 0, err
	}
	err = dao.db.Model(&model.Node{}).
		Where("workspace_id = ? AND status = ? AND type = ?", workspaceID, "active", "folder").
		Count(&folders).Error
	return files, folders, err
}

func (dao *DashboardDAO) ListActiveNodeCandidates(workspaceID, afterID uint, limit int) ([]DashboardNodeCandidate, error) {
	if limit <= 0 || limit > 2000 {
		limit = 1000
	}
	var nodes []DashboardNodeCandidate
	err := dao.db.Model(&model.Node{}).
		Select("id", "type").
		Where("workspace_id = ? AND status = ? AND id > ?", workspaceID, "active", afterID).
		Order("id ASC").Limit(limit).Find(&nodes).Error
	return nodes, err
}

func (dao *DashboardDAO) loadTaskSummary(workspaceID, userID uint, now time.Time, summary *DashboardTaskSummary) error {
	uploads := dao.db.Model(&model.UploadSession{}).Where("workspace_id = ? AND created_by = ?", workspaceID, userID)
	if err := uploads.Where("status IN ?", []string{"initialized", "uploading", "merging", "scanning"}).Count(&summary.UploadInProgress).Error; err != nil {
		return err
	}
	if err := dao.db.Model(&model.UploadSession{}).
		Where("workspace_id = ? AND created_by = ? AND status = ?", workspaceID, userID, "failed").
		Count(&summary.UploadFailed).Error; err != nil {
		return err
	}
	downloads := dao.db.Model(&model.BatchDownloadJob{}).Where("workspace_id = ? AND created_by = ?", workspaceID, userID)
	if err := downloads.Where("status IN ?", []string{"queued", "running"}).Count(&summary.DownloadInProgress).Error; err != nil {
		return err
	}
	if err := dao.db.Model(&model.BatchDownloadJob{}).
		Where("workspace_id = ? AND created_by = ? AND status = ?", workspaceID, userID, "completed").
		Where("expires_at IS NULL OR expires_at > ?", now).Count(&summary.DownloadReady).Error; err != nil {
		return err
	}
	return dao.db.Model(&model.BatchDownloadJob{}).
		Where("workspace_id = ? AND created_by = ? AND status = ?", workspaceID, userID, "failed").
		Count(&summary.DownloadFailed).Error
}
