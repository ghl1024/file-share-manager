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
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestImportObjectValidatesAndPublishes(t *testing.T) {
	store, err := NewPOSIX(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	content := "restored file"
	digest := sha256.Sum256([]byte(content))
	result, err := store.ImportObject(9, strings.NewReader(content), int64(len(content)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenObject(result.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	_ = file.Close()
	if _, err := store.ImportObject(9, strings.NewReader("bad"), 3, hex.EncodeToString(digest[:])); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}
