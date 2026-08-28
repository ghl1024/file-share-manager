/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/storage"
)

type archiveDAO interface {
	ListCandidates(cutoff time.Time, limit int) ([]dao.ArchiveCandidate, error)
	Complete(candidate dao.ArchiveCandidate, cutoff time.Time) error
	RecordFailure(candidate dao.ArchiveCandidate, message string) error
}

type Service struct {
	dao     archiveDAO
	primary *storage.POSIX
	archive storage.ArchiveStorage
	prefix  string
}

func NewConfiguredService(ctx context.Context) (*Service, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	if !cfg.Archive.Enabled || !strings.EqualFold(cfg.Storage.Mode, "local") {
		return nil, nil
	}
	primary, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	configured, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return nil, err
	}
	archiveStore, ok := configured.(storage.ArchiveStorage)
	if !ok {
		return nil, errors.New("configured archive storage does not support deletion")
	}
	return &Service{dao: dao.NewArchiveDAO(), primary: primary, archive: archiveStore, prefix: cfg.Archive.Prefix}, nil
}

func StartGlobal(ctx context.Context) error {
	service, err := NewConfiguredService(ctx)
	if err != nil || service == nil {
		return err
	}
	cfg := config.GetConfig()
	interval := time.Duration(cfg.Lifecycle.IntervalMinutes) * time.Minute
	after := time.Duration(cfg.Archive.AfterDays) * 24 * time.Hour
	batchSize := cfg.Archive.BatchSize
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case now := <-ticker.C:
				if _, runErr := service.RunOnce(ctx, now.Add(-after), batchSize); runErr != nil {
					logger.Error("archive_worker_failed", "error", runErr)
				}
			}
		}
	}()
	logger.Info("archive_worker_started", "after_days", cfg.Archive.AfterDays, "batch_size", batchSize)
	return nil
}

func (s *Service) RunOnce(ctx context.Context, cutoff time.Time, limit int) (int, error) {
	candidates, err := s.dao.ListCandidates(cutoff, limit)
	if err != nil {
		return 0, err
	}
	completed := 0
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return completed, err
		}
		migrated, err := s.archiveCandidate(candidate, cutoff)
		if err != nil {
			_ = s.dao.RecordFailure(candidate, err.Error())
			logger.Warn("archive_object_failed", "workspace_id", candidate.WorkspaceID, "storage_key", candidate.StorageKey, "error", err)
			continue
		}
		if migrated {
			completed++
		}
	}
	return completed, nil
}

func (s *Service) archiveCandidate(candidate dao.ArchiveCandidate, cutoff time.Time) (bool, error) {
	file, err := s.primary.OpenObject(candidate.StorageKey)
	if err != nil {
		return false, fmt.Errorf("open primary object: %w", err)
	}
	archiveKey := storage.ArchiveObjectKey(s.prefix, candidate.StorageKey)
	size, digest, putErr := s.archive.Put(archiveKey, file)
	closeErr := file.Close()
	if closeErr != nil && putErr == nil {
		putErr = closeErr
	}
	if errors.Is(putErr, storage.ErrBackupImmutable) || errors.Is(putErr, storage.ErrObjectAlreadyExists) {
		size, digest, putErr = verifyArchivedObject(s.archive, archiveKey)
	}
	if putErr != nil {
		return false, fmt.Errorf("write archive object: %w", putErr)
	}
	if size != candidate.Size || !strings.EqualFold(digest, candidate.SHA256) {
		return false, fmt.Errorf("archive verification mismatch: size=%d sha256=%s", size, digest)
	}
	if err := s.dao.Complete(candidate, cutoff); err != nil {
		if errors.Is(err, dao.ErrArchiveCandidateChanged) {
			// Another process may have committed this immutable object already.
			// Never delete it here; doing so could break the winning transaction.
			return false, nil
		}
		return false, fmt.Errorf("commit archive metadata: %w", err)
	}
	if err := s.primary.RemoveObject(candidate.StorageKey); err != nil {
		logger.Warn("archive_primary_cleanup_failed", "workspace_id", candidate.WorkspaceID, "storage_key", candidate.StorageKey, "error", err)
	}
	logger.Info("archive_object_completed", "workspace_id", candidate.WorkspaceID, "storage_key", candidate.StorageKey, "size", candidate.Size)
	return true, nil
}

func verifyArchivedObject(store storage.BackupStorage, key string) (int64, string, error) {
	reader, err := store.Get(key)
	if err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(hash, reader)
	closeErr := reader.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
