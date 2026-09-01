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
	"regexp"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const redactedValue = "[REDACTED]"

var (
	sensitiveKeyParts = []string{
		"authorization", "cookie", "jwt", "token", "secret", "password",
		"passwd", "credential", "session", "fileshare_session",
	}
	bearerPattern       = regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/=-]+`)
	cookiePattern       = regexp.MustCompile(`(?i)\b(fileshare_session|fileshare_share_access)=([^;\s]+)`)
	keyValuePattern     = regexp.MustCompile(`(?i)\b(password|passwd|token|jwt|secret|session)(\s*[:=]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
	jsonKeyValuePattern = regexp.MustCompile(`(?i)"(password|passwd|token|jwt|secret|authorization|cookie|session)"\s*:\s*"[^"]*"`)
	sharePathPattern    = regexp.MustCompile(`/share/[A-Za-z0-9_-]{16,}`)
)

type sanitizingCore struct {
	zapcore.Core
}

func newSanitizingCore(core zapcore.Core) zapcore.Core {
	return sanitizingCore{Core: core}
}

func (core sanitizingCore) With(fields []zap.Field) zapcore.Core {
	return sanitizingCore{Core: core.Core.With(sanitizeFields(fields))}
}

func (core sanitizingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}
	return checked
}

func (core sanitizingCore) Write(entry zapcore.Entry, fields []zap.Field) error {
	entry.Message = SanitizeText(entry.Message)
	return core.Core.Write(entry, sanitizeFields(fields))
}

func sanitizeFields(fields []zap.Field) []zap.Field {
	if len(fields) == 0 {
		return fields
	}
	sanitized := make([]zap.Field, len(fields))
	for index, field := range fields {
		sanitized[index] = sanitizeField(field)
	}
	return sanitized
}

func sanitizeField(field zap.Field) zap.Field {
	if IsSensitiveKey(field.Key) {
		return zap.String(field.Key, redactedValue)
	}
	switch field.Type {
	case zapcore.StringType:
		field.String = SanitizeText(field.String)
	case zapcore.ByteStringType:
		if bytes, ok := field.Interface.([]byte); ok {
			field.Interface = []byte(SanitizeText(string(bytes)))
		}
	case zapcore.ErrorType:
		if err, ok := field.Interface.(error); ok && err != nil {
			return zap.String(field.Key, SanitizeText(err.Error()))
		}
	case zapcore.StringerType:
		if stringer, ok := field.Interface.(interface{ String() string }); ok && stringer != nil {
			return zap.String(field.Key, SanitizeText(stringer.String()))
		}
	}
	return field
}

func IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "-", "_"))
	for _, part := range sensitiveKeyParts {
		if strings.Contains(normalized, part) {
			return true
		}
	}
	return false
}

func SanitizeText(value string) string {
	if value == "" {
		return value
	}
	value = bearerPattern.ReplaceAllString(value, "Bearer "+redactedValue)
	value = cookiePattern.ReplaceAllString(value, "$1="+redactedValue)
	value = jsonKeyValuePattern.ReplaceAllString(value, `"$1":"`+redactedValue+`"`)
	value = keyValuePattern.ReplaceAllString(value, "$1$2"+redactedValue)
	value = sharePathPattern.ReplaceAllString(value, "/share/:token")
	return value
}
