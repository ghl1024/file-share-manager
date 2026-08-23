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

// FileVersion 文件版本实体表
type FileVersion struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	WorkspaceID       uint       `gorm:"index;not null;uniqueIndex:idx_file_version_no" json:"workspace_id"`
	NodeID            uint       `gorm:"index;not null;uniqueIndex:idx_file_version_no" json:"node_id"`
	VersionNo         int        `gorm:"not null;uniqueIndex:idx_file_version_no;comment:'版本号'" json:"version_no"`
	StorageKey        string     `gorm:"type:varchar(512);not null;comment:'底层存储对象路径'" json:"-"`
	StorageClass      string     `gorm:"type:varchar(32);default:'standard';comment:'standard, archive, glacier'" json:"storage_class"`
	ArchiveError      string     `gorm:"type:varchar(1000)" json:"archive_error,omitempty"`
	LastAccessedAt    *time.Time `gorm:"index" json:"last_accessed_at,omitempty"`
	Size              int64      `gorm:"not null" json:"size"`
	SHA256            string     `gorm:"type:char(64);not null" json:"sha256"`
	Extension         string     `gorm:"type:varchar(32)" json:"extension"`
	DetectedMime      string     `gorm:"type:varchar(128)" json:"detected_mime"`
	RiskLevel         string     `gorm:"type:varchar(32);default:'unknown'" json:"risk_level"`
	ScanStatus        string     `gorm:"type:varchar(32);default:'unscanned';index:idx_scan_retry_queue;comment:'clean, infected, pending_scan, scan_error, unscanned'" json:"scan_status"`
	ScanMessage       string     `gorm:"type:text" json:"scan_message"`
	ScanRetryCount    int        `gorm:"not null;default:0" json:"scan_retry_count"`
	ScanNextRetryAt   *time.Time `gorm:"index:idx_scan_retry_queue" json:"scan_next_retry_at,omitempty"`
	ScanLastAttemptAt *time.Time `json:"scan_last_attempt_at,omitempty"`
	Encrypted         bool       `gorm:"not null;default:false" json:"encrypted"`
	CreatedBy         uint       `gorm:"index" json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

func (FileVersion) TableName() string {
	return "file_versions"
}

// UploadSession 上传会话表
type UploadSession struct {
	ID             string  `gorm:"type:varchar(64);primaryKey;comment:'upload_token'"`
	WorkspaceID    uint    `gorm:"index;not null"`
	NodeID         *uint   `gorm:"index;comment:'若是新文件则为NULL，覆盖更新则记录NodeID'"`
	TargetParentID *uint   `gorm:"index;comment:'目标父目录ID'"`
	BaseVersionNo  *int    `gorm:"comment:'用于并发上传的乐观锁校验'"`
	DisplayName    string  `gorm:"type:varchar(255);not null"`
	TotalSize      int64   `gorm:"not null"`
	ChunkSize      int64   `gorm:"not null"`
	TotalChunks    int     `gorm:"not null"`
	ReceivedChunks string  `gorm:"type:text;comment:'已接收的块索引列表(JSON或位图)'"`
	ClientSHA256   *string `gorm:"type:char(64)"`
	Status         string  `gorm:"type:varchar(32);default:'initialized';comment:'initialized, uploading, merging, scanning, completed, failed, expired'"`
	ExpiresAt      time.Time
	CreatedBy      uint `gorm:"index"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (UploadSession) TableName() string {
	return "upload_sessions"
}
