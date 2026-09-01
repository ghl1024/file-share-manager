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
	"regexp"
	"strings"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	backupservice "file-share-manager/server/internal/service/backup"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type BackupHandler struct {
	service *backupservice.Service
}

// @Summary Restore Workspace
// @Description Handles POST /api/fileshare/v1/management/backups/{id}/restore-workspace.
// @Tags Backup and restore
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
// @Router /management/backups/{id}/restore-workspace [post]
func (h *BackupHandler) RestoreWorkspace(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Name    string `json:"name" binding:"required,max=128"`
		Code    string `json:"code" binding:"required,max=64"`
		Confirm bool   `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Code = strings.ToLower(strings.TrimSpace(req.Code))
	if req.Name == "" {
		response.BadRequest(c, "工作空间名称不能为空")
		return
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]{2,63}$`).MatchString(req.Code) {
		response.BadRequest(c, "工作空间代号必须以小写字母开头，仅包含小写字母、数字和连字符")
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "恢复完整工作空间必须明确确认")
		return
	}
	result, err := h.service.RestoreWorkspace(c.Request.Context(), actor.WorkspaceID, actor.UserID, c.Param("id"), req.Name, req.Code)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "备份任务不存在")
	case errors.Is(err, backupservice.ErrMetadataSnapshotMissing):
		response.Conflict(c, "该历史备份不包含完整元数据快照，请先创建新的备份点")
	case errors.Is(err, backupservice.ErrRestoreWorkspaceExists):
		response.Conflict(c, "工作空间代号已存在，请更换后重试")
	case errors.Is(err, backupservice.ErrRestoreWorkspaceReference):
		response.Conflict(c, "备份引用的用户或权限已不存在，无法完整恢复")
	case errors.Is(err, backupservice.ErrRestoreObjectNotFound):
		response.Conflict(c, "备份链缺少工作空间恢复所需的文件对象")
	case errors.Is(err, dao.ErrQuotaExceeded):
		response.Conflict(c, "备份数据量超过快照中的空间配额，无法恢复")
	case errors.Is(err, backupservice.ErrBackupBackendUnsupported):
		response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
	case errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing):
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
	case err != nil:
		_ = c.Error(err)
		response.Conflict(c, "完整工作空间恢复失败")
	default:
		response.Success(c, result)
	}
}

