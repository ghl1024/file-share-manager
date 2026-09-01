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
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	rotatelogs "github.com/lestrrat-go/file-rotatelogs"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	baseLogger  = zap.NewNop()
	sugar       = baseLogger.Sugar()
	atomicLevel = zap.NewAtomicLevelAt(zapcore.InfoLevel)
)

// Config controls structured log output. Filename and MaxAge are retained for
// compatibility; Directory and RetentionDays are preferred.
type Config struct {
	Level             string
	Filename          string
	Directory         string
	Format            string
	SplitByLevel      bool
	MaxSize           int
	MaxBackups        int
	MaxAge            int
	RetentionDays     int
	Compress          bool
	Console           bool
	RotationTime      string
	ReloadOnSIGHUP    bool
	ServiceName       string
	ServiceVersion    string
	ServiceInstanceID string
	Environment       string
}

// Init initializes NDJSON file logging and an optional NDJSON console sink.
func Init(cfg Config) error {
	level, err := parseLevel(cfg.Level)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(level)

	directory := strings.TrimSpace(cfg.Directory)
	if directory == "" {
		directory = legacyDirectory(cfg.Filename, cfg.ServiceName)
	}
	if directory == "" {
		return fmt.Errorf("log directory is required")
	}
	if err := os.MkdirAll(directory, 0750); err != nil {
		return fmt.Errorf("create log directory %s: %w", directory, err)
	}

	retentionDays := cfg.RetentionDays
	if retentionDays <= 0 {
		retentionDays = cfg.MaxAge
	}
	if retentionDays <= 0 {
		retentionDays = 7
	}
	rotation, pattern, err := rotationSettings(cfg.RotationTime)
	if err != nil {
		return err
	}

	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.TimeKey = "time"
	encoderConfig.LevelKey = "level"
	encoderConfig.MessageKey = "msg"
	encoderConfig.CallerKey = "caller"
	encoderConfig.NameKey = "logger"
	encoderConfig.EncodeTime = zapcore.RFC3339NanoTimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	encoderConfig.EncodeDuration = zapcore.MillisDurationEncoder
	encoder := zapcore.NewJSONEncoder(encoderConfig)

	cores := make([]zapcore.Core, 0, 5)
	if cfg.SplitByLevel {
		for _, output := range []struct {
			name    string
			enabler zapcore.LevelEnabler
		}{
			{name: "debug", enabler: exactLevel(zapcore.DebugLevel)},
			{name: "info", enabler: exactLevel(zapcore.InfoLevel)},
			{name: "warn", enabler: exactLevel(zapcore.WarnLevel)},
			{name: "error", enabler: errorLevel()},
		} {
			writer, writerErr := rotatingWriter(directory, output.name, pattern, rotation, retentionDays)
			if writerErr != nil {
				return writerErr
			}
			cores = append(cores, newSanitizingCore(zapcore.NewCore(encoder, writer, output.enabler)))
		}
	} else {
		name := strings.TrimSuffix(filepath.Base(cfg.Filename), filepath.Ext(cfg.Filename))
		if name == "" || name == "." {
			name = "application"
		}
		writer, writerErr := rotatingWriter(directory, name, pattern, rotation, retentionDays)
		if writerErr != nil {
			return writerErr
		}
		cores = append(cores, newSanitizingCore(zapcore.NewCore(encoder, writer, atomicLevel)))
	}

	if cfg.Console {
		cores = append(cores, newSanitizingCore(zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), atomicLevel)))
	}

	fields := []zap.Field{
		zap.String("service", defaultString(cfg.ServiceName, "fileshare-server")),
		zap.String("version", defaultString(cfg.ServiceVersion, "unknown")),
		zap.String("instance", defaultString(cfg.ServiceInstanceID, "unknown")),
		zap.String("env", defaultString(cfg.Environment, "unknown")),
	}
	baseLogger = zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.ErrorOutput(zapcore.AddSync(os.Stderr))).With(fields...)
	sugar = baseLogger.Sugar()
	return nil
}

func rotatingWriter(directory, levelName, pattern string, rotation time.Duration, retentionDays int) (zapcore.WriteSyncer, error) {
	writer, err := rotatelogs.New(
		filepath.Join(directory, levelName+"-"+pattern+".log"),
		rotatelogs.WithRotationTime(rotation),
		rotatelogs.WithMaxAge(time.Duration(retentionDays)*24*time.Hour),
	)
	if err != nil {
		return nil, fmt.Errorf("initialize %s log writer: %w", levelName, err)
	}
	return zapcore.AddSync(writer), nil
}

func exactLevel(target zapcore.Level) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level == target && atomicLevel.Enabled(level)
	})
}

func errorLevel() zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(level zapcore.Level) bool {
		return level >= zapcore.ErrorLevel && atomicLevel.Enabled(level)
	})
}

func rotationSettings(value string) (time.Duration, string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "day":
		return 24 * time.Hour, "%Y-%m-%d", nil
	case "hour":
		return time.Hour, "%Y-%m-%d-%H", nil
	default:
		return 0, "", fmt.Errorf("log rotation_time must be day or hour")
	}
}

func legacyDirectory(filename, serviceName string) string {
	if strings.TrimSpace(filename) == "" {
		return ""
	}
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	if base == "" || base == "." {
		base = defaultString(serviceName, "application")
	}
	return filepath.Join(filepath.Dir(filename), base)
}

func parseLevel(value string) (zapcore.Level, error) {
	var level zapcore.Level
	if err := level.UnmarshalText([]byte(defaultString(strings.TrimSpace(value), "info"))); err != nil {
		return zapcore.InfoLevel, fmt.Errorf("invalid log level %q: %w", value, err)
	}
	return level, nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// SetLevel atomically changes the minimum emitted level without rebuilding sinks.
func SetLevel(value string) error {
	level, err := parseLevel(value)
	if err != nil {
		return err
	}
	atomicLevel.SetLevel(level)
	return nil
}

func Level() string { return atomicLevel.Level().String() }

// FromContext returns a logger correlated with the active OpenTelemetry span.
func FromContext(ctx context.Context) *zap.Logger {
	if ctx == nil {
		return baseLogger
	}
	spanContext := trace.SpanContextFromContext(ctx)
	if !spanContext.IsValid() {
		return baseLogger
	}
	return baseLogger.With(
		zap.String("trace_id", spanContext.TraceID().String()),
		zap.String("span_id", spanContext.SpanID().String()),
	)
}

func Get() *zap.Logger { return baseLogger }

func Sync() {
	_ = baseLogger.Sync()
}

func Debugf(template string, args ...interface{}) { sugar.Debugf(template, args...) }
func Infof(template string, args ...interface{})  { sugar.Infof(template, args...) }
func Warnf(template string, args ...interface{})  { sugar.Warnf(template, args...) }
func Errorf(template string, args ...interface{}) { sugar.Errorf(template, args...) }
func Debug(message string, fields ...interface{}) { sugar.Debugw(message, fields...) }
func Info(message string, fields ...interface{})  { sugar.Infow(message, fields...) }
func Warn(message string, fields ...interface{})  { sugar.Warnw(message, fields...) }
func Error(message string, fields ...interface{}) { sugar.Errorw(message, fields...) }
func Fatalf(template string, args ...interface{}) { sugar.Fatalf(template, args...) }
