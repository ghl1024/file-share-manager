/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package backup

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/storage"

	"gorm.io/gorm"
)

func TestDrillObjectsDeduplicatesVersionContent(t *testing.T) {
	object := ObjectEntry{VersionID: 7, SHA256: "abc", BackupKey: "a"}
	objects := drillObjects([]Manifest{{Objects: []ObjectEntry{object}}, {Objects: []ObjectEntry{object, {VersionID: 8, SHA256: "def", BackupKey: "b"}}}})
	if len(objects) != 2 || objects[0].VersionID != 7 || objects[1].VersionID != 8 {
		t.Fatalf("drillObjects() = %+v", objects)
	}
}

func TestWriteDrillObjectValidatesSizeAndHash(t *testing.T) {
	content := []byte("restore rehearsal")
	hash := sha256.Sum256(content)
	path := t.TempDir() + "/object"
	written, err := writeDrillObject(path, bytes.NewReader(content), int64(len(content)), hex.EncodeToString(hash[:]))
	if err != nil || written != int64(len(content)) {
		t.Fatalf("writeDrillObject() = %d, %v", written, err)
	}
	if _, err := writeDrillObject(path+"-bad", bytes.NewReader(content), int64(len(content)), strings.Repeat("0", 64)); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestRetryBackupKind(t *testing.T) {
	for _, kind := range []string{"baseline", "incremental"} {
		got, err := retryBackupKind(&model.BackupJob{Kind: kind, Status: "failed"})
		if err != nil || got != kind {
			t.Fatalf("retryBackupKind(%s) = %q, %v", kind, got, err)
		}
	}
	if _, err := retryBackupKind(nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("nil job error = %v", err)
	}
	for _, job := range []*model.BackupJob{
		{Kind: "baseline", Status: "complete"},
		{Kind: "unknown", Status: "failed"},
	} {
		if _, err := retryBackupKind(job); !errors.Is(err, ErrBackupNotRetryable) {
			t.Fatalf("job %+v error = %v", job, err)
		}
	}
}

func TestFullManifestHashCoversObjects(t *testing.T) {
	manifest := Manifest{Objects: []ObjectEntry{{VersionID: 1, NodeID: 2, StorageKey: "objects/a", BackupKey: "backup/a", Size: 5, SHA256: "abc"}}}
	data, _, err := encodeFullManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeFullManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	decoded.Objects[0].BackupKey = "backup/tampered"
	stale, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFullManifest(stale); err == nil {
		t.Fatal("expected object changes with stale hash to fail")
	}
}

func TestFullManifestHashCoversChanges(t *testing.T) {
	manifest := Manifest{Changes: []model.ChangeLog{{Seq: 11, WorkspaceID: 7, EntityType: "node", EntityID: "3", Operation: "rename", Payload: `{"name":"report.pdf"}`}}}
	data, _, err := encodeFullManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeFullManifest(data)
	if err != nil {
		t.Fatal(err)
	}
	decoded.Changes[0].Payload = `{"name":"tampered.pdf"}`
	stale, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFullManifest(stale); err == nil {
		t.Fatal("expected change log mutation with stale hash to fail")
	}
}

func TestFullManifestHashSupportsOlderJSONShape(t *testing.T) {
	manifest := Manifest{Objects: []ObjectEntry{{VersionID: 1, NodeID: 2, Size: 5, SHA256: "abc", Encrypted: false}}}
	data, _, err := encodeFullManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	objects := document["objects"].([]any)
	delete(objects[0].(map[string]any), "encrypted")
	document["manifest_hash"] = ""
	legacyWithoutHash, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	hash, err := fullManifestHash(legacyWithoutHash)
	if err != nil {
		t.Fatal(err)
	}
	document["manifest_hash"] = hash
	legacy, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeFullManifest(legacy)
	if err != nil {
		t.Fatalf("older manifest shape rejected: %v", err)
	}
	if len(decoded.Objects) != 1 || decoded.Objects[0].Encrypted {
		t.Fatalf("decoded older manifest = %+v", decoded)
	}
}

func TestMetadataHashSupportsOlderJSONShape(t *testing.T) {
	versionID := uint(1)
	snapshot := &MetadataSnapshot{
		SchemaVersion: metadataSchemaVersion,
		CapturedAt:    time.Unix(1, 0).UTC(),
		Workspace:     model.Workspace{ID: 7},
		Nodes:         []model.Node{{ID: 2, WorkspaceID: 7, Type: "file", ActiveVersion: &versionID}},
		Versions:      []VersionSnapshot{{ID: 1, NodeID: 2, StorageKey: "objects/a", ScanRetryCount: 0}},
	}
	hash, err := metadataHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MetadataHash = hash
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	versions := document["versions"].([]any)
	delete(versions[0].(map[string]any), "scan_retry_count")
	document["metadata_hash"] = ""
	legacyWithoutHash, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(legacyWithoutHash)
	document["metadata_hash"] = hex.EncodeToString(digest[:])
	legacy, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded MetadataSnapshot
	if err := json.Unmarshal(legacy, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := verifyDecodedMetadataHash(legacy, &decoded); err != nil {
		t.Fatalf("older metadata shape rejected: %v", err)
	}
	if err := validateMetadata(&decoded, 7); err != nil {
		t.Fatalf("verified older metadata rejected: %v", err)
	}
	decoded.Workspace.Name = "tampered"
	if err := validateMetadata(&decoded, 7); err == nil {
		t.Fatal("metadata mutation after raw verification was accepted")
	}
}

func TestProtectedManifestRoundTripAndLegacyCompatibility(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x2a}, 32))
	manifest := Manifest{Objects: []ObjectEntry{{VersionID: 7, NodeID: 3, Name: "report.txt", Size: 128, SHA256: strings.Repeat("a", 64)}}}
	first, hash, err := encodeProtectedManifest(manifest, key)
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := encodeProtectedManifest(manifest, key)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) {
		t.Fatal("encrypted manifests must use unique nonces")
	}
	if bytes.Contains(first, []byte("report.txt")) || bytes.HasPrefix(first, []byte("{")) {
		t.Fatal("encrypted manifest leaks plaintext metadata")
	}
	decoded, err := decodeProtectedManifest(first, key)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ManifestHash != hash || len(decoded.Objects) != 1 || decoded.Objects[0].Name != "report.txt" {
		t.Fatalf("decoded protected manifest = %+v", decoded)
	}

	legacy, _, err := encodeFullManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeProtectedManifest(legacy, ""); err != nil {
		t.Fatalf("legacy plaintext manifest rejected: %v", err)
	}
}