// @Summary Restore
// @Description Handles POST /api/fileshare/v1/management/backups/{id}/restore.
// @Tags Backup and restore
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
// @Router /management/backups/{id}/restore [post]
func (h *BackupHandler) Restore(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id := c.Param("id")
	var req struct {
		VersionID uint  `json:"version_id" binding:"required,gt=0"`
		ParentID  *uint `json:"parent_id"`
		Confirm   bool  `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "恢复文件必须明确确认")
		return
	}
	node, err := h.service.RestoreFile(c.Request.Context(), actor.WorkspaceID, actor.UserID, id, req.VersionID, req.ParentID)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound), errors.Is(err, backupservice.ErrRestoreObjectNotFound):
		response.NotFound(c, "备份任务或文件对象不存在")
	case errors.Is(err, backupservice.ErrRestoreNameConflict):
		response.Conflict(c, "目标目录中已存在同名文件，恢复不会覆盖现有文件")
	case errors.Is(err, dao.ErrQuotaExceeded):
		response.Conflict(c, "工作空间或个人配额不足，无法恢复该文件")
	case errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing):
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
	case err != nil:
		response.Conflict(c, "文件恢复失败")
	default:
		response.Success(c, node)
	}
}

func NewBackupHandler() *BackupHandler { return &BackupHandler{service: backupservice.NewService()} }

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/backups.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups [get]
func (h *BackupHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	result, err := h.service.List(actor.WorkspaceID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取备份任务失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

// @Summary List Restore Drills
// @Description Handles GET /api/fileshare/v1/management/backup-restore-drills.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backup-restore-drills [get]
func (h *BackupHandler) ListRestoreDrills(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 10, MaxPageSize: 100})
	result, err := h.service.ListRestoreDrills(actor.WorkspaceID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取恢复演练记录失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

// @Summary Health
// @Description Handles GET /api/fileshare/v1/management/backups/health.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups/health [get]
func (h *BackupHandler) Health(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	health, err := h.service.Health(c.Request.Context(), actor.WorkspaceID)
	if err != nil {
		response.InternalError(c, "检查备份健康状态失败", err)
		return
	}
	response.Success(c, health)
}

// @Summary Create Baseline
// @Description Handles POST /api/fileshare/v1/management/backups/baseline.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups/baseline [post]
func (h *BackupHandler) CreateBaseline(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	job, err := h.service.CreateBaseline(c.Request.Context(), actor.WorkspaceID, actor.UserID)
	if err != nil {
		if errors.Is(err, backupservice.ErrBackupInProgress) {
			response.Conflict(c, "当前工作空间已有备份任务正在执行")
			return
		}
		if errors.Is(err, backupservice.ErrBackupBackendUnsupported) {
			response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
			return
		}
		if errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing) {
			response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
			return
		}
		response.InternalError(c, "创建基线备份失败", err)
		return
	}
	response.Success(c, job)
}

// @Summary Create Incremental
// @Description Handles POST /api/fileshare/v1/management/backups/incremental.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups/incremental [post]
func (h *BackupHandler) CreateIncremental(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	job, err := h.service.CreateIncremental(c.Request.Context(), actor.WorkspaceID, actor.UserID)
	if errors.Is(err, backupservice.ErrBackupBaselineMissing) {
		response.Conflict(c, "创建增量备份前必须先完成一次基线备份")
		return
	}
	if errors.Is(err, backupservice.ErrBackupInProgress) {
		response.Conflict(c, "当前工作空间已有备份任务正在执行")
		return
	}
	if errors.Is(err, backupservice.ErrBackupBackendUnsupported) {
		response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
		return
	}
	if errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing) {
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
		return
	}
	if err != nil {
		response.InternalError(c, "创建增量备份失败", err)
		return
	}
	response.Success(c, job)
}

// @Summary Compact
// @Description Handles POST /api/fileshare/v1/management/backups/compact.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body object true "Request body"
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups/compact [post]
func (h *BackupHandler) Compact(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Confirm bool `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "压缩备份链必须明确确认")
		return
	}
	job, err := h.service.CompactManual(c.Request.Context(), actor.WorkspaceID, actor.UserID)
	switch {
	case errors.Is(err, backupservice.ErrBackupCompactionNotNeeded):
		response.Conflict(c, "当前备份链没有需要压缩的增量任务")
	case errors.Is(err, backupservice.ErrBackupBaselineMissing):
		response.Conflict(c, "压缩备份链前必须先完成一次基线备份")
	case errors.Is(err, backupservice.ErrBackupInProgress):
		response.Conflict(c, "当前工作空间已有备份任务正在执行")
	case errors.Is(err, backupservice.ErrMetadataSnapshotMissing):
		response.Conflict(c, "当前备份链缺少完整元数据快照，无法压缩")
	case errors.Is(err, backupservice.ErrBackupBackendUnsupported):
		response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
	case errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing):
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
	case err != nil:
		response.Conflict(c, "备份链压缩失败，源链、对象或新基线校验未通过")
	default:
		response.Success(c, job)
	}
}

// @Summary Retry
// @Description Handles POST /api/fileshare/v1/management/backups/{id}/retry.
// @Tags Backup and restore
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
// @Router /management/backups/{id}/retry [post]
func (h *BackupHandler) Retry(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		response.BadRequest(c, "备份任务 ID 不能为空")
		return
	}
	job, err := h.service.Retry(c.Request.Context(), actor.WorkspaceID, actor.UserID, id)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "备份任务不存在")
	case errors.Is(err, backupservice.ErrBackupNotRetryable):
		response.Conflict(c, "只有失败的基线或增量备份任务可以重试")
	case errors.Is(err, backupservice.ErrBackupBaselineMissing):
		response.Conflict(c, "重试增量备份前必须先完成一次基线备份")
	case errors.Is(err, backupservice.ErrBackupInProgress):
		response.Conflict(c, "当前工作空间已有备份任务正在执行")
	case errors.Is(err, backupservice.ErrBackupBackendUnsupported):
		response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
	case errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing):
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
	case err != nil:
		response.InternalError(c, "重试备份任务失败", err)
	default:
		response.Success(c, job)
	}
}

