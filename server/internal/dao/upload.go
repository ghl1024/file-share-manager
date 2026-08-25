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
	"encoding/json"
	"errors"
	"sort"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUploadNotFound   = errors.New("upload session not found")
	ErrUploadState      = errors.New("upload session is not writable")
	ErrVersionConflict  = errors.New("concurrent update detected: version mismatch")
	ErrUploadIncomplete = errors.New("upload is missing one or more parts")
	ErrQuotaExceeded    = errors.New("storage quota exceeded")
)

type UploadDAO struct {
	db *gorm.DB
}

func NewUploadDAO() *UploadDAO {
	return &UploadDAO{db: database.DB}
}

func (dao *UploadDAO) CreateSession(session *model.UploadSession) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := reserveQuota(tx, session.WorkspaceID, session.CreatedBy, session.TotalSize); err != nil {
			return err
		}
		return tx.Create(session).Error
	})
}

func (dao *UploadDAO) GetSession(workspaceID, userID uint, sessionID string) (*model.UploadSession, error) {
	var session model.UploadSession
	err := dao.db.Where("id = ? AND workspace_id = ? AND created_by = ?", sessionID, workspaceID, userID).First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUploadNotFound
		}
		return nil, err
	}
	if session.Status != "completed" && session.Status != "expired" && time.Now().After(session.ExpiresAt) {
		if err := dao.expireSession(workspaceID, userID, sessionID); err != nil {
			return nil, err
		}
		session.Status = "expired"
	}
	return &session, nil
}

func (dao *UploadDAO) expireSession(workspaceID, userID uint, sessionID string) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ? AND created_by = ?", sessionID, workspaceID, userID).First(&session).Error; err != nil {
			return err
		}
		if session.Status == "completed" || session.Status == "expired" {
			return nil
		}
		if err := tx.Model(&session).Update("status", "expired").Error; err != nil {
			return err
		}
		return releaseQuota(tx, workspaceID, userID, session.TotalSize)
	})
}

func (dao *UploadDAO) MarkPartReceived(workspaceID, userID uint, sessionID string, partNo int) ([]int, error) {
	var received []int
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND workspace_id = ? AND created_by = ?", sessionID, workspaceID, userID).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUploadNotFound
			}
			return err
		}
		if session.Status != "initialized" && session.Status != "uploading" {
			return ErrUploadState
		}
		decoded, decodeErr := DecodeReceivedChunks(session.ReceivedChunks)
		if decodeErr != nil {
			return decodeErr
		}
		received = decoded
		if !containsInt(received, partNo) {
			received = append(received, partNo)
			sort.Ints(received)
		}
		encoded, err := json.Marshal(received)
		if err != nil {
			return err
		}
		return tx.Model(&session).Updates(map[string]any{"received_chunks": string(encoded), "status": "uploading"}).Error
	})
	return received, err
}

func (dao *UploadDAO) Cancel(workspaceID, userID uint, sessionID string) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ? AND created_by = ?", sessionID, workspaceID, userID).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUploadNotFound
			}
			return err
		}
		if session.Status == "completed" || session.Status == "expired" {
			return ErrUploadState
		}
		if err := tx.Model(&session).Update("status", "expired").Error; err != nil {
			return err
		}
		return releaseQuota(tx, workspaceID, userID, session.TotalSize)
	})
}

// ExpireSessions marks stale sessions and releases their reserved quota. It
// returns session IDs so the lifecycle worker can remove staging directories.
func (dao *UploadDAO) ExpireSessions(now time.Time) ([]string, error) {
	var expired []string
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var sessions []model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status NOT IN ? AND expires_at <= ?", []string{"completed", "expired"}, now).
			Find(&sessions).Error; err != nil {
			return err
		}
		for _, session := range sessions {
			if err := tx.Model(&session).Update("status", "expired").Error; err != nil {
				return err
			}
			if err := releaseQuota(tx, session.WorkspaceID, session.CreatedBy, session.TotalSize); err != nil {
				return err
			}
			expired = append(expired, session.ID)
		}
		return nil
	})
	return expired, err
}

