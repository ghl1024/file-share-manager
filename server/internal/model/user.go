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

import (
	"time"
)

// User 用户表
type User struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	Username     string    `gorm:"type:varchar(64);uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"type:varchar(255);not null" json:"-"`
	RealName     string    `gorm:"type:varchar(64);not null" json:"real_name"`
	Email        string    `gorm:"type:varchar(128)" json:"email"`
	Phone        string    `gorm:"type:varchar(32)" json:"phone"`
	Status       int       `gorm:"type:tinyint;default:1;comment:'0-禁用, 1-启用'" json:"status"`
	Source       string    `gorm:"type:varchar(32);default:'local';comment:'local, ldap'" json:"source"`
	IsSuperAdmin bool      `gorm:"default:false;comment:'超级管理员标志'" json:"is_super_admin"`
	AuthVersion  uint64    `gorm:"default:1;comment:'认证版本号，用于快速失效JWT'" json:"-"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func (User) TableName() string {
	return "users"
}
