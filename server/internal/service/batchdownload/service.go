/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package batchdownload

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/storage"
)

type Service struct {
	jobs      *dao.BatchDownloadDAO
	files     *dao.FileDAO
	queue     chan string
	startOnce sync.Once
	store     func() (*storage.POSIX, error)
	reader    func() (*storage.VersionReader, error)
}

var (
	defaultMu      sync.Mutex
	defaultService *Service
)

func NewService() *Service {
	return &Service{jobs: dao.NewBatchDownloadDAO(), files: dao.NewFileDAO(), queue: make(chan string, 1024), store: configuredStorage, reader: configuredVersionReader}
}

// DefaultService is initialized after the database connection is ready and is
// shared by HTTP handlers and background workers.
func DefaultService() *Service {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultService == nil {
		defaultService = NewService()
	}
	return defaultService
}

func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		if err := s.jobs.RequeueInterrupted(); err != nil {
			logger.Error("batch_download_requeue_failed", "error", err)
		}
		workers := 2
		if cfg := config.GetConfig(); cfg != nil {
			workers = cfg.BatchDownload.WorkerCount
		}
		for index := 0; index < workers; index++ {
			go s.worker(ctx)
		}
		go s.dispatch(ctx)
	})
}

func (s *Service) Enqueue(id string) {
	select {
	case s.queue <- id:
	default:
		// The dispatcher will pick up durable queued jobs when the in-memory
		// buffer is saturated.
	}
}

func (s *Service) OpenArchive(id string) (*os.File, error) {
	store, err := s.store()
	if err != nil {
		return nil, err
	}
	return store.OpenBatchArchive(id)
}

func (s *Service) RemoveArchive(id string) error {
	store, err := s.store()
	if err != nil {
		return err
	}
	return store.RemoveBatchArchive(id)
}

func (s *Service) dispatch(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.enqueueQueued()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.enqueueQueued()
		}
	}
}

func (s *Service) enqueueQueued() {
	ids, err := s.jobs.ListQueuedIDs(1000)
	if err != nil {
		logger.Error("batch_download_dispatch_failed", "error", err)
		return
	}
	for _, id := range ids {
		s.Enqueue(id)
	}
}

func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.queue:
			s.process(ctx, id)
		}
	}
}

func (s *Service) process(ctx context.Context, id string) {
	claimed, err := s.jobs.Claim(id, time.Now())
	if err != nil {
		logger.Error("batch_download_claim_failed", "job_id", id, "error", err)
		return
	}
	if !claimed {
		return
	}
	if err := s.buildArchive(ctx, id); err != nil {
		if errors.Is(err, context.Canceled) {
			return
		}
		message := strings.TrimSpace(err.Error())
		if len(message) > 1000 {
			message = message[:1000]
		}
		if updateErr := s.jobs.Fail(id, message); updateErr != nil {
			logger.Error("batch_download_fail_update_failed", "job_id", id, "error", updateErr)
		}
		logger.Error("batch_download_build_failed", "job_id", id, "error", err)
	}
}

func (s *Service) buildArchive(ctx context.Context, id string) error {
	items, err := s.jobs.ListItems(id)
	if err != nil {
		return fmt.Errorf("load task snapshot: %w", err)
	}
	if len(items) == 0 {
		return errors.New("task snapshot contains no files")
	}
	store, err := s.store()
	if err != nil {
		return err
	}
	reader, err := s.reader()
	if err != nil {
		return err
	}
	destination, tmpPath, err := store.CreateBatchArchive(id)
	if err != nil {
		return fmt.Errorf("create temporary archive: %w", err)
	}
	committed := false
	defer func() {
		_ = destination.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	archive := zip.NewWriter(destination)
	var processedBytes int64
	for index, item := range items {
		if err := ctx.Err(); err != nil {
			_ = archive.Close()
			return err
		}
		source, err := reader.Open(item.StorageClass, item.StorageKey)
		if err != nil {
			_ = archive.Close()
			return fmt.Errorf("open snapshot file %q: %w", item.RelativePath, err)
		}
		header := &zip.FileHeader{Name: item.RelativePath, Method: zip.Store}
		header.SetModTime(time.Now())
		entry, err := archive.CreateHeader(header)
		if err == nil {
			_, err = io.Copy(entry, source)
		}
		closeErr := source.Close()
		if err != nil {
			_ = archive.Close()
			return fmt.Errorf("write snapshot file %q: %w", item.RelativePath, err)
		}
		if closeErr != nil {
			_ = archive.Close()
			return fmt.Errorf("close snapshot file %q: %w", item.RelativePath, closeErr)
		}
		processedBytes += item.Size
		_ = s.files.TouchAccessByStorageKey(item.StorageKey, time.Now())
		if err := s.jobs.UpdateProgress(id, index+1, processedBytes); err != nil {
			_ = archive.Close()
			return fmt.Errorf("update task progress: %w", err)
		}
	}
	if err := archive.Close(); err != nil {
		return fmt.Errorf("finalize archive: %w", err)
	}
	if err := destination.Sync(); err != nil {
		return fmt.Errorf("sync archive: %w", err)
	}
	if err := destination.Close(); err != nil {
		return fmt.Errorf("close archive: %w", err)
	}
	archiveSize, err := store.CommitBatchArchive(id, tmpPath)
	if err != nil {
		return fmt.Errorf("publish archive: %w", err)
	}
	committed = true
	completedAt := time.Now()
	retention := 24 * time.Hour
	if cfg := config.GetConfig(); cfg != nil {
		retention = time.Duration(cfg.BatchDownload.RetentionHours) * time.Hour
	}
	if err := s.jobs.Complete(id, archiveSize, completedAt, completedAt.Add(retention)); err != nil {
		_ = store.RemoveBatchArchive(id)
		return fmt.Errorf("complete task: %w", err)
	}
	if job, loadErr := s.jobs.GetByID(id); loadErr != nil {
		logger.Warn("batch_download_user_notification_load_failed", "job_id", id, "error", loadErr)
	} else if job != nil {
		workspaceID := job.WorkspaceID
		if _, notifyErr := notification.PublishUser(ctx, notification.UserEvent{
			Key: "batch-download:completed:" + job.ID, UserID: job.CreatedBy, WorkspaceID: &workspaceID,
			Type: "task:batch_download_completed", Category: notification.UserCategoryTask, Severity: "info",
			Title: "批量下载已完成", Content: fmt.Sprintf("“%s”已生成，可在 24 小时内下载。", job.Name),
			TargetType: "batch_download", TargetID: job.ID,
		}); notifyErr != nil {
			logger.Warn("batch_download_user_notification_publish_failed", "job_id", id, "error", notifyErr)
		}
	}
	logger.Info("batch_download_completed", "job_id", id, "files", len(items), "archive_size", archiveSize)
	return nil
}

func configuredStorage() (*storage.POSIX, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	return storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
}

func configuredVersionReader() (*storage.VersionReader, error) {
	return storage.NewConfiguredVersionReader(context.Background(), config.GetConfig())
}
