/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package lifecycle

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	auditexport "file-share-manager/server/internal/service/auditexport"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/service/reconcile"
	"file-share-manager/server/internal/storage"
)

type Service struct {
	uploads *dao.UploadDAO
	shares  *dao.ShareDAO
	nodes   *dao.NodeDAO
	batches *dao.BatchDownloadDAO
}

func NewService() *Service {
	return &Service{uploads: dao.NewUploadDAO(), shares: dao.NewShareDAO(), nodes: dao.NewNodeDAO(), batches: dao.NewBatchDownloadDAO()}
}

// Start runs maintenance in a background goroutine and stops with ctx.
func (s *Service) Start(ctx context.Context) {
	interval := 15 * time.Minute
	trashRetention := 30 * 24 * time.Hour
	if cfg := config.GetConfig(); cfg != nil {
		interval = time.Duration(cfg.Lifecycle.IntervalMinutes) * time.Minute
		trashRetention = time.Duration(cfg.Lifecycle.TrashRetentionDays) * 24 * time.Hour
	}
	ticker := time.NewTicker(interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if err := s.RunOnce(now, trashRetention); err != nil {
					logger.Error("lifecycle_cleanup_failed", "error", err)
				}
			}
		}
	}()
}

func (s *Service) RunOnce(now time.Time, trashRetention time.Duration) error {
	uploadIDs, err := s.uploads.ExpireSessions(now)
	if err != nil {
		return err
	}
	expiringShares, err := s.shares.ListExpiringSoon(now, now.Add(24*time.Hour))
	if err != nil {
		return err
	}
	for _, share := range expiringShares {
		publishShareUserNotification("share:expiring", "外链即将过期", "你的外链“"+share.Name+"”将在 24 小时内过期。", share, "warning")
	}
	expiredShares, err := s.shares.Expire(now)
	if err != nil {
		return err
	}
	for _, share := range expiredShares {
		publishShareUserNotification("share:expired", "外链已过期", "你的外链“"+share.Name+"”已过期。", share, "info")
	}
	versions, err := s.nodes.PurgeExpiredTrash(now.Add(-trashRetention), now)
	if err != nil {
		return err
	}
	store, storeErr := configuredStorage()
	if storeErr != nil {
		return storeErr
	}
	for _, id := range uploadIDs {
		if err := store.RemoveUpload(id); err != nil {
			logger.Warn("remove_expired_upload_staging_failed", "upload_id", id, "error", err)
		}
	}
	reader, readerErr := storage.NewConfiguredVersionReader(context.Background(), config.GetConfig())
	if readerErr != nil {
		return readerErr
	}
	removed := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		storageClass := strings.ToLower(strings.TrimSpace(version.StorageClass))
		if storageClass == "" {
			storageClass = "standard"
		}
		identity := storageClass + "\x00" + version.StorageKey
		if _, exists := removed[identity]; exists {
			continue
		}
		removed[identity] = struct{}{}
		if err := reader.Remove(storageClass, version.StorageKey); err != nil {
			logger.Warn("remove_trash_object_failed", "storage_key", version.StorageKey, "storage_class", version.StorageClass, "error", err)
		}
	}
	archiveIDs, err := s.batches.Expire(now)
	if err != nil {
		return err
	}
	for _, id := range archiveIDs {
		if err := store.RemoveBatchArchive(id); err != nil {
			logger.Warn("remove_expired_batch_archive_failed", "job_id", id, "error", err)
		}
	}
	exportPaths, err := auditexport.DefaultService().Expire(now)
	if err != nil {
		return err
	}
	for _, path := range exportPaths {
		if filepath.Base(path) == "." || filepath.Base(path) == ".." {
			continue
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Warn("remove_expired_audit_export_failed", "path", filepath.Base(path), "error", err)
		}
	}
	batchSize := 100
	if cfg := config.GetConfig(); cfg != nil {
		batchSize = cfg.Lifecycle.ReconcileBatchSize
	}
	reconcileReport, err := reconcile.PurgeDueQuarantines(context.Background(), now, batchSize)
	if err != nil {
		return err
	}
	for _, failure := range reconcileReport.Failed {
		logger.Warn("purge_quarantined_object_failed", "storage_key", failure.StorageKey, "error", failure.Error)
	}
	return nil
}

func publishShareUserNotification(eventType, title, content string, share model.Share, severity string) {
	workspaceID := share.WorkspaceID
	if _, err := notification.PublishUser(context.Background(), notification.UserEvent{
		Key: eventType + ":" + strconv.FormatUint(uint64(share.ID), 10), UserID: share.CreatedBy, WorkspaceID: &workspaceID,
		Type: eventType, Category: notification.UserCategoryShare, Severity: severity, Title: title, Content: content,
		TargetType: "share", TargetID: strconv.FormatUint(uint64(share.ID), 10),
	}); err != nil {
		logger.Warn("share_user_notification_publish_failed", "event_type", eventType, "share_id", share.ID, "error", err)
	}
}

func configuredStorage() (*storage.POSIX, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	return storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
}
