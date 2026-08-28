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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrBackupBackendUnsupported = errors.New("configured backup backend is not supported")
var ErrBackupBaselineMissing = errors.New("a complete baseline backup is required")
var ErrBackupInProgress = errors.New("another backup job is already running")
var ErrBackupNotRetryable = errors.New("backup job is not retryable")
var ErrBackupCompactionNotNeeded = errors.New("backup chain does not need compaction")
var ErrRestoreDrillInProgress = errors.New("another restore drill is already running")
var ErrRestoreObjectNotFound = errors.New("backup object is not present in manifest")
var ErrRestoreNameConflict = errors.New("restore destination already contains an active node with the same name")

func mapBackupStorageError(err error) error {
	if errors.Is(err, storage.ErrBackupStorageConfig) {
		return ErrBackupBackendUnsupported
	}
	return err
}

type ObjectEntry struct {
	VersionID  uint   `json:"version_id"`
	NodeID     uint   `json:"node_id"`
	Name       string `json:"name"`
	StorageKey string `json:"storage_key"`
	BackupKey  string `json:"backup_key"`
	Size       int64  `json:"size"`
	SHA256     string `json:"sha256"`
	Extension  string `json:"extension,omitempty"`
	Mime       string `json:"detected_mime,omitempty"`
	RiskLevel  string `json:"risk_level,omitempty"`
	ScanStatus string `json:"scan_status,omitempty"`
	Encrypted  bool   `json:"encrypted"`
}

type Manifest struct {
	storage.BackupManifest
	Trigger         string            `json:"trigger,omitempty"`
	CompactedFromID string            `json:"compacted_from_id,omitempty"`
	Objects         []ObjectEntry     `json:"objects"`
	Changes         []model.ChangeLog `json:"changes,omitempty"`
	Metadata        *MetadataSnapshot `json:"metadata,omitempty"`
}

type Service struct {
	jobs *dao.BackupDAO
	db   *gorm.DB
}

type Health struct {
	Status          string                    `json:"status"`
	CheckedAt       time.Time                 `json:"checked_at"`
	ChainValid      bool                      `json:"chain_valid"`
	LatestBackup    *model.BackupJob          `json:"latest_backup,omitempty"`
	LatestComplete  *model.BackupJob          `json:"latest_complete,omitempty"`
	LatestDrill     *model.BackupRestoreDrill `json:"latest_drill,omitempty"`
	Alerts          []string                  `json:"alerts"`
	RecommendedNext string                    `json:"recommended_next,omitempty"`
	Compaction      CompactionStatus          `json:"compaction"`
}

func NewService() *Service { return &Service{jobs: dao.NewBackupDAO(), db: database.DB} }

func (s *Service) List(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.BackupJob], error) {
	return s.jobs.ListPage(workspaceID, page, pageSize)
}

func (s *Service) ListRestoreDrills(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.BackupRestoreDrill], error) {
	return s.jobs.ListDrillsPage(workspaceID, page, pageSize)
}

func (s *Service) Health(ctx context.Context, workspaceID uint) (*Health, error) {
	latest, err := s.jobs.Latest(workspaceID)
	if err != nil {
		return nil, err
	}
	latestComplete, err := s.jobs.LatestComplete(workspaceID)
	if err != nil {
		return nil, err
	}
	latestDrill, err := s.jobs.LatestDrill(workspaceID)
	if err != nil {
		return nil, err
	}
	health := &Health{
		Status: "healthy", CheckedAt: time.Now(), ChainValid: true,
		LatestBackup: latest, LatestComplete: latestComplete, LatestDrill: latestDrill, Alerts: []string{},
	}
	compaction, err := s.compactionStatus(workspaceID, latestComplete)
	if err != nil {
		return nil, err
	}
	health.Compaction = compaction
	if latest == nil {
		health.Status = "warning"
		health.ChainValid = false
		health.Alerts = append(health.Alerts, "当前工作空间尚未创建备份")
		health.RecommendedNext = "先创建一次基线备份"
		return health, nil
	}
	if latest.Status == "failed" {
		health.Status = "critical"
		health.Alerts = append(health.Alerts, "最新备份任务执行失败")
		health.RecommendedNext = "检查备份存储后重试失败任务"
	}
	if latestComplete == nil {
		health.ChainValid = false
		if health.Status != "critical" {
			health.Status = "warning"
		}
		health.Alerts = append(health.Alerts, "当前没有可恢复的完整备份点")
		if health.RecommendedNext == "" {
			health.RecommendedNext = "创建一次基线备份"
		}
	} else if latestComplete.VerifyStatus == "invalid" {
		health.Status = "critical"
		health.ChainValid = false
		health.Alerts = append(health.Alerts, "最新完整备份链最近一次校验失败")
		health.RecommendedNext = "检查备份清单与对象完整性"
	} else if latestComplete.VerifyStatus != "valid" {
		health.Status = "warning"
		health.ChainValid = false
		health.Alerts = append(health.Alerts, "最新完整备份链尚未执行完整校验")
		health.RecommendedNext = "校验最新完整备份或执行恢复演练"
	}
	if latestDrill == nil {
		if health.Status == "healthy" {
			health.Status = "warning"
		}
		health.Alerts = append(health.Alerts, "尚未执行过恢复演练")
		if health.RecommendedNext == "" {
			health.RecommendedNext = "对最新完整备份执行恢复演练"
		}
	} else if latestDrill.Status == "failed" {
		health.Status = "critical"
		health.Alerts = append(health.Alerts, "最近一次恢复演练失败")
		health.RecommendedNext = "检查恢复目标写入权限和备份对象完整性"
	}
	return health, nil
}

