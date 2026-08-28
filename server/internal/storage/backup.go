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
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrBackupImmutable = errors.New("completed backup manifest is immutable")
var ErrArchiveRestoreRequired = errors.New("archived object must be restored before download")

type BackupStorage interface {
	Put(key string, reader io.Reader) (int64, string, error)
	Get(key string) (io.ReadCloser, error)
	List(prefix string) ([]string, error)
	Exists(key string) (bool, error)
}

type ArchiveStorage interface {
	BackupStorage
	Delete(key string) error
}

type LocalBackupStorage struct {
	root string
}

func NewLocalBackupStorage(root string) (*LocalBackupStorage, error) {
	root = filepath.Clean(strings.TrimSpace(root))
	if root == "" || root == "." {
		return nil, errors.New("backup root is required")
	}
	if err := os.MkdirAll(root, 0o750); err != nil {
		return nil, err
	}
	return &LocalBackupStorage{root: root}, nil
}

func (s *LocalBackupStorage) Put(key string, reader io.Reader) (int64, string, error) {
	path, err := s.path(key)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".backup-*")
	if err != nil {
		return 0, "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(tmp, hash), reader)
	syncErr := tmp.Sync()
	closeErr := tmp.Close()
	if copyErr != nil {
		return 0, "", copyErr
	}
	if syncErr != nil {
		return 0, "", syncErr
	}
	if closeErr != nil {
		return 0, "", closeErr
	}
	// Hard-linking publishes the fully written temporary file only when the key
	// does not exist, making immutable writes atomic across concurrent workers.
	if err := os.Link(tmpName, path); errors.Is(err, os.ErrExist) {
		return 0, "", ErrBackupImmutable
	} else if err != nil {
		return 0, "", err
	}
	return written, hex.EncodeToString(hash.Sum(nil)), nil
}

func (s *LocalBackupStorage) Get(key string) (io.ReadCloser, error) {
	path, err := s.path(key)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *LocalBackupStorage) List(prefix string) ([]string, error) {
	root := filepath.Join(s.root, filepath.FromSlash(strings.TrimPrefix(prefix, "/")))
	if info, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []string{}, nil
		}
		return nil, err
	} else if !info.IsDir() {
		return nil, errors.New("backup prefix is not a directory")
	}
	keys := make([]string, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return err
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	return keys, err
}

func (s *LocalBackupStorage) Exists(key string) (bool, error) {
	path, err := s.path(key)
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (s *LocalBackupStorage) Delete(key string) error {
	path, err := s.path(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *LocalBackupStorage) path(key string) (string, error) {
	key = filepath.ToSlash(strings.TrimSpace(key))
	if key == "" || filepath.IsAbs(key) || strings.ContainsRune(key, '\x00') {
		return "", ErrInvalidObjectKey
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", ErrInvalidObjectKey
	}
	return filepath.Join(s.root, clean), nil
}

type BackupManifest struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Status         string    `json:"status"`
	WorkspaceID    uint      `json:"workspace_id"`
	ParentID       string    `json:"parent_id,omitempty"`
	ChangeLogStart uint64    `json:"change_log_start,omitempty"`
	ChangeLogEnd   uint64    `json:"change_log_end,omitempty"`
	ObjectCount    int       `json:"object_count"`
	TotalBytes     int64     `json:"total_bytes"`
	CreatedAt      time.Time `json:"created_at"`
	ManifestHash   string    `json:"manifest_hash"`
}

func EncodeManifest(manifest BackupManifest) ([]byte, string, error) {
	manifest.ManifestHash = ""
	data, err := json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	digest := sha256.Sum256(data)
	manifest.ManifestHash = hex.EncodeToString(digest[:])
	data, err = json.Marshal(manifest)
	if err != nil {
		return nil, "", err
	}
	return data, manifest.ManifestHash, nil
}

func DecodeManifest(data []byte) (BackupManifest, error) {
	var manifest BackupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifest, err
	}
	provided := manifest.ManifestHash
	_, expected, err := EncodeManifest(manifest)
	if err != nil {
		return manifest, err
	}
	if provided == "" || provided != expected {
		return manifest, fmt.Errorf("manifest hash mismatch")
	}
	return manifest, nil
}

func ManifestReader(manifest BackupManifest) (io.Reader, string, error) {
	data, hash, err := EncodeManifest(manifest)
	if err != nil {
		return nil, "", err
	}
	return bytes.NewReader(data), hash, nil
}
