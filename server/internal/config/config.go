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
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server        ServerConfig        `toml:"server"`
	Database      DatabaseConfig      `toml:"database"`
	AuditDatabase AuditDatabaseConfig `toml:"audit_database"`
	Storage       StorageConfig       `toml:"storage"`
	Upload        UploadConfig        `toml:"upload"`
	Preview       PreviewConfig       `toml:"preview"`
	BatchDownload BatchDownloadConfig `toml:"batch_download"`
	JWT           JWTConfig           `toml:"jwt"`
	Backup        BackupConfig        `toml:"backup"`
	Archive       ArchiveConfig       `toml:"archive"`
	Audit         AuditConfig         `toml:"audit"`
	ClamAV        ClamAVConfig        `toml:"clamav"`
	Notification  NotificationConfig  `toml:"notification"`
	Log           LogConfig           `toml:"log"`
	Lifecycle     LifecycleConfig     `toml:"lifecycle"`
}

type ServerConfig struct {
	Host                     string `toml:"host"`
	Port                     int    `toml:"port"`
	Mode                     string `toml:"mode"`
	WebURL                   string `toml:"web_url"`
	ReadHeaderTimeoutSeconds int    `toml:"read_header_timeout_seconds"`
	ReadTimeoutSeconds       int    `toml:"read_timeout_seconds"`
	WriteTimeoutSeconds      int    `toml:"write_timeout_seconds"`
	IdleTimeoutSeconds       int    `toml:"idle_timeout_seconds"`
	ShutdownTimeoutSeconds   int    `toml:"shutdown_timeout_seconds"`
	MaxRequestBodyBytes      int64  `toml:"max_request_body_bytes"`
	MaxUploadBodyBytes       int64  `toml:"max_upload_body_bytes"`
}

type DatabaseConfig struct {
	Host         string `toml:"host"`
	Port         int    `toml:"port"`
	User         string `toml:"user"`
	Password     string `toml:"password"`
	DBName       string `toml:"dbname"`
	MaxIdleConns int    `toml:"max_idle_conns"`
	MaxOpenConns int    `toml:"max_open_conns"`
	AutoMigrate  bool   `toml:"auto_migrate"`
	LogQueries   bool   `toml:"log_queries"`
}

// AuditDatabaseConfig is the restricted connection used only by the audit
// archive worker. Schema migrations must continue to use DatabaseConfig.
type AuditDatabaseConfig struct {
	Host         string `toml:"host"`
	Port         int    `toml:"port"`
	User         string `toml:"user"`
	Password     string `toml:"password"`
	DBName       string `toml:"dbname"`
	MaxIdleConns int    `toml:"max_idle_conns"`
	MaxOpenConns int    `toml:"max_open_conns"`
}

type StorageConfig struct {
	RootPath                   string `toml:"root_path"`
	StagingPath                string `toml:"staging_path"`
	Mode                       string `toml:"mode"`
	MinFreeBytes               int64  `toml:"min_free_bytes"`
	WarnFreePercent            int    `toml:"warn_free_percent"`
	MinFreePercent             int    `toml:"min_free_percent"`
	HealthCheckIntervalMinutes int    `toml:"health_check_interval_minutes"`
}

type UploadConfig struct {
	AllowedExtensions []string `toml:"allowed_extensions"`
	AllowMIMEMismatch bool     `toml:"allow_mime_mismatch"`
}

// PreviewConfig limits browser-rendered content independently from downloads.
// Only explicitly supported formats are eligible for preview.
type PreviewConfig struct {
	MaxBinaryBytes int64 `toml:"max_binary_bytes"`
	MaxTextBytes   int64 `toml:"max_text_bytes"`
}

type BatchDownloadConfig struct {
	MaxFiles       int   `toml:"max_files"`
	MaxTotalBytes  int64 `toml:"max_total_bytes"`
	RetentionHours int   `toml:"retention_hours"`
	WorkerCount    int   `toml:"worker_count"`
}

type JWTConfig struct {
	Secret       string `toml:"secret"`
	ExpiresHours int    `toml:"expires_hours"`
}

type BackupConfig struct {
	Type                  string `toml:"type"`
	LocalPath             string `toml:"local_path"`
	Endpoint              string `toml:"endpoint"`
	Bucket                string `toml:"bucket"`
	Prefix                string `toml:"prefix"`
	AccessKey             string `toml:"access_key"`
	SecretKey             string `toml:"secret_key"`
	Region                string `toml:"region"`
	ManifestEncryptionKey string `toml:"manifest_encryption_key"`
	CompactionEnabled     bool   `toml:"compaction_enabled"`
	CompactionIntervalMin int    `toml:"compaction_interval_minutes"`
	CompactionThreshold   int    `toml:"compaction_incremental_threshold"`
}

// ArchiveConfig controls migration from the POSIX primary store into the
// separately-prefixed backup storage. It is deliberately disabled by default.
type ArchiveConfig struct {
	Enabled   bool   `toml:"enabled"`
	AfterDays int    `toml:"after_days"`
	BatchSize int    `toml:"batch_size"`
	Prefix    string `toml:"prefix"`
}

