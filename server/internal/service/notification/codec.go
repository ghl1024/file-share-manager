/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package notification

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"file-share-manager/server/internal/config"
)

var credentialEnvelopeMagic = []byte("FSMNOTIFY1")

var ErrCredentialKeyMissing = errors.New("notification credential encryption key is not configured")

type ChannelSettings struct {
	WebhookURL     string   `json:"webhook_url,omitempty"`
	Secret         string   `json:"secret,omitempty"`
	SMTPHost       string   `json:"smtp_host,omitempty"`
	SMTPPort       int      `json:"smtp_port,omitempty"`
	SMTPEncryption string   `json:"smtp_encryption,omitempty"`
	SMTPUsername   string   `json:"smtp_username,omitempty"`
	SMTPPassword   string   `json:"smtp_password,omitempty"`
	SMTPFrom       string   `json:"smtp_from,omitempty"`
	SMTPRecipients []string `json:"smtp_recipients,omitempty"`
}

func EncryptSettings(settings ChannelSettings) (string, error) {
	key, err := credentialKey()
	if err != nil {
		return "", err
	}
	payload, err := json.Marshal(settings)
	if err != nil {
		return "", err
	}
	gcm, err := credentialGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, payload, credentialEnvelopeMagic)
	envelope := make([]byte, 0, len(credentialEnvelopeMagic)+len(nonce)+len(ciphertext))
	envelope = append(envelope, credentialEnvelopeMagic...)
	envelope = append(envelope, nonce...)
	envelope = append(envelope, ciphertext...)
	return base64.StdEncoding.EncodeToString(envelope), nil
}

func DecryptSettings(value string) (ChannelSettings, error) {
	var settings ChannelSettings
	key, err := credentialKey()
	if err != nil {
		return settings, err
	}
	envelope, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return settings, errors.New("notification credential envelope is invalid")
	}
	gcm, err := credentialGCM(key)
	if err != nil {
		return settings, err
	}
	minimum := len(credentialEnvelopeMagic) + gcm.NonceSize() + gcm.Overhead()
	if len(envelope) < minimum || string(envelope[:len(credentialEnvelopeMagic)]) != string(credentialEnvelopeMagic) {
		return settings, errors.New("notification credential envelope is invalid")
	}
	nonce := envelope[len(credentialEnvelopeMagic) : len(credentialEnvelopeMagic)+gcm.NonceSize()]
	ciphertext := envelope[len(credentialEnvelopeMagic)+gcm.NonceSize():]
	payload, err := gcm.Open(nil, nonce, ciphertext, credentialEnvelopeMagic)
	if err != nil {
		return settings, errors.New("notification credential authentication failed")
	}
	if err := json.Unmarshal(payload, &settings); err != nil {
		return settings, errors.New("notification credential payload is invalid")
	}
	return settings, nil
}

func credentialKey() ([]byte, error) {
	cfg := config.GetConfig()
	if cfg == nil || strings.TrimSpace(cfg.Notification.CredentialEncryptionKey) == "" {
		return nil, ErrCredentialKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cfg.Notification.CredentialEncryptionKey))
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("notification credential key must contain 32 bytes")
	}
	return key, nil
}

func credentialGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
