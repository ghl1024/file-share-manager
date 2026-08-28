/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package notification

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"

	"github.com/google/uuid"
)

type Event struct {
	Key      string
	Type     string
	Severity string
	Title    string
	Content  string
	Payload  map[string]any
}

const (
	UserCategoryCollaboration = "collaboration"
	UserCategoryTask          = "task"
	UserCategorySecurity      = "security"
	UserCategoryShare         = "share"
)

type UserEvent struct {
	Key         string
	UserID      uint
	WorkspaceID *uint
	Type        string
	Category    string
	Severity    string
	Title       string
	Content     string
	TargetType  string
	TargetID    string
}

type notificationStore interface {
	EnabledChannels() ([]model.NotificationChannel, error)
	Enqueue([]model.NotificationOutbox) (int64, error)
	ListDueIDs(time.Time, int) ([]string, error)
	Claim(string, time.Time) (bool, error)
	GetOutbox(string) (*model.NotificationOutbox, error)
	GetChannel(uint) (*model.NotificationChannel, error)
	MarkSent(string, time.Time) error
	MarkFailed(string, string, time.Time, bool) error
	RequeueInterrupted(time.Time, time.Time) (int64, error)
}

type Service struct {
	store        notificationStore
	deliver      func(context.Context, string, ChannelSettings, Message) error
	workers      int
	batchSize    int
	pollInterval time.Duration
	baseRetry    time.Duration
	maxRetry     time.Duration
	staleAfter   time.Duration
	mu           sync.Mutex
}

func NewService() *Service {
	cfg := config.GetConfig()
	service := &Service{store: dao.NewNotificationDAO(), deliver: Deliver, workers: 1, batchSize: 50, pollInterval: 5 * time.Second, baseRetry: 30 * time.Second, maxRetry: time.Hour, staleAfter: 10 * time.Minute}
	if cfg != nil {
		service.workers = cfg.Notification.WorkerCount
		service.batchSize = cfg.Notification.BatchSize
		service.pollInterval = time.Duration(cfg.Notification.PollIntervalSeconds) * time.Second
		service.baseRetry = time.Duration(cfg.Notification.BaseRetrySeconds) * time.Second
		service.maxRetry = time.Duration(cfg.Notification.MaxRetrySeconds) * time.Second
	}
	return service
}

func Publish(ctx context.Context, event Event) (int64, error) {
	return NewService().Publish(ctx, event)
}

func PublishUser(ctx context.Context, event UserEvent) (int64, error) {
	return PublishUsers(ctx, []UserEvent{event})
}

func PublishUsers(_ context.Context, events []UserEvent) (int64, error) {
	if len(events) == 0 {
		return 0, nil
	}
	dao := dao.NewNotificationDAO()
	rows := make([]model.UserNotification, 0, len(events))
	seen := make(map[string]struct{}, len(events))
	for _, event := range events {
		if err := validateUserEvent(&event); err != nil {
			return 0, err
		}
		preference, err := dao.GetUserNotificationPreference(event.UserID)
		if err != nil {
			return 0, err
		}
		if !userPreferenceAllows(preference, event.Category) {
			continue
		}
		baseKey := strings.TrimSpace(event.Key)
		if baseKey == "" {
			baseKey = uuid.NewString()
		}
		digest := sha256.Sum256([]byte(fmt.Sprintf("%s:%d", baseKey, event.UserID)))
		dedupKey := hex.EncodeToString(digest[:])
		if _, exists := seen[dedupKey]; exists {
			continue
		}
		seen[dedupKey] = struct{}{}
		rows = append(rows, model.UserNotification{
			ID: uuid.NewString(), DedupKey: dedupKey, UserID: event.UserID, WorkspaceID: event.WorkspaceID,
			EventType: event.Type, Category: event.Category, Severity: event.Severity,
			Title: event.Title, Content: event.Content, TargetType: event.TargetType, TargetID: event.TargetID,
		})
	}
	return dao.CreateUserNotifications(rows)
}

