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
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// OperationLogDAO persists the request audit stream and exposes the same
// paginated query shape used by the other management resources.
type OperationLogDAO struct {
	db *gorm.DB
}

type AuditFilters struct {
	Username   string
	ActorType  string
	Method     string
	Action     string
	Category   string
	Severity   string
	Result     string
	TargetType string
	TargetID   string
	IP         string
	RequestID  string
	Status     *int
	From       *time.Time
	To         *time.Time
}

type AuditVerification struct {
	Valid      bool   `json:"valid"`
	Checked    int    `json:"checked"`
	Archived   int    `json:"archived"`
	StreamKey  string `json:"stream_key"`
	LastSeq    uint64 `json:"last_seq"`
	BrokenID   uint   `json:"broken_id,omitempty"`
	BrokenKind string `json:"broken_kind,omitempty"`
}

func NewOperationLogDAO() *OperationLogDAO {
	return &OperationLogDAO{db: database.DB}
}

func (dao *OperationLogDAO) Create(log *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error { return createOperationLogWithTx(tx, log) })
}

// appendAuditEvent persists a business mutation event using the caller's
// transaction. The event is intentionally written before the transaction
// commits so a failed mutation cannot leave an audit record for a state that
// never became visible.
func appendAuditEvent(tx *gorm.DB, event *model.OperationLog, before, after any) error {
	if event == nil {
		return nil
	}
	if before != nil {
		encoded, err := json.Marshal(before)
		if err != nil {
			return err
		}
		value := string(encoded)
		event.BeforeJSON = &value
	}
	if after != nil {
		encoded, err := json.Marshal(after)
		if err != nil {
			return err
		}
		value := string(encoded)
		event.AfterJSON = &value
	}
	return createOperationLogWithTx(tx, event)
}

func createOperationLogWithTx(tx *gorm.DB, log *model.OperationLog) error {
	// MySQL stores operation_logs.created_at as datetime(3). Normalize before
	// hashing so the persisted value produces the same digest when read back.
	log.CreatedAt = normalizeAuditTimestamp(log.CreatedAt)
	log.PrevHash = nil
	log.CurrentHash = nil
	sanitizeAuditEvent(log)
	if strings.TrimSpace(log.Details) == "" {
		log.Details = "{}"
	}
	log.BeforeJSON = normalizeOptionalAuditJSON(log.BeforeJSON)
	log.AfterJSON = normalizeOptionalAuditJSON(log.AfterJSON)
	log.MetadataJSON = normalizeOptionalAuditJSON(log.MetadataJSON)
	if log.ActorWorkspaceID == nil && log.SourceWorkspaceID == nil && log.TargetWorkspaceID == nil && log.WorkspaceID != nil && log.UserID > 0 {
		workspaceID := *log.WorkspaceID
		log.ActorWorkspaceID = &workspaceID
	}
	stream, err := lockAuditStream(tx, log.WorkspaceID)
	if err != nil {
		return err
	}
	log.StreamKey = stream.StreamKey
	log.StreamSeq = stream.NextSeq
	if stream.LastHash != "" {
		previous := stream.LastHash
		log.PrevHash = &previous
	}
	if strings.TrimSpace(log.ActorType) == "" {
		log.ActorType = auditActorType(log)
	}
	if strings.TrimSpace(log.Category) == "" {
		log.Category = auditCategory(log.Action)
	}
	if strings.TrimSpace(log.Severity) == "" {
		log.Severity = auditSeverity(log.Status)
	}
	if strings.TrimSpace(log.Result) == "" {
		log.Result = auditResult(log.Status)
	}
	if strings.TrimSpace(log.ReasonCode) == "" {
		log.ReasonCode = auditReasonCode(log.Status, log.Details)
	}
	if !model.ValidAuditActorType(log.ActorType) || !model.ValidAuditCategory(log.Category) ||
		!model.ValidAuditSeverity(log.Severity) || !model.ValidAuditResult(log.Result) ||
		(log.ReasonCode != "" && !model.ValidAuditReasonCode(log.ReasonCode)) {
		return fmt.Errorf("invalid audit event classification")
	}
	currentHash := calculateHash(log, log.PrevHash)
	log.CurrentHash = &currentHash
	if err := tx.Create(log).Error; err != nil {
		return err
	}
	return tx.Model(&model.AuditStream{}).Where("stream_key = ?", stream.StreamKey).
		Updates(map[string]any{"next_seq": stream.NextSeq + 1, "last_hash": currentHash, "updated_at": log.CreatedAt}).Error
}

