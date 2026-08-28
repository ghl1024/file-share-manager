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
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/storage"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrMetadataSnapshotMissing = errors.New("backup does not contain a workspace metadata snapshot")
var ErrRestoreWorkspaceExists = errors.New("restore workspace code already exists")
var ErrRestoreWorkspaceReference = errors.New("restore workspace contains a missing global reference")

type WorkspaceRestoreResult struct {
	WorkspaceID uint   `json:"workspace_id"`
	Name        string `json:"name"`
	Code        string `json:"code"`
	NodeCount   int    `json:"node_count"`
	FileCount   int    `json:"file_count"`
	TotalBytes  int64  `json:"total_bytes"`
	MemberCount int    `json:"member_count"`
	ShareCount  int    `json:"share_count"`
}

func (s *Service) RestoreWorkspace(ctx context.Context, sourceWorkspaceID, actorID uint, jobID, name, code string) (*WorkspaceRestoreResult, error) {
	manifest, err := s.Verify(ctx, sourceWorkspaceID, jobID)
	if err != nil {
		return nil, err
	}
	if manifest.Metadata == nil {
		return nil, ErrMetadataSnapshotMissing
	}
	if err := s.validateGlobalRestoreReferences(manifest.Metadata, actorID); err != nil {
		return nil, err
	}
	var existing int64
	if err := s.db.Model(&model.Workspace{}).Where("code = ?", code).Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, ErrRestoreWorkspaceExists
	}

	cfg := config.GetConfig()
	if cfg == nil {
		return nil, ErrBackupBackendUnsupported
	}
	backupStore, err := storage.NewConfiguredBackupStorage(ctx, cfg.Backup)
	if err != nil {
		return nil, mapBackupStorageError(err)
	}
	primary, err := storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
	if err != nil {
		return nil, err
	}
	job, err := s.jobs.Get(sourceWorkspaceID, strings.TrimSpace(jobID))
	if err != nil {
		return nil, err
	}
	manifests, err := s.loadManifestChain(backupStore, job)
	if err != nil {
		return nil, err
	}
	chainObjects := make(map[string]ObjectEntry)
	for _, chainManifest := range manifests {
		for _, object := range chainManifest.Objects {
			chainObjects[versionObjectKey(object.VersionID, object.SHA256)] = object
		}
	}

	var totalBytes int64
	for _, version := range manifest.Metadata.Versions {
		totalBytes += version.Size
	}
	if manifest.Metadata.Workspace.QuotaBytes != nil && totalBytes > *manifest.Metadata.Workspace.QuotaBytes {
		return nil, dao.ErrQuotaExceeded
	}
	workspace := &model.Workspace{
		UUID: uuid.NewString(), Name: strings.TrimSpace(name), Code: strings.ToLower(strings.TrimSpace(code)),
		Description: manifest.Metadata.Workspace.Description, Status: -1, QuotaBytes: manifest.Metadata.Workspace.QuotaBytes,
		UsedBytes: totalBytes, ReservedBytes: 0, CreatedBy: actorID,
	}
	if err := s.db.Create(workspace).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			return nil, ErrRestoreWorkspaceExists
		}
		return nil, err
	}

	importedKeys := make([]string, 0, len(manifest.Metadata.Versions))
	importedVersions := make(map[uint]model.FileVersion, len(manifest.Metadata.Versions))
	storageKeys := make(map[string]string, len(manifest.Metadata.Versions))
	memberUsage := make(map[uint]int64)
	cleanup := true
	defer func() {
		if !cleanup {
			return
		}
		for _, key := range importedKeys {
			_ = primary.RemoveObject(key)
		}
		_ = s.db.Unscoped().Delete(&model.Workspace{}, workspace.ID).Error
	}()
	for _, version := range manifest.Metadata.Versions {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		object, exists := chainObjects[versionObjectKey(version.ID, version.SHA256)]
		if !exists {
			return nil, fmt.Errorf("%w: version %d", ErrRestoreObjectNotFound, version.ID)
		}
		reader, err := backupStore.Get(object.BackupKey)
		if err != nil {
			return nil, err
		}
		imported, importErr := primary.ImportObject(workspace.ID, reader, version.Size, version.SHA256)
		_ = reader.Close()
		if importErr != nil {
			return nil, importErr
		}
		importedKeys = append(importedKeys, imported.StorageKey)
		storageKeys[version.StorageKey] = imported.StorageKey
		memberUsage[version.CreatedBy] += version.Size
		importedVersions[version.ID] = model.FileVersion{
			WorkspaceID: workspace.ID, VersionNo: version.VersionNo, StorageKey: imported.StorageKey,
			StorageClass: "standard", LastAccessedAt: version.LastAccessedAt, Size: version.Size, SHA256: version.SHA256,
			Extension: version.Extension, DetectedMime: version.DetectedMime, RiskLevel: version.RiskLevel,
			ScanStatus: version.ScanStatus, ScanMessage: version.ScanMessage, ScanRetryCount: version.ScanRetryCount,
			ScanNextRetryAt: version.ScanNextRetryAt, ScanLastAttemptAt: version.ScanLastAttemptAt, Encrypted: version.Encrypted,
			CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt,
		}
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		var locked model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, workspace.ID).Error; err != nil {
			return err
		}
		if locked.Status != -1 {
			return errors.New("restore workspace is not isolated")
		}
		if err := restoreMemberships(tx, manifest.Metadata, workspace.ID, actorID, memberUsage); err != nil {
			return err
		}
		nodeIDs, versionIDs, err := restoreNodesAndVersions(tx, manifest.Metadata, workspace.ID, importedVersions)
		if err != nil {
			return err
		}
		groupIDs, err := restoreGroups(tx, manifest.Metadata, workspace.ID)
		if err != nil {
			return err
		}
		if err := restoreACLsAndFavorites(tx, manifest.Metadata, workspace.ID, nodeIDs, groupIDs); err != nil {
			return err
		}
		if err := restoreRoles(tx, manifest.Metadata, workspace.ID); err != nil {
			return err
		}
		if err := restoreShares(tx, manifest.Metadata, workspace.ID, nodeIDs, storageKeys); err != nil {
			return err
		}
		if err := restoreComments(tx, manifest.Metadata, workspace.ID, nodeIDs); err != nil {
			return err
		}
		if err := tx.Model(&locked).Updates(map[string]any{"status": 1, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return dao.AppendChange(tx, workspace.ID, "workspace", workspace.ID, "restore_from_backup", map[string]any{
			"source_workspace_id": sourceWorkspaceID, "backup_id": jobID, "source_version_count": len(versionIDs),
		})
	})
	if err != nil {
		return nil, err
	}
	cleanup = false
	return &WorkspaceRestoreResult{
		WorkspaceID: workspace.ID, Name: workspace.Name, Code: workspace.Code, NodeCount: len(manifest.Metadata.Nodes),
		FileCount: len(manifest.Metadata.Versions), TotalBytes: totalBytes, MemberCount: len(manifest.Metadata.Members),
		ShareCount: len(manifest.Metadata.Shares),
	}, nil
}