func (s *Service) CreateBaseline(ctx context.Context, workspaceID, actorID uint) (*model.BackupJob, error) {
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
	job, err := s.startJob(workspaceID, actorID, "baseline", nil)
	if err != nil {
		return nil, err
	}
	if err := s.buildBaseline(ctx, cfg, job); err != nil {
		_ = s.jobs.Update(job.ID, map[string]any{"status": "failed", "error_message": "备份任务执行失败", "completed_at": time.Now()})
		enqueueBackupAlert(ctx, "backup:failed", "基线备份执行失败", job, "请检查备份存储连通性和写入权限。")
		return nil, err
	}
	return s.jobs.Get(workspaceID, job.ID)
}

func (s *Service) CreateIncremental(ctx context.Context, workspaceID, actorID uint) (*model.BackupJob, error) {
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
	job, err := s.startJob(workspaceID, actorID, "incremental", func(parent *model.BackupJob) error {
		if parent == nil {
			return ErrBackupBaselineMissing
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := s.buildIncremental(ctx, cfg, job); err != nil {
		_ = s.jobs.Update(job.ID, map[string]any{"status": "failed", "error_message": "备份任务执行失败", "completed_at": time.Now()})
		enqueueBackupAlert(ctx, "backup:failed", "增量备份执行失败", job, "请检查备份链、存储连通性和写入权限。")
		return nil, err
	}
	return s.jobs.Get(workspaceID, job.ID)
}

// Retry preserves the failed attempt and creates a fresh immutable backup job.
// Reusing a failed job ID could overwrite partially written backup objects.
func (s *Service) Retry(ctx context.Context, workspaceID, actorID uint, jobID string) (*model.BackupJob, error) {
	failedJob, err := s.jobs.Get(workspaceID, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	kind, err := retryBackupKind(failedJob)
	if err != nil {
		return nil, err
	}
	if kind == "baseline" {
		return s.CreateBaseline(ctx, workspaceID, actorID)
	}
	return s.CreateIncremental(ctx, workspaceID, actorID)
}

func retryBackupKind(job *model.BackupJob) (string, error) {
	if job == nil {
		return "", gorm.ErrRecordNotFound
	}
	if job.Status != "failed" || (job.Kind != "baseline" && job.Kind != "incremental") {
		return "", ErrBackupNotRetryable
	}
	return job.Kind, nil
}

func (s *Service) RunRestoreDrill(ctx context.Context, workspaceID, actorID uint, jobID string) (*model.BackupRestoreDrill, error) {
	target, err := s.jobs.Get(workspaceID, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if target.Status != "complete" || target.ManifestKey == "" {
		return nil, errors.New("restore drill requires a complete backup job")
	}
	now := time.Now()
	drill := &model.BackupRestoreDrill{
		ID: uuid.NewString(), WorkspaceID: workspaceID, BackupJobID: target.ID,
		CreatedBy: actorID, Status: "running", StartedAt: &now,
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		var running int64
		if err := tx.Model(&model.BackupRestoreDrill{}).Where("workspace_id = ? AND status = ?", workspaceID, "running").Count(&running).Error; err != nil {
			return err
		}
		if running > 0 {
			return ErrRestoreDrillInProgress
		}
		return tx.Create(drill).Error
	}); err != nil {
		return nil, err
	}

	objectCount, totalBytes, drillErr := s.executeRestoreDrill(ctx, target, drill.ID)
	completedAt := time.Now()
	fields := map[string]any{
		"status": "complete", "object_count": objectCount, "total_bytes": totalBytes,
		"completed_at": completedAt, "error_message": "",
	}
	if drillErr != nil {
		fields["status"] = "failed"
		fields["error_message"] = "恢复演练执行失败"
		enqueueBackupAlert(ctx, "backup:restore_drill_failed", "备份恢复演练失败", target, "请检查恢复目标写入权限和备份对象完整性。")
	}
	if updateErr := s.jobs.UpdateDrill(drill.ID, fields); updateErr != nil {
		return nil, updateErr
	}
	result, getErr := s.jobs.GetDrill(workspaceID, drill.ID)
	if getErr != nil {
		return nil, getErr
	}
	return result, drillErr
}

func (s *Service) executeRestoreDrill(ctx context.Context, target *model.BackupJob, drillID string) (int, int64, error) {
	if _, err := s.Verify(ctx, target.WorkspaceID, target.ID); err != nil {
		return 0, 0, err
	}
	cfg := config.GetConfig()
	if cfg == nil {
		return 0, 0, ErrBackupBackendUnsupported
	}
	store, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return 0, 0, err
	}
	manifests, err := s.loadManifestChain(store, target)
	if err != nil {
		return 0, 0, err
	}
	objects := drillObjects(manifests)
	drillRoot := filepath.Join(cfg.Storage.StagingPath, "backup-restore-drills", drillID)
	if err := os.MkdirAll(drillRoot, 0o750); err != nil {
		return 0, 0, err
	}
	defer func() { _ = os.RemoveAll(drillRoot) }()

	var totalBytes int64
	for _, object := range objects {
		if err := ctx.Err(); err != nil {
			return 0, totalBytes, err
		}
		reader, err := store.Get(object.BackupKey)
		if err != nil {
			return 0, totalBytes, err
		}
		name := fmt.Sprintf("%020d-%s", object.VersionID, shortHash(object.SHA256))
		path := filepath.Join(drillRoot, name)
		written, copyErr := writeDrillObject(path, reader, object.Size, object.SHA256)
		_ = reader.Close()
		if copyErr != nil {
			return 0, totalBytes, copyErr
		}
		totalBytes += written
	}
	return len(objects), totalBytes, nil
}

func (s *Service) loadManifestChain(store storage.BackupStorage, target *model.BackupJob) ([]Manifest, error) {
	jobs := make([]*model.BackupJob, 0, 8)
	seen := make(map[string]struct{})
	current := target
	for current != nil {
		if _, exists := seen[current.ID]; exists {
			return nil, errors.New("backup parent chain contains a cycle")
		}
		seen[current.ID] = struct{}{}
		jobs = append(jobs, current)
		if current.ParentID == "" {
			break
		}
		parent, err := s.jobs.Get(current.WorkspaceID, current.ParentID)
		if err != nil {
			return nil, err
		}
		if parent == nil {
			return nil, errors.New("backup parent is missing")
		}
		current = parent
	}
	manifests := make([]Manifest, 0, len(jobs))
	for index := len(jobs) - 1; index >= 0; index-- {
		reader, err := store.Get(jobs[index].ManifestKey)
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			return nil, readErr
		}
		manifest, err := decodeProtectedManifest(data, config.GetConfig().Backup.ManifestEncryptionKey)
		if err != nil {
			return nil, err
		}
		manifests = append(manifests, manifest)
	}
	return manifests, nil
}

func drillObjects(manifests []Manifest) []ObjectEntry {
	objects := make([]ObjectEntry, 0)
	seen := make(map[string]struct{})
	for _, manifest := range manifests {
		for _, object := range manifest.Objects {
			key := strconv.FormatUint(uint64(object.VersionID), 10) + ":" + strings.ToLower(object.SHA256)
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			objects = append(objects, object)
		}
	}
	return objects
}

func writeDrillObject(path string, reader io.Reader, expectedSize int64, expectedSHA256 string) (int64, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return 0, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(reader, expectedSize+1))
	syncErr := file.Sync()
	closeErr := file.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(path)
		for _, value := range []error{copyErr, syncErr, closeErr} {
			if value != nil {
				return 0, value
			}
		}
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if written != expectedSize || (expectedSHA256 != "" && !strings.EqualFold(actualSHA256, expectedSHA256)) {
		_ = os.Remove(path)
		return 0, errors.New("restored drill object checksum mismatch")
	}
	return written, nil
}

func shortHash(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) >= 12 {
		return value[:12]
	}
	return value
}

