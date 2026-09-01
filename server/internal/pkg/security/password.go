/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package security

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	MinPasswordLength = 12
	MaxPasswordLength = 128
)

var commonPasswords = map[string]struct{}{
	"12345678": {}, "123456789": {}, "admin123": {}, "password": {},
	"password123": {}, "qwerty123": {}, "letmein": {},
}

func ValidatePassword(password string) error {
	length := utf8.RuneCountInString(password)
	if length < MinPasswordLength || length > MaxPasswordLength {
		return fmt.Errorf("密码长度必须在 %d 到 %d 个字符之间", MinPasswordLength, MaxPasswordLength)
	}
	if _, found := commonPasswords[strings.ToLower(password)]; found {
		return fmt.Errorf("密码过于常见")
	}

	classes := 0
	var lower, upper, digit, symbol bool
	for _, char := range password {
		switch {
		case unicode.IsLower(char):
			lower = true
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsDigit(char):
			digit = true
		case unicode.IsSpace(char):
			return fmt.Errorf("密码不能包含空白字符")
		default:
			symbol = true
		}
	}
	for _, present := range []bool{lower, upper, digit, symbol} {
		if present {
			classes++
		}
	}
	if classes < 3 {
		return fmt.Errorf("密码必须包含大小写字母、数字、特殊字符中的至少三类")
	}
	return nil
}

// HashPassword hashes a password using bcrypt
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	return string(bytes), err
}

// CheckPasswordHash checks if a password matches a bcrypt hash
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}
