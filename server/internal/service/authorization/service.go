/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package authorization

import (
	"errors"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/ldapdn"
)

const (
	AccessRead      = "read"
	AccessReadWrite = "read_write"
	AccessAdmin     = "admin"
)

var ErrNodeNotFound = errors.New("node not found in workspace")

type Actor struct {
	UserID        uint
	WorkspaceID   uint
	IsSuperAdmin  bool
	WorkspaceRole string
}

type Service struct {
	nodes  *dao.NodeDAO
	groups *dao.GroupDAO
	acls   *dao.ACLDAO
}

type AccessSource struct {
	Type            string   `json:"type"`
	ID              uint     `json:"id"`
	Name            string   `json:"name"`
	DirectorySource string   `json:"directory_source"`
	DirectoryPath   []string `json:"directory_path"`
	GrantedLevel    string   `json:"granted_level"`
	Inherited       bool     `json:"inherited"`
	SourceNodeID    uint     `json:"source_node_id,omitempty"`
	SourceNodeName  string   `json:"source_node_name,omitempty"`
}

type AccessSummary struct {
	EffectiveAccessLevel string         `json:"effective_access_level"`
	Sources              []AccessSource `json:"sources"`
}

func NewService() *Service {
	return &Service{nodes: dao.NewNodeDAO(), groups: dao.NewGroupDAO(), acls: dao.NewACLDAO()}
}

func (s *Service) CanRead(actor Actor, nodeID uint) (bool, error) {
	return s.authorize(actor, nodeID, AccessRead)
}

func (s *Service) CanWrite(actor Actor, nodeID uint) (bool, error) {
	return s.authorize(actor, nodeID, AccessReadWrite)
}

func (s *Service) CanManageACL(actor Actor, nodeID uint) (bool, error) {
	return s.authorize(actor, nodeID, AccessAdmin)
}

// CanCreateShare checks the node-level half of share authorization. The route
// separately requires file:share:create, so a read-write user needs both grants.
func (s *Service) CanCreateShare(actor Actor, nodeID uint) (bool, error) {
	return s.authorize(actor, nodeID, AccessReadWrite)
}

func (s *Service) CanRestore(actor Actor, nodeID uint) (bool, error) {
	return s.authorizeWithState(actor, nodeID, AccessReadWrite, "trashed")
}

// ReadableNodeIDs evaluates a workspace-scoped candidate set in bulk. Callers
// must obtain nodeIDs from a query already restricted to actor.WorkspaceID and
// active nodes.
func (s *Service) ReadableNodeIDs(actor Actor, nodeIDs []uint) (map[uint]bool, error) {
	levels, err := s.NodeAccessLevels(actor, nodeIDs)
	if err != nil {
		return nil, err
	}
	allowed := make(map[uint]bool, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		allowed[nodeID] = levels[nodeID] != ""
	}
	return allowed, nil
}

// NodeAccessLevels resolves the effective level for a workspace-scoped
// candidate set without exposing raw ACL precedence rules to handlers.
func (s *Service) NodeAccessLevels(actor Actor, nodeIDs []uint) (map[uint]string, error) {
	levels := make(map[uint]string, len(nodeIDs))
	if actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin" {
		for _, nodeID := range nodeIDs {
			levels[nodeID] = AccessAdmin
		}
		return levels, nil
	}
	groupIDs, err := s.groups.ListUserGroupIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return nil, err
	}
	entriesByNode, err := s.acls.ListEffectiveEntriesForNodes(actor.WorkspaceID, actor.UserID, groupIDs, nodeIDs)
	if err != nil {
		return nil, err
	}
	for _, nodeID := range nodeIDs {
		levels[nodeID] = ResolveAccessLevel(entriesByNode[nodeID])
	}
	return levels, nil
}

func (s *Service) AccessSummary(actor Actor, nodeID uint) (AccessSummary, error) {
	if actor.IsSuperAdmin {
		return roleAccessSummary("平台管理员"), nil
	}
	if actor.WorkspaceRole == "workspace_admin" {
		return roleAccessSummary("空间管理员"), nil
	}
	groupIDs, err := s.groups.ListUserGroupIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return AccessSummary{}, err
	}
	entries, err := s.acls.ListEffectiveSources(actor.WorkspaceID, nodeID, actor.UserID, groupIDs)
	if err != nil {
		return AccessSummary{}, err
	}
	level, effective := resolveEffectiveSources(entries)
	summary := AccessSummary{EffectiveAccessLevel: level, Sources: make([]AccessSource, 0, len(effective))}
	for _, entry := range effective {
		path := []string{}
		if entry.SubjectType == "group" && entry.SubjectSource == "ldap" {
			path = ldapdn.OrganizationalPath(entry.SubjectLDAPDN)
		}
		summary.Sources = append(summary.Sources, AccessSource{
			Type: entry.SubjectType, ID: entry.SubjectID, Name: entry.SubjectName,
			DirectorySource: entry.SubjectSource, DirectoryPath: path,
			GrantedLevel: entry.AccessLevel, Inherited: entry.Depth > 0,
			SourceNodeID: entry.SourceNodeID, SourceNodeName: entry.SourceNodeName,
		})
	}
	return summary, nil
}