func lockAuditStream(tx *gorm.DB, workspaceID *uint) (*model.AuditStream, error) {
	key := auditStreamKey(workspaceID)
	stream := model.AuditStream{StreamKey: key, WorkspaceID: workspaceID, NextSeq: 1}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&stream).Error; err != nil {
		return nil, err
	}
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("stream_key = ?", key).First(&stream).Error; err != nil {
		return nil, err
	}
	if stream.NextSeq == 0 {
		stream.NextSeq = 1
	}
	return &stream, nil
}

func auditStreamKey(workspaceID *uint) string {
	if workspaceID == nil {
		return "global"
	}
	return "workspace:" + strconv.FormatUint(uint64(*workspaceID), 10)
}

func (dao *OperationLogDAO) ListPage(workspaceID uint, page, pageSize int, username, method string) (*pagination.PageResult[model.OperationLog], error) {
	return dao.ListPageWithFilters(workspaceID, page, pageSize, AuditFilters{Username: username, Method: method})
}

func (dao *OperationLogDAO) ListPageWithFilters(workspaceID uint, page, pageSize int, filters AuditFilters) (*pagination.PageResult[model.OperationLog], error) {
	query := applyAuditFilters(dao.db.Model(&model.OperationLog{}).Where("workspace_id = ?", workspaceID), filters)
	query = query.Order("created_at DESC, id DESC")
	return pagination.Paging[model.OperationLog](query, page, pageSize)
}

func (dao *OperationLogDAO) ListForExport(workspaceID uint, filters AuditFilters, limit int) ([]model.OperationLog, error) {
	if limit < 1 || limit > 100000 {
		limit = 100000
	}
	var logs []model.OperationLog
	err := applyAuditFilters(dao.db.Model(&model.OperationLog{}).Where("workspace_id = ?", workspaceID), filters).
		Order("created_at ASC, id ASC").Limit(limit).Find(&logs).Error
	return logs, err
}

func applyAuditFilters(query *gorm.DB, filters AuditFilters) *gorm.DB {
	if value := strings.TrimSpace(filters.Username); value != "" {
		query = query.Where("username LIKE ?", "%"+value+"%")
	}
	if value := strings.TrimSpace(filters.ActorType); value != "" {
		query = query.Where("actor_type = ?", value)
	}
	if value := strings.TrimSpace(filters.Method); value != "" {
		query = query.Where("method = ?", value)
	}
	if value := strings.TrimSpace(filters.Action); value != "" {
		query = query.Where("action = ?", value)
	}
	if value := strings.TrimSpace(filters.Category); value != "" {
		query = query.Where("category = ?", value)
	}
	if value := strings.TrimSpace(filters.Severity); value != "" {
		query = query.Where("severity = ?", value)
	}
	if value := strings.TrimSpace(filters.Result); value != "" {
		query = query.Where("result = ?", value)
	}
	if value := strings.TrimSpace(filters.TargetType); value != "" {
		query = query.Where("target_type = ?", value)
	}
	if value := strings.TrimSpace(filters.TargetID); value != "" {
		query = query.Where("target_id = ?", value)
	}
	if value := strings.TrimSpace(filters.IP); value != "" {
		query = query.Where("ip = ?", value)
	}
	if value := strings.TrimSpace(filters.RequestID); value != "" {
		query = query.Where("request_id = ?", value)
	}
	if filters.Status != nil {
		query = query.Where("status = ?", *filters.Status)
	}
	if filters.From != nil {
		query = query.Where("created_at >= ?", *filters.From)
	}
	if filters.To != nil {
		query = query.Where("created_at < ?", *filters.To)
	}
	return query
}