func TestProtectedManifestRejectsTamperingAndWrongKey(t *testing.T) {
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x11}, 32))
	wrongKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x22}, 32))
	data, _, err := encodeProtectedManifest(Manifest{BackupManifest: storage.BackupManifest{ID: "backup-1"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)-1] ^= 1
	if _, err := decodeProtectedManifest(data, key); err == nil {
		t.Fatal("tampered encrypted manifest was accepted")
	}
	clean, _, err := encodeProtectedManifest(Manifest{BackupManifest: storage.BackupManifest{ID: "backup-1"}}, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodeProtectedManifest(clean, wrongKey); err == nil {
		t.Fatal("manifest encrypted with another key was accepted")
	}
	if _, _, err := encodeProtectedManifest(Manifest{}, ""); !errors.Is(err, ErrManifestEncryptionKeyMissing) {
		t.Fatalf("missing key error = %v", err)
	}
}

func TestValidateChangeRange(t *testing.T) {
	valid := []model.ChangeLog{{Seq: 12, WorkspaceID: 7}, {Seq: 15, WorkspaceID: 7}}
	if err := validateChangeRange(7, 10, 15, valid); err != nil {
		t.Fatalf("valid range rejected: %v", err)
	}
	tests := []struct {
		name    string
		start   uint64
		end     uint64
		changes []model.ChangeLog
	}{
		{name: "reversed range", start: 15, end: 10},
		{name: "wrong workspace", start: 10, end: 15, changes: []model.ChangeLog{{Seq: 12, WorkspaceID: 8}}},
		{name: "before start", start: 10, end: 15, changes: []model.ChangeLog{{Seq: 10, WorkspaceID: 7}}},
		{name: "after end", start: 10, end: 15, changes: []model.ChangeLog{{Seq: 16, WorkspaceID: 7}}},
		{name: "not increasing", start: 10, end: 15, changes: []model.ChangeLog{{Seq: 13, WorkspaceID: 7}, {Seq: 12, WorkspaceID: 7}}},
		{name: "missing end", start: 10, end: 15, changes: []model.ChangeLog{{Seq: 13, WorkspaceID: 7}}},
		{name: "missing changes", start: 10, end: 15},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := validateChangeRange(7, test.start, test.end, test.changes); err == nil {
				t.Fatal("expected invalid change range to fail")
			}
		})
	}
}

func TestMetadataHashAndReferences(t *testing.T) {
	workspaceID := uint(7)
	folderID := uint(10)
	fileID := uint(11)
	versionID := uint(21)
	snapshot := &MetadataSnapshot{
		SchemaVersion: metadataSchemaVersion,
		CapturedAt:    time.Unix(100, 0).UTC(),
		Workspace:     model.Workspace{ID: workspaceID},
		Nodes: []model.Node{
			{ID: folderID, WorkspaceID: workspaceID, Type: "folder"},
			{ID: fileID, WorkspaceID: workspaceID, ParentID: &folderID, Type: "file", ActiveVersion: &versionID},
		},
		Versions: []VersionSnapshot{{ID: versionID, NodeID: fileID, VersionNo: 1, StorageKey: "objects/7/a", Size: 5, SHA256: strings.Repeat("a", 64)}},
	}
	hash, err := metadataHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MetadataHash = hash
	if err := validateMetadata(snapshot, workspaceID); err != nil {
		t.Fatalf("valid metadata rejected: %v", err)
	}

	snapshot.Versions[0].Size++
	if err := validateMetadata(snapshot, workspaceID); err == nil {
		t.Fatal("expected stale metadata hash to fail")
	}
}

