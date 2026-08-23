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
	"sort"
	"strconv"
	"time"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/pagination"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrCommentConflict  = errors.New("comment revision conflict")
	ErrCommentForbidden = errors.New("comment operation is forbidden")
)

type CommentDAO struct {
	db *gorm.DB
}

type CommentView struct {
	model.NodeComment
	AuthorUsername string `gorm:"column:author_username" json:"author_username"`
	AuthorRealName string `gorm:"column:author_real_name" json:"author_real_name"`
}

type CommentMentionView struct {
	CommentID uint   `gorm:"column:comment_id" json:"-"`
	UserID    uint   `gorm:"column:user_id" json:"user_id"`
	Username  string `json:"username"`
	RealName  string `json:"real_name"`
}

type NodeActivityRecord struct {
	ID               string    `json:"id"`
	Action           string    `json:"action"`
	ActorID          uint      `json:"actor_id"`
	ActorUsername    string    `json:"actor_username"`
	ActorDisplayName string    `json:"actor_display_name"`
	VersionNo        int       `json:"version_no,omitempty"`
	OccurredAt       time.Time `json:"occurred_at"`
}

func NewCommentDAO() *CommentDAO { return &CommentDAO{db: database.DB} }

func (dao *CommentDAO) ListPage(workspaceID, nodeID uint, page, pageSize int) (*pagination.PageResult[CommentView], error) {
	query := dao.db.Table("node_comments").
		Select("node_comments.*, users.username AS author_username, users.real_name AS author_real_name").
		Joins("JOIN users ON users.id = node_comments.author_id").
		Where("node_comments.workspace_id = ? AND node_comments.node_id = ?", workspaceID, nodeID).
		Order("node_comments.created_at DESC, node_comments.id DESC")
	return pagination.Paging[CommentView](query, page, pageSize)
}

func (dao *CommentDAO) Get(workspaceID, nodeID, commentID uint) (*model.NodeComment, error) {
	var comment model.NodeComment
	err := dao.db.Where("workspace_id = ? AND node_id = ? AND id = ?", workspaceID, nodeID, commentID).First(&comment).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &comment, nil
}

func (dao *CommentDAO) ListMentions(commentIDs []uint) (map[uint][]CommentMentionView, error) {
	result := make(map[uint][]CommentMentionView, len(commentIDs))
	if len(commentIDs) == 0 {
		return result, nil
	}
	var mentions []CommentMentionView
	err := dao.db.Table("node_comment_mentions").
		Select("node_comment_mentions.comment_id, users.id AS user_id, users.username, users.real_name").
		Joins("JOIN users ON users.id = node_comment_mentions.user_id").
		Where("node_comment_mentions.comment_id IN ?", commentIDs).
		Order("node_comment_mentions.comment_id ASC, users.username ASC").Scan(&mentions).Error
	if err != nil {
		return nil, err
	}
	for _, mention := range mentions {
		result[mention.CommentID] = append(result[mention.CommentID], mention)
	}
	return result, nil
}

func (dao *CommentDAO) ListMentionUserIDs(commentID uint) ([]uint, error) {
	var ids []uint
	err := dao.db.Model(&model.NodeCommentMention{}).Where("comment_id = ?", commentID).Order("user_id ASC").Pluck("user_id", &ids).Error
	return ids, err
}

func (dao *CommentDAO) Create(comment *model.NodeComment, mentionIDs []uint, event *model.OperationLog) error {
	mentionIDs = normalizeIDs(mentionIDs)
	return dao.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(comment).Error; err != nil {
			return err
		}
		if err := replaceCommentMentions(tx, comment.ID, mentionIDs); err != nil {
			return err
		}
		if err := appendChange(tx, comment.WorkspaceID, "node_comment", comment.ID, "create", commentChangePayload(*comment, mentionIDs)); err != nil {
			return err
		}
		prepareCommentAuditEvent(event, *comment)
		return appendAuditEvent(tx, event, nil, commentAuditSnapshot(*comment, mentionIDs))
	})
}

