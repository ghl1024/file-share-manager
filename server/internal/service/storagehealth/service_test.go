/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storagehealth

import (
	"os"
	"path/filepath"
	"testing"

	"file-share-manager/server/internal/config"
)

func TestCheckReadyAndCleansWriteProbes(t *testing.T) {
	root := t.TempDir()
	staging := t.TempDir()
	backup := t.TempDir()
	report := Check(&config.Config{
		Storage: config.StorageConfig{RootPath: root, StagingPath: staging},
		Backup:  config.BackupConfig{Type: "local", LocalPath: backup},
	})
	if report.Status != "ready" || !report.Root.Writable || !report.Staging.Writable || report.Backup == nil || !report.Backup.Writable {
		t.Fatalf("report = %#v", report)
	}
	for _, directory := range []string{root, staging, backup} {
		matches, err := filepath.Glob(filepath.Join(directory, ".fileshare-health-*"))
		if err != nil || len(matches) != 0 {
			t.Fatalf("write probe leaked in %s: %v, %v", directory, matches, err)
		}
	}
}

func TestCheckEscalatesWhenCriticalCapacityThresholdIsBreached(t *testing.T) {
	root := t.TempDir()
	report := Check(&config.Config{
		Storage: config.StorageConfig{RootPath: root, StagingPath: root, MinFreeBytes: 1 << 50, WarnFreePercent: 99, MinFreePercent: 98},
		Backup:  config.BackupConfig{Type: "s3"},
	})
	if report.Status != "critical" || !report.Root.CriticalSpace || len(report.Alerts) == 0 {
		t.Fatalf("report = %#v", report)
	}
}

func TestCapacityStateWarnsBeforeCriticalThreshold(t *testing.T) {
	critical, low := capacityState(8<<30, 15, 1<<20, 20, 10)
	if critical || !low {
		t.Fatalf("capacityState() = critical %v, low %v; want warning only", critical, low)
	}
}

func TestCapacityStateCriticalThresholds(t *testing.T) {
	for _, test := range []struct {
		name        string
		freeBytes   uint64
		freePercent float64
	}{
		{name: "minimum bytes", freeBytes: 512 << 10, freePercent: 50},
		{name: "minimum percent", freeBytes: 8 << 30, freePercent: 5},
	} {
		t.Run(test.name, func(t *testing.T) {
			critical, low := capacityState(test.freeBytes, test.freePercent, 1<<20, 20, 10)
			if !critical || !low {
				t.Fatalf("capacityState() = critical %v, low %v; want both true", critical, low)
			}
		})
	}
}

func TestCheckDegradesForMissingOrNonDirectoryPath(t *testing.T) {
	root := t.TempDir()
	filePath := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	report := Check(&config.Config{
		Storage: config.StorageConfig{RootPath: filePath, StagingPath: filepath.Join(root, "missing"), MinFreeBytes: 1 << 20, WarnFreePercent: 2, MinFreePercent: 1},
		Backup:  config.BackupConfig{Type: "s3"},
	})
	if report.Status != "degraded" || report.Root.Available || report.Staging.Available || len(report.Alerts) != 2 {
		t.Fatalf("report = %#v", report)
	}
}
