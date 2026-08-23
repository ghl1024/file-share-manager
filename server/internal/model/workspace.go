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

// Workspace 工作空间表
type Workspace struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UUID          string    `gorm:"type:char(36);uniqueIndex;not null" json:"uuid"`
	Name          string    `gorm:"type:varchar(128);not null" json:"name"`
	Code          string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"`
	Description   string    `gorm:"type:text" json:"description"`
	Status        int       `gorm:"type:tinyint;default:1;comment:'0-禁用, 1-启用'" json:"status"`
	QuotaBytes    *int64    `gorm:"comment:'总容量配额(字节)，NULL表示无限制'" json:"quota_bytes"`
	UsedBytes     int64     `gorm:"default:0;comment:'已使用容量(字节)'" json:"used_bytes"`
	ReservedBytes int64     `gorm:"default:0;comment:'上传会话预留容量(字节)'" json:"reserved_bytes"`
	CreatedBy     uint      `gorm:"index" json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func (Workspace) TableName() string {
	return "workspaces"
}

// WorkspaceAccessView adds the caller's role without leaking membership data
// from other workspaces.
type WorkspaceAccessView struct {
	ID            uint      `json:"id"`
	UUID          string    `json:"uuid"`
	Name          string    `json:"name"`
	Code          string    `json:"code"`
	Description   string    `json:"description"`
	Status        int       `json:"status"`
	QuotaBytes    *int64    `json:"quota_bytes"`
	UsedBytes     int64     `json:"used_bytes"`
	ReservedBytes int64     `json:"reserved_bytes"`
	CreatedBy     uint      `json:"created_by"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CurrentRole   string    `json:"current_role"`
	IsMember      bool      `json:"is_member"`
}

// WorkspaceMembership 用户与工作空间关系表
type WorkspaceMembership struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID   uint      `gorm:"index;not null;uniqueIndex:idx_workspace_user" json:"workspace_id"`
	UserID        uint      `gorm:"index;not null;uniqueIndex:idx_workspace_user" json:"user_id"`
	Role          string    `gorm:"type:varchar(32);default:'member';comment:'workspace_admin, member'" json:"role"`
	QuotaBytes    *int64    `gorm:"comment:'个人在该工作空间的容量配额，NULL表示跟随工作空间或无限制'" json:"quota_bytes"`
	UsedBytes     int64     `gorm:"default:0;comment:'个人在该工作空间已使用容量'" json:"used_bytes"`
	ReservedBytes int64     `gorm:"default:0;comment:'个人上传会话预留容量(字节)'" json:"reserved_bytes"`
	JoinedAt      time.Time `gorm:"autoCreateTime" json:"joined_at"`
	CreatedBy     uint      `gorm:"index" json:"created_by"`
}

func (WorkspaceMembership) TableName() string {
	return "workspace_memberships"
}

// WorkspaceMemberView is the read model used by member management screens.
type WorkspaceMemberView struct {
	UserID        uint      `json:"user_id"`
	Username      string    `json:"username"`
	RealName      string    `json:"real_name"`
	Email         string    `json:"email"`
	Status        int       `json:"status"`
	Role          string    `json:"role"`
	QuotaBytes    *int64    `json:"quota_bytes"`
	UsedBytes     int64     `json:"used_bytes"`
	ReservedBytes int64     `json:"reserved_bytes"`
	JoinedAt      time.Time `json:"joined_at"`
}
