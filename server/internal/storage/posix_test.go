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
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPOSIXWriteAndMerge(t *testing.T) {
	store, err := NewPOSIX(t.TempDir()+"/objects", t.TempDir()+"/staging")
	if err != nil {
		t.Fatal(err)
	}
	parts := []string{"hello ", "file share"}
	for index, part := range parts {
		if _, err := store.WritePart("upload-1", index, strings.NewReader(part), int64(len(part))); err != nil {
			t.Fatal(err)
		}
	}
	content := strings.Join(parts, "")
	digest := sha256.Sum256([]byte(content))
	result, err := store.Merge("upload-1", 7, len(parts), int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenObject(result.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(file)
	_ = file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("merged content = %q, want %q", got, content)
	}
}

func TestPOSIXResumesInterruptedMultipartUpload(t *testing.T) {
	root := t.TempDir()
	objectRoot := filepath.Join(root, "objects")
	stagingRoot := filepath.Join(root, "staging")
	store, err := NewPOSIX(objectRoot, stagingRoot)
	if err != nil {
		t.Fatal(err)
	}

	const (
		uploadID  = "upload-resume-large"
		partSize  = int64(4 << 20)
		partCount = 5
	)
	partPayload := func(partNo int) []byte {
		return bytes.Repeat([]byte{byte('A' + partNo)}, int(partSize))
	}
	for _, partNo := range []int{0, 3} {
		payload := partPayload(partNo)
		if _, err := store.WritePart(uploadID, partNo, bytes.NewReader(payload), partSize); err != nil {
			t.Fatalf("write initial part %d: %v", partNo, err)
		}
	}

	// Recreating the store simulates an API process restart or a client
	// reconnecting after the network was interrupted.
	store, err = NewPOSIX(objectRoot, stagingRoot)
	if err != nil {
		t.Fatal(err)
	}
	for partNo := 0; partNo < partCount; partNo++ {
		exists, size, err := store.PartExists(uploadID, partNo)
		if err != nil {
			t.Fatal(err)
		}
		wantExists := partNo == 0 || partNo == 3
		if exists != wantExists || (exists && size != partSize) {
			t.Fatalf("part %d state = exists %v size %d, want exists %v size %d", partNo, exists, size, wantExists, partSize)
		}
		if !exists {
			payload := partPayload(partNo)
			if _, err := store.WritePart(uploadID, partNo, bytes.NewReader(payload), partSize); err != nil {
				t.Fatalf("resume part %d: %v", partNo, err)
			}
		}
	}

	// Retrying an already persisted part is idempotent and must not replace it.
	if _, err := store.WritePart(uploadID, 3, bytes.NewReader(partPayload(3)), partSize); err != nil {
		t.Fatalf("retry existing part: %v", err)
	}
	hash := sha256.New()
	for partNo := 0; partNo < partCount; partNo++ {
		_, _ = hash.Write(partPayload(partNo))
	}
	wantDigest := hex.EncodeToString(hash.Sum(nil))
	wantSize := partSize * partCount
	merged, err := store.Merge(uploadID, 7, partCount, wantSize, wantDigest)
	if err != nil {
		t.Fatalf("merge resumed upload: %v", err)
	}
	if merged.Size != wantSize || merged.SHA256 != wantDigest {
		t.Fatalf("merged result = size %d sha256 %s, want size %d sha256 %s", merged.Size, merged.SHA256, wantSize, wantDigest)
	}
}

func TestPOSIXMergeFailureCanRecoverWithoutRestartingUpload(t *testing.T) {
	root := t.TempDir()
	store, err := NewPOSIX(filepath.Join(root, "objects"), filepath.Join(root, "staging"))
	if err != nil {
		t.Fatal(err)
	}
	const uploadID = "upload-merge-recovery"
	parts := [][]byte{[]byte("first-part"), []byte("second-part"), []byte("third-part")}
	for partNo, payload := range parts {
		if _, err := store.WritePart(uploadID, partNo, bytes.NewReader(payload), int64(len(payload))); err != nil {
			t.Fatal(err)
		}
	}
	content := bytes.Join(parts, nil)
	digest := sha256.Sum256(content)
	wantDigest := hex.EncodeToString(digest[:])

	// Simulate a staging-volume interruption after the database recorded all
	// parts. Merge must fail cleanly while preserving the remaining parts.
	if err := os.Remove(store.partPath(uploadID, 1)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(uploadID, 9, len(parts), int64(len(content)), wantDigest); err == nil || !strings.Contains(err.Error(), "open part 1") {
		t.Fatalf("merge with missing part error = %v", err)
	}
	for _, partNo := range []int{0, 2} {
		if exists, _, err := store.PartExists(uploadID, partNo); err != nil || !exists {
			t.Fatalf("part %d was not preserved after failed merge: exists=%v err=%v", partNo, exists, err)
		}
	}
	if _, err := store.WritePart(uploadID, 1, bytes.NewReader(parts[1]), int64(len(parts[1]))); err != nil {
		t.Fatalf("restore missing part: %v", err)
	}

	// A same-size corrupted part is caught by the whole-file digest. Removing
	// and retransmitting only that part allows the session to complete.
	if err := os.WriteFile(store.partPath(uploadID, 2), bytes.Repeat([]byte{'X'}, len(parts[2])), 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Merge(uploadID, 9, len(parts), int64(len(content)), wantDigest); err == nil || !strings.Contains(err.Error(), "validation failed") {
		t.Fatalf("merge with corrupt part error = %v", err)
	}
	if err := os.Remove(store.partPath(uploadID, 2)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.WritePart(uploadID, 2, bytes.NewReader(parts[2]), int64(len(parts[2]))); err != nil {
		t.Fatalf("restore corrupt part: %v", err)
	}
	merged, err := store.Merge(uploadID, 9, len(parts), int64(len(content)), wantDigest)
	if err != nil {
		t.Fatalf("merge after recovery: %v", err)
	}
	if merged.Size != int64(len(content)) || merged.SHA256 != wantDigest {
		t.Fatalf("recovered result = %#v", merged)
	}

	err = filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.Contains(info.Name(), ".tmp-") {
			t.Fatalf("merge left temporary object %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPOSIXRejectsPathTraversal(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenObject("../outside"); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("OpenObject() error = %v, want ErrInvalidObjectKey", err)
	}
}

func TestPOSIXAcceptsGeneratedUploadID(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	uploadID := "upload-0x123456-7890-abcd-ef01-234567890abc"
	if err := store.EnsureUpload(uploadID); err != nil {
		t.Fatalf("EnsureUpload(%q) error = %v", uploadID, err)
	}
}

func TestPOSIXRejectsInvalidUploadID(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, uploadID := range []string{"", ".", "..", "../outside", `folder\\upload`, "bad\x00id"} {
		if err := store.EnsureUpload(uploadID); !errors.Is(err, ErrInvalidObjectKey) {
			t.Errorf("EnsureUpload(%q) error = %v, want ErrInvalidObjectKey", uploadID, err)
		}
	}
}

func TestPOSIXBatchArchiveLifecycle(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	file, tmpPath, err := store.CreateBatchArchive("6ecfa3c3-965b-46fd-b6aa-90308a25a8a9")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("zip-content"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	size, err := store.CommitBatchArchive("6ecfa3c3-965b-46fd-b6aa-90308a25a8a9", tmpPath)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len("zip-content")) {
		t.Fatalf("archive size = %d", size)
	}
	archive, err := store.OpenBatchArchive("6ecfa3c3-965b-46fd-b6aa-90308a25a8a9")
	if err != nil {
		t.Fatal(err)
	}
	content, err := io.ReadAll(archive)
	_ = archive.Close()
	if err != nil || string(content) != "zip-content" {
		t.Fatalf("archive content = %q, error = %v", content, err)
	}
	if err := store.RemoveBatchArchive("6ecfa3c3-965b-46fd-b6aa-90308a25a8a9"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.OpenBatchArchive("../outside"); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("invalid batch archive ID error = %v", err)
	}
}

func TestPOSIXQuarantineObjectMovesWorkspaceObject(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	store, err := NewPOSIX(root, staging)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(root, "objects", "7", "orphan-key")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("orphan"), 0o640); err != nil {
		t.Fatal(err)
	}

	quarantineKey, err := store.QuarantineObject(7, "objects/7/orphan-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sourcePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source path stat error = %v, want not exists", err)
	}
	content, err := os.ReadFile(filepath.Join(staging, filepath.FromSlash(quarantineKey)))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "orphan" {
		t.Fatalf("quarantine content = %q", content)
	}
}

func TestPOSIXQuarantineRejectsCrossWorkspaceObject(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.QuarantineObject(7, "objects/8/other"); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("QuarantineObject() error = %v, want ErrInvalidObjectKey", err)
	}
}

func TestPOSIXQuarantineRestoreAndPurge(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	store, err := NewPOSIX(root, staging)
	if err != nil {
		t.Fatal(err)
	}
	storageKey := "objects/7/restorable"
	sourcePath := filepath.Join(root, filepath.FromSlash(storageKey))
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}
	quarantineKey, err := store.QuarantineObject(7, storageKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreQuarantinedObject(8, quarantineKey, storageKey); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("cross-workspace restore error = %v, want ErrInvalidObjectKey", err)
	}
	if err := store.RestoreQuarantinedObject(7, quarantineKey, storageKey); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(sourcePath)
	if err != nil || string(content) != "original" {
		t.Fatalf("restored content = %q, error = %v", content, err)
	}

	quarantineKey, err = store.QuarantineObject(7, storageKey)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourcePath, []byte("replacement"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := store.RestoreQuarantinedObject(7, quarantineKey, storageKey); !errors.Is(err, os.ErrExist) {
		t.Fatalf("overwrite protection error = %v, want os.ErrExist", err)
	}
	if err := store.RemoveQuarantinedObject(8, quarantineKey); !errors.Is(err, ErrInvalidObjectKey) {
		t.Fatalf("cross-workspace purge error = %v, want ErrInvalidObjectKey", err)
	}
	if err := store.RemoveQuarantinedObject(7, quarantineKey); err != nil {
		t.Fatal(err)
	}
	if err := store.RemoveQuarantinedObject(7, quarantineKey); err != nil {
		t.Fatalf("idempotent purge error = %v", err)
	}
}
