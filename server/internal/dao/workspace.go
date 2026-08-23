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
	"errors"
	"strconv"
	"strings"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type WorkspaceDAO struct {
	db *gorm.DB
}

type WorkspaceMentionCandidate struct {
	UserID       uint   `gorm:"column:user_id" json:"user_id"`
	Username     string `json:"username"`
	RealName     string `json:"real_name"`
	Role         string `json:"-"`
	IsSuperAdmin bool   `json:"-"`
}

var (
	ErrInvalidQuota         = errors.New("quota cannot be negative")
	ErrQuotaBelowUsage      = errors.New("quota is below used or reserved bytes")
	ErrInvalidWorkspaceRole = errors.New("role must be workspace_admin or member")
	ErrLastWorkspaceAdmin   = errors.New("workspace must retain at least one admin")
)

func NewWorkspaceDAO() *WorkspaceDAO {
	return &WorkspaceDAO{db: database.DB}
}

// CreateWorkspace 创建工作空间，并在同一个事务中将创建者添加为系统级 admin
func (dao *WorkspaceDAO) CreateWorkspace(ws *model.Workspace, ownerID, actorID uint) error {
	return dao.CreateWorkspaceWithAudit(ws, ownerID, actorID, nil)
}

func (dao *WorkspaceDAO) CreateWorkspaceWithAudit(ws *model.Workspace, ownerID, actorID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ws).Error; err != nil {
			return err
		}
		// 自动添加创建者为空间管理员 (内置角色 workspace_admin)
		member := &model.WorkspaceMembership{
			WorkspaceID: ws.ID,
			UserID:      ownerID,
			Role:        "workspace_admin",
			QuotaBytes:  nil,
			CreatedBy:   actorID,
		}
		if err := tx.Create(member).Error; err != nil {
			return err
		}
		if err := appendChange(tx, ws.ID, "workspace", ws.ID, "create", map[string]any{
			"name": ws.Name, "code": ws.Code, "owner_id": ownerID,
		}); err != nil {
			return err
		}
		prepareWorkspaceAuditEvent(event, ws.ID, ws.Name, "workspace")
		return appendAuditEvent(tx, event, nil, workspaceAuditSnapshot(*ws, ownerID))
	})
}

func (dao *WorkspaceDAO) GetByID(id uint) (*model.Workspace, error) {
	var ws model.Workspace
	err := dao.db.First(&ws, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ws, nil
}

func (dao *WorkspaceDAO) GetByCode(code string) (*model.Workspace, error) {
	var ws model.Workspace
	err := dao.db.Where("code = ?", code).First(&ws).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &ws, nil
}

// ListWorkspacesForUser 查询用户加入的所有空间，如果是超管，传入 isSuperAdmin = true 则获取全部
func (dao *WorkspaceDAO) ListWorkspacesForUser(userID uint, isSuperAdmin bool) ([]model.WorkspaceAccessView, error) {
	var workspaces []model.WorkspaceAccessView
	if isSuperAdmin {
		err := dao.db.Model(&model.Workspace{}).
			Select("workspaces.*, workspace_memberships.role AS current_role, workspace_memberships.id IS NOT NULL AS is_member").
			Joins("LEFT JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id AND workspace_memberships.user_id = ?", userID).
			Where("workspaces.status = ?", 1).Order("workspaces.id DESC").Scan(&workspaces).Error
		for index := range workspaces {
			if workspaces[index].CurrentRole == "" {
				workspaces[index].CurrentRole = "super_admin"
			}
		}
		return workspaces, err
	}

	err := dao.db.Model(&model.Workspace{}).
		Select("workspaces.*, workspace_memberships.role AS current_role, TRUE AS is_member").
		Joins("JOIN workspace_memberships on workspace_memberships.workspace_id = workspaces.id").
		Where("workspace_memberships.user_id = ? AND workspaces.status = ?", userID, 1).
		Order("workspaces.id DESC").
		Scan(&workspaces).Error

	return workspaces, err
}

func (dao *WorkspaceDAO) ListPageForUser(userID uint, isSuperAdmin bool, page, pageSize int, keyword string) (*pagination.PageResult[model.WorkspaceAccessView], error) {
	query := dao.db.Model(&model.Workspace{}).Where("workspaces.status = ?", 1)
	if !isSuperAdmin {
		query = query.Joins("JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id").
			Where("workspace_memberships.user_id = ?", userID)
	}
	if keyword != "" {
		prefix := keyword + "%"
		query = query.Where("(workspaces.name LIKE ? OR workspaces.code LIKE ?)", prefix, prefix)
	}
	query = query.Order("workspaces.id DESC")
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, err
	}
	dataQuery := query.Select("workspaces.*")
	if isSuperAdmin {
		dataQuery = dataQuery.Select("workspaces.*, workspace_memberships.role AS current_role, workspace_memberships.id IS NOT NULL AS is_member").
			Joins("LEFT JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id AND workspace_memberships.user_id = ?", userID)
	} else {
		dataQuery = dataQuery.Select("workspaces.*, workspace_memberships.role AS current_role, TRUE AS is_member")
	}
	var list []model.WorkspaceAccessView
	if err := dataQuery.Offset((page - 1) * pageSize).Limit(pageSize).Find(&list).Error; err != nil {
		return nil, err
	}
	result := &pagination.PageResult[model.WorkspaceAccessView]{
		Total: total, List: list, Page: page, PageSize: pageSize,
	}
	if isSuperAdmin {
		for index := range result.List {
			if result.List[index].CurrentRole == "" {
				result.List[index].CurrentRole = "super_admin"
			}
		}
	}
	return result, nil
}

