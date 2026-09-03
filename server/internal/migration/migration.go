/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package migration

import (
	"fmt"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"

	"gorm.io/gorm"
)

// Run is the single schema definition used by development auto-migration and
// the explicit production migration command.
func Run(db *gorm.DB) error {
	if err := migrateUserGroupLDAPDNColumn(db); err != nil {
		return err
	}
	if err := db.AutoMigrate(schemaModels()...); err != nil {
		return err
	}
	if err := ensureNodeSearchIndexes(db); err != nil {
		return err
	}
	if err := ensureUploadSessionConstraints(db); err != nil {
		return err
	}
	return dao.EnsureAuditStreams(db)
}

func ensureUploadSessionConstraints(db *gorm.DB) error {
	constraints := []struct {
		name string
		expr string
	}{
		{name: "chk_upload_sessions_total_size_positive", expr: "total_size > 0"},
		{name: "chk_upload_sessions_chunk_size_positive", expr: "chunk_size > 0"},
		{name: "chk_upload_sessions_total_chunks_positive", expr: "total_chunks > 0"},
	}
	for _, constraint := range constraints {
		var count int64
		if err := db.Raw(`
			SELECT COUNT(*)
			FROM information_schema.TABLE_CONSTRAINTS
			WHERE CONSTRAINT_SCHEMA = DATABASE()
			  AND TABLE_NAME = 'upload_sessions'
			  AND CONSTRAINT_NAME = ?
			  AND CONSTRAINT_TYPE = 'CHECK'
		`, constraint.name).Scan(&count).Error; err != nil {
			return fmt.Errorf("check upload_sessions constraint %s: %w", constraint.name, err)
		}
		if count > 0 {
			continue
		}
		if err := db.Exec("ALTER TABLE upload_sessions ADD CONSTRAINT " + constraint.name + " CHECK (" + constraint.expr + ")").Error; err != nil {
			return fmt.Errorf("add upload_sessions constraint %s: %w", constraint.name, err)
		}
	}
	return nil
}

func ensureNodeSearchIndexes(db *gorm.DB) error {
	migrator := db.Migrator()
	if !migrator.HasIndex(&model.Node{}, "idx_nodes_fulltext_name") {
		if err := db.Exec("ALTER TABLE nodes ADD FULLTEXT INDEX idx_nodes_fulltext_name (normalized_name) WITH PARSER ngram").Error; err != nil {
			return err
		}
	}
	if !migrator.HasIndex(&model.Node{}, "idx_nodes_search_prefix") {
		if err := db.Exec("CREATE INDEX idx_nodes_search_prefix ON nodes (workspace_id, status, normalized_name)").Error; err != nil {
			return err
		}
	}
	return nil
}

func schemaModels() []any {
	return []any{
		&model.User{}, &model.Role{}, &model.UserRole{}, &model.Permission{}, &model.RolePermission{}, &model.Menu{}, &model.MenuPermission{}, &model.Workspace{},
		&model.WorkspaceMembership{}, &model.UserGroup{}, &model.UserGroupMember{}, &model.Node{}, &model.NodeClosure{},
		&model.NodeACL{}, &model.FileVersion{}, &model.UploadSession{}, &model.Share{}, &model.ShareItem{}, &model.OperationLog{}, &model.AuditStream{},
		&model.Favorite{}, &model.BatchDownloadJob{}, &model.BatchDownloadItem{}, &model.ChangeLog{}, &model.BackupJob{}, &model.BackupRestoreDrill{},
		&model.AuditExportJob{}, &model.AuditArchive{}, &model.LDAPConfig{}, &model.LDAPSyncHistory{}, &model.StorageQuarantine{},
		&model.NotificationChannel{}, &model.NotificationOutbox{}, &model.UserNotification{}, &model.UserNotificationPreference{}, &model.RecentNodeAccess{},
		&model.NodeComment{}, &model.NodeCommentMention{},
	}
}

func migrateUserGroupLDAPDNColumn(db *gorm.DB) error {
	migrator := db.Migrator()
	hasLegacy := migrator.HasColumn(&model.UserGroup{}, "ldapdn")
	hasCurrent := migrator.HasColumn(&model.UserGroup{}, "ldap_dn")
	switch {
	case hasLegacy && !hasCurrent:
		return migrator.RenameColumn(&model.UserGroup{}, "ldapdn", "ldap_dn")
	case hasLegacy && hasCurrent:
		return db.Exec("UPDATE user_groups SET ldap_dn = ldapdn WHERE ldap_dn = '' AND ldapdn <> ''").Error
	default:
		return nil
	}
}
