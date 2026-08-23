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

// Role 功能角色表
type Role struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID uint      `gorm:"not null;uniqueIndex:idx_workspace_role_code" json:"workspace_id"`
	Code        string    `gorm:"type:varchar(64);not null;uniqueIndex:idx_workspace_role_code" json:"code"`
	Name        string    `gorm:"type:varchar(64);not null" json:"name"`
	Description string    `gorm:"type:text" json:"description"`
	SortOrder   int       `gorm:"default:0" json:"sort_order"`
	Status      int       `gorm:"type:tinyint;default:1" json:"status"`
	CreatedBy   uint      `gorm:"index" json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (Role) TableName() string {
	return "roles"
}

// UserRole 用户功能角色绑定表
type UserRole struct {
	ID          uint `gorm:"primaryKey" json:"id"`
	WorkspaceID uint `gorm:"not null;uniqueIndex:idx_workspace_user_role" json:"workspace_id"`
	UserID      uint `gorm:"not null;uniqueIndex:idx_workspace_user_role" json:"user_id"`
	RoleID      uint `gorm:"not null;uniqueIndex:idx_workspace_user_role" json:"role_id"`
}

func (UserRole) TableName() string {
	return "user_roles"
}

type Permission struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	Code        string `gorm:"type:varchar(96);uniqueIndex;not null" json:"code"`
	Name        string `gorm:"type:varchar(128);not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
}

func (Permission) TableName() string {
	return "permissions"
}

type RolePermission struct {
	RoleID       uint `gorm:"primaryKey;autoIncrement:false" json:"role_id"`
	PermissionID uint `gorm:"primaryKey;autoIncrement:false" json:"permission_id"`
}

func (RolePermission) TableName() string {
	return "role_permissions"
}

// Menu defines a route/menu/button node shown by the frontend. Functional
// roles still bind stable permission codes; menu_permissions maps visible
// navigation and buttons to those permission codes.
type Menu struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"code"`
	ParentID  uint      `gorm:"default:0;index" json:"parent_id"`
	Name      string    `gorm:"type:varchar(64);not null" json:"name"`
	Path      string    `gorm:"type:varchar(255)" json:"path"`
	Component string    `gorm:"type:varchar(255)" json:"component"`
	Type      int8      `gorm:"type:tinyint;not null" json:"type"` // 0 directory, 1 menu, 2 button
	Icon      string    `gorm:"type:varchar(64)" json:"icon"`
	SortOrder int       `gorm:"default:0" json:"sort_order"`
	Hidden    bool      `gorm:"default:false" json:"hidden"`
	Status    int       `gorm:"type:tinyint;not null" json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Permissions []string `gorm:"-" json:"permissions"`
	Children    []Menu   `gorm:"-" json:"children,omitempty"`
}

func (Menu) TableName() string {
	return "menus"
}

type MenuPermission struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	MenuID         uint   `gorm:"not null;uniqueIndex:idx_menu_permission;index" json:"menu_id"`
	PermissionCode string `gorm:"type:varchar(96);not null;uniqueIndex:idx_menu_permission" json:"permission_code"`
}

func (MenuPermission) TableName() string {
	return "menu_permissions"
}
