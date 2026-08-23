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
	"fmt"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RoleDAO struct {
	db *gorm.DB
}

// ErrUserNotWorkspaceMember protects the role binding boundary even when a
// caller reaches the DAO outside the HTTP handler. Role assignments must
// never create a user_role row for a user who is not a member of the target
// workspace.
var ErrUserNotWorkspaceMember = errors.New("user is not a member of this workspace")

type PermissionDefinition struct {
	Code string
	Name string
}

var BuiltinPermissions = []PermissionDefinition{
	{Code: "file:list", Name: "查看文件"},
	{Code: "file:upload", Name: "上传文件和创建目录"},
	{Code: "file:download", Name: "下载文件"},
	{Code: "file:delete", Name: "删除文件"},
	{Code: "file:restore", Name: "恢复文件"},
	{Code: "file:share:create", Name: "创建分享链接"},
	{Code: "acl:manage", Name: "管理目录权限"},
	{Code: "workspace:user:manage", Name: "管理工作空间成员"},
	{Code: "workspace:role:manage", Name: "管理工作空间角色"},
	{Code: "workspace:config:manage", Name: "管理工作空间配置"},
	{Code: "backup:manage", Name: "管理备份与恢复"},
	{Code: "audit:list", Name: "查看审计事件"},
	{Code: "audit:export", Name: "导出审计事件"},
	{Code: "audit:archive", Name: "执行审计归档"},
}

func NewRoleDAO() *RoleDAO {
	return &RoleDAO{db: database.DB}
}

func (dao *RoleDAO) EnsurePermissionDefinitions() error {
	for _, definition := range BuiltinPermissions {
		permission := model.Permission{Code: definition.Code, Name: definition.Name}
		if err := dao.db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "code"}},
			DoUpdates: clause.AssignmentColumns([]string{"name"}),
		}).Create(&permission).Error; err != nil {
			return err
		}
	}
	return nil
}

func (dao *RoleDAO) ListPermissions() ([]model.Permission, error) {
	var permissions []model.Permission
	err := dao.db.Order("code ASC").Find(&permissions).Error
	return permissions, err
}

func (dao *RoleDAO) List(workspaceID uint) ([]model.Role, error) {
	var roles []model.Role
	err := dao.db.Where("workspace_id = ?", workspaceID).Order("sort_order ASC, id ASC").Find(&roles).Error
	return roles, err
}

func (dao *RoleDAO) ListPage(workspaceID uint, page, pageSize int, keyword string) (*pagination.PageResult[model.Role], error) {
	query := dao.db.Model(&model.Role{}).Where("workspace_id = ?", workspaceID)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(code LIKE ? OR name LIKE ?)", like, like)
	}
	query = query.Order("sort_order ASC, id ASC")
	return pagination.Paging[model.Role](query, page, pageSize)
}

func (dao *RoleDAO) Get(workspaceID, roleID uint) (*model.Role, error) {
	var role model.Role
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, roleID).First(&role).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &role, nil
}

func (dao *RoleDAO) GetWithPermissions(workspaceID, roleID uint) (*model.Role, []model.Permission, error) {
	role, err := dao.Get(workspaceID, roleID)
	if err != nil || role == nil {
		return role, nil, err
	}
	var permissions []model.Permission
	err = dao.db.Model(&model.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.code ASC").Find(&permissions).Error
	return role, permissions, err
}

func (dao *RoleDAO) Create(role *model.Role) error {
	return dao.CreateWithAudit(role, nil)
}

func (dao *RoleDAO) CreateWithAudit(role *model.Role, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(role).Error; err != nil {
			return err
		}
		if err := appendChange(tx, role.WorkspaceID, "role", role.ID, "create", role); err != nil {
			return err
		}
		prepareRoleAuditEvent(event, role)
		return appendAuditEvent(tx, event, nil, roleAuditSnapshot(role))
	})
}

func (dao *RoleDAO) Update(workspaceID, roleID uint, updates map[string]any) error {
	return dao.UpdateWithAudit(workspaceID, roleID, updates, nil)
}

func (dao *RoleDAO) UpdateWithAudit(workspaceID, roleID uint, updates map[string]any, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.Role
		if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, roleID).First(&before).Error; err != nil {
			return err
		}
		result := tx.Model(&model.Role{}).Where("workspace_id = ? AND id = ?", workspaceID, roleID).Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := incrementRoleUsersAuthVersion(tx, workspaceID, roleID); err != nil {
			return err
		}
		var after model.Role
		if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, roleID).First(&after).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "role", roleID, "update", updates); err != nil {
			return err
		}
		prepareRoleAuditEvent(event, &after)
		return appendAuditEvent(tx, event, roleAuditSnapshot(&before), roleAuditSnapshot(&after))
	})
}

func (dao *RoleDAO) Delete(workspaceID, roleID uint) error {
	return dao.DeleteWithAudit(workspaceID, roleID, nil)
}

func (dao *RoleDAO) DeleteWithAudit(workspaceID, roleID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.Role
		if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, roleID).First(&before).Error; err != nil {
			return err
		}
		if err := incrementRoleUsersAuthVersion(tx, workspaceID, roleID); err != nil {
			return err
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id = ? AND role_id = ?", workspaceID, roleID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		result := tx.Where("workspace_id = ? AND id = ?", workspaceID, roleID).Delete(&model.Role{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := appendChange(tx, workspaceID, "role", roleID, "delete", map[string]any{"role_id": roleID}); err != nil {
			return err
		}
		prepareRoleAuditEvent(event, &before)
		return appendAuditEvent(tx, event, roleAuditSnapshot(&before), nil)
	})
}