// AddMember 向工作空间添加成员，或更新已有成员的角色/配额
func (dao *WorkspaceDAO) UpsertMember(membership *model.WorkspaceMembership) error {
	return dao.UpsertMemberWithAudit(membership, nil)
}

func (dao *WorkspaceDAO) UpsertMemberWithAudit(membership *model.WorkspaceMembership, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, membership.WorkspaceID).Error; err != nil {
			return err
		}
		if membership.Role != "workspace_admin" && membership.Role != "member" {
			return ErrInvalidWorkspaceRole
		}
		if membership.QuotaBytes != nil && *membership.QuotaBytes < 0 {
			return ErrInvalidQuota
		}
		var existing model.WorkspaceMembership
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND user_id = ?", membership.WorkspaceID, membership.UserID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(membership).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.User{}).Where("id = ?", membership.UserID).
				UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
				return err
			}
			if err := appendChange(tx, membership.WorkspaceID, "workspace_membership", membership.UserID, "create", membership); err != nil {
				return err
			}
			var user model.User
			if err := tx.First(&user, membership.UserID).Error; err != nil {
				return err
			}
			if err := appendUserChange(tx, user, "snapshot"); err != nil {
				return err
			}
			prepareWorkspaceMemberAuditEvent(event, *membership, user.Username)
			return appendAuditEvent(tx, event, nil, workspaceMembershipAuditSnapshot(*membership))
		}
		if err != nil {
			return err
		}
		var adminCount int64
		if err := tx.Model(&model.WorkspaceMembership{}).
			Where("workspace_id = ? AND role = ?", membership.WorkspaceID, "workspace_admin").Count(&adminCount).Error; err != nil {
			return err
		}
		if err := validateWorkspaceRoleTransition(existing.Role, membership.Role, adminCount); err != nil {
			return err
		}
		if membership.QuotaBytes != nil && *membership.QuotaBytes < existing.UsedBytes+existing.ReservedBytes {
			return ErrQuotaBelowUsage
		}
		if err := tx.Model(&existing).Updates(map[string]any{
			"role": membership.Role, "quota_bytes": membership.QuotaBytes, "created_by": membership.CreatedBy,
		}).Error; err != nil {
			return err
		}
		if err := appendChange(tx, membership.WorkspaceID, "workspace_membership", membership.UserID, "update", map[string]any{
			"role": membership.Role, "quota_bytes": membership.QuotaBytes,
		}); err != nil {
			return err
		}
		if existing.Role != membership.Role {
			if err := tx.Model(&model.User{}).Where("id = ?", membership.UserID).
				UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
				return err
			}
		}
		var user model.User
		if err := tx.First(&user, membership.UserID).Error; err != nil {
			return err
		}
		if err := appendUserChange(tx, user, "snapshot"); err != nil {
			return err
		}
		membership.ID = existing.ID
		membership.UsedBytes = existing.UsedBytes
		membership.ReservedBytes = existing.ReservedBytes
		membership.JoinedAt = existing.JoinedAt
		prepareWorkspaceMemberAuditEvent(event, *membership, user.Username)
		return appendAuditEvent(tx, event, workspaceMembershipAuditSnapshot(existing), workspaceMembershipAuditSnapshot(*membership))
	})
}

func (dao *WorkspaceDAO) EnsureMember(workspaceID, userID, actorID uint) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		var existing model.WorkspaceMembership
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&existing).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		membership := &model.WorkspaceMembership{
			WorkspaceID: workspaceID,
			UserID:      userID,
			Role:        "member",
			QuotaBytes:  nil,
			CreatedBy:   actorID,
		}
		if err := tx.Create(membership).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "workspace_membership", userID, "create", membership); err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		return appendUserChange(tx, user, "snapshot")
	})
}

