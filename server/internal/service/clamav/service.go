/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package clamav

import (
	"bufio"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
)

const (
	StatusClean     = "clean"
	StatusInfected  = "infected"
	StatusScanError = "scan_error"
	StatusUnscanned = "unscanned"
	maxStreamChunk  = 32 * 1024
)

type Result struct {
	Status  string
	Message string
}

type Health struct {
	Reachable          bool        `json:"reachable"`
	Status             string      `json:"status"`
	EngineVersion      string      `json:"engine_version,omitempty"`
	VirusDBVersion     string      `json:"virus_db_version,omitempty"`
	VirusDBUpdatedAt   *time.Time  `json:"virus_db_updated_at,omitempty"`
	VirusDBAgeHours    *int        `json:"virus_db_age_hours,omitempty"`
	VirusDBMaxAgeHours int         `json:"virus_db_max_age_hours"`
	VirusDBStale       bool        `json:"virus_db_stale"`
	Response           string      `json:"response,omitempty"`
	CheckedAt          time.Time   `json:"checked_at"`
	Retry              RetryStatus `json:"retry"`
}

type Scanner struct {
	dial    func(ctx context.Context, network, address string) (net.Conn, error)
	addr    string
	timeout time.Duration
}

func NewFromConfig(cfg config.ClamAVConfig) *Scanner {
	return &Scanner{
		dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, address)
		},
		addr:    fmt.Sprintf("%s:%d", strings.TrimSpace(cfg.Host), cfg.Port),
		timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
	}
}

func ScanFile(ctx context.Context, path string) Result {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.ClamAV.Enabled() {
		return Result{Status: StatusUnscanned, Message: "ClamAV 未配置"}
	}
	return NewFromConfig(cfg.ClamAV).ScanFile(ctx, path)
}

func CheckHealth(ctx context.Context) (Health, error) {
	cfg := config.GetConfig()
	if cfg == nil || !cfg.ClamAV.Enabled() {
		return Health{CheckedAt: time.Now()}, errors.New("ClamAV 未配置")
	}
	health, err := NewFromConfig(cfg.ClamAV).Health(ctx, cfg.ClamAV.VirusDBMaxAgeHours)
	if err != nil {
		return health, err
	}
	health.Retry, err = CurrentRetryStatus()
	return health, err
}

func (s *Scanner) Ping(ctx context.Context) (Health, error) {
	health := Health{CheckedAt: time.Now(), Status: "unreachable"}
	response, err := s.command(ctx, "PING")
	if err != nil {
		return health, err
	}
	health.Response = response
	health.Reachable = response == "PONG"
	if !health.Reachable {
		return health, errors.New("ClamAV PING 响应异常")
	}
	health.Status = "healthy"
	return health, nil
}

func (s *Scanner) Health(ctx context.Context, maxAgeHours int) (Health, error) {
	health := Health{CheckedAt: time.Now(), Status: "unreachable", VirusDBMaxAgeHours: maxAgeHours}
	response, err := s.command(ctx, "VERSION")
	if err != nil {
		return health, err
	}
	health.Response = response
	engine, databaseVersion, updatedAt, err := parseVersionResponse(response)
	if err != nil {
		return health, err
	}
	health.Reachable = true
	health.Status = "healthy"
	health.EngineVersion = engine
	health.VirusDBVersion = databaseVersion
	health.VirusDBUpdatedAt = updatedAt
	if updatedAt != nil {
		age := int(time.Since(*updatedAt).Hours())
		if age < 0 {
			age = 0
		}
		health.VirusDBAgeHours = &age
		health.VirusDBStale = maxAgeHours > 0 && age > maxAgeHours
		if health.VirusDBStale {
			health.Status = "stale"
		}
	}
	return health, nil
}

func (s *Scanner) command(ctx context.Context, command string) (string, error) {
	if s == nil || s.dial == nil || s.addr == ":0" {
		return "", errors.New("ClamAV 配置无效")
	}
	timeout := s.timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	conn, err := s.dial(pingCtx, "tcp", s.addr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := io.WriteString(conn, "z"+command+"\x00"); err != nil {
		return "", err
	}
	response, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(strings.TrimSuffix(response, "\x00")), nil
}

func parseVersionResponse(response string) (string, string, *time.Time, error) {
	parts := strings.Split(strings.TrimSpace(response), "/")
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "ClamAV ") {
		return "", "", nil, errors.New("ClamAV VERSION 响应异常")
	}
	engine := strings.TrimSpace(strings.TrimPrefix(parts[0], "ClamAV "))
	databaseVersion := strings.TrimSpace(parts[1])
	if engine == "" || databaseVersion == "" {
		return "", "", nil, errors.New("ClamAV VERSION 响应缺少版本信息")
	}
	if len(parts) < 3 || strings.TrimSpace(parts[2]) == "" {
		return engine, databaseVersion, nil, nil
	}
	databaseTime := strings.Join(parts[2:], "/")
	for _, layout := range []string{time.ANSIC, "Mon Jan _2 15:04:05 2006", time.RFC1123, time.RFC1123Z} {
		parsed, err := time.ParseInLocation(layout, strings.TrimSpace(databaseTime), time.Local)
		if err == nil {
			return engine, databaseVersion, &parsed, nil
		}
	}
	return engine, databaseVersion, nil, nil
}

func (s *Scanner) ScanFile(ctx context.Context, path string) Result {
	if s == nil || s.dial == nil || s.addr == ":0" {
		return Result{Status: StatusScanError, Message: "ClamAV 配置无效"}
	}
	file, err := os.Open(path)
	if err != nil {
		return Result{Status: StatusScanError, Message: "读取待扫描文件失败"}
	}
	defer file.Close()
	if s.timeout <= 0 {
		s.timeout = 60 * time.Second
	}
	scanCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	conn, err := s.dial(scanCtx, "tcp", s.addr)
	if err != nil {
		return Result{Status: StatusScanError, Message: "连接 ClamAV 失败"}
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(s.timeout))
	if _, err := io.WriteString(conn, "zINSTREAM\x00"); err != nil {
		return Result{Status: StatusScanError, Message: "发送 ClamAV 扫描请求失败"}
	}
	buffer := make([]byte, maxStreamChunk)
	for {
		read, readErr := file.Read(buffer)
		if read > 0 {
			var length [4]byte
			binary.BigEndian.PutUint32(length[:], uint32(read))
			if _, err := conn.Write(length[:]); err != nil {
				return Result{Status: StatusScanError, Message: "发送文件扫描数据失败"}
			}
			if _, err := conn.Write(buffer[:read]); err != nil {
				return Result{Status: StatusScanError, Message: "发送文件扫描数据失败"}
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return Result{Status: StatusScanError, Message: "读取文件扫描数据失败"}
		}
	}
	if _, err := conn.Write([]byte{0, 0, 0, 0}); err != nil {
		return Result{Status: StatusScanError, Message: "结束 ClamAV 扫描失败"}
	}
	line, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil {
		return Result{Status: StatusScanError, Message: "读取 ClamAV 扫描结果失败"}
	}
	line = strings.TrimSpace(strings.TrimSuffix(line, "\x00"))
	switch {
	case strings.HasSuffix(line, "OK"):
		return Result{Status: StatusClean, Message: line}
	case strings.Contains(line, "FOUND"):
		return Result{Status: StatusInfected, Message: line}
	default:
		return Result{Status: StatusScanError, Message: line}
	}
}
