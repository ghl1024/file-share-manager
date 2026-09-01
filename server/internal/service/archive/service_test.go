/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package archive

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/storage"
)

type archiveDAOStub struct {
	candidates  []dao.ArchiveCandidate
	completed   []dao.ArchiveCandidate
	failures    []string
	completeErr error
}

func (s *archiveDAOStub) ListCandidates(time.Time, int) ([]dao.ArchiveCandidate, error) {
	return s.candidates, nil
}

func (s *archiveDAOStub) Complete(candidate dao.ArchiveCandidate, _ time.Time) error {
	if s.completeErr != nil {
		return s.completeErr
	}
	s.completed = append(s.completed, candidate)
	return nil
}

func (s *archiveDAOStub) RecordFailure(_ dao.ArchiveCandidate, message string) error {
	s.failures = append(s.failures, message)
	return nil
}

func TestRunOnceMovesVerifiedObject(t *testing.T) {
	root := t.TempDir()
	primary, err := storage.NewPOSIX(root, filepath.Join(root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	payload := "cold-object"
	key := "objects/9/cold"
	writePrimaryObject(t, primary, root, key, payload)
	digest := sha256.Sum256([]byte(payload))
	candidate := dao.ArchiveCandidate{WorkspaceID: 9, StorageKey: key, Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
	daoStub := &archiveDAOStub{candidates: []dao.ArchiveCandidate{candidate}}
	archiveStore, err := storage.NewLocalBackupStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{dao: daoStub, primary: primary, archive: archiveStore, prefix: "fileshare-archive/"}
	completed, err := service.RunOnce(context.Background(), time.Now(), 10)
	if err != nil || completed != 1 || len(daoStub.completed) != 1 || len(daoStub.failures) != 0 {
		t.Fatalf("completed=%d committed=%d failures=%v error=%v", completed, len(daoStub.completed), daoStub.failures, err)
	}
	if _, err := primary.OpenObject(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("primary object still exists: %v", err)
	}
	reader, err := archiveStore.Get(storage.ArchiveObjectKey("fileshare-archive/", key))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
}

func TestRunOnceLeavesPrimaryOnMetadataFailure(t *testing.T) {
	root := t.TempDir()
	primary, err := storage.NewPOSIX(root, filepath.Join(root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	payload := "cold-object"
	key := "objects/9/failure"
	writePrimaryObject(t, primary, root, key, payload)
	digest := sha256.Sum256([]byte(payload))
	candidate := dao.ArchiveCandidate{WorkspaceID: 9, StorageKey: key, Size: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
	daoStub := &archiveDAOStub{candidates: []dao.ArchiveCandidate{candidate}, completeErr: errors.New("database unavailable")}
	archiveStore, err := storage.NewLocalBackupStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := &Service{dao: daoStub, primary: primary, archive: archiveStore, prefix: "fileshare-archive/"}
	completed, err := service.RunOnce(context.Background(), time.Now(), 10)
	if err != nil || completed != 0 || len(daoStub.failures) != 1 {
		t.Fatalf("completed=%d failures=%v error=%v", completed, daoStub.failures, err)
	}
	file, err := primary.OpenObject(key)
	if err != nil {
		t.Fatalf("primary object was removed: %v", err)
	}
	_ = file.Close()
}

func writePrimaryObject(t *testing.T, primary *storage.POSIX, root, key, payload string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(payload), 0o640); err != nil {
		t.Fatal(err)
	}
	file, err := primary.OpenObject(key)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if strings.TrimSpace(payload) == "" {
		t.Fatal("payload must not be empty")
	}
}
