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
	"archive/zip"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/clamav"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const (
	minChunkSize                     = 1 << 20
	maxChunkSize                     = 64 << 20
	maxUploadChunks                  = 1 << 20
	defaultMaxUploadFileBytes int64  = 100 << 30
	maxZIPEntries                    = 100000
	maxZIPExpandedSize        uint64 = 10 << 30
	maxZIPCompressionRatio           = 1000
	minZIPRatioCheckSize             = 1 << 20
	maxZIPNestingDepth               = 3
	maxNestedZIPSize          int64  = 64 << 20
)

type UploadHandler struct {
	uploads *dao.UploadDAO
	nodes   *dao.NodeDAO
	files   *dao.FileDAO
	authz   *authorization.Service
}

func NewUploadHandler() *UploadHandler {
	return &UploadHandler{uploads: dao.NewUploadDAO(), nodes: dao.NewNodeDAO(), files: dao.NewFileDAO(), authz: authorization.NewService()}
}

// @Summary Init
// @Description Handles POST /api/fileshare/v1/management/uploads/init.
// @Tags Uploads
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 413 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/uploads/init [post]
func (h *UploadHandler) Init(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		NodeID         *uint  `json:"node_id"`
		TargetParentID *uint  `json:"target_parent_id"`
		BaseVersionNo  *int   `json:"base_version_no"`
		DisplayName    string `json:"display_name" binding:"required,max=255"`
		TotalSize      int64  `json:"total_size" binding:"required,gt=0"`
		ChunkSize      int64  `json:"chunk_size" binding:"required,gt=0"`
		ClientSHA256   string `json:"sha256" binding:"omitempty,len=64"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	displayName, normalizedName, valid := normalizeNodeName(req.DisplayName)
	if !valid {
		response.BadRequest(c, "文件名不合法")
		return
	}
	if err := validateUploadExtension(displayName); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if req.ChunkSize < minChunkSize || req.ChunkSize > maxChunkSize {
		response.BadRequest(c, "分片大小必须在 1 MiB 到 64 MiB 之间")
		return
	}
	maxFileBytes := configuredMaxUploadFileBytes()
	if req.TotalSize > maxFileBytes {
		response.PayloadTooLarge(c, fmt.Sprintf("文件大小不能超过 %d 字节", maxFileBytes))
		return
	}
	totalChunks, err := calculateUploadChunkCount(req.TotalSize, req.ChunkSize, maxFileBytes)
	if err != nil {
		response.PayloadTooLarge(c, err.Error())
		return
	}
	if req.ClientSHA256 != "" {
		if _, err := hex.DecodeString(strings.ToLower(req.ClientSHA256)); err != nil {
			response.BadRequest(c, "sha256 必须是十六进制字符串")
			return
		}
		req.ClientSHA256 = strings.ToLower(req.ClientSHA256)
	}

	if req.TargetParentID != nil {
		parent, err := h.nodes.GetByID(actor.WorkspaceID, *req.TargetParentID)
		if err != nil {
			response.InternalError(c, "读取目标目录失败", err)
			return
		}
		if parent == nil || parent.Type != "folder" || parent.Status != "active" {
			response.NotFound(c, "目标目录不存在")
			return
		}
		allowed, err := h.authz.CanWrite(actor, parent.ID)
		if err != nil {
			response.InternalError(c, "目录权限校验失败", err)
			return
		}
		recordDataAuthorization(c, allowed, "node:write", "folder", parent.ID)
		if !allowed {
			response.Forbidden(c, "无权在该目录上传文件")
			return
		}
	} else {
		allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
		recordDataAuthorization(c, allowed, "node:upload_to_root", "workspace", actor.WorkspaceID)
		if !allowed {
			response.Forbidden(c, "只有工作空间管理员可以上传到根目录")
			return
		}
	}

	if req.NodeID != nil {
		node, err := h.nodes.GetByID(actor.WorkspaceID, *req.NodeID)
		if err != nil {
			response.InternalError(c, "读取目标文件失败", err)
			return
		}
		if node == nil || node.Type != "file" || node.Status != "active" {
			response.NotFound(c, "目标文件不存在")
			return
		}
		if req.TargetParentID != nil && (node.ParentID == nil || *node.ParentID != *req.TargetParentID) {
			response.BadRequest(c, "覆盖上传的目标目录与文件不匹配")
			return
		}
		if req.BaseVersionNo == nil {
			response.BadRequest(c, "覆盖上传必须提供 base_version_no")
			return
		}
		allowed, err := h.authz.CanWrite(actor, node.ID)
		if err != nil {
			response.InternalError(c, "文件权限校验失败", err)
			return
		}
		recordDataAuthorization(c, allowed, "node:write", "file", node.ID)
		if !allowed {
			response.Forbidden(c, "无权覆盖该文件")
			return
		}
		latest, err := h.files.GetLatestVersion(actor.WorkspaceID, node.ID)
		if err != nil {
			response.InternalError(c, "读取文件版本失败", err)
			return
		}
		if latest == nil || latest.VersionNo != *req.BaseVersionNo {
			response.Conflict(c, "文件版本已变化，请刷新后重试")
			return
		}
	} else {
		existing, err := h.nodes.FindActiveByName(actor.WorkspaceID, req.TargetParentID, normalizedName)
		if err != nil {
			response.InternalError(c, "检查同名文件失败", err)
			return
		}
		if existing != nil {
			response.Conflict(c, "同名文件已存在，请使用目标文件 ID 和 base_version_no 创建新版本")
			return
		}
	}

	uploadID := randomUploadID()
	store, err := h.posixStorage()
	if err != nil {
		response.InternalError(c, "存储未配置", err)
		return
	}
	if err := store.EnsureUpload(uploadID); err != nil {
		response.InternalError(c, "创建上传暂存目录失败", err)
		return
	}
	uploadLifetime := 24 * time.Hour
	if cfg := config.GetConfig(); cfg != nil && cfg.Lifecycle.UploadSessionHours > 0 {
		uploadLifetime = time.Duration(cfg.Lifecycle.UploadSessionHours) * time.Hour
	}
	expiresAt := time.Now().Add(uploadLifetime)
	userID := actor.UserID
	session := &model.UploadSession{
		ID: uploadID, WorkspaceID: actor.WorkspaceID, NodeID: req.NodeID,
		TargetParentID: req.TargetParentID, BaseVersionNo: req.BaseVersionNo,
		DisplayName: displayName, TotalSize: req.TotalSize, ChunkSize: req.ChunkSize,
		TotalChunks: totalChunks, ReceivedChunks: "[]", ClientSHA256: nullableString(req.ClientSHA256),
		Status: "initialized", ExpiresAt: expiresAt, CreatedBy: userID,
	}
	if err := h.uploads.CreateSession(session); err != nil {
		_ = store.RemoveUpload(uploadID)
		if errors.Is(err, dao.ErrQuotaExceeded) {
			response.Conflict(c, "存储配额不足")
			return
		}
		response.InternalError(c, "创建上传会话失败", err)
		return
	}
	response.Success(c, gin.H{"upload_id": uploadID, "total_chunks": totalChunks, "chunk_size": req.ChunkSize, "expires_at": expiresAt})
}

// @Summary Part
// @Description Handles PUT /api/fileshare/v1/management/uploads/{id}/parts/{part_no}.
// @Tags Uploads
// @Accept application/octet-stream
// @Param body body string true "Binary request body"
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param part_no path string true "part_no"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/uploads/{id}/parts/{part_no} [put]
func (h *UploadHandler) Part(c *gin.Context) {
	actor, session, partNo, ok := h.loadSession(c)
	if !ok {
		return
	}
	if err := validateUploadSessionDimensions(session, configuredMaxUploadFileBytes()); err != nil {
		response.Conflict(c, err.Error())
		return
	}
	expectedSize, err := expectedUploadPartSize(session, partNo)
	if err != nil {
		response.BadRequest(c, "分片编号超出范围")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		response.Conflict(c, "上传会话已过期")
		return
	}
	store, err := h.posixStorage()
	if err != nil {
		response.InternalError(c, "存储未配置", err)
		return
	}
	exists, size, err := store.PartExists(session.ID, partNo)
	if err != nil {
		response.InternalError(c, "读取分片状态失败", err)
		return
	}
	if !exists {
		if _, err := store.WritePart(session.ID, partNo, c.Request.Body, expectedSize); err != nil {
			response.BadRequest(c, "分片大小不正确: "+err.Error())
			return
		}
		size = expectedSize
	}
	if size != expectedSize {
		response.Conflict(c, "已存在的分片大小不匹配，请取消后重新上传")
		return
	}
	received, err := h.uploads.MarkPartReceived(actor.WorkspaceID, actor.UserID, session.ID, partNo)
	if err != nil {
		h.handleUploadError(c, err)
		return
	}
	response.Success(c, gin.H{"upload_id": session.ID, "part_no": partNo, "size": size, "received_parts": received})
}

// @Summary Status
// @Description Handles GET /api/fileshare/v1/management/uploads/{id} and GET /api/fileshare/v1/management/uploads/{id}/status.
// @Tags Uploads
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/uploads/{id} [get]
// @Router /management/uploads/{id}/status [get]
func (h *UploadHandler) Status(c *gin.Context) {
	_, session, _, ok := h.loadSession(c)
	if !ok {
		return
	}
	received, err := dao.DecodeReceivedChunks(session.ReceivedChunks)
	if err != nil {
		response.InternalError(c, "解析上传状态失败", err)
		return
	}
	response.Success(c, gin.H{
		"upload_id": session.ID, "status": session.Status,
		"display_name": session.DisplayName, "total_size": session.TotalSize,
		"chunk_size": session.ChunkSize, "total_chunks": session.TotalChunks,
		"received_parts": received, "expires_at": session.ExpiresAt,
		"target_parent_id": session.TargetParentID, "node_id": session.NodeID,
		"base_version_no": session.BaseVersionNo,
	})
}

// @Summary Complete
// @Description Handles POST /api/fileshare/v1/management/uploads/{id}/complete.
// @Tags Uploads
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
// @Router /management/uploads/{id}/complete [post]
func (h *UploadHandler) Complete(c *gin.Context) {
	actor, session, _, ok := h.loadSession(c)
	if !ok {
		return
	}
	if err := validateUploadSessionDimensions(session, configuredMaxUploadFileBytes()); err != nil {
		response.Conflict(c, err.Error())
		return
	}
	var req struct {
		SHA256 string `json:"sha256" binding:"omitempty,len=64"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	req.SHA256 = strings.ToLower(strings.TrimSpace(req.SHA256))
	if req.SHA256 != "" {
		if _, err := hex.DecodeString(req.SHA256); err != nil {
			response.BadRequest(c, "sha256 必须是十六进制字符串")
			return
		}
	}
	if session.ClientSHA256 != nil && strings.ToLower(*session.ClientSHA256) != req.SHA256 {
		response.BadRequest(c, "sha256 与初始化时不一致")
		return
	}
	if time.Now().After(session.ExpiresAt) {
		response.Conflict(c, "上传会话已过期")
		return
	}
	received, err := dao.DecodeReceivedChunks(session.ReceivedChunks)
	if err != nil || len(received) != session.TotalChunks {
		response.Conflict(c, "上传尚未收到全部分片")
		return
	}
	store, err := h.posixStorage()
	if err != nil {
		response.InternalError(c, "存储未配置", err)
		return
	}
	merged, err := store.Merge(session.ID, actor.WorkspaceID, session.TotalChunks, session.TotalSize, req.SHA256)
	if err != nil {
		response.BadRequest(c, "文件校验失败: "+err.Error())
		return
	}
	detectedMime, encrypted, inspectErr := inspectObject(store, merged.StorageKey, session.DisplayName, merged.Size)
	if inspectErr != nil {
		if removeErr := store.RemoveObject(merged.StorageKey); removeErr != nil {
			logger.Warn("rejected_upload_object_cleanup_failed", "upload_id", session.ID, "storage_key", merged.StorageKey, "error", removeErr)
		}
		if cancelErr := h.uploads.Cancel(actor.WorkspaceID, actor.UserID, session.ID); cancelErr != nil {
			logger.Warn("rejected_upload_session_cleanup_failed", "upload_id", session.ID, "error", cancelErr)
		}
		if removeErr := store.RemoveUpload(session.ID); removeErr != nil {
			logger.Warn("rejected_upload_staging_cleanup_failed", "upload_id", session.ID, "error", removeErr)
		}
		response.BadRequest(c, inspectErr.Error())
		return
	}
	version := &model.FileVersion{
		WorkspaceID: actor.WorkspaceID, StorageKey: merged.StorageKey, StorageClass: "standard",
		Size: merged.Size, SHA256: merged.SHA256, Extension: strings.ToLower(filepath.Ext(session.DisplayName)),
		DetectedMime: detectedMime, RiskLevel: riskLevel(encrypted), Encrypted: encrypted, ScanStatus: "unscanned", CreatedBy: actor.UserID,
	}
	if cfg := config.GetConfig(); cfg != nil && cfg.ClamAV.Enabled() {
		scanResult := clamav.ScanFile(c.Request.Context(), filepath.Join(cfg.Storage.RootPath, filepath.FromSlash(merged.StorageKey)))
		version.ScanStatus = scanResult.Status
		version.ScanMessage = scanResult.Message
	}
	var node *model.Node
	if session.NodeID == nil {
		node = &model.Node{WorkspaceID: actor.WorkspaceID, ParentID: session.TargetParentID, Name: session.DisplayName, NormalizedName: strings.ToLower(session.DisplayName), Type: "file", InheritMode: "inherit", Status: "active", CreatedBy: actor.UserID, UpdatedBy: actor.UserID}
	}
	if err := h.uploads.FinalizeSession(session.ID, actor.WorkspaceID, actor.UserID, version, node); err != nil {
		_ = store.RemoveObject(merged.StorageKey)
		h.handleUploadError(c, err)
		return
	}
	if version.ScanStatus == clamav.StatusInfected || version.ScanStatus == clamav.StatusScanError {
		workspaceID := actor.WorkspaceID
		severity := "warning"
		title := "文件扫描失败"
		if version.ScanStatus == clamav.StatusInfected {
			severity = "critical"
			title = "文件未通过安全扫描"
		}
		if _, notifyErr := notification.PublishUser(c.Request.Context(), notification.UserEvent{
			Key:    "upload:scan:" + strconv.FormatUint(uint64(version.ID), 10) + ":" + version.ScanStatus,
			UserID: actor.UserID, WorkspaceID: &workspaceID, Type: "security:upload_scan_" + version.ScanStatus,
			Category: notification.UserCategorySecurity, Severity: severity, Title: title,
			Content:    "“" + session.DisplayName + "”需要安全处理，当前不可对外分享。",
			TargetType: "node", TargetID: strconv.FormatUint(uint64(version.NodeID), 10),
		}); notifyErr != nil {
			logger.Warn("upload_user_notification_publish_failed", "upload_id", session.ID, "version_id", version.ID, "error", notifyErr)
		}
	}
	_ = store.RemoveUpload(session.ID)
	response.Success(c, gin.H{"node_id": version.NodeID, "version_no": version.VersionNo, "size": version.Size, "sha256": version.SHA256, "scan_status": version.ScanStatus})
}

