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

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/security"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserDAO struct {
	db *gorm.DB
}

type UserRoleSummary struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
	Code string `json:"code"`
}

type UserProfileWorkspace struct {
	WorkspaceID     uint              `json:"workspace_id"`
	Name            string            `json:"name"`
	Code            string            `json:"code"`
	MembershipRole  string            `json:"membership_role"`
	IsMember        bool              `json:"is_member"`
	QuotaBytes      *int64            `json:"quota_bytes"`
	QuotaSource     string            `json:"quota_source"`
	UsedBytes       int64             `json:"used_bytes"`
	ReservedBytes   int64             `json:"reserved_bytes"`
	FunctionalRoles []UserRoleSummary `json:"functional_roles"`
}

var (
	ErrInvalidCurrentPassword = errors.New("current password is invalid")
	ErrPasswordUnchanged      = errors.New("new password must differ from current password")
	ErrLDAPAccountReadOnly    = errors.New("ldap account profile is read-only")
)

func NewUserDAO() *UserDAO {
	return &UserDAO{db: database.DB}
}

func (dao *UserDAO) Create(user *model.User) error {
	return dao.CreateWithAudit(user, nil)
}

// CreateWithAudit keeps the account row and its business audit event in one
// transaction. The password hash is deliberately excluded from the snapshot.
func (dao *UserDAO) CreateWithAudit(user *model.User, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(user).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		return appendAuditEvent(tx, event, nil, userAuditSnapshot(*user))
	})
}

func (dao *UserDAO) GetByID(id uint) (*model.User, error) {
	var user model.User
	err := dao.db.First(&user, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil, nil if not found
		}
		return nil, err
	}
	return &user, nil
}

func (dao *UserDAO) GetByUsername(username string) (*model.User, error) {
	var user model.User
	err := dao.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &user, nil
}

// GetByIDs returns the requested users keyed by ID. Missing users are omitted
// so historical records can still be rendered after an account is deleted.
func (dao *UserDAO) GetByIDs(ids []uint) (map[uint]model.User, error) {
	usersByID := make(map[uint]model.User, len(ids))
	if len(ids) == 0 {
		return usersByID, nil
	}
	var users []model.User
	if err := dao.db.Where("id IN ?", ids).Find(&users).Error; err != nil {
		return nil, err
	}
	for _, user := range users {
		usersByID[user.ID] = user
	}
	return usersByID, nil
}

func (dao *UserDAO) Update(user *model.User) error {
	return dao.UpdateWithAudit(user, nil)
}

func (dao *UserDAO) UpdateWithAudit(user *model.User, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&before, user.ID).Error; err != nil {
			return err
		}
		if err := tx.Save(user).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		if err := appendUserChange(tx, *user, "update"); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, userAuditSnapshot(before), userAuditSnapshot(*user))
	})
}

// ListPage returns users using the shared pagination contract. Password and
// authentication fields are excluded by the model JSON tags.
func (dao *UserDAO) ListPage(page, pageSize int, keyword string) (*pagination.PageResult[model.User], error) {
	return dao.listPage(page, pageSize, keyword, false)
}

func (dao *UserDAO) ListActivePage(page, pageSize int, keyword string) (*pagination.PageResult[model.User], error) {
	return dao.listPage(page, pageSize, keyword, true)
}

func (dao *UserDAO) listPage(page, pageSize int, keyword string, activeOnly bool) (*pagination.PageResult[model.User], error) {
	query := dao.db.Model(&model.User{}).Order("id ASC")
	if activeOnly {
		query = query.Where("status = ?", 1)
	}
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("username LIKE ? OR real_name LIKE ? OR email LIKE ?", like, like, like)
	}
	return pagination.Paging[model.User](query, page, pageSize)
}

func (dao *UserDAO) WorkspaceMetadata(workspaceID uint, userIDs []uint) (map[uint]bool, map[uint][]UserRoleSummary, error) {
	members := make(map[uint]bool, len(userIDs))
	roles := make(map[uint][]UserRoleSummary, len(userIDs))
	if workspaceID == 0 || len(userIDs) == 0 {
		return members, roles, nil
	}

	var memberIDs []uint
	if err := dao.db.Model(&model.WorkspaceMembership{}).
		Where("workspace_id = ? AND user_id IN ?", workspaceID, userIDs).
		Pluck("user_id", &memberIDs).Error; err != nil {
		return nil, nil, err
	}
	for _, userID := range memberIDs {
		members[userID] = true
	}

	var rows []struct {
		UserID uint
		RoleID uint
		Name   string
		Code   string
	}
	if err := dao.db.Table("user_roles").
		Select("user_roles.user_id, roles.id AS role_id, roles.name, roles.code").
		Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.workspace_id = user_roles.workspace_id").
		Where("user_roles.workspace_id = ? AND user_roles.user_id IN ?", workspaceID, userIDs).
		Order("roles.sort_order ASC, roles.id ASC").
		Scan(&rows).Error; err != nil {
		return nil, nil, err
	}
	for _, row := range rows {
		roles[row.UserID] = append(roles[row.UserID], UserRoleSummary{ID: row.RoleID, Name: row.Name, Code: row.Code})
	}
	return members, roles, nil
}

