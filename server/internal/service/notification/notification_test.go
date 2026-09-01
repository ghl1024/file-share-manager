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
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/model"
)

type memoryNotificationStore struct {
	channel       *model.NotificationChannel
	outbox        *model.NotificationOutbox
	claimed       bool
	sent          bool
	failedStatus  string
	failedMessage string
	failedNext    time.Time
}

func (store *memoryNotificationStore) EnabledChannels() ([]model.NotificationChannel, error) {
	if store.channel == nil || store.channel.Status != 1 {
		return nil, nil
	}
	return []model.NotificationChannel{*store.channel}, nil
}

func (store *memoryNotificationStore) Enqueue(rows []model.NotificationOutbox) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	row := rows[0]
	store.outbox = &row
	return int64(len(rows)), nil
}

func (store *memoryNotificationStore) ListDueIDs(time.Time, int) ([]string, error) {
	if store.outbox == nil {
		return nil, nil
	}
	return []string{store.outbox.ID}, nil
}

func (store *memoryNotificationStore) Claim(id string, _ time.Time) (bool, error) {
	if store.outbox == nil || store.outbox.ID != id || store.claimed {
		return false, nil
	}
	store.claimed = true
	store.outbox.Attempts++
	store.outbox.Status = "sending"
	return true, nil
}

func (store *memoryNotificationStore) GetOutbox(id string) (*model.NotificationOutbox, error) {
	if store.outbox == nil || store.outbox.ID != id {
		return nil, nil
	}
	return store.outbox, nil
}

func (store *memoryNotificationStore) GetChannel(id uint) (*model.NotificationChannel, error) {
	if store.channel == nil || store.channel.ID != id {
		return nil, nil
	}
	return store.channel, nil
}

func (store *memoryNotificationStore) MarkSent(id string, _ time.Time) error {
	if store.outbox == nil || store.outbox.ID != id {
		return errors.New("outbox row not found")
	}
	store.sent = true
	store.outbox.Status = "sent"
	return nil
}

func (store *memoryNotificationStore) MarkFailed(id, message string, next time.Time, exhausted bool) error {
	if store.outbox == nil || store.outbox.ID != id {
		return errors.New("outbox row not found")
	}
	store.failedStatus = "failed"
	if exhausted {
		store.failedStatus = "exhausted"
	}
	store.failedMessage = message
	store.failedNext = next
	store.outbox.Status = store.failedStatus
	return nil
}

func (store *memoryNotificationStore) RequeueInterrupted(time.Time, time.Time) (int64, error) {
	return 0, nil
}

func TestCredentialEnvelopeRoundTripAndTamperDetection(t *testing.T) {
	previous := config.GetConfig()
	config.SetTestConfig(&config.Config{Notification: config.NotificationConfig{
		CredentialEncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
	}})
	t.Cleanup(func() { config.SetTestConfig(previous) })

	original := ChannelSettings{WebhookURL: "https://open.feishu.cn/open-apis/bot/v2/hook/test-token", Secret: "signing-secret"}
	ciphertext, err := EncryptSettings(original)
	if err != nil {
		t.Fatalf("EncryptSettings() error = %v", err)
	}
	if strings.Contains(ciphertext, "test-token") || strings.Contains(ciphertext, "signing-secret") {
		t.Fatal("encrypted channel settings contain plaintext credentials")
	}
	decoded, err := DecryptSettings(ciphertext)
	if err != nil {
		t.Fatalf("DecryptSettings() error = %v", err)
	}
	if decoded.WebhookURL != original.WebhookURL || decoded.Secret != original.Secret {
		t.Fatalf("DecryptSettings() = %#v, want %#v", decoded, original)
	}

	tampered := ciphertext[:len(ciphertext)-2] + "AA"
	if _, err := DecryptSettings(tampered); err == nil {
		t.Fatal("DecryptSettings() accepted a tampered credential envelope")
	}
}

func TestValidateSettingsRestrictsWebhookHostsAndSMTPEncryption(t *testing.T) {
	validWebhook := ChannelSettings{WebhookURL: "https://oapi.dingtalk.com/robot/send?access_token=test"}
	if err := ValidateSettings("dingtalk", &validWebhook); err != nil {
		t.Fatalf("ValidateSettings(valid dingtalk) error = %v", err)
	}
	for _, settings := range []ChannelSettings{
		{WebhookURL: "http://oapi.dingtalk.com/robot/send?access_token=test"},
		{WebhookURL: "https://example.com/robot/send?access_token=test"},
		{WebhookURL: "https://oapi.dingtalk.com.evil.example/robot/send"},
	} {
		candidate := settings
		if err := ValidateSettings("dingtalk", &candidate); err == nil {
			t.Fatalf("ValidateSettings() accepted unsafe webhook %q", candidate.WebhookURL)
		}
	}

	validSMTP := ChannelSettings{
		SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPEncryption: "starttls",
		SMTPUsername: "notify@example.com", SMTPPassword: "password", SMTPFrom: "notify@example.com",
		SMTPRecipients: []string{"ops@example.com", "Ops <ops@example.com>"},
	}
	if err := ValidateSettings("smtp", &validSMTP); err != nil {
		t.Fatalf("ValidateSettings(valid smtp) error = %v", err)
	}
	if len(validSMTP.SMTPRecipients) != 1 || validSMTP.SMTPRecipients[0] != "ops@example.com" {
		t.Fatalf("SMTP recipients were not normalized: %#v", validSMTP.SMTPRecipients)
	}
	validSMTP.SMTPEncryption = "none"
	if err := ValidateSettings("smtp", &validSMTP); err == nil {
		t.Fatal("ValidateSettings() accepted plaintext SMTP")
	}
}

