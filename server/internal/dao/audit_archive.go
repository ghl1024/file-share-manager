/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type AuditArchiveDAO struct{ db *gorm.DB }

type AuditArchiveCandidate struct {
	Archive model.AuditArchive
	Events  []model.OperationLog
}

func NewAuditArchiveDAO() *AuditArchiveDAO { return &AuditArchiveDAO{db: database.DB} }

func NewAuditArchiveWorkerDAO() *AuditArchiveDAO {
	return &AuditArchiveDAO{db: database.AuditArchiveDB}
}

func (dao *AuditArchiveDAO) ListPage(workspaceID uint, page, pageSize int) (*pagination.PageResult[model.AuditArchive], error) {
	query := dao.db.Model(&model.AuditArchive{}).Where("workspace_id = ?", workspaceID).Order("created_at DESC, id DESC")
	return pagination.Paging[model.AuditArchive](query, page, pageSize)
}

func (dao *AuditArchiveDAO) RequeueInterrupted() error {
	return dao.db.Model(&model.AuditArchive{}).Where("status = ?", "running").Updates(map[string]any{
		"status": "queued", "started_at": nil, "updated_at": time.Now(),
	}).Error
}

func (dao *AuditArchiveDAO) ListProcessableIDs(limit int) ([]string, error) {
	if limit < 1 || limit > 100 {
		limit = 20
	}
	var ids []string
	err := dao.db.Model(&model.AuditArchive{}).
		Where("status IN ? AND failure_count < ?", []string{"queued", "failed"}, 10).
		Order("created_at ASC, id ASC").Limit(limit).Pluck("id", &ids).Error
	return ids, err
}

func (dao *AuditArchiveDAO) Claim(id string, now time.Time) (bool, error) {
	result := dao.db.Model(&model.AuditArchive{}).
		Where("id = ? AND status IN ? AND failure_count < ?", id, []string{"queued", "failed"}, 10).
		Updates(map[string]any{"status": "running", "started_at": now, "error_message": "", "updated_at": now})
	return result.RowsAffected == 1, result.Error
}