func (s *Service) startJob(workspaceID, actorID uint, kind string, validateParent func(*model.BackupJob) error) (*model.BackupJob, error) {
	now := time.Now()
	job := &model.BackupJob{ID: uuid.NewString(), WorkspaceID: workspaceID, CreatedBy: actorID, Kind: kind, Trigger: "manual", Status: "running", StartedAt: &now}
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
		var parent *model.BackupJob
		if validateParent != nil {
			var latest model.BackupJob
			err := tx.Where("workspace_id = ? AND status = ?", workspaceID, "complete").Order("created_at DESC, id DESC").First(&latest).Error
			if err == nil {
				parent = &latest
			} else if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			if err := validateParent(parent); err != nil {
				return err
			}
			job.ParentID = parent.ID
			job.ChangeLogStart = parent.ChangeLogEnd
		}
		return tx.Create(job).Error
	})
	return job, err
}

func (s *Service) buildBaseline(ctx context.Context, cfg *config.Config, job *model.BackupJob) error {
	primary, err := storage.NewConfiguredVersionReader(ctx, cfg)
	if err != nil {
		return err
	}
	backupStore, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return err
	}
	var changeLogEnd uint64
	var metadata *MetadataSnapshot
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ChangeLog{}).Where("workspace_id = ?", job.WorkspaceID).Select("COALESCE(MAX(seq), 0)").Scan(&changeLogEnd).Error; err != nil {
			return err
		}
		var err error
		metadata, err = captureMetadata(tx, job.WorkspaceID)
		return err
	}); err != nil {
		return err
	}
	names := make(map[uint]string, len(metadata.Nodes))
	for _, node := range metadata.Nodes {
		names[node.ID] = node.Name
	}
	manifest := Manifest{
		BackupManifest: storage.BackupManifest{ID: job.ID, Kind: job.Kind, Status: "complete", WorkspaceID: job.WorkspaceID, ChangeLogEnd: changeLogEnd, CreatedAt: time.Now()},
		Trigger:        job.Trigger,
		Metadata:       metadata,
	}
	seen := make(map[string]struct{})
	for _, version := range metadata.Versions {
		if err := ctx.Err(); err != nil {
			return err
		}
		backupKey := filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "objects", filepath.Base(version.StorageKey)))
		if _, exists := seen[version.StorageKey]; !exists {
			file, err := primary.Open(version.StorageClass, version.StorageKey)
			if err != nil {
				return fmt.Errorf("open object %s: %w", version.StorageKey, err)
			}
			_, _, copyErr := backupStore.Put(backupKey, file)
			_ = file.Close()
			if copyErr != nil && !errors.Is(copyErr, storage.ErrBackupImmutable) && !errors.Is(copyErr, storage.ErrObjectAlreadyExists) {
				return copyErr
			}
			seen[version.StorageKey] = struct{}{}
		}
		manifest.Objects = append(manifest.Objects, ObjectEntry{
			VersionID: version.ID, NodeID: version.NodeID, Name: names[version.NodeID], StorageKey: version.StorageKey, BackupKey: backupKey,
			Size: version.Size, SHA256: version.SHA256, Extension: version.Extension, Mime: version.DetectedMime,
			RiskLevel: version.RiskLevel, ScanStatus: version.ScanStatus, Encrypted: version.Encrypted,
		})
	}
	manifest.ObjectCount = len(manifest.Objects)
	for _, object := range manifest.Objects {
		manifest.TotalBytes += object.Size
	}
	manifestData, _, err := encodeProtectedManifest(manifest, cfg.Backup.ManifestEncryptionKey)
	if err != nil {
		return err
	}
	manifestKey := filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "manifest.json.gz.enc"))
	if _, _, err := backupStore.Put(manifestKey, bytes.NewReader(manifestData)); err != nil {
		return err
	}
	completed := time.Now()
	return s.jobs.Update(job.ID, map[string]any{"status": "complete", "manifest_key": manifestKey, "object_count": manifest.ObjectCount, "total_bytes": manifest.TotalBytes, "change_log_end": changeLogEnd, "completed_at": completed})
}

