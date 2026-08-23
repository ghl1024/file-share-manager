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
	"encoding/json"
	"fmt"

	"file-share-manager/server/internal/model"

	"gorm.io/gorm"
)

// appendChange must receive the transaction that performs the business
// mutation. Returning its error makes a missing change record roll back the
// corresponding mutation as well.
func appendChange(tx *gorm.DB, workspaceID uint, entityType string, entityID any, operation string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return tx.Create(&model.ChangeLog{
		WorkspaceID: workspaceID,
		EntityType:  entityType,
		EntityID:    fmt.Sprint(entityID),
		Operation:   operation,
		Payload:     string(data),
	}).Error
}

// AppendChange exposes transaction-safe logging to services that own a
// multi-model transaction, such as backup restore.
func AppendChange(tx *gorm.DB, workspaceID uint, entityType string, entityID any, operation string, payload any) error {
	return appendChange(tx, workspaceID, entityType, entityID, operation, payload)
}

func appendUserChange(tx *gorm.DB, user model.User, operation string) error {
	var workspaceIDs []uint
	if err := tx.Model(&model.WorkspaceMembership{}).Where("user_id = ?", user.ID).Pluck("workspace_id", &workspaceIDs).Error; err != nil {
		return err
	}
	payload := map[string]any{
		"id": user.ID, "username": user.Username, "real_name": user.RealName, "email": user.Email,
		"phone": user.Phone, "status": user.Status, "source": user.Source, "is_super_admin": user.IsSuperAdmin,
	}
	for _, workspaceID := range workspaceIDs {
		if err := appendChange(tx, workspaceID, "user", user.ID, operation, payload); err != nil {
			return err
		}
	}
	return nil
}

func incrementUsersAuthVersion(tx *gorm.DB, userIDs []uint) error {
	if len(userIDs) == 0 {
		return nil
	}
	return tx.Model(&model.User{}).Where("id IN ?", userIDs).
		UpdateColumn("auth_version", gorm.Expr("auth_version + 1")).Error
}

func incrementWorkspaceUsersAuthVersion(tx *gorm.DB, workspaceID uint) error {
	var userIDs []uint
	if err := tx.Model(&model.WorkspaceMembership{}).Where("workspace_id = ?", workspaceID).Pluck("user_id", &userIDs).Error; err != nil {
		return err
	}
	return incrementUsersAuthVersion(tx, userIDs)
}
