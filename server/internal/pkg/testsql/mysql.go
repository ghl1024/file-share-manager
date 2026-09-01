/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

// Package testsql contains small helpers for integration-test SQL that cannot
// use query parameters for identifiers such as temporary database names.
package testsql

import (
	"fmt"
	"regexp"
	"strings"
)

var mysqlIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9_]+$`)

// Identifier validates and quotes a MySQL identifier before it is embedded in
// DDL. Test callers only pass locally generated names, but validation keeps a
// future change from turning these statements into an injection sink.
func Identifier(value string) (string, error) {
	if !mysqlIdentifierPattern.MatchString(value) {
		return "", fmt.Errorf("invalid MySQL identifier %q", value)
	}
	return "`" + value + "`", nil
}

// Literal quotes a value for a MySQL string literal. It is used only for
// integration-test credentials where MySQL does not accept a parameter marker.
func Literal(value string) string {
	escaped := strings.NewReplacer(
		"\\", "\\\\",
		"'", "''",
		"\x00", "\\0",
		"\n", "\\n",
		"\r", "\\r",
		"\x1a", "\\Z",
	).Replace(value)
	return "'" + escaped + "'"
}
