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
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

func TestSplitJSONLogsAndContextCorrelation(t *testing.T) {
	directory := t.TempDir()
	if err := Init(Config{
		Level: "debug", Directory: directory, Format: "json", SplitByLevel: true,
		RetentionDays: 7, RotationTime: "day", ServiceName: "test-server",
		ServiceVersion: "v1", ServiceInstanceID: "instance-1", Environment: "test",
	}); err != nil {
		t.Fatalf("initialize logger: %v", err)
	}

	spanContext := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1, 2, 3}, SpanID: trace.SpanID{4, 5, 6}, TraceFlags: trace.FlagsSampled,
	})
	ctx := trace.ContextWithSpanContext(context.Background(), spanContext)
	Get().Debug("debug_event")
	FromContext(ctx).Info("info_event", zap.String("operation", "test"))
	Get().Warn("warn_event")
	Get().Error("error_event")
	if err := SetLevel("warn"); err != nil {
		t.Fatalf("set level: %v", err)
	}
	Get().Info("suppressed_info")
	Get().Warn("second_warn")
	Sync()

	assertEvents(t, directory, "debug", []string{"debug_event"})
	infoEvents := assertEvents(t, directory, "info", []string{"info_event"})
	if infoEvents[0]["trace_id"] != spanContext.TraceID().String() || infoEvents[0]["span_id"] != spanContext.SpanID().String() {
		t.Fatalf("trace correlation fields missing: %#v", infoEvents[0])
	}
	if infoEvents[0]["service"] != "test-server" {
		t.Fatalf("service resource fields missing: %#v", infoEvents[0])
	}
	assertEvents(t, directory, "warn", []string{"warn_event", "second_warn"})
	assertEvents(t, directory, "error", []string{"error_event"})
}

func assertEvents(t *testing.T, directory, level string, expectedBodies []string) []map[string]any {
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
	if len(events) != len(expectedBodies) {
		t.Fatalf("%s event count = %d, want %d: %#v", level, len(events), len(expectedBodies), events)
	}
	for index, message := range expectedBodies {
		if events[index]["msg"] != message {
			t.Fatalf("%s event %d msg = %v, want %s", level, index, events[index]["msg"], message)
		}
		if events[index]["level"] != strings.ToUpper(level) {
			t.Fatalf("%s event %d level = %v", level, index, events[index]["level"])
		}
		for _, key := range []string{"time", "level", "msg", "caller", "service", "version", "instance", "env"} {
			if _, exists := events[index][key]; !exists {
				t.Fatalf("%s event %d missing field %s: %#v", level, index, key, events[index])
			}
		}
		for key := range events[index] {
			if strings.Contains(key, ".") {
				t.Fatalf("%s event %d contains dotted field %s", level, index, key)
			}
		}
	}
	return events
}
