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
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/pkg/security"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	shareAccessCookieName = "fileshare_share_access"
	shareAccessLifetime   = time.Hour
)

type ShareHandler struct {
	shares *dao.ShareDAO
	nodes  *dao.NodeDAO
	files  *dao.FileDAO
	users  *dao.UserDAO
	audit  *dao.OperationLogDAO
	authz  *authorization.Service
	reader func() (*storage.VersionReader, error)
}

func NewShareHandler() *ShareHandler {
	return &ShareHandler{
		shares: dao.NewShareDAO(), nodes: dao.NewNodeDAO(), files: dao.NewFileDAO(), users: dao.NewUserDAO(), audit: dao.NewOperationLogDAO(),
		authz: authorization.NewService(), reader: configuredVersionReader,
	}
}

type managedShareResponse struct {
	model.Share
	ItemCount        int64                  `json:"item_count"`
	CreatorName      string                 `json:"creator_name"`
	CreatorUsername  string                 `json:"creator_username,omitempty"`
	EffectiveStatus  string                 `json:"effective_status"`
	RequiresPassword bool                   `json:"requires_password"`
	IsOwner          bool                   `json:"is_owner"`
	CanRevoke        bool                   `json:"can_revoke"`
	CanViewSource    bool                   `json:"can_view_source"`
	CanLocateSource  bool                   `json:"can_locate_source,omitempty"`
	SourceLocation   *shareLocationResponse `json:"source_location,omitempty"`
	Items            []model.ShareItem      `json:"items,omitempty"`
}

type shareBreadcrumbResponse struct {
	ID   uint   `json:"id"`
	Name string `json:"name"`
}

type shareLocationResponse struct {
	NodeID      uint                      `json:"node_id"`
	NodeName    string                    `json:"node_name"`
	NodeType    string                    `json:"node_type"`
	Breadcrumbs []shareBreadcrumbResponse `json:"breadcrumbs"`
}

type createShareRequest struct {
	NodeID       uint   `json:"node_id" binding:"required"`
	Name         string `json:"name" binding:"max=255"`
	VersionNo    int    `json:"version_no"`
	Password     string `json:"password" binding:"max=128"`
	ExpiresAt    string `json:"expires_at" binding:"required"`
	MaxDownloads *int   `json:"max_downloads"`
}