func (s *Service) buildIncremental(ctx context.Context, cfg *config.Config, job *model.BackupJob) error {
	primary, err := storage.NewConfiguredVersionReader(ctx, cfg)
	if err != nil {
		return err
	}
	backupStore, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return err
	}
	var changes []model.ChangeLog
	var changeLogEnd uint64
	var metadata *MetadataSnapshot
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ChangeLog{}).Where("workspace_id = ?", job.WorkspaceID).Select("COALESCE(MAX(seq), 0)").Scan(&changeLogEnd).Error; err != nil {
			return err
		}
		if changeLogEnd > job.ChangeLogStart {
			if err := tx.Where("workspace_id = ? AND seq > ? AND seq <= ?", job.WorkspaceID, job.ChangeLogStart, changeLogEnd).Order("seq ASC").Find(&changes).Error; err != nil {
				return err
			}
		}
		var err error
		metadata, err = captureMetadata(tx, job.WorkspaceID)
		return err
	}); err != nil {
		return err
	}

	versionIDSet := make(map[uint]struct{})
	for _, change := range changes {
		if change.EntityType != "file_version" || (change.Operation != "create" && change.Operation != "restore") {
			continue
		}
		var versionID uint
		if _, err := fmt.Sscan(change.EntityID, &versionID); err == nil && versionID > 0 {
			versionIDSet[versionID] = struct{}{}
		}
	}
	versionIDs := make([]uint, 0, len(versionIDSet))
	for versionID := range versionIDSet {
		versionIDs = append(versionIDs, versionID)
	}
	versions := make([]VersionSnapshot, 0, len(versionIDs))
	for _, version := range metadata.Versions {
		if _, exists := versionIDSet[version.ID]; exists {
			versions = append(versions, version)
		}
	}
	names := make(map[uint]string, len(metadata.Nodes))
	for _, node := range metadata.Nodes {
		names[node.ID] = node.Name
	}
	manifest := Manifest{BackupManifest: storage.BackupManifest{
		ID: job.ID, Kind: job.Kind, Status: "complete", WorkspaceID: job.WorkspaceID, ParentID: job.ParentID,
		ChangeLogStart: job.ChangeLogStart, ChangeLogEnd: changeLogEnd, CreatedAt: time.Now(),
	}, Trigger: job.Trigger, Changes: changes, Metadata: metadata}
	seen := make(map[string]struct{})
	for _, version := range versions {
		if err := ctx.Err(); err != nil {
			return err
		}
		backupKey := filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "objects", filepath.Base(version.StorageKey)))
		if _, exists := seen[version.StorageKey]; !exists {
			file, err := primary.Open(version.StorageClass, version.StorageKey)
			if err != nil {
				return fmt.Errorf("open object %s: %w", version.StorageKey, err)
			}
			_, _, copyErr := backupStore.Put(backupKey, file)
			_ = file.Close()
			if copyErr != nil && !errors.Is(copyErr, storage.ErrBackupImmutable) && !errors.Is(copyErr, storage.ErrObjectAlreadyExists) {
				return copyErr
			}
			seen[version.StorageKey] = struct{}{}
		}
		manifest.Objects = append(manifest.Objects, ObjectEntry{
			VersionID: version.ID, NodeID: version.NodeID, Name: names[version.NodeID], StorageKey: version.StorageKey, BackupKey: backupKey,
			Size: version.Size, SHA256: version.SHA256, Extension: version.Extension, Mime: version.DetectedMime,
			RiskLevel: version.RiskLevel, ScanStatus: version.ScanStatus, Encrypted: version.Encrypted,
		})
	}
	manifest.ObjectCount = len(manifest.Objects)
	for _, object := range manifest.Objects {
		manifest.TotalBytes += object.Size
	}
	manifestData, _, err := encodeProtectedManifest(manifest, cfg.Backup.ManifestEncryptionKey)
	if err != nil {
		return err
	}
	manifestKey := filepath.ToSlash(filepath.Join(cfg.Backup.Prefix, job.ID, "manifest.json.gz.enc"))
	if _, _, err := backupStore.Put(manifestKey, bytes.NewReader(manifestData)); err != nil {
		return err
	}
	completed := time.Now()
	return s.jobs.Update(job.ID, map[string]any{
		"status": "complete", "manifest_key": manifestKey, "object_count": manifest.ObjectCount, "total_bytes": manifest.TotalBytes,
		"change_log_start": job.ChangeLogStart, "change_log_end": changeLogEnd, "completed_at": completed,
	})
}

