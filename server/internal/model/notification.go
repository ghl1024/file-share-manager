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

// NotificationChannel stores non-secret channel metadata. The complete
// adapter settings are encrypted before they reach this model.
type NotificationChannel struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	Name             string    `gorm:"type:varchar(80);not null" json:"name"`
	Type             string    `gorm:"type:varchar(20);not null;index" json:"type"`
	ConfigCiphertext string    `gorm:"type:text;not null" json:"-"`
	Status           int       `gorm:"type:tinyint;not null;default:1;index" json:"status"`
	Remark           string    `gorm:"type:varchar(255);not null;default:''" json:"remark"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

func (NotificationChannel) TableName() string { return "notification_channels" }

// NotificationOutbox is a durable, independently retryable delivery. One
// event creates one row per enabled channel so a failing adapter never blocks
// successful channels.
type NotificationOutbox struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	DedupKey      string     `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	ChannelID     uint       `gorm:"not null;index" json:"channel_id"`
	ChannelName   string     `gorm:"type:varchar(80);not null" json:"channel_name"`
	ChannelType   string     `gorm:"type:varchar(20);not null" json:"channel_type"`
	EventType     string     `gorm:"type:varchar(64);not null;index" json:"event_type"`
	Severity      string     `gorm:"type:varchar(16);not null;index" json:"severity"`
	Title         string     `gorm:"type:varchar(255);not null" json:"title"`
	Content       string     `gorm:"type:text;not null" json:"content"`
	PayloadJSON   string     `gorm:"type:json;not null" json:"-"`
	Status        string     `gorm:"type:varchar(16);not null;index:idx_notification_due,priority:1" json:"status"`
	Attempts      int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts   int        `gorm:"not null" json:"max_attempts"`
	NextAttemptAt time.Time  `gorm:"not null;index:idx_notification_due,priority:2" json:"next_attempt_at"`
	LockedAt      *time.Time `gorm:"index" json:"locked_at,omitempty"`
	SentAt        *time.Time `json:"sent_at,omitempty"`
	ErrorMessage  string     `gorm:"type:varchar(1000);not null;default:''" json:"error_message,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (NotificationOutbox) TableName() string { return "notification_outbox" }

// UserNotification is the in-product notification feed for one user. Target
// identifiers are server-side references and must be re-authorized before a
// handler returns a navigation destination.
type UserNotification struct {
	ID          string     `gorm:"type:char(36);primaryKey" json:"id"`
	DedupKey    string     `gorm:"type:char(64);not null;uniqueIndex" json:"-"`
	UserID      uint       `gorm:"not null;index:idx_user_notification_feed,priority:1;index:idx_user_notification_unread,priority:1" json:"-"`
	WorkspaceID *uint      `gorm:"index" json:"workspace_id,omitempty"`
	EventType   string     `gorm:"type:varchar(64);not null;index" json:"event_type"`
	Category    string     `gorm:"type:varchar(24);not null;index" json:"category"`
	Severity    string     `gorm:"type:varchar(16);not null;index" json:"severity"`
	Title       string     `gorm:"type:varchar(255);not null" json:"title"`
	Content     string     `gorm:"type:text;not null" json:"content"`
	TargetType  string     `gorm:"type:varchar(32);not null;default:''" json:"target_type,omitempty"`
	TargetID    string     `gorm:"type:varchar(64);not null;default:''" json:"-"`
	IsRead      bool       `gorm:"not null;default:false;index:idx_user_notification_unread,priority:2" json:"is_read"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `gorm:"index:idx_user_notification_feed,priority:2;index:idx_user_notification_unread,priority:3" json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (UserNotification) TableName() string { return "user_notifications" }

// UserNotificationView adds display-only workspace context without exposing
// any workspace membership or notification target internals.
type UserNotificationView struct {
	UserNotification
	WorkspaceName string `gorm:"column:workspace_name" json:"workspace_name,omitempty"`
}

// UserNotificationPreference controls broad user-facing categories. Missing
// rows are interpreted as all categories enabled.
type UserNotificationPreference struct {
	UserID               uint      `gorm:"primaryKey;autoIncrement:false" json:"-"`
	CollaborationEnabled bool      `gorm:"not null;default:true" json:"collaboration_enabled"`
	TaskEnabled          bool      `gorm:"not null;default:true" json:"task_enabled"`
	SecurityEnabled      bool      `gorm:"not null;default:true" json:"security_enabled"`
	ShareEnabled         bool      `gorm:"not null;default:true" json:"share_enabled"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

func (UserNotificationPreference) TableName() string { return "user_notification_preferences" }
