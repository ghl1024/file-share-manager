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
	"errors"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidMove = errors.New("node cannot be moved into itself or its descendant")
	ErrNodeState   = errors.New("node state does not allow this operation")
)

type NodeDAO struct {
	db *gorm.DB
}

type NodeSearchFilter struct {
	Keyword     string
	NodeType    string
	Extension   string
	CreatedBy   string
	UpdatedBy   string
	CreatedFrom *time.Time
	CreatedTo   *time.Time
	UpdatedFrom *time.Time
	UpdatedTo   *time.Time
	MinSize     *int64
	MaxSize     *int64
	Sort        string
}

func NewNodeDAO() *NodeDAO {
	return &NodeDAO{db: database.DB}
}

// CreateNode 创建节点，并维护闭包表
func (dao *NodeDAO) CreateNode(node *model.Node) error {
	return dao.CreateNodeWithAudit(node, nil)
}

func (dao *NodeDAO) CreateNodeWithAudit(node *model.Node, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		// 1. 创建节点本身
		if err := tx.Create(node).Error; err != nil {
			return err
		}

		// 2. 插入闭包表自身的关联 (Ancestor = ID, Descendant = ID, Depth = 0)
		selfClosure := model.NodeClosure{
			AncestorID:   node.ID,
			DescendantID: node.ID,
			Depth:        0,
		}
		if err := tx.Create(&selfClosure).Error; err != nil {
			return err
		}

		// 3. 如果有父节点，则继承父节点的所有祖先
		if node.ParentID != nil {
			// INSERT INTO node_closures (ancestor_id, descendant_id, depth)
			// SELECT ancestor_id, ?, depth + 1 FROM node_closures WHERE descendant_id = ?
			err := tx.Exec(`
				INSERT INTO node_closures (ancestor_id, descendant_id, depth)
				SELECT ancestor_id, ?, depth + 1 
				FROM node_closures 
				WHERE descendant_id = ?
			`, node.ID, *node.ParentID).Error
			if err != nil {
				return err
			}
		}
		if err := appendChange(tx, node.WorkspaceID, "node", node.ID, "create", node); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, node)
		return appendAuditEvent(tx, event, nil, nodeAuditSnapshot(node))
	})
}

func (dao *NodeDAO) NameExists(workspaceID uint, parentID *uint, normalizedName string, excludeNodeID *uint) (bool, error) {
	query := dao.db.Model(&model.Node{}).
		Where("workspace_id = ? AND normalized_name = ? AND status = ?", workspaceID, normalizedName, "active")
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if excludeNodeID != nil {
		query = query.Where("id <> ?", *excludeNodeID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (dao *NodeDAO) FindActiveByName(workspaceID uint, parentID *uint, normalizedName string) (*model.Node, error) {
	var node model.Node
	query := dao.db.Where("workspace_id = ? AND normalized_name = ? AND status = ?", workspaceID, normalizedName, "active")
	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.First(&node).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// GetByID 获取节点
func (dao *NodeDAO) GetByID(workspaceID, id uint) (*model.Node, error) {
	var node model.Node
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&node).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &node, nil
}

// ListActiveByIDs returns only active nodes from the requested workspace. It is
// intended for callers that need to evaluate a bounded, already-scoped set in
// bulk without treating deleted or trashed source nodes as readable.
func (dao *NodeDAO) ListActiveByIDs(workspaceID uint, ids []uint) ([]model.Node, error) {
	if len(ids) == 0 {
		return []model.Node{}, nil
	}
	var nodes []model.Node
	err := dao.db.Where("workspace_id = ? AND status = ? AND id IN ?", workspaceID, "active", ids).
		Find(&nodes).Error
	return nodes, err
}

// ListChildren 获取指定目录下的直系子节点
func (dao *NodeDAO) ListChildren(workspaceID uint, parentID *uint) ([]model.Node, error) {
	var nodes []model.Node
	query := dao.db.Where("workspace_id = ? AND status = ?", workspaceID, "active")

	if parentID == nil {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}

	err := query.Order("type ASC, normalized_name ASC, id ASC").Find(&nodes).Error
	return nodes, err
}

// ListAllDescendants 通过闭包表一次性查出某目录下的所有子孙节点
func (dao *NodeDAO) ListAllDescendants(workspaceID, ancestorID uint) ([]model.Node, error) {
	var nodes []model.Node
	err := dao.db.Select("nodes.*").
		Joins("JOIN node_closures ON node_closures.descendant_id = nodes.id").
		Where("nodes.workspace_id = ? AND node_closures.ancestor_id = ? AND node_closures.depth > 0", workspaceID, ancestorID).
		Find(&nodes).Error
	return nodes, err
}

// ListAncestors returns active ancestors from the workspace root down to the
// direct parent. It intentionally excludes the node itself.
func (dao *NodeDAO) ListAncestors(workspaceID, descendantID uint) ([]model.Node, error) {
	var nodes []model.Node
	err := dao.db.Select("nodes.*").
		Joins("JOIN node_closures ON node_closures.ancestor_id = nodes.id").
		Where("nodes.workspace_id = ? AND nodes.status = ? AND node_closures.descendant_id = ? AND node_closures.depth > 0", workspaceID, "active", descendantID).
		Order("node_closures.depth DESC").Find(&nodes).Error
	return nodes, err
}

func (dao *NodeDAO) UpdateInheritMode(workspaceID, nodeID uint, mode string) error {
	return dao.UpdateInheritModeWithAudit(workspaceID, nodeID, mode, nil)
}

func (dao *NodeDAO) UpdateInheritModeWithAudit(workspaceID, nodeID uint, mode string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, nodeID, "folder", "active").
			First(&node).Error; err != nil {
			return err
		}
		before := nodeAuditSnapshot(&node)
		result := tx.Model(&model.Node{}).
			Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, nodeID, "folder", "active").
			Update("inherit_mode", mode)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := incrementWorkspaceUsersAuthVersion(tx, workspaceID); err != nil {
			return err
		}
		node.InheritMode = mode
		if err := appendChange(tx, workspaceID, "node", nodeID, "update_inherit_mode", map[string]any{"inherit_mode": mode}); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, &node)
		return appendAuditEvent(tx, event, before, nodeAuditSnapshot(&node))
	})
}

