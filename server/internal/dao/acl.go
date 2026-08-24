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
	"database/sql"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type ACLDAO struct {
	db *gorm.DB
}

type EffectiveACLEntry struct {
	Depth       int
	SubjectType string
	SubjectID   uint
	Effect      string
	AccessLevel string
}

type EffectiveACLSource struct {
	Depth             int
	SourceNodeID      uint
	SourceNodeName    string
	SubjectType       string
	SubjectID         uint
	SubjectName       string
	SubjectSource     string
	SubjectLDAPDN     string
	Effect            string
	AccessLevel       string
	InheritToChildren bool
}

func NewACLDAO() *ACLDAO {
	return &ACLDAO{db: database.DB}
}

func (dao *ACLDAO) GrantPermission(acl *model.NodeACL) error {
	return dao.GrantPermissionWithAudit(acl, nil)
}

func (dao *ACLDAO) GrantPermissionWithAudit(acl *model.NodeACL, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.NodeACL
		tx.Where("workspace_id = ? AND node_id = ? AND subject_type = ? AND subject_id = ?", acl.WorkspaceID, acl.NodeID, acl.SubjectType, acl.SubjectID).First(&before)
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "workspace_id"}, {Name: "node_id"}, {Name: "subject_type"}, {Name: "subject_id"},
			},
			DoUpdates: clause.AssignmentColumns([]string{"effect", "access_level", "inherit_to_children", "created_by", "updated_at"}),
		}).Create(acl).Error; err != nil {
			return err
		}
		if err := incrementWorkspaceUsersAuthVersion(tx, acl.WorkspaceID); err != nil {
			return err
		}
		if err := appendChange(tx, acl.WorkspaceID, "node_acl", acl.NodeID, "upsert", acl); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, aclAuditBefore(before), aclAuditAfter(acl))
	})
}

func (dao *ACLDAO) RevokePermission(workspaceID, nodeID uint, subjectType string, subjectID uint) error {
	return dao.RevokePermissionWithAudit(workspaceID, nodeID, subjectType, subjectID, nil)
}

func (dao *ACLDAO) RevokePermissionWithAudit(workspaceID, nodeID uint, subjectType string, subjectID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.NodeACL
		tx.Where("workspace_id = ? AND node_id = ? AND subject_type = ? AND subject_id = ?", workspaceID, nodeID, subjectType, subjectID).First(&before)
		if err := tx.Where("workspace_id = ? AND node_id = ? AND subject_type = ? AND subject_id = ?", workspaceID, nodeID, subjectType, subjectID).
			Delete(&model.NodeACL{}).Error; err != nil {
			return err
		}
		if err := incrementWorkspaceUsersAuthVersion(tx, workspaceID); err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "node_acl", nodeID, "revoke", map[string]any{"subject_type": subjectType, "subject_id": subjectID}); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, aclAuditBefore(before), nil)
	})
}

func (dao *ACLDAO) ListDirectPermissions(workspaceID, nodeID uint) ([]model.NodeACL, error) {
	var entries []model.NodeACL
	err := dao.db.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).
		Order("subject_type ASC, subject_id ASC").Find(&entries).Error
	return entries, err
}

func (dao *ACLDAO) ListDirectPermissionsForSubject(workspaceID uint, subjectType string, subjectID uint) ([]model.NodeACL, error) {
	var entries []model.NodeACL
	err := dao.db.Where("workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceID, subjectType, subjectID).
		Order("node_id ASC, id ASC").Find(&entries).Error
	return entries, err
}

func (dao *ACLDAO) ReplaceDirectPermissions(workspaceID, nodeID, actorID uint, entries []model.NodeACL) error {
	return dao.ReplaceDirectPermissionsWithAudit(workspaceID, nodeID, actorID, entries, nil)
}

func (dao *ACLDAO) ReplaceDirectPermissionsWithAudit(workspaceID, nodeID, actorID uint, entries []model.NodeACL, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before []model.NodeACL
		if err := tx.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).
			Order("subject_type ASC, subject_id ASC").Find(&before).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID).Delete(&model.NodeACL{}).Error; err != nil {
			return err
		}
		now := time.Now()
		for i := range entries {
			entries[i].ID = 0
			entries[i].WorkspaceID = workspaceID
			entries[i].NodeID = nodeID
			entries[i].CreatedBy = actorID
			entries[i].CreatedAt = now
			entries[i].UpdatedAt = now
		}
		if len(entries) == 0 {
			if err := incrementWorkspaceUsersAuthVersion(tx, workspaceID); err != nil {
				return err
			}
			if err := appendChange(tx, workspaceID, "node_acl", nodeID, "replace", []model.NodeACL{}); err != nil {
				return err
			}
			return appendAuditEvent(tx, event, aclAuditList(before), []map[string]any{})
		}
		if err := tx.Create(&entries).Error; err != nil {
			return err
		}
		if err := incrementWorkspaceUsersAuthVersion(tx, workspaceID); err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "node_acl", nodeID, "replace", entries); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, aclAuditList(before), aclAuditList(entries))
	})
}