func TestNotificationDedupAndRetryPolicy(t *testing.T) {
	first := notificationDedupKey("backup:failed:job-1", 7)
	if first != notificationDedupKey("backup:failed:job-1", 7) {
		t.Fatal("notification dedup key is not deterministic")
	}
	if first == notificationDedupKey("backup:failed:job-1", 8) || len(first) != 64 {
		t.Fatalf("notification dedup key has invalid channel isolation or length: %q", first)
	}
	base, maximum := 30*time.Second, 5*time.Minute
	if got := notificationRetryDelay(base, maximum, 1); got != 30*time.Second {
		t.Fatalf("retry delay attempt 1 = %s", got)
	}
	if got := notificationRetryDelay(base, maximum, 4); got != 4*time.Minute {
		t.Fatalf("retry delay attempt 4 = %s", got)
	}
	if got := notificationRetryDelay(base, maximum, 10); got != maximum {
		t.Fatalf("retry delay cap = %s, want %s", got, maximum)
	}
}

func TestValidateUserEventAndPreferences(t *testing.T) {
	workspaceID := uint(3)
	event := UserEvent{
		UserID: 7, WorkspaceID: &workspaceID, Type: "task:completed", Category: UserCategoryTask,
		Severity: "INFO", Title: " 任务完成 ", Content: " 可以下载 ", TargetType: "batch_download", TargetID: "job-1",
	}
	if err := validateUserEvent(&event); err != nil {
		t.Fatalf("validateUserEvent(valid) error = %v", err)
	}
	if event.Severity != "info" || event.Title != "任务完成" || event.Content != "可以下载" {
		t.Fatalf("validateUserEvent() did not normalize fields: %#v", event)
	}
	invalid := event
	invalid.Category = "administrator"
	if err := validateUserEvent(&invalid); err == nil {
		t.Fatal("validateUserEvent() accepted an invalid category")
	}
	preference := &model.UserNotificationPreference{
		CollaborationEnabled: true, TaskEnabled: false, SecurityEnabled: true, ShareEnabled: false,
	}
	if !userPreferenceAllows(preference, UserCategoryCollaboration) || !userPreferenceAllows(preference, UserCategorySecurity) {
		t.Fatal("enabled notification category was rejected")
	}
	if userPreferenceAllows(preference, UserCategoryTask) || userPreferenceAllows(preference, UserCategoryShare) {
		t.Fatal("disabled notification category was accepted")
	}
}

func TestSigningDoesNotExposeSecrets(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	endpoint := signDingTalkWebhook("https://oapi.dingtalk.com/robot/send?access_token=token", "secret-value", now)
	if !strings.Contains(endpoint, "timestamp=") || !strings.Contains(endpoint, "sign=") || strings.Contains(endpoint, "secret-value") {
		t.Fatalf("signed DingTalk endpoint is invalid: %s", endpoint)
	}
	payload := map[string]any{}
	addFeishuSignature(payload, "secret-value", now)
	if payload["timestamp"] == nil || payload["sign"] == nil || strings.Contains(payload["sign"].(string), "secret-value") {
		t.Fatalf("Feishu signature payload is invalid: %#v", payload)
	}
}

func TestNotificationWorkerCompletesAndRetriesJobs(t *testing.T) {
	previous := config.GetConfig()
	config.SetTestConfig(&config.Config{Notification: config.NotificationConfig{
		CredentialEncryptionKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
	}})
	t.Cleanup(func() { config.SetTestConfig(previous) })

	ciphertext, err := EncryptSettings(ChannelSettings{WebhookURL: "https://oapi.dingtalk.com/robot/send?access_token=test"})
	if err != nil {
		t.Fatalf("EncryptSettings() error = %v", err)
	}
	now := time.Now()
	newStore := func(status int) *memoryNotificationStore {
		return &memoryNotificationStore{
			channel: &model.NotificationChannel{ID: 7, Type: "dingtalk", Status: status, ConfigCiphertext: ciphertext},
			outbox:  &model.NotificationOutbox{ID: "job-1", ChannelID: 7, Status: "pending", MaxAttempts: 3, Title: "title", Content: "content"},
		}
	}

	successStore := newStore(1)
	successService := &Service{
		store:     successStore,
		deliver:   func(context.Context, string, ChannelSettings, Message) error { return nil },
		baseRetry: time.Second,
		maxRetry:  time.Minute,
	}
	successService.process(context.Background(), "job-1", now)
	if !successStore.claimed || !successStore.sent || successStore.outbox.Attempts != 1 || successStore.outbox.Status != "sent" {
		t.Fatalf("successful delivery state = %#v", successStore)
	}

	failureStore := newStore(0)
	failureService := &Service{store: failureStore, baseRetry: time.Second, maxRetry: time.Minute}
	failureService.process(context.Background(), "job-1", now)
	if failureStore.failedStatus != "failed" || failureStore.failedMessage != "通知渠道不存在或已停用" {
		t.Fatalf("failed delivery state = %#v", failureStore)
	}
	if got := failureStore.failedNext.Sub(now); got != time.Second {
		t.Fatalf("retry delay = %s, want 1s", got)
	}
}
