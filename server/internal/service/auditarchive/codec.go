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
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
)

const archiveMagic = "FSM-AUDIT-ARCHIVE-V1\n"

func encodeArchive(archive *model.AuditArchive, events []model.OperationLog, encodedKey string) ([]byte, error) {
	if err := dao.ValidateAuditArchiveEvents(archive, events); err != nil {
		return nil, err
	}
	manifest, err := archive.Manifest().WithHash()
	if err != nil {
		return nil, err
	}
	archive.ManifestHash = manifest.ManifestHash
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(true)
	if err := encoder.Encode(manifest); err != nil {
		return nil, err
	}
	for index := range events {
		if err := encoder.Encode(struct {
			StreamKey string             `json:"stream_key"`
			Event     model.OperationLog `json:"event"`
		}{StreamKey: events[index].StreamKey, Event: events[index]}); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	key, err := decodeArchiveKey(encodedKey)
	if err != nil {
		return nil, err
	}
	gcm, err := archiveGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ciphertext := gcm.Seal(nil, nonce, compressed.Bytes(), []byte(archiveMagic))
	result := make([]byte, 0, len(archiveMagic)+len(nonce)+len(ciphertext))
	result = append(result, archiveMagic...)
	result = append(result, nonce...)
	result = append(result, ciphertext...)
	return result, nil
}

func decodeArchive(data []byte, encodedKey string) (model.AuditArchiveManifest, []model.OperationLog, error) {
	if !bytes.HasPrefix(data, []byte(archiveMagic)) {
		return model.AuditArchiveManifest{}, nil, errors.New("audit archive format is invalid")
	}
	key, err := decodeArchiveKey(encodedKey)
	if err != nil {
		return model.AuditArchiveManifest{}, nil, err
	}
	gcm, err := archiveGCM(key)
	if err != nil {
		return model.AuditArchiveManifest{}, nil, err
	}
	payload := data[len(archiveMagic):]
	if len(payload) <= gcm.NonceSize() {
		return model.AuditArchiveManifest{}, nil, errors.New("audit archive is truncated")
	}
	plaintext, err := gcm.Open(nil, payload[:gcm.NonceSize()], payload[gcm.NonceSize():], []byte(archiveMagic))
	if err != nil {
		return model.AuditArchiveManifest{}, nil, errors.New("audit archive authentication failed")
	}
	reader, err := gzip.NewReader(bytes.NewReader(plaintext))
	if err != nil {
		return model.AuditArchiveManifest{}, nil, errors.New("audit archive compression is invalid")
	}
	defer reader.Close()
	decoder := json.NewDecoder(bufio.NewReader(io.LimitReader(reader, 1<<30)))
	var manifest model.AuditArchiveManifest
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, nil, err
	}
	if err := manifest.Validate(); err != nil {
		return manifest, nil, err
	}
	events := make([]model.OperationLog, 0, manifest.EventCount)
	for {
		var archivedEvent struct {
			StreamKey string             `json:"stream_key"`
			Event     model.OperationLog `json:"event"`
		}
		if err := decoder.Decode(&archivedEvent); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return manifest, nil, err
		}
		archivedEvent.Event.StreamKey = archivedEvent.StreamKey
		event := archivedEvent.Event
		events = append(events, event)
		if len(events) > manifest.EventCount {
			return manifest, nil, errors.New("audit archive contains extra events")
		}
	}
	if len(events) != manifest.EventCount {
		return manifest, nil, fmt.Errorf("audit archive contains %d events, want %d", len(events), manifest.EventCount)
	}
	return manifest, events, nil
}

func verifyArchive(data []byte, archive *model.AuditArchive, encodedKey string) error {
	manifest, events, err := decodeArchive(data, encodedKey)
	if err != nil {
		return err
	}
	if manifest.ArchiveID != archive.ID || manifest.StreamKey != archive.StreamKey || manifest.FromSeq != archive.FromSeq || manifest.ToSeq != archive.ToSeq || manifest.EventCount != archive.EventCount || manifest.FirstPrevHash != archive.FirstPrevHash || manifest.LastHash != archive.LastHash || manifest.EventsSHA256 != archive.EventsSHA256 || manifest.ManifestHash != archive.ManifestHash {
		return errors.New("audit archive manifest does not match database receipt")
	}
	if (manifest.WorkspaceID == nil) != (archive.WorkspaceID == nil) || (manifest.WorkspaceID != nil && archive.WorkspaceID != nil && *manifest.WorkspaceID != *archive.WorkspaceID) {
		return errors.New("audit archive workspace does not match database receipt")
	}
	digest, err := dao.AuditArchiveEventsDigest(events)
	if err != nil {
		return err
	}
	if digest != manifest.EventsSHA256 {
		return errors.New("audit archive event digest mismatch")
	}
	return dao.ValidateAuditArchiveEvents(archive, events)
}

func decodeArchiveKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("audit archive encryption key is not configured")
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, errors.New("audit archive encryption key must be base64-encoded 32 bytes")
	}
	return key, nil
}

func archiveGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func objectDigest(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
