/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrInvalidObjectKey = errors.New("invalid storage object key")

type POSIX struct {
	rootPath    string
	stagingPath string
}

type MergeResult struct {
	StorageKey string
	Size       int64
	SHA256     string
}

func NewPOSIX(rootPath, stagingPath string) (*POSIX, error) {
	if strings.TrimSpace(rootPath) == "" || strings.TrimSpace(stagingPath) == "" {
		return nil, errors.New("storage root and staging paths are required")
	}
	if err := os.MkdirAll(rootPath, 0o750); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(stagingPath, 0o750); err != nil {
		return nil, err
	}
	return &POSIX{rootPath: filepath.Clean(rootPath), stagingPath: filepath.Clean(stagingPath)}, nil
}

func (s *POSIX) EnsureUpload(uploadID string) error {
	if err := validateID(uploadID); err != nil {
		return err
	}
	return os.MkdirAll(filepath.Join(s.stagingPath, uploadID), 0o750)
}

func (s *POSIX) WritePart(uploadID string, partNo int, src io.Reader, expectedSize int64) (int64, error) {
	if err := validateID(uploadID); err != nil {
		return 0, err
	}
	if partNo < 0 {
		return 0, errors.New("part number must not be negative")
	}
	if expectedSize <= 0 {
		return 0, errors.New("part size must be positive")
	}
	if err := s.EnsureUpload(uploadID); err != nil {
		return 0, err
	}
	partPath := s.partPath(uploadID, partNo)
	if info, err := os.Stat(partPath); err == nil {
		if info.Size() != expectedSize {
			return info.Size(), fmt.Errorf("existing part size mismatch: got %d, want %d", info.Size(), expectedSize)
		}
		return info.Size(), nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	tmpPath := partPath + ".tmp-" + uuid.NewString()
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(file, io.LimitReader(src, sizeProbeLimit(expectedSize)))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return 0, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return 0, closeErr
	}
	if written != expectedSize {
		_ = os.Remove(tmpPath)
		return written, fmt.Errorf("part size mismatch: got %d, want %d", written, expectedSize)
	}
	if err := os.Rename(tmpPath, partPath); err != nil {
		_ = os.Remove(tmpPath)
		return 0, err
	}
	return written, nil
}

func (s *POSIX) PartExists(uploadID string, partNo int) (bool, int64, error) {
	if err := validateID(uploadID); err != nil {
		return false, 0, err
	}
	info, err := os.Stat(s.partPath(uploadID, partNo))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, 0, nil
		}
		return false, 0, err
	}
	return true, info.Size(), nil
}

func (s *POSIX) Merge(uploadID string, workspaceID uint, totalChunks int, expectedTotal int64, expectedSHA256 string) (MergeResult, error) {
	if err := validateID(uploadID); err != nil {
		return MergeResult{}, err
	}
	if totalChunks <= 0 || expectedTotal <= 0 {
		return MergeResult{}, errors.New("invalid upload dimensions")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if expectedSHA256 != "" {
		if len(expectedSHA256) != sha256.Size*2 {
			return MergeResult{}, errors.New("sha256 must be 64 hexadecimal characters")
		}
		if _, err := hex.DecodeString(expectedSHA256); err != nil {
			return MergeResult{}, errors.New("sha256 must be hexadecimal")
		}
	}
	objectKey := filepath.ToSlash(filepath.Join("objects", strconv.FormatUint(uint64(workspaceID), 10), uuid.NewString()))
	objectPath, err := s.objectPath(objectKey)
	if err != nil {
		return MergeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o750); err != nil {
		return MergeResult{}, err
	}
	tmpPath := objectPath + ".tmp-" + uuid.NewString()
	destination, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return MergeResult{}, err
	}
	hash := sha256.New()
	writer := io.MultiWriter(destination, hash)
	var total int64
	for partNo := 0; partNo < totalChunks; partNo++ {
		part, err := os.Open(s.partPath(uploadID, partNo))
		if err != nil {
			_ = destination.Close()
			_ = os.Remove(tmpPath)
			return MergeResult{}, fmt.Errorf("open part %d: %w", partNo, err)
		}
		written, copyErr := io.Copy(writer, part)
		closeErr := part.Close()
		if copyErr != nil {
			_ = destination.Close()
			_ = os.Remove(tmpPath)
			return MergeResult{}, copyErr
		}
		if closeErr != nil {
			_ = destination.Close()
			_ = os.Remove(tmpPath)
			return MergeResult{}, closeErr
		}
		if written < 0 || total > maxInt64-written {
			_ = destination.Close()
			_ = os.Remove(tmpPath)
			return MergeResult{}, errors.New("merged object size overflow")
		}
		total += written
	}
	if err := destination.Sync(); err != nil {
		_ = destination.Close()
		_ = os.Remove(tmpPath)
		return MergeResult{}, err
	}
	if err := destination.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return MergeResult{}, err
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if total != expectedTotal || (expectedSHA256 != "" && actualSHA256 != expectedSHA256) {
		_ = os.Remove(tmpPath)
		return MergeResult{}, fmt.Errorf("merged object validation failed: size=%d sha256=%s", total, actualSHA256)
	}
	if err := os.Rename(tmpPath, objectPath); err != nil {
		_ = os.Remove(tmpPath)
		return MergeResult{}, err
	}
	return MergeResult{StorageKey: objectKey, Size: total, SHA256: actualSHA256}, nil
}

