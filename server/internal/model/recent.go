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

// RecentNodeAccess stores user-facing navigation history independently from
// immutable audit events. Readability is always re-evaluated when listing it.
type RecentNodeAccess struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID    uint      `gorm:"not null;uniqueIndex:idx_recent_node_actor;index:idx_recent_node_user_time,priority:1" json:"workspace_id"`
	UserID         uint      `gorm:"not null;uniqueIndex:idx_recent_node_actor;index:idx_recent_node_user_time,priority:2" json:"user_id"`
	NodeID         uint      `gorm:"not null;uniqueIndex:idx_recent_node_actor;index" json:"node_id"`
	AccessCount    uint64    `gorm:"not null;default:1" json:"access_count"`
	LastAccessedAt time.Time `gorm:"not null;index:idx_recent_node_user_time,priority:3" json:"last_accessed_at"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (RecentNodeAccess) TableName() string { return "recent_node_accesses" }
