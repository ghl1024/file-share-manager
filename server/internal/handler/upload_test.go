/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package handler

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"file-share-manager/server/internal/storage"
)

func TestInspectObjectRejectsZipTraversal(t *testing.T) {
	sample := zipSample(t, zipSampleEntry{name: "../escape.txt", content: []byte("unsafe")})
	if _, _, err := inspectSample(t, "archive.zip", sample); err == nil || !strings.Contains(err.Error(), "不安全路径") {
		t.Fatalf("zip traversal error = %v", err)
	}
}

func TestInspectObjectRejectsZipBombCompressionRatio(t *testing.T) {
	sample := zipSample(t, zipSampleEntry{name: "zeros.bin", content: make([]byte, 16<<20)})
	if _, _, err := inspectSample(t, "compression-bomb.zip", sample); err == nil || !strings.Contains(err.Error(), "压缩比") {
		t.Fatalf("zip bomb error = %v", err)
	}
}

func TestInspectObjectRejectsExcessiveZIPNesting(t *testing.T) {
	sample := zipSample(t, zipSampleEntry{name: "payload.txt", content: []byte("safe")})
	for level := 1; level < maxZIPNestingDepth; level++ {
		sample = zipSample(t, zipSampleEntry{name: "nested.zip", content: sample})
	}
	if _, _, err := inspectSample(t, "nested-within-limit.zip", sample); err != nil {
		t.Fatalf("ZIP at maximum nesting depth rejected: %v", err)
	}
	sample = zipSample(t, zipSampleEntry{name: "nested.zip", content: sample})
	if _, _, err := inspectSample(t, "nested.zip", sample); err == nil || !strings.Contains(err.Error(), "嵌套层数") {
		t.Fatalf("nested zip error = %v", err)
	}
}

func TestInspectObjectMarksEncryptedZIPAsHighRisk(t *testing.T) {
	sample := zipSample(t, zipSampleEntry{name: "secret.txt", content: []byte("not actually encrypted"), encrypted: true})
	_, encrypted, err := inspectSample(t, "encrypted.zip", sample)
	if err != nil {
		t.Fatalf("encrypted zip rejected: %v", err)
	}
	if !encrypted || riskLevel(encrypted) != "high" {
		t.Fatalf("encrypted = %v, risk = %s", encrypted, riskLevel(encrypted))
	}
}

func TestInspectObjectRejectsMacroDisguisedAsDOCX(t *testing.T) {
	sample := zipSample(t,
		zipSampleEntry{name: "[Content_Types].xml", content: []byte("<Types/>")},
		zipSampleEntry{name: "word/document.xml", content: []byte("<document/>")},
		zipSampleEntry{name: "word/vbaProject.bin", content: []byte("synthetic-vba-project")},
	)
	if _, _, err := inspectSample(t, "macro-disguised.docx", sample); err == nil || !strings.Contains(err.Error(), "VBA 宏") {
		t.Fatalf("macro-disguised DOCX error = %v", err)
	}
	if _, _, err := inspectSample(t, "macro-enabled.docm", sample); err != nil {
		t.Fatalf("valid DOCM sample rejected: %v", err)
	}
}

type zipSampleEntry struct {
	name      string
	content   []byte
	encrypted bool
}

func zipSample(t *testing.T, entries ...zipSampleEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, sample := range entries {
		header := &zip.FileHeader{Name: sample.name, Method: zip.Deflate}
		if sample.encrypted {
			header.Flags |= 0x1
		}
		entry, err := archive.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(sample.content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func inspectSample(t *testing.T, displayName string, sample []byte) (string, bool, error) {
	t.Helper()

	root := t.TempDir()
	store, err := storage.NewPOSIX(root+"/objects", root+"/staging")
	if err != nil {
		t.Fatal(err)
	}
	uploadID := "upload-security-test"
	if err := store.EnsureUpload(uploadID); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(sample)
	if _, err := store.WritePart(uploadID, 0, bytes.NewReader(sample), int64(len(sample))); err != nil {
		t.Fatal(err)
	}
	merged, err := store.Merge(uploadID, 1, 1, int64(len(sample)), hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.RemoveObject(merged.StorageKey) })
	return inspectObject(store, merged.StorageKey, displayName, merged.Size)
}
