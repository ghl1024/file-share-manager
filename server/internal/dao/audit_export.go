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

type AuditExportDAO struct{ db *gorm.DB }

func NewAuditExportDAO() *AuditExportDAO                           { return &AuditExportDAO{db: database.DB} }
func (dao *AuditExportDAO) Create(job *model.AuditExportJob) error { return dao.db.Create(job).Error }

func (dao *AuditExportDAO) ListPage(workspaceID, userID uint, page, pageSize int) (*pagination.PageResult[model.AuditExportJob], error) {
	return pagination.Paging[model.AuditExportJob](dao.db.Model(&model.AuditExportJob{}).Where("workspace_id = ? AND created_by = ?", workspaceID, userID).Order("created_at DESC, id DESC"), page, pageSize)
}

func (dao *AuditExportDAO) GetForOwner(workspaceID, userID uint, id string) (*model.AuditExportJob, error) {
	var job model.AuditExportJob
	err := dao.db.Where("workspace_id = ? AND created_by = ? AND id = ?", workspaceID, userID, id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *AuditExportDAO) GetByID(id string) (*model.AuditExportJob, error) {
	var job model.AuditExportJob
	err := dao.db.Where("id = ?", id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *AuditExportDAO) ListQueuedIDs(limit int) ([]string, error) {
	var ids []string
	err := dao.db.Model(&model.AuditExportJob{}).Where("status = ?", "queued").Order("created_at ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (dao *AuditExportDAO) Claim(id string, now time.Time) (bool, error) {
	result := dao.db.Model(&model.AuditExportJob{}).Where("id = ? AND status = ?", id, "queued").Updates(map[string]any{"status": "running", "started_at": now, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (dao *AuditExportDAO) Complete(id, path string, records int, size int64, now, expires time.Time) error {
	return dao.db.Model(&model.AuditExportJob{}).Where("id = ? AND status = ?", id, "running").Updates(map[string]any{"status": "completed", "file_path": path, "record_count": records, "file_size": size, "completed_at": now, "expires_at": expires, "updated_at": now}).Error
}

func (dao *AuditExportDAO) Fail(id string, now time.Time) error {
	return dao.db.Model(&model.AuditExportJob{}).Where("id = ? AND status = ?", id, "running").Updates(map[string]any{"status": "failed", "error_message": "导出任务执行失败", "updated_at": now}).Error
}

func (dao *AuditExportDAO) RequeueInterrupted() error {
	return dao.db.Model(&model.AuditExportJob{}).Where("status = ?", "running").Updates(map[string]any{"status": "queued", "started_at": nil, "updated_at": time.Now()}).Error
}

func (dao *AuditExportDAO) Expire(now time.Time) ([]string, error) {
	var paths []string
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AuditExportJob{}).Where("status = ? AND expires_at <= ?", "completed", now).Pluck("file_path", &paths).Error; err != nil {
			return err
		}
		if len(paths) == 0 {
			return nil
		}
		return tx.Model(&model.AuditExportJob{}).Where("status = ? AND expires_at <= ?", "completed", now).Updates(map[string]any{"status": "expired", "file_path": "", "file_size": 0, "updated_at": now}).Error
	})
	return paths, err
}