// @Summary Restore Drill
// @Description Handles POST /api/fileshare/v1/management/backups/{id}/restore-drill.
// @Tags Backup and restore
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
// @Router /management/backups/{id}/restore-drill [post]
func (h *BackupHandler) RestoreDrill(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Confirm bool `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "恢复演练必须明确确认")
		return
	}
	drill, err := h.service.RunRestoreDrill(c.Request.Context(), actor.WorkspaceID, actor.UserID, c.Param("id"))
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		response.NotFound(c, "备份任务不存在")
	case errors.Is(err, backupservice.ErrRestoreDrillInProgress):
		response.Conflict(c, "当前工作空间已有恢复演练正在执行")
	case errors.Is(err, backupservice.ErrBackupBackendUnsupported):
		response.ServiceUnavailable(c, "当前备份后端尚未配置或不受支持")
	case errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing):
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
	case err != nil:
		response.Conflict(c, "恢复演练失败，备份链、对象或恢复写入校验未通过")
	default:
		response.Success(c, drill)
	}
}

// @Summary Verify
// @Description Handles POST /api/fileshare/v1/management/backups/{id}/verify.
// @Tags Backup and restore
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
// @Router /management/backups/{id}/verify [post]
func (h *BackupHandler) Verify(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id := c.Param("id")
	if id == "" {
		response.BadRequest(c, "备份任务 ID 不能为空")
		return
	}
	manifest, err := h.service.Verify(c.Request.Context(), actor.WorkspaceID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "备份任务不存在")
		return
	}
	if errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing) {
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
		return
	}
	if err != nil {
		response.Conflict(c, "备份链路校验失败")
		return
	}
	response.Success(c, gin.H{"valid": true, "job_id": id, "kind": manifest.Kind, "parent_id": manifest.ParentID, "change_log_start": manifest.ChangeLogStart, "change_log_end": manifest.ChangeLogEnd, "object_count": manifest.ObjectCount, "total_bytes": manifest.TotalBytes, "manifest_hash": manifest.ManifestHash})
}

// @Summary Detail
// @Description Handles GET /api/fileshare/v1/management/backups/{id}.
// @Tags Backup and restore
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/backups/{id} [get]
func (h *BackupHandler) Detail(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id := c.Param("id")
	manifest, err := h.service.Verify(c.Request.Context(), actor.WorkspaceID, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "备份任务不存在")
		return
	}
	if errors.Is(err, backupservice.ErrManifestEncryptionKeyMissing) {
		response.ServiceUnavailable(c, "备份清单加密密钥尚未配置")
		return
	}
	if err != nil {
		response.Conflict(c, "备份链路校验失败")
		return
	}
	type backupObject struct {
		VersionID  uint   `json:"version_id"`
		Name       string `json:"name"`
		Size       int64  `json:"size"`
		SHA256     string `json:"sha256"`
		Extension  string `json:"extension,omitempty"`
		Mime       string `json:"detected_mime,omitempty"`
		ScanStatus string `json:"scan_status"`
		Encrypted  bool   `json:"encrypted"`
	}
	objects := make([]backupObject, 0, len(manifest.Objects))
	for _, object := range manifest.Objects {
		objects = append(objects, backupObject{VersionID: object.VersionID, Name: object.Name, Size: object.Size, SHA256: object.SHA256, Extension: object.Extension, Mime: object.Mime, ScanStatus: object.ScanStatus, Encrypted: object.Encrypted})
	}
	metadataAvailable := manifest.Metadata != nil
	metadataNodes := 0
	metadataVersions := 0
	if metadataAvailable {
		metadataNodes = len(manifest.Metadata.Nodes)
		metadataVersions = len(manifest.Metadata.Versions)
	}
	response.Success(c, gin.H{"id": manifest.ID, "kind": manifest.Kind, "trigger": manifest.Trigger, "compacted_from_id": manifest.CompactedFromID, "parent_id": manifest.ParentID, "change_log_start": manifest.ChangeLogStart, "change_log_end": manifest.ChangeLogEnd, "change_count": len(manifest.Changes), "object_count": manifest.ObjectCount, "total_bytes": manifest.TotalBytes, "metadata_available": metadataAvailable, "metadata_node_count": metadataNodes, "metadata_version_count": metadataVersions, "objects": objects})
}