func (s *Service) Verify(ctx context.Context, workspaceID uint, jobID string) (*Manifest, error) {
	job, err := s.jobs.Get(workspaceID, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	if job == nil {
		return nil, gorm.ErrRecordNotFound
	}
	if job.Status != "complete" || job.ManifestKey == "" {
		return nil, errors.New("backup job is not complete")
	}
	manifest, verifyErr := s.verifyJob(ctx, job, make(map[string]struct{}))
	if verifyErr == nil && manifest.Metadata != nil {
		cfg := config.GetConfig()
		var store storage.BackupStorage
		if cfg == nil {
			verifyErr = ErrBackupBackendUnsupported
		} else if configuredStore, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup); err != nil {
			verifyErr = mapBackupStorageError(err)
		} else {
			store = configuredStore
		}
		if verifyErr == nil {
			manifests, err := s.loadManifestChain(store, job)
			if err != nil {
				verifyErr = err
			} else {
				verifyErr = validateMetadataObjects(manifests)
			}
		}
	}
	now := time.Now()
	fields := map[string]any{"verify_status": "valid", "verified_at": now, "verify_error": ""}
	if verifyErr != nil {
		logger.Error("backup_verification_failed", "workspace_id", workspaceID, "job_id", job.ID, "error", verifyErr)
		fields["verify_status"] = "invalid"
		fields["verify_error"] = "备份链或对象校验失败"
		enqueueBackupAlert(ctx, "backup:verify_failed", "备份链完整性校验失败", job, "请检查备份清单、父链和对象完整性。")
	}
	if updateErr := s.jobs.Update(job.ID, fields); updateErr != nil && verifyErr == nil {
		return nil, updateErr
	}
	return manifest, verifyErr
}

