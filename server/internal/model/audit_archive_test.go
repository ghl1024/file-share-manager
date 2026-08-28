/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package model

import (
	"strings"
	"testing"
	"time"
)

func TestAuditArchiveManifestHashAndValidation(t *testing.T) {
	manifest, err := (AuditArchiveManifest{
		Version: AuditArchiveFormatVersion, ArchiveID: "archive-1", StreamKey: "workspace:7",
		WorkspaceID: uintPointer(7), FromSeq: 1, ToSeq: 2, EventCount: 2,
		LastHash: strings.Repeat("a", 64), EventsSHA256: strings.Repeat("b", 64),
		CreatedAtMS: time.Now().UnixMilli(),
	}).WithHash()
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	manifest.EventCount++
	if err := manifest.Validate(); err == nil {
		t.Fatal("tampered manifest was accepted")
	}
}

func uintPointer(value uint) *uint { return &value }