func (dao *RoleDAO) ReplacePermissions(workspaceID, roleID uint, codes []string) error {
	return dao.ReplacePermissionsWithAudit(workspaceID, roleID, codes, nil)
}

func (dao *RoleDAO) ReplacePermissionsWithAudit(workspaceID, roleID uint, codes []string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var role model.Role
		if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, roleID).First(&role).Error; err != nil {
			return err
		}
		beforeCodes, err := rolePermissionCodes(tx, roleID)
		if err != nil {
			return err
		}
		var permissions []model.Permission
		if len(codes) > 0 {
			if err := tx.Where("code IN ?", codes).Find(&permissions).Error; err != nil {
				return err
			}
			if len(permissions) != len(codes) {
				return errors.New("one or more permission codes are invalid")
			}
		}
		if err := tx.Where("role_id = ?", roleID).Delete(&model.RolePermission{}).Error; err != nil {
			return err
		}
		bindings := make([]model.RolePermission, 0, len(permissions))
		for _, permission := range permissions {
			bindings = append(bindings, model.RolePermission{RoleID: roleID, PermissionID: permission.ID})
		}
		if len(bindings) > 0 {
			if err := tx.Create(&bindings).Error; err != nil {
				return err
			}
		}
		if err := incrementRoleUsersAuthVersion(tx, workspaceID, roleID); err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "role_permission", roleID, "replace", map[string]any{"permission_codes": codes}); err != nil {
			return err
		}
		prepareRoleAuditEvent(event, &role)
		return appendAuditEvent(tx, event, map[string]any{"permission_codes": beforeCodes}, map[string]any{"permission_codes": codes})
	})
}

func (dao *RoleDAO) AssignUserRoles(workspaceID, userID uint, roleIDs []uint) error {
	return dao.AssignUserRolesWithAudit(workspaceID, userID, roleIDs, nil)
}

func (dao *RoleDAO) AssignUserRolesWithAudit(workspaceID, userID uint, roleIDs []uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var membershipCount int64
		if err := tx.Model(&model.WorkspaceMembership{}).
			Where("workspace_id = ? AND user_id = ?", workspaceID, userID).
			Count(&membershipCount).Error; err != nil {
			return err
		}
		if membershipCount != 1 {
			return ErrUserNotWorkspaceMember
		}
		var beforeRoleIDs []uint
		if err := tx.Model(&model.UserRole{}).Where("workspace_id = ? AND user_id = ?", workspaceID, userID).Order("role_id ASC").Pluck("role_id", &beforeRoleIDs).Error; err != nil {
			return err
		}
		if len(roleIDs) > 0 {
			var count int64
			if err := tx.Model(&model.Role{}).Where("workspace_id = ? AND id IN ? AND status = ?", workspaceID, roleIDs, 1).Count(&count).Error; err != nil {
				return err
			}
			if count != int64(len(roleIDs)) {
				return errors.New("one or more roles are invalid for this workspace")
			}
		}
		if err := tx.Where("workspace_id = ? AND user_id = ?", workspaceID, userID).Delete(&model.UserRole{}).Error; err != nil {
			return err
		}
		bindings := make([]model.UserRole, 0, len(roleIDs))
		for _, roleID := range roleIDs {
			bindings = append(bindings, model.UserRole{WorkspaceID: workspaceID, UserID: userID, RoleID: roleID})
		}
		if len(bindings) > 0 {
			if err := tx.Create(&bindings).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.User{}).Where("id = ?", userID).
			UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "user_role", userID, "replace", map[string]any{"role_ids": roleIDs}); err != nil {
			return err
		}
		if event != nil {
			event.TargetType, event.TargetID = "user", fmt.Sprintf("%d", userID)
		}
		return appendAuditEvent(tx, event, map[string]any{"role_ids": beforeRoleIDs}, map[string]any{"role_ids": roleIDs})
	})
}

func prepareRoleAuditEvent(event *model.OperationLog, role *model.Role) {
	if event == nil || role == nil {
		return
	}
	if event.TargetType == "" {
		event.TargetType = "role"
	}
	if event.TargetID == "" || event.TargetID == "0" {
		event.TargetID = fmt.Sprintf("%d", role.ID)
	}
	if event.TargetName == "" {
		event.TargetName = role.Name
	}
}

func roleAuditSnapshot(role *model.Role) map[string]any {
	if role == nil {
		return nil
	}
	return map[string]any{
		"id": role.ID, "workspace_id": role.WorkspaceID, "code": role.Code,
		"name": role.Name, "description": role.Description, "sort_order": role.SortOrder,
		"status": role.Status, "created_by": role.CreatedBy,
	}
}

func rolePermissionCodes(tx *gorm.DB, roleID uint) ([]string, error) {
	var codes []string
	err := tx.Model(&model.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Where("role_permissions.role_id = ?", roleID).
		Order("permissions.code ASC").Pluck("permissions.code", &codes).Error
	return codes, err
}

func incrementRoleUsersAuthVersion(tx *gorm.DB, workspaceID, roleID uint) error {
	var userIDs []uint
	if err := tx.Model(&model.UserRole{}).
		Where("workspace_id = ? AND role_id = ?", workspaceID, roleID).
		Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}
	return tx.Model(&model.User{}).Where("id IN ?", userIDs).
		UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error
}
