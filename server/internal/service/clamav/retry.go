/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package clamav

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/service/notification"
)

const retryWorkerPollInterval = time.Minute

type scanRetryDAO interface {
	ListScanRetryCandidates(now time.Time, maxAttempts, limit int) ([]model.FileVersion, error)
	ClaimScanRetry(versionID uint, expectedRetryCount int, attemptedAt time.Time) (bool, error)
	CompleteScanRetry(versionID uint, status, message string, nextRetryAt *time.Time) (bool, error)
	RequeueStaleScanRetries(cutoff, nextRetryAt time.Time) (int64, error)
}

type RetryStatus struct {
	Enabled             bool       `json:"enabled"`
	MaxAttempts         int        `json:"max_attempts"`
	BaseIntervalMinutes int        `json:"base_interval_minutes"`
	BatchSize           int        `json:"batch_size"`
	Retryable           int64      `json:"retryable"`
	Pending             int64      `json:"pending"`
	Exhausted           int64      `json:"exhausted"`
	Infected            int64      `json:"infected"`
	NextRetryAt         *time.Time `json:"next_retry_at,omitempty"`
}

type RetryReport struct {
	Recovered int `json:"recovered"`
	Scanned   int `json:"scanned"`
	Clean     int `json:"clean"`
	Infected  int `json:"infected"`
	Failed    int `json:"failed"`
	Exhausted int `json:"exhausted"`
}

type RetryService struct {
	files        scanRetryDAO
	scan         func(context.Context, string) Result
	storageRoot  string
	maxAttempts  int
	batchSize    int
	baseInterval time.Duration
	staleAfter   time.Duration
}

func NewRetryService() (*RetryService, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	if !cfg.ClamAV.Enabled() {
		return nil, nil
	}
	staleAfter := 2 * time.Duration(cfg.ClamAV.TimeoutSeconds) * time.Second
	if staleAfter < 5*time.Minute {
		staleAfter = 5 * time.Minute
	}
	return &RetryService{
		files: dao.NewFileDAO(), scan: ScanFile, storageRoot: cfg.Storage.RootPath,
		maxAttempts: cfg.ClamAV.RetryMaxAttempts, batchSize: cfg.ClamAV.RetryBatchSize,
		baseInterval: time.Duration(cfg.ClamAV.RetryIntervalMinutes) * time.Minute,
		staleAfter:   staleAfter,
	}, nil
}

func StartRetryWorker(ctx context.Context) error {
	service, err := NewRetryService()
	if err != nil || service == nil {
		return err
	}
	go service.start(ctx)
	logger.Info("clamav_retry_worker_started", "max_attempts", service.maxAttempts, "base_interval", service.baseInterval.String(), "batch_size", service.batchSize)
	return nil
}

func (s *RetryService) start(ctx context.Context) {
	run := func(now time.Time) {
		report, err := s.RunOnce(ctx, now)
		if err != nil {
			logger.Error("clamav_retry_worker_failed", "error", err)
			return
		}
		if report.Scanned > 0 || report.Recovered > 0 {
			logger.Info("clamav_retry_batch_completed", "recovered", report.Recovered, "scanned", report.Scanned, "clean", report.Clean, "infected", report.Infected, "failed", report.Failed, "exhausted", report.Exhausted)
		}
	}
	run(time.Now())
	ticker := time.NewTicker(retryWorkerPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			run(now)
		}
	}
}