// @Summary Cancel
// @Description Handles DELETE /api/fileshare/v1/management/uploads/{id}.
// @Tags Uploads
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/uploads/{id} [delete]
func (h *UploadHandler) Cancel(c *gin.Context) {
	actor, session, _, ok := h.loadSession(c)
	if !ok {
		return
	}
	if err := h.uploads.Cancel(actor.WorkspaceID, actor.UserID, session.ID); err != nil {
		h.handleUploadError(c, err)
		return
	}
	if store, err := h.posixStorage(); err == nil {
		_ = store.RemoveUpload(session.ID)
	}
	response.Success(c, gin.H{"upload_id": session.ID, "status": "expired"})
}

func (h *UploadHandler) loadSession(c *gin.Context) (authorization.Actor, *model.UploadSession, int, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, 0, false
	}
	sessionID := strings.TrimSpace(c.Param("id"))
	session, err := h.uploads.GetSession(actor.WorkspaceID, actor.UserID, sessionID)
	if err != nil {
		h.handleUploadError(c, err)
		return authorization.Actor{}, nil, 0, false
	}
	partNo := 0
	if value := strings.TrimSpace(c.Param("part_no")); value != "" {
		parsed, err := request.ParseUintParamAllowZero(c, "part_no")
		if err != nil {
			response.BadRequest(c, err.Error())
			return authorization.Actor{}, nil, 0, false
		}
		if parsed > uint(maxUploadChunks) {
			response.BadRequest(c, "part_no 超出允许范围")
			return authorization.Actor{}, nil, 0, false
		}
		partNo = int(parsed)
	}
	return actor, session, partNo, true
}