func (s *Service) validateGlobalRestoreReferences(snapshot *MetadataSnapshot, actorID uint) error {
	userIDs := map[uint]struct{}{actorID: {}}
	addUser := func(id uint) {
		if id > 0 {
			userIDs[id] = struct{}{}
		}
	}
	for _, member := range snapshot.Members {
		addUser(member.UserID)
		addUser(member.CreatedBy)
	}
	for _, node := range snapshot.Nodes {
		addUser(node.CreatedBy)
		addUser(node.UpdatedBy)
	}
	for _, version := range snapshot.Versions {
		addUser(version.CreatedBy)
	}
	for _, acl := range snapshot.ACLs {
		addUser(acl.CreatedBy)
		if acl.SubjectType == "user" {
			addUser(acl.SubjectID)
		}
	}
	for _, favorite := range snapshot.Favorites {
		addUser(favorite.UserID)
	}
	for _, group := range snapshot.Groups {
		addUser(group.CreatedBy)
	}
	for _, member := range snapshot.GroupMembers {
		addUser(member.UserID)
	}
	for _, role := range snapshot.Roles {
		addUser(role.CreatedBy)
	}
	for _, relation := range snapshot.UserRoles {
		addUser(relation.UserID)
	}
	for _, comment := range snapshot.Comments {
		addUser(comment.AuthorID)
	}
	for _, mention := range snapshot.CommentMentions {
		addUser(mention.UserID)
	}
	for _, share := range snapshot.Shares {
		addUser(share.CreatedBy)
	}
	ids := make([]uint, 0, len(userIDs))
	for id := range userIDs {
		ids = append(ids, id)
	}
	var count int64
	if err := s.db.Model(&model.User{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(ids)) {
		return fmt.Errorf("%w: one or more users no longer exist", ErrRestoreWorkspaceReference)
	}
	permissionCodes := make(map[string]struct{})
	for _, relation := range snapshot.RolePermissions {
		permissionCodes[relation.PermissionCode] = struct{}{}
	}
	if len(permissionCodes) > 0 {
		codes := make([]string, 0, len(permissionCodes))
		for code := range permissionCodes {
			codes = append(codes, code)
		}
		count = 0
		if err := s.db.Model(&model.Permission{}).Where("code IN ?", codes).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(codes)) {
			return fmt.Errorf("%w: one or more permissions no longer exist", ErrRestoreWorkspaceReference)
		}
	}
	return nil
}

