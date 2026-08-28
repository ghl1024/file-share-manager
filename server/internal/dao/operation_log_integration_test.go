/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"file-share-manager/server/internal/model"

	"gorm.io/gorm"
)

func TestOperationLogConcurrentStreamSequenceMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatal(err)
	}

	const workers = 32
	workspaceID := uint(42)
	logs := &OperationLogDAO{db: db}
	start := make(chan struct{})
	errors := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			error := logs.Create(&model.OperationLog{
				UserID: 1, Username: fmt.Sprintf("writer-%02d", index), WorkspaceID: &workspaceID,
				Method: "POST", Path: "/concurrent-audit", Action: "audit:test",
				Status: 200, Details: "{}", RequestID: fmt.Sprintf("request-%02d", index),
				CreatedAt: time.Now(),
			})
			errors <- error
		}()
	}
	close(start)
	wait.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatalf("concurrent Create() failed: %v", err)
		}
	}

	var persisted []model.OperationLog
	if err := db.Where("workspace_id = ?", workspaceID).Order("stream_seq ASC").Find(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	if len(persisted) != workers {
		t.Fatalf("persisted logs = %d, want %d", len(persisted), workers)
	}
	for index, log := range persisted {
		want := uint64(index + 1)
		if log.StreamSeq != want {
			t.Fatalf("stream sequence at %d = %d, want %d", index, log.StreamSeq, want)
		}
		if log.StreamKey != "workspace:42" {
			t.Fatalf("stream key = %q, want workspace:42", log.StreamKey)
		}
	}
	verification, err := logs.VerifyChain(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || verification.Checked != workers || verification.LastSeq != workers {
		t.Fatalf("verification = %+v", verification)
	}

	var stream model.AuditStream
	if err := db.First(&stream, "stream_key = ?", "workspace:42").Error; err != nil {
		t.Fatal(err)
	}
	if stream.NextSeq != workers+1 || stream.LastHash == "" {
		t.Fatalf("stream tail = %+v", stream)
	}
}

func TestOperationLogVerificationConcurrentWritesMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatal(err)
	}

	workspaceID := uint(43)
	logs := &OperationLogDAO{db: db}
	writerDone := make(chan error, 1)
	go func() {
		for index := 0; index < 64; index++ {
			if err := logs.Create(&model.OperationLog{
				UserID: 1, Username: "concurrent-writer", WorkspaceID: &workspaceID,
				Method: "POST", Path: "/concurrent-verify", Action: "audit:test",
				Status: 200, Details: "{}", RequestID: fmt.Sprintf("verify-request-%02d", index),
				CreatedAt: time.Now(),
			}); err != nil {
				writerDone <- err
				return
			}
			time.Sleep(time.Millisecond)
		}
		writerDone <- nil
	}()

	checks := 0
	var verificationErr error
	for {
		select {
		case writerErr := <-writerDone:
			if writerErr != nil {
				t.Fatalf("concurrent writer failed: %v", writerErr)
			}
			if verificationErr != nil {
				t.Fatal(verificationErr)
			}
			final, err := logs.VerifyChain(workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if !final.Valid || final.Checked != 64 || final.LastSeq != 64 {
				t.Fatalf("final verification = %+v", final)
			}
			if checks == 0 {
				t.Fatal("verification did not overlap concurrent writes")
			}
			return
		default:
			verification, err := logs.VerifyChain(workspaceID)
			checks++
			if err != nil {
				verificationErr = fmt.Errorf("concurrent verification failed: %w", err)
			} else if !verification.Valid {
				verificationErr = fmt.Errorf("concurrent verification reported a broken chain: %+v", verification)
			}
		}
	}
}

func TestOperationLogWorkspaceIsolationMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatal(err)
	}

	logs := &OperationLogDAO{db: db}
	for _, workspaceID := range []uint{11, 22} {
		workspaceID := workspaceID
		if err := logs.Create(&model.OperationLog{
			UserID: 1, Username: "audit-user", WorkspaceID: &workspaceID, Method: "GET",
			Path: "/workspace-scoped", Action: "file:list", Category: model.AuditCategoryAccess,
			Status: 200, Details: "{}", CreatedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}

	page, err := logs.ListPageWithFilters(11, 1, 20, AuditFilters{})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 1 || len(page.List) != 1 || page.List[0].WorkspaceID == nil || *page.List[0].WorkspaceID != 11 {
		t.Fatalf("workspace 11 audit page leaked another workspace: %+v", page)
	}
	var workspaceTwoLog model.OperationLog
	if err := db.Where("workspace_id = ?", 22).First(&workspaceTwoLog).Error; err != nil {
		t.Fatal(err)
	}
	got, err := logs.GetByID(11, workspaceTwoLog.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("workspace 11 retrieved workspace 22 audit event: %+v", got)
	}
	verification, err := logs.VerifyChain(11)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || verification.Checked != 1 || verification.LastSeq != 1 {
		t.Fatalf("workspace 11 audit verification = %+v", verification)
	}
}

func TestEnsureAuditStreamsBackfillsLegacyRowsMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatal(err)
	}
	workspaceID := uint(7)
	legacy := []model.OperationLog{
		{UserID: 1, Username: "admin", WorkspaceID: &workspaceID, Method: "POST", Path: "/first", Action: "file:create", Status: 200, Details: "{}", CreatedAt: time.Now().Add(-time.Second)},
		{UserID: 0, Username: "system", Method: "POST", Path: "/global", Action: "system:job", Status: 200, Details: "{}", CreatedAt: time.Now()},
		{UserID: 1, Username: "admin", WorkspaceID: &workspaceID, Method: "POST", Path: "/second", Action: "file:update", Status: 200, Details: "{}", CreatedAt: time.Now().Add(time.Second)},
	}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatalf("idempotent EnsureAuditStreams() failed: %v", err)
	}

	verification, err := (&OperationLogDAO{db: db}).VerifyChain(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || verification.Checked != 2 || verification.LastSeq != 2 {
		t.Fatalf("workspace verification = %+v", verification)
	}
	var global model.OperationLog
	if err := db.First(&global, "workspace_id IS NULL").Error; err != nil {
		t.Fatal(err)
	}
	if global.StreamKey != "global" || global.StreamSeq != 1 || global.CurrentHash == nil {
		t.Fatalf("global legacy row = %+v", global)
	}
	if !db.Migrator().HasIndex(&model.OperationLog{}, "uidx_audit_stream_seq") {
		t.Fatal("unique stream sequence index was not created")
	}
}

