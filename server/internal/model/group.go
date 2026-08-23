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

// UserGroup 用户组（组织架构/部门）
type UserGroup struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID uint      `gorm:"not null;uniqueIndex:idx_workspace_group_name" json:"workspace_id"`
	Name        string    `gorm:"type:varchar(128);not null;uniqueIndex:idx_workspace_group_name" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	Source      string    `gorm:"type:varchar(32);not null;default:'local';index" json:"source"`
	LDAPDN      string    `gorm:"column:ldap_dn;type:varchar(512);not null;default:'';index" json:"ldap_dn"`
	CreatedBy   uint      `gorm:"index" json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (UserGroup) TableName() string {
	return "user_groups"
}

// UserGroupMember 用户组与成员关系表
type UserGroupMember struct {
	GroupID  uint      `gorm:"primaryKey;autoIncrement:false" json:"group_id"`
	UserID   uint      `gorm:"primaryKey;autoIncrement:false" json:"user_id"`
	JoinedAt time.Time `gorm:"autoCreateTime" json:"joined_at"`
}

func (UserGroupMember) TableName() string {
	return "user_group_members"
}

type GroupMemberView struct {
	UserID   uint      `json:"user_id"`
	Username string    `json:"username"`
	RealName string    `json:"real_name"`
	Email    string    `json:"email"`
	Status   int       `json:"status"`
	JoinedAt time.Time `json:"joined_at"`
}
