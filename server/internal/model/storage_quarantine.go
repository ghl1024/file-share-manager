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

const (
	StorageQuarantineStatusQuarantined = "quarantined"
	StorageQuarantineStatusPurged      = "purged"
	StorageQuarantineStatusRestored    = "restored"
)

// StorageQuarantine persists the lifecycle of an orphaned primary-storage
// object after it has been moved out of the live object tree.
type StorageQuarantine struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint       `gorm:"not null;uniqueIndex:idx_quarantine_workspace_object;index" json:"workspace_id"`
	StorageKey    string     `gorm:"type:varchar(512);not null;uniqueIndex:idx_quarantine_workspace_object" json:"storage_key"`
	QuarantineKey string     `gorm:"type:varchar(768);not null" json:"quarantine_key"`
	Status        string     `gorm:"type:varchar(32);not null;index;default:'quarantined'" json:"status"`
	RetryCount    int        `gorm:"not null;default:0" json:"retry_count"`
	LastError     string     `gorm:"type:varchar(1000)" json:"last_error,omitempty"`
	QuarantinedAt time.Time  `gorm:"not null" json:"quarantined_at"`
	PurgeAfter    time.Time  `gorm:"not null;index" json:"purge_after"`
	PurgedAt      *time.Time `json:"purged_at,omitempty"`
	RestoredAt    *time.Time `json:"restored_at,omitempty"`
	CreatedBy     uint       `gorm:"index" json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (StorageQuarantine) TableName() string { return "storage_quarantines" }
