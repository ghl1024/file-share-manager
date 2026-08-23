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

// ChangeLog is an append-only cursor for mutations that affect workspace
// recovery. It is written in the same transaction as the business mutation.
type ChangeLog struct {
	Seq         uint64    `gorm:"primaryKey;autoIncrement" json:"seq"`
	WorkspaceID uint      `gorm:"index:idx_change_log_workspace_seq,priority:1;not null" json:"workspace_id"`
	EntityType  string    `gorm:"type:varchar(64);index;not null" json:"entity_type"`
	EntityID    string    `gorm:"type:varchar(128);not null" json:"entity_id"`
	Operation   string    `gorm:"type:varchar(32);not null" json:"operation"`
	Payload     string    `gorm:"type:json;not null" json:"payload"`
	CreatedAt   time.Time `gorm:"index:idx_change_log_workspace_seq,priority:2" json:"created_at"`
}

func (ChangeLog) TableName() string { return "change_logs" }