func (dao *NodeDAO) Rename(workspaceID, nodeID, actorID uint, name, normalizedName string) error {
	return dao.RenameWithAudit(workspaceID, nodeID, actorID, name, normalizedName, nil)
}

func (dao *NodeDAO) RenameWithAudit(workspaceID, nodeID, actorID uint, name, normalizedName string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ? AND status = ?", workspaceID, nodeID, "active").First(&node).Error; err != nil {
			return err
		}
		before := nodeAuditSnapshot(&node)
		if err := tx.Model(&node).Updates(map[string]any{"name": name, "normalized_name": normalizedName, "updated_by": actorID}).Error; err != nil {
			return err
		}
		node.Name, node.NormalizedName, node.UpdatedBy = name, normalizedName, actorID
		if err := appendChange(tx, workspaceID, "node", nodeID, "rename", map[string]any{"name": name, "updated_by": actorID}); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, &node)
		return appendAuditEvent(tx, event, before, nodeAuditSnapshot(&node))
	})
}

func (dao *NodeDAO) Move(workspaceID, nodeID, actorID uint, newParentID *uint) error {
	return dao.MoveWithAudit(workspaceID, nodeID, actorID, newParentID, nil)
}

func (dao *NodeDAO) MoveWithAudit(workspaceID, nodeID, actorID uint, newParentID *uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ?", workspaceID, nodeID).First(&node).Error; err != nil {
			return err
		}
		if node.Status != "active" {
			return ErrNodeState
		}
		before := nodeAuditSnapshot(&node)
		if newParentID != nil {
			if *newParentID == nodeID {
				return ErrInvalidMove
			}
			var parent model.Node
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, *newParentID, "folder", "active").First(&parent).Error; err != nil {
				return err
			}
			var descendantCount int64
			if err := tx.Model(&model.NodeClosure{}).Where("ancestor_id = ? AND descendant_id = ?", nodeID, *newParentID).Count(&descendantCount).Error; err != nil {
				return err
			}
			if descendantCount > 0 {
				return ErrInvalidMove
			}
		}
		if sameUintPointer(node.ParentID, newParentID) {
			return nil
		}
		if err := tx.Exec(`
			DELETE paths FROM node_closures AS paths
			JOIN node_closures AS subtree
			  ON subtree.ancestor_id = ? AND subtree.descendant_id = paths.descendant_id
			JOIN node_closures AS ancestors
			  ON ancestors.descendant_id = ? AND ancestors.ancestor_id = paths.ancestor_id
			WHERE paths.ancestor_id <> ?
		`, nodeID, nodeID, nodeID).Error; err != nil {
			return err
		}
		if newParentID != nil {
			if err := tx.Exec(`
				INSERT INTO node_closures (ancestor_id, descendant_id, depth)
				SELECT supertree.ancestor_id, subtree.descendant_id,
				       supertree.depth + subtree.depth + 1
				FROM node_closures AS supertree
				JOIN node_closures AS subtree ON subtree.ancestor_id = ?
				WHERE supertree.descendant_id = ?
			`, nodeID, *newParentID).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&node).Updates(map[string]any{"parent_id": newParentID, "updated_by": actorID}).Error; err != nil {
			return err
		}
		node.ParentID, node.UpdatedBy = newParentID, actorID
		if err := appendChange(tx, workspaceID, "node", nodeID, "move", map[string]any{"parent_id": newParentID, "updated_by": actorID}); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, &node)
		return appendAuditEvent(tx, event, before, nodeAuditSnapshot(&node))
	})
}