func configuredMaxUploadFileBytes() int64 {
	if cfg := config.GetConfig(); cfg != nil && cfg.Upload.MaxFileBytes > 0 {
		return cfg.Upload.MaxFileBytes
	}
	return defaultMaxUploadFileBytes
}

func calculateUploadChunkCount(totalSize, chunkSize, maxFileBytes int64) (int, error) {
	if totalSize <= 0 || chunkSize <= 0 {
		return 0, errors.New("上传文件大小和分片大小必须为正数")
	}
	if maxFileBytes <= 0 || totalSize > maxFileBytes {
		return 0, fmt.Errorf("文件大小不能超过 %d 字节", maxFileBytes)
	}
	if chunkSize < minChunkSize || chunkSize > maxChunkSize {
		return 0, errors.New("分片大小必须在 1 MiB 到 64 MiB 之间")
	}
	chunks := totalSize / chunkSize
	if totalSize%chunkSize != 0 {
		chunks++
	}
	if chunks <= 0 || chunks > maxUploadChunks || chunks > int64(^uint(0)>>1) {
		return 0, errors.New("文件分片数量过多")
	}
	return int(chunks), nil
}

func validateUploadSessionDimensions(session *model.UploadSession, maxFileBytes int64) error {
	if session == nil {
		return errors.New("上传会话不存在")
	}
	chunks, err := calculateUploadChunkCount(session.TotalSize, session.ChunkSize, maxFileBytes)
	if err != nil {
		return errors.New("上传会话已超过当前系统限制，请取消后重新上传")
	}
	if session.TotalChunks != chunks {
		return errors.New("上传会话分片信息无效，请取消后重新上传")
	}
	return nil
}