func ValidUserCategory(category string) bool {
	switch strings.TrimSpace(category) {
	case UserCategoryCollaboration, UserCategoryTask, UserCategorySecurity, UserCategoryShare:
		return true
	default:
		return false
	}
}

func validateUserEvent(event *UserEvent) error {
	if event == nil || event.UserID == 0 {
		return errors.New("notification user is required")
	}
	event.Type = strings.TrimSpace(event.Type)
	event.Category = strings.TrimSpace(event.Category)
	event.Severity = strings.ToLower(strings.TrimSpace(event.Severity))
	event.Title = strings.TrimSpace(event.Title)
	event.Content = strings.TrimSpace(event.Content)
	event.TargetType = strings.TrimSpace(event.TargetType)
	event.TargetID = strings.TrimSpace(event.TargetID)
	if event.Type == "" || !ValidUserCategory(event.Category) || event.Title == "" || event.Content == "" {
		return errors.New("user notification type, category, title and content are required")
	}
	if event.Severity != "info" && event.Severity != "warning" && event.Severity != "critical" {
		return errors.New("user notification severity is invalid")
	}
	if len(event.Type) > 64 || len(event.Title) > 255 || len(event.Content) > 4000 || len(event.TargetType) > 32 || len(event.TargetID) > 64 {
		return errors.New("user notification fields exceed their maximum length")
	}
	return nil
}

func userPreferenceAllows(preference *model.UserNotificationPreference, category string) bool {
	if preference == nil {
		return true
	}
	switch category {
	case UserCategoryCollaboration:
		return preference.CollaborationEnabled
	case UserCategoryTask:
		return preference.TaskEnabled
	case UserCategorySecurity:
		return preference.SecurityEnabled
	case UserCategoryShare:
		return preference.ShareEnabled
	default:
		return false
	}
}

func (s *Service) Publish(_ context.Context, event Event) (int64, error) {
	channels, err := s.store.EnabledChannels()
	if err != nil || len(channels) == 0 {
		return 0, err
	}
	event.Type = strings.TrimSpace(event.Type)
	event.Severity = strings.ToLower(strings.TrimSpace(event.Severity))
	event.Title = strings.TrimSpace(event.Title)
	event.Content = strings.TrimSpace(event.Content)
	if event.Type == "" || event.Title == "" || event.Content == "" {
		return 0, errors.New("notification event type, title and content are required")
	}
	if event.Severity != "info" && event.Severity != "warning" && event.Severity != "critical" {
		return 0, errors.New("notification event severity is invalid")
	}
	if len(event.Type) > 64 || len(event.Title) > 255 || len(event.Content) > 16000 {
		return 0, errors.New("notification event fields exceed their maximum length")
	}
	payload, err := json.Marshal(event.Payload)
	if err != nil {
		return 0, err
	}
	if len(payload) == 0 || string(payload) == "null" {
		payload = []byte("{}")
	}
	baseKey := strings.TrimSpace(event.Key)
	if baseKey == "" {
		baseKey = uuid.NewString()
	}
	cfg := config.GetConfig()
	maxAttempts := 5
	if cfg != nil {
		maxAttempts = cfg.Notification.MaxAttempts
	}
	now := time.Now()
	rows := make([]model.NotificationOutbox, 0, len(channels))
	for _, channel := range channels {
		rows = append(rows, model.NotificationOutbox{
			ID: uuid.NewString(), DedupKey: notificationDedupKey(baseKey, channel.ID), ChannelID: channel.ID,
			ChannelName: channel.Name, ChannelType: channel.Type, EventType: event.Type, Severity: event.Severity,
			Title: event.Title, Content: event.Content, PayloadJSON: string(payload), Status: "pending",
			MaxAttempts: maxAttempts, NextAttemptAt: now,
		})
	}
	return s.store.Enqueue(rows)
}

