/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package migration

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"file-share-manager/server/internal/model"

	"gorm.io/gorm"
)

const (
	CurrentVersion    = "202608160001"
	migrationLockName = "file_share_manager_schema_migration"
	migrationManifest = "core-schema-v2:file-versions-scan-retry:storage-quarantine:audit-streams:audit-archives:ldap-sync-history:notification-channels:notification-outbox:user-notifications:user-notification-preferences:nodes-ngram-search:backup-compaction:recent-node-access:node-comments:node-comment-mentions"
)

type SchemaMigration struct {
	Version    string    `gorm:"type:varchar(64);primaryKey" json:"version"`
	Name       string    `gorm:"type:varchar(255);not null" json:"name"`
	Checksum   string    `gorm:"type:char(64);not null" json:"checksum"`
	AppliedAt  time.Time `gorm:"not null" json:"applied_at"`
	DurationMS int64     `gorm:"not null" json:"duration_ms"`
}

func (SchemaMigration) TableName() string { return "schema_migrations" }

type VersionedReport struct {
	CurrentVersion string   `json:"current_version"`
	Applied        []string `json:"applied"`
	Skipped        []string `json:"skipped"`
}

type migrationStep struct {
	version  string
	name     string
	checksum string
	apply    func(*gorm.DB) error
}

func versionedSteps() []migrationStep {
	return []migrationStep{{
		version:  CurrentVersion,
		name:     "current application schema",
		checksum: migrationChecksum(CurrentVersion, migrationManifest),
		apply:    Run,
	}}
}

func RunVersioned(db *gorm.DB) (VersionedReport, error) {
	report := VersionedReport{CurrentVersion: CurrentVersion}
	if db == nil {
		return report, errors.New("database is not initialized")
	}
	acquired, err := acquireMigrationLock(db, 30)
	if err != nil {
		return report, err
	}
	if !acquired {
		return report, errors.New("another schema migration is still running")
	}
	defer func() { _ = db.Exec("SELECT RELEASE_LOCK(?)", migrationLockName).Error }()

	if err := db.AutoMigrate(&SchemaMigration{}); err != nil {
		return report, fmt.Errorf("create schema migration ledger: %w", err)
	}
	for _, step := range versionedSteps() {
		var receipt SchemaMigration
		err := db.Where("version = ?", step.version).First(&receipt).Error
		switch {
		case err == nil:
			if receipt.Checksum != step.checksum || receipt.Name != step.name {
				return report, fmt.Errorf("migration %s receipt checksum or name does not match this release", step.version)
			}
			report.Skipped = append(report.Skipped, step.version)
			continue
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return report, fmt.Errorf("read migration %s receipt: %w", step.version, err)
		}

		started := time.Now()
		if err := step.apply(db); err != nil {
			return report, fmt.Errorf("apply migration %s: %w", step.version, err)
		}
		if err := VerifySchema(db); err != nil {
			return report, fmt.Errorf("verify migration %s: %w", step.version, err)
		}
		receipt = SchemaMigration{
			Version: step.version, Name: step.name, Checksum: step.checksum,
			AppliedAt: time.Now(), DurationMS: time.Since(started).Milliseconds(),
		}
		if err := db.Create(&receipt).Error; err != nil {
			return report, fmt.Errorf("record migration %s receipt: %w", step.version, err)
		}
		report.Applied = append(report.Applied, step.version)
	}
	return report, VerifyCurrent(db)
}

func VerifyCurrent(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not initialized")
	}
	if !db.Migrator().HasTable(&SchemaMigration{}) {
		return fmt.Errorf("schema migration ledger is missing; run the migration command for version %s", CurrentVersion)
	}
	step := versionedSteps()[len(versionedSteps())-1]
	var receipt SchemaMigration
	if err := db.Where("version = ?", step.version).First(&receipt).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("schema migration %s is pending", step.version)
		}
		return err
	}
	if receipt.Name != step.name || receipt.Checksum != step.checksum {
		return fmt.Errorf("schema migration %s receipt checksum or name is invalid", step.version)
	}
	return VerifySchema(db)
}

func VerifySchema(db *gorm.DB) error {
	migrator := db.Migrator()
	for _, schemaModel := range schemaModels() {
		if !migrator.HasTable(schemaModel) {
			return fmt.Errorf("required table for %T is missing", schemaModel)
		}
	}
	checks := []struct {
		model  any
		column string
	}{
		{&model.FileVersion{}, "scan_retry_count"},
		{&model.FileVersion{}, "scan_next_retry_at"},
		{&model.FileVersion{}, "scan_last_attempt_at"},
		{&model.StorageQuarantine{}, "purge_after"},
		{&model.OperationLog{}, "stream_seq"},
		{&model.NotificationChannel{}, "config_ciphertext"},
		{&model.NotificationOutbox{}, "next_attempt_at"},
		{&model.BackupJob{}, "trigger"},
		{&model.BackupJob{}, "compacted_from_id"},
	}
	for _, check := range checks {
		if !migrator.HasColumn(check.model, check.column) {
			return fmt.Errorf("required column %s for %T is missing", check.column, check.model)
		}
	}
	if !migrator.HasIndex(&model.FileVersion{}, "idx_scan_retry_queue") {
		return errors.New("required index idx_scan_retry_queue is missing")
	}
	if !migrator.HasIndex(&model.NotificationOutbox{}, "idx_notification_due") {
		return errors.New("required index idx_notification_due is missing")
	}
	if !migrator.HasIndex(&model.UserNotification{}, "idx_user_notification_feed") {
		return errors.New("required index idx_user_notification_feed is missing")
	}
	if !migrator.HasIndex(&model.UserNotification{}, "idx_user_notification_unread") {
		return errors.New("required index idx_user_notification_unread is missing")
	}
	if !migrator.HasIndex(&model.Node{}, "idx_nodes_fulltext_name") {
		return errors.New("required index idx_nodes_fulltext_name is missing")
	}
	if !migrator.HasIndex(&model.Node{}, "idx_nodes_search_prefix") {
		return errors.New("required index idx_nodes_search_prefix is missing")
	}
	if !migrator.HasIndex(&model.RecentNodeAccess{}, "idx_recent_node_actor") {
		return errors.New("required index idx_recent_node_actor is missing")
	}
	if !migrator.HasIndex(&model.RecentNodeAccess{}, "idx_recent_node_user_time") {
		return errors.New("required index idx_recent_node_user_time is missing")
	}
	if !migrator.HasIndex(&model.NodeComment{}, "idx_node_comment_feed") {
		return errors.New("required index idx_node_comment_feed is missing")
	}
	return nil
}

func acquireMigrationLock(db *gorm.DB, timeoutSeconds int) (bool, error) {
	var acquired int
	if err := db.Raw("SELECT GET_LOCK(?, ?)", migrationLockName, timeoutSeconds).Scan(&acquired).Error; err != nil {
		return false, fmt.Errorf("acquire schema migration lock: %w", err)
	}
	return acquired == 1, nil
}

func migrationChecksum(version, manifest string) string {
	digest := sha256.Sum256([]byte(version + "\n" + manifest))
	return hex.EncodeToString(digest[:])
}
