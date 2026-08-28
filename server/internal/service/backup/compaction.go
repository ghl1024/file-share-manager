/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package backup

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	compactionTriggerManual    = "manual_compaction"
	compactionTriggerScheduled = "scheduled_compaction"
	compactionCompletedFilter  = "workspace_id = ? AND `trigger` IN ? AND status = ?"
)

type CompactionStatus struct {
	Enabled              bool       `json:"enabled"`
	CheckIntervalMinutes int        `json:"check_interval_minutes"`
	IncrementalThreshold int        `json:"incremental_threshold"`
	CurrentIncrementals  int        `json:"current_incrementals"`
	Due                  bool       `json:"due"`
	ManualAvailable      bool       `json:"manual_available"`
	LastCompactionJobID  string     `json:"last_compaction_job_id,omitempty"`
	LastCompactionAt     *time.Time `json:"last_compaction_at,omitempty"`
}

func (s *Service) compactionStatus(workspaceID uint, latest *model.BackupJob) (CompactionStatus, error) {
	cfg := config.GetConfig()
	status := CompactionStatus{}
	if cfg != nil {
		status.Enabled = cfg.Backup.CompactionEnabled
		status.CheckIntervalMinutes = cfg.Backup.CompactionIntervalMin
		status.IncrementalThreshold = cfg.Backup.CompactionThreshold
	}
	depth, err := s.incrementalDepth(latest)
	if err != nil {
		return status, err
	}
	status.CurrentIncrementals = depth
	status.ManualAvailable = depth > 0
	status.Due = status.Enabled && depth >= status.IncrementalThreshold
	var last model.BackupJob
	// MySQL treats TRIGGER as a reserved word, so raw conditions must quote it.
	err = s.db.Where(compactionCompletedFilter, workspaceID, []string{compactionTriggerManual, compactionTriggerScheduled}, "complete").
		Order("created_at DESC, id DESC").First(&last).Error
	if err == nil {
		status.LastCompactionJobID = last.ID
		status.LastCompactionAt = last.CompletedAt
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return status, err
	}
	return status, nil
}

func (s *Service) incrementalDepth(latest *model.BackupJob) (int, error) {
	return incrementalDepthWithLookup(latest, func(workspaceID uint, id string) (*model.BackupJob, error) {
		return s.jobs.Get(workspaceID, id)
	})
}

func incrementalDepthWithLookup(latest *model.BackupJob, lookup func(uint, string) (*model.BackupJob, error)) (int, error) {
	depth := 0
	seen := make(map[string]struct{})
	current := latest
	for current != nil && current.Kind == "incremental" {
		if current.ID == "" || current.ParentID == "" {
			return 0, errors.New("incremental backup parent is missing")
		}
		if _, exists := seen[current.ID]; exists {
			return 0, errors.New("backup parent chain contains a cycle")
		}
		seen[current.ID] = struct{}{}
		depth++
		parent, err := lookup(current.WorkspaceID, current.ParentID)
		if err != nil {
			return 0, err
		}
		if parent == nil || parent.Status != "complete" {
			return 0, errors.New("incremental backup parent is missing or incomplete")
		}
		current = parent
	}
	if current != nil && current.Kind != "baseline" {
		return 0, errors.New("backup chain does not start with a baseline")
	}
	return depth, nil
}

func (s *Service) CompactManual(ctx context.Context, workspaceID, actorID uint) (*model.BackupJob, error) {
	return s.compact(ctx, workspaceID, actorID, compactionTriggerManual, 1)
}