func roleAccessSummary(name string) AccessSummary {
	return AccessSummary{
		EffectiveAccessLevel: AccessAdmin,
		Sources:              []AccessSource{{Type: "role", Name: name, GrantedLevel: AccessAdmin, DirectoryPath: []string{}}},
	}
}

func resolveEffectiveSources(entries []dao.EffectiveACLSource) (string, []dao.EffectiveACLSource) {
	for start := 0; start < len(entries); {
		end := start + 1
		for end < len(entries) && entries[end].Depth == entries[start].Depth {
			end++
		}
		atDepth := entries[start:end]
		decisive := sourceEntries(atDepth, "user")
		if len(decisive) == 0 {
			decisive = sourceEntries(atDepth, "group")
		}
		raw := make([]dao.EffectiveACLEntry, 0, len(decisive))
		for _, entry := range decisive {
			raw = append(raw, dao.EffectiveACLEntry{
				Depth: entry.Depth, SubjectType: entry.SubjectType, SubjectID: entry.SubjectID,
				Effect: entry.Effect, AccessLevel: entry.AccessLevel,
			})
		}
		if level := ResolveAccessLevel(raw); level != "" {
			allowed := make([]dao.EffectiveACLSource, 0, len(decisive))
			for _, entry := range decisive {
				if entry.Effect == "allow" {
					allowed = append(allowed, entry)
				}
			}
			return level, allowed
		}
		return "", []dao.EffectiveACLSource{}
	}
	return "", []dao.EffectiveACLSource{}
}

func sourceEntries(entries []dao.EffectiveACLSource, subjectType string) []dao.EffectiveACLSource {
	result := make([]dao.EffectiveACLSource, 0, len(entries))
	for _, entry := range entries {
		if entry.SubjectType == subjectType {
			result = append(result, entry)
		}
	}
	return result
}

func (s *Service) authorize(actor Actor, nodeID uint, required string) (bool, error) {
	return s.authorizeWithState(actor, nodeID, required, "active")
}

func (s *Service) authorizeWithState(actor Actor, nodeID uint, required, expectedStatus string) (bool, error) {
	node, err := s.nodes.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		return false, err
	}
	if node == nil || node.Status != expectedStatus {
		return false, ErrNodeNotFound
	}
	if actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin" {
		return true, nil
	}
	groupIDs, err := s.groups.ListUserGroupIDs(actor.WorkspaceID, actor.UserID)
	if err != nil {
		return false, err
	}
	entries, err := s.acls.ListEffectiveEntries(actor.WorkspaceID, nodeID, actor.UserID, groupIDs)
	if err != nil {
		return false, err
	}
	return Resolve(entries, required), nil
}

func Resolve(entries []dao.EffectiveACLEntry, required string) bool {
	for start := 0; start < len(entries); {
		end := start + 1
		for end < len(entries) && entries[end].Depth == entries[start].Depth {
			end++
		}
		atDepth := entries[start:end]
		if direct := subjectEntries(atDepth, "user"); len(direct) > 0 {
			return entriesAllow(direct, required)
		}
		if groups := subjectEntries(atDepth, "group"); len(groups) > 0 {
			return entriesAllow(groups, required)
		}
		start = end
	}
	return false
}

func ResolveAccessLevel(entries []dao.EffectiveACLEntry) string {
	for _, level := range []string{AccessAdmin, AccessReadWrite, AccessRead} {
		if Resolve(entries, level) {
			return level
		}
	}
	return ""
}

func subjectEntries(entries []dao.EffectiveACLEntry, subjectType string) []dao.EffectiveACLEntry {
	result := make([]dao.EffectiveACLEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.SubjectType == subjectType {
			result = append(result, entry)
		}
	}
	return result
}

func entriesAllow(entries []dao.EffectiveACLEntry, required string) bool {
	for _, entry := range entries {
		if entry.Effect == "deny" {
			return false
		}
	}
	requiredRank := accessRank(required)
	for _, entry := range entries {
		if entry.Effect == "allow" && accessRank(entry.AccessLevel) >= requiredRank {
			return true
		}
	}
	return false
}

func accessRank(level string) int {
	switch level {
	case AccessRead:
		return 1
	case AccessReadWrite:
		return 2
	case AccessAdmin:
		return 3
	default:
		return 100
	}
}
