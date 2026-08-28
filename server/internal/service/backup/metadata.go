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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"file-share-manager/server/internal/model"

	"gorm.io/gorm"
)

const (
	metadataSchemaVersion        = 2
	minimumMetadataSchemaVersion = 1
)

// MetadataSnapshot is the complete workspace control-plane state at a recovery
// point. Authentication secrets and transient upload sessions are excluded.
type MetadataSnapshot struct {
	SchemaVersion    int                         `json:"schema_version"`
	CapturedAt       time.Time                   `json:"captured_at"`
	Workspace        model.Workspace             `json:"workspace"`
	Members          []model.WorkspaceMembership `json:"members"`
	Nodes            []model.Node                `json:"nodes"`
	Versions         []VersionSnapshot           `json:"versions"`
	ACLs             []model.NodeACL             `json:"acls"`
	Favorites        []model.Favorite            `json:"favorites"`
	Groups           []model.UserGroup           `json:"groups"`
	GroupMembers     []model.UserGroupMember     `json:"group_members"`
	Roles            []model.Role                `json:"roles"`
	RolePermissions  []RolePermissionSnapshot    `json:"role_permissions"`
	UserRoles        []model.UserRole            `json:"user_roles"`
	Shares           []ShareSnapshot             `json:"shares"`
	ShareItems       []ShareItemSnapshot         `json:"share_items"`
	Comments         []model.NodeComment         `json:"comments,omitempty"`
	CommentMentions  []model.NodeCommentMention  `json:"comment_mentions,omitempty"`
	MetadataHash     string                      `json:"metadata_hash"`
	hashVerified     bool
	typedFingerprint string
}

type VersionSnapshot struct {
	ID                uint       `json:"id"`
	NodeID            uint       `json:"node_id"`
	VersionNo         int        `json:"version_no"`
	StorageKey        string     `json:"storage_key"`
	StorageClass      string     `json:"storage_class"`
	ArchiveError      string     `json:"archive_error,omitempty"`
	LastAccessedAt    *time.Time `json:"last_accessed_at,omitempty"`
	Size              int64      `json:"size"`
	SHA256            string     `json:"sha256"`
	Extension         string     `json:"extension"`
	DetectedMime      string     `json:"detected_mime"`
	RiskLevel         string     `json:"risk_level"`
	ScanStatus        string     `json:"scan_status"`
	ScanMessage       string     `json:"scan_message"`
	ScanRetryCount    int        `json:"scan_retry_count"`
	ScanNextRetryAt   *time.Time `json:"scan_next_retry_at,omitempty"`
	ScanLastAttemptAt *time.Time `json:"scan_last_attempt_at,omitempty"`
	Encrypted         bool       `json:"encrypted"`
	CreatedBy         uint       `json:"created_by"`
	CreatedAt         time.Time  `json:"created_at"`
}

type RolePermissionSnapshot struct {
	RoleID         uint   `json:"role_id"`
	PermissionCode string `json:"permission_code"`
}