func (dao *OperationLogDAO) GetByID(workspaceID, id uint) (*model.OperationLog, error) {
	var log model.OperationLog
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&log).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &log, nil
}

func (dao *OperationLogDAO) VerifyChain(workspaceID uint) (*AuditVerification, error) {
	var result *AuditVerification
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var verifyErr error
		result, verifyErr = verifyAuditChainSnapshot(tx, workspaceID)
		return verifyErr
	}, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	return result, err
}

func verifyAuditChainSnapshot(db *gorm.DB, workspaceID uint) (*AuditVerification, error) {
	streamKey := auditStreamKey(&workspaceID)
	var archives []model.AuditArchive
	if err := db.Where("stream_key = ? AND status = ?", streamKey, "completed").Order("from_seq ASC").Find(&archives).Error; err != nil {
		return nil, err
	}
	var logs []model.OperationLog
	if err := db.Where("workspace_id = ?", workspaceID).Order("stream_seq ASC").Find(&logs).Error; err != nil {
		return nil, err
	}
	result := &AuditVerification{Valid: true, StreamKey: streamKey}
	var previous *string
	var expectedSeq uint64 = 1
	for _, archive := range archives {
		if archive.FromSeq != expectedSeq || archive.ToSeq < archive.FromSeq || archive.EventCount != int(archive.ToSeq-archive.FromSeq+1) || archive.VerifiedAt == nil || archive.ObjectSHA256 == "" {
			result.Valid = false
			result.BrokenKind = "archive_sequence"
			return result, nil
		}
		manifest := archive.Manifest()
		if manifest.Validate() != nil {
			result.Valid = false
			result.BrokenKind = "archive_manifest"
			return result, nil
		}
		if archive.FromSeq == 1 {
			if archive.FirstPrevHash != "" {
				result.Valid = false
				result.BrokenKind = "archive_previous_hash"
				return result, nil
			}
		} else if previous == nil || archive.FirstPrevHash != *previous {
			result.Valid = false
			result.BrokenKind = "archive_previous_hash"
			return result, nil
		}
		current := archive.LastHash
		previous = &current
		expectedSeq = archive.ToSeq + 1
		result.Archived += archive.EventCount
		result.LastSeq = archive.ToSeq
	}
	for _, log := range logs {
		if log.StreamSeq != expectedSeq {
			result.Valid = false
			result.BrokenID = log.ID
			result.BrokenKind = "stream_sequence"
			return result, nil
		}
		if !sameHash(previous, log.PrevHash) {
			result.Valid = false
			result.BrokenID = log.ID
			result.BrokenKind = "previous_hash"
			return result, nil
		}
		expected := calculateHash(&log, previous)
		if log.CurrentHash == nil || *log.CurrentHash != expected {
			result.Valid = false
			result.BrokenID = log.ID
			result.BrokenKind = "current_hash"
			return result, nil
		}
		current := *log.CurrentHash
		previous = &current
		result.LastSeq = log.StreamSeq
		expectedSeq++
	}
	result.Checked = result.Archived + len(logs)
	var stream model.AuditStream
	if err := db.Where("stream_key = ?", streamKey).First(&stream).Error; err == nil && result.Checked > 0 {
		if stream.NextSeq != expectedSeq || previous == nil || stream.LastHash != *previous {
			result.Valid = false
			if len(logs) > 0 {
				result.BrokenID = logs[len(logs)-1].ID
			}
			result.BrokenKind = "stream_tail"
		}
	} else if errors.Is(err, gorm.ErrRecordNotFound) && result.Checked > 0 {
		result.Valid = false
		if len(logs) > 0 {
			result.BrokenID = logs[len(logs)-1].ID
		}
		result.BrokenKind = "stream_missing"
	} else if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	return result, nil
}

