/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const recentNodeLimit = 100

type DirectSharedGrant struct {
	model.Node
	SourceType      string    `gorm:"column:source_type" json:"source_type"`
	SourceID        uint      `gorm:"column:source_id" json:"source_id"`
	SourceName      string    `gorm:"column:source_name" json:"source_name"`
	SourceDirectory string    `gorm:"column:source_directory" json:"source_directory"`
	SourceLDAPDN    string    `gorm:"column:source_ldap_dn" json:"-"`
	GrantedLevel    string    `gorm:"column:granted_level" json:"granted_level"`
	SharedAt        time.Time `gorm:"column:shared_at" json:"shared_at"`
	InheritChildren bool      `gorm:"column:inherit_children" json:"inherit_to_children"`
}

type RecentNodeItem struct {
	model.Node
	RecentAccessedAt time.Time `gorm:"column:recent_accessed_at" json:"recent_accessed_at"`
	AccessCount      uint64    `gorm:"column:recent_access_count" json:"access_count"`
}

type CollaborationDAO struct {
	db *gorm.DB
}

func NewCollaborationDAO() *CollaborationDAO { return &CollaborationDAO{db: database.DB} }

func (dao *CollaborationDAO) ListDirectSharedGrants(workspaceID, userID uint, groupIDs []uint) ([]DirectSharedGrant, error) {
	var grants []DirectSharedGrant
	subjects := dao.db.Where("node_acls.subject_type = ? AND node_acls.subject_id = ?", "user", userID)
	if len(groupIDs) > 0 {
		subjects = subjects.Or("node_acls.subject_type = ? AND node_acls.subject_id IN ?", "group", groupIDs)
	}
	err := dao.db.Table("node_acls").
		Select(`nodes.*, node_acls.subject_type AS source_type, node_acls.subject_id AS source_id,
			CASE WHEN node_acls.subject_type = 'user' THEN COALESCE(NULLIF(users.real_name, ''), users.username)
			ELSE user_groups.name END AS source_name,
			CASE WHEN node_acls.subject_type = 'user' THEN users.source ELSE user_groups.source END AS source_directory,
			COALESCE(user_groups.ldap_dn, '') AS source_ldap_dn,
			node_acls.access_level AS granted_level, node_acls.updated_at AS shared_at,
			node_acls.inherit_to_children AS inherit_children`).
		Joins("JOIN nodes ON nodes.id = node_acls.node_id AND nodes.workspace_id = node_acls.workspace_id").
		Joins("LEFT JOIN users ON node_acls.subject_type = 'user' AND users.id = node_acls.subject_id").
		Joins("LEFT JOIN user_groups ON node_acls.subject_type = 'group' AND user_groups.id = node_acls.subject_id AND user_groups.workspace_id = node_acls.workspace_id").
		Where("node_acls.workspace_id = ? AND node_acls.effect = ? AND nodes.status = ?", workspaceID, "allow", "active").
		Where(subjects).
		Order("node_acls.updated_at DESC, node_acls.id DESC").
		Find(&grants).Error
	return grants, err
}

func (dao *CollaborationDAO) TouchRecent(workspaceID, userID, nodeID uint) error {
	now := time.Now()
	return dao.db.Transaction(func(tx *gorm.DB) error {
		entry := model.RecentNodeAccess{
			WorkspaceID: workspaceID, UserID: userID, NodeID: nodeID,
			AccessCount: 1, LastAccessedAt: now,
		}
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "workspace_id"}, {Name: "user_id"}, {Name: "node_id"}},
			DoUpdates: clause.Assignments(map[string]any{
				"access_count":     gorm.Expr("access_count + 1"),
				"last_accessed_at": now,
				"updated_at":       now,
			}),
		}).Create(&entry).Error; err != nil {
			return err
		}
		var staleIDs []uint
		if err := tx.Model(&model.RecentNodeAccess{}).
			Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
			Order("last_accessed_at DESC, id DESC").Offset(recentNodeLimit).Limit(1000).
			Pluck("id", &staleIDs).Error; err != nil {
			return err
		}
		if len(staleIDs) > 0 {
			return tx.Where("id IN ?", staleIDs).Delete(&model.RecentNodeAccess{}).Error
		}
		return nil
	})
}

func (dao *CollaborationDAO) ListRecentNodes(workspaceID, userID uint) ([]RecentNodeItem, error) {
	var items []RecentNodeItem
	err := dao.db.Table("recent_node_accesses").
		Select("nodes.*, recent_node_accesses.last_accessed_at AS recent_accessed_at, recent_node_accesses.access_count AS recent_access_count").
		Joins("JOIN nodes ON nodes.id = recent_node_accesses.node_id AND nodes.workspace_id = recent_node_accesses.workspace_id").
		Where("recent_node_accesses.workspace_id = ? AND recent_node_accesses.user_id = ? AND nodes.status = ?", workspaceID, userID, "active").
		Order("recent_node_accesses.last_accessed_at DESC, recent_node_accesses.id DESC").
		Limit(recentNodeLimit).Find(&items).Error
	return items, err
}
