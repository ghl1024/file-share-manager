/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package handler

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/service/notification"
)

type aclRecipientState struct {
	Effect string
	Level  string
	Source string
}

func (h *ACLHandler) publishACLChangeNotifications(ctx context.Context, workspaceID, nodeID uint, nodeName string, before, after []model.NodeACL) {
	beforeStates, err := h.aclRecipientStates(workspaceID, before)
	if err != nil {
		logger.Warn("acl_user_notification_expand_failed", "workspace_id", workspaceID, "node_id", nodeID, "error", err)
		return
	}
	afterStates, err := h.aclRecipientStates(workspaceID, after)
	if err != nil {
		logger.Warn("acl_user_notification_expand_failed", "workspace_id", workspaceID, "node_id", nodeID, "error", err)
		return
	}
	userIDs := make(map[uint]struct{}, len(beforeStates)+len(afterStates))
	for userID := range beforeStates {
		userIDs[userID] = struct{}{}
	}
	for userID := range afterStates {
		userIDs[userID] = struct{}{}
	}
	events := make([]notification.UserEvent, 0, len(userIDs))
	for userID := range userIDs {
		previous, hadBefore := beforeStates[userID]
		current, hasAfter := afterStates[userID]
		if hadBefore == hasAfter && previous == current {
			continue
		}
		workspace := workspaceID
		event := notification.UserEvent{
			Key:    fmt.Sprintf("acl:%d:%d:%d:%d", workspaceID, nodeID, userID, time.Now().UnixNano()),
			UserID: userID, WorkspaceID: &workspace, Category: notification.UserCategoryCollaboration,
			TargetType: "node", TargetID: strconv.FormatUint(uint64(nodeID), 10),
		}
		if hasAfter && current.Effect == "allow" {
			event.Type = "collaboration:permission_granted"
			event.Severity = "info"
			event.Title = "目录权限已更新"
			event.Content = fmt.Sprintf("你现在可以%s“%s”（来源：%s）。", accessLevelAction(current.Level), nodeName, current.Source)
		} else {
			event.Type = "collaboration:permission_revoked"
			event.Severity = "warning"
			event.Title = "目录权限已撤销"
			event.Content = fmt.Sprintf("你对“%s”的当前授权已被撤销或拒绝。", nodeName)
		}
		events = append(events, event)
	}
	if _, err := notification.PublishUsers(ctx, events); err != nil {
		logger.Warn("acl_user_notification_publish_failed", "workspace_id", workspaceID, "node_id", nodeID, "error", err)
	}
}

func (h *ACLHandler) aclRecipientStates(workspaceID uint, entries []model.NodeACL) (map[uint]aclRecipientState, error) {
	direct := make(map[uint]model.NodeACL)
	groupEntries := make(map[uint][]model.NodeACL)
	for _, entry := range entries {
		switch entry.SubjectType {
		case "user":
			direct[entry.SubjectID] = entry
		case "group":
			userIDs, err := h.groups.ListGroupMemberUserIDs(workspaceID, entry.SubjectID)
			if err != nil {
				return nil, err
			}
			for _, userID := range userIDs {
				groupEntries[userID] = append(groupEntries[userID], entry)
			}
		}
	}
	states := make(map[uint]aclRecipientState, len(direct)+len(groupEntries))
	for userID, entry := range direct {
		states[userID] = aclRecipientState{Effect: entry.Effect, Level: entry.AccessLevel, Source: "个人授权"}
	}
	for userID, values := range groupEntries {
		if _, exists := direct[userID]; exists {
			continue
		}
		state := aclRecipientState{Effect: "allow", Level: "read", Source: "用户组授权"}
		for _, entry := range values {
			if entry.Effect == "deny" {
				state.Effect = "deny"
				state.Level = entry.AccessLevel
				break
			}
			if accessLevelRank(entry.AccessLevel) > accessLevelRank(state.Level) {
				state.Level = entry.AccessLevel
			}
		}
		states[userID] = state
	}
	return states, nil
}

func (h *GroupHandler) publishGroupAccessNotification(ctx context.Context, workspaceID, groupID, userID uint, groupName string, granted bool) {
	entries, err := h.acls.ListDirectPermissionsForSubject(workspaceID, "group", groupID)
	if err != nil {
		logger.Warn("group_user_notification_acl_failed", "workspace_id", workspaceID, "group_id", groupID, "error", err)
		return
	}
	allowed := make([]model.NodeACL, 0, len(entries))
	for _, entry := range entries {
		if entry.Effect == "allow" {
			allowed = append(allowed, entry)
		}
	}
	if len(allowed) == 0 {
		return
	}
	workspace := workspaceID
	event := notification.UserEvent{
		Key:    fmt.Sprintf("group-membership:%d:%d:%d:%t:%d", workspaceID, groupID, userID, granted, time.Now().UnixNano()),
		UserID: userID, WorkspaceID: &workspace, Category: notification.UserCategoryCollaboration,
		Severity: "info", TargetType: "node", TargetID: strconv.FormatUint(uint64(allowed[0].NodeID), 10),
	}
	if granted {
		event.Type = "collaboration:group_access_granted"
		event.Title = "用户组权限已生效"
		event.Content = fmt.Sprintf("你已加入“%s”，获得 %d 个目录的组授权。", groupName, len(allowed))
	} else {
		event.Type = "collaboration:group_access_revoked"
		event.Severity = "warning"
		event.Title = "用户组权限已撤销"
		event.Content = fmt.Sprintf("你已离开“%s”，该组关联的 %d 个目录授权不再适用。", groupName, len(allowed))
	}
	if _, err := notification.PublishUser(ctx, event); err != nil {
		logger.Warn("group_user_notification_publish_failed", "workspace_id", workspaceID, "group_id", groupID, "user_id", userID, "error", err)
	}
}

func accessLevelRank(level string) int {
	switch level {
	case "read":
		return 1
	case "read_write":
		return 2
	case "admin":
		return 3
	default:
		return 0
	}
}

func accessLevelAction(level string) string {
	switch level {
	case "admin":
		return "管理"
	case "read_write":
		return "编辑"
	default:
		return "查看"
	}
}
