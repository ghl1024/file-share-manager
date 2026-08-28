/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package reconcile

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/storage"

	"gorm.io/gorm"
)

type Report struct {
	WorkspaceID        uint                      `json:"workspace_id"`
	Scanned            int                       `json:"scanned"`
	Referenced         int                       `json:"referenced"`
	OrphanObjects      []string                  `json:"orphan_objects"`
	MissingObjects     []string                  `json:"missing_objects"`
	QuarantinedObjects []QuarantinedObject       `json:"quarantined_objects,omitempty"`
	SkippedObjects     []string                  `json:"skipped_objects,omitempty"`
	FailedObjects      []ObjectError             `json:"failed_objects,omitempty"`
	QuarantineRecords  []model.StorageQuarantine `json:"quarantine_records"`
}

type QuarantinedObject struct {
	StorageKey    string `json:"storage_key"`
	QuarantineKey string `json:"quarantine_key"`
}

type ObjectError struct {
	StorageKey string `json:"storage_key"`
	Error      string `json:"error"`
}

type PurgeReport struct {
	Processed int           `json:"processed"`
	Purged    []string      `json:"purged"`
	Restored  []string      `json:"restored"`
	Failed    []ObjectError `json:"failed"`
}

func ScanWorkspace(ctx context.Context, workspaceID uint) (*Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	store, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	actual, err := store.ListWorkspaceObjects(workspaceID)
	if err != nil {
		return nil, err
	}
	referenced := map[string]struct{}{}
	var versions []model.FileVersion
	if err := database.DB.Where("workspace_id = ?", workspaceID).Find(&versions).Error; err != nil {
		return nil, err
	}
	for _, version := range versions {
		if version.StorageClass == "" || version.StorageClass == "standard" {
			referenced[version.StorageKey] = struct{}{}
		}
	}
	var shares []model.ShareItem
	if err := database.DB.Joins("JOIN shares ON shares.id = share_items.share_id").Where("shares.workspace_id = ?", workspaceID).Find(&shares).Error; err != nil {
		return nil, err
	}
	for _, item := range shares {
		if item.StorageClass == "" || item.StorageClass == "standard" {
			referenced[item.StorageKey] = struct{}{}
		}
	}
	var batches []model.BatchDownloadItem
	if err := database.DB.Joins("JOIN batch_download_jobs ON batch_download_jobs.id = batch_download_items.job_id").Where("batch_download_jobs.workspace_id = ?", workspaceID).Find(&batches).Error; err != nil {
		return nil, err
	}
	for _, item := range batches {
		if item.StorageClass == "" || item.StorageClass == "standard" {
			referenced[item.StorageKey] = struct{}{}
		}
	}
	report := &Report{WorkspaceID: workspaceID, Scanned: len(actual), Referenced: len(referenced), OrphanObjects: []string{}, MissingObjects: []string{}}
	actualSet := make(map[string]struct{}, len(actual))
	for _, key := range actual {
		actualSet[key] = struct{}{}
		if _, ok := referenced[key]; !ok {
			report.OrphanObjects = append(report.OrphanObjects, key)
		}
	}
	for key := range referenced {
		if _, ok := actualSet[key]; !ok {
			report.MissingObjects = append(report.MissingObjects, key)
		}
	}
	sort.Strings(report.OrphanObjects)
	sort.Strings(report.MissingObjects)
	if err := database.DB.Where("workspace_id = ?", workspaceID).
		Order("quarantined_at DESC, id DESC").Limit(100).Find(&report.QuarantineRecords).Error; err != nil {
		return nil, err
	}
	return report, nil
}

func QuarantineWorkspaceOrphans(ctx context.Context, workspaceID, actorID uint, requestedKeys []string) (*Report, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	report, err := ScanWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if len(requestedKeys) == 0 || len(report.OrphanObjects) == 0 {
		return report, nil
	}
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	store, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}

	orphanSet := make(map[string]struct{}, len(report.OrphanObjects))
	for _, key := range report.OrphanObjects {
		orphanSet[key] = struct{}{}
	}
	for _, key := range uniqueKeys(requestedKeys) {
		if _, ok := orphanSet[key]; !ok {
			report.SkippedObjects = append(report.SkippedObjects, key)
			continue
		}
		quarantineKey, err := store.QuarantineObject(workspaceID, key)
		if err != nil {
			report.FailedObjects = append(report.FailedObjects, ObjectError{StorageKey: key, Error: err.Error()})
			continue
		}
		quarantinedAt := time.Now().UTC()
		record := &model.StorageQuarantine{
			WorkspaceID: workspaceID, StorageKey: key, QuarantineKey: quarantineKey,
			Status: model.StorageQuarantineStatusQuarantined, QuarantinedAt: quarantinedAt,
			PurgeAfter: quarantinedAt.AddDate(0, 0, cfg.Lifecycle.QuarantineRetentionDays), CreatedBy: actorID,
		}
		if err := persistQuarantine(record); err != nil {
			restoreErr := store.RestoreQuarantinedObject(workspaceID, quarantineKey, key)
			message := fmt.Sprintf("persist quarantine record: %v", err)
			if restoreErr != nil {
				message += fmt.Sprintf("; restore object: %v", restoreErr)
			}
			report.FailedObjects = append(report.FailedObjects, ObjectError{StorageKey: key, Error: message})
			continue
		}
		report.QuarantinedObjects = append(report.QuarantinedObjects, QuarantinedObject{StorageKey: key, QuarantineKey: quarantineKey})
	}
	sort.Slice(report.QuarantinedObjects, func(i, j int) bool {
		return report.QuarantinedObjects[i].StorageKey < report.QuarantinedObjects[j].StorageKey
	})
	sort.Strings(report.SkippedObjects)
	sort.Slice(report.FailedObjects, func(i, j int) bool {
		return report.FailedObjects[i].StorageKey < report.FailedObjects[j].StorageKey
	})
	latest, err := ScanWorkspace(ctx, workspaceID)
	if err != nil {
		return report, nil
	}
	latest.QuarantinedObjects = report.QuarantinedObjects
	latest.SkippedObjects = report.SkippedObjects
	latest.FailedObjects = report.FailedObjects
	return latest, nil
}