func aclAuditBefore(value model.NodeACL) any {
	if value.ID == 0 {
		return nil
	}
	return aclAuditEntry(value)
}

func aclAuditAfter(value *model.NodeACL) any {
	if value == nil {
		return nil
	}
	return aclAuditEntry(*value)
}

func aclAuditList(values []model.NodeACL) []map[string]any {
	result := make([]map[string]any, 0, len(values))
	for _, value := range values {
		result = append(result, aclAuditEntry(value))
	}
	return result
}

func aclAuditEntry(value model.NodeACL) map[string]any {
	return map[string]any{
		"id": value.ID, "workspace_id": value.WorkspaceID, "node_id": value.NodeID,
		"subject_type": value.SubjectType, "subject_id": value.SubjectID,
		"effect": value.Effect, "access_level": value.AccessLevel,
		"inherit_to_children": value.InheritToChildren, "created_by": value.CreatedBy,
	}
}

func (dao *ACLDAO) HasDirectAllowAdmin(workspaceID, nodeID uint) (bool, error) {
	var count int64
	err := dao.db.Model(&model.NodeACL{}).
		Where("workspace_id = ? AND node_id = ? AND effect = ? AND access_level = ?", workspaceID, nodeID, "allow", "admin").
		Count(&count).Error
	return count > 0, err
}

func (dao *ACLDAO) ListEffectiveEntries(workspaceID, nodeID, userID uint, groupIDs []uint) ([]EffectiveACLEntry, error) {
	var breakDepth sql.NullInt64
	row := dao.db.Table("node_closures").
		Select("MIN(node_closures.depth)").
		Joins("JOIN nodes ON nodes.id = node_closures.ancestor_id").
		Where("node_closures.descendant_id = ? AND nodes.workspace_id = ? AND nodes.inherit_mode = ?", nodeID, workspaceID, "break").
		Row()
	if err := row.Scan(&breakDepth); err != nil {
		return nil, err
	}

	var entries []EffectiveACLEntry
	query := dao.db.Table("node_acls").
		Select("node_closures.depth, node_acls.subject_type, node_acls.subject_id, node_acls.effect, node_acls.access_level").
		Joins("JOIN node_closures ON node_closures.ancestor_id = node_acls.node_id").
		Where("node_closures.descendant_id = ? AND node_acls.workspace_id = ?", nodeID, workspaceID).
		Where("(node_closures.depth = 0 OR node_acls.inherit_to_children = ?)", true)
	if breakDepth.Valid {
		query = query.Where("node_closures.depth <= ?", breakDepth.Int64)
	}
	if len(groupIDs) == 0 {
		query = query.Where("node_acls.subject_type = ? AND node_acls.subject_id = ?", "user", userID)
	} else {
		query = query.Where("(node_acls.subject_type = ? AND node_acls.subject_id = ?) OR (node_acls.subject_type = ? AND node_acls.subject_id IN ?)", "user", userID, "group", groupIDs)
	}
	if err := query.Order("node_closures.depth ASC").Find(&entries).Error; err != nil {
		return nil, err
	}
	return entries, nil
}