func enqueueBackupAlert(ctx context.Context, eventType, title string, job *model.BackupJob, advice string) {
	if job == nil {
		return
	}
	_, err := notification.Publish(ctx, notification.Event{
		Key: eventType + ":" + job.ID, Type: eventType, Severity: "critical", Title: title,
		Content: fmt.Sprintf("工作空间 %d 的备份任务 %s 异常。%s", job.WorkspaceID, job.ID, advice),
		Payload: map[string]any{"workspace_id": job.WorkspaceID, "backup_job_id": job.ID, "kind": job.Kind},
	})
	if err != nil {
		logger.Error("backup_notification_enqueue_failed", "event_type", eventType, "job_id", job.ID, "error", err)
	}
}

func (s *Service) verifyJob(ctx context.Context, job *model.BackupJob, seen map[string]struct{}) (*Manifest, error) {
	if _, exists := seen[job.ID]; exists {
		return nil, errors.New("backup parent chain contains a cycle")
	}
	seen[job.ID] = struct{}{}
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, ErrBackupBackendUnsupported
	}
	store, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return nil, err
	}
	manifestReader, err := store.Get(job.ManifestKey)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(manifestReader)
	_ = manifestReader.Close()
	if readErr != nil {
		return nil, readErr
	}
	manifest, err := decodeProtectedManifest(data, cfg.Backup.ManifestEncryptionKey)
	if err != nil {
		return nil, err
	}
	if manifest.ID != job.ID || manifest.WorkspaceID != job.WorkspaceID || manifest.Status != "complete" ||
		manifest.Kind != job.Kind || manifest.ParentID != job.ParentID ||
		manifest.Trigger != job.Trigger || manifest.CompactedFromID != job.CompactedFromID ||
		manifest.ChangeLogStart != job.ChangeLogStart || manifest.ChangeLogEnd != job.ChangeLogEnd ||
		manifest.ObjectCount != job.ObjectCount || manifest.TotalBytes != job.TotalBytes {
		return nil, errors.New("backup manifest metadata mismatch")
	}
	if manifest.ObjectCount != len(manifest.Objects) {
		return nil, errors.New("backup manifest object count mismatch")
	}
	if err := validateMetadata(manifest.Metadata, job.WorkspaceID); err != nil {
		return nil, err
	}
	var totalBytes int64
	for _, object := range manifest.Objects {
		totalBytes += object.Size
	}
	if totalBytes != manifest.TotalBytes {
		return nil, errors.New("backup manifest total bytes mismatch")
	}
	if manifest.Kind == "baseline" {
		if manifest.ParentID != "" || manifest.ChangeLogStart != 0 {
			return nil, errors.New("baseline backup cannot have a parent or start cursor")
		}
	} else if manifest.Kind == "incremental" {
		if manifest.ParentID == "" || manifest.ChangeLogEnd < manifest.ChangeLogStart {
			return nil, errors.New("incremental backup cursor metadata is invalid")
		}
		parent, err := s.jobs.Get(job.WorkspaceID, manifest.ParentID)
		if err != nil {
			return nil, err
		}
		if parent == nil || parent.Status != "complete" {
			return nil, errors.New("incremental backup parent is missing or incomplete")
		}
		parentManifest, err := s.verifyJob(ctx, parent, seen)
		if err != nil {
			return nil, err
		}
		if parentManifest.ChangeLogEnd != manifest.ChangeLogStart {
			return nil, errors.New("incremental backup cursor is not continuous")
		}
		if err := validateChangeRange(job.WorkspaceID, manifest.ChangeLogStart, manifest.ChangeLogEnd, manifest.Changes); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("backup kind is invalid")
	}
	for _, object := range manifest.Objects {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		reader, err := store.Get(object.BackupKey)
		if err != nil {
			return nil, fmt.Errorf("backup object %s is missing: %w", object.BackupKey, err)
		}
		hash := sha256.New()
		size, copyErr := io.Copy(hash, reader)
		_ = reader.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if size != object.Size || (object.SHA256 != "" && hex.EncodeToString(hash.Sum(nil)) != object.SHA256) {
			return nil, fmt.Errorf("backup object %s checksum mismatch", object.BackupKey)
		}
	}
	return &manifest, nil
}

