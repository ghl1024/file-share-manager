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

type BackupJob struct {
	ID              string     `gorm:"type:char(36);primaryKey" json:"id"`
	WorkspaceID     uint       `gorm:"index;not null" json:"workspace_id"`
	CreatedBy       uint       `gorm:"index;not null" json:"created_by"`
	Kind            string     `gorm:"type:varchar(16);index;not null" json:"kind"`
	Trigger         string     `gorm:"type:varchar(32);index;not null;default:''" json:"trigger,omitempty"`
	Status          string     `gorm:"type:varchar(16);index;not null" json:"status"`
	ParentID        string     `gorm:"type:char(36);index" json:"parent_id,omitempty"`
	CompactedFromID string     `gorm:"type:char(36);index" json:"compacted_from_id,omitempty"`
	ChangeLogStart  uint64     `gorm:"not null;default:0" json:"change_log_start"`
	ChangeLogEnd    uint64     `gorm:"not null;default:0" json:"change_log_end"`
	ManifestKey     string     `gorm:"type:varchar(512);not null" json:"manifest_key"`
	ObjectCount     int        `gorm:"not null;default:0" json:"object_count"`
	TotalBytes      int64      `gorm:"not null;default:0" json:"total_bytes"`
	VerifyStatus    string     `gorm:"type:varchar(16);not null;default:'unknown';index" json:"verify_status"`
	VerifiedAt      *time.Time `json:"verified_at,omitempty"`
	VerifyError     string     `gorm:"type:varchar(1000)" json:"verify_error,omitempty"`
	ErrorMessage    string     `gorm:"type:varchar(1000)" json:"error_message,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (BackupJob) TableName() string { return "backup_jobs" }

// BackupRestoreDrill records a non-destructive object restore rehearsal.
// Rehearsal files are written to an isolated staging directory and removed
// after checksum verification; this table keeps the durable execution result.
type BackupRestoreDrill struct {
	ID           string     `gorm:"type:char(36);primaryKey" json:"id"`
	WorkspaceID  uint       `gorm:"index;not null" json:"workspace_id"`
	BackupJobID  string     `gorm:"type:char(36);index;not null" json:"backup_job_id"`
	CreatedBy    uint       `gorm:"index;not null" json:"created_by"`
	Status       string     `gorm:"type:varchar(16);index;not null" json:"status"`
	ObjectCount  int        `gorm:"not null;default:0" json:"object_count"`
	TotalBytes   int64      `gorm:"not null;default:0" json:"total_bytes"`
	ErrorMessage string     `gorm:"type:varchar(1000)" json:"error_message,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (BackupRestoreDrill) TableName() string { return "backup_restore_drills" }
