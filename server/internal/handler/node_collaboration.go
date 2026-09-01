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
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/notification"

	"github.com/gin-gonic/gin"
)

const (
	maxCommentLength   = 2000
	maxCommentMentions = 20
)

var commentMentionPattern = regexp.MustCompile(`(^|[^\pL\pN._-])@([\pL\pN._-]{1,64})`)

type NodeCollaborationHandler struct {
	comments   *dao.CommentDAO
	nodes      *dao.NodeDAO
	users      *dao.UserDAO
	workspaces *dao.WorkspaceDAO
	authz      *authorization.Service
}

type commentResponse struct {
	ID                uint                     `json:"id"`
	AuthorID          uint                     `json:"author_id"`
	AuthorUsername    string                   `json:"author_username"`
	AuthorDisplayName string                   `json:"author_display_name"`
	Content           string                   `json:"content"`
	Revision          uint                     `json:"revision"`
	Mentions          []dao.CommentMentionView `json:"mentions"`
	CanEdit           bool                     `json:"can_edit"`
	CanDelete         bool                     `json:"can_delete"`
	CreatedAt         time.Time                `json:"created_at"`
	UpdatedAt         time.Time                `json:"updated_at"`
}

type activityResponse struct {
	dao.NodeActivityRecord
	Summary string `json:"summary"`
}

func NewNodeCollaborationHandler() *NodeCollaborationHandler {
	return &NodeCollaborationHandler{
		comments: dao.NewCommentDAO(), nodes: dao.NewNodeDAO(), users: dao.NewUserDAO(),
		workspaces: dao.NewWorkspaceDAO(), authz: authorization.NewService(),
	}
}

// @Summary List Activity
// @Description Handles GET /api/fileshare/v1/management/nodes/{id}/activity.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/activity [get]
func (h *NodeCollaborationHandler) ListActivity(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 50})
	result, err := h.comments.ListActivity(actor.WorkspaceID, node.ID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取文件活动失败", err)
		return
	}
	items := make([]activityResponse, 0, len(result.List))
	for _, item := range result.List {
		items = append(items, activityResponse{NodeActivityRecord: item, Summary: activitySummary(item)})
	}
	rememberRecentNode(dao.NewCollaborationDAO(), actor, node.ID)
	response.SuccessWithPage(c, items, result.Total, result.Page, result.PageSize)
}

// @Summary List Comments
// @Description Handles GET /api/fileshare/v1/management/nodes/{id}/comments.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/comments [get]
func (h *NodeCollaborationHandler) ListComments(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 50})
	result, err := h.comments.ListPage(actor.WorkspaceID, node.ID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取评论失败", err)
		return
	}
	ids := make([]uint, 0, len(result.List))
	for _, item := range result.List {
		ids = append(ids, item.ID)
	}
	mentions, err := h.comments.ListMentions(ids)
	if err != nil {
		response.InternalError(c, "读取评论提及失败", err)
		return
	}
	items := make([]commentResponse, 0, len(result.List))
	for _, item := range result.List {
		items = append(items, buildCommentResponse(item, mentions[item.ID], actor))
	}
	rememberRecentNode(dao.NewCollaborationDAO(), actor, node.ID)
	response.SuccessWithPage(c, items, result.Total, result.Page, result.PageSize)
}

// @Summary Mention Candidates
// @Description Handles GET /api/fileshare/v1/management/nodes/{id}/mention-candidates.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param keyword query string false "keyword"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/mention-candidates [get]
func (h *NodeCollaborationHandler) MentionCandidates(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	keyword := strings.TrimSpace(c.Query("keyword"))
	if utf8.RuneCountInString(keyword) > 64 {
		response.BadRequest(c, "搜索关键词不能超过 64 个字符")
		return
	}
	candidates, err := h.workspaces.ListMentionCandidates(actor.WorkspaceID, keyword, 50)
	if err != nil {
		response.InternalError(c, "读取可提及成员失败", err)
		return
	}
	visible := make([]dao.WorkspaceMentionCandidate, 0, 20)
	for _, candidate := range candidates {
		candidateActor := authorization.Actor{
			UserID: candidate.UserID, WorkspaceID: actor.WorkspaceID,
			WorkspaceRole: candidate.Role, IsSuperAdmin: candidate.IsSuperAdmin,
		}
		allowed, authErr := h.authz.CanRead(candidateActor, node.ID)
		if authErr != nil && !errors.Is(authErr, authorization.ErrNodeNotFound) {
			response.InternalError(c, "校验成员文件权限失败", authErr)
			return
		}
		if allowed {
			visible = append(visible, candidate)
			if len(visible) == 20 {
				break
			}
		}
	}
	response.Success(c, visible)
}