// PurgeDueQuarantines deletes only objects that remain unreferenced at the end
// of their quarantine window. A newly referenced object is restored instead.
func PurgeDueQuarantines(ctx context.Context, now time.Time, limit int) (*PurgeReport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit < 1 {
		return nil, errors.New("reconcile purge limit must be positive")
	}
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	store, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	var records []model.StorageQuarantine
	if err := database.DB.Where("status = ? AND purge_after <= ?", model.StorageQuarantineStatusQuarantined, now).
		Order("purge_after ASC, id ASC").Limit(limit).Find(&records).Error; err != nil {
		return nil, err
	}
	report := &PurgeReport{Purged: []string{}, Restored: []string{}, Failed: []ObjectError{}}
	for index := range records {
		if err := ctx.Err(); err != nil {
			return report, err
		}
		record := &records[index]
		report.Processed++
		referenced, err := storageKeyIsReferenced(database.DB, record.WorkspaceID, record.StorageKey)
		if err != nil {
			recordQuarantineFailure(record.ID, err)
			report.Failed = append(report.Failed, ObjectError{StorageKey: record.StorageKey, Error: err.Error()})
			continue
		}
		status := model.StorageQuarantineStatusPurged
		operation := "purge"
		if referenced {
			status = model.StorageQuarantineStatusRestored
			operation = "restore"
			err = store.RestoreQuarantinedObject(record.WorkspaceID, record.QuarantineKey, record.StorageKey)
		} else {
			err = store.RemoveQuarantinedObject(record.WorkspaceID, record.QuarantineKey)
		}
		if err != nil {
			recordQuarantineFailure(record.ID, err)
			report.Failed = append(report.Failed, ObjectError{StorageKey: record.StorageKey, Error: err.Error()})
			continue
		}
		if err := completeQuarantine(record, status, operation, now); err != nil {
			recordQuarantineFailure(record.ID, err)
			report.Failed = append(report.Failed, ObjectError{StorageKey: record.StorageKey, Error: err.Error()})
			continue
		}
		if referenced {
			report.Restored = append(report.Restored, record.StorageKey)
		} else {
			report.Purged = append(report.Purged, record.StorageKey)
		}
	}
	return report, nil
}

func uniqueKeys(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	keys := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		keys = append(keys, value)
	}
	sort.Strings(keys)
	return keys
}

func persistQuarantine(record *model.StorageQuarantine) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(record).Error; err != nil {
			return err
		}
		return dao.AppendChange(tx, record.WorkspaceID, "storage_object", record.StorageKey, "quarantine", map[string]any{
			"storage_key":    record.StorageKey,
			"quarantine_key": record.QuarantineKey,
			"purge_after":    record.PurgeAfter,
		})
	})
}

func storageKeyIsReferenced(db *gorm.DB, workspaceID uint, storageKey string) (bool, error) {
	checks := []*gorm.DB{
		db.Model(&model.FileVersion{}).
			Where("workspace_id = ? AND storage_key = ? AND (storage_class = '' OR storage_class = 'standard')", workspaceID, storageKey),
		db.Model(&model.ShareItem{}).
			Joins("JOIN shares ON shares.id = share_items.share_id").
			Where("shares.workspace_id = ? AND share_items.storage_key = ? AND (share_items.storage_class = '' OR share_items.storage_class = 'standard')", workspaceID, storageKey),
		db.Model(&model.BatchDownloadItem{}).
			Joins("JOIN batch_download_jobs ON batch_download_jobs.id = batch_download_items.job_id").
			Where("batch_download_jobs.workspace_id = ? AND batch_download_items.storage_key = ? AND (batch_download_items.storage_class = '' OR batch_download_items.storage_class = 'standard')", workspaceID, storageKey),
	}
	for _, query := range checks {
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return false, err
		}
		if count > 0 {
			return true, nil
		}
	}
	return false, nil
}

func completeQuarantine(record *model.StorageQuarantine, status, operation string, now time.Time) error {
	return database.DB.Transaction(func(tx *gorm.DB) error {
		updates := map[string]any{"status": status, "last_error": "", "updated_at": now}
		if status == model.StorageQuarantineStatusPurged {
			updates["purged_at"] = now
		} else {
			updates["restored_at"] = now
		}
		result := tx.Model(&model.StorageQuarantine{}).
			Where("id = ? AND status = ?", record.ID, model.StorageQuarantineStatusQuarantined).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return errors.New("quarantine record state changed concurrently")
		}
		return dao.AppendChange(tx, record.WorkspaceID, "storage_object", record.StorageKey, operation, map[string]any{
			"storage_key": record.StorageKey, "quarantine_key": record.QuarantineKey,
		})
	})
}

func recordQuarantineFailure(id uint, cause error) {
	message := strings.TrimSpace(cause.Error())
	if len(message) > 1000 {
		message = message[:1000]
	}
	_ = database.DB.Model(&model.StorageQuarantine{}).Where("id = ?", id).Updates(map[string]any{
		"retry_count": gorm.Expr("retry_count + 1"), "last_error": message,
	}).Error
}
