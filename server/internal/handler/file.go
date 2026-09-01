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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/auditcontext"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/clamav"
	"file-share-manager/server/internal/storage"

	"github.com/gin-gonic/gin"
)

type FileHandler struct {
	nodes         *dao.NodeDAO
	files         *dao.FileDAO
	audit         *dao.OperationLogDAO
	collaboration *dao.CollaborationDAO
	authz         *authorization.Service
	reader        func() (*storage.VersionReader, error)
}

func NewFileHandler() *FileHandler {
	return &FileHandler{nodes: dao.NewNodeDAO(), files: dao.NewFileDAO(), audit: dao.NewOperationLogDAO(), collaboration: dao.NewCollaborationDAO(), authz: authorization.NewService(), reader: configuredVersionReader}
}

// @Summary List Versions
// @Description Handles GET /api/fileshare/v1/management/files/{id}/versions.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/versions [get]
func (h *FileHandler) ListVersions(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	versions, err := h.files.ListVersions(actor.WorkspaceID, node.ID)
	if err != nil {
		response.InternalError(c, "读取文件版本失败", err)
		return
	}
	for index := range versions {
		versions[index].StorageKey = ""
	}
	rememberRecentNode(h.collaboration, actor, node.ID)
	response.Success(c, versions)
}

// @Summary Download
// @Description Handles GET /api/fileshare/v1/management/files/{id}/download.
// @Tags Files and folders
// @Accept json
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path string true "id"
// @Param version query string false "version"
// @Success 200 {file} binary
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/download [get]
func (h *FileHandler) Download(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	version, ok := h.requestedVersion(c, actor.WorkspaceID, node.ID)
	if !ok {
		return
	}
	if version.ScanStatus == "infected" || version.ScanStatus == "pending_scan" || version.ScanStatus == "scan_error" {
		started := time.Now()
		_ = h.recordDownloadAuditWithReason(c, actor, node, version, "file:download_denied", http.StatusForbidden, started, 0, model.AuditReasonUnsafeScanStatus)
		response.Forbidden(c, "文件当前不可下载")
		return
	}
	if storageRequiresRestore(version.StorageClass) {
		response.Conflict(c, "文件处于深冷归档状态，需要先在对象存储中解冻")
		return
	}
	if err := h.recordCrossWorkspaceReadAudit(c, actor, node, version); err != nil {
		response.ServiceUnavailable(c, "跨空间读取审计服务暂不可用")
		return
	}
	start := time.Now()
	if err := h.recordDownloadAudit(c, actor, node, version, "file:download_start", http.StatusOK, start, 0); err != nil {
		response.ServiceUnavailable(c, "下载审计服务暂不可用")
		return
	}
	reader, err := h.reader()
	if err != nil {
		_ = h.recordDownloadAudit(c, actor, node, version, "file:download_failed", http.StatusInternalServerError, start, 0)
		response.InternalError(c, "存储未配置", err)
		return
	}
	file, err := reader.Open(version.StorageClass, version.StorageKey)
	if err != nil {
		status := http.StatusNotFound
		if errors.Is(err, storage.ErrArchiveRestoreRequired) {
			status = http.StatusConflict
		}
		_ = h.recordDownloadAudit(c, actor, node, version, "file:download_failed", status, start, 0)
		if errors.Is(err, storage.ErrArchiveRestoreRequired) {
			response.Conflict(c, "文件处于深冷归档状态，需要先在对象存储中解冻")
			return
		}
		if errors.Is(err, storage.ErrInvalidObjectKey) {
			response.InternalError(c, "文件存储对象无效", err)
			return
		}
		response.NotFound(c, "文件对象不存在")
		return
	}
	defer file.Close()
	filename := strings.ReplaceAll(strings.ReplaceAll(node.Name, "\r", "_"), "\n", "_")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; %s", contentDisposition(filename)))
	if version.DetectedMime != "" {
		c.Header("Content-Type", version.DetectedMime)
	}
	if seeker, ok := file.(io.ReadSeeker); ok {
		http.ServeContent(c.Writer, c.Request, filename, version.CreatedAt, seeker)
	} else {
		c.Header("Content-Length", strconv.FormatInt(version.Size, 10))
		c.Status(http.StatusOK)
		_, _ = io.Copy(c.Writer, file)
	}
	_ = h.files.TouchAccess(version.ID, time.Now())
	rememberRecentNode(h.collaboration, actor, node.ID)
	if err := h.recordDownloadAudit(c, actor, node, version, "file:download_complete", c.Writer.Status(), start, c.Writer.Size()); err != nil {
		// The response is already streaming; retain the error in request logs.
		c.Error(err)
	}
}

