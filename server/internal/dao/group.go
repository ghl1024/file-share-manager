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
	"slices"
	"strings"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrManagedUserGroup          = errors.New("managed user group cannot be changed manually")
	ErrGroupIsSoleDirectoryAdmin = errors.New("user group is the sole administrator of a directory")
)

type GroupDAO struct {
	db *gorm.DB
}

func NewGroupDAO() *GroupDAO {
	return &GroupDAO{db: database.DB}
}

func (dao *GroupDAO) CreateGroup(group *model.UserGroup) error {
	return dao.CreateGroupWithAudit(group, nil)
}

func (dao *GroupDAO) CreateGroupWithAudit(group *model.UserGroup, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(group).Error; err != nil {
			return err
		}
		if err := appendChange(tx, group.WorkspaceID, "user_group", group.ID, "create", group); err != nil {
			return err
		}
		prepareGroupAuditEvent(event, group)
		return appendAuditEvent(tx, event, nil, groupAuditSnapshot(group))
	})
}

func (dao *GroupDAO) UpdateLocalGroupWithAudit(workspaceID, groupID uint, name, description string, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var before model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND id = ?", workspaceID, groupID).First(&before).Error; err != nil {
			return err
		}
		if IsManagedGroupSource(before.Source) {
			return ErrManagedUserGroup
		}
		updates := map[string]any{"name": strings.TrimSpace(name), "description": strings.TrimSpace(description)}
		if err := tx.Model(&model.UserGroup{}).Where("workspace_id = ? AND id = ?", workspaceID, groupID).Updates(updates).Error; err != nil {
			return err
		}
		var after model.UserGroup
		if err := tx.Where("workspace_id = ? AND id = ?", workspaceID, groupID).First(&after).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "user_group", groupID, "update", updates); err != nil {
			return err
		}
		prepareGroupAuditEvent(event, &after)
		return appendAuditEvent(tx, event, groupAuditSnapshot(&before), groupAuditSnapshot(&after))
	})
}

func (dao *GroupDAO) GetGroupByID(workspaceID, id uint) (*model.UserGroup, error) {
	var g model.UserGroup
	err := dao.db.Where("workspace_id = ? AND id = ?", workspaceID, id).First(&g).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &g, nil
}

func (dao *GroupDAO) GetGroupByName(workspaceID uint, name string) (*model.UserGroup, error) {
	var group model.UserGroup
	err := dao.db.Where("workspace_id = ? AND name = ?", workspaceID, name).First(&group).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (dao *GroupDAO) GetLDAPGroupByDN(workspaceID uint, ldapDN string) (*model.UserGroup, error) {
	var group model.UserGroup
	err := dao.db.Where("workspace_id = ? AND source = ? AND ldap_dn = ?", workspaceID, "ldap", ldapDN).First(&group).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &group, nil
}

func (dao *GroupDAO) UpdateGroupFields(workspaceID, groupID uint, fields map[string]any) error {
	if len(fields) == 0 {
		return nil
	}
	return dao.db.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&model.UserGroup{}).Where("workspace_id = ? AND id = ?", workspaceID, groupID).Updates(fields)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		return appendChange(tx, workspaceID, "user_group", groupID, "update", fields)
	})
}

func (dao *GroupDAO) ListGroupsByWorkspace(workspaceID uint) ([]model.UserGroup, error) {
	var groups []model.UserGroup
	err := dao.db.Where("workspace_id = ?", workspaceID).Order("name ASC, id ASC").Find(&groups).Error
	return groups, err
}

func (dao *GroupDAO) ListPageByWorkspace(workspaceID uint, page, pageSize int, keyword string) (*pagination.PageResult[model.UserGroup], error) {
	query := dao.db.Model(&model.UserGroup{}).Where("workspace_id = ?", workspaceID)
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("(name LIKE ? OR description LIKE ?)", like, like)
	}
	query = query.Order("name ASC, id ASC")
	return pagination.Paging[model.UserGroup](query, page, pageSize)
}

func (dao *GroupDAO) ReplaceGroupMembers(workspaceID, groupID uint, userIDs []uint) (bool, error) {
	changed := false
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var group model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND id = ?", workspaceID, groupID).First(&group).Error; err != nil {
			return err
		}

		normalizedUserIDs := uniqueUintIDs(userIDs)
		var existingUserIDs []uint
		if err := tx.Model(&model.UserGroupMember{}).
			Where("group_id = ?", groupID).
			Pluck("user_id", &existingUserIDs).Error; err != nil {
			return err
		}
		changedUserIDs := changedMembershipUserIDs(existingUserIDs, normalizedUserIDs)
		changed = len(changedUserIDs) > 0
		if !changed {
			return nil
		}

		if err := tx.Where("group_id = ?", groupID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if len(normalizedUserIDs) > 0 {
			members := make([]model.UserGroupMember, 0, len(normalizedUserIDs))
			for _, userID := range normalizedUserIDs {
				members = append(members, model.UserGroupMember{GroupID: groupID, UserID: userID})
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&members).Error; err != nil {
				return err
			}
		}

		if err := incrementUsersAuthVersion(tx, changedUserIDs); err != nil {
			return err
		}
		return appendChange(tx, workspaceID, "user_group_member", groupID, "replace", map[string]any{"user_ids": normalizedUserIDs})
	})
	return changed, err
}

