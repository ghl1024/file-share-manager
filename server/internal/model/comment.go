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

// NodeComment stores user-authored collaboration content for a file or folder.
// Access is always re-authorized against the current node ACL by the handler.
type NodeComment struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID uint      `gorm:"not null;index:idx_node_comment_feed,priority:1;index" json:"workspace_id"`
	NodeID      uint      `gorm:"not null;index:idx_node_comment_feed,priority:2;index" json:"node_id"`
	AuthorID    uint      `gorm:"not null;index" json:"author_id"`
	Content     string    `gorm:"type:varchar(4000);not null" json:"content"`
	Revision    uint      `gorm:"not null;default:1" json:"revision"`
	CreatedAt   time.Time `gorm:"index:idx_node_comment_feed,priority:3,sort:desc" json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (NodeComment) TableName() string { return "node_comments" }

// NodeCommentMention records resolved mentions only. Plain text that looks
// like an @mention is never treated as a notification recipient implicitly.
type NodeCommentMention struct {
	CommentID uint      `gorm:"primaryKey;autoIncrement:false" json:"comment_id"`
	UserID    uint      `gorm:"primaryKey;autoIncrement:false;index" json:"user_id"`
	CreatedAt time.Time `gorm:"not null" json:"created_at"`
}

func (NodeCommentMention) TableName() string { return "node_comment_mentions" }
