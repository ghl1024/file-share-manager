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
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrShareNotFound        = errors.New("share not found")
	ErrShareExpired         = errors.New("share expired")
	ErrShareRevoked         = errors.New("share revoked")
	ErrShareLimit           = errors.New("share download limit reached")
	ErrShareItem            = errors.New("share item not found")
	ErrShareUnsafe          = errors.New("share item is not safe for external download")
	ErrShareRestoreRequired = errors.New("share item requires archive restore")
)

type ShareDAO struct {
	db *gorm.DB
}

// ShareListFilter contains only workspace-scoped management filters. The
// handler decides whether OwnerID must be set before this reaches the DAO.
type ShareListFilter struct {
	OwnerID     *uint
	Name        string
	Status      string
	Creator     string
	ExpiresFrom *time.Time
	ExpiresTo   *time.Time
	Now         time.Time
}

func NewShareDAO() *ShareDAO { return &ShareDAO{db: database.DB} }

func (dao *ShareDAO) Create(share *model.Share, items []model.ShareItem) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(share).Error; err != nil {
			return err
		}
		for index := range items {
			items[index].ShareID = share.ID
		}
		if len(items) > 0 {
			if err := tx.Create(&items).Error; err != nil {
				return err
			}
		}
		return appendChange(tx, share.WorkspaceID, "share", share.ID, "create", map[string]any{
			"share": share, "items": items,
		})
	})
}

func (dao *ShareDAO) GetByID(workspaceID, id uint) (*model.Share, error) {
	var share model.Share
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&share).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &share, err
}

func (dao *ShareDAO) GetByTokenHash(tokenHash string) (*model.Share, error) {
	var share model.Share
	err := dao.db.Where("token_hash = ?", tokenHash).First(&share).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &share, err
}

func (dao *ShareDAO) ListItems(shareID uint) ([]model.ShareItem, error) {
	var items []model.ShareItem
	err := dao.db.Where("share_id = ?", shareID).Order("relative_path ASC, id ASC").Find(&items).Error
	return items, err
}

func (dao *ShareDAO) ListItemPreview(shareID uint, limit int) ([]model.ShareItem, error) {
	if limit < 1 {
		limit = 10
	}
	var items []model.ShareItem
	err := dao.db.Where("share_id = ?", shareID).
		Order("relative_path ASC, id ASC").Limit(limit).Find(&items).Error
	return items, err
}

func (dao *ShareDAO) CountItemsByShareIDs(shareIDs []uint) (map[uint]int64, error) {
	counts := make(map[uint]int64, len(shareIDs))
	if len(shareIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		ShareID uint
		Count   int64
	}
	if err := dao.db.Model(&model.ShareItem{}).
		Select("share_id, COUNT(*) AS count").
		Where("share_id IN ?", shareIDs).
		Group("share_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		counts[row.ShareID] = row.Count
	}
	return counts, nil
}

func (dao *ShareDAO) ListPage(workspaceID uint, page, pageSize int, filter ShareListFilter) (*pagination.PageResult[model.Share], error) {
	now := filter.Now
	if now.IsZero() {
		now = time.Now()
	}
	query := dao.db.Model(&model.Share{}).
		Where("shares.workspace_id = ?", workspaceID)
	if filter.OwnerID != nil {
		query = query.Where("shares.created_by = ?", *filter.OwnerID)
	}
	if filter.Name != "" {
		query = query.Where("shares.name LIKE ? ESCAPE '\\\\'", "%"+escapeSearchLike(filter.Name)+"%")
	}
	if filter.Creator != "" {
		creator := escapeSearchLike(filter.Creator) + "%"
		query = query.
			Joins("LEFT JOIN users AS share_creators ON share_creators.id = shares.created_by").
			Where("(share_creators.username LIKE ? ESCAPE '\\\\' OR share_creators.real_name LIKE ? ESCAPE '\\\\')", creator, creator)
	}
	switch filter.Status {
	case "active":
		query = query.Where("shares.status = ? AND shares.expires_at > ? AND (shares.max_downloads IS NULL OR shares.download_count < shares.max_downloads)", "active", now)
	case "revoked":
		query = query.Where("shares.status = ?", "revoked")
	case "expired":
		query = query.Where("shares.status <> ? AND (shares.status = ? OR shares.expires_at <= ?)", "revoked", "expired", now)
	case "exhausted":
		query = query.Where("shares.status = ? AND shares.expires_at > ? AND shares.max_downloads IS NOT NULL AND shares.download_count >= shares.max_downloads", "active", now)
	}
	if filter.ExpiresFrom != nil {
		query = query.Where("shares.expires_at >= ?", *filter.ExpiresFrom)
	}
	if filter.ExpiresTo != nil {
		query = query.Where("shares.expires_at <= ?", *filter.ExpiresTo)
	}
	query = query.Order("shares.created_at DESC, shares.id DESC")
	return pagination.Paging[model.Share](query, page, pageSize)
}