// AuditConfig defines retention and verified immutable-archive policy.
type AuditConfig struct {
	HotRetentionDays       int    `toml:"hot_retention_days"`
	ExportRetentionHours   int    `toml:"export_retention_hours"`
	ArchiveEnabled         bool   `toml:"archive_enabled"`
	ArchiveIntervalMinutes int    `toml:"archive_interval_minutes"`
	ArchiveBatchSize       int    `toml:"archive_batch_size"`
	ArchivePrefix          string `toml:"archive_prefix"`
}

type ClamAVConfig struct {
	Host                 string `toml:"host"`
	Port                 int    `toml:"port"`
	TimeoutSeconds       int    `toml:"timeout_seconds"`
	VirusDBMaxAgeHours   int    `toml:"virus_db_max_age_hours"`
	RetryMaxAttempts     int    `toml:"retry_max_attempts"`
	RetryIntervalMinutes int    `toml:"retry_interval_minutes"`
	RetryBatchSize       int    `toml:"retry_batch_size"`
}

// NotificationConfig controls encrypted channel credentials and the durable
// delivery worker. Channel definitions themselves are maintained in MySQL.
type NotificationConfig struct {
	CredentialEncryptionKey string `toml:"credential_encryption_key"`
	WorkerCount             int    `toml:"worker_count"`
	PollIntervalSeconds     int    `toml:"poll_interval_seconds"`
	BatchSize               int    `toml:"batch_size"`
	MaxAttempts             int    `toml:"max_attempts"`
	BaseRetrySeconds        int    `toml:"base_retry_seconds"`
	MaxRetrySeconds         int    `toml:"max_retry_seconds"`
}

func (c ClamAVConfig) Enabled() bool {
	return strings.TrimSpace(c.Host) != ""
}

type LifecycleConfig struct {
	IntervalMinutes         int `toml:"interval_minutes"`
	TrashRetentionDays      int `toml:"trash_retention_days"`
	UploadSessionHours      int `toml:"upload_session_hours"`
	QuarantineRetentionDays int `toml:"quarantine_retention_days"`
	ReconcileBatchSize      int `toml:"reconcile_batch_size"`
}

type LogConfig struct {
	Level          string `toml:"level"`
	Filename       string `toml:"filename"`
	Directory      string `toml:"directory"`
	Format         string `toml:"format"`
	SplitByLevel   bool   `toml:"split_by_level"`
	MaxSize        int    `toml:"max_size"`
	MaxBackups     int    `toml:"max_backups"`
	MaxAge         int    `toml:"max_age"`
	RetentionDays  int    `toml:"retention_days"`
	Compress       bool   `toml:"compress"`
	Console        bool   `toml:"console"`
	RotationTime   string `toml:"rotation_time"`
	ReloadOnSighup bool   `toml:"reload_on_sighup"`
}

var globalConfig *Config

func LoadConfig(path string) error {
	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return err
	}
	applyDefaults(&cfg)
	if err := applyEnv(&cfg); err != nil {
		return err
	}
	if cfg.JWT.Secret == "" && cfg.Server.Mode != "release" {
		secret, err := randomSecret(32)
		if err != nil {
			return fmt.Errorf("generate development JWT secret: %w", err)
		}
		cfg.JWT.Secret = secret
	}
	if err := validate(&cfg); err != nil {
		return err
	}
	globalConfig = &cfg
	return nil
}

func GetConfig() *Config {
	return globalConfig
}