func (dao *NodeDAO) TrashSubtree(workspaceID, nodeID, actorID uint) error {
	return dao.TrashSubtreeWithAudit(workspaceID, nodeID, actorID, nil)
}

func (dao *NodeDAO) TrashSubtreeWithAudit(workspaceID, nodeID, actorID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ?", workspaceID, nodeID).First(&node).Error; err != nil {
			return err
		}
		before := nodeAuditSnapshot(&node)
		result := tx.Model(&model.Node{}).
			Where("workspace_id = ? AND id IN (?)", workspaceID,
				tx.Model(&model.NodeClosure{}).Select("descendant_id").Where("ancestor_id = ?", nodeID)).
			Where("status = ?", "active").
			Updates(map[string]any{"status": "trashed", "trashed_at": now, "updated_by": actorID})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		node.Status, node.TrashedAt, node.UpdatedBy = "trashed", &now, actorID
		if err := appendChange(tx, workspaceID, "node", nodeID, "trash_subtree", map[string]any{"updated_by": actorID}); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, &node)
		return appendAuditEvent(tx, event, before, map[string]any{"node": nodeAuditSnapshot(&node), "affected_nodes": result.RowsAffected})
	})
}

func (dao *NodeDAO) RestoreSubtree(workspaceID, nodeID, actorID uint) error {
	return dao.RestoreSubtreeWithAudit(workspaceID, nodeID, actorID, nil)
}

func (dao *NodeDAO) RestoreSubtreeWithAudit(workspaceID, nodeID, actorID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var node model.Node
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND id = ?", workspaceID, nodeID).First(&node).Error; err != nil {
			return err
		}
		if node.Status != "trashed" {
			return ErrNodeState
		}
		before := nodeAuditSnapshot(&node)
		if node.ParentID != nil {
			var parent model.Node
			if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, *node.ParentID).First(&parent).Error; err != nil {
				return err
			}
			if parent.Status != "active" {
				return ErrNodeState
			}
		}
		query := tx.Model(&model.Node{}).Where("workspace_id = ? AND normalized_name = ? AND status = ? AND id <> ?", workspaceID, node.NormalizedName, "active", nodeID)
		if node.ParentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *node.ParentID)
		}
		var conflictCount int64
		if err := query.Count(&conflictCount).Error; err != nil {
			return err
		}
		if conflictCount > 0 {
			return gorm.ErrDuplicatedKey
		}
		if err := tx.Model(&model.Node{}).
			Where("workspace_id = ? AND id IN (?)", workspaceID,
				tx.Model(&model.NodeClosure{}).Select("descendant_id").Where("ancestor_id = ?", nodeID)).
			Where("status = ?", "trashed").
			Updates(map[string]any{"status": "active", "trashed_at": nil, "updated_by": actorID}).Error; err != nil {
			return err
		}
		node.Status, node.TrashedAt, node.UpdatedBy = "active", nil, actorID
		if err := appendChange(tx, workspaceID, "node", nodeID, "restore_subtree", map[string]any{"updated_by": actorID}); err != nil {
			return err
		}
		prepareNodeAuditEvent(event, &node)
		return appendAuditEvent(tx, event, before, nodeAuditSnapshot(&node))
	})
}