func (dao *GroupDAO) AddGroupMember(member *model.UserGroupMember) error {
	return dao.AddGroupMemberWithAudit(member, nil)
}

func (dao *GroupDAO) AddGroupMemberWithAudit(member *model.UserGroupMember, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var group model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&group, member.GroupID).Error; err != nil {
			return err
		}
		if IsManagedGroupSource(group.Source) {
			return ErrManagedUserGroup
		}
		var membershipCount int64
		if err := tx.Model(&model.WorkspaceMembership{}).
			Where("workspace_id = ? AND user_id = ?", group.WorkspaceID, member.UserID).Count(&membershipCount).Error; err != nil {
			return err
		}
		if membershipCount != 1 {
			return errors.New("user is not a workspace member")
		}
		var existing model.UserGroupMember
		err := tx.Where("group_id = ? AND user_id = ?", member.GroupID, member.UserID).First(&existing).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(member).Error; err != nil {
			return err
		}
		if err := incrementUsersAuthVersion(tx, []uint{member.UserID}); err != nil {
			return err
		}
		membership := groupMemberAuditSnapshot(member.GroupID, member.UserID)
		if err := appendChange(tx, group.WorkspaceID, "user_group_member", member.GroupID, "add", membership); err != nil {
			return err
		}
		prepareGroupAuditEvent(event, &group)
		return appendAuditEvent(tx, event, nil, membership)
	})
}

func uniqueUintIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	slices.Sort(ids)
	return slices.Compact(ids)
}

func changedMembershipUserIDs(before, after []uint) []uint {
	before = uniqueUintIDs(before)
	after = uniqueUintIDs(after)
	if len(before) == 0 && len(after) == 0 {
		return nil
	}
	changed := make([]uint, 0, len(before)+len(after))
	beforeIndex, afterIndex := 0, 0
	for beforeIndex < len(before) || afterIndex < len(after) {
		if beforeIndex >= len(before) {
			changed = append(changed, after[afterIndex:]...)
			break
		}
		if afterIndex >= len(after) {
			changed = append(changed, before[beforeIndex:]...)
			break
		}
		switch {
		case before[beforeIndex] == after[afterIndex]:
			beforeIndex++
			afterIndex++
		case before[beforeIndex] < after[afterIndex]:
			changed = append(changed, before[beforeIndex])
			beforeIndex++
		default:
			changed = append(changed, after[afterIndex])
			afterIndex++
		}
	}
	return changed
}

func (dao *GroupDAO) RemoveGroupMember(groupID, userID uint) error {
	var group model.UserGroup
	if err := dao.db.First(&group, groupID).Error; err != nil {
		return err
	}
	return dao.RemoveGroupMemberWithAudit(group.WorkspaceID, groupID, userID, nil)
}

func (dao *GroupDAO) RemoveGroupMemberWithAudit(workspaceID, groupID, userID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var group model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND id = ?", workspaceID, groupID).First(&group).Error; err != nil {
			return err
		}
		if IsManagedGroupSource(group.Source) {
			return ErrManagedUserGroup
		}
		result := tx.Where("group_id = ? AND user_id = ?", groupID, userID).Delete(&model.UserGroupMember{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		if err := incrementUsersAuthVersion(tx, []uint{userID}); err != nil {
			return err
		}
		membership := groupMemberAuditSnapshot(groupID, userID)
		if err := appendChange(tx, group.WorkspaceID, "user_group_member", groupID, "remove", membership); err != nil {
			return err
		}
		prepareGroupAuditEvent(event, &group)
		return appendAuditEvent(tx, event, membership, nil)
	})
}

func (dao *GroupDAO) ListMembersPage(workspaceID, groupID uint, page, pageSize int, keyword string) (*pagination.PageResult[model.GroupMemberView], error) {
	query := dao.db.Model(&model.UserGroupMember{}).
		Select("users.id AS user_id, users.username, users.real_name, users.email, users.status, user_group_members.joined_at").
		Joins("JOIN user_groups ON user_groups.id = user_group_members.group_id").
		Joins("JOIN users ON users.id = user_group_members.user_id").
		Where("user_groups.workspace_id = ? AND user_group_members.group_id = ?", workspaceID, groupID).
		Order("users.username ASC")
	if keyword != "" {
		like := "%" + keyword + "%"
		query = query.Where("users.username LIKE ? OR users.real_name LIKE ? OR users.email LIKE ?", like, like, like)
	}
	return pagination.Paging[model.GroupMemberView](query, page, pageSize)
}