// SetTestConfig replaces the process configuration for isolated unit tests.
func SetTestConfig(cfg *Config) {
	globalConfig = cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}
	if cfg.Server.ReadHeaderTimeoutSeconds <= 0 {
		cfg.Server.ReadHeaderTimeoutSeconds = 5
	}
	if cfg.Server.ReadTimeoutSeconds <= 0 {
		cfg.Server.ReadTimeoutSeconds = 30
	}
	if cfg.Server.WriteTimeoutSeconds <= 0 {
		cfg.Server.WriteTimeoutSeconds = 60
	}
	if cfg.Server.IdleTimeoutSeconds <= 0 {
		cfg.Server.IdleTimeoutSeconds = 120
	}
	if cfg.Server.ShutdownTimeoutSeconds <= 0 {
		cfg.Server.ShutdownTimeoutSeconds = 15
	}
	if cfg.Server.MaxRequestBodyBytes <= 0 {
		cfg.Server.MaxRequestBodyBytes = 10 << 20
	}
	if cfg.Server.MaxUploadBodyBytes <= 0 {
		cfg.Server.MaxUploadBodyBytes = 110 << 20
	}
	if cfg.Database.MaxIdleConns <= 0 {
		cfg.Database.MaxIdleConns = 10
	}
	if cfg.Database.MaxOpenConns <= 0 {
		cfg.Database.MaxOpenConns = 100
	}
	if cfg.AuditDatabase.MaxIdleConns <= 0 {
		cfg.AuditDatabase.MaxIdleConns = 2
	}
	if cfg.AuditDatabase.MaxOpenConns <= 0 {
		cfg.AuditDatabase.MaxOpenConns = 5
	}
	if cfg.Storage.RootPath == "" {
		cfg.Storage.RootPath = "data/storage"
	}
	if cfg.Storage.StagingPath == "" {
		cfg.Storage.StagingPath = "data/staging"
	}
	if cfg.Storage.Mode == "" {
		cfg.Storage.Mode = "local"
	}
	if cfg.Storage.MinFreeBytes <= 0 {
		cfg.Storage.MinFreeBytes = 5 << 30
	}
	if cfg.Storage.MinFreePercent <= 0 {
		cfg.Storage.MinFreePercent = 10
	}
	if cfg.Storage.WarnFreePercent <= 0 {
		cfg.Storage.WarnFreePercent = 20
	}
	if cfg.Storage.HealthCheckIntervalMinutes <= 0 {
		cfg.Storage.HealthCheckIntervalMinutes = 5
	}
	if len(cfg.Upload.AllowedExtensions) == 0 {
		cfg.Upload.AllowedExtensions = []string{
			".txt", ".md", ".csv", ".json", ".xml", ".yaml", ".yml", ".log", ".conf", ".ini", ".rtf",
			".pdf", ".doc", ".docx", ".docm", ".xls", ".xlsx", ".xlsm", ".ppt", ".pptx", ".pptm",
			".odt", ".ods", ".odp", ".zip", ".7z", ".rar", ".tar", ".gz", ".tgz",
			".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".svg",
			".mp3", ".wav", ".m4a", ".mp4", ".mov", ".avi", ".mkv", ".webm",
		}
	}
	if cfg.Preview.MaxBinaryBytes <= 0 {
		cfg.Preview.MaxBinaryBytes = 25 << 20
	}
	if cfg.Preview.MaxTextBytes <= 0 {
		cfg.Preview.MaxTextBytes = 1 << 20
	}
	if cfg.BatchDownload.MaxFiles <= 0 {
		cfg.BatchDownload.MaxFiles = 1000
	}
	if cfg.BatchDownload.MaxTotalBytes <= 0 {
		cfg.BatchDownload.MaxTotalBytes = 5 << 30
	}
	if cfg.BatchDownload.RetentionHours <= 0 {
		cfg.BatchDownload.RetentionHours = 24
	}
	if cfg.BatchDownload.WorkerCount <= 0 {
		cfg.BatchDownload.WorkerCount = 2
	}
	if cfg.JWT.ExpiresHours <= 0 {
		cfg.JWT.ExpiresHours = 24
	}
	if cfg.Backup.Type == "" {
		cfg.Backup.Type = "local"
	}
	if cfg.Backup.Prefix == "" {
		cfg.Backup.Prefix = "fileshare-backup/"
	}
	if strings.EqualFold(cfg.Backup.Type, "local") && cfg.Backup.LocalPath == "" {
		cfg.Backup.LocalPath = "data/backup"
	}
	if cfg.Backup.CompactionIntervalMin <= 0 {
		cfg.Backup.CompactionIntervalMin = 60
	}
	if cfg.Backup.CompactionThreshold <= 0 {
		cfg.Backup.CompactionThreshold = 30
	}
	if cfg.Archive.AfterDays <= 0 {
		cfg.Archive.AfterDays = 365
	}
	if cfg.Archive.BatchSize <= 0 {
		cfg.Archive.BatchSize = 100
	}
	if cfg.Archive.Prefix == "" {
		cfg.Archive.Prefix = "fileshare-archive/"
	}
	if cfg.Audit.HotRetentionDays <= 0 {
		cfg.Audit.HotRetentionDays = 365
	}
	if cfg.Audit.ExportRetentionHours <= 0 {
		cfg.Audit.ExportRetentionHours = 24
	}
	if cfg.Audit.ArchiveIntervalMinutes <= 0 {
		cfg.Audit.ArchiveIntervalMinutes = 60
	}
	if cfg.Audit.ArchiveBatchSize <= 0 {
		cfg.Audit.ArchiveBatchSize = 10000
	}
	if cfg.Audit.ArchivePrefix == "" {
		cfg.Audit.ArchivePrefix = "fileshare-audit/"
	}
	if cfg.ClamAV.Port <= 0 {
		cfg.ClamAV.Port = 3310
	}
	if cfg.ClamAV.TimeoutSeconds <= 0 {
		cfg.ClamAV.TimeoutSeconds = 60
	}
	if cfg.ClamAV.VirusDBMaxAgeHours <= 0 {
		cfg.ClamAV.VirusDBMaxAgeHours = 48
	}
	if cfg.ClamAV.RetryMaxAttempts <= 0 {
		cfg.ClamAV.RetryMaxAttempts = 3
	}
	if cfg.ClamAV.RetryIntervalMinutes <= 0 {
		cfg.ClamAV.RetryIntervalMinutes = 5
	}
	if cfg.ClamAV.RetryBatchSize <= 0 {
		cfg.ClamAV.RetryBatchSize = 50
	}
	if cfg.Notification.WorkerCount <= 0 {
		cfg.Notification.WorkerCount = 2
	}
	if cfg.Notification.PollIntervalSeconds <= 0 {
		cfg.Notification.PollIntervalSeconds = 5
	}
	if cfg.Notification.BatchSize <= 0 {
		cfg.Notification.BatchSize = 50
	}
	if cfg.Notification.MaxAttempts <= 0 {
		cfg.Notification.MaxAttempts = 5
	}
	if cfg.Notification.BaseRetrySeconds <= 0 {
		cfg.Notification.BaseRetrySeconds = 30
	}
	if cfg.Notification.MaxRetrySeconds <= 0 {
		cfg.Notification.MaxRetrySeconds = 3600
	}
	if cfg.Lifecycle.IntervalMinutes <= 0 {
		cfg.Lifecycle.IntervalMinutes = 15
	}
	if cfg.Lifecycle.TrashRetentionDays <= 0 {
		cfg.Lifecycle.TrashRetentionDays = 30
	}
	if cfg.Lifecycle.UploadSessionHours <= 0 {
		cfg.Lifecycle.UploadSessionHours = 24
	}
	if cfg.Lifecycle.QuarantineRetentionDays <= 0 {
		cfg.Lifecycle.QuarantineRetentionDays = 7
	}
	if cfg.Lifecycle.ReconcileBatchSize <= 0 {
		cfg.Lifecycle.ReconcileBatchSize = 100
	}
	if cfg.Log.Level == "" {
		cfg.Log.Level = "info"
	}
	if cfg.Log.Format == "" {
		cfg.Log.Format = "json"
	}
	if cfg.Log.Directory == "" && cfg.Log.Filename == "" {
		cfg.Log.Directory = "logs/server"
	}
	if cfg.Log.RotationTime == "" {
		cfg.Log.RotationTime = "day"
	}
	if cfg.Log.RetentionDays <= 0 {
		cfg.Log.RetentionDays = 7
	}
}

