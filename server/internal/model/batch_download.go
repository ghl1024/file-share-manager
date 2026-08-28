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

// BatchDownloadJob represents an asynchronous ZIP archive owned by one user.
type BatchDownloadJob struct {
	ID             string     `gorm:"type:char(36);primaryKey" json:"id"`
	WorkspaceID    uint       `gorm:"index:idx_batch_download_owner,priority:1;not null" json:"-"`
	CreatedBy      uint       `gorm:"index:idx_batch_download_owner,priority:2;not null" json:"created_by"`
	Name           string     `gorm:"type:varchar(255);not null" json:"name"`
	Status         string     `gorm:"type:varchar(16);index;not null" json:"status"`
	TotalFiles     int        `gorm:"not null" json:"total_files"`
	ProcessedFiles int        `gorm:"not null;default:0" json:"processed_files"`
	TotalBytes     int64      `gorm:"not null" json:"total_bytes"`
	ProcessedBytes int64      `gorm:"not null;default:0" json:"processed_bytes"`
	ArchiveSize    int64      `gorm:"not null;default:0" json:"archive_size"`
	ErrorMessage   string     `gorm:"type:varchar(1000)" json:"error_message,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	ExpiresAt      *time.Time `gorm:"index" json:"expires_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (BatchDownloadJob) TableName() string { return "batch_download_jobs" }

func (job BatchDownloadJob) CompletedAtOrCreatedAt() time.Time {
	if job.CompletedAt != nil {
		return *job.CompletedAt
	}
	return job.CreatedAt
}

// BatchDownloadItem freezes the file version and archive path at task creation.
type BatchDownloadItem struct {
	ID           uint   `gorm:"primaryKey" json:"-"`
	JobID        string `gorm:"type:char(36);index;not null" json:"-"`
	NodeID       uint   `gorm:"index;not null" json:"node_id"`
	VersionID    uint   `gorm:"index;not null" json:"version_id"`
	RelativePath string `gorm:"type:varchar(1024);not null" json:"relative_path"`
	StorageKey   string `gorm:"type:varchar(512);not null" json:"-"`
	StorageClass string `gorm:"type:varchar(32);not null;default:'standard'" json:"storage_class"`
	Size         int64  `gorm:"not null" json:"size"`
	SHA256       string `gorm:"type:char(64);not null" json:"-"`
}

func (BatchDownloadItem) TableName() string { return "batch_download_items" }
