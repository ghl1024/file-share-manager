/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package logger

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestSanitizeTextRemovesSensitiveValues(t *testing.T) {
	input := `Authorization: Bearer abc.def.ghi Cookie: fileshare_session=session-token password="plain" token=share-token path=/api/fileshare/v1/share/Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA/download json={"password":"secret","token":"raw"}`
	sanitized := SanitizeText(input)

	for _, forbidden := range []string{"abc.def.ghi", "session-token", "plain", "share-token", "secret", "raw", "Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA"} {
		if strings.Contains(sanitized, forbidden) {
			t.Fatalf("sanitized text still contains %q: %s", forbidden, sanitized)
		}
	}
	for _, expected := range []string{"Authorization: Bearer [REDACTED]", "fileshare_session=[REDACTED]", "password=[REDACTED]", "/share/:token"} {
		if !strings.Contains(sanitized, expected) {
			t.Fatalf("sanitized text missing %q: %s", expected, sanitized)
		}
	}
}

func TestLoggerRedactsSensitiveFieldsAndErrors(t *testing.T) {
	directory := t.TempDir()
	if err := Init(Config{
		Level: "debug", Directory: directory, Format: "json", SplitByLevel: true,
		RetentionDays: 7, RotationTime: "day", ServiceName: "test-server",
	}); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}
	Get().Info("login password=secret-token",
		zap.String("authorization", "Bearer real-jwt-token"),
		zap.String("path", "/api/fileshare/v1/share/Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA/download"),
		zap.Error(errors.New("mysql password=db-secret")),
		zap.ByteString("details", []byte(`{"token":"raw-token"}`)),
	)
	Sync()

	events := readLogEvents(t, directory, "info")
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1: %#v", len(events), events)
	}
	serialized, err := json.Marshal(events[0])
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	logLine := string(serialized)
	for _, forbidden := range []string{"secret-token", "real-jwt-token", "db-secret", "raw-token", "Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA"} {
		if strings.Contains(logLine, forbidden) {
			t.Fatalf("log still contains %q: %s", forbidden, logLine)
		}
	}
	if events[0]["authorization"] != redactedValue {
		t.Fatalf("authorization field was not fully redacted: %#v", events[0])
	}
}

func readLogEvents(t *testing.T, directory, level string) []map[string]any {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(directory, level+"-*.log"))
	if err != nil || len(files) != 1 {
		t.Fatalf("%s files = %v, err = %v", level, files, err)
	}
	file, err := os.Open(files[0])
	if err != nil {
		t.Fatalf("open %s log: %v", level, err)
	}
	defer file.Close()

	var events []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var event map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			t.Fatalf("decode %s log as NDJSON: %v", level, err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan %s log: %v", level, err)
	}
	return events
}
