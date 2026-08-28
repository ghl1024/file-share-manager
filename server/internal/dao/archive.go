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

type ArchiveCandidate struct {
	WorkspaceID uint
	StorageKey  string
	Size        int64
	SHA256      string
}

type ArchiveDAO struct {
	db *gorm.DB
}

var ErrArchiveCandidateChanged = errors.New("archive candidate changed")

func NewArchiveDAO() *ArchiveDAO { return &ArchiveDAO{db: database.DB} }

// ListCandidates groups by immutable storage key. A key is eligible only when
// every version referencing it is cold, preventing a restored recent version
// from being moved because an older version shares the same object.
func (dao *ArchiveDAO) ListCandidates(cutoff time.Time, limit int) ([]ArchiveCandidate, error) {
	var candidates []ArchiveCandidate
	err := dao.db.Model(&model.FileVersion{}).
		Select("workspace_id, storage_key, MAX(size) AS size, MAX(sha256) AS sha256").
		Where("storage_class IN ?", []string{"", "standard"}).
		Group("workspace_id, storage_key").
		Having("MAX(COALESCE(last_accessed_at, created_at)) <= ?", cutoff).
		Order("MAX(COALESCE(last_accessed_at, created_at)) ASC, storage_key ASC").
		Limit(limit).
		Scan(&candidates).Error
	return candidates, err
}

func (dao *ArchiveDAO) Complete(candidate ArchiveCandidate, cutoff time.Time) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var versions []model.FileVersion
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND storage_key = ? AND storage_class IN ?", candidate.WorkspaceID, candidate.StorageKey, []string{"", "standard"}).
			Find(&versions).Error; err != nil {
			return err
		}
		if len(versions) == 0 {
			return ErrArchiveCandidateChanged
		}
		for _, version := range versions {
			lastAccess := version.CreatedAt
			if version.LastAccessedAt != nil {
				lastAccess = *version.LastAccessedAt
			}
			if lastAccess.After(cutoff) {
				return ErrArchiveCandidateChanged
			}
		}
		if err := tx.Model(&model.FileVersion{}).
			Where("workspace_id = ? AND storage_key = ?", candidate.WorkspaceID, candidate.StorageKey).
			Updates(map[string]any{"storage_class": "archive", "archive_error": ""}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ShareItem{}).
			Where("storage_key = ? AND share_id IN (?)", candidate.StorageKey, tx.Model(&model.Share{}).Select("id").Where("workspace_id = ?", candidate.WorkspaceID)).
			Update("storage_class", "archive").Error; err != nil {
			return err
		}
		if err := tx.Model(&model.BatchDownloadItem{}).
			Where("storage_key = ? AND job_id IN (?)", candidate.StorageKey, tx.Model(&model.BatchDownloadJob{}).Select("id").Where("workspace_id = ?", candidate.WorkspaceID)).
			Update("storage_class", "archive").Error; err != nil {
			return err
		}
		for _, version := range versions {
			if err := appendChange(tx, candidate.WorkspaceID, "file_version", version.ID, "archive", map[string]any{
				"node_id": version.NodeID, "version_no": version.VersionNo, "storage_key": version.StorageKey, "storage_class": "archive",
			}); err != nil {
				return err
			}
		}
		return nil
	})
}

func (dao *ArchiveDAO) RecordFailure(candidate ArchiveCandidate, message string) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	return dao.db.Model(&model.FileVersion{}).
		Where("workspace_id = ? AND storage_key = ? AND storage_class IN ?", candidate.WorkspaceID, candidate.StorageKey, []string{"", "standard"}).
		Update("archive_error", message).Error
}
