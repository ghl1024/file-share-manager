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

type BackupDAO struct{ db *gorm.DB }

func NewBackupDAO() *BackupDAO { return &BackupDAO{db: database.DB} }

func (dao *BackupDAO) Create(job *model.BackupJob) error { return dao.db.Create(job).Error }

func (dao *BackupDAO) Update(jobID string, fields map[string]any) error {
	return dao.db.Model(&model.BackupJob{}).Where("id = ?", jobID).Updates(fields).Error
}

func (dao *BackupDAO) Get(workspaceID uint, id string) (*model.BackupJob, error) {
	var job model.BackupJob
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

func (dao *BackupDAO) LatestComplete(workspaceID uint) (*model.BackupJob, error) {
	var job model.BackupJob
	err := dao.db.Where("workspace_id = ? AND status = ?", workspaceID, "complete").Order("created_at DESC, id DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *BackupDAO) Latest(workspaceID uint) (*model.BackupJob, error) {
	var job model.BackupJob
	err := dao.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC, id DESC").First(&job).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &job, err
}

func (dao *BackupDAO) ListPage(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.BackupJob], error) {
	query := dao.db.Model(&model.BackupJob{}).Where("workspace_id = ?", workspaceID).Order("created_at DESC, id DESC")
	return pagination.Paging[model.BackupJob](query, page, pageSize)
}

func (dao *BackupDAO) MarkRunning(jobID string, now time.Time) error {
	return dao.Update(jobID, map[string]any{"status": "running", "started_at": now})
}

func (dao *BackupDAO) CreateDrill(drill *model.BackupRestoreDrill) error {
	return dao.db.Create(drill).Error
}

func (dao *BackupDAO) UpdateDrill(id string, fields map[string]any) error {
	return dao.db.Model(&model.BackupRestoreDrill{}).Where("id = ?", id).Updates(fields).Error
}

func (dao *BackupDAO) GetDrill(workspaceID uint, id string) (*model.BackupRestoreDrill, error) {
	var drill model.BackupRestoreDrill
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&drill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &drill, nil
}

func (dao *BackupDAO) ListDrillsPage(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.BackupRestoreDrill], error) {
	query := dao.db.Model(&model.BackupRestoreDrill{}).Where("workspace_id = ?", workspaceID).Order("created_at DESC, id DESC")
	return pagination.Paging[model.BackupRestoreDrill](query, page, pageSize)
}

func (dao *BackupDAO) LatestDrill(workspaceID uint) (*model.BackupRestoreDrill, error) {
	var drill model.BackupRestoreDrill
	err := dao.db.Where("workspace_id = ?", workspaceID).Order("created_at DESC, id DESC").First(&drill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &drill, err
}