func prepareNodeAuditEvent(event *model.OperationLog, node *model.Node) {
	if event == nil || node == nil {
		return
	}
	event.NodeID = &node.ID
	if event.TargetType == "" {
		event.TargetType = "node"
	}
	if event.TargetID == "" || event.TargetID == "0" {
		event.TargetID = strconv.FormatUint(uint64(node.ID), 10)
	}
	if event.TargetName == "" {
		event.TargetName = node.Name
	}
}

func nodeAuditSnapshot(node *model.Node) map[string]any {
	if node == nil {
		return nil
	}
	return map[string]any{
		"id": node.ID, "workspace_id": node.WorkspaceID, "parent_id": node.ParentID,
		"name": node.Name, "type": node.Type, "active_version_id": node.ActiveVersion,
		"inherit_mode": node.InheritMode, "status": node.Status, "trashed_at": node.TrashedAt,
		"created_by": node.CreatedBy, "updated_by": node.UpdatedBy,
	}
}

func (dao *NodeDAO) ListTrashRoots(workspaceID uint) ([]model.Node, error) {
	var nodes []model.Node
	err := dao.db.Model(&model.Node{}).
		Where("nodes.workspace_id = ? AND nodes.status = ?", workspaceID, "trashed").
		Where("nodes.parent_id IS NULL OR NOT EXISTS (SELECT 1 FROM nodes AS parent WHERE parent.id = nodes.parent_id AND parent.status = ?)", "trashed").
		Order("nodes.trashed_at DESC, nodes.id DESC").Find(&nodes).Error
	return nodes, err
}

type TrashPurgeResult struct {
	Versions        []model.FileVersion
	UploadIDs       []string
	BatchArchiveIDs []string
}