func (h *ShareHandler) Create(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req createShareRequest
	if !request.BindJSON(c, &req) {
		return
	}
	if req.NodeID == 0 {
		response.BadRequest(c, "node_id 必须是正整数")
		return
	}
	target, err := h.nodes.GetByID(actor.WorkspaceID, req.NodeID)
	if err != nil {
		response.InternalError(c, "读取分享目标失败", err)
		return
	}
	if target == nil || target.Status != "active" {
		response.NotFound(c, "分享目标不存在")
		return
	}
	allowed, err := h.authz.CanCreateShare(actor, target.ID)
	if err != nil {
		handleShareAuthorizationError(c, err)
		return
	}
	recordDataAuthorization(c, allowed, "node:create_share", target.Type, target.ID)
	if !allowed {
		response.Forbidden(c, "无权创建该分享")
		return
	}

	expiresAt, err := time.Parse(time.RFC3339, strings.TrimSpace(req.ExpiresAt))
	if err != nil || !expiresAt.After(time.Now().Add(time.Minute)) {
		response.BadRequest(c, "expires_at 必须是未来的 RFC3339 时间")
		return
	}
	if expiresAt.After(time.Now().Add(365 * 24 * time.Hour)) {
		response.BadRequest(c, "分享有效期不能超过一年")
		return
	}
	if req.MaxDownloads != nil && (*req.MaxDownloads < 1 || *req.MaxDownloads > 1000000) {
		response.BadRequest(c, "max_downloads 必须在 1 到 1000000 之间")
		return
	}
	if req.Password != "" && (len([]rune(req.Password)) < 8 || len([]rune(req.Password)) > 128 || strings.ContainsAny(req.Password, "\r\n")) {
		response.BadRequest(c, "分享密码长度必须在 8 到 128 个字符之间")
		return
	}

	items, err := h.snapshot(actor.WorkspaceID, target, req.VersionNo)
	if err != nil {
		switch {
		case errors.Is(err, errShareUnsafeFile):
			response.Conflict(c, "分享内容包含当前不可外链的文件")
		case errors.Is(err, errShareEmpty):
			response.Conflict(c, "分享目录中没有可分享的文件")
		default:
			response.InternalError(c, "生成分享快照失败", err)
		}
		return
	}
	rawToken, tokenHash, err := newShareToken()
	if err != nil {
		response.InternalError(c, "生成分享令牌失败", err)
		return
	}
	passwordHash := ""
	if req.Password != "" {
		passwordHash, err = security.HashPassword(req.Password)
		if err != nil {
			response.InternalError(c, "保存分享密码失败", err)
			return
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = target.Name
	}
	share := &model.Share{
		WorkspaceID: actor.WorkspaceID, SourceNodeID: target.ID, PublicID: uuid.NewString(), TokenHash: tokenHash,
		Name: name, RootType: target.Type, RootName: target.Name, PasswordHash: passwordHash,
		ExpiresAt: expiresAt, MaxDownloads: req.MaxDownloads, Status: "active", CreatedBy: actor.UserID,
	}
	if err := h.shares.Create(share, items); err != nil {
		response.InternalError(c, "创建分享失败", err)
		return
	}
	h.recordManagedShareAudit(c, actor, share, "share:create", http.StatusOK, fmt.Sprintf(`{"share_id":%d,"item_count":%d}`, share.ID, len(items)))
	response.Success(c, gin.H{
		"id": share.ID, "name": share.Name, "token": rawToken,
		"url": shareURL(rawToken), "expires_at": share.ExpiresAt,
		"requires_password": share.PasswordHash != "", "item_count": len(items),
	})
}

func (h *ShareHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	scope, filter, message := parseShareListQuery(c)
	if message != "" {
		response.BadRequest(c, message)
		return
	}
	canViewWorkspace := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
	if scope == "workspace" && !canViewWorkspace {
		response.Forbidden(c, "仅空间管理员可以查看全部分享")
		return
	}
	if scope == "mine" {
		ownerID := actor.UserID
		filter.OwnerID = &ownerID
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	result, err := h.shares.ListPage(actor.WorkspaceID, page, pageSize, filter)
	if err != nil {
		response.InternalError(c, "读取分享列表失败", err)
		return
	}
	items, err := h.managedShareList(actor, result.List)
	if err != nil {
		response.InternalError(c, "读取分享摘要失败", err)
		return
	}
	response.SuccessWithPage(c, items, result.Total, result.Page, result.PageSize)
}

func (h *ShareHandler) Detail(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	share, err := h.shares.GetByID(actor.WorkspaceID, id)
	if err != nil {
		response.InternalError(c, "读取分享失败", err)
		return
	}
	if share == nil {
		response.NotFound(c, "分享不存在")
		return
	}
	if !canViewManagedShare(actor, share) {
		recordDataAuthorization(c, false, "share:view", "share", share.ID)
		response.NotFound(c, "分享不存在")
		return
	}
	recordDataAuthorization(c, true, "share:view", "share", share.ID)
	items, err := h.managedShareList(actor, []model.Share{*share})
	if err != nil {
		response.InternalError(c, "读取分享详情失败", err)
		return
	}
	if len(items) != 1 {
		response.InternalError(c, "读取分享详情失败", errors.New("share detail summary is empty"))
		return
	}
	preview, err := h.shares.ListItemPreview(share.ID, 20)
	if err != nil {
		response.InternalError(c, "读取分享文件失败", err)
		return
	}
	items[0].Items = preview
	items[0].CanRevoke, err = h.canManageShare(actor, share)
	if err != nil {
		response.InternalError(c, "校验分享管理权限失败", err)
		return
	}
	location, err := h.sourceLocation(actor, share)
	if err != nil {
		response.InternalError(c, "校验源文件权限失败", err)
		return
	}
	items[0].SourceLocation = location
	items[0].CanLocateSource = location != nil
	response.Success(c, items[0])
}

func (h *ShareHandler) Revoke(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	share, err := h.shares.GetByID(actor.WorkspaceID, id)
	if err != nil {
		response.InternalError(c, "读取分享失败", err)
		return
	}
	if share == nil {
		response.NotFound(c, "分享不存在")
		return
	}
	allowed, err := h.canManageShare(actor, share)
	if err != nil {
		response.InternalError(c, "校验分享管理权限失败", err)
		return
	}
	if !allowed {
		recordDataAuthorization(c, false, "share:revoke", "share", share.ID)
		response.Forbidden(c, "无权撤销该分享")
		return
	}
	recordDataAuthorization(c, true, "share:revoke", "share", share.ID)
	if err := h.shares.Revoke(actor.WorkspaceID, id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			response.NotFound(c, "分享不存在或已撤销")
			return
		}
		response.InternalError(c, "撤销分享失败", err)
		return
	}
	h.recordManagedShareAudit(c, actor, share, "share:revoke", http.StatusOK, fmt.Sprintf(`{"share_id":%d}`, share.ID))
	response.Success(c, gin.H{"id": id, "status": "revoked"})
}

func (h *ShareHandler) managedShareList(actor authorization.Actor, shares []model.Share) ([]managedShareResponse, error) {
	shareIDs := make([]uint, 0, len(shares))
	creatorIDs := make([]uint, 0, len(shares))
	sourceNodeIDs := make([]uint, 0, len(shares))
	for _, share := range shares {
		shareIDs = append(shareIDs, share.ID)
		creatorIDs = append(creatorIDs, share.CreatedBy)
		sourceNodeIDs = append(sourceNodeIDs, share.SourceNodeID)
	}
	counts, err := h.shares.CountItemsByShareIDs(shareIDs)
	if err != nil {
		return nil, err
	}
	users, err := h.users.GetByIDs(creatorIDs)
	if err != nil {
		return nil, err
	}
	activeNodes, err := h.nodes.ListActiveByIDs(actor.WorkspaceID, sourceNodeIDs)
	if err != nil {
		return nil, err
	}
	activeNodeIDs := make([]uint, 0, len(activeNodes))
	for _, node := range activeNodes {
		activeNodeIDs = append(activeNodeIDs, node.ID)
	}
	readableNodes := make(map[uint]bool, len(activeNodeIDs))
	if len(activeNodeIDs) > 0 {
		readableNodes, err = h.authz.ReadableNodeIDs(actor, activeNodeIDs)
		if err != nil {
			return nil, err
		}
	}
	items := make([]managedShareResponse, 0, len(shares))
	for _, share := range shares {
		creatorName := "已删除用户"
		creatorUsername := ""
		if user, exists := users[share.CreatedBy]; exists {
			creatorName = user.RealName
			creatorUsername = user.Username
			if strings.TrimSpace(creatorName) == "" {
				creatorName = user.Username
			}
		}
		canRevoke, err := h.canManageShare(actor, &share)
		if err != nil {
			return nil, err
		}
		items = append(items, managedShareResponse{
			Share: share, ItemCount: counts[share.ID], CreatorName: creatorName,
			CreatorUsername: creatorUsername, EffectiveStatus: effectiveShareStatus(share),
			RequiresPassword: share.PasswordHash != "", IsOwner: share.CreatedBy == actor.UserID,
			CanRevoke: canRevoke, CanViewSource: readableNodes[share.SourceNodeID],
		})
	}
	return items, nil
}

func canViewManagedShare(actor authorization.Actor, share *model.Share) bool {
	return share != nil && (actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin" || share.CreatedBy == actor.UserID)
}

func parseShareListQuery(c *gin.Context) (string, dao.ShareListFilter, string) {
	scope := strings.ToLower(strings.TrimSpace(c.DefaultQuery("scope", "mine")))
	if scope != "mine" && scope != "workspace" {
		return "", dao.ShareListFilter{}, "scope 必须是 mine 或 workspace"
	}
	filter := dao.ShareListFilter{
		Name: strings.TrimSpace(c.Query("name")), Status: strings.ToLower(strings.TrimSpace(c.Query("status"))),
		Creator: strings.TrimSpace(c.Query("creator")), Now: time.Now(),
	}
	if len([]rune(filter.Name)) > 255 {
		return "", dao.ShareListFilter{}, "name 长度不能超过 255 个字符"
	}
	if len([]rune(filter.Creator)) > 255 {
		return "", dao.ShareListFilter{}, "creator 长度不能超过 255 个字符"
	}
	if filter.Status != "" {
		switch filter.Status {
		case "active", "revoked", "expired", "exhausted":
		default:
			return "", dao.ShareListFilter{}, "status 不是受支持的分享状态"
		}
	}
	var err error
	if filter.ExpiresFrom, err = parseOptionalShareTime(c.Query("expires_from")); err != nil {
		return "", dao.ShareListFilter{}, "expires_from 必须是 RFC3339 时间"
	}
	if filter.ExpiresTo, err = parseOptionalShareTime(c.Query("expires_to")); err != nil {
		return "", dao.ShareListFilter{}, "expires_to 必须是 RFC3339 时间"
	}
	if filter.ExpiresFrom != nil && filter.ExpiresTo != nil && !filter.ExpiresFrom.Before(*filter.ExpiresTo) {
		return "", dao.ShareListFilter{}, "expires_from 必须早于 expires_to"
	}
	return scope, filter, ""
}

func parseOptionalShareTime(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return nil, err
	}
	return &value, nil
}

func effectiveShareStatus(share model.Share) string {
	if share.Status == "revoked" {
		return "revoked"
	}
	if share.Status == "expired" || !share.ExpiresAt.After(time.Now()) {
		return "expired"
	}
	if share.MaxDownloads != nil && share.DownloadCount >= *share.MaxDownloads {
		return "exhausted"
	}
	return "active"
}

func (h *ShareHandler) canManageShare(actor authorization.Actor, share *model.Share) (bool, error) {
	if actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin" || share.CreatedBy == actor.UserID {
		return true, nil
	}
	node, err := h.nodes.GetByID(actor.WorkspaceID, share.SourceNodeID)
	if err != nil || node == nil || node.Status != "active" {
		return false, err
	}
	allowed, err := h.authz.CanManageACL(actor, node.ID)
	if errors.Is(err, authorization.ErrNodeNotFound) {
		return false, nil
	}
	return allowed, err
}