func (dao *UploadDAO) FinalizeSession(sessionID string, workspaceID, userID uint, version *model.FileVersion, node *model.Node) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var session model.UploadSession
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND workspace_id = ? AND created_by = ?", sessionID, workspaceID, userID).First(&session).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrUploadNotFound
			}
			return err
		}
		if session.Status != "uploading" && session.Status != "initialized" {
			return ErrUploadState
		}
		received, err := DecodeReceivedChunks(session.ReceivedChunks)
		if err != nil {
			return err
		}
		if len(received) != session.TotalChunks {
			return ErrUploadIncomplete
		}

		if session.NodeID != nil {
			var currentNode model.Node
			if err := tx.Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, *session.NodeID, "file", "active").First(&currentNode).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrUploadNotFound
				}
				return err
			}
			var latest model.FileVersion
			err := tx.Where("workspace_id = ? AND node_id = ?", workspaceID, currentNode.ID).Order("version_no DESC").First(&latest).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			currentVersionNo := 0
			if err == nil {
				currentVersionNo = latest.VersionNo
			}
			if session.BaseVersionNo == nil || *session.BaseVersionNo != currentVersionNo {
				return ErrVersionConflict
			}
			version.NodeID = currentNode.ID
			version.VersionNo = currentVersionNo + 1
			version.WorkspaceID = workspaceID
			if err := tx.Create(version).Error; err != nil {
				return err
			}
			if err := tx.Model(&currentNode).Update("active_version", version.ID).Error; err != nil {
				return err
			}
			if err := appendChange(tx, workspaceID, "file_version", version.ID, "create", map[string]any{
				"node_id": version.NodeID, "version_no": version.VersionNo, "size": version.Size,
				"sha256": version.SHA256, "storage_key": version.StorageKey,
			}); err != nil {
				return err
			}
		} else {
			version.VersionNo = 1
			version.WorkspaceID = workspaceID
			node.WorkspaceID = workspaceID
			if err := tx.Create(node).Error; err != nil {
				return err
			}
			if err := tx.Create(&model.NodeClosure{AncestorID: node.ID, DescendantID: node.ID, Depth: 0}).Error; err != nil {
				return err
			}
			if node.ParentID != nil {
				if err := tx.Exec(`
					INSERT INTO node_closures (ancestor_id, descendant_id, depth)
					SELECT ancestor_id, ?, depth + 1 FROM node_closures WHERE descendant_id = ?
				`, node.ID, *node.ParentID).Error; err != nil {
					return err
				}
			}
			version.NodeID = node.ID
			if err := tx.Create(version).Error; err != nil {
				return err
			}
			if err := tx.Model(node).Update("active_version", version.ID).Error; err != nil {
				return err
			}
			if err := appendChange(tx, workspaceID, "node", node.ID, "create", node); err != nil {
				return err
			}
			if err := appendChange(tx, workspaceID, "file_version", version.ID, "create", map[string]any{
				"node_id": version.NodeID, "version_no": version.VersionNo, "size": version.Size,
				"sha256": version.SHA256, "storage_key": version.StorageKey,
			}); err != nil {
				return err
			}
		}
		if err := consumeQuota(tx, workspaceID, userID, session.TotalSize); err != nil {
			return err
		}
		return tx.Model(&session).Updates(map[string]any{"status": "completed", "received_chunks": session.ReceivedChunks}).Error
	})
}

func reserveQuota(tx *gorm.DB, workspaceID, userID uint, bytes int64) error {
	var workspace model.Workspace
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
		return err
	}
	if workspace.QuotaBytes != nil && workspace.UsedBytes+workspace.ReservedBytes+bytes > *workspace.QuotaBytes {
		return ErrQuotaExceeded
	}
	if err := tx.Model(&workspace).UpdateColumn("reserved_bytes", gorm.Expr("reserved_bytes + ?", bytes)).Error; err != nil {
		return err
	}
	var membership model.WorkspaceMembership
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if membership.QuotaBytes != nil && membership.UsedBytes+membership.ReservedBytes+bytes > *membership.QuotaBytes {
		return ErrQuotaExceeded
	}
	return tx.Model(&membership).UpdateColumn("reserved_bytes", gorm.Expr("reserved_bytes + ?", bytes)).Error
}

func releaseQuota(tx *gorm.DB, workspaceID, userID uint, bytes int64) error {
	if err := tx.Model(&model.Workspace{}).Where("id = ?", workspaceID).
		UpdateColumn("reserved_bytes", gorm.Expr("GREATEST(reserved_bytes - ?, 0)", bytes)).Error; err != nil {
		return err
	}
	return tx.Model(&model.WorkspaceMembership{}).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		UpdateColumn("reserved_bytes", gorm.Expr("GREATEST(reserved_bytes - ?, 0)", bytes)).Error
}

func consumeQuota(tx *gorm.DB, workspaceID, userID uint, bytes int64) error {
	if err := tx.Model(&model.Workspace{}).Where("id = ?", workspaceID).
		Updates(map[string]any{
			"used_bytes":     gorm.Expr("used_bytes + ?", bytes),
			"reserved_bytes": gorm.Expr("GREATEST(reserved_bytes - ?, 0)", bytes),
		}).Error; err != nil {
		return err
	}
	return tx.Model(&model.WorkspaceMembership{}).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		Updates(map[string]any{
			"used_bytes":     gorm.Expr("used_bytes + ?", bytes),
			"reserved_bytes": gorm.Expr("GREATEST(reserved_bytes - ?, 0)", bytes),
		}).Error
}

func DecodeReceivedChunks(encoded string) ([]int, error) {
	if encoded == "" {
		return []int{}, nil
	}
	var chunks []int
	if err := json.Unmarshal([]byte(encoded), &chunks); err != nil {
		return nil, err
	}
	for _, chunk := range chunks {
		if chunk < 0 {
			return nil, errors.New("received chunk index cannot be negative")
		}
	}
	sort.Ints(chunks)
	return chunks, nil
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
