/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldapsync

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	ldapservice "file-share-manager/server/internal/service/ldap"
)

const (
	DefaultCron = "0 0 2 * * *"

	syncTypeAuto   = "auto"
	syncTypeManual = "manual"
	syncTimeout    = 15 * time.Minute
)

var (
	ErrNotEnabled     = errors.New("ldap sync is disabled or not fully configured")
	ErrAlreadyRunning = errors.New("ldap sync is already running")
	runningSync       atomic.Bool
)

type Service struct {
	configDAO    *dao.LDAPConfigDAO
	historyDAO   *dao.LDAPSyncHistoryDAO
	userDAO      *dao.UserDAO
	groupDAO     *dao.GroupDAO
	workspaceDAO *dao.WorkspaceDAO
	ldap         *ldapservice.Service
}

func NewService() *Service {
	return &Service{
		configDAO:    dao.NewLDAPConfigDAO(),
		historyDAO:   dao.NewLDAPSyncHistoryDAO(),
		userDAO:      dao.NewUserDAO(),
		groupDAO:     dao.NewGroupDAO(),
		workspaceDAO: dao.NewWorkspaceDAO(),
		ldap:         ldapservice.NewService(),
	}
}

func (s *Service) StartAsync(syncType string) error {
	if !runningSync.CompareAndSwap(false, true) {
		return ErrAlreadyRunning
	}
	cfg, err := s.configDAO.Get()
	if err != nil {
		runningSync.Store(false)
		return err
	}
	if !syncEnabled(cfg) {
		runningSync.Store(false)
		return ErrNotEnabled
	}

	go func() {
		defer runningSync.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), syncTimeout)
		defer cancel()
		if _, runErr := s.runWithConfig(ctx, cfg, normalizeSyncType(syncType)); runErr != nil {
			logger.Warn("ldap_sync_async_finished_with_error", "sync_type", normalizeSyncType(syncType), "error", runErr)
		}
	}()
	return nil
}

func (s *Service) Run(ctx context.Context, syncType string) (*model.LDAPSyncHistory, error) {
	if !runningSync.CompareAndSwap(false, true) {
		return nil, ErrAlreadyRunning
	}
	defer runningSync.Store(false)

	cfg, err := s.configDAO.Get()
	if err != nil {
		return nil, err
	}
	if !syncEnabled(cfg) {
		return nil, ErrNotEnabled
	}
	return s.runWithConfig(ctx, cfg, normalizeSyncType(syncType))
}