// ShareSnapshot intentionally omits token and password hashes.
type ShareSnapshot struct {
	ID            uint       `json:"id"`
	SourceNodeID  uint       `json:"source_node_id"`
	Name          string     `json:"name"`
	RootType      string     `json:"root_type"`
	RootName      string     `json:"root_name"`
	ExpiresAt     time.Time  `json:"expires_at"`
	MaxDownloads  *int       `json:"max_downloads"`
	DownloadCount int        `json:"download_count"`
	Status        string     `json:"status"`
	CreatedBy     uint       `json:"created_by"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	RevokedAt     *time.Time `json:"revoked_at"`
}

type ShareItemSnapshot struct {
	ID           uint      `json:"id"`
	ShareID      uint      `json:"share_id"`
	RelativePath string    `json:"relative_path"`
	Name         string    `json:"name"`
	VersionNo    int       `json:"version_no"`
	StorageKey   string    `json:"storage_key"`
	Size         int64     `json:"size"`
	SHA256       string    `json:"sha256"`
	DetectedMime string    `json:"detected_mime"`
	ScanStatus   string    `json:"scan_status"`
	CreatedAt    time.Time `json:"created_at"`
}

func captureMetadata(tx *gorm.DB, workspaceID uint) (*MetadataSnapshot, error) {
	snapshot := &MetadataSnapshot{SchemaVersion: metadataSchemaVersion, CapturedAt: time.Now().UTC()}
	if err := tx.First(&snapshot.Workspace, workspaceID).Error; err != nil {
		return nil, err
	}
	queries := []struct {
		order string
		out   any
		where string
		args  []any
	}{
		{"id ASC", &snapshot.Members, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.Nodes, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.ACLs, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.Favorites, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.Groups, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.Roles, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.UserRoles, "workspace_id = ?", []any{workspaceID}},
		{"id ASC", &snapshot.Comments, "workspace_id = ?", []any{workspaceID}},
	}
	for _, query := range queries {
		if err := tx.Where(query.where, query.args...).Order(query.order).Find(query.out).Error; err != nil {
			return nil, err
		}
	}

	var versions []model.FileVersion
	if err := tx.Where("workspace_id = ?", workspaceID).Order("id ASC").Find(&versions).Error; err != nil {
		return nil, err
	}
	snapshot.Versions = make([]VersionSnapshot, 0, len(versions))
	for _, version := range versions {
		snapshot.Versions = append(snapshot.Versions, VersionSnapshot{
			ID: version.ID, NodeID: version.NodeID, VersionNo: version.VersionNo, StorageKey: version.StorageKey,
			StorageClass: version.StorageClass, ArchiveError: version.ArchiveError, LastAccessedAt: version.LastAccessedAt, Size: version.Size, SHA256: version.SHA256, Extension: version.Extension,
			DetectedMime: version.DetectedMime, RiskLevel: version.RiskLevel, ScanStatus: version.ScanStatus,
			ScanMessage: version.ScanMessage, ScanRetryCount: version.ScanRetryCount, ScanNextRetryAt: version.ScanNextRetryAt,
			ScanLastAttemptAt: version.ScanLastAttemptAt, Encrypted: version.Encrypted, CreatedBy: version.CreatedBy, CreatedAt: version.CreatedAt,
		})
	}

	if err := tx.Table("user_group_members AS ugm").
		Select("ugm.group_id, ugm.user_id, ugm.joined_at").
		Joins("JOIN user_groups ug ON ug.id = ugm.group_id").
		Where("ug.workspace_id = ?", workspaceID).
		Order("ugm.group_id ASC, ugm.user_id ASC").Scan(&snapshot.GroupMembers).Error; err != nil {
		return nil, err
	}
	if err := tx.Table("role_permissions AS rp").
		Select("rp.role_id, permissions.code AS permission_code").
		Joins("JOIN roles ON roles.id = rp.role_id").
		Joins("JOIN permissions ON permissions.id = rp.permission_id").
		Where("roles.workspace_id = ?", workspaceID).
		Order("rp.role_id ASC, permissions.code ASC").Scan(&snapshot.RolePermissions).Error; err != nil {
		return nil, err
	}

	var shares []model.Share
	if err := tx.Table("shares AS s").Select("s.*").
		Joins("JOIN nodes n ON n.id = s.source_node_id AND n.workspace_id = s.workspace_id").
		Where("s.workspace_id = ?", workspaceID).Order("s.id ASC").Scan(&shares).Error; err != nil {
		return nil, err
	}
	snapshot.Shares = make([]ShareSnapshot, 0, len(shares))
	for _, share := range shares {
		snapshot.Shares = append(snapshot.Shares, ShareSnapshot{
			ID: share.ID, SourceNodeID: share.SourceNodeID, Name: share.Name, RootType: share.RootType,
			RootName: share.RootName, ExpiresAt: share.ExpiresAt, MaxDownloads: share.MaxDownloads,
			DownloadCount: share.DownloadCount, Status: share.Status, CreatedBy: share.CreatedBy,
			CreatedAt: share.CreatedAt, UpdatedAt: share.UpdatedAt, RevokedAt: share.RevokedAt,
		})
	}
	if err := tx.Table("share_items AS si").
		Select("si.id, si.share_id, si.relative_path, si.name, si.version_no, si.storage_key, si.size, si.sha256, si.detected_mime, si.scan_status, si.created_at").
		Joins("JOIN shares s ON s.id = si.share_id").
		Joins("JOIN nodes n ON n.id = s.source_node_id AND n.workspace_id = s.workspace_id").
		Where("s.workspace_id = ?", workspaceID).
		Order("si.id ASC").Scan(&snapshot.ShareItems).Error; err != nil {
		return nil, err
	}
	if err := tx.Table("node_comment_mentions AS ncm").
		Select("ncm.comment_id, ncm.user_id, ncm.created_at").
		Joins("JOIN node_comments nc ON nc.id = ncm.comment_id").
		Where("nc.workspace_id = ?", workspaceID).
		Order("ncm.comment_id ASC, ncm.user_id ASC").Scan(&snapshot.CommentMentions).Error; err != nil {
		return nil, err
	}

	hash, err := metadataHash(snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.MetadataHash = hash
	return snapshot, nil
}

func metadataHash(snapshot *MetadataSnapshot) (string, error) {
	if snapshot == nil {
		return "", errors.New("metadata snapshot is missing")
	}
	provided := snapshot.MetadataHash
	snapshot.MetadataHash = ""
	data, err := json.Marshal(snapshot)
	snapshot.MetadataHash = provided
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateMetadata(snapshot *MetadataSnapshot, workspaceID uint) error {
	if snapshot == nil {
		return nil // Old manifests remain valid but cannot perform a full workspace restore.
	}
	if snapshot.SchemaVersion < minimumMetadataSchemaVersion || snapshot.SchemaVersion > metadataSchemaVersion || snapshot.Workspace.ID != workspaceID {
		return errors.New("backup metadata schema or workspace is invalid")
	}
	expected, err := metadataHash(snapshot)
	if snapshot.hashVerified {
		if err != nil || expected != snapshot.typedFingerprint {
			return errors.New("backup metadata changed after verification")
		}
	} else if err != nil || snapshot.MetadataHash == "" || !strings.EqualFold(expected, snapshot.MetadataHash) {
		return errors.New("backup metadata hash mismatch")
	}
	nodes := make(map[uint]model.Node, len(snapshot.Nodes))
	for _, node := range snapshot.Nodes {
		if node.ID == 0 || node.WorkspaceID != workspaceID {
			return errors.New("backup metadata contains an invalid node")
		}
		if _, exists := nodes[node.ID]; exists {
			return errors.New("backup metadata contains duplicate nodes")
		}
		nodes[node.ID] = node
	}
	for _, node := range snapshot.Nodes {
		if node.ParentID != nil {
			parent, exists := nodes[*node.ParentID]
			if !exists || parent.Type != "folder" || parent.ID == node.ID {
				return errors.New("backup metadata node parent is invalid")
			}
		}
		seen := map[uint]struct{}{node.ID: {}}
		current := node
		for current.ParentID != nil {
			if _, exists := seen[*current.ParentID]; exists {
				return errors.New("backup metadata node tree contains a cycle")
			}
			seen[*current.ParentID] = struct{}{}
			current = nodes[*current.ParentID]
		}
	}
	versions := make(map[uint]VersionSnapshot, len(snapshot.Versions))
	storageKeys := make(map[string]struct{}, len(snapshot.Versions))
	for _, version := range snapshot.Versions {
		node, exists := nodes[version.NodeID]
		if version.ID == 0 || !exists || node.Type != "file" || version.Size < 0 || strings.TrimSpace(version.StorageKey) == "" {
			return errors.New("backup metadata contains an invalid file version")
		}
		if _, exists := versions[version.ID]; exists {
			return errors.New("backup metadata contains duplicate file versions")
		}
		versions[version.ID] = version
		storageKeys[version.StorageKey] = struct{}{}
	}
	for _, node := range snapshot.Nodes {
		if node.ActiveVersion == nil {
			continue
		}
		version, exists := versions[*node.ActiveVersion]
		if !exists || version.NodeID != node.ID {
			return errors.New("backup metadata active file version is invalid")
		}
	}
	groups := make(map[uint]struct{}, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		groups[group.ID] = struct{}{}
	}
	roles := make(map[uint]struct{}, len(snapshot.Roles))
	for _, role := range snapshot.Roles {
		roles[role.ID] = struct{}{}
	}
	shares := make(map[uint]string, len(snapshot.Shares))
	for _, share := range snapshot.Shares {
		if _, exists := nodes[share.SourceNodeID]; !exists {
			return errors.New("backup metadata share source is invalid")
		}
		shares[share.ID] = share.Status
	}
	for _, acl := range snapshot.ACLs {
		if _, exists := nodes[acl.NodeID]; !exists || (acl.SubjectType == "group" && !containsID(groups, acl.SubjectID)) {
			return errors.New("backup metadata ACL reference is invalid")
		}
	}
	for _, relation := range snapshot.GroupMembers {
		if !containsID(groups, relation.GroupID) {
			return errors.New("backup metadata group member reference is invalid")
		}
	}
	for _, relation := range snapshot.RolePermissions {
		if !containsID(roles, relation.RoleID) || strings.TrimSpace(relation.PermissionCode) == "" {
			return errors.New("backup metadata role permission reference is invalid")
		}
	}
	for _, relation := range snapshot.UserRoles {
		if !containsID(roles, relation.RoleID) {
			return errors.New("backup metadata user role reference is invalid")
		}
	}
	for _, item := range snapshot.ShareItems {
		shareStatus, exists := shares[item.ShareID]
		if !exists {
			return errors.New("backup metadata share item reference is invalid")
		}
		if _, exists := storageKeys[item.StorageKey]; !exists && shareStatus == "active" {
			return fmt.Errorf("backup metadata share item %d has no file version", item.ID)
		}
	}
	comments := make(map[uint]struct{}, len(snapshot.Comments))
	for _, comment := range snapshot.Comments {
		if comment.ID == 0 || comment.WorkspaceID != workspaceID || comment.AuthorID == 0 || comment.Revision == 0 || strings.TrimSpace(comment.Content) == "" {
			return errors.New("backup metadata contains an invalid comment")
		}
		if _, exists := nodes[comment.NodeID]; !exists {
			return errors.New("backup metadata comment node reference is invalid")
		}
		if _, exists := comments[comment.ID]; exists {
			return errors.New("backup metadata contains duplicate comments")
		}
		comments[comment.ID] = struct{}{}
	}
	mentions := make(map[string]struct{}, len(snapshot.CommentMentions))
	for _, mention := range snapshot.CommentMentions {
		if mention.UserID == 0 {
			return errors.New("backup metadata contains an invalid comment mention")
		}
		if _, exists := comments[mention.CommentID]; !exists {
			return errors.New("backup metadata comment mention reference is invalid")
		}
		key := fmt.Sprintf("%d:%d", mention.CommentID, mention.UserID)
		if _, exists := mentions[key]; exists {
			return errors.New("backup metadata contains duplicate comment mentions")
		}
		mentions[key] = struct{}{}
	}
	return nil
}

func verifyDecodedMetadataHash(data []byte, snapshot *MetadataSnapshot) error {
	if snapshot == nil {
		return errors.New("backup metadata is missing")
	}
	blanked, provided, err := blankJSONStringField(data, "metadata_hash")
	if err != nil {
		return err
	}
	digest := sha256.Sum256(blanked)
	if provided == "" || !strings.EqualFold(provided, hex.EncodeToString(digest[:])) || !strings.EqualFold(provided, snapshot.MetadataHash) {
		return errors.New("backup metadata hash mismatch")
	}
	fingerprint, err := metadataHash(snapshot)
	if err != nil {
		return err
	}
	snapshot.hashVerified = true
	snapshot.typedFingerprint = fingerprint
	return nil
}

func blankJSONStringField(data []byte, field string) ([]byte, string, error) {
	var document map[string]json.RawMessage
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, "", err
	}
	rawValue, exists := document[field]
	if !exists {
		return nil, "", fmt.Errorf("%s is missing", field)
	}
	var provided string
	if err := json.Unmarshal(rawValue, &provided); err != nil {
		return nil, "", fmt.Errorf("%s must be a string", field)
	}
	marker := []byte(`"` + field + `"`)
	keyIndex := bytes.LastIndex(data, marker)
	if keyIndex < 0 {
		return nil, "", fmt.Errorf("%s is missing", field)
	}
	cursor := keyIndex + len(marker)
	for cursor < len(data) && (data[cursor] == ' ' || data[cursor] == '\t' || data[cursor] == '\r' || data[cursor] == '\n') {
		cursor++
	}
	if cursor >= len(data) || data[cursor] != ':' {
		return nil, "", fmt.Errorf("%s is malformed", field)
	}
	cursor++
	for cursor < len(data) && (data[cursor] == ' ' || data[cursor] == '\t' || data[cursor] == '\r' || data[cursor] == '\n') {
		cursor++
	}
	if cursor >= len(data) || data[cursor] != '"' {
		return nil, "", fmt.Errorf("%s must be a string", field)
	}
	valueStart := cursor
	escaped := false
	for cursor++; cursor < len(data); cursor++ {
		if escaped {
			escaped = false
			continue
		}
		if data[cursor] == '\\' {
			escaped = true
			continue
		}
		if data[cursor] == '"' {
			blanked := make([]byte, 0, len(data)-(cursor-valueStart-1))
			blanked = append(blanked, data[:valueStart+1]...)
			blanked = append(blanked, data[cursor:]...)
			return blanked, provided, nil
		}
	}
	return nil, "", fmt.Errorf("%s is malformed", field)
}

func containsID(values map[uint]struct{}, id uint) bool {
	_, exists := values[id]
	return exists
}
