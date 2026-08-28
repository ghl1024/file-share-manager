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
	"fmt"
	"strings"

	"file-share-manager/server/internal/config"
)

var ErrBackupStorageConfig = errors.New("invalid backup storage configuration")

func NewConfiguredBackupStorage(ctx context.Context, cfg config.BackupConfig) (BackupStorage, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Type)) {
	case "local":
		store, err := NewLocalBackupStorage(cfg.LocalPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBackupStorageConfig, err)
		}
		return store, nil
	case "s3":
		store, err := NewS3BackupStorage(ctx, cfg.Endpoint, cfg.Bucket, cfg.Region, cfg.AccessKey, cfg.SecretKey, false)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBackupStorageConfig, err)
		}
		return store, nil
	case "minio":
		store, err := NewS3BackupStorage(ctx, cfg.Endpoint, cfg.Bucket, cfg.Region, cfg.AccessKey, cfg.SecretKey, true)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBackupStorageConfig, err)
		}
		return store, nil
	case "oss":
		// Alibaba Cloud OSS only supports virtual-hosted-style requests.
		store, err := NewOSSBackupStorage(ctx, cfg.Endpoint, cfg.Bucket, cfg.Region, cfg.AccessKey, cfg.SecretKey)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrBackupStorageConfig, err)
		}
		return store, nil
	default:
		return nil, fmt.Errorf("%w: unsupported backup storage type", ErrBackupStorageConfig)
	}
}