func (s *Service) runWithConfig(ctx context.Context, cfg *model.LDAPConfig, syncType string) (*model.LDAPSyncHistory, error) {
	history := &model.LDAPSyncHistory{
		SyncType:  syncType,
		Status:    "running",
		StartTime: time.Now(),
	}
	if err := s.historyDAO.Create(history); err != nil {
		return nil, err
	}

	finish := func(status string, message string) {
		now := time.Now()
		history.Status = status
		history.EndTime = &now
		history.ErrorMessage = truncateMessage(message, 4000)
		if err := s.historyDAO.Update(history); err != nil {
			logger.Error("ldap_sync_history_update_failed", "history_id", history.ID, "error", err)
		}
	}

	runtimeConfig := dao.LDAPRuntimeConfig(cfg)
	users, err := s.ldap.ListUsers(ctx, runtimeConfig)
	if err != nil {
		message := fmt.Sprintf("检索 LDAP 用户失败：%v", err)
		finish("failed", message)
		return history, err
	}

	var createdCount, updatedCount, skippedCount, failedCount int
	details := make([]string, 0, 8)
	syncedUsersByDN := make(map[string]*model.User, len(users))
	syncedUsersByUsername := make(map[string]*model.User, len(users))
	for _, identity := range users {
		username := strings.TrimSpace(identity.Username)
		if username == "" {
			skippedCount++
			details = appendLimited(details, "跳过缺少用户名属性的 LDAP 条目")
			continue
		}

		existing, lookupErr := s.userDAO.GetByUsername(username)
		if lookupErr != nil {
			failedCount++
			details = appendLimited(details, fmt.Sprintf("查询用户 %s 失败", username))
			logger.Error("ldap_sync_user_lookup_failed", "username", username, "error", lookupErr)
			continue
		}

		realName := firstNonEmpty(identity.RealName, username)
		email := strings.TrimSpace(identity.Email)
		if existing != nil {
			if existing.Source != "ldap" {
				skippedCount++
				details = appendLimited(details, fmt.Sprintf("跳过同名本地用户 %s", username))
				continue
			}
			fields := map[string]any{}
			if realName != "" && existing.RealName != realName {
				fields["real_name"] = realName
			}
			if email != "" && existing.Email != email {
				fields["email"] = email
			}
			if len(fields) == 0 {
				registerSyncedLDAPUser(identity, existing, syncedUsersByDN, syncedUsersByUsername)
				continue
			}
			if err := s.userDAO.UpdateFields(existing.ID, fields); err != nil {
				failedCount++
				details = appendLimited(details, fmt.Sprintf("更新 LDAP 用户 %s 失败", username))
				logger.Error("ldap_sync_user_update_failed", "username", username, "error", err)
				continue
			}
			updatedCount++
			registerSyncedLDAPUser(identity, existing, syncedUsersByDN, syncedUsersByUsername)
			continue
		}

		hash, hashErr := ldapservice.NewLDAPPasswordHash()
		if hashErr != nil {
			failedCount++
			details = appendLimited(details, fmt.Sprintf("生成 LDAP 用户 %s 占位密码失败", username))
			logger.Error("ldap_sync_password_hash_failed", "username", username, "error", hashErr)
			continue
		}
		user := &model.User{
			Username:     username,
			PasswordHash: hash,
			RealName:     realName,
			Email:        email,
			Status:       1,
			Source:       "ldap",
		}
		if err := s.userDAO.Create(user); err != nil {
			failedCount++
			details = appendLimited(details, fmt.Sprintf("创建 LDAP 用户 %s 失败", username))
			logger.Error("ldap_sync_user_create_failed", "username", username, "error", err)
			continue
		}
		createdCount++
		registerSyncedLDAPUser(identity, user, syncedUsersByDN, syncedUsersByUsername)
	}

	history.TotalUsers = len(users)
	history.SuccessCount = createdCount
	history.UpdateCount = updatedCount
	history.SkipCount = skippedCount

	if cfg.GroupSyncEnabled == 1 {
		groups, groupListErr := s.ldap.ListGroups(ctx, runtimeConfig)
		if groupListErr != nil {
			failedCount++
			details = appendLimited(details, fmt.Sprintf("检索 LDAP 用户组失败：%v", groupListErr))
			logger.Error("ldap_sync_group_search_failed", "error", groupListErr)
		} else {
			groupOutcome := s.syncGroups(cfg, groups, syncedUsersByDN, syncedUsersByUsername)
			history.TotalGroups = groupOutcome.total
			history.GroupSuccessCount = groupOutcome.created
			history.GroupUpdateCount = groupOutcome.updated
			history.GroupSkipCount = groupOutcome.skipped
			failedCount += groupOutcome.failed
			for _, detail := range groupOutcome.details {
				details = appendLimited(details, detail)
			}
		}
	}

	message := strings.Join(details, "；")
	if failedCount > 0 {
		if message != "" {
			message = fmt.Sprintf("同步完成但 %d 项处理失败；%s", failedCount, message)
		} else {
			message = fmt.Sprintf("同步完成但 %d 项处理失败", failedCount)
		}
		finish("failed", message)
		return history, errors.New(message)
	}

	finish("success", message)
	logger.Info("ldap_sync_completed", "sync_type", syncType, "total", len(users), "created", createdCount, "updated", updatedCount, "skipped", skippedCount, "groups", history.TotalGroups, "group_created", history.GroupSuccessCount, "group_updated", history.GroupUpdateCount, "group_skipped", history.GroupSkipCount)
	return history, nil
}

type groupSyncOutcome struct {
	total   int
	created int
	updated int
	skipped int
	failed  int
	details []string
}