func (s *Service) compact(ctx context.Context, workspaceID, actorID uint, trigger string, minimumDepth int) (*model.BackupJob, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, ErrBackupBackendUnsupported
	}
	if _, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup); err != nil {
		return nil, mapBackupStorageError(err)
	}
	if _, err := decodeManifestEncryptionKey(cfg.Backup.ManifestEncryptionKey); err != nil {
		return nil, err
	}
	job, source, err := s.startCompactionJob(workspaceID, actorID, trigger, minimumDepth)
	if err != nil {
		return nil, err
	}
	if err := s.buildCompactedBaseline(ctx, cfg, job, source); err != nil {
		logger.Error("backup_compaction_failed", "workspace_id", workspaceID, "job_id", job.ID, "source_job_id", source.ID, "error", err)
		_ = s.jobs.Update(job.ID, map[string]any{"status": "failed", "error_message": "备份基线压缩失败", "completed_at": time.Now()})
		enqueueBackupAlert(ctx, "backup:compaction_failed", "备份基线压缩失败", job, "请检查源备份链、对象完整性和备份存储写入权限。")
		return nil, err
	}
	return s.jobs.Get(workspaceID, job.ID)
}

func (s *Service) startCompactionJob(workspaceID, actorID uint, trigger string, minimumDepth int) (*model.BackupJob, *model.BackupJob, error) {
	if trigger != compactionTriggerManual && trigger != compactionTriggerScheduled {
		return nil, nil, errors.New("backup compaction trigger is invalid")
	}
	if minimumDepth < 1 {
		minimumDepth = 1
	}
	now := time.Now()
	job := &model.BackupJob{
		ID: uuid.NewString(), WorkspaceID: workspaceID, CreatedBy: actorID, Kind: "baseline",
		Trigger: trigger, Status: "running", StartedAt: &now,
	}
	var source model.BackupJob
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		var running int64
		if err := tx.Model(&model.BackupJob{}).Where("workspace_id = ? AND status = ?", workspaceID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrBackupInProgress
		}
		if err := tx.Where("workspace_id = ? AND status = ?", workspaceID, "complete").Order("created_at DESC, id DESC").First(&source).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrBackupBaselineMissing
			}
			return err
		}
		depth, err := incrementalDepthWithLookup(&source, func(workspaceID uint, id string) (*model.BackupJob, error) {
			var parent model.BackupJob
			if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&parent).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, nil
				}
				return nil, err
			}
			return &parent, nil
		})
		if err != nil {
			return err
		}
		if depth < minimumDepth {
			return ErrBackupCompactionNotNeeded
		}
		job.CompactedFromID = source.ID
		return tx.Create(job).Error
	})
	if err != nil {
		return nil, nil, err
	}
	return job, &source, nil
}

