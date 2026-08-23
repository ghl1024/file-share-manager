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
	"gorm.io/gorm/clause"
)

type FileDAO struct {
	db *gorm.DB
}

type ScanRetrySummary struct {
	Retryable   int64      `json:"retryable"`
	Pending     int64      `json:"pending"`
	Exhausted   int64      `json:"exhausted"`
	Infected    int64      `json:"infected"`
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`
}

func NewFileDAO() *FileDAO {
	return &FileDAO{db: database.DB}
}

func (dao *FileDAO) GetVersion(workspaceID, nodeID uint, versionNo int) (*model.FileVersion, error) {
	var version model.FileVersion
	err := dao.db.Where("workspace_id = ? AND node_id = ? AND version_no = ?", workspaceID, nodeID, versionNo).First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}

func (dao *FileDAO) GetLatestVersion(workspaceID, nodeID uint) (*model.FileVersion, error) {
	var version model.FileVersion
	err := dao.db.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).Order("version_no DESC").First(&version).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}

func (dao *FileDAO) ListVersions(workspaceID, nodeID uint) ([]model.FileVersion, error) {
	var versions []model.FileVersion
	err := dao.db.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).Order("version_no DESC").Find(&versions).Error
	return versions, err
}

func (dao *FileDAO) TouchAccess(versionID uint, accessedAt time.Time) error {
	return dao.db.Model(&model.FileVersion{}).Where("id = ?", versionID).Update("last_accessed_at", accessedAt).Error
}

func (dao *FileDAO) TouchAccessByStorageKey(storageKey string, accessedAt time.Time) error {
	return dao.db.Model(&model.FileVersion{}).Where("storage_key = ?", storageKey).Update("last_accessed_at", accessedAt).Error
}

func (dao *FileDAO) ActiveStorageByNodeIDs(workspaceID uint, nodeIDs []uint) (map[uint]model.FileVersion, error) {
	result := make(map[uint]model.FileVersion)
	if len(nodeIDs) == 0 {
		return result, nil
	}
	var versions []model.FileVersion
	if err := dao.db.Table("file_versions AS fv").
		Select("fv.*").
		Joins("JOIN nodes AS n ON n.active_version = fv.id AND n.workspace_id = fv.workspace_id").
		Where("fv.workspace_id = ? AND n.id IN ?", workspaceID, nodeIDs).
		Find(&versions).Error; err != nil {
		return nil, err
	}
	for _, version := range versions {
		result[version.NodeID] = version
	}
	return result, nil
}

func (dao *FileDAO) UpdateScanStatus(workspaceID, nodeID uint, versionNo int, status, message string) (*model.FileVersion, error) {
	var version model.FileVersion
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND node_id = ? AND version_no = ?", workspaceID, nodeID, versionNo).
			First(&version).Error; err != nil {
			return err
		}
		updates := map[string]any{"scan_status": status, "scan_message": message, "scan_next_retry_at": nil}
		if status == "pending_scan" {
			now := time.Now()
			updates["scan_retry_count"] = 0
			updates["scan_last_attempt_at"] = now
			version.ScanRetryCount = 0
			version.ScanLastAttemptAt = &now
		}
		if err := tx.Model(&version).Updates(updates).Error; err != nil {
			return err
		}
		version.ScanStatus = status
		version.ScanMessage = message
		version.ScanNextRetryAt = nil
		return appendChange(tx, workspaceID, "file_version", version.ID, "update_scan_status", map[string]any{
			"node_id": version.NodeID, "version_no": version.VersionNo, "scan_status": status, "scan_message": message,
			"scan_retry_count": version.ScanRetryCount,
		})
	})
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &version, nil
}

func (dao *FileDAO) ListScanRetryCandidates(now time.Time, maxAttempts, limit int) ([]model.FileVersion, error) {
	var versions []model.FileVersion
	err := dao.db.Where("scan_status = ? AND scan_retry_count < ? AND (scan_next_retry_at IS NULL OR scan_next_retry_at <= ?) AND (storage_class = ? OR storage_class = '' OR storage_class IS NULL)",
		"scan_error", maxAttempts, now, "standard").
		Order("COALESCE(scan_next_retry_at, created_at) ASC, id ASC").Limit(limit).Find(&versions).Error
	return versions, err
}

func (dao *FileDAO) ClaimScanRetry(versionID uint, expectedRetryCount int, attemptedAt time.Time) (bool, error) {
	claimed := false
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var version model.FileVersion
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND scan_status = ? AND scan_retry_count = ?", versionID, "scan_error", expectedRetryCount).
			First(&version).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		retryCount := expectedRetryCount + 1
		if err := tx.Model(&version).Updates(map[string]any{
			"scan_status": "pending_scan", "scan_message": "自动重试扫描中",
			"scan_retry_count": retryCount, "scan_last_attempt_at": attemptedAt, "scan_next_retry_at": nil,
		}).Error; err != nil {
			return err
		}
		if err := appendChange(tx, version.WorkspaceID, "file_version", version.ID, "update_scan_status", map[string]any{
			"node_id": version.NodeID, "version_no": version.VersionNo, "scan_status": "pending_scan", "scan_retry_count": retryCount,
		}); err != nil {
			return err
		}
		claimed = true
		return nil
	})
	return claimed, err
}

func (dao *FileDAO) CompleteScanRetry(versionID uint, status, message string, nextRetryAt *time.Time) (bool, error) {
	completed := false
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var version model.FileVersion
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND scan_status = ?", versionID, "pending_scan").First(&version).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if err := tx.Model(&version).Updates(map[string]any{
			"scan_status": status, "scan_message": message, "scan_next_retry_at": nextRetryAt,
		}).Error; err != nil {
			return err
		}
		if err := appendChange(tx, version.WorkspaceID, "file_version", version.ID, "update_scan_status", map[string]any{
			"node_id": version.NodeID, "version_no": version.VersionNo, "scan_status": status,
			"scan_message": message, "scan_retry_count": version.ScanRetryCount, "scan_next_retry_at": nextRetryAt,
		}); err != nil {
			return err
		}
		completed = true
		return nil
	})
	return completed, err
}

func (dao *FileDAO) RequeueStaleScanRetries(cutoff, nextRetryAt time.Time) (int64, error) {
	result := dao.db.Model(&model.FileVersion{}).
		Where("scan_status = ? AND (scan_last_attempt_at IS NULL OR scan_last_attempt_at <= ?)", "pending_scan", cutoff).
		Updates(map[string]any{
			"scan_status": "scan_error", "scan_message": "扫描进程中断，已重新加入自动重试队列", "scan_next_retry_at": nextRetryAt,
		})
	return result.RowsAffected, result.Error
}

func (dao *FileDAO) ScanRetrySummary(maxAttempts int) (ScanRetrySummary, error) {
	var summary ScanRetrySummary
	queries := []struct {
		target *int64
		where  string
		args   []any
	}{
		{&summary.Retryable, "scan_status = ? AND scan_retry_count < ?", []any{"scan_error", maxAttempts}},
		{&summary.Pending, "scan_status = ?", []any{"pending_scan"}},
		{&summary.Exhausted, "scan_status = ? AND scan_retry_count >= ?", []any{"scan_error", maxAttempts}},
		{&summary.Infected, "scan_status = ?", []any{"infected"}},
	}
	for _, query := range queries {
		if err := dao.db.Model(&model.FileVersion{}).Where(query.where, query.args...).Count(query.target).Error; err != nil {
			return summary, err
		}
	}
	var earliest struct{ NextRetryAt *time.Time }
	if err := dao.db.Model(&model.FileVersion{}).
		Select("MIN(scan_next_retry_at) AS next_retry_at").
		Where("scan_status = ? AND scan_retry_count < ?", "scan_error", maxAttempts).
		Scan(&earliest).Error; err != nil {
		return summary, err
	}
	summary.NextRetryAt = earliest.NextRetryAt
	return summary, nil
}

func (dao *FileDAO) RestoreVersion(workspaceID, nodeID uint, sourceVersionNo int, actorID uint) (*model.FileVersion, error) {
	var restored model.FileVersion
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, nodeID, "file", "active").First(&node).Error; err != nil {
			return err
		}
		var source model.FileVersion
		if err := tx.Where("workspace_id = ? AND node_id = ? AND version_no = ?", workspaceID, nodeID, sourceVersionNo).First(&source).Error; err != nil {
			return err
		}
		var latest model.FileVersion
		if err := tx.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).Order("version_no DESC").First(&latest).Error; err != nil {
			return err
		}
		restored = source
		restored.ID = 0
		restored.VersionNo = latest.VersionNo + 1
		restored.CreatedBy = actorID
		restored.CreatedAt = time.Time{}
		if err := tx.Create(&restored).Error; err != nil {
			return err
		}
		if err := tx.Model(&node).Updates(map[string]any{"active_version": restored.ID, "updated_by": actorID}).Error; err != nil {
			return err
		}
		return appendChange(tx, workspaceID, "file_version", restored.ID, "restore", map[string]any{
			"node_id": restored.NodeID, "version_no": restored.VersionNo, "source_version_no": sourceVersionNo,
			"size": restored.Size, "sha256": restored.SHA256, "storage_key": restored.StorageKey,
		})
	})
	if err != nil {
		return nil, err
	}
	return &restored, nil
}