func expectedUploadPartSize(session *model.UploadSession, partNo int) (int64, error) {
	if session == nil || partNo < 0 || partNo >= session.TotalChunks {
		return 0, errors.New("part number out of range")
	}
	if partNo != session.TotalChunks-1 {
		return session.ChunkSize, nil
	}
	lastSize := session.TotalSize % session.ChunkSize
	if lastSize == 0 {
		lastSize = session.ChunkSize
	}
	return lastSize, nil
}

func (h *UploadHandler) posixStorage() (*storage.POSIX, error) {
	return configuredStorage()
}

func configuredStorage() (*storage.POSIX, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, errors.New("configuration is not loaded")
	}
	return storage.NewPOSIX(cfg.Storage.RootPath, cfg.Storage.StagingPath)
}

func (h *UploadHandler) handleUploadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, dao.ErrUploadNotFound):
		response.NotFound(c, "上传会话不存在")
	case errors.Is(err, dao.ErrUploadState):
		response.Conflict(c, "上传会话当前状态不允许此操作")
	case errors.Is(err, dao.ErrVersionConflict):
		response.Conflict(c, "文件版本已变化，请刷新后重试")
	case errors.Is(err, dao.ErrUploadIncomplete):
		response.Conflict(c, "上传尚未收到全部分片")
	default:
		response.InternalError(c, "上传操作失败", err)
	}
}