func sameHash(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func calculateHash(log *model.OperationLog, previous *string) string {
	// A struct gives the digest a stable field order while length-delimited JSON
	// avoids separator ambiguity. All persisted event fields are included so a
	// verifier detects changes to metadata as well as the HTTP request fields.
	payload, err := json.Marshal(struct {
		UserID            uint    `json:"user_id"`
		StreamKey         string  `json:"stream_key"`
		StreamSeq         uint64  `json:"stream_seq"`
		ActorType         string  `json:"actor_type"`
		Username          string  `json:"username"`
		WorkspaceID       *uint   `json:"workspace_id"`
		ActorWorkspaceID  *uint   `json:"actor_workspace_id"`
		SourceWorkspaceID *uint   `json:"source_workspace_id"`
		TargetWorkspaceID *uint   `json:"target_workspace_id"`
		Method            string  `json:"method"`
		Path              string  `json:"path"`
		Action            string  `json:"action"`
		Category          string  `json:"category"`
		Severity          string  `json:"severity"`
		Result            string  `json:"result"`
		ReasonCode        string  `json:"reason_code"`
		NodeID            *uint   `json:"node_id"`
		TargetType        string  `json:"target_type"`
		TargetID          string  `json:"target_id"`
		TargetName        string  `json:"target_name"`
		Status            int     `json:"status"`
		IP                string  `json:"ip"`
		Latency           int64   `json:"latency"`
		ErrorMessage      string  `json:"error_message"`
		Details           string  `json:"details"`
		RequestID         string  `json:"request_id"`
		TraceID           string  `json:"trace_id"`
		UserAgent         string  `json:"user_agent"`
		Origin            string  `json:"origin"`
		BeforeJSON        *string `json:"before_json"`
		AfterJSON         *string `json:"after_json"`
		MetadataJSON      *string `json:"metadata_json"`
		PreviousHash      *string `json:"previous_hash"`
		CreatedAt         int64   `json:"created_at_unix_ms"`
	}{
		UserID: log.UserID, StreamKey: log.StreamKey, StreamSeq: log.StreamSeq, ActorType: log.ActorType, Username: log.Username, WorkspaceID: log.WorkspaceID,
		ActorWorkspaceID: log.ActorWorkspaceID, SourceWorkspaceID: log.SourceWorkspaceID, TargetWorkspaceID: log.TargetWorkspaceID,
		Method: log.Method, Path: log.Path, Action: log.Action, Category: log.Category,
		Severity: log.Severity, Result: log.Result, ReasonCode: log.ReasonCode, NodeID: log.NodeID,
		TargetType: log.TargetType, TargetID: log.TargetID, TargetName: log.TargetName,
		Status: log.Status, IP: log.IP, Latency: log.Latency, ErrorMessage: log.ErrorMessage,
		Details: normalizeAuditDetails(log.Details), RequestID: log.RequestID, TraceID: log.TraceID, UserAgent: log.UserAgent,
		Origin: log.Origin, BeforeJSON: normalizeOptionalAuditJSON(log.BeforeJSON), AfterJSON: normalizeOptionalAuditJSON(log.AfterJSON), MetadataJSON: normalizeOptionalAuditJSON(log.MetadataJSON),
		PreviousHash: previous, CreatedAt: normalizeAuditTimestamp(log.CreatedAt).UnixMilli(),
	})
	if err != nil {
		panic("marshal operation log hash payload: " + err.Error())
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

// CalculateAuditEventHash is shared by archive verification and tests so all
// audit-chain consumers use the same canonical event representation.
func CalculateAuditEventHash(log *model.OperationLog, previous *string) string {
	return calculateHash(log, previous)
}

func normalizeAuditTimestamp(value time.Time) time.Time {
	if value.IsZero() {
		value = time.Now()
	}
	return value.Truncate(time.Millisecond)
}

func normalizeAuditDetails(value string) string {
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return value
	}
	normalized, err := json.Marshal(decoded)
	if err != nil {
		return value
	}
	return string(normalized)
}

func normalizeOptionalAuditJSON(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	normalized := normalizeAuditDetails(*value)
	return &normalized
}

func sanitizeAuditEvent(log *model.OperationLog) {
	if log == nil {
		return
	}
	log.Path = logger.SanitizeText(log.Path)
	log.ErrorMessage = logger.SanitizeText(log.ErrorMessage)
	log.UserAgent = logger.SanitizeText(log.UserAgent)
	log.Origin = logger.SanitizeText(log.Origin)
	log.Details = sanitizeAuditPayload(log.Details)
	log.BeforeJSON = sanitizeOptionalAuditPayload(log.BeforeJSON)
	log.AfterJSON = sanitizeOptionalAuditPayload(log.AfterJSON)
	log.MetadataJSON = sanitizeOptionalAuditPayload(log.MetadataJSON)
}

func sanitizeOptionalAuditPayload(value *string) *string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil
	}
	sanitized := sanitizeAuditPayload(*value)
	return &sanitized
}