// ListProfileWorkspaces returns the caller's workspace membership, quota and
// functional roles without exposing other users' membership data.
func (dao *UserDAO) ListProfileWorkspaces(userID uint, isSuperAdmin bool) ([]UserProfileWorkspace, error) {
	type workspaceRow struct {
		WorkspaceID         uint
		Name                string
		Code                string
		MembershipRole      string
		IsMember            bool
		WorkspaceQuotaBytes *int64
		MemberQuotaBytes    *int64
		UsedBytes           int64
		ReservedBytes       int64
	}

	query := dao.db.Table("workspaces").
		Joins("LEFT JOIN workspace_memberships ON workspace_memberships.workspace_id = workspaces.id AND workspace_memberships.user_id = ?", userID).
		Where("workspaces.status = ?", 1)
	if !isSuperAdmin {
		query = query.Where("workspace_memberships.id IS NOT NULL")
	}

	var rows []workspaceRow
	if err := query.Select(`
		workspaces.id AS workspace_id,
		workspaces.name,
		workspaces.code,
		workspace_memberships.role AS membership_role,
		workspace_memberships.id IS NOT NULL AS is_member,
		workspaces.quota_bytes AS workspace_quota_bytes,
		workspace_memberships.quota_bytes AS member_quota_bytes,
		COALESCE(workspace_memberships.used_bytes, 0) AS used_bytes,
		COALESCE(workspace_memberships.reserved_bytes, 0) AS reserved_bytes`).
		Order("workspaces.id DESC").Scan(&rows).Error; err != nil {
		return nil, err
	}

	workspaceIDs := make([]uint, 0, len(rows))
	for _, row := range rows {
		workspaceIDs = append(workspaceIDs, row.WorkspaceID)
	}
	rolesByWorkspace := make(map[uint][]UserRoleSummary, len(rows))
	if len(workspaceIDs) > 0 {
		var roleRows []struct {
			WorkspaceID uint
			ID          uint
			Name        string
			Code        string
		}
		if err := dao.db.Table("user_roles").
			Select("user_roles.workspace_id, roles.id, roles.name, roles.code").
			Joins("JOIN roles ON roles.id = user_roles.role_id AND roles.workspace_id = user_roles.workspace_id").
			Where("user_roles.user_id = ? AND user_roles.workspace_id IN ? AND roles.status = ?", userID, workspaceIDs, 1).
			Order("roles.sort_order ASC, roles.id ASC").Scan(&roleRows).Error; err != nil {
			return nil, err
		}
		for _, row := range roleRows {
			rolesByWorkspace[row.WorkspaceID] = append(rolesByWorkspace[row.WorkspaceID], UserRoleSummary{ID: row.ID, Name: row.Name, Code: row.Code})
		}
	}

	result := make([]UserProfileWorkspace, 0, len(rows))
	for _, row := range rows {
		membershipRole := row.MembershipRole
		if membershipRole == "" && isSuperAdmin {
			membershipRole = "super_admin"
		}
		quota, quotaSource := row.MemberQuotaBytes, "member"
		if quota == nil {
			quota, quotaSource = row.WorkspaceQuotaBytes, "workspace"
		}
		if quota == nil {
			quotaSource = "unlimited"
		}
		roles := rolesByWorkspace[row.WorkspaceID]
		if roles == nil {
			roles = []UserRoleSummary{}
		}
		result = append(result, UserProfileWorkspace{
			WorkspaceID: row.WorkspaceID, Name: row.Name, Code: row.Code,
			MembershipRole: membershipRole, IsMember: row.IsMember,
			QuotaBytes: quota, QuotaSource: quotaSource,
			UsedBytes: row.UsedBytes, ReservedBytes: row.ReservedBytes,
			FunctionalRoles: roles,
		})
	}
	return result, nil
}

func (dao *UserDAO) UpdateFields(userID uint, fields map[string]any) error {
	return dao.UpdateFieldsWithAudit(userID, fields, nil)
}

func (dao *UserDAO) UpdateFieldsWithAudit(userID uint, fields map[string]any, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&before, userID).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).Updates(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		if err := appendUserChange(tx, user, "update"); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, userAuditSnapshot(before), userAuditSnapshot(user))
	})
}

func (dao *UserDAO) UpdateStatus(userID uint, status int) error {
	return dao.UpdateStatusWithAudit(userID, status, nil)
}

func (dao *UserDAO) UpdateStatusWithAudit(userID uint, status int, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&before, userID).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
			"status": status, "auth_version": gorm.Expr("auth_version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		if err := appendUserChange(tx, user, "update_status"); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, userAuditSnapshot(before), userAuditSnapshot(user))
	})
}

