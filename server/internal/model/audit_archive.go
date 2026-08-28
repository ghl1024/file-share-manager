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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"
)

const AuditArchiveFormatVersion = 1

// AuditArchive is the durable receipt for one immutable audit-stream prefix.
// Hot rows are removed only after the referenced object has been read back,
// decrypted and verified against this receipt.
type AuditArchive struct {
	ID            string     `gorm:"type:char(36);primaryKey" json:"id"`
	StreamKey     string     `gorm:"type:varchar(64);not null;index;uniqueIndex:uidx_audit_archive_start,priority:1" json:"stream_key"`
	WorkspaceID   *uint      `gorm:"index" json:"workspace_id,omitempty"`
	Status        string     `gorm:"type:varchar(16);not null;index" json:"status"`
	FromSeq       uint64     `gorm:"not null;uniqueIndex:uidx_audit_archive_start,priority:2" json:"from_seq"`
	ToSeq         uint64     `gorm:"not null" json:"to_seq"`
	EventCount    int        `gorm:"not null" json:"event_count"`
	FirstPrevHash string     `gorm:"type:char(64)" json:"first_prev_hash,omitempty"`
	LastHash      string     `gorm:"type:char(64);not null" json:"last_hash"`
	EventsSHA256  string     `gorm:"type:char(64);not null" json:"events_sha256"`
	ManifestHash  string     `gorm:"type:char(64);not null" json:"manifest_hash"`
	ObjectKey     string     `gorm:"type:varchar(512);not null" json:"object_key"`
	ObjectSize    int64      `gorm:"not null;default:0" json:"object_size"`
	ObjectSHA256  string     `gorm:"type:char(64)" json:"object_sha256"`
	FailureCount  int        `gorm:"not null;default:0" json:"failure_count"`
	ErrorMessage  string     `gorm:"type:varchar(1000)" json:"error_message,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	VerifiedAt    *time.Time `json:"verified_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (AuditArchive) TableName() string { return "audit_archives" }

// AuditArchiveManifest is stored as the first JSONL record in the encrypted
// archive. ManifestHash authenticates the metadata independently of the
// encrypted object's checksum.
type AuditArchiveManifest struct {
	Version       int    `json:"version"`
	ArchiveID     string `json:"archive_id"`
	StreamKey     string `json:"stream_key"`
	WorkspaceID   *uint  `json:"workspace_id,omitempty"`
	FromSeq       uint64 `json:"from_seq"`
	ToSeq         uint64 `json:"to_seq"`
	EventCount    int    `json:"event_count"`
	FirstPrevHash string `json:"first_prev_hash,omitempty"`
	LastHash      string `json:"last_hash"`
	EventsSHA256  string `json:"events_sha256"`
	CreatedAtMS   int64  `json:"created_at_unix_ms"`
	ManifestHash  string `json:"manifest_hash"`
}

func (manifest AuditArchiveManifest) WithHash() (AuditArchiveManifest, error) {
	manifest.ManifestHash = ""
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return manifest, err
	}
	digest := sha256.Sum256(encoded)
	manifest.ManifestHash = hex.EncodeToString(digest[:])
	return manifest, nil
}

func (manifest AuditArchiveManifest) Validate() error {
	provided := manifest.ManifestHash
	expected, err := manifest.WithHash()
	if err != nil {
		return err
	}
	if manifest.Version != AuditArchiveFormatVersion || provided == "" || provided != expected.ManifestHash {
		return errors.New("audit archive manifest hash mismatch")
	}
	if manifest.ArchiveID == "" || manifest.StreamKey == "" || manifest.FromSeq == 0 || manifest.ToSeq < manifest.FromSeq || manifest.EventCount != int(manifest.ToSeq-manifest.FromSeq+1) || len(manifest.LastHash) != 64 || len(manifest.EventsSHA256) != 64 {
		return errors.New("audit archive manifest is invalid")
	}
	return nil
}

func (archive AuditArchive) Manifest() AuditArchiveManifest {
	return AuditArchiveManifest{
		Version: AuditArchiveFormatVersion, ArchiveID: archive.ID, StreamKey: archive.StreamKey,
		WorkspaceID: archive.WorkspaceID, FromSeq: archive.FromSeq, ToSeq: archive.ToSeq,
		EventCount: archive.EventCount, FirstPrevHash: archive.FirstPrevHash, LastHash: archive.LastHash,
		EventsSHA256: archive.EventsSHA256, CreatedAtMS: archive.CreatedAt.UnixMilli(), ManifestHash: archive.ManifestHash,
	}
}