func sanitizeAuditPayload(value string) string {
	if strings.TrimSpace(value) == "" {
		return value
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return logger.SanitizeText(value)
	}
	encoded, err := json.Marshal(sanitizeAuditValue(decoded, ""))
	if err != nil {
		return logger.SanitizeText(value)
	}
	return string(encoded)
}

func sanitizeAuditValue(value any, parentKey string) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveAuditKey(key) {
				result[key] = "[REDACTED]"
				continue
			}
			result[key] = sanitizeAuditValue(child, key)
		}
		return result
	case []any:
		result := make([]any, len(typed))
		for index, child := range typed {
			result[index] = sanitizeAuditValue(child, parentKey)
		}
		return result
	case string:
		if isAuditEnumKey(parentKey) {
			return typed
		}
		return logger.SanitizeText(typed)
	default:
		return value
	}
}

func isAuditEnumKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "action", "actor_type", "category", "decision", "method", "permission", "reason_code", "result", "scope", "severity", "source", "target_type":
		return true
	default:
		return false
	}
}

func isSensitiveAuditKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	switch normalized {
	case "password_configured", "password_changed":
		return false
	}
	return logger.IsSensitiveKey(normalized)
}

// RebuildChain recalculates a workspace audit chain from persisted values.
// It is intentionally not exposed over HTTP and is only used by the explicit
// maintenance command for data written before timestamp normalization.
func (dao *OperationLogDAO) RebuildChain(workspaceID uint) (int, error) {
	count := 0
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var logs []model.OperationLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ?", workspaceID).
			Order("id ASC").
			Find(&logs).Error; err != nil {
			return err
		}

		var previous *string
		streamKey := auditStreamKey(&workspaceID)
		for i := range logs {
			legacy := logs[i].StreamSeq == 0
			logs[i].StreamKey = streamKey
			logs[i].StreamSeq = uint64(i + 1)
			if legacy || strings.TrimSpace(logs[i].ActorType) == "" {
				logs[i].ActorType = auditActorType(&logs[i])
			}
			current := calculateHash(&logs[i], previous)
			if err := tx.Model(&model.OperationLog{}).
				Where("id = ? AND workspace_id = ?", logs[i].ID, workspaceID).
				UpdateColumns(map[string]any{"stream_key": streamKey, "stream_seq": logs[i].StreamSeq, "actor_type": logs[i].ActorType, "prev_hash": previous, "current_hash": current}).Error; err != nil {
				return err
			}
			previous = &current
			count++
		}
		lastHash := ""
		if previous != nil {
			lastHash = *previous
		}
		stream := model.AuditStream{StreamKey: streamKey, WorkspaceID: &workspaceID, NextSeq: uint64(len(logs) + 1), LastHash: lastHash, UpdatedAt: time.Now()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stream_key"}}, DoUpdates: clause.AssignmentColumns([]string{"workspace_id", "next_seq", "last_hash", "updated_at"})}).Create(&stream).Error
	})
	return count, err
}

func auditActorType(log *model.OperationLog) string {
	if log.UserID == 0 && log.Username == "external_share" {
		return model.AuditActorExternalShare
	}
	if log.UserID == 0 {
		return model.AuditActorSystem
	}
	return model.AuditActorUser
}