func (s *Service) syncGroups(cfg *model.LDAPConfig, groups []ldapservice.Group, usersByDN, usersByUsername map[string]*model.User) groupSyncOutcome {
	outcome := groupSyncOutcome{total: len(groups), details: make([]string, 0, 4)}
	if cfg.SyncWorkspaceID == 0 {
		outcome.failed++
		outcome.details = append(outcome.details, "启用了 LDAP 用户组同步，但未配置目标工作空间")
		return outcome
	}
	workspace, err := s.workspaceDAO.GetByID(cfg.SyncWorkspaceID)
	if err != nil {
		outcome.failed++
		outcome.details = append(outcome.details, "查询 LDAP 用户组目标工作空间失败")
		logger.Error("ldap_sync_group_workspace_lookup_failed", "workspace_id", cfg.SyncWorkspaceID, "error", err)
		return outcome
	}
	if workspace == nil || workspace.Status != 1 {
		outcome.failed++
		outcome.details = append(outcome.details, "LDAP 用户组目标工作空间不存在或已禁用")
		return outcome
	}

	for _, ldapGroup := range groups {
		name := strings.TrimSpace(ldapGroup.Name)
		dn := strings.TrimSpace(ldapGroup.DN)
		if name == "" || dn == "" {
			outcome.skipped++
			outcome.details = appendLimited(outcome.details, "跳过缺少组名或 DN 的 LDAP 用户组")
			continue
		}

		group, err := s.groupDAO.GetLDAPGroupByDN(cfg.SyncWorkspaceID, dn)
		if err != nil {
			outcome.failed++
			outcome.details = appendLimited(outcome.details, fmt.Sprintf("查询 LDAP 用户组 %s 失败", name))
			logger.Error("ldap_sync_group_lookup_by_dn_failed", "group", name, "dn", dn, "error", err)
			continue
		}

		createdGroup := false
		updatedGroup := false
		description := ldapGroupDescription(dn)
		if group == nil {
			existingByName, err := s.groupDAO.GetGroupByName(cfg.SyncWorkspaceID, name)
			if err != nil {
				outcome.failed++
				outcome.details = appendLimited(outcome.details, fmt.Sprintf("查询同名用户组 %s 失败", name))
				logger.Error("ldap_sync_group_lookup_by_name_failed", "group", name, "error", err)
				continue
			}
			if existingByName != nil && existingByName.Source != "ldap" {
				outcome.skipped++
				outcome.details = appendLimited(outcome.details, fmt.Sprintf("跳过同名本地用户组 %s", name))
				continue
			}
			if existingByName != nil {
				group = existingByName
				fields := map[string]any{"ldap_dn": dn, "description": description}
				if err := s.groupDAO.UpdateGroupFields(cfg.SyncWorkspaceID, group.ID, fields); err != nil {
					outcome.failed++
					outcome.details = appendLimited(outcome.details, fmt.Sprintf("绑定 LDAP 用户组 %s 失败", name))
					logger.Error("ldap_sync_group_bind_existing_failed", "group", name, "dn", dn, "error", err)
					continue
				}
				updatedGroup = true
				group.LDAPDN = dn
				group.Description = description
			} else {
				group = &model.UserGroup{
					WorkspaceID: cfg.SyncWorkspaceID,
					Name:        name,
					Description: description,
					Source:      "ldap",
					LDAPDN:      dn,
					CreatedBy:   0,
				}
				if err := s.groupDAO.CreateGroup(group); err != nil {
					outcome.failed++
					outcome.details = appendLimited(outcome.details, fmt.Sprintf("创建 LDAP 用户组 %s 失败", name))
					logger.Error("ldap_sync_group_create_failed", "group", name, "dn", dn, "error", err)
					continue
				}
				createdGroup = true
			}
		} else {
			fields := map[string]any{}
			if group.Name != name {
				existingByName, err := s.groupDAO.GetGroupByName(cfg.SyncWorkspaceID, name)
				if err != nil {
					outcome.failed++
					outcome.details = appendLimited(outcome.details, fmt.Sprintf("查询同名用户组 %s 失败", name))
					logger.Error("ldap_sync_group_rename_lookup_failed", "group", name, "dn", dn, "error", err)
					continue
				}
				if existingByName != nil && existingByName.ID != group.ID {
					outcome.skipped++
					outcome.details = appendLimited(outcome.details, fmt.Sprintf("跳过重命名后冲突的 LDAP 用户组 %s", name))
					continue
				}
				fields["name"] = name
			}
			if group.Description != description {
				fields["description"] = description
			}
			if len(fields) > 0 {
				if err := s.groupDAO.UpdateGroupFields(cfg.SyncWorkspaceID, group.ID, fields); err != nil {
					outcome.failed++
					outcome.details = appendLimited(outcome.details, fmt.Sprintf("更新 LDAP 用户组 %s 失败", name))
					logger.Error("ldap_sync_group_update_failed", "group", name, "dn", dn, "error", err)
					continue
				}
				updatedGroup = true
			}
		}

		memberIDs := resolveLDAPGroupMemberIDs(ldapGroup.MemberValues, usersByDN, usersByUsername)
		if err := s.ensureWorkspaceMembers(cfg.SyncWorkspaceID, memberIDs); err != nil {
			outcome.failed++
			outcome.details = appendLimited(outcome.details, fmt.Sprintf("同步 LDAP 用户组 %s 的工作空间成员失败", name))
			logger.Error("ldap_sync_group_workspace_members_failed", "group", name, "dn", dn, "error", err)
			continue
		}
		membersChanged, err := s.groupDAO.ReplaceGroupMembers(cfg.SyncWorkspaceID, group.ID, memberIDs)
		if err != nil {
			outcome.failed++
			outcome.details = appendLimited(outcome.details, fmt.Sprintf("同步 LDAP 用户组 %s 成员失败", name))
			logger.Error("ldap_sync_group_members_failed", "group", name, "dn", dn, "error", err)
			continue
		}

		if createdGroup {
			outcome.created++
		}
		if updatedGroup || membersChanged {
			outcome.updated++
		}
	}
	return outcome
}

