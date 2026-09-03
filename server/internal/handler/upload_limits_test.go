/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package handler

import (
	"math"
	"strings"
	"testing"

	"file-share-manager/server/internal/model"
)

func TestCalculateUploadChunkCount(t *testing.T) {
	const maxFileBytes = int64(100 << 30)
	tests := []struct {
		name      string
		totalSize int64
		chunkSize int64
		maxBytes  int64
		want      int
		wantErr   string
	}{
		{name: "zero total size", totalSize: 0, chunkSize: 1 << 20, maxBytes: maxFileBytes, wantErr: "必须为正数"},
		{name: "negative total size", totalSize: -1, chunkSize: 1 << 20, maxBytes: maxFileBytes, wantErr: "必须为正数"},
		{name: "single small file", totalSize: 1, chunkSize: 1 << 20, maxBytes: maxFileBytes, want: 1},
		{name: "exact multiple", totalSize: 3 << 20, chunkSize: 1 << 20, maxBytes: maxFileBytes, want: 3},
		{name: "remainder", totalSize: 3<<20 + 1, chunkSize: 1 << 20, maxBytes: maxFileBytes, want: 4},
		{name: "configured boundary", totalSize: maxFileBytes, chunkSize: 1 << 20, maxBytes: maxFileBytes, want: 100 << 10},
		{name: "max int64 is rejected by file limit", totalSize: math.MaxInt64, chunkSize: 1 << 20, maxBytes: maxFileBytes, wantErr: "文件大小不能超过"},
		{name: "too many chunks", totalSize: 1<<40 + 1, chunkSize: 1 << 20, maxBytes: 1 << 41, wantErr: "文件分片数量过多"},
		{name: "invalid chunk size", totalSize: 1 << 20, chunkSize: 1 << 10, maxBytes: maxFileBytes, wantErr: "分片大小必须"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := calculateUploadChunkCount(test.totalSize, test.chunkSize, test.maxBytes)
			if test.wantErr == "" {
				if err != nil || got != test.want {
					t.Fatalf("calculateUploadChunkCount() = %d, %v, want %d", got, err, test.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("calculateUploadChunkCount() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}

func TestExpectedUploadPartSizeUsesRemainderWithoutMultiplication(t *testing.T) {
	session := &model.UploadSession{TotalSize: 3<<20 + 1, ChunkSize: 1 << 20, TotalChunks: 4}
	for _, test := range []struct {
		partNo int
		want   int64
	}{
		{partNo: 0, want: 1 << 20},
		{partNo: 2, want: 1 << 20},
		{partNo: 3, want: 1},
	} {
		got, err := expectedUploadPartSize(session, test.partNo)
		if err != nil || got != test.want {
			t.Errorf("part %d size = %d, %v, want %d", test.partNo, got, err, test.want)
		}
	}
	if _, err := expectedUploadPartSize(session, 4); err == nil {
		t.Fatal("out-of-range part was accepted")
	}
	exact := &model.UploadSession{TotalSize: 2 << 20, ChunkSize: 1 << 20, TotalChunks: 2}
	if got, err := expectedUploadPartSize(exact, 1); err != nil || got != 1<<20 {
		t.Fatalf("exact-multiple last part size = %d, %v, want %d", got, err, 1<<20)
	}
}

func TestValidateUploadSessionDimensions(t *testing.T) {
	const maxFileBytes = int64(100 << 30)
	tests := []struct {
		name    string
		session *model.UploadSession
		wantErr string
	}{
		{name: "valid", session: &model.UploadSession{TotalSize: 3 << 20, ChunkSize: 1 << 20, TotalChunks: 3}},
		{name: "count mismatch", session: &model.UploadSession{TotalSize: 3 << 20, ChunkSize: 1 << 20, TotalChunks: 2}, wantErr: "分片信息无效"},
		{name: "over current limit", session: &model.UploadSession{TotalSize: 101 << 30, ChunkSize: 1 << 20, TotalChunks: 101 << 10}, wantErr: "超过当前系统限制"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateUploadSessionDimensions(test.session, maxFileBytes)
			if test.wantErr == "" {
				if err != nil {
					t.Fatalf("validateUploadSessionDimensions() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("validateUploadSessionDimensions() error = %v, want %q", err, test.wantErr)
			}
		})
	}
}
