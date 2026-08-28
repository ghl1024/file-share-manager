/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditexport

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"

	"github.com/google/uuid"
)

type Service struct {
	jobs      *dao.AuditExportDAO
	logs      *dao.OperationLogDAO
	queue     chan string
	startOnce sync.Once
}

var defaultService *Service
var defaultMu sync.Mutex

func NewService() *Service {
	return &Service{jobs: dao.NewAuditExportDAO(), logs: dao.NewOperationLogDAO(), queue: make(chan string, 512)}
}
func DefaultService() *Service {
	defaultMu.Lock()
	defer defaultMu.Unlock()
	if defaultService == nil {
		defaultService = NewService()
	}
	return defaultService
}

func (s *Service) Start(ctx context.Context) {
	s.startOnce.Do(func() {
		_ = s.jobs.RequeueInterrupted()
		go s.worker(ctx)
		go s.dispatch(ctx)
	})
}

func (s *Service) Create(workspaceID, userID uint, format string, filters dao.AuditFilters) (*model.AuditExportJob, error) {
	encoded, err := json.Marshal(filters)
	if err != nil {
		return nil, err
	}
	job := &model.AuditExportJob{ID: uuid.NewString(), WorkspaceID: workspaceID, CreatedBy: userID, Format: format, Status: "queued", FilterJSON: string(encoded)}
	if err := s.jobs.Create(job); err != nil {
		return nil, err
	}
	s.Enqueue(job.ID)
	return job, nil
}

func (s *Service) List(workspaceID, userID uint, page, pageSize int) (*pagination.PageResult[model.AuditExportJob], error) {
	return s.jobs.ListPage(workspaceID, userID, page, pageSize)
}
func (s *Service) Get(workspaceID, userID uint, id string) (*model.AuditExportJob, error) {
	return s.jobs.GetForOwner(workspaceID, userID, id)
}

func (s *Service) Expire(now time.Time) ([]string, error) { return s.jobs.Expire(now) }
func (s *Service) Enqueue(id string) {
	select {
	case s.queue <- id:
	default:
	}
}

func (s *Service) dispatch(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ids, err := s.jobs.ListQueuedIDs(500)
			if err == nil {
				for _, id := range ids {
					s.Enqueue(id)
				}
			}
		}
	}
}

func (s *Service) worker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.queue:
			s.process(id)
		}
	}
}

func (s *Service) process(id string) {
	claimed, err := s.jobs.Claim(id, time.Now())
	if err != nil || !claimed {
		return
	}
	job, err := s.jobs.GetByID(id)
	if err != nil || job == nil {
		_ = s.jobs.Fail(id, time.Now())
		return
	}
	var filters dao.AuditFilters
	if err := json.Unmarshal([]byte(job.FilterJSON), &filters); err != nil {
		_ = s.jobs.Fail(id, time.Now())
		return
	}
	logs, err := s.logs.ListForExport(job.WorkspaceID, filters, 100000)
	if err != nil {
		_ = s.jobs.Fail(id, time.Now())
		return
	}
	path, size, err := writeExport(job, logs)
	if err != nil {
		_ = s.jobs.Fail(id, time.Now())
		logger.Error("audit_export_failed", "job_id", id, "error", err)
		return
	}
	now := time.Now()
	retention := 24 * time.Hour
	if cfg := config.GetConfig(); cfg != nil && cfg.Audit.ExportRetentionHours > 0 {
		retention = time.Duration(cfg.Audit.ExportRetentionHours) * time.Hour
	}
	if err := s.jobs.Complete(id, path, len(logs), size, now, now.Add(retention)); err != nil {
		_ = os.Remove(path)
	}
}

func writeExport(job *model.AuditExportJob, logs []model.OperationLog) (string, int64, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return "", 0, errors.New("configuration is not loaded")
	}
	directory := filepath.Join(cfg.Storage.StagingPath, "audit-exports")
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return "", 0, err
	}
	path := filepath.Join(directory, job.ID+"."+job.Format)
	file, err := os.OpenFile(path+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", 0, err
	}
	committed := false
	defer func() {
		_ = file.Close()
		if !committed {
			_ = os.Remove(path + ".tmp")
		}
	}()
	if job.Format == "csv" {
		err = writeCSV(file, logs)
	} else {
		err = writeJSON(file, logs)
	}
	if err != nil {
		return "", 0, err
	}
	if err := file.Sync(); err != nil {
		return "", 0, err
	}
	if err := file.Close(); err != nil {
		return "", 0, err
	}
	if err := os.Rename(path+".tmp", path); err != nil {
		return "", 0, err
	}
	committed = true
	info, err := os.Stat(path)
	if err != nil {
		return "", 0, err
	}
	return path, info.Size(), nil
}

type exportRow struct {
	ID         uint      `json:"id"`
	StreamSeq  uint64    `json:"stream_seq"`
	ActorType  string    `json:"actor_type"`
	Username   string    `json:"username"`
	Category   string    `json:"category"`
	Action     string    `json:"action"`
	Severity   string    `json:"severity"`
	Result     string    `json:"result"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	Status     int       `json:"status"`
	IP         string    `json:"ip"`
	Latency    int64     `json:"latency_ms"`
	RequestID  string    `json:"request_id"`
	TraceID    string    `json:"trace_id"`
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	CreatedAt  time.Time `json:"created_at"`
}

func rows(logs []model.OperationLog) []exportRow {
	result := make([]exportRow, 0, len(logs))
	for _, log := range logs {
		result = append(result, exportRow{
			ID: log.ID, StreamSeq: log.StreamSeq, ActorType: log.ActorType, Username: log.Username,
			Category: log.Category, Action: log.Action, Severity: log.Severity, Result: log.Result,
			Method: log.Method, Path: logger.SanitizeText(log.Path), Status: log.Status, IP: log.IP,
			Latency: log.Latency, RequestID: log.RequestID, TraceID: log.TraceID,
			TargetType: log.TargetType, TargetID: log.TargetID, CreatedAt: log.CreatedAt,
		})
	}
	return result
}
func writeJSON(file *os.File, logs []model.OperationLog) error {
	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(true)
	return encoder.Encode(rows(logs))
}
func writeCSV(file *os.File, logs []model.OperationLog) error {
	writer := csv.NewWriter(file)
	if err := writer.Write([]string{"id", "stream_seq", "actor_type", "username", "category", "action", "severity", "result", "method", "path", "status", "ip", "latency_ms", "request_id", "trace_id", "target_type", "target_id", "created_at"}); err != nil {
		return err
	}
	for _, row := range rows(logs) {
		if err := writer.Write([]string{strconv.FormatUint(uint64(row.ID), 10), strconv.FormatUint(row.StreamSeq, 10), csvCell(row.ActorType), csvCell(row.Username), csvCell(row.Category), csvCell(row.Action), csvCell(row.Severity), csvCell(row.Result), csvCell(row.Method), csvCell(row.Path), strconv.Itoa(row.Status), csvCell(row.IP), strconv.FormatInt(row.Latency, 10), csvCell(row.RequestID), csvCell(row.TraceID), csvCell(row.TargetType), csvCell(row.TargetID), row.CreatedAt.Format(time.RFC3339)}); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}
func csvCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
}
func DownloadName(job *model.AuditExportJob) string {
	return fmt.Sprintf("audit-export-%s.%s", job.ID, job.Format)
}
