/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package model

import "time"

type AuditExportJob struct {
	ID           string     `gorm:"type:char(36);primaryKey" json:"id"`
	WorkspaceID  uint       `gorm:"index;not null" json:"workspace_id"`
	CreatedBy    uint       `gorm:"index;not null" json:"created_by"`
	Format       string     `gorm:"type:varchar(8);not null" json:"format"`
	Status       string     `gorm:"type:varchar(16);index;not null" json:"status"`
	FilterJSON   string     `gorm:"type:json;not null" json:"-"`
	FilePath     string     `gorm:"type:varchar(512)" json:"-"`
	RecordCount  int        `gorm:"not null;default:0" json:"record_count"`
	FileSize     int64      `gorm:"not null;default:0" json:"file_size"`
	ErrorMessage string     `gorm:"type:varchar(1000)" json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	ExpiresAt    *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (AuditExportJob) TableName() string { return "audit_export_jobs" }
