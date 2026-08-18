/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigAppliesEnvironmentOverrides(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILESHARE_SERVER_PORT", "29100")
	t.Setenv("FILESHARE_DB_PASSWORD", "test-password")
	t.Setenv("FILESHARE_DB_LOG_QUERIES", "true")
	t.Setenv("FILESHARE_STORAGE_MIN_FREE_BYTES", "1073741824")
	t.Setenv("FILESHARE_STORAGE_WARN_FREE_PERCENT", "25")
	t.Setenv("FILESHARE_STORAGE_MIN_FREE_PERCENT", "15")
	t.Setenv("FILESHARE_STORAGE_HEALTH_INTERVAL_MINUTES", "2")
	t.Setenv("FILESHARE_BATCH_DOWNLOAD_MAX_FILES", "250")
	t.Setenv("FILESHARE_PREVIEW_MAX_BINARY_BYTES", "8388608")
	t.Setenv("FILESHARE_PREVIEW_MAX_TEXT_BYTES", "524288")
	t.Setenv("FILESHARE_BACKUP_MANIFEST_KEY", "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio=")
	t.Setenv("FILESHARE_BACKUP_COMPACTION_ENABLED", "true")
	t.Setenv("FILESHARE_BACKUP_COMPACTION_INTERVAL_MINUTES", "15")
	t.Setenv("FILESHARE_BACKUP_COMPACTION_INCREMENTAL_THRESHOLD", "12")
	t.Setenv("FILESHARE_ARCHIVE_ENABLED", "true")
	t.Setenv("FILESHARE_ARCHIVE_AFTER_DAYS", "730")
	t.Setenv("FILESHARE_AUDIT_HOT_RETENTION_DAYS", "730")
	t.Setenv("FILESHARE_AUDIT_EXPORT_RETENTION_HOURS", "48")
	t.Setenv("FILESHARE_AUDIT_ARCHIVE_ENABLED", "true")
	t.Setenv("FILESHARE_AUDIT_ARCHIVE_INTERVAL_MINUTES", "30")
	t.Setenv("FILESHARE_AUDIT_ARCHIVE_BATCH_SIZE", "500")
	t.Setenv("FILESHARE_AUDIT_ARCHIVE_PREFIX", "audit-evidence")
	t.Setenv("FILESHARE_AUDIT_DB_USER", "fileshare_archive")
	t.Setenv("FILESHARE_AUDIT_DB_PASSWORD", "archive-password")
	t.Setenv("FILESHARE_CLAMAV_RETRY_MAX_ATTEMPTS", "5")
	t.Setenv("FILESHARE_CLAMAV_RETRY_INTERVAL_MINUTES", "10")
	t.Setenv("FILESHARE_CLAMAV_RETRY_BATCH_SIZE", "75")
	t.Setenv("FILESHARE_QUARANTINE_RETENTION_DAYS", "14")
	t.Setenv("FILESHARE_RECONCILE_BATCH_SIZE", "250")

	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	if got := GetConfig().Server.Port; got != 29100 {
		t.Fatalf("Server.Port = %d, want 29100", got)
	}
	if got := GetConfig().Database.Password; got != "test-password" {
		t.Fatalf("Database.Password = %q", got)
	}
	if !GetConfig().Database.LogQueries {
		t.Fatalf("Database.LogQueries = false, want true")
	}
	if GetConfig().Storage.MinFreeBytes != 1073741824 || GetConfig().Storage.WarnFreePercent != 25 || GetConfig().Storage.MinFreePercent != 15 || GetConfig().Storage.HealthCheckIntervalMinutes != 2 {
		t.Fatalf("Storage health config = %#v", GetConfig().Storage)
	}
	if len(GetConfig().JWT.Secret) < 32 {
		t.Fatalf("development JWT secret was not generated")
	}
	if got := GetConfig().BatchDownload.MaxFiles; got != 250 {
		t.Fatalf("BatchDownload.MaxFiles = %d, want 250", got)
	}
	if GetConfig().Preview.MaxBinaryBytes != 8388608 || GetConfig().Preview.MaxTextBytes != 524288 {
		t.Fatalf("Preview config = %#v", GetConfig().Preview)
	}
	if got := GetConfig().Backup.ManifestEncryptionKey; got != "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio=" {
		t.Fatalf("Backup.ManifestEncryptionKey was not overridden")
	}
	if !GetConfig().Backup.CompactionEnabled || GetConfig().Backup.CompactionIntervalMin != 15 || GetConfig().Backup.CompactionThreshold != 12 {
		t.Fatalf("Backup compaction config = %#v", GetConfig().Backup)
	}
	if !GetConfig().Archive.Enabled || GetConfig().Archive.AfterDays != 730 {
		t.Fatalf("Archive config = %#v", GetConfig().Archive)
	}
	if GetConfig().Audit.HotRetentionDays != 730 || GetConfig().Audit.ExportRetentionHours != 48 {
		t.Fatalf("Audit config = %#v", GetConfig().Audit)
	}
	if !GetConfig().Audit.ArchiveEnabled || GetConfig().Audit.ArchiveIntervalMinutes != 30 || GetConfig().Audit.ArchiveBatchSize != 500 || GetConfig().Audit.ArchivePrefix != "audit-evidence/" {
		t.Fatalf("Audit archive config = %#v", GetConfig().Audit)
	}
	if GetConfig().AuditDatabase.User != "fileshare_archive" || GetConfig().AuditDatabase.Password != "archive-password" || GetConfig().AuditDatabase.Host != "localhost" || GetConfig().AuditDatabase.DBName != "fileshare" {
		t.Fatalf("Audit database config = %#v", GetConfig().AuditDatabase)
	}
	if GetConfig().ClamAV.RetryMaxAttempts != 5 || GetConfig().ClamAV.RetryIntervalMinutes != 10 || GetConfig().ClamAV.RetryBatchSize != 75 {
		t.Fatalf("ClamAV retry config = %#v", GetConfig().ClamAV)
	}
	if GetConfig().Lifecycle.QuarantineRetentionDays != 14 || GetConfig().Lifecycle.ReconcileBatchSize != 250 {
		t.Fatalf("Lifecycle reconciliation config = %#v", GetConfig().Lifecycle)
	}
}