type previewDescriptor struct {
	Kind      string `json:"kind"`
	MIMEType  string `json:"mime_type"`
	Size      int64  `json:"size"`
	VersionNo int    `json:"version_no"`
}

type previewRestriction struct {
	status int
	reason string
	text   string
}

var imagePreviewMIMEs = map[string]string{
	".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp",
}

var textPreviewMIMEs = map[string]map[string]struct{}{
	".txt":  {"text/plain": {}},
	".md":   {"text/plain": {}, "text/markdown": {}},
	".csv":  {"text/plain": {}, "text/csv": {}},
	".json": {"text/plain": {}, "application/json": {}},
	".xml":  {"text/plain": {}, "application/xml": {}, "text/xml": {}},
	".yaml": {"text/plain": {}, "application/yaml": {}, "text/yaml": {}, "application/x-yaml": {}},
	".yml":  {"text/plain": {}, "application/yaml": {}, "text/yaml": {}, "application/x-yaml": {}},
	".log":  {"text/plain": {}},
	".conf": {"text/plain": {}},
	".ini":  {"text/plain": {}},
}

// @Summary Preview Info
// @Description Handles GET /api/fileshare/v1/management/files/{id}/preview.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param version query string false "version"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/preview [get]
func (h *FileHandler) PreviewInfo(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	version, ok := h.requestedVersion(c, actor.WorkspaceID, node.ID)
	if !ok {
		return
	}
	descriptor, restriction := previewPolicy(version, config.GetConfig())
	if restriction != nil {
		started := time.Now()
		if err := h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_denied", restriction.status, started, 0, restriction.reason); err != nil {
			response.ServiceUnavailable(c, "预览审计服务暂不可用")
			return
		}
		writePreviewRestriction(c, restriction)
		return
	}
	response.Success(c, descriptor)
}

