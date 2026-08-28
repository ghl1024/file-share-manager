/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package backup

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

var manifestEnvelopeMagic = []byte("FSMANIFEST\x01")

const maxManifestPlaintextBytes = 256 << 20

var ErrManifestEncryptionKeyMissing = errors.New("backup manifest encryption key is not configured")

func encodeProtectedManifest(manifest Manifest, encodedKey string) ([]byte, string, error) {
	plaintext, hash, err := encodeFullManifest(manifest)
	if err != nil {
		return nil, "", err
	}
	key, err := decodeManifestEncryptionKey(encodedKey)
	if err != nil {
		return nil, "", err
	}

	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write(plaintext); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}

	gcm, err := newManifestGCM(key)
	if err != nil {
		return nil, "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, "", fmt.Errorf("generate manifest nonce: %w", err)
	}
	envelope := make([]byte, 0, len(manifestEnvelopeMagic)+len(nonce)+compressed.Len()+gcm.Overhead())
	envelope = append(envelope, manifestEnvelopeMagic...)
	envelope = append(envelope, nonce...)
	envelope = gcm.Seal(envelope, nonce, compressed.Bytes(), manifestEnvelopeMagic)
	return envelope, hash, nil
}

func decodeProtectedManifest(data []byte, encodedKey string) (Manifest, error) {
	if !bytes.HasPrefix(data, manifestEnvelopeMagic) {
		// Backups created before encrypted manifests remain readable.
		return decodeFullManifest(data)
	}
	key, err := decodeManifestEncryptionKey(encodedKey)
	if err != nil {
		return Manifest{}, err
	}
	gcm, err := newManifestGCM(key)
	if err != nil {
		return Manifest{}, err
	}
	payload := data[len(manifestEnvelopeMagic):]
	if len(payload) < gcm.NonceSize()+gcm.Overhead() {
		return Manifest{}, errors.New("encrypted manifest is truncated")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	compressed, err := gcm.Open(nil, nonce, ciphertext, manifestEnvelopeMagic)
	if err != nil {
		return Manifest{}, errors.New("encrypted manifest authentication failed")
	}

	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return Manifest{}, errors.New("encrypted manifest compression is invalid")
	}
	plaintext, readErr := io.ReadAll(io.LimitReader(reader, maxManifestPlaintextBytes+1))
	closeErr := reader.Close()
	if readErr != nil {
		return Manifest{}, readErr
	}
	if closeErr != nil {
		return Manifest{}, closeErr
	}
	if len(plaintext) > maxManifestPlaintextBytes {
		return Manifest{}, errors.New("decompressed manifest exceeds size limit")
	}
	return decodeFullManifest(plaintext)
}

func decodeManifestEncryptionKey(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, ErrManifestEncryptionKeyMissing
	}
	key, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(key) != 32 {
		return nil, errors.New("backup manifest encryption key must be base64-encoded 32 bytes")
	}
	return key, nil
}

func newManifestGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