func TestLoadConfigRejectsAuditArchiveWithoutDedicatedDatabaseUser(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"
auto_migrate = false

[backup]
manifest_encryption_key = "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio="

[audit]
archive_enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "audit_database") {
		t.Fatalf("LoadConfig() error = %v, want audit_database error", err)
	}
}

func TestLoadConfigRejectsAuditArchiveWithAutoMigration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"
auto_migrate = true

[audit_database]
user = "fileshare_archive"

[backup]
manifest_encryption_key = "KioqKioqKioqKioqKioqKioqKioqKioqKioqKioqKio="

[audit]
archive_enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "auto_migrate=false") {
		t.Fatalf("LoadConfig() error = %v, want auto_migrate error", err)
	}
}

func TestLoadConfigRejectsArchiveMigrationForCloudMount(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"

[storage]
mode = "cloud_mount"

[archive]
enabled = true
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "local primary storage") {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}

func TestLoadConfigRejectsInvalidBackupManifestKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"

[backup]
manifest_encryption_key = "not-a-32-byte-base64-key"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "manifest_encryption_key") {
		t.Fatalf("LoadConfig() error = %v, want manifest encryption key error", err)
	}
}

func TestLoadConfigRejectsInvalidBackupPrefix(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"

[backup]
prefix = "../escape/"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "backup.prefix") {
		t.Fatalf("LoadConfig() error = %v, want backup prefix error", err)
	}
}

func TestLoadConfigRejectsInvalidBackupCompactionPolicy(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"

[backup]
compaction_interval_minutes = 4
compaction_incremental_threshold = 1
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "compaction_interval_minutes") {
		t.Fatalf("LoadConfig() error = %v, want backup compaction interval error", err)
	}
}

func TestLoadConfigRejectsIncompleteObjectStorage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"

[backup]
type = "minio"
endpoint = "http://minio:9000"
bucket = "fileshare"
region = "us-east-1"
access_key = "minio"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := LoadConfig(path); err == nil || !strings.Contains(err.Error(), "secret_key") {
		t.Fatalf("LoadConfig() error = %v, want object storage credential error", err)
	}
}

func TestLoadConfigRejectsReleaseDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "release"
web_url = "https://fileshare.example.com"

[database]
host = "db"
port = 3306
user = "fileshare"
password = "database-password"
dbname = "fileshare"
auto_migrate = false

[storage]
root_path = "/data/fileshare"
staging_path = "/data/fileshare/staging"

[jwt]
secret = "super-secret-key-change-in-prod"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	err := LoadConfig(path)
	if err == nil || !strings.Contains(err.Error(), "JWT secret") {
		t.Fatalf("LoadConfig() error = %v, want JWT secret validation error", err)
	}
}

func TestLoadConfigNormalizesUploadExtensions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `[server]
port = 29000
mode = "debug"
web_url = "http://localhost:39000"

[database]
host = "localhost"
port = 3306
user = "fileshare"
dbname = "fileshare"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FILESHARE_ALLOWED_EXTENSIONS", "PDF, .Md, pdf")
	if err := LoadConfig(path); err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}
	got := GetConfig().Upload.AllowedExtensions
	if len(got) != 2 || got[0] != ".pdf" || got[1] != ".md" {
		t.Fatalf("AllowedExtensions = %#v, want [.pdf .md]", got)
	}
}