func (h *ShareHandler) sourceLocation(actor authorization.Actor, share *model.Share) (*shareLocationResponse, error) {
	node, err := h.nodes.GetByID(actor.WorkspaceID, share.SourceNodeID)
	if err != nil || node == nil || node.Status != "active" {
		return nil, err
	}
	allowed, err := h.authz.CanRead(actor, node.ID)
	if errors.Is(err, authorization.ErrNodeNotFound) {
		return nil, nil
	}
	if err != nil || !allowed {
		return nil, err
	}
	ancestors, err := h.nodes.ListAncestors(actor.WorkspaceID, node.ID)
	if err != nil {
		return nil, err
	}
	if !completeAncestorPath(node, ancestors) {
		return nil, nil
	}
	ancestorIDs := make([]uint, 0, len(ancestors))
	for _, ancestor := range ancestors {
		ancestorIDs = append(ancestorIDs, ancestor.ID)
	}
	readable := make(map[uint]bool, len(ancestorIDs))
	if len(ancestorIDs) > 0 {
		readable, err = h.authz.ReadableNodeIDs(actor, ancestorIDs)
		if err != nil {
			return nil, err
		}
	}
	breadcrumbs := make([]shareBreadcrumbResponse, 0, len(ancestors))
	for _, ancestor := range ancestors {
		if !readable[ancestor.ID] || ancestor.Type != "folder" {
			return nil, nil
		}
		breadcrumbs = append(breadcrumbs, shareBreadcrumbResponse{ID: ancestor.ID, Name: ancestor.Name})
	}
	return &shareLocationResponse{
		NodeID: node.ID, NodeName: node.Name, NodeType: node.Type, Breadcrumbs: breadcrumbs,
	}, nil
}