func (s *POSIX) ImportObject(workspaceID uint, src io.Reader, expectedSize int64, expectedSHA256 string) (MergeResult, error) {
	if workspaceID == 0 || expectedSize < 0 {
		return MergeResult{}, errors.New("invalid import dimensions")
	}
	expectedSHA256 = strings.ToLower(strings.TrimSpace(expectedSHA256))
	if len(expectedSHA256) != sha256.Size*2 {
		return MergeResult{}, errors.New("sha256 must be 64 hexadecimal characters")
	}
	if _, err := hex.DecodeString(expectedSHA256); err != nil {
		return MergeResult{}, errors.New("sha256 must be hexadecimal")
	}
	objectKey := filepath.ToSlash(filepath.Join("objects", strconv.FormatUint(uint64(workspaceID), 10), uuid.NewString()))
	objectPath, err := s.objectPath(objectKey)
	if err != nil {
		return MergeResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(objectPath), 0o750); err != nil {
		return MergeResult{}, err
	}
	tmpPath := objectPath + ".tmp-" + uuid.NewString()
	destination, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return MergeResult{}, err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(destination, hash), io.LimitReader(src, sizeProbeLimit(expectedSize)))
	syncErr := destination.Sync()
	closeErr := destination.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		_ = os.Remove(tmpPath)
		return MergeResult{}, firstError(copyErr, syncErr, closeErr)
	}
	actualSHA256 := hex.EncodeToString(hash.Sum(nil))
	if written != expectedSize || actualSHA256 != expectedSHA256 {
		_ = os.Remove(tmpPath)
		return MergeResult{}, fmt.Errorf("imported object validation failed: size=%d sha256=%s", written, actualSHA256)
	}
	if err := os.Rename(tmpPath, objectPath); err != nil {
		_ = os.Remove(tmpPath)
		return MergeResult{}, err
	}
	return MergeResult{StorageKey: objectKey, Size: written, SHA256: actualSHA256}, nil
}

func firstError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

const maxInt64 = int64(^uint64(0) >> 1)

func sizeProbeLimit(expectedSize int64) int64 {
	if expectedSize >= maxInt64 {
		return maxInt64
	}
	return expectedSize + 1
}

func (s *POSIX) RemoveUpload(uploadID string) error {
	if err := validateID(uploadID); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.stagingPath, uploadID))
}

func (s *POSIX) RemoveObject(storageKey string) error {
	path, err := s.objectPath(storageKey)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *POSIX) QuarantineObject(workspaceID uint, storageKey string) (string, error) {
	if workspaceID == 0 {
		return "", errors.New("workspace id is required")
	}
	cleanKey := filepath.ToSlash(filepath.Clean(filepath.FromSlash(storageKey)))
	expectedPrefix := filepath.ToSlash(filepath.Join("objects", strconv.FormatUint(uint64(workspaceID), 10))) + "/"
	if !strings.HasPrefix(cleanKey, expectedPrefix) {
		return "", ErrInvalidObjectKey
	}
	sourcePath, err := s.objectPath(cleanKey)
	if err != nil {
		return "", err
	}
	quarantineKey := filepath.ToSlash(filepath.Join(
		"quarantine",
		strconv.FormatUint(uint64(workspaceID), 10),
		time.Now().UTC().Format("20060102T150405Z")+"-"+uuid.NewString(),
		strings.TrimPrefix(cleanKey, expectedPrefix),
	))
	destinationPath := filepath.Join(s.stagingPath, filepath.FromSlash(quarantineKey))
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o750); err != nil {
		return "", err
	}
	if err := os.Rename(sourcePath, destinationPath); err != nil {
		return "", err
	}
	return quarantineKey, nil
}