func (s *Service) ensureWorkspaceMembers(workspaceID uint, userIDs []uint) error {
	for _, userID := range userIDs {
		if err := s.workspaceDAO.EnsureMember(workspaceID, userID, 0); err != nil {
			return err
		}
	}
	return nil
}

func syncEnabled(cfg *model.LDAPConfig) bool {
	if cfg == nil || cfg.Status != 1 {
		return false
	}
	runtimeConfig := dao.LDAPRuntimeConfig(cfg)
	return runtimeConfig.Enabled() && strings.TrimSpace(runtimeConfig.AdminDN) != "" && strings.TrimSpace(runtimeConfig.Password) != ""
}

func normalizeSyncType(value string) string {
	if strings.TrimSpace(value) == syncTypeManual {
		return syncTypeManual
	}
	return syncTypeAuto
}

func appendLimited(values []string, value string) []string {
	if len(values) >= 8 {
		if len(values) == 8 {
			values = append(values, "更多明细已省略")
		}
		return values
	}
	return append(values, value)
}

func truncateMessage(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func registerSyncedLDAPUser(identity ldapservice.Identity, user *model.User, byDN, byUsername map[string]*model.User) {
	if user == nil || user.Source != "ldap" {
		return
	}
	if dn := normalizeLDAPKey(identity.DN); dn != "" {
		byDN[dn] = user
	}
	if username := normalizeLDAPKey(user.Username); username != "" {
		byUsername[username] = user
	}
}

func resolveLDAPGroupMemberIDs(memberValues []string, usersByDN, usersByUsername map[string]*model.User) []uint {
	if len(memberValues) == 0 {
		return nil
	}
	seen := make(map[uint]bool)
	ids := make([]uint, 0, len(memberValues))
	for _, value := range memberValues {
		key := normalizeLDAPKey(value)
		if key == "" {
			continue
		}
		if user := usersByDN[key]; user != nil && !seen[user.ID] {
			ids = append(ids, user.ID)
			seen[user.ID] = true
			continue
		}
		if user := usersByUsername[key]; user != nil && !seen[user.ID] {
			ids = append(ids, user.ID)
			seen[user.ID] = true
		}
	}
	return ids
}

func normalizeLDAPKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func ldapGroupDescription(dn string) string {
	return fmt.Sprintf("LDAP 同步：%s", strings.TrimSpace(dn))
}
