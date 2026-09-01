/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditexport

import (
	"os"
	"strings"
	"testing"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/model"
)

func TestWriteExportOmitsSensitiveDetails(t *testing.T) {
	staging := t.TempDir()
	config.SetTestConfig(&config.Config{Storage: config.StorageConfig{StagingPath: staging}})
	defer config.SetTestConfig(nil)
	job := &model.AuditExportJob{ID: "export-test", Format: "json"}
	logs := []model.OperationLog{{ID: 1, Username: "admin", Action: "share:create", Path: "/api/fileshare/v1/share/Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA/download", Details: `{"password":"secret","token":"share-token"}`, CreatedAt: time.Now()}}
	path, _, err := writeExport(job, logs)
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "secret") || strings.Contains(string(data), "share-token") || strings.Contains(string(data), "Q29kZXhTaGFyZVRva2VuXzEyMzQ1Njc4OTA") || strings.Contains(string(data), "details") {
		t.Fatalf("sensitive data leaked in export: %s", data)
	}
	if !strings.Contains(string(data), "/share/:token/download") {
		t.Fatalf("redacted share path missing in export: %s", data)
	}
}

func TestCSVCellPreventsFormulaInjection(t *testing.T) {
	for _, value := range []string{"=CMD()", "+1", "-2", "@SUM(A1)", "  =CMD()"} {
		if got := csvCell(value); !strings.HasPrefix(got, "'") {
			t.Fatalf("csvCell(%q) = %q, want apostrophe prefix", value, got)
		}
	}
	if got := csvCell("normal"); got != "normal" {
		t.Fatalf("csvCell(normal) = %q", got)
	}
}
