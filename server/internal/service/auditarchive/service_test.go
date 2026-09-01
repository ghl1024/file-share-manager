/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditarchive

import (
	"testing"

	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
)

func TestDefaultServiceInitializesAfterDatabase(t *testing.T) {
	previousDB := database.DB
	previousAuditDB := database.AuditArchiveDB
	previousService := defaultService
	t.Cleanup(func() {
		database.DB = previousDB
		database.AuditArchiveDB = previousAuditDB
		defaultService = previousService
	})

	database.DB = &gorm.DB{}
	database.AuditArchiveDB = &gorm.DB{}
	defaultService = nil

	service := DefaultService()
	if service == nil || service.archives == nil || service.workerArchives == nil {
		t.Fatal("DefaultService() did not initialize the archive DAO")
	}
	if DefaultService() != service {
		t.Fatal("DefaultService() did not reuse the shared service")
	}
}
