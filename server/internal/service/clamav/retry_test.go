/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package clamav

import (
	"context"
	"testing"
	"time"

	"file-share-manager/server/internal/model"
)

type retryDAOStub struct {
	candidates []model.FileVersion
	recovered  int64
	claimed    []uint
	completed  []retryCompletion
}

type retryCompletion struct {
	versionID uint
	status    string
	next      *time.Time
}

func (s *retryDAOStub) ListScanRetryCandidates(time.Time, int, int) ([]model.FileVersion, error) {
	return s.candidates, nil
}

func (s *retryDAOStub) ClaimScanRetry(versionID uint, _ int, _ time.Time) (bool, error) {
	s.claimed = append(s.claimed, versionID)
	return true, nil
}

func (s *retryDAOStub) CompleteScanRetry(versionID uint, status, _ string, next *time.Time) (bool, error) {
	s.completed = append(s.completed, retryCompletion{versionID: versionID, status: status, next: next})
	return true, nil
}

func (s *retryDAOStub) RequeueStaleScanRetries(time.Time, time.Time) (int64, error) {
	return s.recovered, nil
}

func TestRetryServiceSchedulesExponentialBackoffAndStopsAtLimit(t *testing.T) {
	now := time.Date(2026, 8, 14, 14, 0, 0, 0, time.Local)
	daoStub := &retryDAOStub{
		recovered: 1,
		candidates: []model.FileVersion{
			{ID: 1, WorkspaceID: 7, StorageKey: "objects/7/first", ScanRetryCount: 0},
			{ID: 2, WorkspaceID: 7, StorageKey: "objects/7/final", ScanRetryCount: 2},
		},
	}
	service := &RetryService{
		files: daoStub, storageRoot: "/storage", maxAttempts: 3, batchSize: 10,
		baseInterval: 5 * time.Minute, staleAfter: 5 * time.Minute,
		scan: func(context.Context, string) Result { return Result{Status: StatusScanError, Message: "timeout"} },
	}
	report, err := service.RunOnce(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Recovered != 1 || report.Scanned != 2 || report.Failed != 2 || report.Exhausted != 1 {
		t.Fatalf("report = %#v", report)
	}
	if len(daoStub.completed) != 2 || daoStub.completed[0].next == nil || !daoStub.completed[0].next.Equal(now.Add(5*time.Minute)) {
		t.Fatalf("first retry completion = %#v", daoStub.completed)
	}
	if daoStub.completed[1].next != nil {
		t.Fatalf("exhausted retry scheduled again: %#v", daoStub.completed[1])
	}
}

func TestRetryServicePersistsCleanAndInfectedResults(t *testing.T) {
	daoStub := &retryDAOStub{candidates: []model.FileVersion{
		{ID: 11, StorageKey: "objects/1/clean"},
		{ID: 12, StorageKey: "objects/1/infected"},
	}}
	service := &RetryService{
		files: daoStub, maxAttempts: 3, batchSize: 10, baseInterval: time.Minute, staleAfter: time.Minute,
		scan: func(_ context.Context, path string) Result {
			if path == "objects/1/infected" {
				return Result{Status: StatusInfected, Message: "FOUND"}
			}
			return Result{Status: StatusClean, Message: "OK"}
		},
	}
	report, err := service.RunOnce(context.Background(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if report.Clean != 1 || report.Infected != 1 || report.Failed != 0 {
		t.Fatalf("report = %#v", report)
	}
	if daoStub.completed[0].status != StatusClean || daoStub.completed[1].status != StatusInfected {
		t.Fatalf("completed = %#v", daoStub.completed)
	}
}

func TestRetryDelayCapsAtOneDay(t *testing.T) {
	if got := retryDelay(10*time.Hour, 4); got != 24*time.Hour {
		t.Fatalf("retryDelay() = %s, want 24h", got)
	}
}