func validateMetadataObjects(manifests []Manifest) error {
	if len(manifests) == 0 || manifests[len(manifests)-1].Metadata == nil {
		return nil
	}
	objects := make(map[string]struct{})
	for _, manifest := range manifests {
		for _, object := range manifest.Objects {
			key := fmt.Sprintf("%d:%s", object.VersionID, strings.ToLower(object.SHA256))
			objects[key] = struct{}{}
		}
	}
	for _, version := range manifests[len(manifests)-1].Metadata.Versions {
		key := fmt.Sprintf("%d:%s", version.ID, strings.ToLower(version.SHA256))
		if _, exists := objects[key]; !exists {
			return fmt.Errorf("file version %d is not present in the backup chain", version.ID)
		}
	}
	return nil
}

func validateChangeRange(workspaceID uint, start, end uint64, changes []model.ChangeLog) error {
	if end < start {
		return errors.New("incremental change log range is invalid")
	}
	previousSeq := start
	for _, change := range changes {
		if change.WorkspaceID != workspaceID || change.Seq <= start || change.Seq > end || change.Seq <= previousSeq {
			return errors.New("incremental change log sequence is invalid")
		}
		previousSeq = change.Seq
	}
	if len(changes) == 0 && start != end {
		return errors.New("incremental change log range is incomplete")
	}
	if len(changes) > 0 && previousSeq != end {
		return errors.New("incremental change log end cursor is missing")
	}
	return nil
}