func validateWorkspaceRoleTransition(currentRole, nextRole string, adminCount int64) error {
	if nextRole != "workspace_admin" && nextRole != "member" {
		return ErrInvalidWorkspaceRole
	}
	if currentRole == "workspace_admin" && nextRole != "workspace_admin" && adminCount <= 1 {
		return ErrLastWorkspaceAdmin
	}
	return nil
}

func (dao *WorkspaceDAO) UpdateQuota(workspaceID uint, quotaBytes *int64) error {
	return dao.UpdateQuotaWithAudit(workspaceID, quotaBytes, nil)
}

func (dao *WorkspaceDAO) UpdateQuotaWithAudit(workspaceID uint, quotaBytes *int64, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if quotaBytes != nil && *quotaBytes < 0 {
			return ErrInvalidQuota
		}
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		before := workspaceAuditSnapshot(workspace, 0)
		if quotaBytes != nil && *quotaBytes < workspace.UsedBytes+workspace.ReservedBytes {
			return ErrQuotaBelowUsage
		}
		if err := tx.Model(&workspace).Updates(map[string]any{"quota_bytes": quotaBytes}).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "workspace", workspaceID, "update_quota", map[string]any{"quota_bytes": quotaBytes}); err != nil {
			return err
		}
		prepareWorkspaceAuditEvent(event, workspaceID, workspace.Name, "workspace")
		workspace.QuotaBytes = quotaBytes
		return appendAuditEvent(tx, event, before, workspaceAuditSnapshot(workspace, 0))
	})
}

func (dao *WorkspaceDAO) UpdateMemberQuota(workspaceID, userID uint, quotaBytes *int64) error {
	return dao.UpdateMemberQuotaWithAudit(workspaceID, userID, quotaBytes, nil)
}

func (dao *WorkspaceDAO) UpdateMemberQuotaWithAudit(workspaceID, userID uint, quotaBytes *int64, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if quotaBytes != nil && *quotaBytes < 0 {
			return ErrInvalidQuota
		}
		var membership model.WorkspaceMembership
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&membership).Error; err != nil {
			return err
		}
		before := workspaceMembershipAuditSnapshot(membership)
		if quotaBytes != nil && *quotaBytes < membership.UsedBytes+membership.ReservedBytes {
			return ErrQuotaBelowUsage
		}
		if err := tx.Model(&membership).Updates(map[string]any{"quota_bytes": quotaBytes}).Error; err != nil {
			return err
		}
		membership.QuotaBytes = quotaBytes
		if err := appendChange(tx, workspaceID, "workspace_membership", userID, "update_quota", map[string]any{"quota_bytes": quotaBytes}); err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		prepareWorkspaceMemberAuditEvent(event, membership, user.Username)
		return appendAuditEvent(tx, event, before, workspaceMembershipAuditSnapshot(membership))
	})
}

func (dao *WorkspaceDAO) GetMembership(workspaceID, userID uint) (*model.WorkspaceMembership, error) {
	var m model.WorkspaceMembership
	err := dao.db.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

func (dao *WorkspaceDAO) ListMembersPage(workspaceID uint, page, pageSize int, keyword string) (*pagination.PageResult[model.WorkspaceMemberView], error) {
	query := dao.db.Model(&model.WorkspaceMembership{}).
		Select("users.id AS user_id, users.username, users.real_name, users.email, users.status, workspace_memberships.role, workspace_memberships.quota_bytes, workspace_memberships.used_bytes, workspace_memberships.reserved_bytes, workspace_memberships.joined_at").
		Joins("JOIN users ON users.id = workspace_memberships.user_id").
		Where("workspace_memberships.workspace_id = ?", workspaceID).
		Order("workspace_memberships.role ASC, users.username ASC")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("users.username LIKE ? OR users.real_name LIKE ? OR users.email LIKE ?", like, like, like)
	}
	return pagination.Paging[model.WorkspaceMemberView](query, page, pageSize)
}

