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
	"net/http"
	"strings"
	"testing"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/model"
)

func TestPreviewPolicyAllowsOnlyExplicitSafeFormats(t *testing.T) {
	cfg := &config.Config{Preview: config.PreviewConfig{MaxBinaryBytes: 8 << 20, MaxTextBytes: 512 << 10}}
	tests := []struct {
		name      string
		extension string
		mimeType  string
		kind      string
		servedAs  string
	}{
		{name: "png", extension: ".png", mimeType: "image/png", kind: "image", servedAs: "image/png"},
		{name: "jpeg", extension: ".jpeg", mimeType: "image/jpeg", kind: "image", servedAs: "image/jpeg"},
		{name: "pdf", extension: ".pdf", mimeType: "application/pdf", kind: "pdf", servedAs: "application/pdf"},
		{name: "text with charset", extension: ".txt", mimeType: "text/plain; charset=utf-8", kind: "text", servedAs: "text/plain; charset=utf-8"},
		{name: "json", extension: ".json", mimeType: "application/json", kind: "text", servedAs: "text/plain; charset=utf-8"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			version := &model.FileVersion{VersionNo: 2, Size: 1024, Extension: test.extension, DetectedMime: test.mimeType, ScanStatus: "clean"}
			descriptor, restriction := previewPolicy(version, cfg)
			if restriction != nil {
				t.Fatalf("previewPolicy() restriction = %#v", restriction)
			}
			if descriptor.Kind != test.kind || descriptor.MIMEType != test.servedAs || descriptor.VersionNo != 2 {
				t.Fatalf("previewPolicy() descriptor = %#v", descriptor)
			}
		})
	}
}

func TestPreviewPolicyRejectsUnsupportedMismatchedAndOversizedContent(t *testing.T) {
	cfg := &config.Config{Preview: config.PreviewConfig{MaxBinaryBytes: 2 << 20, MaxTextBytes: 16 << 10}}
	tests := []struct {
		name    string
		version model.FileVersion
		status  int
		reason  string
	}{
		{name: "svg is never rendered", version: model.FileVersion{Extension: ".svg", DetectedMime: "image/svg+xml", Size: 100}, status: http.StatusUnsupportedMediaType, reason: model.AuditReasonUnsupportedMediaType},
		{name: "html is never rendered", version: model.FileVersion{Extension: ".html", DetectedMime: "text/html", Size: 100}, status: http.StatusUnsupportedMediaType, reason: model.AuditReasonUnsupportedMediaType},
		{name: "mismatched pdf", version: model.FileVersion{Extension: ".pdf", DetectedMime: "text/plain", Size: 100}, status: http.StatusUnsupportedMediaType, reason: model.AuditReasonUnsupportedMediaType},
		{name: "oversized image", version: model.FileVersion{Extension: ".png", DetectedMime: "image/png", Size: 2<<20 + 1}, status: http.StatusRequestEntityTooLarge, reason: model.AuditReasonPreviewTooLarge},
		{name: "oversized text", version: model.FileVersion{Extension: ".txt", DetectedMime: "text/plain", Size: 16<<10 + 1}, status: http.StatusRequestEntityTooLarge, reason: model.AuditReasonPreviewTooLarge},
		{name: "unsafe scan", version: model.FileVersion{Extension: ".png", DetectedMime: "image/png", Size: 100, ScanStatus: "infected"}, status: http.StatusForbidden, reason: model.AuditReasonUnsafeScanStatus},
		{name: "deep archive", version: model.FileVersion{Extension: ".png", DetectedMime: "image/png", Size: 100, ScanStatus: "clean", StorageClass: "glacier"}, status: http.StatusConflict, reason: model.AuditReasonArchiveRestoreRequired},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, restriction := previewPolicy(&test.version, cfg)
			if restriction == nil || restriction.status != test.status || restriction.reason != test.reason {
				t.Fatalf("previewPolicy() restriction = %#v, want status=%d reason=%s", restriction, test.status, test.reason)
			}
		})
	}
}

func TestValidPreviewText(t *testing.T) {
	if !validPreviewText([]byte("中文 UTF-8\nsecond line")) {
		t.Fatal("expected valid UTF-8 text")
	}
	if validPreviewText([]byte{'a', 0, 'b'}) {
		t.Fatal("expected NUL-containing content to be rejected")
	}
	if validPreviewText([]byte{0xff, 0xfe}) {
		t.Fatal("expected invalid UTF-8 content to be rejected")
	}
	large := strings.Repeat("a", 1024)
	if !validPreviewText([]byte(large)) {
		t.Fatal("expected ordinary text to remain valid")
	}
}

func TestValidPreviewContentChecksMagicNumbers(t *testing.T) {
	if !validPreviewContent(".pdf", "pdf", []byte("%PDF-1.7\n")) {
		t.Fatal("expected PDF signature to be accepted")
	}
	if validPreviewContent(".pdf", "pdf", []byte("<html>not a pdf</html>")) {
		t.Fatal("expected spoofed PDF content to be rejected")
	}
	if !validPreviewContent(".png", "image", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		t.Fatal("expected PNG signature to be accepted")
	}
	if validPreviewContent(".png", "image", []byte("GIF89a")) {
		t.Fatal("expected mismatched image signature to be rejected")
	}
}