func applyEnv(cfg *Config) error {
	setString := func(key string, target *string) {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			*target = value
		}
	}
	setInt := func(key string, target *int) error {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			return nil
		}
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("%s must be an integer: %w", key, err)
		}
		*target = parsed
		return nil
	}
	setInt64 := func(key string, target *int64) error {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			return nil
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return fmt.Errorf("%s must be an integer: %w", key, err)
		}
		*target = parsed
		return nil
	}
	setBool := func(key string, target *bool) error {
		value := strings.TrimSpace(os.Getenv(key))
		if value == "" {
			return nil
		}
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("%s must be a boolean: %w", key, err)
		}
		*target = parsed
		return nil
	}

	setString("FILESHARE_DB_HOST", &cfg.Database.Host)
	setString("FILESHARE_DB_USER", &cfg.Database.User)
	setString("FILESHARE_DB_PASSWORD", &cfg.Database.Password)
	setString("FILESHARE_DB_NAME", &cfg.Database.DBName)
	setString("FILESHARE_AUDIT_DB_HOST", &cfg.AuditDatabase.Host)
	setString("FILESHARE_AUDIT_DB_USER", &cfg.AuditDatabase.User)
	setString("FILESHARE_AUDIT_DB_PASSWORD", &cfg.AuditDatabase.Password)
	setString("FILESHARE_AUDIT_DB_NAME", &cfg.AuditDatabase.DBName)
	setString("FILESHARE_JWT_SECRET", &cfg.JWT.Secret)
	setString("FILESHARE_WEB_URL", &cfg.Server.WebURL)
	setString("FILESHARE_STORAGE_ROOT", &cfg.Storage.RootPath)
	setString("FILESHARE_STORAGE_STAGING", &cfg.Storage.StagingPath)
	setString("FILESHARE_STORAGE_MODE", &cfg.Storage.Mode)
	setString("FILESHARE_BACKUP_TYPE", &cfg.Backup.Type)
	setString("FILESHARE_BACKUP_LOCAL_PATH", &cfg.Backup.LocalPath)
	setString("FILESHARE_BACKUP_ENDPOINT", &cfg.Backup.Endpoint)
	setString("FILESHARE_BACKUP_BUCKET", &cfg.Backup.Bucket)
	setString("FILESHARE_BACKUP_PREFIX", &cfg.Backup.Prefix)
	setString("FILESHARE_BACKUP_ACCESS_KEY", &cfg.Backup.AccessKey)
	setString("FILESHARE_BACKUP_SECRET_KEY", &cfg.Backup.SecretKey)
	setString("FILESHARE_BACKUP_REGION", &cfg.Backup.Region)
	setString("FILESHARE_BACKUP_MANIFEST_KEY", &cfg.Backup.ManifestEncryptionKey)
	setString("FILESHARE_AUDIT_ARCHIVE_PREFIX", &cfg.Audit.ArchivePrefix)
	setString("FILESHARE_ARCHIVE_PREFIX", &cfg.Archive.Prefix)
	setString("FILESHARE_CLAMAV_HOST", &cfg.ClamAV.Host)
	setString("FILESHARE_NOTIFICATION_CREDENTIAL_KEY", &cfg.Notification.CredentialEncryptionKey)
	if value := strings.TrimSpace(os.Getenv("FILESHARE_ALLOWED_EXTENSIONS")); value != "" {
		cfg.Upload.AllowedExtensions = strings.Split(value, ",")
	}
	if err := setInt("FILESHARE_DB_PORT", &cfg.Database.Port); err != nil {
		return err
	}
	if err := setInt("FILESHARE_AUDIT_DB_PORT", &cfg.AuditDatabase.Port); err != nil {
		return err
	}
	if err := setBool("FILESHARE_DB_LOG_QUERIES", &cfg.Database.LogQueries); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_PORT", &cfg.ClamAV.Port); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_TIMEOUT_SECONDS", &cfg.ClamAV.TimeoutSeconds); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_VIRUS_DB_MAX_AGE_HOURS", &cfg.ClamAV.VirusDBMaxAgeHours); err != nil {
		return err
	}
	if err := setInt64("FILESHARE_STORAGE_MIN_FREE_BYTES", &cfg.Storage.MinFreeBytes); err != nil {
		return err
	}
	if err := setInt("FILESHARE_STORAGE_MIN_FREE_PERCENT", &cfg.Storage.MinFreePercent); err != nil {
		return err
	}
	if err := setInt("FILESHARE_STORAGE_WARN_FREE_PERCENT", &cfg.Storage.WarnFreePercent); err != nil {
		return err
	}
	if err := setInt("FILESHARE_STORAGE_HEALTH_INTERVAL_MINUTES", &cfg.Storage.HealthCheckIntervalMinutes); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_RETRY_MAX_ATTEMPTS", &cfg.ClamAV.RetryMaxAttempts); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_RETRY_INTERVAL_MINUTES", &cfg.ClamAV.RetryIntervalMinutes); err != nil {
		return err
	}
	if err := setInt("FILESHARE_CLAMAV_RETRY_BATCH_SIZE", &cfg.ClamAV.RetryBatchSize); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_WORKER_COUNT", &cfg.Notification.WorkerCount); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_POLL_SECONDS", &cfg.Notification.PollIntervalSeconds); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_BATCH_SIZE", &cfg.Notification.BatchSize); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_MAX_ATTEMPTS", &cfg.Notification.MaxAttempts); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_BASE_RETRY_SECONDS", &cfg.Notification.BaseRetrySeconds); err != nil {
		return err
	}
	if err := setInt("FILESHARE_NOTIFICATION_MAX_RETRY_SECONDS", &cfg.Notification.MaxRetrySeconds); err != nil {
		return err
	}
	if err := setInt("FILESHARE_SERVER_PORT", &cfg.Server.Port); err != nil {
		return err
	}
	if err := setInt("FILESHARE_BATCH_DOWNLOAD_MAX_FILES", &cfg.BatchDownload.MaxFiles); err != nil {
		return err
	}
	if err := setInt64("FILESHARE_PREVIEW_MAX_BINARY_BYTES", &cfg.Preview.MaxBinaryBytes); err != nil {
		return err
	}
	if err := setInt64("FILESHARE_PREVIEW_MAX_TEXT_BYTES", &cfg.Preview.MaxTextBytes); err != nil {
		return err
	}
	if err := setInt64("FILESHARE_BATCH_DOWNLOAD_MAX_TOTAL_BYTES", &cfg.BatchDownload.MaxTotalBytes); err != nil {
		return err
	}
	if err := setInt("FILESHARE_BATCH_DOWNLOAD_RETENTION_HOURS", &cfg.BatchDownload.RetentionHours); err != nil {
		return err
	}
	if err := setInt("FILESHARE_BATCH_DOWNLOAD_WORKER_COUNT", &cfg.BatchDownload.WorkerCount); err != nil {
		return err
	}
	if err := setBool("FILESHARE_BACKUP_COMPACTION_ENABLED", &cfg.Backup.CompactionEnabled); err != nil {
		return err
	}
	if err := setInt("FILESHARE_BACKUP_COMPACTION_INTERVAL_MINUTES", &cfg.Backup.CompactionIntervalMin); err != nil {
		return err
	}
	if err := setInt("FILESHARE_BACKUP_COMPACTION_INCREMENTAL_THRESHOLD", &cfg.Backup.CompactionThreshold); err != nil {
		return err
	}
	if err := setInt("FILESHARE_QUARANTINE_RETENTION_DAYS", &cfg.Lifecycle.QuarantineRetentionDays); err != nil {
		return err
	}
	if err := setInt("FILESHARE_RECONCILE_BATCH_SIZE", &cfg.Lifecycle.ReconcileBatchSize); err != nil {
		return err
	}
	if err := setBool("FILESHARE_ARCHIVE_ENABLED", &cfg.Archive.Enabled); err != nil {
		return err
	}
	if err := setInt("FILESHARE_ARCHIVE_AFTER_DAYS", &cfg.Archive.AfterDays); err != nil {
		return err
	}
	if err := setInt("FILESHARE_ARCHIVE_BATCH_SIZE", &cfg.Archive.BatchSize); err != nil {
		return err
	}
	if err := setInt("FILESHARE_AUDIT_HOT_RETENTION_DAYS", &cfg.Audit.HotRetentionDays); err != nil {
		return err
	}
	if err := setInt("FILESHARE_AUDIT_EXPORT_RETENTION_HOURS", &cfg.Audit.ExportRetentionHours); err != nil {
		return err
	}
	if err := setBool("FILESHARE_AUDIT_ARCHIVE_ENABLED", &cfg.Audit.ArchiveEnabled); err != nil {
		return err
	}
	if err := setInt("FILESHARE_AUDIT_ARCHIVE_INTERVAL_MINUTES", &cfg.Audit.ArchiveIntervalMinutes); err != nil {
		return err
	}
	if err := setInt("FILESHARE_AUDIT_ARCHIVE_BATCH_SIZE", &cfg.Audit.ArchiveBatchSize); err != nil {
		return err
	}
	return nil
}

