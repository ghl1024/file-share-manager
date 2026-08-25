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

// Share is an externally accessible, immutable snapshot of a node.
// Token and password hashes are intentionally excluded from JSON responses.
type Share struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint       `gorm:"index;not null" json:"-"`
	SourceNodeID  uint       `gorm:"index;not null" json:"-"`
	PublicID      string     `gorm:"type:char(36);uniqueIndex;not null" json:"public_id"`
	TokenHash     string     `gorm:"type:char(64);uniqueIndex;not null" json:"-"`
	Name          string     `gorm:"type:varchar(255);not null" json:"name"`
	RootType      string     `gorm:"type:varchar(16);not null" json:"root_type"`
	RootName      string     `gorm:"type:varchar(255);not null" json:"root_name"`
	PasswordHash  string     `gorm:"type:varchar(255)" json:"-"`
	ExpiresAt     time.Time  `gorm:"index;not null" json:"expires_at"`
	MaxDownloads  *int       `json:"max_downloads"`
	DownloadCount int        `gorm:"not null;default:0" json:"download_count"`
	Status        string     `gorm:"type:varchar(16);index;not null;default:'active'" json:"status"`
	CreatedBy     uint       `gorm:"index;not null" json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
}

func (Share) TableName() string { return "shares" }

// ShareItem is the file version captured when a share is created.
// StorageKey and node identifiers remain server-side only.
type ShareItem struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	ShareID      uint      `gorm:"index;not null" json:"-"`
	PublicID     string    `gorm:"type:char(36);uniqueIndex;not null" json:"public_id"`
	RelativePath string    `gorm:"type:varchar(1024);not null" json:"relative_path"`
	Name         string    `gorm:"type:varchar(255);not null" json:"name"`
	VersionNo    int       `gorm:"not null" json:"version_no"`
	StorageKey   string    `gorm:"type:varchar(512);not null" json:"-"`
	StorageClass string    `gorm:"type:varchar(32);not null;default:'standard'" json:"storage_class"`
	Size         int64     `gorm:"not null" json:"size"`
	SHA256       string    `gorm:"type:char(64);not null" json:"-"`
	DetectedMime string    `gorm:"type:varchar(128)" json:"detected_mime"`
	ScanStatus   string    `gorm:"type:varchar(32);not null" json:"scan_status"`
	CreatedAt    time.Time `json:"created_at"`
}

func (ShareItem) TableName() string { return "share_items" }
