/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditarchive

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
)

func TestArchiveCodecRoundTripAndTamperDetection(t *testing.T) {
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	workspaceID := uint(7)
	firstHash := strings.Repeat("a", 64)
	secondHash := strings.Repeat("b", 64)
	events := []model.OperationLog{
		{ID: 1, StreamKey: "workspace:7", StreamSeq: 1, WorkspaceID: &workspaceID, CurrentHash: &firstHash, Details: "{}", CreatedAt: time.Unix(100, 0)},
		{ID: 2, StreamKey: "workspace:7", StreamSeq: 2, WorkspaceID: &workspaceID, PrevHash: &firstHash, CurrentHash: &secondHash, Details: "{}", CreatedAt: time.Unix(101, 0)},
	}
	// Replace placeholder hashes with the same production chain calculation.
	events[0].CurrentHash = nil
	computedFirst := dao.CalculateAuditEventHash(&events[0], nil)
	events[0].CurrentHash = &computedFirst
	events[1].PrevHash = &computedFirst
	events[1].CurrentHash = nil
	computedSecond := dao.CalculateAuditEventHash(&events[1], &computedFirst)
	events[1].CurrentHash = &computedSecond
	digest, err := dao.AuditArchiveEventsDigest(events)
	if err != nil {
		t.Fatal(err)
	}
	archive := &model.AuditArchive{
		ID: "archive-1", StreamKey: "workspace:7", WorkspaceID: &workspaceID,
		FromSeq: 1, ToSeq: 2, EventCount: 2, LastHash: computedSecond,
		EventsSHA256: digest, CreatedAt: time.Unix(200, 0),
	}
	data, err := encodeArchive(archive, events, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := verifyArchive(data, archive, key); err != nil {
		t.Fatalf("verifyArchive() error = %v", err)
	}
	tampered := append([]byte(nil), data...)
	tampered[len(tampered)-1] ^= 0xff
	if err := verifyArchive(tampered, archive, key); err == nil {
		t.Fatal("tampered archive was accepted")
	}
}
