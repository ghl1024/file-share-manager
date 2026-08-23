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
	"strings"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type NotificationDAO struct{ db *gorm.DB }

type UserNotificationFilter struct {
	Category   string
	UnreadOnly bool
}

func NewNotificationDAO() *NotificationDAO { return &NotificationDAO{db: database.DB} }

func (dao *NotificationDAO) CreateChannel(channel *model.NotificationChannel) error {
	return dao.db.Create(channel).Error
}

func (dao *NotificationDAO) UpdateChannel(channel *model.NotificationChannel) error {
	return dao.db.Model(&model.NotificationChannel{}).Where("id = ?", channel.ID).Updates(map[string]any{
		"name": channel.Name, "type": channel.Type, "config_ciphertext": channel.ConfigCiphertext,
		"status": channel.Status, "remark": channel.Remark, "updated_at": time.Now(),
	}).Error
}

func (dao *NotificationDAO) DeleteChannel(id uint) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		if err := tx.Model(&model.NotificationOutbox{}).
			Where("channel_id = ? AND status IN ?", id, []string{"pending", "failed"}).
			Updates(map[string]any{"status": "cancelled", "error_message": "通知渠道已删除", "updated_at": now}).Error; err != nil {
			return err
		}
		result := tx.Delete(&model.NotificationChannel{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

func (dao *NotificationDAO) GetChannel(id uint) (*model.NotificationChannel, error) {
	var channel model.NotificationChannel
	err := dao.db.Where("id = ?", id).First(&channel).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &channel, err
}

func (dao *NotificationDAO) ListChannels(page, pageSize int) (*pagination.PageResult[model.NotificationChannel], error) {
	query := dao.db.Model(&model.NotificationChannel{}).Order("created_at DESC, id DESC")
	return pagination.Paging[model.NotificationChannel](query, page, pageSize)
}

func (dao *NotificationDAO) EnabledChannels() ([]model.NotificationChannel, error) {
	if dao == nil || dao.db == nil {
		return []model.NotificationChannel{}, nil
	}
	var channels []model.NotificationChannel
	err := dao.db.Where("status = ?", 1).Order("id ASC").Limit(100).Find(&channels).Error
	return channels, err
}

func (dao *NotificationDAO) Enqueue(rows []model.NotificationOutbox) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	result := dao.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedup_key"}}, DoNothing: true}).Create(&rows)
	return result.RowsAffected, result.Error
}

func (dao *NotificationDAO) ListOutbox(page, pageSize int, status string) (*pagination.PageResult[model.NotificationOutbox], error) {
	query := dao.db.Model(&model.NotificationOutbox{}).Order("created_at DESC, id DESC")
	if status = strings.TrimSpace(status); status != "" {
		query = query.Where("status = ?", status)
	}
	return pagination.Paging[model.NotificationOutbox](query, page, pageSize)
}

func (dao *NotificationDAO) ListDueIDs(now time.Time, limit int) ([]string, error) {
	var ids []string
	err := dao.db.Model(&model.NotificationOutbox{}).
		Where("status IN ? AND attempts < max_attempts AND next_attempt_at <= ?", []string{"pending", "failed"}, now).
		Order("next_attempt_at ASC, created_at ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (dao *NotificationDAO) Claim(id string, now time.Time) (bool, error) {
	result := dao.db.Model(&model.NotificationOutbox{}).
		Where("id = ? AND status IN ? AND attempts < max_attempts AND next_attempt_at <= ?", id, []string{"pending", "failed"}, now).
		Updates(map[string]any{"status": "sending", "attempts": gorm.Expr("attempts + 1"), "locked_at": now, "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (dao *NotificationDAO) GetOutbox(id string) (*model.NotificationOutbox, error) {
	var row model.NotificationOutbox
	err := dao.db.Where("id = ?", strings.TrimSpace(id)).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}

func (dao *NotificationDAO) MarkSent(id string, now time.Time) error {
	return dao.db.Model(&model.NotificationOutbox{}).Where("id = ? AND status = ?", id, "sending").
		Updates(map[string]any{"status": "sent", "sent_at": now, "locked_at": nil, "error_message": "", "updated_at": now}).Error
}

func (dao *NotificationDAO) MarkFailed(id, message string, next time.Time, exhausted bool) error {
	if len(message) > 1000 {
		message = message[:1000]
	}
	status := "failed"
	if exhausted {
		status = "exhausted"
	}
	return dao.db.Model(&model.NotificationOutbox{}).Where("id = ? AND status = ?", id, "sending").
		Updates(map[string]any{"status": status, "next_attempt_at": next, "locked_at": nil, "error_message": message, "updated_at": time.Now()}).Error
}

func (dao *NotificationDAO) RequeueInterrupted(cutoff, now time.Time) (int64, error) {
	result := dao.db.Model(&model.NotificationOutbox{}).Where("status = ? AND locked_at <= ?", "sending", cutoff).
		Updates(map[string]any{"status": "failed", "next_attempt_at": now, "locked_at": nil, "error_message": "通知进程中断，已重新排队", "updated_at": now})
	return result.RowsAffected, result.Error
}

func (dao *NotificationDAO) Retry(id string, now time.Time) (bool, error) {
	result := dao.db.Model(&model.NotificationOutbox{}).Where("id = ? AND status IN ?", strings.TrimSpace(id), []string{"failed", "exhausted"}).
		Updates(map[string]any{"status": "pending", "attempts": 0, "next_attempt_at": now, "locked_at": nil, "error_message": "", "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (dao *NotificationDAO) CreateUserNotifications(rows []model.UserNotification) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	result := dao.db.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "dedup_key"}}, DoNothing: true}).Create(&rows)
	return result.RowsAffected, result.Error
}

func (dao *NotificationDAO) ListUserNotifications(userID uint, page, pageSize int, filter UserNotificationFilter) (*pagination.PageResult[model.UserNotificationView], error) {
	query := dao.db.Model(&model.UserNotification{}).
		Select("user_notifications.*, COALESCE(workspaces.name, '') AS workspace_name").
		Joins("LEFT JOIN workspaces ON workspaces.id = user_notifications.workspace_id").
		Where("user_notifications.user_id = ?", userID).
		Order("user_notifications.created_at DESC, user_notifications.id DESC")
	if category := strings.TrimSpace(filter.Category); category != "" {
		query = query.Where("user_notifications.category = ?", category)
	}
	if filter.UnreadOnly {
		query = query.Where("user_notifications.is_read = ?", false)
	}
	return pagination.Paging[model.UserNotificationView](query, page, pageSize)
}

func (dao *NotificationDAO) CountUnreadUserNotifications(userID uint) (int64, error) {
	var count int64
	err := dao.db.Model(&model.UserNotification{}).Where("user_id = ? AND is_read = ?", userID, false).Count(&count).Error
	return count, err
}

func (dao *NotificationDAO) GetUserNotification(userID uint, id string) (*model.UserNotification, error) {
	var item model.UserNotification
	err := dao.db.Where("user_id = ? AND id = ?", userID, strings.TrimSpace(id)).First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &item, err
}

func (dao *NotificationDAO) MarkUserNotificationRead(userID uint, id string, now time.Time) error {
	return dao.db.Model(&model.UserNotification{}).
		Where("user_id = ? AND id = ? AND is_read = ?", userID, strings.TrimSpace(id), false).
		Updates(map[string]any{"is_read": true, "read_at": now, "updated_at": now}).Error
}

func (dao *NotificationDAO) MarkAllUserNotificationsRead(userID uint, now time.Time) (int64, error) {
	result := dao.db.Model(&model.UserNotification{}).Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]any{"is_read": true, "read_at": now, "updated_at": now})
	return result.RowsAffected, result.Error
}

func (dao *NotificationDAO) GetUserNotificationPreference(userID uint) (*model.UserNotificationPreference, error) {
	preference := model.UserNotificationPreference{
		UserID: userID, CollaborationEnabled: true, TaskEnabled: true, SecurityEnabled: true, ShareEnabled: true,
	}
	err := dao.db.Where("user_id = ?", userID).First(&preference).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return &preference, nil
	}
	return &preference, err
}

func (dao *NotificationDAO) SaveUserNotificationPreference(preference *model.UserNotificationPreference) error {
	if preference == nil {
		return errors.New("notification preference is required")
	}
	now := time.Now()
	if preference.UpdatedAt.IsZero() {
		preference.UpdatedAt = now
	}
	values := map[string]any{
		"user_id": preference.UserID, "collaboration_enabled": preference.CollaborationEnabled,
		"task_enabled": preference.TaskEnabled, "security_enabled": preference.SecurityEnabled,
		"share_enabled": preference.ShareEnabled, "created_at": now, "updated_at": preference.UpdatedAt,
	}
	return dao.db.Table(preference.TableName()).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"collaboration_enabled", "task_enabled", "security_enabled", "share_enabled", "updated_at",
		}),
	}).Create(values).Error
}