func TestOperationLogVerificationLocatesCorruptionMySQL(t *testing.T) {
	tests := []struct {
		name       string
		mutate     func(*testing.T, *gorm.DB, []model.OperationLog)
		brokenKind string
		brokenAt   int
	}{
		{
			name: "duplicate sequence",
			mutate: func(t *testing.T, db *gorm.DB, logs []model.OperationLog) {
				t.Helper()
				if err := db.Exec("DROP INDEX uidx_audit_stream_seq ON operation_logs").Error; err != nil {
					t.Fatal(err)
				}
				if err := db.Model(&model.OperationLog{}).Where("id = ?", logs[1].ID).UpdateColumn("stream_seq", 1).Error; err != nil {
					t.Fatal(err)
				}
			},
			brokenKind: "stream_sequence",
			brokenAt:   1,
		},
		{
			name: "changed event",
			mutate: func(t *testing.T, db *gorm.DB, logs []model.OperationLog) {
				t.Helper()
				if err := db.Model(&model.OperationLog{}).Where("id = ?", logs[1].ID).UpdateColumn("username", "tampered").Error; err != nil {
					t.Fatal(err)
				}
			},
			brokenKind: "current_hash",
			brokenAt:   1,
		},
		{
			name: "missing stream tail",
			mutate: func(t *testing.T, db *gorm.DB, _ []model.OperationLog) {
				t.Helper()
				if err := db.Delete(&model.AuditStream{}, "stream_key = ?", "workspace:9").Error; err != nil {
					t.Fatal(err)
				}
			},
			brokenKind: "stream_missing",
			brokenAt:   2,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db := openTransactionTestDB(t)
			if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
				t.Fatal(err)
			}
			if err := EnsureAuditStreams(db); err != nil {
				t.Fatal(err)
			}
			workspaceID := uint(9)
			dao := &OperationLogDAO{db: db}
			for index := 0; index < 3; index++ {
				if err := dao.Create(&model.OperationLog{
					UserID: 1, Username: "admin", WorkspaceID: &workspaceID, Method: "POST",
					Path: fmt.Sprintf("/event/%d", index), Action: "audit:test", Status: 200,
					Details: "{}", CreatedAt: time.Now().Add(time.Duration(index) * time.Millisecond),
				}); err != nil {
					t.Fatal(err)
				}
			}
			var logs []model.OperationLog
			if err := db.Where("workspace_id = ?", workspaceID).Order("stream_seq ASC").Find(&logs).Error; err != nil {
				t.Fatal(err)
			}
			test.mutate(t, db, logs)
			verification, err := dao.VerifyChain(workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			if verification.Valid || verification.BrokenKind != test.brokenKind || verification.BrokenID != logs[test.brokenAt].ID {
				t.Fatalf("verification = %+v, want kind %s at id %d", verification, test.brokenKind, logs[test.brokenAt].ID)
			}
		})
	}
}

func TestOperationLogVerificationContinuesFromCompletedArchiveMySQL(t *testing.T) {
	db := openTransactionTestDB(t)
	if err := db.AutoMigrate(&model.OperationLog{}, &model.AuditStream{}, &model.AuditArchive{}); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAuditStreams(db); err != nil {
		t.Fatal(err)
	}
	workspaceID := uint(17)
	logs := &OperationLogDAO{db: db}
	for index := 0; index < 4; index++ {
		if err := logs.Create(&model.OperationLog{
			UserID: 1, Username: "admin", WorkspaceID: &workspaceID, Method: "POST",
			Path: fmt.Sprintf("/archive/%d", index), Action: "audit:test", Status: 200,
			Details: "{}", CreatedAt: time.Now().Add(time.Duration(index) * time.Millisecond),
		}); err != nil {
			t.Fatal(err)
		}
	}
	var persisted []model.OperationLog
	if err := db.Where("workspace_id = ?", workspaceID).Order("stream_seq ASC").Find(&persisted).Error; err != nil {
		t.Fatal(err)
	}
	digest, err := AuditArchiveEventsDigest(persisted[:2])
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Truncate(time.Millisecond)
	archive := model.AuditArchive{
		ID: "00000000-0000-0000-0000-000000000017", StreamKey: "workspace:17", WorkspaceID: &workspaceID,
		Status: "completed", FromSeq: 1, ToSeq: 2, EventCount: 2, LastHash: *persisted[1].CurrentHash,
		EventsSHA256: digest, ObjectKey: "fileshare-audit/workspace-17/test.enc", ObjectSize: 128,
		ObjectSHA256: strings.Repeat("f", 64), StartedAt: &now, CompletedAt: &now, VerifiedAt: &now, CreatedAt: now,
	}
	manifest, err := archive.Manifest().WithHash()
	if err != nil {
		t.Fatal(err)
	}
	archive.ManifestHash = manifest.ManifestHash
	if err := db.Create(&archive).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Where("workspace_id = ? AND stream_seq <= ?", workspaceID, 2).Delete(&model.OperationLog{}).Error; err != nil {
		t.Fatal(err)
	}
	verification, err := logs.VerifyChain(workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if !verification.Valid || verification.Checked != 4 || verification.Archived != 2 || verification.LastSeq != 4 {
		t.Fatalf("verification = %+v", verification)
	}
}