func (s *RetryService) RunOnce(ctx context.Context, now time.Time) (RetryReport, error) {
	var report RetryReport
	recovered, err := s.files.RequeueStaleScanRetries(now.Add(-s.staleAfter), now)
	if err != nil {
		return report, err
	}
	report.Recovered = int(recovered)
	candidates, err := s.files.ListScanRetryCandidates(now, s.maxAttempts, s.batchSize)
	if err != nil {
		return report, err
	}
	for _, version := range candidates {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		claimed, err := s.files.ClaimScanRetry(version.ID, version.ScanRetryCount, now)
		if err != nil {
			return report, err
		}
		if !claimed {
			continue
		}
		attempt := version.ScanRetryCount + 1
		result := s.scan(ctx, filepath.Join(s.storageRoot, filepath.FromSlash(version.StorageKey)))
		var nextRetryAt *time.Time
		if result.Status == StatusScanError && attempt < s.maxAttempts {
			next := now.Add(retryDelay(s.baseInterval, attempt))
			nextRetryAt = &next
		}
		completed, err := s.files.CompleteScanRetry(version.ID, result.Status, result.Message, nextRetryAt)
		if err != nil {
			return report, err
		}
		if !completed {
			continue
		}
		report.Scanned++
		switch result.Status {
		case StatusClean:
			report.Clean++
		case StatusInfected:
			report.Infected++
			logger.Error("clamav_infected_file_detected", "version_id", version.ID, "workspace_id", version.WorkspaceID, "attempt", attempt)
			enqueueClamAVAlert(ctx, "clamav:infected", "发现病毒文件", version, attempt)
		default:
			report.Failed++
			if attempt >= s.maxAttempts {
				report.Exhausted++
				logger.Error("clamav_scan_retries_exhausted", "version_id", version.ID, "workspace_id", version.WorkspaceID, "attempts", attempt)
				enqueueClamAVAlert(ctx, "clamav:retry_exhausted", "病毒扫描重试已耗尽", version, attempt)
			}
		}
	}
	return report, nil
}

func enqueueClamAVAlert(ctx context.Context, eventType, title string, version model.FileVersion, attempt int) {
	_, err := notification.Publish(ctx, notification.Event{
		Key: fmt.Sprintf("%s:%d:%d", eventType, version.ID, attempt), Type: eventType, Severity: "critical", Title: title,
		Content: fmt.Sprintf("工作空间 %d 的文件版本 %d 需要管理员处理。", version.WorkspaceID, version.ID),
		Payload: map[string]any{"workspace_id": version.WorkspaceID, "version_id": version.ID, "attempt": attempt},
	})
	if err != nil {
		logger.Error("clamav_notification_enqueue_failed", "event_type", eventType, "version_id", version.ID, "error", err)
	}
	workspaceID := version.WorkspaceID
	if _, userErr := notification.PublishUser(ctx, notification.UserEvent{
		Key: fmt.Sprintf("%s:user:%d:%d", eventType, version.ID, attempt), UserID: version.CreatedBy, WorkspaceID: &workspaceID,
		Type: eventType, Category: notification.UserCategorySecurity, Severity: "critical", Title: title,
		Content:    "你上传的文件未通过安全扫描，请联系管理员处理。",
		TargetType: "node", TargetID: fmt.Sprint(version.NodeID),
	}); userErr != nil {
		logger.Error("clamav_user_notification_enqueue_failed", "event_type", eventType, "version_id", version.ID, "error", userErr)
	}
}

func retryDelay(base time.Duration, attempt int) time.Duration {
	if base <= 0 {
		base = 5 * time.Minute
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := base
	for current := 1; current < attempt && delay < 24*time.Hour; current++ {
		delay *= 2
	}
	if delay > 24*time.Hour {
		return 24 * time.Hour
	}
	return delay
}

func CurrentRetryStatus() (RetryStatus, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return RetryStatus{}, errors.New("configuration is not loaded")
	}
	status := RetryStatus{
		Enabled: cfg.ClamAV.Enabled(), MaxAttempts: cfg.ClamAV.RetryMaxAttempts,
		BaseIntervalMinutes: cfg.ClamAV.RetryIntervalMinutes, BatchSize: cfg.ClamAV.RetryBatchSize,
	}
	summary, err := dao.NewFileDAO().ScanRetrySummary(cfg.ClamAV.RetryMaxAttempts)
	if err != nil {
		return status, err
	}
	status.Retryable = summary.Retryable
	status.Pending = summary.Pending
	status.Exhausted = summary.Exhausted
	status.Infected = summary.Infected
	status.NextRetryAt = summary.NextRetryAt
	return status, nil
}