// @Summary Preview Content
// @Description Handles GET /api/fileshare/v1/management/files/{id}/preview/content.
// @Tags Files and folders
// @Accept json
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path string true "id"
// @Param version query string false "version"
// @Success 200 {file} binary
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/preview/content [get]
func (h *FileHandler) PreviewContent(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	version, ok := h.requestedVersion(c, actor.WorkspaceID, node.ID)
	if !ok {
		return
	}
	descriptor, restriction := previewPolicy(version, config.GetConfig())
	started := time.Now()
	if restriction != nil {
		if err := h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_denied", restriction.status, started, 0, restriction.reason); err != nil {
			response.ServiceUnavailable(c, "预览审计服务暂不可用")
			return
		}
		writePreviewRestriction(c, restriction)
		return
	}
	if err := h.recordCrossWorkspaceReadAudit(c, actor, node, version); err != nil {
		response.ServiceUnavailable(c, "跨空间读取审计服务暂不可用")
		return
	}
	if err := h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_start", http.StatusOK, started, 0, ""); err != nil {
		response.ServiceUnavailable(c, "预览审计服务暂不可用")
		return
	}
	reader, err := h.reader()
	if err != nil {
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", http.StatusInternalServerError, started, 0, model.AuditReasonStorageUnavailable)
		response.InternalError(c, "存储未配置", err)
		return
	}
	file, err := reader.Open(version.StorageClass, version.StorageKey)
	if err != nil {
		status, reason, message := previewStorageError(err)
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", status, started, 0, reason)
		if status == http.StatusConflict {
			response.Conflict(c, message)
		} else if status == http.StatusInternalServerError {
			response.InternalError(c, message, err)
		} else {
			response.NotFound(c, message)
		}
		return
	}
	defer file.Close()

	limits := previewLimits(config.GetConfig())
	limit := limits.binary
	if descriptor.Kind == "text" {
		limit = limits.text
	}
	content, readErr := io.ReadAll(io.LimitReader(file, limit+1))
	if readErr != nil {
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", http.StatusInternalServerError, started, 0, model.AuditReasonStorageUnavailable)
		response.InternalError(c, "读取预览内容失败", readErr)
		return
	}
	if int64(len(content)) > limit {
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", http.StatusRequestEntityTooLarge, started, 0, model.AuditReasonPreviewTooLarge)
		response.PayloadTooLarge(c, "文件实际内容超过在线预览大小限制")
		return
	}
	if !validPreviewContent(version.Extension, descriptor.Kind, content) {
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", http.StatusUnsupportedMediaType, started, 0, model.AuditReasonUnsupportedMediaType)
		response.UnsupportedMediaType(c, "文件实际内容与预览格式不匹配")
		return
	}

	filename := strings.ReplaceAll(strings.ReplaceAll(node.Name, "\r", "_"), "\n", "_")
	setPreviewHeaders(c, filename, descriptor.MIMEType, int64(len(content)))
	c.Status(http.StatusOK)
	written, copyErr := io.Copy(c.Writer, bytes.NewReader(content))
	if copyErr != nil {
		_ = h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_failed", http.StatusInternalServerError, started, int(written), model.AuditReasonStorageUnavailable)
		_ = c.Error(copyErr)
		return
	}
	_ = h.files.TouchAccess(version.ID, time.Now())
	rememberRecentNode(h.collaboration, actor, node.ID)
	if err := h.recordPreviewAudit(c, actor, node, version, descriptor.Kind, "file:preview_complete", http.StatusOK, started, int(written), ""); err != nil {
		_ = c.Error(err)
	}
}

func (h *FileHandler) requestedVersion(c *gin.Context, workspaceID, nodeID uint) (*model.FileVersion, bool) {
	versionNo := 0
	if value := strings.TrimSpace(c.Query("version")); value != "" {
		parsed, err := strconv.ParseUint(value, 10, 32)
		if err != nil || parsed == 0 {
			response.BadRequest(c, "version 必须是正整数")
			return nil, false
		}
		versionNo = int(parsed)
	}
	var version *model.FileVersion
	var err error
	if versionNo == 0 {
		version, err = h.files.GetLatestVersion(workspaceID, nodeID)
	} else {
		version, err = h.files.GetVersion(workspaceID, nodeID, versionNo)
	}
	if err != nil {
		response.InternalError(c, "读取文件版本失败", err)
		return nil, false
	}
	if version == nil {
		response.NotFound(c, "文件版本不存在")
		return nil, false
	}
	return version, true
}

type configuredPreviewLimits struct {
	binary int64
	text   int64
}

func previewLimits(cfg *config.Config) configuredPreviewLimits {
	limits := configuredPreviewLimits{binary: 25 << 20, text: 1 << 20}
	if cfg == nil {
		return limits
	}
	if cfg.Preview.MaxBinaryBytes > 0 {
		limits.binary = cfg.Preview.MaxBinaryBytes
	}
	if cfg.Preview.MaxTextBytes > 0 {
		limits.text = cfg.Preview.MaxTextBytes
	}
	return limits
}