func validate(cfg *Config) error {
	if cfg.Server.Mode != "debug" && cfg.Server.Mode != "release" {
		return fmt.Errorf("server.mode must be debug or release")
	}
	if cfg.Server.Port < 1 || cfg.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535")
	}
	webURL, err := url.Parse(cfg.Server.WebURL)
	if cfg.Server.WebURL == "" || err != nil || webURL.Scheme == "" || webURL.Host == "" {
		return fmt.Errorf("server.web_url must be an absolute URL")
	}
	if cfg.Database.Host == "" || cfg.Database.User == "" || cfg.Database.DBName == "" || cfg.Database.Port < 1 || cfg.Database.Port > 65535 {
		return fmt.Errorf("database host, port, user and dbname are required")
	}
	if cfg.Database.MaxOpenConns < 1 || cfg.Database.MaxOpenConns > 1000 || cfg.Database.MaxIdleConns < 0 || cfg.Database.MaxIdleConns > cfg.Database.MaxOpenConns {
		return fmt.Errorf("database connection pool settings are invalid")
	}
	if cfg.AuditDatabase.Host == "" {
		cfg.AuditDatabase.Host = cfg.Database.Host
	}
	if cfg.AuditDatabase.Port == 0 {
		cfg.AuditDatabase.Port = cfg.Database.Port
	}
	if cfg.AuditDatabase.DBName == "" {
		cfg.AuditDatabase.DBName = cfg.Database.DBName
	}
	if cfg.AuditDatabase.MaxOpenConns < 1 || cfg.AuditDatabase.MaxOpenConns > 100 || cfg.AuditDatabase.MaxIdleConns < 0 || cfg.AuditDatabase.MaxIdleConns > cfg.AuditDatabase.MaxOpenConns {
		return fmt.Errorf("audit_database connection pool settings are invalid")
	}
	if cfg.JWT.ExpiresHours < 1 || cfg.JWT.ExpiresHours > 720 {
		return fmt.Errorf("jwt.expires_hours must be between 1 and 720")
	}
	if cfg.Server.MaxRequestBodyBytes < 1024 || cfg.Server.MaxRequestBodyBytes > 100<<20 {
		return fmt.Errorf("server.max_request_body_bytes must be between 1 KiB and 100 MiB")
	}
	if cfg.Server.MaxUploadBodyBytes < cfg.Server.MaxRequestBodyBytes || cfg.Server.MaxUploadBodyBytes > 512<<20 {
		return fmt.Errorf("server.max_upload_body_bytes must be between max_request_body_bytes and 512 MiB")
	}
	if cfg.Preview.MaxBinaryBytes < 1<<20 || cfg.Preview.MaxBinaryBytes > 100<<20 {
		return fmt.Errorf("preview.max_binary_bytes must be between 1 MiB and 100 MiB")
	}
	if cfg.Preview.MaxTextBytes < 1<<10 || cfg.Preview.MaxTextBytes > 10<<20 || cfg.Preview.MaxTextBytes > cfg.Preview.MaxBinaryBytes {
		return fmt.Errorf("preview.max_text_bytes must be between 1 KiB and 10 MiB and not exceed max_binary_bytes")
	}
	if cfg.Storage.RootPath == "" || cfg.Storage.StagingPath == "" {
		return fmt.Errorf("storage root_path and staging_path are required")
	}
	if cfg.Storage.MinFreeBytes < 1<<20 || cfg.Storage.MinFreeBytes > 1<<50 || cfg.Storage.MinFreePercent < 1 || cfg.Storage.MinFreePercent > 98 || cfg.Storage.WarnFreePercent <= cfg.Storage.MinFreePercent || cfg.Storage.WarnFreePercent > 99 || cfg.Storage.HealthCheckIntervalMinutes < 1 || cfg.Storage.HealthCheckIntervalMinutes > 1440 {
		return fmt.Errorf("storage free-space thresholds or health interval are invalid")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.Storage.Mode)) {
	case "local", "cloud_mount":
		cfg.Storage.Mode = strings.ToLower(strings.TrimSpace(cfg.Storage.Mode))
	default:
		return fmt.Errorf("storage.mode must be local or cloud_mount")
	}
	if cfg.ClamAV.Port < 1 || cfg.ClamAV.Port > 65535 || cfg.ClamAV.TimeoutSeconds < 1 || cfg.ClamAV.TimeoutSeconds > 600 ||
		cfg.ClamAV.VirusDBMaxAgeHours < 1 || cfg.ClamAV.VirusDBMaxAgeHours > 720 ||
		cfg.ClamAV.RetryMaxAttempts < 1 || cfg.ClamAV.RetryMaxAttempts > 20 ||
		cfg.ClamAV.RetryIntervalMinutes < 1 || cfg.ClamAV.RetryIntervalMinutes > 1440 ||
		cfg.ClamAV.RetryBatchSize < 1 || cfg.ClamAV.RetryBatchSize > 1000 {
		return fmt.Errorf("clamav port, timeout, virus database age or retry policy is invalid")
	}
	if cfg.Notification.WorkerCount < 1 || cfg.Notification.WorkerCount > 16 ||
		cfg.Notification.PollIntervalSeconds < 1 || cfg.Notification.PollIntervalSeconds > 300 ||
		cfg.Notification.BatchSize < 1 || cfg.Notification.BatchSize > 1000 ||
		cfg.Notification.MaxAttempts < 1 || cfg.Notification.MaxAttempts > 20 ||
		cfg.Notification.BaseRetrySeconds < 1 || cfg.Notification.BaseRetrySeconds > 86400 ||
		cfg.Notification.MaxRetrySeconds < cfg.Notification.BaseRetrySeconds || cfg.Notification.MaxRetrySeconds > 604800 {
		return fmt.Errorf("notification worker or retry policy is invalid")
	}
	if cfg.Notification.CredentialEncryptionKey != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Notification.CredentialEncryptionKey))
		if err != nil || len(key) != 32 {
			return fmt.Errorf("notification.credential_encryption_key must be base64-encoded 32 bytes")
		}
	}
	switch strings.ToLower(cfg.Backup.Type) {
	case "local", "s3", "minio", "oss":
	default:
		return fmt.Errorf("backup.type must be local, s3, minio, or oss")
	}
	if strings.EqualFold(cfg.Backup.Type, "local") {
		if strings.TrimSpace(cfg.Backup.LocalPath) == "" {
			return fmt.Errorf("backup.local_path is required for local storage")
		}
	} else if strings.TrimSpace(cfg.Backup.Endpoint) == "" || strings.TrimSpace(cfg.Backup.Bucket) == "" || strings.TrimSpace(cfg.Backup.Region) == "" || strings.TrimSpace(cfg.Backup.AccessKey) == "" || strings.TrimSpace(cfg.Backup.SecretKey) == "" {
		return fmt.Errorf("backup endpoint, bucket, region, access_key and secret_key are required for object storage")
	}
	backupPrefix := strings.TrimSpace(strings.ReplaceAll(cfg.Backup.Prefix, "\\", "/"))
	if backupPrefix == "" || strings.HasPrefix(backupPrefix, "/") || strings.ContainsRune(backupPrefix, '\x00') {
		return fmt.Errorf("backup.prefix must be a relative object key prefix")
	}
	for _, part := range strings.Split(strings.TrimSuffix(backupPrefix, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("backup.prefix must be a relative object key prefix")
		}
	}
	cfg.Backup.Prefix = strings.TrimSuffix(backupPrefix, "/") + "/"
	archivePrefix := strings.TrimSpace(strings.ReplaceAll(cfg.Archive.Prefix, "\\", "/"))
	if archivePrefix == "" || strings.HasPrefix(archivePrefix, "/") || strings.ContainsRune(archivePrefix, '\x00') {
		return fmt.Errorf("archive.prefix must be a relative object key prefix")
	}
	for _, part := range strings.Split(strings.TrimSuffix(archivePrefix, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("archive.prefix must be a relative object key prefix")
		}
	}
	cfg.Archive.Prefix = strings.TrimSuffix(archivePrefix, "/") + "/"
	if cfg.Archive.AfterDays < 1 || cfg.Archive.AfterDays > 36500 {
		return fmt.Errorf("archive.after_days must be between 1 and 36500")
	}
	if cfg.Archive.BatchSize < 1 || cfg.Archive.BatchSize > 10000 {
		return fmt.Errorf("archive.batch_size must be between 1 and 10000")
	}
	if cfg.Archive.Enabled && cfg.Storage.Mode != "local" {
		return fmt.Errorf("archive migration can only be enabled for local primary storage")
	}
	if cfg.Audit.HotRetentionDays < 30 || cfg.Audit.HotRetentionDays > 36500 {
		return fmt.Errorf("audit.hot_retention_days must be between 30 and 36500")
	}
	if cfg.Audit.ExportRetentionHours < 1 || cfg.Audit.ExportRetentionHours > 720 {
		return fmt.Errorf("audit.export_retention_hours must be between 1 and 720")
	}
	if cfg.Audit.ArchiveIntervalMinutes < 5 || cfg.Audit.ArchiveIntervalMinutes > 10080 {
		return fmt.Errorf("audit.archive_interval_minutes must be between 5 and 10080")
	}
	if cfg.Audit.ArchiveBatchSize < 100 || cfg.Audit.ArchiveBatchSize > 100000 {
		return fmt.Errorf("audit.archive_batch_size must be between 100 and 100000")
	}
	auditArchivePrefix := strings.TrimSpace(strings.ReplaceAll(cfg.Audit.ArchivePrefix, "\\", "/"))
	if auditArchivePrefix == "" || strings.HasPrefix(auditArchivePrefix, "/") || strings.ContainsRune(auditArchivePrefix, '\x00') {
		return fmt.Errorf("audit.archive_prefix must be a relative object key prefix")
	}
	for _, part := range strings.Split(strings.TrimSuffix(auditArchivePrefix, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			return fmt.Errorf("audit.archive_prefix must be a relative object key prefix")
		}
	}
	cfg.Audit.ArchivePrefix = strings.TrimSuffix(auditArchivePrefix, "/") + "/"
	if cfg.Audit.ArchiveEnabled && strings.TrimSpace(cfg.Backup.ManifestEncryptionKey) == "" {
		return fmt.Errorf("audit archiving requires backup.manifest_encryption_key")
	}
	if cfg.Audit.ArchiveEnabled {
		if cfg.Database.AutoMigrate {
			return fmt.Errorf("audit archiving requires database.auto_migrate=false and a separate migration account")
		}
		if strings.TrimSpace(cfg.AuditDatabase.Host) == "" || cfg.AuditDatabase.Port < 1 || cfg.AuditDatabase.Port > 65535 || strings.TrimSpace(cfg.AuditDatabase.User) == "" || strings.TrimSpace(cfg.AuditDatabase.DBName) == "" {
			return fmt.Errorf("audit archiving requires audit_database host, port, user and dbname")
		}
		if cfg.Server.Mode == "release" && strings.TrimSpace(cfg.AuditDatabase.Password) == "" {
			return fmt.Errorf("release mode audit archiving requires FILESHARE_AUDIT_DB_PASSWORD")
		}
	}
	if cfg.Backup.ManifestEncryptionKey != "" {
		key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Backup.ManifestEncryptionKey))
		if err != nil || len(key) != 32 {
			return fmt.Errorf("backup.manifest_encryption_key must be base64-encoded 32 bytes")
		}
	}
	if cfg.Backup.CompactionIntervalMin < 5 || cfg.Backup.CompactionIntervalMin > 10080 {
		return fmt.Errorf("backup.compaction_interval_minutes must be between 5 and 10080")
	}
	if cfg.Backup.CompactionThreshold < 2 || cfg.Backup.CompactionThreshold > 1000 {
		return fmt.Errorf("backup.compaction_incremental_threshold must be between 2 and 1000")
	}
	if len(cfg.Upload.AllowedExtensions) > 256 {
		return fmt.Errorf("upload.allowed_extensions must contain at most 256 entries")
	}
	normalizedExtensions := make([]string, 0, len(cfg.Upload.AllowedExtensions))
	seenExtensions := make(map[string]struct{}, len(cfg.Upload.AllowedExtensions))
	for _, value := range cfg.Upload.AllowedExtensions {
		extension := strings.ToLower(strings.TrimSpace(value))
		if extension == "*" {
			normalizedExtensions = []string{"*"}
			break
		}
		if extension != "" && !strings.HasPrefix(extension, ".") {
			extension = "." + extension
		}
		if extension == "" || extension == "." || len(extension) > 32 || strings.ContainsAny(extension, "/\\\x00") {
			return fmt.Errorf("upload.allowed_extensions contains invalid extension %q", value)
		}
		if _, exists := seenExtensions[extension]; exists {
			continue
		}
		seenExtensions[extension] = struct{}{}
		normalizedExtensions = append(normalizedExtensions, extension)
	}
	if len(normalizedExtensions) == 0 {
		return fmt.Errorf("upload.allowed_extensions must not be empty")
	}
	cfg.Upload.AllowedExtensions = normalizedExtensions
	if cfg.BatchDownload.MaxFiles < 1 || cfg.BatchDownload.MaxFiles > 10000 {
		return fmt.Errorf("batch_download.max_files must be between 1 and 10000")
	}
	if cfg.BatchDownload.MaxTotalBytes < 1<<20 || cfg.BatchDownload.MaxTotalBytes > 1<<40 {
		return fmt.Errorf("batch_download.max_total_bytes must be between 1 MiB and 1 TiB")
	}
	if cfg.BatchDownload.RetentionHours < 1 || cfg.BatchDownload.RetentionHours > 720 {
		return fmt.Errorf("batch_download.retention_hours must be between 1 and 720")
	}
	if cfg.BatchDownload.WorkerCount < 1 || cfg.BatchDownload.WorkerCount > 16 {
		return fmt.Errorf("batch_download.worker_count must be between 1 and 16")
	}
	if cfg.Lifecycle.QuarantineRetentionDays < 1 || cfg.Lifecycle.QuarantineRetentionDays > 365 {
		return fmt.Errorf("lifecycle.quarantine_retention_days must be between 1 and 365")
	}
	if cfg.Lifecycle.ReconcileBatchSize < 1 || cfg.Lifecycle.ReconcileBatchSize > 1000 {
		return fmt.Errorf("lifecycle.reconcile_batch_size must be between 1 and 1000")
	}
	if cfg.Server.Mode == "release" {
		if cfg.Database.AutoMigrate {
			return fmt.Errorf("release mode requires database.auto_migrate=false")
		}
		if len(cfg.JWT.Secret) < 32 || cfg.JWT.Secret == "super-secret-key-change-in-prod" {
			return fmt.Errorf("release mode requires a non-default JWT secret of at least 32 characters")
		}
		if cfg.Database.Password == "" {
			return fmt.Errorf("release mode requires an externally supplied database password")
		}
		if !filepath.IsAbs(cfg.Storage.RootPath) || !filepath.IsAbs(cfg.Storage.StagingPath) {
			return fmt.Errorf("release mode requires absolute storage paths")
		}
		backupConfigured := (strings.EqualFold(cfg.Backup.Type, "local") && strings.TrimSpace(cfg.Backup.LocalPath) != "") ||
			(!strings.EqualFold(cfg.Backup.Type, "local") && strings.TrimSpace(cfg.Backup.Endpoint) != "" && strings.TrimSpace(cfg.Backup.Bucket) != "")
		if backupConfigured && strings.TrimSpace(cfg.Backup.ManifestEncryptionKey) == "" {
			return fmt.Errorf("release mode requires FILESHARE_BACKUP_MANIFEST_KEY when backup storage is configured")
		}
	}
	return nil
}

func randomSecret(length int) (string, error) {
	buffer := make([]byte, length)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}