func inspectObject(store *storage.POSIX, key, displayName string, size int64) (string, bool, error) {
	file, err := store.OpenObject(key)
	if err != nil {
		return "application/octet-stream", false, err
	}
	defer file.Close()
	buffer := make([]byte, 512)
	read, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return "application/octet-stream", false, err
	}
	mime := http.DetectContentType(buffer[:read])
	if err := validateDetectedFile(displayName, mime, buffer[:read]); err != nil {
		return mime, false, err
	}
	zipCandidate := isZIPHeader(buffer[:read])
	if !zipCandidate {
		return mime, false, nil
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return mime, false, err
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return mime, false, errors.New("ZIP 文件结构无效")
	}
	if err := validateArchivePackage(filepath.Ext(displayName), archive.File); err != nil {
		return mime, false, err
	}
	encrypted, err := validateZIPArchive(archive, 1, &zipValidationBudget{})
	if err != nil {
		return mime, false, err
	}
	return mime, encrypted, nil
}

type zipValidationBudget struct {
	entries      int
	expandedSize uint64
}

func validateZIPArchive(archive *zip.Reader, depth int, budget *zipValidationBudget) (bool, error) {
	if archive == nil || budget == nil {
		return false, errors.New("ZIP 文件结构无效")
	}
	if len(archive.File) > maxZIPEntries-budget.entries {
		return false, errors.New("ZIP 文件条目数量超过限制")
	}
	encrypted := false
	for _, entry := range archive.File {
		budget.entries++
		cleanName := path.Clean(strings.ReplaceAll(entry.Name, "\\", "/"))
		if strings.ContainsRune(entry.Name, '\x00') || strings.HasPrefix(cleanName, "/") || cleanName == ".." || strings.HasPrefix(cleanName, "../") {
			return false, errors.New("ZIP 文件包含不安全路径")
		}
		if entry.UncompressedSize64 > maxZIPExpandedSize-budget.expandedSize {
			return false, errors.New("ZIP 解压大小超过限制")
		}
		budget.expandedSize += entry.UncompressedSize64
		if entry.UncompressedSize64 >= minZIPRatioCheckSize &&
			(entry.CompressedSize64 == 0 || entry.UncompressedSize64/entry.CompressedSize64 > maxZIPCompressionRatio) {
			return false, errors.New("ZIP 压缩比超过限制")
		}

		if entry.Flags&0x1 != 0 {
			encrypted = true
			continue
		}
		if entry.FileInfo().IsDir() || strings.ToLower(path.Ext(cleanName)) != ".zip" {
			continue
		}
		if depth >= maxZIPNestingDepth {
			return false, errors.New("ZIP 嵌套层数超过限制")
		}
		if entry.UncompressedSize64 > uint64(maxNestedZIPSize) {
			return false, errors.New("嵌套 ZIP 文件超过检查大小限制")
		}
		reader, err := entry.Open()
		if err != nil {
			return false, errors.New("嵌套 ZIP 文件无法读取")
		}
		data, readErr := io.ReadAll(io.LimitReader(reader, maxNestedZIPSize+1))
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || int64(len(data)) > maxNestedZIPSize {
			return false, errors.New("嵌套 ZIP 文件无法安全读取")
		}
		nested, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return false, errors.New("嵌套 ZIP 文件结构无效")
		}
		nestedEncrypted, err := validateZIPArchive(nested, depth+1, budget)
		if err != nil {
			return false, err
		}
		encrypted = encrypted || nestedEncrypted
	}
	return encrypted, nil
}

func riskLevel(encrypted bool) string {
	if encrypted {
		return "high"
	}
	return "unknown"
}

func nullableString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func randomUploadID() string {
	return "upload-" + uuid.NewString()
}
