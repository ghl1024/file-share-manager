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
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FavoriteDAO struct {
	db *gorm.DB
}

func NewFavoriteDAO() *FavoriteDAO {
	return &FavoriteDAO{db: database.DB}
}

func (dao *FavoriteDAO) Set(workspaceID, userID, nodeID uint, favorite bool) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if favorite {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.Favorite{
				WorkspaceID: workspaceID,
				UserID:      userID,
				NodeID:      nodeID,
			})
			if result.Error != nil || result.RowsAffected == 0 {
				return result.Error
			}
			return appendChange(tx, workspaceID, "favorite", nodeID, "create", map[string]any{"user_id": userID})
		}
		result := tx.Where("workspace_id = ? AND user_id = ? AND node_id = ?", workspaceID, userID, nodeID).
			Delete(&model.Favorite{})
		if result.Error != nil || result.RowsAffected == 0 {
			return result.Error
		}
		return appendChange(tx, workspaceID, "favorite", nodeID, "delete", map[string]any{"user_id": userID})
	})
}

func (dao *FavoriteDAO) Exists(workspaceID, userID, nodeID uint) (bool, error) {
	var count int64
	err := dao.db.Model(&model.Favorite{}).
		Where("workspace_id = ? AND user_id = ? AND node_id = ?", workspaceID, userID, nodeID).
		Count(&count).Error
	return count > 0, err
}

func (dao *FavoriteDAO) ListNodeIDs(workspaceID, userID uint) ([]uint, error) {
	var nodeIDs []uint
	err := dao.db.Model(&model.Favorite{}).
		Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
		Order("created_at DESC, id DESC").
		Pluck("node_id", &nodeIDs).Error
	return nodeIDs, err
}

func (dao *FavoriteDAO) ListActiveNodes(workspaceID, userID uint, limit int) ([]model.Node, error) {
	if limit <= 0 {
		limit = 5000
	}
	var nodes []model.Node
	err := dao.db.Model(&model.Node{}).
		Select("nodes.*").
		Joins("JOIN favorites ON favorites.node_id = nodes.id AND favorites.workspace_id = nodes.workspace_id").
		Where("favorites.workspace_id = ? AND favorites.user_id = ? AND nodes.status = ?", workspaceID, userID, "active").
		Order("favorites.created_at DESC, favorites.id DESC").
		Limit(limit).
		Find(&nodes).Error
	return nodes, err
}
