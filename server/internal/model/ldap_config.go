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

// LDAPConfig stores the single system-level LDAP/AD connection profile.
// The password is intentionally omitted from JSON responses.
type LDAPConfig struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	Host               string    `gorm:"type:varchar(255);not null" json:"host"`
	Port               int       `gorm:"not null;default:389" json:"port"`
	AdminDN            string    `gorm:"type:varchar(255);not null" json:"admin_dn"`
	Password           string    `gorm:"-" json:"-"`
	PasswordCiphertext string    `gorm:"column:password_ciphertext;type:varchar(1024);not null;default:''" json:"-"`
	Transport          string    `gorm:"type:varchar(16);not null;default:'starttls'" json:"transport"`
	TLSCA              string    `gorm:"type:text;not null;default:''" json:"-"`
	TLSServerName      string    `gorm:"type:varchar(255);not null;default:''" json:"tls_server_name"`
	TLSMinVersion      string    `gorm:"type:varchar(8);not null;default:'1.2'" json:"tls_min_version"`
	BaseDN             string    `gorm:"type:varchar(255);not null" json:"base_dn"`
	UserFilter         string    `gorm:"type:varchar(255);not null;default:'(&(objectClass=user)(sAMAccountName=*))'" json:"user_filter"`
	UsernameAttr       string    `gorm:"type:varchar(64);not null;default:'sAMAccountName'" json:"username_attr"`
	EmailAttr          string    `gorm:"type:varchar(64);not null;default:'mail'" json:"email_attr"`
	RealNameAttr       string    `gorm:"type:varchar(64);not null;default:'displayName'" json:"real_name_attr"`
	SyncCron           string    `gorm:"type:varchar(64);not null;default:'0 0 2 * * *'" json:"sync_cron"`
	SyncWorkspaceID    uint      `gorm:"index;default:0" json:"sync_workspace_id"`
	GroupSyncEnabled   int       `gorm:"type:tinyint;not null;default:0;comment:'0-禁用, 1-启用 LDAP 用户组同步'" json:"group_sync_enabled"`
	GroupBaseDN        string    `gorm:"type:varchar(255);not null;default:''" json:"group_base_dn"`
	GroupFilter        string    `gorm:"type:varchar(255);not null;default:'(objectClass=group)'" json:"group_filter"`
	GroupNameAttr      string    `gorm:"type:varchar(64);not null;default:'cn'" json:"group_name_attr"`
	GroupMemberAttr    string    `gorm:"type:varchar(64);not null;default:'member'" json:"group_member_attr"`
	Status             int       `gorm:"type:tinyint;not null;default:0;comment:'0-禁用, 1-启用'" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

func (LDAPConfig) TableName() string {
	return "ldap_configs"
}

// LDAPSyncHistory records every manual or scheduled LDAP user sync attempt.
type LDAPSyncHistory struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	SyncType          string     `gorm:"type:varchar(20);not null;index;comment:'auto, manual'" json:"sync_type"`
	Status            string     `gorm:"type:varchar(20);not null;index;comment:'running, success, failed'" json:"status"`
	StartTime         time.Time  `gorm:"not null;index" json:"start_time"`
	EndTime           *time.Time `json:"end_time,omitempty"`
	TotalUsers        int        `gorm:"default:0" json:"total_users"`
	SuccessCount      int        `gorm:"default:0;comment:'created users'" json:"success_count"`
	UpdateCount       int        `gorm:"default:0" json:"update_count"`
	SkipCount         int        `gorm:"default:0;comment:'skipped local collisions or invalid LDAP entries'" json:"skip_count"`
	TotalGroups       int        `gorm:"default:0" json:"total_groups"`
	GroupSuccessCount int        `gorm:"default:0;comment:'created ldap groups'" json:"group_success_count"`
	GroupUpdateCount  int        `gorm:"default:0;comment:'updated ldap groups or memberships'" json:"group_update_count"`
	GroupSkipCount    int        `gorm:"default:0;comment:'skipped ldap groups'" json:"group_skip_count"`
	ErrorMessage      string     `gorm:"type:text" json:"error_message"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

func (LDAPSyncHistory) TableName() string {
	return "ldap_sync_histories"
}
