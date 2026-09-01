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
	crypto_rand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// RandomString 使用 crypto/rand 生成密码学安全的随机字符串
func RandomString(n int) string {
	b := make([]byte, n)
	if _, err := crypto_rand.Read(b); err != nil {
		panic("crypto/rand.Read failed: " + err.Error())
	}
	for i := range b {
		b[i] = letterBytes[b[i]%byte(len(letterBytes))]
	}
	return string(b)
}

func SHA256(text string) string {
	hash := sha256.Sum256([]byte(text))
	return hex.EncodeToString(hash[:])
}
