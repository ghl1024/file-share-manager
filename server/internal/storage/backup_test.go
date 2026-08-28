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
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
)

func TestLocalBackupStorageImmutableAndManifest(t *testing.T) {
	store, err := NewLocalBackupStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Put("baseline/one.txt", strings.NewReader("one")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Put("baseline/one.txt", strings.NewReader("two")); err != ErrBackupImmutable {
		t.Fatalf("second Put error = %v, want immutable error", err)
	}
	manifest := BackupManifest{ID: "b1", Kind: "baseline", Status: "complete", WorkspaceID: 1, ObjectCount: 1}
	data, _, err := EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeManifest(data); err != nil {
		t.Fatalf("DecodeManifest() error = %v", err)
	}
	data[len(data)-2] ^= 1
	if _, err := DecodeManifest(data); err == nil {
		t.Fatal("expected tampered manifest to fail verification")
	}
}

func TestLocalBackupStorageConcurrentPutIsImmutable(t *testing.T) {
	store, err := NewLocalBackupStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const workers = 12
	start := make(chan struct{})
	results := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		wait.Add(1)
		go func(value string) {
			defer wait.Done()
			<-start
			_, _, putErr := store.Put("baseline/concurrent.txt", strings.NewReader(value))
			results <- putErr
		}(fmt.Sprintf("worker-%d", index))
	}
	close(start)
	wait.Wait()
	close(results)

	succeeded := 0
	immutable := 0
	for result := range results {
		switch result {
		case nil:
			succeeded++
		case ErrBackupImmutable:
			immutable++
		default:
			t.Fatalf("Put error = %v", result)
		}
	}
	if succeeded != 1 || immutable != workers-1 {
		t.Fatalf("success = %d, immutable = %d", succeeded, immutable)
	}
	reader, err := store.Get("baseline/concurrent.txt")
	if err != nil {
		t.Fatal(err)
	}
	payload, err := io.ReadAll(reader)
	closeErr := reader.Close()
	if err != nil || closeErr != nil || !strings.HasPrefix(string(payload), "worker-") {
		t.Fatalf("stored payload = %q, read error = %v, close error = %v", payload, err, closeErr)
	}
}