func (s *Service) buildCompactedBaseline(ctx context.Context, cfg *config.Config, job, source *model.BackupJob) error {
	if _, err := s.Verify(ctx, source.WorkspaceID, source.ID); err != nil {
		return err
	}
	store, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return err
	}
	manifests, err := s.loadManifestChain(store, source)
	if err != nil {
		return err
	}
	if len(manifests) == 0 || manifests[len(manifests)-1].Metadata == nil {
		return ErrMetadataSnapshotMissing
	}
	if err := validateMetadataObjects(manifests); err != nil {
		return err
	}
	objects, err := compactionObjects(manifests)
	if err != nil {
		return err
	}
	manifest := Manifest{
		BackupManifest: storage.BackupManifest{
			ID: job.ID, Kind: "baseline", Status: "complete", WorkspaceID: job.WorkspaceID,
			ChangeLogEnd: source.ChangeLogEnd, CreatedAt: time.Now(),
		},
		Trigger: job.Trigger, CompactedFromID: source.ID,
		Metadata: manifests[len(manifests)-1].Metadata,
	}
	targetKeys := make(map[string]string)
	for _, sourceObject := range objects {
		if err := ctx.Err(); err != nil {
			return err
		}
		targetKey, exists := targetKeys[sourceObject.StorageKey]
		if !exists {
			targetKey = filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "objects", filepath.Base(sourceObject.StorageKey)))
			reader, err := store.Get(sourceObject.BackupKey)
			if err != nil {
				return err
			}
			written, digest, putErr := store.Put(targetKey, reader)
			_ = reader.Close()
			if putErr != nil {
				return putErr
			}
			if written != sourceObject.Size || (sourceObject.SHA256 != "" && !strings.EqualFold(digest, sourceObject.SHA256)) {
				return fmt.Errorf("compacted backup object %d checksum mismatch", sourceObject.VersionID)
			}
			targetKeys[sourceObject.StorageKey] = targetKey
		}
		entry := sourceObject
		entry.BackupKey = targetKey
		manifest.Objects = append(manifest.Objects, entry)
		manifest.TotalBytes += entry.Size
	}
	manifest.ObjectCount = len(manifest.Objects)
	manifestData, _, err := encodeProtectedManifest(manifest, cfg.Backup.ManifestEncryptionKey)
	if err != nil {
		return err
	}
	manifestKey := filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "manifest.json.gz.enc"))
	if _, _, err := store.Put(manifestKey, bytes.NewReader(manifestData)); err != nil {
		return err
	}
	job.Status = "complete"
	job.ManifestKey = manifestKey
	job.ObjectCount = manifest.ObjectCount
	job.TotalBytes = manifest.TotalBytes
	job.ChangeLogEnd = source.ChangeLogEnd
	if _, err := s.verifyJob(ctx, job, make(map[string]struct{})); err != nil {
		return err
	}
	completed := time.Now()
	return s.jobs.Update(job.ID, map[string]any{
		"status": "complete", "manifest_key": manifestKey, "object_count": manifest.ObjectCount,
		"total_bytes": manifest.TotalBytes, "change_log_end": source.ChangeLogEnd,
		"verify_status": "valid", "verified_at": completed, "verify_error": "",
		"completed_at": completed, "error_message": "",
	})
}

func compactionObjects(manifests []Manifest) ([]ObjectEntry, error) {
	if len(manifests) == 0 || manifests[len(manifests)-1].Metadata == nil {
		return nil, ErrMetadataSnapshotMissing
	}
	available := make(map[string]ObjectEntry)
	for _, manifest := range manifests {
		for _, object := range manifest.Objects {
			available[versionObjectKey(object.VersionID, object.SHA256)] = object
		}
	}
	result := make([]ObjectEntry, 0, len(manifests[len(manifests)-1].Metadata.Versions))
	for _, version := range manifests[len(manifests)-1].Metadata.Versions {
		object, exists := available[versionObjectKey(version.ID, version.SHA256)]
		if !exists {
			return nil, fmt.Errorf("file version %d is not present in the backup chain", version.ID)
		}
		result = append(result, object)
	}
	return result, nil
}

func (s *Service) Start(ctx context.Context) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Backup.CompactionEnabled {
		return
	}
	interval := time.Duration(cfg.Backup.CompactionIntervalMin) * time.Minute
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			if err := s.RunCompactionOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("backup_compaction_run_failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (s *Service) RunCompactionOnce(ctx context.Context) error {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Backup.CompactionEnabled {
		return nil
	}
	var workspaceIDs []uint
	if err := s.db.Model(&model.Workspace{}).Where("status = ?", 1).Order("id ASC").Pluck("id", &workspaceIDs).Error; err != nil {
		return err
	}
	var firstErr error
	for _, workspaceID := range workspaceIDs {
		if err := ctx.Err(); err != nil {
			return err
		}
		job, err := s.compact(ctx, workspaceID, 0, compactionTriggerScheduled, cfg.Backup.CompactionThreshold)
		switch {
		case err == nil:
			logger.Info("backup_compaction_completed", "workspace_id", workspaceID, "job_id", job.ID, "source_job_id", job.CompactedFromID)
		case errors.Is(err, ErrBackupCompactionNotNeeded), errors.Is(err, ErrBackupBaselineMissing), errors.Is(err, ErrBackupInProgress):
			continue
		default:
			logger.Error("backup_compaction_workspace_failed", "workspace_id", workspaceID, "error", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}
