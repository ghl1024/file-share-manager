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
	"path/filepath"
	"strings"
	"testing"

	"file-share-manager/server/internal/config"
)

func TestConfiguredVersionReaderReadsAndDeletesArchive(t *testing.T) {
	root := t.TempDir()
	backupRoot := t.TempDir()
	cfg := &config.Config{
		Storage: config.StorageConfig{RootPath: root, StagingPath: filepath.Join(root, "staging"), Mode: "local"},
		Backup:  config.BackupConfig{Type: "local", LocalPath: backupRoot},
		Archive: config.ArchiveConfig{Prefix: "fileshare-archive/"},
	}
	reader, err := NewConfiguredVersionReader(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	key := "objects/7/example"
	archiveKey := ArchiveObjectKey(cfg.Archive.Prefix, key)
	archive, err := reader.archiveStorage()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := archive.Put(archiveKey, strings.NewReader("archived")); err != nil {
		t.Fatal(err)
	}
	opened, err := reader.Open("archive", key)
	if err != nil {
		t.Fatal(err)
	}
	payload, readErr := io.ReadAll(opened)
	closeErr := opened.Close()
	if readErr != nil || closeErr != nil || string(payload) != "archived" {
		t.Fatalf("payload = %q, read = %v, close = %v", payload, readErr, closeErr)
	}
	if err := reader.Remove("archive", key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(backupRoot, filepath.FromSlash(archiveKey))); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("archive object still exists: %v", err)
	}
}

func TestVersionReaderRejectsGlacier(t *testing.T) {
	reader := &VersionReader{}
	if _, err := reader.Open("glacier", "objects/1/x"); !errors.Is(err, ErrArchiveRestoreRequired) {
		t.Fatalf("Open() error = %v, want ErrArchiveRestoreRequired", err)
	}
}

func TestCloudMountReadsArchiveClassFromPrimary(t *testing.T) {
	root := t.TempDir()
	store, err := NewPOSIX(root, filepath.Join(root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	path, err := store.objectPath("objects/3/cloud")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("cloud-mounted"), 0o640); err != nil {
		t.Fatal(err)
	}
	reader := &VersionReader{primary: store, mode: "cloud_mount"}
	opened, err := reader.Open("archive", "objects/3/cloud")
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	payload, err := io.ReadAll(opened)
	if err != nil || string(payload) != "cloud-mounted" {
		t.Fatalf("payload = %q, error = %v", payload, err)
	}
}

func TestArchiveObjectKeyPreservesWorkspacePath(t *testing.T) {
	if got := ArchiveObjectKey("fileshare-archive/", "objects/42/value"); got != "fileshare-archive/objects/42/value" {
		t.Fatalf("ArchiveObjectKey() = %q", got)
	}
}
