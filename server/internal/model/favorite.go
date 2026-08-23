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

// Favorite records a user's bookmark for an active node in a workspace.
// The unique key keeps repeated clicks idempotent.
type Favorite struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID uint      `gorm:"not null;uniqueIndex:idx_favorite_workspace_user_node" json:"workspace_id"`
	UserID      uint      `gorm:"not null;uniqueIndex:idx_favorite_workspace_user_node" json:"user_id"`
	NodeID      uint      `gorm:"not null;uniqueIndex:idx_favorite_workspace_user_node;index" json:"node_id"`
	CreatedAt   time.Time `json:"created_at"`
}

func (Favorite) TableName() string {
	return "favorites"
}