func (s *Service) RestoreFile(ctx context.Context, workspaceID, actorID uint, jobID string, versionID uint, parentID *uint) (*model.Node, error) {
	manifest, err := s.Verify(ctx, workspaceID, jobID)
	if err != nil {
		return nil, err
	}
	var object *ObjectEntry
	for index := range manifest.Objects {
		if manifest.Objects[index].VersionID == versionID {
			object = &manifest.Objects[index]
			break
		}
	}
	if object == nil {
		return nil, ErrRestoreObjectNotFound
	}
	name := strings.TrimSpace(object.Name)
	if name == "" {
		name = fmt.Sprintf("restored-%d%s", object.VersionID, object.Extension)
	}
	cfg := config.GetConfig()
	primary, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	backupStore, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return nil, err
	}
	reader, err := backupStore.Get(object.BackupKey)
	if err != nil {
		return nil, err
	}
	imported, importErr := primary.ImportObject(workspaceID, reader, object.Size, object.SHA256)
	_ = reader.Close()
	if importErr != nil {
		return nil, importErr
	}
	node := &model.Node{WorkspaceID: workspaceID, ParentID: parentID, Name: name, NormalizedName: strings.ToLower(name), Type: "file", InheritMode: "inherit", Status: "active", CreatedBy: actorID, UpdatedBy: actorID}
	version := &model.FileVersion{WorkspaceID: workspaceID, VersionNo: 1, StorageKey: imported.StorageKey, StorageClass: "standard", Size: imported.Size, SHA256: imported.SHA256, Extension: object.Extension, DetectedMime: object.Mime, RiskLevel: object.RiskLevel, ScanStatus: object.ScanStatus, Encrypted: object.Encrypted, CreatedBy: actorID}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		if workspace.QuotaBytes != nil && workspace.UsedBytes+workspace.ReservedBytes+object.Size > *workspace.QuotaBytes {
			return dao.ErrQuotaExceeded
		}
		var count int64
		query := tx.Model(&model.Node{}).Where("workspace_id = ? AND status = ? AND normalized_name = ?", workspaceID, "active", strings.ToLower(name))
		if parentID == nil {
			query = query.Where("parent_id IS NULL")
		} else {
			query = query.Where("parent_id = ?", *parentID)
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return ErrRestoreNameConflict
		}
		var membership model.WorkspaceMembership
		membershipErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id = ? AND user_id = ?", workspaceID, actorID).First(&membership).Error
		if membershipErr != nil && !errors.Is(membershipErr, gorm.ErrRecordNotFound) {
			return membershipErr
		}
		if membershipErr == nil && membership.QuotaBytes != nil && membership.UsedBytes+membership.ReservedBytes+object.Size > *membership.QuotaBytes {
			return dao.ErrQuotaExceeded
		}
		if parentID != nil {
			var parent model.Node
			if err := tx.Where("workspace_id = ? AND id = ? AND type = ? AND status = ?", workspaceID, *parentID, "folder", "active").First(&parent).Error; err != nil {
				return err
			}
		}
		if err := tx.Create(node).Error; err != nil {
			return err
		}
		if err := tx.Create(&model.NodeClosure{AncestorID: node.ID, DescendantID: node.ID, Depth: 0}).Error; err != nil {
			return err
		}
		if parentID != nil {
			if err := tx.Exec(`INSERT INTO node_closures (ancestor_id, descendant_id, depth) SELECT ancestor_id, ?, depth + 1 FROM node_closures WHERE descendant_id = ?`, node.ID, *parentID).Error; err != nil {
				return err
			}
		}
		version.NodeID = node.ID
		if err := tx.Create(version).Error; err != nil {
			return err
		}
		if err := tx.Model(node).Update("active_version", version.ID).Error; err != nil {
			return err
		}
		if err := tx.Model(&workspace).UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", object.Size)).Error; err != nil {
			return err
		}
		if membershipErr == nil {
			if err := tx.Model(&membership).UpdateColumn("used_bytes", gorm.Expr("used_bytes + ?", object.Size)).Error; err != nil {
				return err
			}
		}
		if err := dao.AppendChange(tx, workspaceID, "node", node.ID, "restore_from_backup", map[string]any{
			"backup_id": jobID, "source_version_id": object.VersionID, "parent_id": parentID,
		}); err != nil {
			return err
		}
		return dao.AppendChange(tx, workspaceID, "file_version", version.ID, "create", map[string]any{
			"node_id": node.ID, "version_no": version.VersionNo, "size": version.Size,
			"sha256": version.SHA256, "storage_key": version.StorageKey,
		})
	})
	if err != nil {
		_ = primary.RemoveObject(imported.StorageKey)
		return nil, err
	}
	return node, nil
}

func encodeFullManifest(manifest Manifest) ([]byte, string, error) {
	manifest.ManifestHash = ""
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	hash, err := fullManifestHash(data)
	if err != nil {
		return nil, "", err
	}
	manifest.ManifestHash = hash
	data, err = json.Marshal(manifest)
	return data, hash, err
}

// fullManifestHash hashes the fields that were actually persisted. Computing
// from raw JSON keeps old manifests verifiable when a later release adds a
// non-omitempty field to one of the typed snapshot structures.
func fullManifestHash(data []byte) (string, error) {
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		return "", err
	}
	document["manifest_hash"] = ""
	canonical, err := json.Marshal(document)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(canonical)
	return hex.EncodeToString(digest[:]), nil
}

func decodeFullManifest(data []byte) (Manifest, error) {
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	provided := manifest.ManifestHash
	expected, err := fullManifestHash(data)
	if err != nil {
		return manifest, err
	}
	if provided == "" || provided != expected {
		return manifest, errors.New("manifest hash mismatch")
	}
	if manifest.Metadata != nil {
		var document struct {
			Metadata json.RawMessage `json:"metadata"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			return manifest, err
		}
		if len(document.Metadata) == 0 || bytes.Equal(bytes.TrimSpace(document.Metadata), []byte("null")) {
			return manifest, errors.New("backup metadata is missing")
		}
		if err := verifyDecodedMetadataHash(document.Metadata, manifest.Metadata); err != nil {
			return manifest, err
		}
	}
	return manifest, nil
}
