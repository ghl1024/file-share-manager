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
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
)

var (
	ErrBatchDownloadNotFound = errors.New("batch download job not found")
	ErrBatchDownloadState    = errors.New("batch download job state does not allow this operation")
)

type BatchDownloadDAO struct {
	db *gorm.DB
}

func NewBatchDownloadDAO() *BatchDownloadDAO { return &BatchDownloadDAO{db: database.DB} }

func (dao *BatchDownloadDAO) Create(job *model.BatchDownloadJob, items []model.BatchDownloadItem) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(job).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].JobID = job.ID
		}
		return tx.Create(&items).Error
	})
}

func (dao *BatchDownloadDAO) ListPage(workspaceID, userID uint, page, pageSize int) (*pagination.PageResult[model.BatchDownloadJob], error) {
	query := dao.db.Model(&model.BatchDownloadJob{}).
		Where("workspace_id = ? AND created_by = ?", workspaceID, userID).
		Order("created_at DESC, id DESC")
	return pagination.Paging[model.BatchDownloadJob](query, page, pageSize)
}

func (dao *BatchDownloadDAO) GetForOwner(workspaceID, userID uint, id string) (*model.BatchDownloadJob, error) {
	var job model.BatchDownloadJob
	err := dao.db.Where("workspace_id = ? AND created_by = ? AND id = ?", workspaceID, userID, id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *BatchDownloadDAO) GetByID(id string) (*model.BatchDownloadJob, error) {
	var job model.BatchDownloadJob
	err := dao.db.Where("id = ?", id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *BatchDownloadDAO) ListItems(id string) ([]model.BatchDownloadItem, error) {
	var items []model.BatchDownloadItem
	err := dao.db.Where("job_id = ?", id).Order("relative_path ASC, id ASC").Find(&items).Error
	return items, err
}

func (dao *BatchDownloadDAO) Claim(id string, startedAt time.Time) (bool, error) {
	result := dao.db.Model(&model.BatchDownloadJob{}).
		Where("id = ? AND status = ?", id, "queued").
		Updates(map[string]any{"status": "running", "started_at": startedAt, "error_message": "", "updated_at": startedAt})
	return result.RowsAffected == 1, result.Error
}

func (dao *BatchDownloadDAO) UpdateProgress(id string, files int, bytes int64) error {
	return dao.db.Model(&model.BatchDownloadJob{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]any{"processed_files": files, "processed_bytes": bytes, "updated_at": time.Now()}).Error
}

func (dao *BatchDownloadDAO) Complete(id string, archiveSize int64, completedAt, expiresAt time.Time) error {
	result := dao.db.Model(&model.BatchDownloadJob{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]any{
			"status": "completed", "archive_size": archiveSize, "completed_at": completedAt,
			"expires_at": expiresAt, "error_message": "", "updated_at": completedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBatchDownloadState
	}
	return nil
}

func (dao *BatchDownloadDAO) Fail(id, message string) error {
	now := time.Now()
	return dao.db.Model(&model.BatchDownloadJob{}).
		Where("id = ? AND status = ?", id, "running").
		Updates(map[string]any{"status": "failed", "error_message": message, "updated_at": now}).Error
}

func (dao *BatchDownloadDAO) Retry(workspaceID, userID uint, id string) error {
	now := time.Now()
	result := dao.db.Model(&model.BatchDownloadJob{}).
		Where("workspace_id = ? AND created_by = ? AND id = ? AND status = ?", workspaceID, userID, id, "failed").
		Updates(map[string]any{
			"status": "queued", "processed_files": 0, "processed_bytes": 0, "archive_size": 0,
			"error_message": "", "started_at": nil, "completed_at": nil, "expires_at": nil, "updated_at": now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrBatchDownloadState
	}
	return nil
}

func (dao *BatchDownloadDAO) RequeueInterrupted() error {
	now := time.Now()
	return dao.db.Model(&model.BatchDownloadJob{}).
		Where("status = ?", "running").
		Updates(map[string]any{"status": "queued", "started_at": nil, "processed_files": 0, "processed_bytes": 0, "updated_at": now}).Error
}

func (dao *BatchDownloadDAO) ListQueuedIDs(limit int) ([]string, error) {
	var ids []string
	err := dao.db.Model(&model.BatchDownloadJob{}).
		Where("status = ?", "queued").Order("created_at ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

// Expire marks completed jobs unavailable before their archive files are removed.
func (dao *BatchDownloadDAO) Expire(now time.Time) ([]string, error) {
	var ids []string
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.BatchDownloadJob{}).
			Where("status = ? AND expires_at IS NOT NULL AND expires_at <= ?", "completed", now).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		return tx.Model(&model.BatchDownloadJob{}).Where("id IN ? AND status = ?", ids, "completed").
			Updates(map[string]any{"status": "expired", "archive_size": 0, "updated_at": now}).Error
	})
	return ids, err
}
