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

// Node 目录与文件节点表
type Node struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	WorkspaceID     uint       `gorm:"index;not null" json:"workspace_id"`
	ParentID        *uint      `gorm:"index;comment:'父节点ID，NULL为根节点'" json:"parent_id"`
	Name            string     `gorm:"type:varchar(255);not null;index:idx_workspace_parent_name" json:"name"`
	NormalizedName  string     `gorm:"type:varchar(255);not null;index:idx_workspace_parent_name" json:"-"`
	Type            string     `gorm:"type:varchar(16);not null;comment:'folder, file'" json:"type"`
	ActiveVersion   *uint      `gorm:"comment:'最新版本ID，仅对file有效'" json:"active_version_id"`
	InheritMode     string     `gorm:"type:varchar(16);not null;default:'inherit';comment:'inherit, break'" json:"inherit_mode"`
	Status          string     `gorm:"type:varchar(16);default:'active';comment:'active, trashed'" json:"status"`
	TrashedAt       *time.Time `json:"trashed_at"`
	CreatedBy       uint       `gorm:"index" json:"created_by"`
	UpdatedBy       uint       `gorm:"index" json:"updated_by"`
	IsFavorite      bool       `gorm:"-" json:"is_favorite,omitempty"`
	StorageClass    string     `gorm:"-" json:"storage_class,omitempty"`
	ArchiveError    string     `gorm:"-" json:"archive_error,omitempty"`
	LastAccessedAt  *time.Time `gorm:"-" json:"last_accessed_at,omitempty"`
	SearchSize      *int64     `gorm:"->;column:search_size;-:migration" json:"size,omitempty"`
	SearchExtension string     `gorm:"->;column:search_extension;-:migration" json:"extension,omitempty"`
	SearchCreatedBy string     `gorm:"->;column:search_created_by;-:migration" json:"created_by_name,omitempty"`
	SearchUpdatedBy string     `gorm:"->;column:search_updated_by;-:migration" json:"updated_by_name,omitempty"`
	SearchRelevance float64    `gorm:"->;column:search_relevance;-:migration" json:"relevance,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

func (Node) TableName() string {
	return "nodes"
}

// NodeClosure 节点闭包表，用于快速查询目录层级
type NodeClosure struct {
	AncestorID   uint `gorm:"primaryKey;autoIncrement:false"`
	DescendantID uint `gorm:"primaryKey;autoIncrement:false"`
	Depth        int  `gorm:"not null;comment:'节点间的深度差，0表示自己'"`
}

func (NodeClosure) TableName() string {
	return "node_closures"
}

// NodeACL 节点权限表
type NodeACL struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	WorkspaceID       uint      `gorm:"not null;uniqueIndex:idx_node_acl_subject" json:"workspace_id"`
	NodeID            uint      `gorm:"not null;uniqueIndex:idx_node_acl_subject" json:"node_id"`
	SubjectType       string    `gorm:"type:varchar(16);not null;uniqueIndex:idx_node_acl_subject;comment:'user, group'" json:"subject_type"`
	SubjectID         uint      `gorm:"not null;uniqueIndex:idx_node_acl_subject" json:"subject_id"`
	Effect            string    `gorm:"type:varchar(16);default:'allow';comment:'allow, deny'" json:"effect"`
	AccessLevel       string    `gorm:"type:varchar(32);not null;comment:'read, read_write, admin'" json:"access_level"`
	InheritToChildren bool      `gorm:"default:true;comment:'是否向下继承'" json:"inherit_to_children"`
	CreatedBy         uint      `gorm:"index" json:"created_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (NodeACL) TableName() string {
	return "node_acls"
}