func restoreMemberships(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID, actorID uint, memberUsage map[uint]int64) error {
	actorFound := false
	for _, source := range snapshot.Members {
		role := source.Role
		if source.UserID == actorID {
			role = "workspace_admin"
			actorFound = true
		}
		member := model.WorkspaceMembership{WorkspaceID: workspaceID, UserID: source.UserID, Role: role, QuotaBytes: source.QuotaBytes, UsedBytes: memberUsage[source.UserID], ReservedBytes: 0, JoinedAt: source.JoinedAt, CreatedBy: source.CreatedBy}
		if err := tx.Create(&member).Error; err != nil {
			return err
		}
	}
	if !actorFound {
		return tx.Create(&model.WorkspaceMembership{WorkspaceID: workspaceID, UserID: actorID, Role: "workspace_admin", JoinedAt: time.Now(), CreatedBy: actorID}).Error
	}
	return nil
}

func restoreNodesAndVersions(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint, imported map[uint]model.FileVersion) (map[uint]uint, map[uint]uint, error) {
	ordered := append([]model.Node(nil), snapshot.Nodes...)
	sourceNodes := make(map[uint]model.Node, len(ordered))
	for _, node := range ordered {
		sourceNodes[node.ID] = node
	}
	sort.SliceStable(ordered, func(i, j int) bool { return nodeDepth(sourceNodes, ordered[i]) < nodeDepth(sourceNodes, ordered[j]) })
	nodeIDs := make(map[uint]uint, len(ordered))
	for _, source := range ordered {
		var parentID *uint
		if source.ParentID != nil {
			mapped := nodeIDs[*source.ParentID]
			parentID = &mapped
		}
		node := model.Node{WorkspaceID: workspaceID, ParentID: parentID, Name: source.Name, NormalizedName: strings.ToLower(strings.TrimSpace(source.Name)), Type: source.Type, InheritMode: source.InheritMode, Status: source.Status, TrashedAt: source.TrashedAt, CreatedBy: source.CreatedBy, UpdatedBy: source.UpdatedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
		if err := tx.Create(&node).Error; err != nil {
			return nil, nil, err
		}
		nodeIDs[source.ID] = node.ID
		if err := tx.Create(&model.NodeClosure{AncestorID: node.ID, DescendantID: node.ID, Depth: 0}).Error; err != nil {
			return nil, nil, err
		}
		if parentID != nil {
			if err := tx.Exec(`INSERT INTO node_closures (ancestor_id, descendant_id, depth) SELECT ancestor_id, ?, depth + 1 FROM node_closures WHERE descendant_id = ?`, node.ID, *parentID).Error; err != nil {
				return nil, nil, err
			}
		}
	}
	versionIDs := make(map[uint]uint, len(snapshot.Versions))
	for _, source := range snapshot.Versions {
		version := imported[source.ID]
		version.NodeID = nodeIDs[source.NodeID]
		if err := tx.Create(&version).Error; err != nil {
			return nil, nil, err
		}
		versionIDs[source.ID] = version.ID
	}
	for _, source := range snapshot.Nodes {
		if source.ActiveVersion != nil {
			if err := tx.Model(&model.Node{}).Where("id = ?", nodeIDs[source.ID]).Update("active_version", versionIDs[*source.ActiveVersion]).Error; err != nil {
				return nil, nil, err
			}
		}
	}
	return nodeIDs, versionIDs, nil
}

func nodeDepth(nodes map[uint]model.Node, node model.Node) int {
	depth := 0
	for node.ParentID != nil {
		depth++
		node = nodes[*node.ParentID]
	}
	return depth
}

func restoreGroups(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint) (map[uint]uint, error) {
	ids := make(map[uint]uint, len(snapshot.Groups))
	for _, source := range snapshot.Groups {
		group := model.UserGroup{WorkspaceID: workspaceID, Name: source.Name, Description: source.Description, Source: source.Source, LDAPDN: source.LDAPDN, CreatedBy: source.CreatedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
		if err := tx.Create(&group).Error; err != nil {
			return nil, err
		}
		ids[source.ID] = group.ID
	}
	for _, source := range snapshot.GroupMembers {
		if err := tx.Create(&model.UserGroupMember{GroupID: ids[source.GroupID], UserID: source.UserID, JoinedAt: source.JoinedAt}).Error; err != nil {
			return nil, err
		}
	}
	return ids, nil
}

func restoreACLsAndFavorites(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint, nodeIDs, groupIDs map[uint]uint) error {
	for _, source := range snapshot.ACLs {
		subjectID := source.SubjectID
		if source.SubjectType == "group" {
			subjectID = groupIDs[source.SubjectID]
		}
		acl := model.NodeACL{WorkspaceID: workspaceID, NodeID: nodeIDs[source.NodeID], SubjectType: source.SubjectType, SubjectID: subjectID, Effect: source.Effect, AccessLevel: source.AccessLevel, InheritToChildren: source.InheritToChildren, CreatedBy: source.CreatedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
		if err := tx.Create(&acl).Error; err != nil {
			return err
		}
	}
	for _, source := range snapshot.Favorites {
		if err := tx.Create(&model.Favorite{WorkspaceID: workspaceID, UserID: source.UserID, NodeID: nodeIDs[source.NodeID], CreatedAt: source.CreatedAt}).Error; err != nil {
			return err
		}
	}
	return nil
}

func restoreRoles(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint) error {
	roleIDs := make(map[uint]uint, len(snapshot.Roles))
	for _, source := range snapshot.Roles {
		role := model.Role{WorkspaceID: workspaceID, Code: source.Code, Name: source.Name, Description: source.Description, SortOrder: source.SortOrder, Status: source.Status, CreatedBy: source.CreatedBy, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt}
		if err := tx.Create(&role).Error; err != nil {
			return err
		}
		roleIDs[source.ID] = role.ID
	}
	permissionIDs := make(map[string]uint)
	if len(snapshot.RolePermissions) > 0 {
		codes := make([]string, 0, len(snapshot.RolePermissions))
		for _, relation := range snapshot.RolePermissions {
			codes = append(codes, relation.PermissionCode)
		}
		var permissions []model.Permission
		if err := tx.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
			return err
		}
		for _, permission := range permissions {
			permissionIDs[permission.Code] = permission.ID
		}
	}
	for _, source := range snapshot.RolePermissions {
		if err := tx.Create(&model.RolePermission{RoleID: roleIDs[source.RoleID], PermissionID: permissionIDs[source.PermissionCode]}).Error; err != nil {
			return err
		}
	}
	for _, source := range snapshot.UserRoles {
		if err := tx.Create(&model.UserRole{WorkspaceID: workspaceID, UserID: source.UserID, RoleID: roleIDs[source.RoleID]}).Error; err != nil {
			return err
		}
	}
	return nil
}

func restoreShares(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint, nodeIDs map[uint]uint, storageKeys map[string]string) error {
	shareIDs := make(map[uint]uint, len(snapshot.Shares))
	now := time.Now()
	for _, source := range snapshot.Shares {
		publicID := uuid.NewString()
		digest := sha256.Sum256([]byte("restored-share:" + publicID))
		share := model.Share{WorkspaceID: workspaceID, SourceNodeID: nodeIDs[source.SourceNodeID], PublicID: publicID, TokenHash: hex.EncodeToString(digest[:]), Name: source.Name, RootType: source.RootType, RootName: source.RootName, ExpiresAt: source.ExpiresAt, MaxDownloads: source.MaxDownloads, DownloadCount: source.DownloadCount, Status: "revoked", CreatedBy: source.CreatedBy, CreatedAt: source.CreatedAt, UpdatedAt: now, RevokedAt: &now}
		if err := tx.Create(&share).Error; err != nil {
			return err
		}
		shareIDs[source.ID] = share.ID
	}
	for _, source := range snapshot.ShareItems {
		storageKey, exists := storageKeys[source.StorageKey]
		if !exists {
			continue
		}
		item := model.ShareItem{ShareID: shareIDs[source.ShareID], PublicID: uuid.NewString(), RelativePath: source.RelativePath, Name: source.Name, VersionNo: source.VersionNo, StorageKey: storageKey, Size: source.Size, SHA256: source.SHA256, DetectedMime: source.DetectedMime, ScanStatus: source.ScanStatus, CreatedAt: source.CreatedAt}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func restoreComments(tx *gorm.DB, snapshot *MetadataSnapshot, workspaceID uint, nodeIDs map[uint]uint) error {
	commentIDs := make(map[uint]uint, len(snapshot.Comments))
	for _, source := range snapshot.Comments {
		comment := model.NodeComment{
			WorkspaceID: workspaceID, NodeID: nodeIDs[source.NodeID], AuthorID: source.AuthorID,
			Content: source.Content, Revision: source.Revision, CreatedAt: source.CreatedAt, UpdatedAt: source.UpdatedAt,
		}
		if err := tx.Create(&comment).Error; err != nil {
			return err
		}
		commentIDs[source.ID] = comment.ID
	}
	for _, source := range snapshot.CommentMentions {
		commentID, exists := commentIDs[source.CommentID]
		if !exists {
			return errors.New("restored comment mention has no comment")
		}
		if err := tx.Create(&model.NodeCommentMention{CommentID: commentID, UserID: source.UserID, CreatedAt: source.CreatedAt}).Error; err != nil {
			return err
		}
	}
	return nil
}

func versionObjectKey(versionID uint, sha string) string {
	return fmt.Sprintf("%d:%s", versionID, strings.ToLower(strings.TrimSpace(sha)))
}
