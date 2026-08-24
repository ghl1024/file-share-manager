/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"

	"file-share-manager/server/internal/config"
)

var ErrArchiveUnavailable = errors.New("archive storage is not configured")

// VersionReader keeps primary and archived object reads behind one interface.
// Standard objects remain POSIX seekable files; archived objects are streamed
// from the separately-prefixed configured backup storage.
type VersionReader struct {
	primary       *POSIX
	archive       ArchiveStorage
	archiveConfig config.BackupConfig
	prefix        string
	mode          string
	mu            sync.Mutex
}

func NewConfiguredVersionReader(ctx context.Context, cfg *config.Config) (*VersionReader, error) {
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	primary, err := NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	_ = ctx
	reader := &VersionReader{primary: primary, archiveConfig: cfg.Backup, prefix: cfg.Archive.Prefix, mode: strings.ToLower(strings.TrimSpace(cfg.Storage.Mode))}
	return reader, nil
}

func (r *VersionReader) Open(storageClass, storageKey string) (io.ReadCloser, error) {
	switch normalizeStorageClass(storageClass) {
	case "standard":
		file, err := r.primary.OpenObject(storageKey)
		if err == nil || !errors.Is(err, os.ErrNotExist) || r.mode == "cloud_mount" {
			return file, err
		}
		// A snapshot may be created while the archive transaction is switching
		// all known references. Falling back keeps that immutable snapshot valid.
		archive, archiveErr := r.archiveStorage()
		if archiveErr != nil {
			return nil, archiveErr
		}
		return archive.Get(ArchiveObjectKey(r.prefix, storageKey))
	case "archive":
		if r.mode == "cloud_mount" {
			return r.primary.OpenObject(storageKey)
		}
		archive, archiveErr := r.archiveStorage()
		if archiveErr != nil {
			return nil, archiveErr
		}
		reader, err := archive.Get(ArchiveObjectKey(r.prefix, storageKey))
		if err == nil {
			return reader, nil
		}
		// Database commit precedes local deletion; this also covers a recoverable
		// cleanup failure without taking an otherwise readable object offline.
		if file, localErr := r.primary.OpenObject(storageKey); localErr == nil {
			return file, nil
		}
		return nil, err
	case "glacier", "restoring":
		return nil, ErrArchiveRestoreRequired
	default:
		return nil, ErrArchiveUnavailable
	}
}

func ArchiveObjectKey(prefix, storageKey string) string {
	return strings.TrimSuffix(strings.TrimSpace(prefix), "/") + "/" + strings.TrimPrefix(strings.TrimSpace(storageKey), "/")
}

func normalizeStorageClass(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "standard"
	}
	return value
}

func (r *VersionReader) Primary() *POSIX { return r.primary }

func (r *VersionReader) Remove(storageClass, storageKey string) error {
	if r.mode == "cloud_mount" || normalizeStorageClass(storageClass) == "standard" {
		return r.primary.RemoveObject(storageKey)
	}
	archive, err := r.archiveStorage()
	if err != nil {
		return err
	}
	return archive.Delete(ArchiveObjectKey(r.prefix, storageKey))
}

func (r *VersionReader) archiveStorage() (ArchiveStorage, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.archive != nil {
		return r.archive, nil
	}
	configured, err := NewConfiguredBackupStorage(context.Background(), r.archiveConfig)
	if err != nil {
		return nil, err
	}
	archive, ok := configured.(ArchiveStorage)
	if !ok {
		return nil, ErrArchiveUnavailable
	}
	r.archive = archive
	return archive, nil
}