// ListEffectiveSources resolves the same inheritance window as authorization,
// while decorating only the current user's own subjects for a readable summary.
func (dao *ACLDAO) ListEffectiveSources(workspaceID, nodeID, userID uint, groupIDs []uint) ([]EffectiveACLSource, error) {
	var breakDepth sql.NullInt64
	row := dao.db.Table("node_closures").
		Select("MIN(node_closures.depth)").
		Joins("JOIN nodes ON nodes.id = node_closures.ancestor_id").
		Where("node_closures.descendant_id = ? AND nodes.workspace_id = ? AND nodes.inherit_mode = ?", nodeID, workspaceID, "break").
		Row()
	if err := row.Scan(&breakDepth); err != nil {
		return nil, err
	}

	var sources []EffectiveACLSource
	query := dao.db.Table("node_acls").
		Select(`node_closures.depth, node_acls.node_id AS source_node_id, source_nodes.name AS source_node_name,
			node_acls.subject_type, node_acls.subject_id,
			CASE WHEN node_acls.subject_type = 'user' THEN COALESCE(NULLIF(users.real_name, ''), users.username)
			ELSE user_groups.name END AS subject_name,
			CASE WHEN node_acls.subject_type = 'user' THEN users.source ELSE user_groups.source END AS subject_source,
			COALESCE(user_groups.ldap_dn, '') AS subject_ldap_dn,
			node_acls.effect, node_acls.access_level, node_acls.inherit_to_children`).
		Joins("JOIN node_closures ON node_closures.ancestor_id = node_acls.node_id").
		Joins("JOIN nodes AS source_nodes ON source_nodes.id = node_acls.node_id AND source_nodes.workspace_id = node_acls.workspace_id").
		Joins("LEFT JOIN users ON node_acls.subject_type = 'user' AND users.id = node_acls.subject_id").
		Joins("LEFT JOIN user_groups ON node_acls.subject_type = 'group' AND user_groups.id = node_acls.subject_id AND user_groups.workspace_id = node_acls.workspace_id").
		Where("node_closures.descendant_id = ? AND node_acls.workspace_id = ?", nodeID, workspaceID).
		Where("(node_closures.depth = 0 OR node_acls.inherit_to_children = ?)", true)
	if breakDepth.Valid {
		query = query.Where("node_closures.depth <= ?", breakDepth.Int64)
	}
	if len(groupIDs) == 0 {
		query = query.Where("node_acls.subject_type = ? AND node_acls.subject_id = ?", "user", userID)
	} else {
		query = query.Where("(node_acls.subject_type = ? AND node_acls.subject_id = ?) OR (node_acls.subject_type = ? AND node_acls.subject_id IN ?)", "user", userID, "group", groupIDs)
	}
	if err := query.Order("node_closures.depth ASC, node_acls.subject_type DESC, node_acls.subject_id ASC").Scan(&sources).Error; err != nil {
		return nil, err
	}
	return sources, nil
}

// ListEffectiveEntriesForNodes resolves the inheritance window for a result
// set in two queries. Search can then evaluate ACLs without issuing one query
// per candidate node.
func (dao *ACLDAO) ListEffectiveEntriesForNodes(workspaceID, userID uint, groupIDs, nodeIDs []uint) (map[uint][]EffectiveACLEntry, error) {
	result := make(map[uint][]EffectiveACLEntry, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return result, nil
	}

	type breakRow struct {
		TargetNodeID uint `gorm:"column:target_node_id"`
		BreakDepth   int  `gorm:"column:break_depth"`
	}
	var breaks []breakRow
	if err := dao.db.Table("node_closures").
		Select("node_closures.descendant_id AS target_node_id, MIN(node_closures.depth) AS break_depth").
		Joins("JOIN nodes AS ancestors ON ancestors.id = node_closures.ancestor_id").
		Where("node_closures.descendant_id IN ? AND ancestors.workspace_id = ? AND ancestors.inherit_mode = ?", nodeIDs, workspaceID, "break").
		Group("node_closures.descendant_id").Scan(&breaks).Error; err != nil {
		return nil, err
	}
	breakDepths := make(map[uint]int, len(breaks))
	for _, item := range breaks {
		breakDepths[item.TargetNodeID] = item.BreakDepth
	}

	type effectiveRow struct {
		TargetNodeID uint `gorm:"column:target_node_id"`
		EffectiveACLEntry
	}
	var rows []effectiveRow
	query := dao.db.Table("node_acls").
		Select("node_closures.descendant_id AS target_node_id, node_closures.depth, node_acls.subject_type, node_acls.subject_id, node_acls.effect, node_acls.access_level").
		Joins("JOIN node_closures ON node_closures.ancestor_id = node_acls.node_id").
		Where("node_closures.descendant_id IN ? AND node_acls.workspace_id = ?", nodeIDs, workspaceID).
		Where("(node_closures.depth = 0 OR node_acls.inherit_to_children = ?)", true)
	if len(groupIDs) == 0 {
		query = query.Where("node_acls.subject_type = ? AND node_acls.subject_id = ?", "user", userID)
	} else {
		query = query.Where("(node_acls.subject_type = ? AND node_acls.subject_id = ?) OR (node_acls.subject_type = ? AND node_acls.subject_id IN ?)", "user", userID, "group", groupIDs)
	}
	if err := query.Order("node_closures.descendant_id ASC, node_closures.depth ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		if breakDepth, ok := breakDepths[row.TargetNodeID]; ok && row.Depth > breakDepth {
			continue
		}
		result[row.TargetNodeID] = append(result[row.TargetNodeID], row.EffectiveACLEntry)
	}
	return result, nil
}