// EnsureAuditStreams backfills legacy event sequences and creates the unique
// stream sequence index only after every existing row has a nonzero sequence.
func EnsureAuditStreams(db *gorm.DB) error {
	var legacyCount int64
	if err := db.Model(&model.OperationLog{}).
		Where("stream_key = '' OR stream_seq = 0").
		Count(&legacyCount).Error; err != nil {
		return err
	}
	if legacyCount > 0 {
		var workspaceIDs []uint
		if err := db.Model(&model.OperationLog{}).Where("workspace_id IS NOT NULL").Distinct().Order("workspace_id ASC").Pluck("workspace_id", &workspaceIDs).Error; err != nil {
			return err
		}
		dao := &OperationLogDAO{db: db}
		for _, workspaceID := range workspaceIDs {
			if _, err := dao.RebuildChain(workspaceID); err != nil {
				return err
			}
		}
		if err := rebuildGlobalAuditChain(db); err != nil {
			return err
		}
	}
	if !db.Migrator().HasIndex(&model.OperationLog{}, "uidx_audit_stream_seq") {
		if err := db.Exec("CREATE UNIQUE INDEX uidx_audit_stream_seq ON operation_logs (stream_key, stream_seq)").Error; err != nil {
			return fmt.Errorf("create audit stream sequence index: %w", err)
		}
	}
	return nil
}

func rebuildGlobalAuditChain(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var logs []model.OperationLog
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("workspace_id IS NULL").Order("id ASC").Find(&logs).Error; err != nil {
			return err
		}
		var previous *string
		for i := range logs {
			legacy := logs[i].StreamSeq == 0
			logs[i].StreamKey = "global"
			logs[i].StreamSeq = uint64(i + 1)
			if legacy || strings.TrimSpace(logs[i].ActorType) == "" {
				logs[i].ActorType = auditActorType(&logs[i])
			}
			current := calculateHash(&logs[i], previous)
			if err := tx.Model(&model.OperationLog{}).Where("id = ?", logs[i].ID).UpdateColumns(map[string]any{"stream_key": "global", "stream_seq": logs[i].StreamSeq, "actor_type": logs[i].ActorType, "prev_hash": previous, "current_hash": current}).Error; err != nil {
				return err
			}
			previous = &current
		}
		lastHash := ""
		if previous != nil {
			lastHash = *previous
		}
		stream := model.AuditStream{StreamKey: "global", NextSeq: uint64(len(logs) + 1), LastHash: lastHash, UpdatedAt: time.Now()}
		return tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "stream_key"}}, DoUpdates: clause.AssignmentColumns([]string{"next_seq", "last_hash", "updated_at"})}).Create(&stream).Error
	})
}

func auditCategory(action string) string {
	action = strings.ToLower(strings.TrimSpace(action))
	if strings.Contains(action, "deny") || strings.Contains(action, "denied") || strings.Contains(action, "password_failed") || strings.Contains(action, "security") {
		return model.AuditCategorySecurity
	}
	if strings.Contains(action, "download") || strings.Contains(action, "preview") || strings.Contains(action, "read") || strings.Contains(action, "list") || strings.Contains(action, "search") ||
		action == "share:access" {
		return model.AuditCategoryAccess
	}
	return model.AuditCategoryOperation
}

func auditSeverity(status int) string {
	if status >= 500 {
		return model.AuditSeverityHigh
	}
	if status >= 400 {
		return model.AuditSeverityWarning
	}
	return model.AuditSeverityInfo
}

func auditResult(status int) string {
	if status == 401 || status == 403 {
		return model.AuditResultDenied
	}
	if status >= 400 {
		return model.AuditResultFailure
	}
	return model.AuditResultSuccess
}

func auditReasonCode(status int, details string) string {
	var payload struct {
		Reason string `json:"reason"`
	}
	if json.Unmarshal([]byte(details), &payload) == nil {
		reason := strings.TrimSpace(payload.Reason)
		if model.ValidAuditReasonCode(reason) {
			return reason
		}
	}
	switch {
	case status == 400:
		return model.AuditReasonInvalidRequest
	case status == 401:
		return model.AuditReasonAuthenticationRequired
	case status == 403:
		return model.AuditReasonPermissionDenied
	case status == 404:
		return model.AuditReasonResourceNotFound
	case status == 409:
		return model.AuditReasonConflict
	case status == 410:
		return model.AuditReasonResourceGone
	case status == 429:
		return model.AuditReasonRateLimited
	case status >= 500:
		return model.AuditReasonInternalError
	default:
		return ""
	}
}