func previewPolicy(version *model.FileVersion, cfg *config.Config) (previewDescriptor, *previewRestriction) {
	descriptor := previewDescriptor{Size: version.Size, VersionNo: version.VersionNo}
	if version.ScanStatus == "infected" || version.ScanStatus == "pending_scan" || version.ScanStatus == "scan_error" {
		return descriptor, &previewRestriction{status: http.StatusForbidden, reason: model.AuditReasonUnsafeScanStatus, text: "文件当前不可预览"}
	}
	if storageRequiresRestore(version.StorageClass) {
		return descriptor, &previewRestriction{status: http.StatusConflict, reason: model.AuditReasonArchiveRestoreRequired, text: "文件处于深冷归档状态，需要先在对象存储中解冻"}
	}
	extension := strings.ToLower(strings.TrimSpace(version.Extension))
	mimeType := strings.ToLower(strings.TrimSpace(version.DetectedMime))
	if parsed, _, err := mime.ParseMediaType(mimeType); err == nil {
		mimeType = parsed
	}
	limits := previewLimits(cfg)
	if expected, supported := imagePreviewMIMEs[extension]; supported && mimeType == expected {
		descriptor.Kind, descriptor.MIMEType = "image", expected
		if version.Size > limits.binary {
			return descriptor, &previewRestriction{status: http.StatusRequestEntityTooLarge, reason: model.AuditReasonPreviewTooLarge, text: "图片超过在线预览大小限制"}
		}
		return descriptor, nil
	}
	if extension == ".pdf" && mimeType == "application/pdf" {
		descriptor.Kind, descriptor.MIMEType = "pdf", "application/pdf"
		if version.Size > limits.binary {
			return descriptor, &previewRestriction{status: http.StatusRequestEntityTooLarge, reason: model.AuditReasonPreviewTooLarge, text: "PDF 超过在线预览大小限制"}
		}
		return descriptor, nil
	}
	if allowedMIMEs, supported := textPreviewMIMEs[extension]; supported {
		if _, allowed := allowedMIMEs[mimeType]; allowed {
			descriptor.Kind, descriptor.MIMEType = "text", "text/plain; charset=utf-8"
			if version.Size > limits.text {
				return descriptor, &previewRestriction{status: http.StatusRequestEntityTooLarge, reason: model.AuditReasonPreviewTooLarge, text: "文本文件超过在线预览大小限制"}
			}
			return descriptor, nil
		}
	}
	return descriptor, &previewRestriction{status: http.StatusUnsupportedMediaType, reason: model.AuditReasonUnsupportedMediaType, text: "该文件类型暂不支持在线预览，请下载后查看"}
}

func validPreviewText(content []byte) bool {
	return utf8.Valid(content) && !bytes.ContainsRune(content, '\x00')
}

func validPreviewContent(extension, kind string, content []byte) bool {
	if kind == "text" {
		return validPreviewText(content)
	}
	extension = strings.ToLower(strings.TrimSpace(extension))
	switch extension {
	case ".pdf":
		return bytes.HasPrefix(content, []byte("%PDF-"))
	case ".png":
		return bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	case ".jpg", ".jpeg":
		return bytes.HasPrefix(content, []byte{0xff, 0xd8, 0xff})
	case ".gif":
		return bytes.HasPrefix(content, []byte("GIF87a")) || bytes.HasPrefix(content, []byte("GIF89a"))
	case ".webp":
		return hasRIFFType(content, "WEBP")
	case ".bmp":
		return bytes.HasPrefix(content, []byte("BM"))
	default:
		return false
	}
}

func writePreviewRestriction(c *gin.Context, restriction *previewRestriction) {
	switch restriction.status {
	case http.StatusForbidden:
		response.Forbidden(c, restriction.text)
	case http.StatusConflict:
		response.Conflict(c, restriction.text)
	case http.StatusRequestEntityTooLarge:
		response.PayloadTooLarge(c, restriction.text)
	default:
		response.UnsupportedMediaType(c, restriction.text)
	}
}

func previewStorageError(err error) (int, string, string) {
	if errors.Is(err, storage.ErrArchiveRestoreRequired) {
		return http.StatusConflict, model.AuditReasonArchiveRestoreRequired, "文件处于深冷归档状态，需要先在对象存储中解冻"
	}
	if errors.Is(err, storage.ErrInvalidObjectKey) {
		return http.StatusInternalServerError, model.AuditReasonStorageUnavailable, "文件存储对象无效"
	}
	return http.StatusNotFound, model.AuditReasonObjectNotFound, "文件对象不存在"
}