// ListMentionCandidates returns only active members and excludes contact
// fields that are irrelevant to the collaboration composer.
func (dao *WorkspaceDAO) ListMentionCandidates(workspaceID uint, keyword string, limit int) ([]WorkspaceMentionCandidate, error) {
	if limit < 1 || limit > 50 {
		limit = 20
	}
	query := dao.db.Table("workspace_memberships").
		Select("users.id AS user_id, users.username, users.real_name, workspace_memberships.role, users.is_super_admin").
		Joins("JOIN users ON users.id = workspace_memberships.user_id").
		Where("workspace_memberships.workspace_id = ? AND users.status = ?", workspaceID, 1)
	if value := strings.TrimSpace(keyword); value != "" {
		like := "%" + escapeSearchLike(value) + "%"
		query = query.Where("(users.username LIKE ? ESCAPE '\\\\' OR users.real_name LIKE ? ESCAPE '\\\\')", like, like)
	}
	var candidates []WorkspaceMentionCandidate
	err := query.Order("users.username ASC").Limit(limit).Scan(&candidates).Error
	return candidates, err
}

func (dao *WorkspaceDAO) FindMentionCandidatesByUsernames(workspaceID uint, usernames []string) ([]WorkspaceMentionCandidate, error) {
	normalized := make([]string, 0, len(usernames))
	seen := make(map[string]struct{}, len(usernames))
	for _, username := range usernames {
		value := strings.ToLower(strings.TrimSpace(username))
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	if len(normalized) == 0 {
		return []WorkspaceMentionCandidate{}, nil
	}
	var candidates []WorkspaceMentionCandidate
	err := dao.db.Table("workspace_memberships").
		Select("users.id AS user_id, users.username, users.real_name, workspace_memberships.role, users.is_super_admin").
		Joins("JOIN users ON users.id = workspace_memberships.user_id").
		Where("workspace_memberships.workspace_id = ? AND users.status = ? AND LOWER(users.username) IN ?", workspaceID, 1, normalized).
		Order("users.username ASC").Scan(&candidates).Error
	return candidates, err
}

func (dao *WorkspaceDAO) RemoveMember(workspaceID, userID uint) error {
	return dao.RemoveMemberWithAudit(workspaceID, userID, nil)
}

func (dao *WorkspaceDAO) RemoveMemberWithAudit(workspaceID, userID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var workspace model.Workspace
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&workspace, workspaceID).Error; err != nil {
			return err
		}
		var membership model.WorkspaceMembership
		if err := tx.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).First(&membership).Error; err != nil {
			return err
		}
		if membership.Role == "workspace_admin" {
			var adminCount int64
			if err := tx.Model(&model.WorkspaceMembership{}).Where("workspace_id = ? AND role = ?", workspaceID, "workspace_admin").Count(&adminCount).Error; err != nil {
				return err
			}
			if adminCount <= 1 {
				return errors.New("工作空间至少需要保留一名管理员")
			}
		}
		if err := tx.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
			return err
		}
		if err := tx.Delete(&membership).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "workspace_membership", userID, "delete", map[string]any{"user_id": userID}); err != nil {
			return err
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		prepareWorkspaceMemberAuditEvent(event, membership, user.Username)
		return appendAuditEvent(tx, event, workspaceMembershipAuditSnapshot(membership), nil)
	})
}

func prepareWorkspaceAuditEvent(event *model.OperationLog, workspaceID uint, name, targetType string) {
	if event == nil {
		return
	}
	event.TargetType = targetType
	event.TargetID = strconv.FormatUint(uint64(workspaceID), 10)
	if event.TargetName == "" {
		event.TargetName = name
	}
}

func prepareWorkspaceMemberAuditEvent(event *model.OperationLog, membership model.WorkspaceMembership, username string) {
	if event == nil {
		return
	}
	event.TargetType = "workspace_membership"
	event.TargetID = strconv.FormatUint(uint64(membership.UserID), 10)
	if event.TargetName == "" {
		event.TargetName = username
	}
}

func workspaceAuditSnapshot(workspace model.Workspace, ownerID uint) map[string]any {
	snapshot := map[string]any{
		"id": workspace.ID, "uuid": workspace.UUID, "name": workspace.Name,
		"code": workspace.Code, "description": workspace.Description, "status": workspace.Status,
		"quota_bytes": workspace.QuotaBytes, "used_bytes": workspace.UsedBytes,
		"reserved_bytes": workspace.ReservedBytes, "created_by": workspace.CreatedBy,
	}
	if ownerID > 0 {
		snapshot["owner_id"] = ownerID
	}
	return snapshot
}

func workspaceMembershipAuditSnapshot(membership model.WorkspaceMembership) map[string]any {
	return map[string]any{
		"id": membership.ID, "workspace_id": membership.WorkspaceID, "user_id": membership.UserID,
		"role": membership.Role, "quota_bytes": membership.QuotaBytes, "used_bytes": membership.UsedBytes,
		"reserved_bytes": membership.ReservedBytes, "created_by": membership.CreatedBy,
	}
}
