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
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	"gorm.io/gorm"
)

type PermissionDAO struct {
	db *gorm.DB
}

func NewPermissionDAO() *PermissionDAO {
	return &PermissionDAO{db: database.DB}
}

func (dao *PermissionDAO) UserHasPermission(workspaceID, userID uint, code string) (bool, error) {
	var count int64
	err := dao.db.Model(&model.Permission{}).
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN roles ON roles.id = role_permissions.role_id AND roles.workspace_id = ? AND roles.status = ?", workspaceID, 1).
		Joins("JOIN user_roles ON user_roles.role_id = roles.id AND user_roles.workspace_id = ?", workspaceID).
		Where("user_roles.user_id = ? AND permissions.code = ?", userID, code).
		Count(&count).Error
	return count > 0, err
}

func (dao *PermissionDAO) ListUserPermissionCodes(workspaceID, userID uint) ([]string, error) {
	var codes []string
	err := dao.db.Model(&model.Permission{}).
		Distinct("permissions.code").
		Joins("JOIN role_permissions ON role_permissions.permission_id = permissions.id").
		Joins("JOIN roles ON roles.id = role_permissions.role_id AND roles.workspace_id = ? AND roles.status = ?", workspaceID, 1).
		Joins("JOIN user_roles ON user_roles.role_id = roles.id AND user_roles.workspace_id = ?", workspaceID).
		Where("user_roles.user_id = ?", userID).
		Order("permissions.code ASC").
		Pluck("permissions.code", &codes).Error
	return codes, err
}