func (dao *CommentDAO) Update(workspaceID, nodeID, commentID, authorID, expectedRevision uint, content string, mentionIDs []uint, event *model.OperationLog) (*model.NodeComment, error) {
	mentionIDs = normalizeIDs(mentionIDs)
	var updated model.NodeComment
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var comment model.NodeComment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND node_id = ? AND id = ?", workspaceID, nodeID, commentID).First(&comment).Error; err != nil {
			return err
		}
		if comment.AuthorID != authorID {
			return ErrCommentForbidden
		}
		if comment.Revision != expectedRevision {
			return ErrCommentConflict
		}
		beforeMentionIDs, err := listCommentMentionIDs(tx, comment.ID)
		if err != nil {
			return err
		}
		before := commentAuditSnapshot(comment, beforeMentionIDs)
		comment.Content = content
		comment.Revision++
		comment.UpdatedAt = time.Now()
		if err := tx.Model(&model.NodeComment{}).Where("id = ? AND revision = ?", comment.ID, expectedRevision).
			Updates(map[string]any{"content": comment.Content, "revision": comment.Revision, "updated_at": comment.UpdatedAt}).Error; err != nil {
			return err
		}
		if err := replaceCommentMentions(tx, comment.ID, mentionIDs); err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "node_comment", comment.ID, "update", commentChangePayload(comment, mentionIDs)); err != nil {
			return err
		}
		prepareCommentAuditEvent(event, comment)
		if err := appendAuditEvent(tx, event, before, commentAuditSnapshot(comment, mentionIDs)); err != nil {
			return err
		}
		updated = comment
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &updated, nil
}