func setPreviewHeaders(c *gin.Context, filename, mimeType string, size int64) {
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=preview; filename*=UTF-8''%s", url.PathEscape(filename)))
	c.Header("Content-Type", mimeType)
	c.Header("Content-Length", strconv.FormatInt(size, 10))
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Security-Policy", "sandbox; default-src 'none'")
	c.Header("Cross-Origin-Resource-Policy", "same-origin")
	c.Header("Referrer-Policy", "no-referrer")
}

func (h *FileHandler) recordPreviewAudit(c *gin.Context, actor authorization.Actor, node *model.Node, version *model.FileVersion, kind, action string, status int, started time.Time, bytesSent int, reason string) error {
	if h.audit == nil {
		return errors.New("preview audit is not configured")
	}
	detailValue := map[string]any{
		"file_version_id": version.ID, "version_no": version.VersionNo, "size": version.Size,
		"sha256": version.SHA256, "scan_status": version.ScanStatus, "preview_kind": kind,
		"bytes_sent": bytesSent, "transfer_duration_ms": time.Since(started).Milliseconds(),
		"authorization_checks": auditcontext.AuthorizationChecks(c),
	}
	if reason != "" {
		detailValue["reason"] = reason
	}
	detailsBytes, err := json.Marshal(detailValue)
	if err != nil {
		return err
	}
	username, _ := c.Get("username")
	requestID, _ := c.Get("request_id")
	workspaceID, nodeID := actor.WorkspaceID, node.ID
	entry := &model.OperationLog{
		UserID: actor.UserID, Username: stringValue(username), WorkspaceID: &workspaceID, NodeID: &nodeID,
		ActorWorkspaceID: &workspaceID, TargetWorkspaceID: &workspaceID,
		TargetType: "file", TargetID: strconv.FormatUint(uint64(node.ID), 10), TargetName: node.Name,
		Method: c.Request.Method, Path: c.Request.URL.Path, Action: action, Status: status,
		IP: c.ClientIP(), Latency: time.Since(started).Milliseconds(), Details: string(detailsBytes),
		RequestID: stringValue(requestID), UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	applyCrossWorkspaceAuditContext(c, entry)
	return h.audit.Create(entry)
}

func storageRequiresRestore(storageClass string) bool {
	storageClass = strings.ToLower(strings.TrimSpace(storageClass))
	return storageClass == "glacier" || storageClass == "restoring"
}

func (h *FileHandler) recordDownloadAudit(c *gin.Context, actor authorization.Actor, node *model.Node, version *model.FileVersion, action string, status int, started time.Time, bytesSent int) error {
	return h.recordDownloadAuditWithReason(c, actor, node, version, action, status, started, bytesSent, "")
}

func (h *FileHandler) recordDownloadAuditWithReason(c *gin.Context, actor authorization.Actor, node *model.Node, version *model.FileVersion, action string, status int, started time.Time, bytesSent int, reason string) error {
	if h.audit == nil {
		return errors.New("download audit is not configured")
	}
	username, _ := c.Get("username")
	usernameValue, _ := username.(string)
	requestID, _ := c.Get("request_id")
	requestIDValue, _ := requestID.(string)
	workspaceID := actor.WorkspaceID
	nodeID := node.ID
	detailValue := map[string]any{
		"file_version_id": version.ID, "size": version.Size, "sha256": version.SHA256,
		"scan_status": version.ScanStatus, "range": c.GetHeader("Range"), "bytes_sent": bytesSent,
		"transfer_duration_ms": time.Since(started).Milliseconds(),
		"authorization_checks": auditcontext.AuthorizationChecks(c),
	}
	if reason != "" {
		detailValue["reason"] = reason
	}
	detailsBytes, err := json.Marshal(detailValue)
	if err != nil {
		return err
	}
	entry := &model.OperationLog{
		UserID: actor.UserID, Username: usernameValue, WorkspaceID: &workspaceID, NodeID: &nodeID,
		ActorWorkspaceID: &workspaceID, TargetWorkspaceID: &workspaceID,
		TargetType: "file", TargetID: strconv.FormatUint(uint64(node.ID), 10), TargetName: node.Name,
		Method: c.Request.Method, Path: c.Request.URL.Path, Action: action, Status: status,
		IP: c.ClientIP(), Latency: time.Since(started).Milliseconds(), Details: string(detailsBytes),
		RequestID: requestIDValue, UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	}
	applyCrossWorkspaceAuditContext(c, entry)
	return h.audit.Create(entry)
}

func (h *FileHandler) recordCrossWorkspaceReadAudit(c *gin.Context, actor authorization.Actor, node *model.Node, version *model.FileVersion) error {
	access, exists := auditcontext.CrossWorkspaceAccessFrom(c)
	if !exists {
		return nil
	}
	if h.audit == nil {
		return errors.New("cross-workspace audit is not configured")
	}
	details, err := json.Marshal(map[string]any{
		"source_workspace_id":  access.SourceWorkspaceID,
		"target_workspace_id":  access.TargetWorkspaceID,
		"operation_reason":     access.Reason,
		"file_version_id":      version.ID,
		"authorization_checks": auditcontext.AuthorizationChecks(c),
	})
	if err != nil {
		return err
	}
	username, _ := c.Get("username")
	requestID, _ := c.Get("request_id")
	nodeID := node.ID
	return h.audit.Create(&model.OperationLog{
		UserID: actor.UserID, Username: stringValue(username), WorkspaceID: &access.TargetWorkspaceID,
		ActorWorkspaceID: access.SourceWorkspaceID, SourceWorkspaceID: access.SourceWorkspaceID, TargetWorkspaceID: &access.TargetWorkspaceID,
		NodeID: &nodeID, TargetType: "file", TargetID: strconv.FormatUint(uint64(node.ID), 10), TargetName: node.Name,
		Method: c.Request.Method, Path: c.Request.URL.Path, Action: "super_admin_cross_workspace_read",
		Category: model.AuditCategorySecurity, Severity: model.AuditSeverityHigh, Result: model.AuditResultSuccess,
		Status: http.StatusOK, IP: c.ClientIP(), Details: string(details), RequestID: stringValue(requestID),
		UserAgent: c.Request.UserAgent(), CreatedAt: time.Now(),
	})
}

func applyCrossWorkspaceAuditContext(c *gin.Context, entry *model.OperationLog) {
	access, exists := auditcontext.CrossWorkspaceAccessFrom(c)
	if !exists || entry == nil {
		return
	}
	entry.ActorWorkspaceID = access.SourceWorkspaceID
	entry.SourceWorkspaceID = access.SourceWorkspaceID
	entry.TargetWorkspaceID = &access.TargetWorkspaceID
}

// @Summary Restore Version
// @Description Handles POST /api/fileshare/v1/management/files/{id}/versions/{version}/restore.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param version path string true "version"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/versions/{version}/restore [post]
func (h *FileHandler) RestoreVersion(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	allowed, err := h.authz.CanWrite(actor, node.ID)
	if err != nil {
		response.InternalError(c, "文件权限校验失败", err)
		return
	}
	recordDataAuthorization(c, allowed, "node:write", "file", node.ID)
	if !allowed {
		response.Forbidden(c, "无权恢复该文件版本")
		return
	}
	versionNo, err := request.ParseUintParam(c, "version")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	source, err := h.files.GetVersion(actor.WorkspaceID, node.ID, int(versionNo))
	if err != nil {
		response.InternalError(c, "读取历史版本失败", err)
		return
	}
	if source == nil {
		response.NotFound(c, "历史版本不存在")
		return
	}
	if source.ScanStatus == "infected" || source.ScanStatus == "pending_scan" || source.ScanStatus == "scan_error" {
		response.Conflict(c, "该历史版本的安全状态不允许恢复")
		return
	}
	if storageRequiresRestore(source.StorageClass) {
		response.Conflict(c, "该历史版本处于深冷归档状态，需要先在对象存储中解冻")
		return
	}
	restored, err := h.files.RestoreVersion(actor.WorkspaceID, node.ID, int(versionNo), actor.UserID)
	if err != nil {
		response.InternalError(c, "恢复历史版本失败", err)
		return
	}
	response.Success(c, restored)
}

// @Summary Rescan Version
// @Description Handles POST /api/fileshare/v1/management/files/{id}/versions/{version}/rescan.
// @Tags Files and folders
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Param version path string true "version"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/files/{id}/versions/{version}/rescan [post]
func (h *FileHandler) RescanVersion(c *gin.Context) {
	actor, node, ok := h.authorize(c)
	if !ok {
		return
	}
	allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
	if !allowed {
		var err error
		allowed, err = h.authz.CanManageACL(actor, node.ID)
		if err != nil {
			response.InternalError(c, "文件权限校验失败", err)
			return
		}
	}
	recordDataAuthorization(c, allowed, "node:manage_acl", "file", node.ID)
	if !allowed {
		response.Forbidden(c, "无权重新扫描该文件版本")
		return
	}
	versionNo, err := request.ParseUintParam(c, "version")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	source, err := h.files.GetVersion(actor.WorkspaceID, node.ID, int(versionNo))
	if err != nil {
		response.InternalError(c, "读取文件版本失败", err)
		return
	}
	if source == nil {
		response.NotFound(c, "文件版本不存在")
		return
	}
	if normalizedVersionStorageClass(source.StorageClass) != "standard" {
		response.Conflict(c, "归档文件需要恢复到标准存储后才能重新扫描")
		return
	}
	if source.ScanStatus == "infected" {
		response.Conflict(c, "感染文件需要人工处置，不能直接重新扫描")
		return
	}
	cfg := config.GetConfig()
	if cfg == nil || !cfg.ClamAV.Enabled() {
		updated, err := h.files.UpdateScanStatus(actor.WorkspaceID, node.ID, int(versionNo), clamav.StatusUnscanned, "ClamAV 未配置")
		if err != nil {
			response.InternalError(c, "更新扫描状态失败", err)
			return
		}
		response.SuccessWithMessage(c, "ClamAV 未配置，文件保持 unscanned 状态", updated)
		return
	}
	if _, err := h.files.UpdateScanStatus(actor.WorkspaceID, node.ID, int(versionNo), "pending_scan", "等待重新扫描"); err != nil {
		response.InternalError(c, "更新扫描状态失败", err)
		return
	}
	scanResult := clamav.ScanFile(c.Request.Context(), filepath.Join(cfg.Storage.RootPath, filepath.FromSlash(source.StorageKey)))
	updated, err := h.files.UpdateScanStatus(actor.WorkspaceID, node.ID, int(versionNo), scanResult.Status, scanResult.Message)
	if err != nil {
		response.InternalError(c, "更新扫描结果失败", err)
		return
	}
	response.SuccessWithMessage(c, "文件版本重新扫描完成", updated)
}

func (h *FileHandler) authorize(c *gin.Context) (authorization.Actor, *model.Node, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, false
	}
	nodeID, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return authorization.Actor{}, nil, false
	}
	node, err := h.nodes.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		response.InternalError(c, "读取文件失败", err)
		return authorization.Actor{}, nil, false
	}
	if node == nil || node.Type != "file" || node.Status != "active" {
		response.NotFound(c, "文件不存在")
		return authorization.Actor{}, nil, false
	}
	allowed, err := h.authz.CanRead(actor, node.ID)
	if err != nil {
		response.InternalError(c, "文件权限校验失败", err)
		return authorization.Actor{}, nil, false
	}
	recordDataAuthorization(c, allowed, "node:read", "file", node.ID)
	if !allowed {
		response.Forbidden(c, "无权读取该文件")
		return authorization.Actor{}, nil, false
	}
	return actor, node, true
}

func contentDisposition(filename string) string {
	return fmt.Sprintf("filename=download; filename*=UTF-8''%s", url.PathEscape(filename))
}

func configuredVersionReader() (*storage.VersionReader, error) {
	return storage.NewConfiguredVersionReader(context.Background(), config.GetConfig())
}
