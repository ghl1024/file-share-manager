/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storagehealth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/service/notification"
)

type PathHealth struct {
	Name          string  `json:"name"`
	Path          string  `json:"path"`
	Available     bool    `json:"available"`
	Writable      bool    `json:"writable"`
	LowSpace      bool    `json:"low_space"`
	CriticalSpace bool    `json:"critical_space"`
	TotalBytes    uint64  `json:"total_bytes"`
	FreeBytes     uint64  `json:"free_bytes"`
	UsedBytes     uint64  `json:"used_bytes"`
	FreePercent   float64 `json:"free_percent"`
	Error         string  `json:"error,omitempty"`
}

type Report struct {
	Status          string      `json:"status"`
	CheckedAt       time.Time   `json:"checked_at"`
	MinFreeBytes    int64       `json:"min_free_bytes"`
	WarnFreePercent int         `json:"warn_free_percent"`
	MinFreePercent  int         `json:"min_free_percent"`
	Root            PathHealth  `json:"root"`
	Staging         PathHealth  `json:"staging"`
	Backup          *PathHealth `json:"backup,omitempty"`
	Alerts          []string    `json:"alerts"`
}

func Check(cfg *config.Config) Report {
	report := Report{Status: "ready", CheckedAt: time.Now()}
	if cfg == nil {
		report.Status = "degraded"
		report.Alerts = []string{"系统配置尚未加载"}
		return report
	}
	report.MinFreeBytes = cfg.Storage.MinFreeBytes
	report.WarnFreePercent = cfg.Storage.WarnFreePercent
	report.MinFreePercent = cfg.Storage.MinFreePercent
	report.Root = inspectPath("主存目录", cfg.Storage.RootPath, cfg.Storage.MinFreeBytes, cfg.Storage.WarnFreePercent, cfg.Storage.MinFreePercent)
	report.Staging = inspectPath("暂存目录", cfg.Storage.StagingPath, cfg.Storage.MinFreeBytes, cfg.Storage.WarnFreePercent, cfg.Storage.MinFreePercent)
	paths := []*PathHealth{&report.Root, &report.Staging}
	if strings.EqualFold(cfg.Backup.Type, "local") && strings.TrimSpace(cfg.Backup.LocalPath) != "" {
		backup := inspectPath("本地备份目录", cfg.Backup.LocalPath, cfg.Storage.MinFreeBytes, cfg.Storage.WarnFreePercent, cfg.Storage.MinFreePercent)
		report.Backup = &backup
		paths = append(paths, report.Backup)
	}
	for _, item := range paths {
		switch {
		case !item.Available:
			report.Status = "degraded"
			report.Alerts = append(report.Alerts, fmt.Sprintf("%s不存在或不是目录", item.Name))
		case !item.Writable:
			report.Status = "degraded"
			report.Alerts = append(report.Alerts, fmt.Sprintf("%s写入探针失败", item.Name))
		case item.CriticalSpace:
			if report.Status != "degraded" {
				report.Status = "critical"
			}
			report.Alerts = append(report.Alerts, fmt.Sprintf("%s可用容量严重不足", item.Name))
		case item.LowSpace:
			if report.Status == "ready" {
				report.Status = "warning"
			}
			report.Alerts = append(report.Alerts, fmt.Sprintf("%s可用容量低于阈值", item.Name))
		}
	}
	return report
}

func inspectPath(name, rawPath string, minFreeBytes int64, warnFreePercent, minFreePercent int) PathHealth {
	path := filepath.Clean(rawPath)
	result := PathHealth{Name: name, Path: path}
	info, err := os.Stat(path)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if !info.IsDir() {
		result.Error = "path is not a directory"
		return result
	}
	result.Available = true

	file, err := os.CreateTemp(path, ".fileshare-health-*")
	if err != nil {
		result.Error = err.Error()
	} else {
		probeName := file.Name()
		_, writeErr := file.Write([]byte("fileshare-storage-health\n"))
		syncErr := file.Sync()
		closeErr := file.Close()
		removeErr := os.Remove(probeName)
		if writeErr == nil && syncErr == nil && closeErr == nil && removeErr == nil {
			result.Writable = true
		} else {
			result.Error = firstError(writeErr, syncErr, closeErr, removeErr)
		}
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		if result.Error == "" {
			result.Error = err.Error()
		}
		return result
	}
	result.TotalBytes = stat.Blocks * uint64(stat.Bsize)
	result.FreeBytes = stat.Bavail * uint64(stat.Bsize)
	if result.TotalBytes >= result.FreeBytes {
		result.UsedBytes = result.TotalBytes - result.FreeBytes
	}
	if result.TotalBytes > 0 {
		result.FreePercent = float64(result.FreeBytes) * 100 / float64(result.TotalBytes)
	}
	result.CriticalSpace, result.LowSpace = capacityState(
		result.FreeBytes, result.FreePercent, minFreeBytes, warnFreePercent, minFreePercent,
	)
	return result
}

func capacityState(freeBytes uint64, freePercent float64, minFreeBytes int64, warnFreePercent, minFreePercent int) (critical, low bool) {
	critical = (minFreeBytes > 0 && freeBytes < uint64(minFreeBytes)) ||
		(minFreePercent > 0 && freePercent < float64(minFreePercent))
	low = critical || (warnFreePercent > 0 && freePercent < float64(warnFreePercent))
	return critical, low
}

func firstError(errors ...error) string {
	for _, err := range errors {
		if err != nil {
			return err.Error()
		}
	}
	return "storage write probe failed"
}

type Monitor struct {
	mu         sync.Mutex
	lastStatus string
}

func NewMonitor() *Monitor { return &Monitor{} }

func (m *Monitor) Start(ctx context.Context) {
	cfg := config.GetConfig()
	if cfg == nil {
		return
	}
	interval := time.Duration(cfg.Storage.HealthCheckIntervalMinutes) * time.Minute
	go func() {
		m.checkAndLog(cfg)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.checkAndLog(cfg)
			}
		}
	}()
}

func (m *Monitor) checkAndLog(cfg *config.Config) {
	report := Check(cfg)
	m.mu.Lock()
	previous := m.lastStatus
	m.lastStatus = report.Status
	m.mu.Unlock()
	if report.Status == previous {
		return
	}
	if report.Status == "ready" {
		if previous != "" {
			logger.Info("storage_health_recovered", "previous_status", previous)
			m.publish(report, previous, true)
		}
		return
	}
	logger.Warn("storage_health_alert", "status", report.Status, "alerts", report.Alerts,
		"root_free_bytes", report.Root.FreeBytes, "root_free_percent", report.Root.FreePercent,
		"staging_free_bytes", report.Staging.FreeBytes, "staging_free_percent", report.Staging.FreePercent)
	m.publish(report, previous, false)
}

func (m *Monitor) publish(report Report, previous string, recovered bool) {
	severity, title := "warning", "存储健康告警"
	content := strings.Join(report.Alerts, "；")
	if report.Status == "critical" || report.Status == "degraded" {
		severity = "critical"
	}
	if recovered {
		severity, title = "info", "存储健康已恢复"
		content = fmt.Sprintf("存储状态已从 %s 恢复为 ready", previous)
	}
	_, err := notification.Publish(context.Background(), notification.Event{
		Key: fmt.Sprintf("storage-health:%s:%d", report.Status, report.CheckedAt.Unix()), Type: "storage:health",
		Severity: severity, Title: title, Content: content,
		Payload: map[string]any{"status": report.Status, "previous_status": previous},
	})
	if err != nil {
		logger.Error("storage_health_notification_enqueue_failed", "error", err)
	}
}