// Delete removes the account bindings before deleting the account itself.
// File and audit records intentionally remain immutable and retain their actor
// IDs for historical traceability.
func (dao *UserDAO) Delete(userID uint) error {
	return dao.DeleteWithAudit(userID, nil)
}

func (dao *UserDAO) DeleteWithAudit(userID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		if err := appendUserChange(tx, user, "delete"); err != nil {
			return err
		}
		for _, relation := range []any{&model.UserRole{}, &model.WorkspaceMembership{}, &model.UserGroupMember{}} {
			if err := tx.Where("user_id = ?", userID).Delete(relation).Error; err != nil {
				return err
			}
		}
		result := tx.Delete(&model.User{}, userID)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		return appendAuditEvent(tx, event, userAuditSnapshot(user), nil)
	})
}

// EnsureSuperAdmin creates the bootstrap account once. Keeping the bootstrap
// environment variable configured must not rotate its password on every start.
func (dao *UserDAO) EnsureSuperAdmin(username, passwordHash string) (bool, error) {
	var user model.User
	err := dao.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			user = model.User{
				Username:     username,
				PasswordHash: passwordHash,
				RealName:     "Super Admin",
				Status:       1,
				IsSuperAdmin: true,
			}
			return true, dao.db.Create(&user).Error
		}
		return false, err
	}
	if !user.IsSuperAdmin {
		return false, errors.New("bootstrap username already belongs to a non-super-admin user")
	}
	return false, nil
}

func (dao *UserDAO) HasSuperAdmin() (bool, error) {
	var count int64
	if err := dao.db.Model(&model.User{}).Where("is_super_admin = ? AND status = ?", true, 1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (dao *UserDAO) IncrementAuthVersion(userID uint) error {
	result := dao.db.Model(&model.User{}).
		Where("id = ?", userID).
		UpdateColumn("auth_version", gorm.Expr("auth_version + 1"))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// UpdatePassword hashes a new password and invalidates every existing session.
func (dao *UserDAO) UpdatePassword(userID uint, password string) error {
	return dao.UpdatePasswordWithAudit(userID, password, nil)
}

func (dao *UserDAO) UpdatePasswordWithAudit(userID uint, password string, event *model.OperationLog) error {
	passwordHash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&before, userID).Error; err != nil {
			return err
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
			"password_hash": passwordHash,
			"auth_version":  gorm.Expr("auth_version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		var user model.User
		if err := tx.First(&user, userID).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, user.ID, user.Username)
		if err := appendUserChange(tx, user, "password_reset"); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, userPasswordAuditSnapshot(before), userPasswordAuditSnapshot(user))
	})
}

// ChangeOwnPasswordWithAudit verifies the current credential and rotates the
// authentication version atomically. The returned user contains the new
// version used to issue a replacement cookie for the current browser only.
func (dao *UserDAO) ChangeOwnPasswordWithAudit(userID uint, currentPassword, newPassword string, event *model.OperationLog) (*model.User, error) {
	passwordHash, err := security.HashPassword(newPassword)
	if err != nil {
		return nil, err
	}
	var updated model.User
	err = dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.User
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&before, userID).Error; err != nil {
			return err
		}
		if before.Source == "ldap" {
			return ErrLDAPAccountReadOnly
		}
		if !security.CheckPasswordHash(currentPassword, before.PasswordHash) {
			return ErrInvalidCurrentPassword
		}
		if security.CheckPasswordHash(newPassword, before.PasswordHash) {
			return ErrPasswordUnchanged
		}
		result := tx.Model(&model.User{}).Where("id = ?", userID).Updates(map[string]any{
			"password_hash": passwordHash,
			"auth_version":  gorm.Expr("auth_version + 1"),
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := tx.First(&updated, userID).Error; err != nil {
			return err
		}
		prepareUserAuditEvent(event, updated.ID, updated.Username)
		if err := appendUserChange(tx, updated, "password_change"); err != nil {
			return err
		}
		return appendAuditEvent(tx, event, userPasswordAuditSnapshot(before), userPasswordAuditSnapshot(updated))
	})
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func prepareUserAuditEvent(event *model.OperationLog, userID uint, username string) {
	if event == nil {
		return
	}
	event.TargetType = "user"
	event.TargetID = strconv.FormatUint(uint64(userID), 10)
	if event.TargetName == "" {
		event.TargetName = username
	}
}

func userAuditSnapshot(user model.User) map[string]any {
	return map[string]any{
		"id": user.ID, "username": user.Username, "real_name": user.RealName,
		"email": user.Email, "phone": user.Phone, "status": user.Status,
		"source": user.Source, "is_super_admin": user.IsSuperAdmin,
		"auth_version":        user.AuthVersion,
		"password_configured": user.PasswordHash != "",
	}
}

func userPasswordAuditSnapshot(user model.User) map[string]any {
	snapshot := userAuditSnapshot(user)
	snapshot["password_changed"] = true
	return snapshot
}