func (dao *CommentDAO) Delete(workspaceID, nodeID, commentID, actorID uint, canAdmin bool, event *model.OperationLog) (*model.NodeComment, error) {
	var deleted model.NodeComment
	err := dao.db.Transaction(func(tx *gorm.DB) error {
		var comment model.NodeComment
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("workspace_id = ? AND node_id = ? AND id = ?", workspaceID, nodeID, commentID).First(&comment).Error; err != nil {
			return err
		}
		if comment.AuthorID != actorID && !canAdmin {
			return ErrCommentForbidden
		}
		mentionIDs, err := listCommentMentionIDs(tx, comment.ID)
		if err != nil {
			return err
		}
		if err := tx.Where("comment_id = ?", comment.ID).Delete(&model.NodeCommentMention{}).Error; err != nil {
			return err
		}
		if err := tx.Delete(&comment).Error; err != nil {
			return err
		}
		if err := appendChange(tx, workspaceID, "node_comment", comment.ID, "delete", map[string]any{"id": comment.ID, "node_id": nodeID}); err != nil {
			return err
		}
		prepareCommentAuditEvent(event, comment)
		if err := appendAuditEvent(tx, event, commentAuditSnapshot(comment, mentionIDs), nil); err != nil {
			return err
		}
		deleted = comment
		return nil
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &deleted, nil
}

func (dao *CommentDAO) ListActivity(workspaceID, nodeID uint, page, pageSize int) (*pagination.PageResult[NodeActivityRecord], error) {
	end := page * pageSize
	if end < pageSize {
		end = pageSize
	}
	actions := []string{
		"node:create", "node:rename", "node:move", "node:trash", "node:restore",
		"acl:replace", "acl:inheritance_update", "share:create", "share:revoke",
		"comment:create", "comment:update", "comment:delete",
	}
	nodeIDText := strconv.FormatUint(uint64(nodeID), 10)
	auditQuery := dao.db.Table("operation_logs").
		Joins("LEFT JOIN shares ON shares.workspace_id = operation_logs.workspace_id AND shares.public_id = operation_logs.target_id AND operation_logs.target_type = ?", "share").
		Where("operation_logs.workspace_id = ? AND operation_logs.action IN ? AND operation_logs.result = ?", workspaceID, actions, model.AuditResultSuccess).
		Where("(operation_logs.node_id = ? OR (operation_logs.target_id = ? AND operation_logs.target_type IN ?) OR shares.source_node_id = ?)", nodeID, nodeIDText, []string{"node", "file", "folder"}, nodeID)
	var auditTotal int64
	if err := auditQuery.Count(&auditTotal).Error; err != nil {
		return nil, err
	}
	var logs []model.OperationLog
	if err := auditQuery.Select("operation_logs.*").Order("operation_logs.created_at DESC, operation_logs.id DESC").Limit(end).Scan(&logs).Error; err != nil {
		return nil, err
	}
	versionQuery := dao.db.Model(&model.FileVersion{}).Where("workspace_id = ? AND node_id = ?", workspaceID, nodeID)
	var versionTotal int64
	if err := versionQuery.Count(&versionTotal).Error; err != nil {
		return nil, err
	}
	var versions []model.FileVersion
	if err := versionQuery.Order("created_at DESC, id DESC").Limit(end).Find(&versions).Error; err != nil {
		return nil, err
	}

	userIDs := make([]uint, 0, len(logs)+len(versions))
	for _, log := range logs {
		userIDs = append(userIDs, log.UserID)
	}
	for _, version := range versions {
		userIDs = append(userIDs, version.CreatedBy)
	}
	users := make(map[uint]model.User)
	if normalizedUserIDs := normalizeIDs(userIDs); len(normalizedUserIDs) > 0 {
		var rows []model.User
		if err := dao.db.Where("id IN ?", normalizedUserIDs).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, user := range rows {
			users[user.ID] = user
		}
	}
	items := make([]NodeActivityRecord, 0, len(logs)+len(versions))
	for _, log := range logs {
		items = append(items, NodeActivityRecord{
			ID: "audit:" + strconv.FormatUint(uint64(log.ID), 10), Action: log.Action,
			ActorID: log.UserID, ActorUsername: log.Username,
			ActorDisplayName: activityActorName(log.UserID, log.Username, users), OccurredAt: log.CreatedAt,
		})
	}
	for _, version := range versions {
		username := ""
		if user, exists := users[version.CreatedBy]; exists {
			username = user.Username
		}
		items = append(items, NodeActivityRecord{
			ID: "version:" + strconv.FormatUint(uint64(version.ID), 10), Action: "file:version_created",
			ActorID: version.CreatedBy, ActorUsername: username,
			ActorDisplayName: activityActorName(version.CreatedBy, username, users), VersionNo: version.VersionNo, OccurredAt: version.CreatedAt,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt.Equal(items[j].OccurredAt) {
			return items[i].ID > items[j].ID
		}
		return items[i].OccurredAt.After(items[j].OccurredAt)
	})
	start := (page - 1) * pageSize
	if start > len(items) {
		start = len(items)
	}
	if end > len(items) {
		end = len(items)
	}
	return &pagination.PageResult[NodeActivityRecord]{
		Total: auditTotal + versionTotal, List: items[start:end], Page: page, PageSize: pageSize,
	}, nil
}

func replaceCommentMentions(tx *gorm.DB, commentID uint, userIDs []uint) error {
	if err := tx.Where("comment_id = ?", commentID).Delete(&model.NodeCommentMention{}).Error; err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}
	now := time.Now()
	mentions := make([]model.NodeCommentMention, 0, len(userIDs))
	for _, userID := range userIDs {
		mentions = append(mentions, model.NodeCommentMention{CommentID: commentID, UserID: userID, CreatedAt: now})
	}
	return tx.Create(&mentions).Error
}

func listCommentMentionIDs(tx *gorm.DB, commentID uint) ([]uint, error) {
	var ids []uint
	err := tx.Model(&model.NodeCommentMention{}).Where("comment_id = ?", commentID).Order("user_id ASC").Pluck("user_id", &ids).Error
	return ids, err
}

func normalizeIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func prepareCommentAuditEvent(event *model.OperationLog, comment model.NodeComment) {
	if event == nil {
		return
	}
	event.NodeID = &comment.NodeID
	event.TargetType = "node"
	event.TargetID = strconv.FormatUint(uint64(comment.NodeID), 10)
}

func commentChangePayload(comment model.NodeComment, mentionIDs []uint) map[string]any {
	return map[string]any{
		"id": comment.ID, "workspace_id": comment.WorkspaceID, "node_id": comment.NodeID,
		"author_id": comment.AuthorID, "content": comment.Content, "revision": comment.Revision,
		"mention_user_ids": mentionIDs, "created_at": comment.CreatedAt, "updated_at": comment.UpdatedAt,
	}
}

func commentAuditSnapshot(comment model.NodeComment, mentionIDs []uint) map[string]any {
	return map[string]any{
		"id": comment.ID, "node_id": comment.NodeID, "author_id": comment.AuthorID,
		"revision": comment.Revision, "content_length": len([]rune(comment.Content)), "mention_user_ids": mentionIDs,
	}
}

func activityActorName(userID uint, fallback string, users map[uint]model.User) string {
	if user, exists := users[userID]; exists {
		if user.RealName != "" {
			return user.RealName
		}
		if user.Username != "" {
			return user.Username
		}
	}
	if fallback != "" {
		return fallback
	}
	return "系统"
}