func (dao *ShareDAO) Revoke(workspaceID, id uint) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		result := tx.Model(&model.Share{}).
			Where("workspace_id = ? AND id = ? AND status = ?", workspaceID, id, "active").
			Updates(map[string]any{"status": "revoked", "revoked_at": now, "updated_at": now})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return appendChange(tx, workspaceID, "share", id, "revoke", map[string]any{"revoked_at": now})
	})
}

func (dao *ShareDAO) Expire(now time.Time) ([]model.Share, error) {
	var shares []model.Share
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("status = ? AND expires_at <= ?", "active", now).Find(&shares).Error; err != nil {
			return err
		}
		for _, share := range shares {
			if err := tx.Model(&share).Updates(map[string]any{"status": "expired", "updated_at": now}).Error; err != nil {
				return err
			}
			if err := appendChange(tx, share.WorkspaceID, "share", share.ID, "expire", map[string]any{"expired_at": now}); err != nil {
				return err
			}
		}
		return nil
	})
	return shares, err
}

func (dao *ShareDAO) ListExpiringSoon(now, horizon time.Time) ([]model.Share, error) {
	var shares []model.Share
	err := dao.db.Where("status = ? AND expires_at > ? AND expires_at <= ?", "active", now, horizon).
		Order("expires_at ASC, id ASC").Limit(1000).Find(&shares).Error
	return shares, err
}

// ClaimDownload atomically checks the share and reserves one download slot.
// This prevents concurrent requests from exceeding max_downloads.
func (dao *ShareDAO) ClaimDownload(tokenHash, itemPublicID string) (*model.Share, *model.ShareItem, error) {
	var share model.Share
	var item model.ShareItem
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("token_hash = ?", tokenHash).First(&share).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrShareNotFound
			}
			return err
		}
		now := time.Now()
		if share.Status != "active" {
			if share.Status == "revoked" {
				return ErrShareRevoked
			}
			return ErrShareExpired
		}
		if !now.Before(share.ExpiresAt) {
			_ = tx.Model(&share).Updates(map[string]any{"status": "expired", "updated_at": now}).Error
			return ErrShareExpired
		}
		if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
			return ErrShareLimit
		}
		query := tx.Where("share_id = ?", share.ID)
		if itemPublicID != "" {
			query = query.Where("public_id = ?", itemPublicID)
		}
		if err := query.Order("id ASC").First(&item).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrShareItem
			}
			return err
		}
		if !shareItemSafeForExternalDownload(item) {
			return ErrShareUnsafe
		}
		if item.StorageClass == "glacier" || item.StorageClass == "restoring" {
			return ErrShareRestoreRequired
		}
		share.DownloadCount++
		if err := tx.Model(&share).Updates(map[string]any{"download_count": share.DownloadCount, "updated_at": now}).Error; err != nil {
			return err
		}
		return appendChange(tx, share.WorkspaceID, "share", share.ID, "claim_download", map[string]any{"download_count": share.DownloadCount})
	})
	if err != nil {
		return nil, nil, err
	}
	return &share, &item, nil
}

func shareItemSafeForExternalDownload(item model.ShareItem) bool {
	return item.ScanStatus == "clean"
}