func completeAncestorPath(node *model.Node, ancestors []model.Node) bool {
	if node == nil {
		return false
	}
	if node.ParentID == nil {
		return len(ancestors) == 0
	}
	if len(ancestors) == 0 || ancestors[len(ancestors)-1].ID != *node.ParentID {
		return false
	}
	for index, ancestor := range ancestors {
		if index == 0 {
			if ancestor.ParentID != nil {
				return false
			}
			continue
		}
		if ancestor.ParentID == nil || *ancestor.ParentID != ancestors[index-1].ID {
			return false
		}
	}
	return true
}

// PublicInfo returns only snapshot metadata that is safe for an anonymous user.
func (h *ShareHandler) PublicInfo(c *gin.Context) {
	share, items, ok := h.loadPublicShare(c)
	if !ok {
		return
	}
	publicItems := make([]gin.H, 0, len(items))
	if share.PasswordHash == "" || validShareAccess(c, share.ID) {
		for _, item := range items {
			publicItems = append(publicItems, gin.H{
				"public_id": item.PublicID, "relative_path": item.RelativePath, "name": item.Name,
				"version_no": item.VersionNo, "size": item.Size, "detected_mime": item.DetectedMime,
			})
		}
	}
	h.recordAudit(c, share, "share:access", http.StatusOK, "{}")
	response.Success(c, gin.H{
		"public_id": share.PublicID, "name": share.Name, "root_type": share.RootType,
		"root_name": share.RootName, "expires_at": share.ExpiresAt,
		"max_downloads": share.MaxDownloads, "download_count": share.DownloadCount,
		"requires_password": share.PasswordHash != "", "items": publicItems,
	})
}