func (dao *GroupDAO) ListGroupMemberUserIDs(workspaceID, groupID uint) ([]uint, error) {
	var userIDs []uint
	err := dao.db.Table("user_group_members").
		Select("user_group_members.user_id").
		Joins("JOIN user_groups ON user_groups.id = user_group_members.group_id").
		Where("user_groups.workspace_id = ? AND user_group_members.group_id = ?", workspaceID, groupID).
		Order("user_group_members.user_id ASC").Pluck("user_group_members.user_id", &userIDs).Error
	return userIDs, err
}

func (dao *GroupDAO) IsUserInGroup(workspaceID, groupID, userID uint) (bool, error) {
	var count int64
	err := dao.db.Table("user_group_members").
		Joins("JOIN user_groups ON user_groups.id = user_group_members.group_id").
		Where("user_groups.workspace_id = ? AND user_group_members.group_id = ? AND user_group_members.user_id = ?", workspaceID, groupID, userID).
		Count(&count).Error
	return count > 0, err
}

func (dao *GroupDAO) DeleteGroup(workspaceID, groupID uint) error {
	return dao.DeleteGroupWithAudit(workspaceID, groupID, nil)
}

func (dao *GroupDAO) DeleteGroupWithAudit(workspaceID, groupID uint, event *model.OperationLog) error {
	return dao.db.Transaction(func(tx *gorm.DB) error {
		var group model.UserGroup
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND id = ?", workspaceID, groupID).First(&group).Error; err != nil {
			return err
		}
		if IsManagedGroupSource(group.Source) {
			return ErrManagedUserGroup
		}
		var userIDs []uint
		if err := tx.Model(&model.UserGroupMember{}).Where("group_id = ?", groupID).Pluck("user_id", &userIDs).Error; err != nil {
			return err
		}
		var aclEntries []model.NodeACL
		if err := tx.Where("workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceID, "group", groupID).
			Order("node_id ASC, id ASC").Find(&aclEntries).Error; err != nil {
			return err
		}
		for _, acl := range aclEntries {
			if acl.Effect != "allow" || acl.AccessLevel != "admin" {
				continue
			}
			var otherAdminCount int64
			if err := tx.Model(&model.NodeACL{}).
				Where("workspace_id = ? AND node_id = ? AND effect = ? AND access_level = ?", workspaceID, acl.NodeID, "allow", "admin").
				Where("NOT (subject_type = ? AND subject_id = ?)", "group", groupID).
				Count(&otherAdminCount).Error; err != nil {
				return err
			}
			if otherAdminCount == 0 {
				return ErrGroupIsSoleDirectoryAdmin
			}
		}
		if err := tx.Where("group_id = ?", groupID).Delete(&model.UserGroupMember{}).Error; err != nil {
			return err
		}
		if err := tx.Where("workspace_id = ? AND subject_type = ? AND subject_id = ?", workspaceID, "group", groupID).
			Delete(&model.NodeACL{}).Error; err != nil {
			return err
		}
		result := tx.Where("workspace_id = ? AND id = ?", workspaceID, groupID).Delete(&model.UserGroup{})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return gorm.ErrRecordNotFound
		}
		if err := incrementUsersAuthVersion(tx, userIDs); err != nil {
			return err
		}
		for _, acl := range aclEntries {
			if err := appendChange(tx, workspaceID, "node_acl", acl.NodeID, "revoke", map[string]any{"subject_type": "group", "subject_id": groupID}); err != nil {
				return err
			}
		}
		if err := appendChange(tx, workspaceID, "user_group", groupID, "delete", map[string]any{"group_id": groupID, "removed_acl_count": len(aclEntries)}); err != nil {
			return err
		}
		before := groupAuditSnapshot(&group)
		before["member_ids"] = userIDs
		before["removed_acl_count"] = len(aclEntries)
		prepareGroupAuditEvent(event, &group)
		return appendAuditEvent(tx, event, before, nil)
	})
}

func prepareGroupAuditEvent(event *model.OperationLog, group *model.UserGroup) {
	if event == nil || group == nil {
		return
	}
	event.TargetType = "user_group"
	event.TargetID = fmt.Sprint(group.ID)
	event.TargetName = group.Name
}

func groupAuditSnapshot(group *model.UserGroup) map[string]any {
	if group == nil {
		return nil
	}
	return map[string]any{
		"id": group.ID, "workspace_id": group.WorkspaceID, "name": group.Name,
		"description": group.Description, "source": group.Source,
	}
}

func groupMemberAuditSnapshot(groupID, userID uint) map[string]any {
	return map[string]any{"group_id": groupID, "user_id": userID}
}

func IsManagedGroupSource(source string) bool {
	source = strings.ToLower(strings.TrimSpace(source))
	return source != "" && source != "local"
}

func (dao *GroupDAO) ListUserGroupIDs(workspaceID, userID uint) ([]uint, error) {
	var ids []uint
	err := dao.db.Table("user_groups").
		Select("user_groups.id").
		Joins("JOIN user_group_members ON user_group_members.group_id = user_groups.id").
		Where("user_groups.workspace_id = ? AND user_group_members.user_id = ?", workspaceID, userID).
		Pluck("user_groups.id", &ids).Error
	return ids, err
}