func (dao *AuditArchiveDAO) Get(id string) (*model.AuditArchive, error) {
	var archive model.AuditArchive
	err := dao.db.Where("id = ?", id).First(&archive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &archive, err
}

func (dao *AuditArchiveDAO) Create(archive *model.AuditArchive) error {
	return dao.db.Create(archive).Error
}

func (dao *AuditArchiveDAO) Fail(id string, failure error) error {
	message := "审计归档任务失败"
	if failure != nil && strings.TrimSpace(failure.Error()) != "" {
		message = failure.Error()
	}
	if len(message) > 1000 {
		message = message[:1000]
	}
	return dao.db.Model(&model.AuditArchive{}).Where("id = ? AND status = ?", id, "running").Updates(map[string]any{
		"status": "failed", "failure_count": gorm.Expr("failure_count + 1"), "error_message": message, "updated_at": time.Now(),
	}).Error
}

func (dao *AuditArchiveDAO) CandidateStreamKeys(cutoff time.Time, limit int) ([]string, error) {
	if limit < 1 || limit > 1000 {
		limit = 100
	}
	var keys []string
	err := dao.db.Model(&model.OperationLog{}).
		Where("created_at < ?", cutoff).Distinct().Order("stream_key ASC").Limit(limit).Pluck("stream_key", &keys).Error
	return keys, err
}

func (dao *AuditArchiveDAO) BuildCandidate(streamKey string, cutoff time.Time, limit int) (*AuditArchiveCandidate, error) {
	if limit < 1 || limit > 100000 {
		return nil, errors.New("audit archive batch size is invalid")
	}
	var active int64
	if err := dao.db.Model(&model.AuditArchive{}).
		Where("stream_key = ? AND status <> ?", streamKey, "completed").Count(&active).Error; err != nil {
		return nil, err
	}
	if active > 0 {
		return nil, nil
	}

	var previous model.AuditArchive
	previousErr := dao.db.Where("stream_key = ? AND status = ?", streamKey, "completed").Order("to_seq DESC").First(&previous).Error
	if previousErr != nil && !errors.Is(previousErr, gorm.ErrRecordNotFound) {
		return nil, previousErr
	}
	expectedSeq := uint64(1)
	previousHash := ""
	if previousErr == nil {
		expectedSeq = previous.ToSeq + 1
		previousHash = previous.LastHash
	}
	var events []model.OperationLog
	if err := dao.db.Where("stream_key = ? AND stream_seq >= ? AND created_at < ?", streamKey, expectedSeq, cutoff).
		Order("stream_seq ASC").Limit(limit).Find(&events).Error; err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	archive := model.AuditArchive{
		StreamKey: streamKey, WorkspaceID: events[0].WorkspaceID, Status: "queued",
		FromSeq: expectedSeq, ToSeq: events[len(events)-1].StreamSeq, EventCount: len(events),
		FirstPrevHash: previousHash, CreatedAt: time.Now(),
	}
	if err := ValidateAuditArchiveEvents(&archive, events); err != nil {
		return nil, err
	}
	archive.LastHash = *events[len(events)-1].CurrentHash
	digest, err := AuditArchiveEventsDigest(events)
	if err != nil {
		return nil, err
	}
	archive.EventsSHA256 = digest
	return &AuditArchiveCandidate{Archive: archive, Events: events}, nil
}

func (dao *AuditArchiveDAO) LoadEvents(archive *model.AuditArchive) ([]model.OperationLog, error) {
	var events []model.OperationLog
	err := dao.db.Where("stream_key = ? AND stream_seq BETWEEN ? AND ?", archive.StreamKey, archive.FromSeq, archive.ToSeq).
		Order("stream_seq ASC").Find(&events).Error
	return events, err
}

func (dao *AuditArchiveDAO) Finalize(id string, objectSize int64, objectSHA256 string, now time.Time) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var archive model.AuditArchive
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND status = ?", id, "running").First(&archive).Error; err != nil {
			return err
		}
		var events []model.OperationLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("stream_key = ? AND stream_seq BETWEEN ? AND ?", archive.StreamKey, archive.FromSeq, archive.ToSeq).
			Order("stream_seq ASC").Find(&events).Error; err != nil {
			return err
		}
		if err := ValidateAuditArchiveEvents(&archive, events); err != nil {
			return err
		}
		digest, err := AuditArchiveEventsDigest(events)
		if err != nil {
			return err
		}
		if digest != archive.EventsSHA256 {
			return errors.New("audit archive source changed before cleanup")
		}
		if err := tx.Model(&model.AuditArchive{}).Where("id = ? AND status = ?", id, "running").Updates(map[string]any{
			"status": "completed", "object_size": objectSize, "object_sha256": objectSHA256,
			"verified_at": now, "completed_at": now, "error_message": "", "updated_at": now,
		}).Error; err != nil {
			return err
		}
		result := tx.Where("stream_key = ? AND stream_seq BETWEEN ? AND ?", archive.StreamKey, archive.FromSeq, archive.ToSeq).Delete(&model.OperationLog{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != int64(archive.EventCount) {
			return fmt.Errorf("deleted %d audit rows, want %d", result.RowsAffected, archive.EventCount)
		}
		return nil
	})
}

func ValidateAuditArchiveEvents(archive *model.AuditArchive, events []model.OperationLog) error {
	if len(events) != archive.EventCount || len(events) == 0 {
		return fmt.Errorf("audit archive event count = %d, want %d", len(events), archive.EventCount)
	}
	var previous *string
	if archive.FromSeq > 1 {
		if len(archive.FirstPrevHash) != 64 {
			return errors.New("audit archive is missing its previous hash anchor")
		}
		value := archive.FirstPrevHash
		previous = &value
	}
	for index := range events {
		event := &events[index]
		expectedSeq := archive.FromSeq + uint64(index)
		if event.StreamKey != archive.StreamKey || event.StreamSeq != expectedSeq || event.StreamSeq > archive.ToSeq {
			return fmt.Errorf("audit archive stream sequence is not contiguous at %d", expectedSeq)
		}
		if !sameHash(previous, event.PrevHash) {
			return fmt.Errorf("audit archive previous hash mismatch at %d", expectedSeq)
		}
		expectedHash := calculateHash(event, previous)
		if event.CurrentHash == nil || *event.CurrentHash != expectedHash {
			return fmt.Errorf("audit archive event hash mismatch at %d", expectedSeq)
		}
		current := *event.CurrentHash
		previous = &current
	}
	if events[len(events)-1].StreamSeq != archive.ToSeq {
		return errors.New("audit archive end sequence mismatch")
	}
	if archive.LastHash != "" && (previous == nil || *previous != archive.LastHash) {
		return errors.New("audit archive last hash mismatch")
	}
	return nil
}

func AuditArchiveEventsDigest(events []model.OperationLog) (string, error) {
	hash := sha256.New()
	for index := range events {
		encoded, err := json.Marshal(struct {
			StreamKey string             `json:"stream_key"`
			Event     model.OperationLog `json:"event"`
		}{StreamKey: events[index].StreamKey, Event: events[index]})
		if err != nil {
			return "", err
		}
		if _, err := hash.Write(encoded); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte{'\n'}); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
