/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditarchive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/storage"

	"github.com/google/uuid"
)

type Service struct {
	archives       *dao.AuditArchiveDAO
	workerArchives *dao.AuditArchiveDAO
	runMu          sync.Mutex
}

var (
	defaultMu      sync.Mutex
	defaultService *Service
)

// DefaultService is initialized after the database connection is ready and is
// shared by HTTP handlers and the background worker.
func DefaultService() *Service {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultService == nil {
		defaultService = NewService()
	}
	return defaultService
}

func NewService() *Service {
	return &Service{
		archives:       dao.NewAuditArchiveDAO(),
		workerArchives: dao.NewAuditArchiveWorkerDAO(),
	}
}

func (s *Service) Start(ctx context.Context) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Audit.ArchiveEnabled {
		return
	}
	if err := s.workerArchives.RequeueInterrupted(); err != nil {
		logger.Error("audit_archive_requeue_failed", "error", err)
	}
	interval := time.Duration(cfg.Audit.ArchiveIntervalMinutes) * time.Minute
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := s.RunOnce(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("audit_archive_run_failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) List(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.AuditArchive], error) {
	return s.archives.ListPage(workspaceID, page, pageSize)
}

func (s *Service) RunOnce(ctx context.Context, now time.Time) error {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Audit.ArchiveEnabled {
		return nil
	}
	store, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return err
	}
	cutoff := now.AddDate(0, 0, -cfg.Audit.HotRetentionDays)
	keys, err := s.workerArchives.CandidateStreamKeys(cutoff, 100)
	if err != nil {
		return err
	}
	for _, streamKey := range keys {
		candidate, err := s.workerArchives.BuildCandidate(streamKey, cutoff, cfg.Audit.ArchiveBatchSize)
		if err != nil {
			return err
		}
		if candidate == nil {
			continue
		}
		candidate.Archive.ID = uuid.NewString()
		candidate.Archive.CreatedAt = candidate.Archive.CreatedAt.Truncate(time.Millisecond)
		candidate.Archive.ObjectKey = archiveObjectKey(cfg.Audit.ArchivePrefix, &candidate.Archive)
		manifest, err := candidate.Archive.Manifest().WithHash()
		if err != nil {
			return err
		}
		candidate.Archive.ManifestHash = manifest.ManifestHash
		if err := s.workerArchives.Create(&candidate.Archive); err != nil {
			return err
		}
	}
	ids, err := s.workerArchives.ListProcessableIDs(20)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.process(ctx, store, id, now, cfg.Backup.ManifestEncryptionKey); err != nil {
			logger.Warn("audit_archive_task_failed", "archive_id", id, "error", err)
		}
	}
	return nil
}

func (s *Service) process(ctx context.Context, store storage.BackupStorage, id string, now time.Time, encodedKey string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	claimed, err := s.workerArchives.Claim(id, now)
	if err != nil || !claimed {
		return err
	}
	archive, err := s.workerArchives.Get(id)
	if err != nil || archive == nil {
		return err
	}
	events, err := s.workerArchives.LoadEvents(archive)
	if err != nil {
		return s.fail(id, err)
	}
	data, err := encodeArchive(archive, events, encodedKey)
	if err != nil {
		return s.fail(id, err)
	}
	objectSize, objectSHA256, err := store.Put(archive.ObjectKey, bytes.NewReader(data))
	if errors.Is(err, storage.ErrBackupImmutable) || errors.Is(err, storage.ErrObjectAlreadyExists) {
		reader, getErr := store.Get(archive.ObjectKey)
		if getErr != nil {
			return s.fail(id, getErr)
		}
		data, err = io.ReadAll(io.LimitReader(reader, int64(len(data))+1))
		closeErr := reader.Close()
		if err != nil {
			return s.fail(id, err)
		}
		if closeErr != nil {
			return s.fail(id, closeErr)
		}
		objectSize = int64(len(data))
		objectSHA256 = objectDigest(data)
	} else if err != nil {
		return s.fail(id, err)
	}
	reader, err := store.Get(archive.ObjectKey)
	if err != nil {
		return s.fail(id, err)
	}
	verifiedData, readErr := io.ReadAll(io.LimitReader(reader, objectSize+1))
	closeErr := reader.Close()
	if readErr != nil {
		return s.fail(id, readErr)
	}
	if closeErr != nil {
		return s.fail(id, closeErr)
	}
	if int64(len(verifiedData)) != objectSize || objectDigest(verifiedData) != objectSHA256 {
		return s.fail(id, errors.New("audit archive object checksum mismatch"))
	}
	if err := verifyArchive(verifiedData, archive, encodedKey); err != nil {
		return s.fail(id, err)
	}
	if err := s.workerArchives.Finalize(id, objectSize, objectSHA256, now); err != nil {
		return s.fail(id, err)
	}
	logger.Info("audit_archive_completed", "archive_id", id, "stream_key", archive.StreamKey, "from_seq", archive.FromSeq, "to_seq", archive.ToSeq, "events", archive.EventCount)
	return nil
}

func (s *Service) fail(id string, failure error) error {
	if updateErr := s.workerArchives.Fail(id, failure); updateErr != nil {
		return fmt.Errorf("%v; record audit archive failure: %w", failure, updateErr)
	}
	return failure
}

func archiveObjectKey(prefix string, archive *model.AuditArchive) string {
	stream := strings.NewReplacer(":", "-", "/", "-", "\\", "-").Replace(archive.StreamKey)
	return fmt.Sprintf("%s%s/%020d-%020d-%s.jsonl.gz.enc", prefix, stream, archive.FromSeq, archive.ToSeq, archive.ID)
}