func (h *ShareHandler) Verify(c *gin.Context) {
	share, _, ok := h.loadPublicShare(c)
	if !ok {
		return
	}
	var req struct {
		Password string `json:"password" binding:"required,max=128"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if share.PasswordHash == "" || !security.CheckPasswordHash(req.Password, share.PasswordHash) {
		h.recordAudit(c, share, "share:password_failed", http.StatusForbidden, `{"reason":"invalid_credentials"}`)
		response.Forbidden(c, "分享密码错误")
		return
	}
	setShareAccessCookie(c, share.ID)
	response.Success(c, gin.H{"verified": true, "expires_at": share.ExpiresAt})
}

func (h *ShareHandler) Download(c *gin.Context) {
	token := strings.TrimSpace(c.Param("token"))
	if !validShareToken(token) {
		h.recordAnonymousAudit(c, "share:access_denied", http.StatusNotFound, model.AuditReasonInvalidToken)
		response.NotFound(c, "分享不存在或已失效")
		return
	}
	share, err := h.shares.GetByTokenHash(security.SHA256(token))
	if err != nil || share == nil {
		h.recordAnonymousAudit(c, "share:access_denied", http.StatusNotFound, model.AuditReasonShareNotFound)
		response.NotFound(c, "分享不存在或已失效")
		return
	}
	if !share.ExpiresAt.After(time.Now()) {
		h.recordAudit(c, share, "share:access_denied", http.StatusGone, `{"reason":"share_expired"}`)
		response.Gone(c, "分享不存在或已失效")
		return
	}
	if share.Status != "active" {
		h.recordAudit(c, share, "share:access_denied", http.StatusGone, `{"reason":"share_revoked"}`)
		response.Gone(c, "分享不存在或已失效")
		return
	}
	if share.PasswordHash != "" && !validShareAccess(c, share.ID) {
		h.recordAudit(c, share, "share:access_denied", http.StatusForbidden, `{"reason":"password_required"}`)
		response.Forbidden(c, "请先验证分享密码")
		return
	}
	itemID := strings.TrimSpace(c.Query("item"))
	if share.RootType == "folder" && itemID == "" {
		response.BadRequest(c, "文件夹分享必须指定快照文件")
		return
	}
	claimedShare, item, err := h.shares.ClaimDownload(security.SHA256(token), itemID)
	if err != nil {
		switch {
		case errors.Is(err, dao.ErrShareLimit):
			h.recordAudit(c, share, "share:download_denied", http.StatusGone, `{"reason":"download_limit_reached"}`)
			response.Gone(c, "分享下载次数已用尽")
		case errors.Is(err, dao.ErrShareExpired):
			h.recordAudit(c, share, "share:download_denied", http.StatusGone, `{"reason":"share_expired"}`)
			response.Gone(c, "分享不存在或已失效")
		case errors.Is(err, dao.ErrShareRevoked):
			h.recordAudit(c, share, "share:download_denied", http.StatusGone, `{"reason":"share_revoked"}`)
			response.Gone(c, "分享不存在或已失效")
		case errors.Is(err, dao.ErrShareNotFound):
			h.recordAudit(c, share, "share:download_denied", http.StatusGone, `{"reason":"share_not_found"}`)
			response.Gone(c, "分享不存在或已失效")
		case errors.Is(err, dao.ErrShareItem):
			response.NotFound(c, "分享文件不存在")
		case errors.Is(err, dao.ErrShareUnsafe):
			h.recordAudit(c, share, "share:download_denied", http.StatusForbidden, `{"reason":"unsafe_scan_status"}`)
			response.Forbidden(c, "文件当前不可通过外链下载")
		case errors.Is(err, dao.ErrShareRestoreRequired):
			h.recordAudit(c, share, "share:download_denied", http.StatusConflict, `{"reason":"archive_restore_required"}`)
			response.Conflict(c, "文件处于深冷归档状态，需要先在对象存储中解冻")
		default:
			response.InternalError(c, "准备分享下载失败", err)
		}
		return
	}
	if claimedShare.MaxDownloads != nil && claimedShare.DownloadCount >= *claimedShare.MaxDownloads {
		workspaceID := claimedShare.WorkspaceID
		if _, notifyErr := notification.PublishUser(c.Request.Context(), notification.UserEvent{
			Key:    "share:download_limit:" + strconv.FormatUint(uint64(claimedShare.ID), 10),
			UserID: claimedShare.CreatedBy, WorkspaceID: &workspaceID, Type: "share:download_limit_reached",
			Category: notification.UserCategoryShare, Severity: "warning", Title: "外链下载次数已用尽",
			Content:    "你的外链“" + claimedShare.Name + "”已达到下载次数上限。",
			TargetType: "share", TargetID: strconv.FormatUint(uint64(claimedShare.ID), 10),
		}); notifyErr != nil {
			logger.Warn("share_user_notification_publish_failed", "event_type", "share:download_limit_reached", "share_id", claimedShare.ID, "error", notifyErr)
		}
	}
	if storageRequiresRestore(item.StorageClass) {
		h.recordAudit(c, share, "share:download_denied", http.StatusConflict, `{"reason":"archive_restore_required"}`)
		response.Conflict(c, "文件处于深冷归档状态，需要先在对象存储中解冻")
		return
	}
	started := time.Now()
	h.recordShareDownloadAudit(c, share, item, "share:download_start", http.StatusOK, started, 0, "")
	reader, err := h.reader()
	if err != nil {
		h.recordShareDownloadAudit(c, share, item, "share:download_failed", http.StatusInternalServerError, started, 0, model.AuditReasonStorageUnavailable)
		response.InternalError(c, "存储未配置", err)
		return
	}
	file, err := reader.Open(item.StorageClass, item.StorageKey)
	if err != nil {
		if errors.Is(err, storage.ErrArchiveRestoreRequired) {
			h.recordShareDownloadAudit(c, share, item, "share:download_failed", http.StatusConflict, started, 0, model.AuditReasonArchiveRestoreRequired)
			response.Conflict(c, "文件处于深冷归档状态，需要先在对象存储中解冻")
			return
		}
		h.recordShareDownloadAudit(c, share, item, "share:download_failed", http.StatusNotFound, started, 0, model.AuditReasonObjectNotFound)
		response.NotFound(c, "文件对象不存在")
		return
	}
	defer file.Close()
	filename := strings.ReplaceAll(strings.ReplaceAll(item.Name, "\r", "_"), "\n", "_")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; %s", contentDisposition(filename)))
	if item.DetectedMime != "" {
		c.Header("Content-Type", item.DetectedMime)
	}
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, filename, item.CreatedAt, seeker)
	} else {
		c.Header("Content-Length", strconv.FormatInt(item.Size, 10))
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, file)
	}
	_ = h.files.TouchAccessByStorageKey(item.StorageKey, time.Now())
	h.recordShareDownloadAudit(c, share, item, "share:download_complete", c.Writer.Status(), started, c.Writer.Size(), "")
}

var (
	errShareUnsafeFile = errors.New("share contains unsafe file")
	errShareEmpty      = errors.New("share contains no files")
)

func (h *ShareHandler) snapshot(workspaceID uint, target *model.Node, versionNo int) ([]model.ShareItem, error) {
	nodes := []model.Node{*target}
	if target.Type == "folder" {
		descendants, err := h.nodes.ListAllDescendants(workspaceID, target.ID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, descendants...)
	}
	nodeMap := make(map[uint]model.Node, len(nodes))
	for _, node := range nodes {
		if node.Status == "active" {
			nodeMap[node.ID] = node
		}
	}
	items := make([]model.ShareItem, 0)
	for _, node := range nodes {
		if node.Type != "file" || node.Status != "active" {
			continue
		}
		version, err := h.files.GetLatestVersion(workspaceID, node.ID)
		if versionNo > 0 && node.ID == target.ID {
			version, err = h.files.GetVersion(workspaceID, node.ID, versionNo)
		}
		if err != nil {
			return nil, err
		}
		if version == nil {
			continue
		}
		if !externalShareVersionAllowed(version) {
			return nil, errShareUnsafeFile
		}
		items = append(items, model.ShareItem{
			PublicID: uuid.NewString(), RelativePath: relativeNodePath(target, node, nodeMap), Name: node.Name,
			VersionNo: version.VersionNo, StorageKey: version.StorageKey, StorageClass: normalizedVersionStorageClass(version.StorageClass), Size: version.Size, SHA256: version.SHA256,
			DetectedMime: version.DetectedMime, ScanStatus: version.ScanStatus,
		})
	}
	if len(items) == 0 {
		return nil, errShareEmpty
	}
	return items, nil
}

func externalShareVersionAllowed(version *model.FileVersion) bool {
	return version != nil && version.ScanStatus == "clean" && !version.Encrypted && !storageRequiresRestore(version.StorageClass)
}

func normalizedVersionStorageClass(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "standard"
	}
	return value
}

func relativeNodePath(root *model.Node, node model.Node, nodes map[uint]model.Node) string {
	parts := []string{node.Name}
	current := node
	for current.ParentID != nil && *current.ParentID != root.ID {
		parent, ok := nodes[*current.ParentID]
		if !ok {
			break
		}
		parts = append(parts, parent.Name)
		current = parent
	}
	for left, right := 0, len(parts)-1; left < right; left, right = left+1, right-1 {
		parts[left], parts[right] = parts[right], parts[left]
	}
	return strings.Join(parts, "/")
}

func handleShareAuthorizationError(c *gin.Context, err error) {
	if errors.Is(err, authorization.ErrNodeNotFound) {
		response.NotFound(c, "目录或文件不存在")
		return
	}
	response.InternalError(c, "目录权限校验失败", err)
}

func (h *ShareHandler) recordAudit(c *gin.Context, share *model.Share, action string, status int, details string) {
	if share == nil || h.audit == nil {
		return
	}
	requestID, _ := c.Get("request_id")
	requestIDValue, _ := requestID.(string)
	path := c.Request.URL.Path
	if strings.Contains(path, "/share/") {
		path = "/api/fileshare/v1/share/:token"
	}
	workspaceID := share.WorkspaceID
	entry := &model.OperationLog{
		UserID: 0, Username: "external_share", WorkspaceID: &workspaceID,
		TargetWorkspaceID: &workspaceID, TargetType: "share", TargetID: share.PublicID, TargetName: share.Name,
		Method: c.Request.Method, Path: path, Action: action, Status: status,
		IP: c.ClientIP(), Details: details, RequestID: requestIDValue,
		UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	if err := h.audit.Create(entry); err != nil {
		// The download itself should not expose audit database details. The
		// request middleware still logs the failure for operators.
		c.Error(err)
	}
}

func (h *ShareHandler) recordManagedShareAudit(c *gin.Context, actor authorization.Actor, share *model.Share, action string, status int, details string) {
	if share == nil || h.audit == nil {
		return
	}
	username, _ := c.Get("username")
	usernameValue, _ := username.(string)
	requestID, _ := c.Get("request_id")
	requestIDValue, _ := requestID.(string)
	workspaceID := actor.WorkspaceID
	entry := &model.OperationLog{
		UserID: actor.UserID, Username: usernameValue, WorkspaceID: &workspaceID,
		ActorWorkspaceID: &workspaceID, TargetWorkspaceID: &workspaceID,
		NodeID:     &share.SourceNodeID,
		TargetType: "share", TargetID: share.PublicID, TargetName: share.Name,
		Method: c.Request.Method, Path: c.Request.URL.Path, Action: action, Status: status,
		IP: c.ClientIP(), Details: details, RequestID: requestIDValue,
		UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	if err := h.audit.Create(entry); err != nil {
		c.Error(err)
	}
}

func (h *ShareHandler) recordAnonymousAudit(c *gin.Context, action string, status int, reason string) {
	if h.audit == nil {
		return
	}
	requestID, _ := c.Get("request_id")
	requestIDValue, _ := requestID.(string)
	details, _ := json.Marshal(map[string]string{"reason": reason})
	entry := &model.OperationLog{
		UserID: 0, Username: "external_share", TargetType: "share",
		Method: c.Request.Method, Path: "/api/fileshare/v1/share/:token", Action: action, Status: status,
		IP: c.ClientIP(), Details: string(details), RequestID: requestIDValue,
		UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	if err := h.audit.Create(entry); err != nil {
		c.Error(err)
	}
}

func (h *ShareHandler) recordShareDownloadAudit(c *gin.Context, share *model.Share, item *model.ShareItem, action string, status int, started time.Time, bytesSent int, reason string) {
	if share == nil || item == nil || h.audit == nil {
		return
	}
	detailsValue := map[string]any{
		"share_id": share.ID, "item_id": item.PublicID, "size": item.Size, "version_no": item.VersionNo,
		"range": c.GetHeader("Range"), "bytes_sent": bytesSent, "transfer_duration_ms": time.Since(started).Milliseconds(),
	}
	if reason != "" {
		detailsValue["reason"] = reason
	}
	details, err := json.Marshal(detailsValue)
	if err != nil {
		c.Error(err)
		return
	}
	requestID, _ := c.Get("request_id")
	requestIDValue, _ := requestID.(string)
	workspaceID := share.WorkspaceID
	entry := &model.OperationLog{
		UserID: 0, Username: "external_share", WorkspaceID: &workspaceID, TargetWorkspaceID: &workspaceID,
		TargetType: "file_snapshot", TargetID: item.PublicID, TargetName: item.Name,
		Method: c.Request.Method, Path: "/api/fileshare/v1/share/:token/download", Action: action, Status: status,
		IP: c.ClientIP(), Latency: time.Since(started).Milliseconds(), Details: string(details), RequestID: requestIDValue,
		UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	if err := h.audit.Create(entry); err != nil {
		c.Error(err)
	}
}

func (h *ShareHandler) loadPublicShare(c *gin.Context) (*model.Share, []model.ShareItem, bool) {
	token := strings.TrimSpace(c.Param("token"))
	if !validShareToken(token) {
		response.NotFound(c, "分享不存在或已失效")
		return nil, nil, false
	}
	share, err := h.shares.GetByTokenHash(security.SHA256(token))
	if err != nil || share == nil {
		response.NotFound(c, "分享不存在或已失效")
		return nil, nil, false
	}
	if share.Status != "active" || !share.ExpiresAt.After(time.Now()) {
		response.Gone(c, "分享不存在或已失效")
		return nil, nil, false
	}
	items, err := h.shares.ListItems(share.ID)
	if err != nil {
		response.InternalError(c, "读取分享快照失败", err)
		return nil, nil, false
	}
	for _, item := range items {
		if item.ScanStatus != "clean" {
			h.recordAudit(c, share, "share:access_denied", http.StatusForbidden, `{"reason":"unsafe_scan_status"}`)
			response.Forbidden(c, "分享包含当前不可公开访问的文件")
			return nil, nil, false
		}
	}
	return share, items, true
}

func newShareToken() (string, string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(buffer)
	return token, security.SHA256(token), nil
}

func validShareToken(token string) bool {
	if len(token) < 40 || len(token) > 64 {
		return false
	}
	_, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil
}

func shareURL(token string) string {
	base := ""
	if cfg := config.GetConfig(); cfg != nil {
		base = strings.TrimRight(cfg.Server.WebURL, "/")
	}
	return base + "/fileshare/share/" + url.PathEscape(token)
}

func setShareAccessCookie(c *gin.Context, shareID uint) {
	expires := time.Now().Add(shareAccessLifetime)
	payload := fmt.Sprintf("%d:%d", shareID, expires.Unix())
	signature := signSharePayload(payload)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload + "." + signature))
	c.SetSameSite(http.SameSiteStrictMode)
	c.SetCookie(shareAccessCookieName, value, int(shareAccessLifetime.Seconds()), "/api/fileshare/v1/share", "", secureShareCookie(), true)
}

func validShareAccess(c *gin.Context, shareID uint) bool {
	value, err := c.Cookie(shareAccessCookieName)
	if err != nil {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	parts := strings.Split(string(decoded), ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(signSharePayload(parts[0]))) {
		return false
	}
	fields := strings.Split(parts[0], ":")
	if len(fields) != 2 || fields[0] != strconv.FormatUint(uint64(shareID), 10) {
		return false
	}
	expires, err := strconv.ParseInt(fields[1], 10, 64)
	return err == nil && expires > time.Now().Unix()
}

func signSharePayload(payload string) string {
	secret := "fileshare-share-cookie"
	if cfg := config.GetConfig(); cfg != nil && cfg.JWT.Secret != "" {
		secret = cfg.JWT.Secret
	}
	hash := hmac.New(sha256.New, []byte(secret))
	_, _ = hash.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(hash.Sum(nil))
}

func secureShareCookie() bool {
	if cfg := config.GetConfig(); cfg != nil {
		return cfg.Server.Mode == "release"
	}
	return false
}