// PurgeExpiredTrash permanently removes eligible trash subtrees and returns
// storage artifacts that must be removed after the transaction commits.
// Active shares and batch-download snapshots keep their source subtree alive.
func (dao *NodeDAO) PurgeExpiredTrash(cutoff, now time.Time) (TrashPurgeResult, error) {
	result := TrashPurgeResult{
		Versions: []model.FileVersion{}, UploadIDs: []string{}, BatchArchiveIDs: []string{},
	}
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var roots []model.Node
		if err := tx.Where("status = ? AND trashed_at IS NOT NULL AND trashed_at <= ?", "trashed", cutoff).
			Where("parent_id IS NULL OR NOT EXISTS (SELECT 1 FROM nodes AS parent WHERE parent.id = nodes.parent_id AND parent.status = ?)", "trashed").
			Find(&roots).Error; err != nil {
			return err
		}
		for _, root := range roots {
			var nodeIDs []uint
			if err := tx.Table("node_closures").
				Select("node_closures.descendant_id").
				Joins("JOIN nodes ON nodes.id = node_closures.descendant_id").
				Where("node_closures.ancestor_id = ? AND nodes.workspace_id = ?", root.ID, root.WorkspaceID).
				Order("node_closures.depth DESC, node_closures.descendant_id ASC").
				Pluck("node_closures.descendant_id", &nodeIDs).Error; err != nil {
				return err
			}
			if len(nodeIDs) == 0 {
				nodeIDs = []uint{root.ID}
			}
			var versions []model.FileVersion
			if err := tx.Where("workspace_id = ? AND node_id IN ?", root.WorkspaceID, nodeIDs).Find(&versions).Error; err != nil {
				return err
			}
			keys := make([]string, 0, len(versions))
			sizeByUser := make(map[uint]int64)
			var totalSize int64
			for _, version := range versions {
				keys = append(keys, version.StorageKey)
				totalSize += version.Size
				sizeByUser[version.CreatedBy] += version.Size
			}
			var retainedShares int64
			if err := tx.Model(&model.Share{}).
				Where("workspace_id = ? AND source_node_id IN ?", root.WorkspaceID, nodeIDs).
				Where("status = ? AND expires_at > ? AND (max_downloads IS NULL OR download_count < max_downloads)", "active", now).
				Count(&retainedShares).Error; err != nil {
				return err
			}
			var retainedObjects []string
			if len(keys) > 0 {
				if err := tx.Table("share_items").
					Select("share_items.storage_key").
					Joins("JOIN shares ON shares.id = share_items.share_id").
					Where("shares.workspace_id = ? AND share_items.storage_key IN ?", root.WorkspaceID, keys).
					Where("shares.status = ? AND shares.expires_at > ? AND (shares.max_downloads IS NULL OR shares.download_count < shares.max_downloads)", "active", now).
					Pluck("share_items.storage_key", &retainedObjects).Error; err != nil {
					return err
				}
			}
			var retainedBatchJobs int64
			if err := tx.Table("batch_download_items AS items").
				Joins("JOIN batch_download_jobs AS jobs ON jobs.id = items.job_id").
				Where("jobs.workspace_id = ? AND items.node_id IN ?", root.WorkspaceID, nodeIDs).
				Where("jobs.status IN ? OR (jobs.status = ? AND (jobs.expires_at IS NULL OR jobs.expires_at > ?))", []string{"queued", "running", "failed"}, "completed", now).
				Count(&retainedBatchJobs).Error; err != nil {
				return err
			}
			if retainedShares > 0 || len(retainedObjects) > 0 || retainedBatchJobs > 0 {
				continue
			}

			uploadIDs, err := purgeNodeUploadSessions(tx, root.WorkspaceID, nodeIDs)
			if err != nil {
				return err
			}
			result.UploadIDs = append(result.UploadIDs, uploadIDs...)
			batchArchiveIDs, err := purgeNodeBatchDownloads(tx, root.WorkspaceID, nodeIDs, now)
			if err != nil {
				return err
			}
			result.BatchArchiveIDs = append(result.BatchArchiveIDs, batchArchiveIDs...)
			if err := purgeNodeShares(tx, root.WorkspaceID, nodeIDs, keys, now); err != nil {
				return err
			}
			if err := purgeNodeComments(tx, root.WorkspaceID, nodeIDs); err != nil {
				return err
			}
			if err := tx.Where("workspace_id = ? AND node_id IN ?", root.WorkspaceID, nodeIDs).Delete(&model.NodeACL{}).Error; err != nil {
				return err
			}
			if err := tx.Where("workspace_id = ? AND node_id IN ?", root.WorkspaceID, nodeIDs).Delete(&model.RecentNodeAccess{}).Error; err != nil {
				return err
			}
			result.Versions = append(result.Versions, versions...)
			if err := tx.Where("workspace_id = ? AND node_id IN ?", root.WorkspaceID, nodeIDs).Delete(&model.FileVersion{}).Error; err != nil {
				return err
			}
			if err := tx.Where("workspace_id = ? AND node_id IN ?", root.WorkspaceID, nodeIDs).Delete(&model.Favorite{}).Error; err != nil {
				return err
			}
			if err := tx.Where("ancestor_id IN ? OR descendant_id IN ?", nodeIDs, nodeIDs).Delete(&model.NodeClosure{}).Error; err != nil {
				return err
			}
			if err := tx.Where("workspace_id = ? AND id IN ?", root.WorkspaceID, nodeIDs).Delete(&model.Node{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Workspace{}).Where("id = ?", root.WorkspaceID).
				UpdateColumn("used_bytes", gorm.Expr("GREATEST(used_bytes - ?, 0)", totalSize)).Error; err != nil {
				return err
			}
			for userID, size := range sizeByUser {
				if err := tx.Model(&model.WorkspaceMembership{}).
					Where("workspace_id = ? AND user_id = ?", root.WorkspaceID, userID).
					UpdateColumn("used_bytes", gorm.Expr("GREATEST(used_bytes - ?, 0)", size)).Error; err != nil {
					return err
				}
			}
			if err := appendChange(tx, root.WorkspaceID, "node", root.ID, "purge_subtree", map[string]any{
				"node_ids": nodeIDs, "upload_ids": uploadIDs, "batch_archive_ids": batchArchiveIDs,
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func purgeNodeUploadSessions(tx *gorm.DB, workspaceID uint, nodeIDs []uint) ([]string, error) {
	var sessions []model.UploadSession
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workspace_id = ? AND (node_id IN ? OR target_parent_id IN ?)", workspaceID, nodeIDs, nodeIDs).
		Find(&sessions).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(sessions))
	for _, session := range sessions {
		ids = append(ids, session.ID)
		if session.Status == "completed" || session.Status == "expired" {
			continue
		}
		if err := releaseQuota(tx, workspaceID, session.CreatedBy, session.TotalSize); err != nil {
			return nil, err
		}
	}
	if len(ids) > 0 {
		if err := tx.Where("workspace_id = ? AND id IN ?", workspaceID, ids).Delete(&model.UploadSession{}).Error; err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func purgeNodeBatchDownloads(tx *gorm.DB, workspaceID uint, nodeIDs []uint, now time.Time) ([]string, error) {
	var jobIDs []string
	if err := tx.Table("batch_download_items AS items").
		Distinct("items.job_id").
		Joins("JOIN batch_download_jobs AS jobs ON jobs.id = items.job_id").
		Where("jobs.workspace_id = ? AND items.node_id IN ?", workspaceID, nodeIDs).
		Pluck("items.job_id", &jobIDs).Error; err != nil {
		return nil, err
	}
	if len(jobIDs) == 0 {
		return []string{}, nil
	}
	if err := tx.Where("job_id IN ?", jobIDs).Delete(&model.BatchDownloadItem{}).Error; err != nil {
		return nil, err
	}
	if err := tx.Model(&model.BatchDownloadJob{}).
		Where("workspace_id = ? AND id IN ?", workspaceID, jobIDs).
		Updates(map[string]any{"status": "expired", "archive_size": 0, "updated_at": now}).Error; err != nil {
		return nil, err
	}
	return jobIDs, nil
}

func purgeNodeShares(tx *gorm.DB, workspaceID uint, nodeIDs []uint, storageKeys []string, now time.Time) error {
	obsolete := tx.Model(&model.Share{}).
		Select("id").
		Where("workspace_id = ?", workspaceID).
		Where("status <> ? OR expires_at <= ? OR (max_downloads IS NOT NULL AND download_count >= max_downloads)", "active", now)
	if len(storageKeys) > 0 {
		if err := tx.Where("share_id IN (?) AND storage_key IN ?", obsolete, storageKeys).Delete(&model.ShareItem{}).Error; err != nil {
			return err
		}
	}
	var sourceShareIDs []uint
	if err := obsolete.Where("source_node_id IN ?", nodeIDs).Pluck("id", &sourceShareIDs).Error; err != nil {
		return err
	}
	if len(sourceShareIDs) == 0 {
		return nil
	}
	if err := tx.Where("share_id IN ?", sourceShareIDs).Delete(&model.ShareItem{}).Error; err != nil {
		return err
	}
	return tx.Where("workspace_id = ? AND id IN ?", workspaceID, sourceShareIDs).Delete(&model.Share{}).Error
}

func purgeNodeComments(tx *gorm.DB, workspaceID uint, nodeIDs []uint) error {
	var commentIDs []uint
	if err := tx.Model(&model.NodeComment{}).
		Where("workspace_id = ? AND node_id IN ?", workspaceID, nodeIDs).
		Pluck("id", &commentIDs).Error; err != nil {
		return err
	}
	if len(commentIDs) > 0 {
		if err := tx.Where("comment_id IN ?", commentIDs).Delete(&model.NodeCommentMention{}).Error; err != nil {
			return err
		}
	}
	return tx.Where("workspace_id = ? AND node_id IN ?", workspaceID, nodeIDs).Delete(&model.NodeComment{}).Error
}

func (dao *NodeDAO) SearchActive(workspaceID uint, filter NodeSearchFilter) ([]model.Node, error) {
	var nodes []model.Node
	selectClause := `nodes.*,
		active_versions.size AS search_size,
		active_versions.extension AS search_extension,
		COALESCE(NULLIF(creators.real_name, ''), creators.username, '') AS search_created_by,
		COALESCE(NULLIF(updaters.real_name, ''), updaters.username, '') AS search_updated_by,
		0 AS search_relevance`
	query := dao.db.Model(&model.Node{}).
		Joins("LEFT JOIN file_versions AS active_versions ON active_versions.id = nodes.active_version AND active_versions.workspace_id = nodes.workspace_id").
		Joins("LEFT JOIN users AS creators ON creators.id = nodes.created_by").
		Joins("LEFT JOIN users AS updaters ON updaters.id = nodes.updated_by").
		Where("nodes.workspace_id = ? AND nodes.status = ?", workspaceID, "active")

	if filter.Keyword != "" {
		if len([]rune(filter.Keyword)) == 1 {
			query = query.Where("nodes.normalized_name LIKE ? ESCAPE '\\\\'", escapeSearchLike(filter.Keyword)+"%")
		} else {
			selectClause = strings.TrimSuffix(selectClause, "0 AS search_relevance") +
				"MATCH(nodes.normalized_name) AGAINST (? IN NATURAL LANGUAGE MODE) AS search_relevance"
			query = query.Select(selectClause, filter.Keyword).
				Where("MATCH(nodes.normalized_name) AGAINST (? IN NATURAL LANGUAGE MODE) > 0", filter.Keyword).
				Where("nodes.normalized_name LIKE ? ESCAPE '\\\\'", "%"+escapeSearchLike(filter.Keyword)+"%")
		}
	}
	if filter.Keyword == "" || len([]rune(filter.Keyword)) == 1 {
		query = query.Select(selectClause)
	}
	if filter.NodeType != "" {
		query = query.Where("nodes.type = ?", filter.NodeType)
	}
	if filter.Extension != "" {
		query = query.Where("nodes.type = ? AND active_versions.extension = ?", "file", filter.Extension)
	}
	if filter.CreatedBy != "" {
		prefix := escapeSearchLike(filter.CreatedBy) + "%"
		query = query.Where("(creators.username LIKE ? ESCAPE '\\\\' OR creators.real_name LIKE ? ESCAPE '\\\\')", prefix, prefix)
	}
	if filter.UpdatedBy != "" {
		prefix := escapeSearchLike(filter.UpdatedBy) + "%"
		query = query.Where("(updaters.username LIKE ? ESCAPE '\\\\' OR updaters.real_name LIKE ? ESCAPE '\\\\')", prefix, prefix)
	}
	if filter.CreatedFrom != nil {
		query = query.Where("nodes.created_at >= ?", *filter.CreatedFrom)
	}
	if filter.CreatedTo != nil {
		query = query.Where("nodes.created_at < ?", *filter.CreatedTo)
	}
	if filter.UpdatedFrom != nil {
		query = query.Where("nodes.updated_at >= ?", *filter.UpdatedFrom)
	}
	if filter.UpdatedTo != nil {
		query = query.Where("nodes.updated_at < ?", *filter.UpdatedTo)
	}
	if filter.MinSize != nil {
		query = query.Where("nodes.type = ? AND active_versions.size >= ?", "file", *filter.MinSize)
	}
	if filter.MaxSize != nil {
		query = query.Where("nodes.type = ? AND active_versions.size <= ?", "file", *filter.MaxSize)
	}

	switch filter.Sort {
	case "name_asc":
		query = query.Order("nodes.normalized_name ASC, nodes.id ASC")
	case "created_desc":
		query = query.Order("nodes.created_at DESC, nodes.id DESC")
	case "size_asc":
		query = query.Order("active_versions.size ASC, nodes.id ASC")
	case "size_desc":
		query = query.Order("active_versions.size DESC, nodes.id DESC")
	case "updated_desc":
		query = query.Order("nodes.updated_at DESC, nodes.id DESC")
	default:
		if filter.Keyword != "" && len([]rune(filter.Keyword)) > 1 {
			query = query.Order("search_relevance DESC, nodes.updated_at DESC, nodes.id DESC")
		} else {
			query = query.Order("nodes.updated_at DESC, nodes.id DESC")
		}
	}
	err := query.Find(&nodes).Error
	return nodes, err
}

func escapeSearchLike(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "%", "\\%")
	return strings.ReplaceAll(value, "_", "\\_")
}

func (dao *NodeDAO) ListActiveFolders(workspaceID uint) ([]model.Node, error) {
	var nodes []model.Node
	err := dao.db.Where("workspace_id = ? AND type = ? AND status = ?", workspaceID, "folder", "active").
		Order("normalized_name ASC, id ASC").Find(&nodes).Error
	return nodes, err
}

func sameUintPointer(left, right *uint) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