func EnqueueTest(channel *model.NotificationChannel) (int64, error) {
	if channel == nil {
		return 0, errors.New("notification channel not found")
	}
	cfg := config.GetConfig()
	maxAttempts := 5
	if cfg != nil {
		maxAttempts = cfg.Notification.MaxAttempts
	}
	now := time.Now()
	row := model.NotificationOutbox{
		ID: uuid.NewString(), DedupKey: notificationDedupKey("test:"+uuid.NewString(), channel.ID), ChannelID: channel.ID,
		ChannelName: channel.Name, ChannelType: channel.Type, EventType: "system:test", Severity: "info",
		Title: "File Share Manager 测试通知", Content: "通知渠道连接测试成功。此消息由可靠 Outbox 投递。",
		PayloadJSON: "{}", Status: "pending", MaxAttempts: maxAttempts, NextAttemptAt: now,
	}
	return dao.NewNotificationDAO().Enqueue([]model.NotificationOutbox{row})
}

func (s *Service) Start(ctx context.Context) {
	now := time.Now()
	if count, err := s.store.RequeueInterrupted(now.Add(-s.staleAfter), now); err != nil {
		logger.Error("notification_outbox_recovery_failed", "error", err)
	} else if count > 0 {
		logger.Warn("notification_outbox_recovered", "count", count)
	}
	go func() {
		s.runOnce(ctx, time.Now())
		ticker := time.NewTicker(s.pollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				s.runOnce(ctx, now)
			}
		}
	}()
	logger.Info("notification_outbox_worker_started", "workers", s.workers, "batch_size", s.batchSize, "poll_interval", s.pollInterval.String())
}

func (s *Service) runOnce(ctx context.Context, now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids, err := s.store.ListDueIDs(now, s.batchSize)
	if err != nil {
		logger.Error("notification_outbox_list_failed", "error", err)
		return
	}
	semaphore := make(chan struct{}, s.workers)
	var wg sync.WaitGroup
	for _, id := range ids {
		if ctx.Err() != nil {
			break
		}
		semaphore <- struct{}{}
		wg.Add(1)
		go func(jobID string) {
			defer wg.Done()
			defer func() { <-semaphore }()
			s.process(ctx, jobID, now)
		}(id)
	}
	wg.Wait()
}

func (s *Service) process(ctx context.Context, id string, now time.Time) {
	claimed, err := s.store.Claim(id, now)
	if err != nil || !claimed {
		if err != nil {
			logger.Error("notification_outbox_claim_failed", "job_id", id, "error", err)
		}
		return
	}
	row, err := s.store.GetOutbox(id)
	if err != nil || row == nil {
		s.fail(id, 1, 1, errors.New("notification outbox row could not be loaded"), now)
		return
	}
	channel, err := s.store.GetChannel(row.ChannelID)
	if err != nil || channel == nil || channel.Status != 1 {
		s.fail(id, row.Attempts, row.MaxAttempts, errors.New("通知渠道不存在或已停用"), now)
		return
	}
	settings, err := DecryptSettings(channel.ConfigCiphertext)
	if err == nil {
		err = s.deliver(ctx, channel.Type, settings, Message{Title: row.Title, Content: row.Content})
	}
	if err != nil {
		s.fail(id, row.Attempts, row.MaxAttempts, err, now)
		return
	}
	if err := s.store.MarkSent(id, time.Now()); err != nil {
		logger.Error("notification_outbox_complete_failed", "job_id", id, "error", err)
	}
}

func (s *Service) fail(id string, attempts, maxAttempts int, cause error, now time.Time) {
	exhausted := attempts >= maxAttempts
	next := now.Add(notificationRetryDelay(s.baseRetry, s.maxRetry, attempts))
	message := logger.SanitizeText(cause.Error())
	if err := s.store.MarkFailed(id, message, next, exhausted); err != nil {
		logger.Error("notification_outbox_fail_update_failed", "job_id", id, "error", err)
		return
	}
	logger.Warn("notification_delivery_failed", "job_id", id, "attempt", attempts, "exhausted", exhausted, "error", message)
}

func notificationDedupKey(eventKey string, channelID uint) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(eventKey) + ":" + fmt.Sprint(channelID)))
	return hex.EncodeToString(digest[:])
}

func notificationRetryDelay(base, maximum time.Duration, attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for current := 1; current < attempt && delay < maximum; current++ {
		delay *= 2
	}
	if delay > maximum {
		return maximum
	}
	return delay
}