func TestMetadataCommentsAndLegacySchema(t *testing.T) {
	workspaceID := uint(7)
	snapshot := &MetadataSnapshot{
		SchemaVersion: metadataSchemaVersion,
		CapturedAt:    time.Unix(100, 0).UTC(), Workspace: model.Workspace{ID: workspaceID},
		Nodes:           []model.Node{{ID: 10, WorkspaceID: workspaceID, Type: "folder"}},
		Comments:        []model.NodeComment{{ID: 20, WorkspaceID: workspaceID, NodeID: 10, AuthorID: 1, Content: "备份评论", Revision: 1}},
		CommentMentions: []model.NodeCommentMention{{CommentID: 20, UserID: 2}},
	}
	hash, err := metadataHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MetadataHash = hash
	if err := validateMetadata(snapshot, workspaceID); err != nil {
		t.Fatalf("valid comment metadata rejected: %v", err)
	}

	snapshot.CommentMentions[0].CommentID = 999
	snapshot.MetadataHash = ""
	hash, err = metadataHash(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.MetadataHash = hash
	if err := validateMetadata(snapshot, workspaceID); err == nil {
		t.Fatal("invalid comment mention reference accepted")
	}

	legacy := &MetadataSnapshot{SchemaVersion: minimumMetadataSchemaVersion, CapturedAt: time.Unix(100, 0).UTC(), Workspace: model.Workspace{ID: workspaceID}}
	hash, err = metadataHash(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacy.MetadataHash = hash
	if err := validateMetadata(legacy, workspaceID); err != nil {
		t.Fatalf("legacy metadata schema rejected: %v", err)
	}
}

func TestValidateMetadataObjectsRequiresEveryVersion(t *testing.T) {
	snapshot := &MetadataSnapshot{Versions: []VersionSnapshot{{ID: 9, SHA256: "abc"}}}
	if err := validateMetadataObjects([]Manifest{{Metadata: snapshot, Objects: []ObjectEntry{{VersionID: 9, SHA256: "abc"}}}}); err != nil {
		t.Fatalf("matching object rejected: %v", err)
	}
	if err := validateMetadataObjects([]Manifest{{Metadata: snapshot}}); err == nil {
		t.Fatal("expected missing file version object to fail")
	}
}

func TestIncrementalDepthWithLookup(t *testing.T) {
	baseline := &model.BackupJob{ID: "baseline", WorkspaceID: 7, Kind: "baseline", Status: "complete"}
	first := &model.BackupJob{ID: "incremental-1", WorkspaceID: 7, Kind: "incremental", Status: "complete", ParentID: baseline.ID}
	second := &model.BackupJob{ID: "incremental-2", WorkspaceID: 7, Kind: "incremental", Status: "complete", ParentID: first.ID}
	jobs := map[string]*model.BackupJob{baseline.ID: baseline, first.ID: first, second.ID: second}
	lookup := func(_ uint, id string) (*model.BackupJob, error) { return jobs[id], nil }

	depth, err := incrementalDepthWithLookup(second, lookup)
	if err != nil || depth != 2 {
		t.Fatalf("incrementalDepthWithLookup() = %d, %v", depth, err)
	}
	if depth, err := incrementalDepthWithLookup(baseline, lookup); err != nil || depth != 0 {
		t.Fatalf("baseline depth = %d, %v", depth, err)
	}

	missing := *second
	missing.ParentID = "missing"
	if _, err := incrementalDepthWithLookup(&missing, lookup); err == nil {
		t.Fatal("expected missing parent to fail")
	}
	cycle := *first
	cycle.ParentID = second.ID
	jobs[first.ID] = &cycle
	if _, err := incrementalDepthWithLookup(second, lookup); err == nil {
		t.Fatal("expected cyclic parent chain to fail")
	}
}

func TestCompactionCompletedFilterQuotesMySQLReservedColumn(t *testing.T) {
	if !strings.Contains(compactionCompletedFilter, "`trigger`") {
		t.Fatalf("compaction filter must quote MySQL reserved column: %s", compactionCompletedFilter)
	}
}

func TestCompactionObjectsUsesLatestMetadataVersions(t *testing.T) {
	old := ObjectEntry{VersionID: 7, SHA256: "aaa", BackupKey: "old-7"}
	latest := ObjectEntry{VersionID: 8, SHA256: "bbb", BackupKey: "latest-8"}
	manifests := []Manifest{
		{Objects: []ObjectEntry{old}},
		{Objects: []ObjectEntry{latest}, Metadata: &MetadataSnapshot{Versions: []VersionSnapshot{{ID: 8, SHA256: "bbb"}}}},
	}
	objects, err := compactionObjects(manifests)
	if err != nil || len(objects) != 1 || objects[0].BackupKey != latest.BackupKey {
		t.Fatalf("compactionObjects() = %+v, %v", objects, err)
	}
	manifests[1].Metadata.Versions = append(manifests[1].Metadata.Versions, VersionSnapshot{ID: 9, SHA256: "missing"})
	if _, err := compactionObjects(manifests); err == nil {
		t.Fatal("expected metadata version without an object to fail")
	}
}