// @Summary Create Comment
// @Description Handles POST /api/fileshare/v1/management/nodes/{id}/comments.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/comments [post]
func (h *NodeCollaborationHandler) CreateComment(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	content, err := normalizeCommentContent(req.Content)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	mentionIDs, err := h.resolveMentions(actor.WorkspaceID, node.ID, content)
	if err != nil {
		h.writeMentionError(c, err)
		return
	}
	comment := &model.NodeComment{
		WorkspaceID: actor.WorkspaceID, NodeID: node.ID, AuthorID: actor.UserID,
		Content: content, Revision: 1,
	}
	workspaceID := actor.WorkspaceID
	event := newBusinessAuditEvent(c, actor.UserID, &workspaceID, "comment:create", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name)
	if err := h.comments.Create(comment, mentionIDs, event); err != nil {
		response.InternalError(c, "发表评论失败", err)
		return
	}
	h.publishMentions(c, actor, node, comment, mentionIDs)
	item, err := h.commentResponse(actor, comment)
	if err != nil {
		response.InternalError(c, "读取新评论失败", err)
		return
	}
	response.Success(c, item)
}

// @Summary Update Comment
// @Description Handles PUT /api/fileshare/v1/management/nodes/{id}/comments/{comment_id}.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param comment_id path string true "comment_id"
// @Param id path string true "id"
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/comments/{comment_id} [put]
func (h *NodeCollaborationHandler) UpdateComment(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	commentID, ok := commentIDFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Content  string `json:"content" binding:"required"`
		Revision *uint  `json:"revision" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if req.Revision == nil || *req.Revision == 0 {
		response.BadRequest(c, "评论版本号无效")
		return
	}
	content, err := normalizeCommentContent(req.Content)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	mentionIDs, err := h.resolveMentions(actor.WorkspaceID, node.ID, content)
	if err != nil {
		h.writeMentionError(c, err)
		return
	}
	previousMentions, err := h.comments.ListMentionUserIDs(commentID)
	if err != nil {
		response.InternalError(c, "读取原评论提及失败", err)
		return
	}
	workspaceID := actor.WorkspaceID
	updated, err := h.comments.Update(actor.WorkspaceID, node.ID, commentID, actor.UserID, *req.Revision, content, mentionIDs,
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "comment:update", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name))
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrCommentConflict):
			response.Conflict(c, "评论已被更新，请刷新后重试")
		case errors.Is(err, dao.ErrCommentForbidden):
			response.Forbidden(c, "只能编辑自己的评论")
		default:
			response.InternalError(c, "更新评论失败", err)
		}
		return
	}
	if updated == nil {
		response.NotFound(c, "评论不存在")
		return
	}
	h.publishMentions(c, actor, node, updated, differenceIDs(mentionIDs, previousMentions))
	item, err := h.commentResponse(actor, updated)
	if err != nil {
		response.InternalError(c, "读取更新后的评论失败", err)
		return
	}
	response.Success(c, item)
}

// @Summary Delete Comment
// @Description Handles DELETE /api/fileshare/v1/management/nodes/{id}/comments/{comment_id}.
// @Tags Collaboration
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param comment_id path string true "comment_id"
// @Param id path string true "id"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/nodes/{id}/comments/{comment_id} [delete]
func (h *NodeCollaborationHandler) DeleteComment(c *gin.Context) {
	actor, node, ok := h.authorizeNode(c)
	if !ok {
		return
	}
	commentID, ok := commentIDFromContext(c)
	if !ok {
		return
	}
	workspaceID := actor.WorkspaceID
	deleted, err := h.comments.Delete(actor.WorkspaceID, node.ID, commentID, actor.UserID, actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin",
		newBusinessAuditEvent(c, actor.UserID, &workspaceID, "comment:delete", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name))
	if err != nil {
		if errors.Is(err, dao.ErrCommentForbidden) {
			response.Forbidden(c, "无权删除该评论")
			return
		}
		response.InternalError(c, "删除评论失败", err)
		return
	}
	if deleted == nil {
		response.NotFound(c, "评论不存在")
		return
	}
	response.Success(c, gin.H{"id": deleted.ID})
}

func (h *NodeCollaborationHandler) authorizeNode(c *gin.Context) (authorization.Actor, *model.Node, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, false
	}
	nodeID, err := strconv.ParseUint(c.Param("id"), 10, 0)
	if err != nil || nodeID == 0 {
		response.BadRequest(c, "文件 ID 格式错误")
		return authorization.Actor{}, nil, false
	}
	node, err := h.nodes.GetByID(actor.WorkspaceID, uint(nodeID))
	if err != nil {
		response.InternalError(c, "读取文件失败", err)
		return authorization.Actor{}, nil, false
	}
	if node == nil || node.Status != "active" {
		response.NotFound(c, "文件或目录不存在")
		return authorization.Actor{}, nil, false
	}
	allowed, err := h.authz.CanRead(actor, node.ID)
	if err != nil && !errors.Is(err, authorization.ErrNodeNotFound) {
		response.InternalError(c, "校验文件权限失败", err)
		return authorization.Actor{}, nil, false
	}
	recordDataAuthorization(c, allowed, "node:collaboration:read", node.Type, node.ID)
	if !allowed {
		response.Forbidden(c, "无权访问该文件或目录")
		return authorization.Actor{}, nil, false
	}
	return actor, node, true
}

var errMentionNotMember = errors.New("mentioned user is not an active workspace member")
var errMentionNoAccess = errors.New("mentioned user cannot read this node")

func (h *NodeCollaborationHandler) resolveMentions(workspaceID, nodeID uint, content string) ([]uint, error) {
	usernames := extractCommentMentions(content)
	if len(usernames) > maxCommentMentions {
		return nil, fmt.Errorf("一条评论最多提及 %d 位成员", maxCommentMentions)
	}
	candidates, err := h.workspaces.FindMentionCandidatesByUsernames(workspaceID, usernames)
	if err != nil {
		return nil, err
	}
	byUsername := make(map[string]dao.WorkspaceMentionCandidate, len(candidates))
	for _, candidate := range candidates {
		byUsername[strings.ToLower(candidate.Username)] = candidate
	}
	ids := make([]uint, 0, len(usernames))
	for _, username := range usernames {
		candidate, exists := byUsername[strings.ToLower(username)]
		if !exists {
			return nil, fmt.Errorf("%w: @%s", errMentionNotMember, username)
		}
		allowed, authErr := h.authz.CanRead(authorization.Actor{
			UserID: candidate.UserID, WorkspaceID: workspaceID,
			WorkspaceRole: candidate.Role, IsSuperAdmin: candidate.IsSuperAdmin,
		}, nodeID)
		if authErr != nil && !errors.Is(authErr, authorization.ErrNodeNotFound) {
			return nil, authErr
		}
		if !allowed {
			return nil, fmt.Errorf("%w: @%s", errMentionNoAccess, username)
		}
		ids = append(ids, candidate.UserID)
	}
	return ids, nil
}

func (h *NodeCollaborationHandler) writeMentionError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errMentionNotMember):
		response.BadRequest(c, strings.TrimPrefix(err.Error(), errMentionNotMember.Error()+": ")+" 不是当前空间的有效成员")
	case errors.Is(err, errMentionNoAccess):
		response.BadRequest(c, strings.TrimPrefix(err.Error(), errMentionNoAccess.Error()+": ")+" 当前无权访问此项目")
	default:
		if strings.Contains(err.Error(), "一条评论最多") {
			response.BadRequest(c, err.Error())
		} else {
			response.InternalError(c, "校验提及成员失败", err)
		}
	}
}

func (h *NodeCollaborationHandler) publishMentions(c *gin.Context, actor authorization.Actor, node *model.Node, comment *model.NodeComment, mentionIDs []uint) {
	if node == nil || comment == nil || len(mentionIDs) == 0 {
		return
	}
	authorName := stringValueFromContext(c, "username")
	if author, err := h.users.GetByID(actor.UserID); err == nil && author != nil && strings.TrimSpace(author.RealName) != "" {
		authorName = author.RealName
	}
	excerpt := commentExcerpt(comment.Content, 100)
	workspaceID := actor.WorkspaceID
	events := make([]notification.UserEvent, 0, len(mentionIDs))
	for _, userID := range mentionIDs {
		if userID == actor.UserID {
			continue
		}
		events = append(events, notification.UserEvent{
			Key: fmt.Sprintf("comment-mention:%d:%d", comment.ID, comment.Revision), UserID: userID,
			WorkspaceID: &workspaceID, Type: "comment:mention", Category: notification.UserCategoryCollaboration,
			Severity: "info", Title: "你在评论中被提及",
			Content:    fmt.Sprintf("%s 在“%s”的评论中提到了你：%s", authorName, node.Name, excerpt),
			TargetType: "node", TargetID: strconv.FormatUint(uint64(node.ID), 10),
		})
	}
	if _, err := notification.PublishUsers(c.Request.Context(), events); err != nil {
		logger.Warn("comment_mention_notification_failed", "comment_id", comment.ID, "error", err)
	}
}

func (h *NodeCollaborationHandler) commentResponse(actor authorization.Actor, comment *model.NodeComment) (commentResponse, error) {
	mentions, err := h.comments.ListMentions([]uint{comment.ID})
	if err != nil {
		return commentResponse{}, err
	}
	user, err := h.users.GetByID(comment.AuthorID)
	if err != nil {
		return commentResponse{}, err
	}
	view := dao.CommentView{NodeComment: *comment}
	if user != nil {
		view.AuthorUsername = user.Username
		view.AuthorRealName = user.RealName
	}
	return buildCommentResponse(view, mentions[comment.ID], actor), nil
}

func buildCommentResponse(item dao.CommentView, mentions []dao.CommentMentionView, actor authorization.Actor) commentResponse {
	if mentions == nil {
		mentions = []dao.CommentMentionView{}
	}
	displayName := strings.TrimSpace(item.AuthorRealName)
	if displayName == "" {
		displayName = item.AuthorUsername
	}
	return commentResponse{
		ID: item.ID, AuthorID: item.AuthorID, AuthorUsername: item.AuthorUsername, AuthorDisplayName: displayName,
		Content: item.Content, Revision: item.Revision, Mentions: mentions,
		CanEdit:   actor.UserID == item.AuthorID,
		CanDelete: actor.UserID == item.AuthorID || actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin",
		CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt,
	}
}

func normalizeCommentContent(value string) (string, error) {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	value = strings.TrimSpace(value)
	length := utf8.RuneCountInString(value)
	if length == 0 {
		return "", errors.New("评论内容不能为空")
	}
	if length > maxCommentLength {
		return "", fmt.Errorf("评论内容不能超过 %d 个字符", maxCommentLength)
	}
	for _, r := range value {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return "", errors.New("评论内容包含不支持的控制字符")
		}
	}
	return value, nil
}

func extractCommentMentions(content string) []string {
	matches := commentMentionPattern.FindAllStringSubmatch(content, -1)
	result := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		username := match[2]
		key := strings.ToLower(username)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, username)
	}
	return result
}

func commentIDFromContext(c *gin.Context) (uint, bool) {
	id, err := strconv.ParseUint(c.Param("comment_id"), 10, 0)
	if err != nil || id == 0 {
		response.BadRequest(c, "评论 ID 格式错误")
		return 0, false
	}
	return uint(id), true
}

func activitySummary(item dao.NodeActivityRecord) string {
	switch item.Action {
	case "node:create":
		return "创建了此项目"
	case "node:rename":
		return "重命名了此项目"
	case "node:move":
		return "移动了此项目"
	case "node:trash":
		return "将此项目移入回收站"
	case "node:restore":
		return "从回收站恢复了此项目"
	case "acl:replace":
		return "更新了访问权限"
	case "acl:inheritance_update":
		return "更新了权限继承方式"
	case "share:create":
		return "创建了外链分享"
	case "share:revoke":
		return "撤销了外链分享"
	case "comment:create":
		return "发表了评论"
	case "comment:update":
		return "编辑了评论"
	case "comment:delete":
		return "删除了评论"
	case "file:version_created":
		return fmt.Sprintf("上传了版本 %d", item.VersionNo)
	default:
		return "更新了此项目"
	}
}

func differenceIDs(current, previous []uint) []uint {
	old := make(map[uint]struct{}, len(previous))
	for _, id := range previous {
		old[id] = struct{}{}
	}
	result := make([]uint, 0, len(current))
	for _, id := range current {
		if _, exists := old[id]; !exists {
			result = append(result, id)
		}
	}
	return result
}

func commentExcerpt(content string, limit int) string {
	runes := []rune(strings.Join(strings.Fields(content), " "))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