func (s *POSIX) RemoveQuarantinedObject(workspaceID uint, quarantineKey string) error {
	path, err := s.quarantinePath(workspaceID, quarantineKey)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// RestoreQuarantinedObject puts an object back without overwriting an object
// that may have appeared after quarantine.
func (s *POSIX) RestoreQuarantinedObject(workspaceID uint, quarantineKey, storageKey string) error {
	sourcePath, err := s.quarantinePath(workspaceID, quarantineKey)
	if err != nil {
		return err
	}
	destinationPath, err := s.objectPath(storageKey)
	if err != nil {
		return err
	}
	expectedPrefix := filepath.ToSlash(filepath.Join("objects", strconv.FormatUint(uint64(workspaceID), 10))) + "/"
	cleanStorageKey := filepath.ToSlash(filepath.Clean(filepath.FromSlash(storageKey)))
	if !strings.HasPrefix(cleanStorageKey, expectedPrefix) {
		return ErrInvalidObjectKey
	}
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o750); err != nil {
		return err
	}
	if _, err := os.Lstat(destinationPath); err == nil {
		if _, sourceErr := os.Lstat(sourcePath); errors.Is(sourceErr, os.ErrNotExist) {
			return nil
		}
		return os.ErrExist
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.Rename(sourcePath, destinationPath)
}

func (s *POSIX) quarantinePath(workspaceID uint, quarantineKey string) (string, error) {
	if workspaceID == 0 {
		return "", errors.New("workspace id is required")
	}
	cleanKey := filepath.ToSlash(filepath.Clean(filepath.FromSlash(quarantineKey)))
	expectedPrefix := filepath.ToSlash(filepath.Join("quarantine", strconv.FormatUint(uint64(workspaceID), 10))) + "/"
	if !strings.HasPrefix(cleanKey, expectedPrefix) {
		return "", ErrInvalidObjectKey
	}
	path := filepath.Join(s.stagingPath, filepath.FromSlash(cleanKey))
	relative, err := filepath.Rel(s.stagingPath, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", ErrInvalidObjectKey
	}
	return path, nil
}

func (s *POSIX) OpenObject(storageKey string) (*os.File, error) {
	path, err := s.objectPath(storageKey)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *POSIX) ListWorkspaceObjects(workspaceID uint) ([]string, error) {
	if workspaceID == 0 {
		return nil, errors.New("workspace id is required")
	}
	root := filepath.Join(s.rootPath, "objects", strconv.FormatUint(uint64(workspaceID), 10))
	var keys []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if errors.Is(walkErr, os.ErrNotExist) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(s.rootPath, path)
		if err != nil {
			return err
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	return keys, err
}

func (s *POSIX) CreateBatchArchive(jobID string) (*os.File, string, error) {
	path, err := s.batchArchivePath(jobID)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, "", err
	}
	tmpPath := path + ".tmp-" + uuid.NewString()
	file, err := os.OpenFile(tmpPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	return file, tmpPath, err
}

func (s *POSIX) CommitBatchArchive(jobID, tmpPath string) (int64, error) {
	path, err := s.batchArchivePath(jobID)
	if err != nil {
		return 0, err
	}
	if filepath.Dir(tmpPath) != filepath.Dir(path) || !strings.HasPrefix(filepath.Base(tmpPath), filepath.Base(path)+".tmp-") {
		return 0, ErrInvalidObjectKey
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return 0, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *POSIX) OpenBatchArchive(jobID string) (*os.File, error) {
	path, err := s.batchArchivePath(jobID)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

func (s *POSIX) RemoveBatchArchive(jobID string) error {
	path, err := s.batchArchivePath(jobID)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *POSIX) partPath(uploadID string, partNo int) string {
	return filepath.Join(s.stagingPath, uploadID, fmt.Sprintf("part-%08d", partNo))
}

func (s *POSIX) batchArchivePath(jobID string) (string, error) {
	if err := validateID(jobID); err != nil {
		return "", err
	}
	return filepath.Join(s.stagingPath, "batch-downloads", jobID+".zip"), nil
}

func (s *POSIX) objectPath(storageKey string) (string, error) {
	if err := validateObjectKey(storageKey); err != nil {
		return "", err
	}
	return filepath.Join(s.rootPath, filepath.FromSlash(storageKey)), nil
}

func validateID(value string) error {
	if value == "" || strings.Contains(value, "/") || strings.Contains(value, "\\") || strings.ContainsRune(value, '\x00') || value == "." || value == ".." {
		return ErrInvalidObjectKey
	}
	return nil
}

func validateObjectKey(value string) error {
	if value == "" || filepath.IsAbs(value) || strings.ContainsRune(value, '\x00') {
		return ErrInvalidObjectKey
	}
	clean := filepath.Clean(filepath.FromSlash(value))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ErrInvalidObjectKey
	}
	return nil
}
