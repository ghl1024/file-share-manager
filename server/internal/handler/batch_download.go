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
	"net/http"
	"path"
	"sort"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"
	"file-share-manager/server/internal/service/batchdownload"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var (
	errBatchDownloadEmpty     = errors.New("batch download contains no downloadable files")
	errBatchDownloadUnsafe    = errors.New("batch download contains an unsafe file")
	errBatchDownloadFileLimit = errors.New("batch download file limit exceeded")
	errBatchDownloadSizeLimit = errors.New("batch download size limit exceeded")
)

type BatchDownloadHandler struct {
	jobs    *dao.BatchDownloadDAO
	nodes   *dao.NodeDAO
	files   *dao.FileDAO
	authz   *authorization.Service
	service *batchdownload.Service
}

func NewBatchDownloadHandler() *BatchDownloadHandler {
	return &BatchDownloadHandler{
		jobs: dao.NewBatchDownloadDAO(), nodes: dao.NewNodeDAO(), files: dao.NewFileDAO(),
		authz: authorization.NewService(), service: batchdownload.DefaultService(),
	}
}

// @Summary Create
// @Description Handles POST /api/fileshare/v1/management/batch-downloads.
// @Tags Batch downloads
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
// @Router /management/batch-downloads [post]
func (h *BatchDownloadHandler) Create(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		NodeIDs []uint `json:"node_ids" binding:"required,min=1,max=200,dive,gt=0"`
		Name    string `json:"name" binding:"max=255"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	items, totalBytes, err := h.snapshot(c, actor, req.NodeIDs)
	if err != nil {
		switch {
		case errors.Is(err, authorization.ErrNodeNotFound):
			response.NotFound(c, "目录或文件不存在")
		case errors.Is(err, errBatchDownloadEmpty):
			response.BadRequest(c, "所选内容中没有可下载文件")
		case errors.Is(err, errBatchDownloadUnsafe):
			response.BadRequest(c, "所选内容包含当前不可下载的文件")
		case errors.Is(err, errBatchDownloadFileLimit):
			response.BadRequest(c, fmt.Sprintf("批量下载最多包含 %d 个文件", batchDownloadLimits().maxFiles))
		case errors.Is(err, errBatchDownloadSizeLimit):
			response.BadRequest(c, fmt.Sprintf("批量下载总大小不能超过 %d 字节", batchDownloadLimits().maxBytes))
		default:
			response.InternalError(c, "创建批量下载快照失败", err)
		}
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "批量下载-" + time.Now().Format("20060102-150405")
	}
	displayName, _, valid := normalizeNodeName(name)
	if !valid {
		response.BadRequest(c, "任务名称包含非法字符")
		return
	}
	if !strings.HasSuffix(strings.ToLower(displayName), ".zip") {
		displayName += ".zip"
	}
	job := &model.BatchDownloadJob{
		ID: uuid.NewString(), WorkspaceID: actor.WorkspaceID, CreatedBy: actor.UserID,
		Name: displayName, Status: "queued", TotalFiles: len(items), TotalBytes: totalBytes,
	}
	if err := h.jobs.Create(job, items); err != nil {
		response.InternalError(c, "创建批量下载任务失败", err)
		return
	}
	h.service.Enqueue(job.ID)
	response.Success(c, job)
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/batch-downloads.
// @Tags Batch downloads
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
// @Router /management/batch-downloads [get]
func (h *BatchDownloadHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	result, err := h.jobs.ListPage(actor.WorkspaceID, actor.UserID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取批量下载任务失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

// @Summary Get
// @Description Handles GET /api/fileshare/v1/management/batch-downloads/{id}.
// @Tags Batch downloads
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/batch-downloads/{id} [get]
func (h *BatchDownloadHandler) Get(c *gin.Context) {
	actor, job, ok := h.ownedJob(c)
	_ = actor
	if !ok {
		return
	}
	response.Success(c, job)
}

// @Summary Retry
// @Description Handles POST /api/fileshare/v1/management/batch-downloads/{id}/retry.
// @Tags Batch downloads
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
// @Router /management/batch-downloads/{id}/retry [post]
func (h *BatchDownloadHandler) Retry(c *gin.Context) {
	actor, job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	if job.Status != "failed" {
		response.Conflict(c, "只有失败任务可以重试")
		return
	}
	if err := h.service.RemoveArchive(job.ID); err != nil {
		response.InternalError(c, "清理旧任务文件失败", err)
		return
	}
	if err := h.jobs.Retry(actor.WorkspaceID, actor.UserID, job.ID); err != nil {
		if errors.Is(err, dao.ErrBatchDownloadState) {
			response.Conflict(c, "任务状态已变化，请刷新后重试")
			return
		}
		response.InternalError(c, "重试批量下载失败", err)
		return
	}
	h.service.Enqueue(job.ID)
	job.Status = "queued"
	job.ProcessedFiles = 0
	job.ProcessedBytes = 0
	job.ErrorMessage = ""
	job.StartedAt = nil
	job.CompletedAt = nil
	job.ExpiresAt = nil
	response.Success(c, job)
}

// @Summary Download
// @Description Handles GET /api/fileshare/v1/management/batch-downloads/{id}/download.
// @Tags Batch downloads
// @Accept json
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {file} binary
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/batch-downloads/{id}/download [get]
func (h *BatchDownloadHandler) Download(c *gin.Context) {
	_, job, ok := h.ownedJob(c)
	if !ok {
		return
	}
	if job.Status == "expired" || (job.ExpiresAt != nil && !job.ExpiresAt.After(time.Now())) {
		response.Gone(c, "批量下载文件已过期")
		return
	}
	if job.Status != "completed" {
		response.Conflict(c, "批量下载任务尚未完成")
		return
	}
	archive, err := h.service.OpenArchive(job.ID)
	if err != nil {
		response.InternalError(c, "批量下载文件不存在", err)
		return
	}
	defer archive.Close()
	filename := strings.ReplaceAll(strings.ReplaceAll(job.Name, "\r", "_"), "\n", "_")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; %s", contentDisposition(filename)))
	c.Header("Content-Type", "application/zip")
	http.ServeContent(c.Writer, c.Request, filename, job.CompletedAtOrCreatedAt(), archive)
}

func (h *BatchDownloadHandler) ownedJob(c *gin.Context) (authorization.Actor, *model.BatchDownloadJob, bool) {
	actor, ok := actorFromContext(c)
	if !ok {
		return authorization.Actor{}, nil, false
	}
	id := strings.TrimSpace(c.Param("id"))
	if _, err := uuid.Parse(id); err != nil {
		response.BadRequest(c, "任务 ID 格式错误")
		return actor, nil, false
	}
	job, err := h.jobs.GetForOwner(actor.WorkspaceID, actor.UserID, id)
	if err != nil {
		response.InternalError(c, "读取批量下载任务失败", err)
		return actor, nil, false
	}
	if job == nil {
		response.NotFound(c, "批量下载任务不存在")
		return actor, nil, false
	}
	return actor, job, true
}

type batchLimits struct {
	maxFiles int
	maxBytes int64
}

func batchDownloadLimits() batchLimits {
	limits := batchLimits{maxFiles: 1000, maxBytes: 5 << 30}
	if cfg := config.GetConfig(); cfg != nil {
		limits.maxFiles = cfg.BatchDownload.MaxFiles
		limits.maxBytes = cfg.BatchDownload.MaxTotalBytes
	}
	return limits
}

func (h *BatchDownloadHandler) snapshot(c *gin.Context, actor authorization.Actor, selectedIDs []uint) ([]model.BatchDownloadItem, int64, error) {
	seenSelected := make(map[uint]struct{}, len(selectedIDs))
	seenFiles := make(map[uint]struct{})
	usedPaths := make(map[string]struct{})
	items := make([]model.BatchDownloadItem, 0)
	var totalBytes int64
	limits := batchDownloadLimits()

	for _, selectedID := range selectedIDs {
		if _, exists := seenSelected[selectedID]; exists {
			continue
		}
		seenSelected[selectedID] = struct{}{}
		root, err := h.nodes.GetByID(actor.WorkspaceID, selectedID)
		if err != nil {
			return nil, 0, err
		}
		if root == nil || root.Status != "active" {
			return nil, 0, authorization.ErrNodeNotFound
		}
		allowed, err := h.authz.CanRead(actor, root.ID)
		if err != nil {
			return nil, 0, err
		}
		recordDataAuthorization(c, allowed, "node:read", root.Type, root.ID)
		if !allowed {
			return nil, 0, authorization.ErrNodeNotFound
		}

		nodes := []model.Node{*root}
		if root.Type == "folder" {
			descendants, err := h.nodes.ListAllDescendants(actor.WorkspaceID, root.ID)
			if err != nil {
				return nil, 0, err
			}
			sort.Slice(descendants, func(left, right int) bool { return descendants[left].ID < descendants[right].ID })
			nodes = append(nodes, descendants...)
		}
		nodeMap := make(map[uint]model.Node, len(nodes))
		for _, node := range nodes {
			if node.Status == "active" {
				nodeMap[node.ID] = node
			}
		}
		for _, node := range nodes {
			if node.Type != "file" || node.Status != "active" {
				continue
			}
			if _, exists := seenFiles[node.ID]; exists {
				continue
			}
			allowed, err := h.authz.CanRead(actor, node.ID)
			if err != nil {
				return nil, 0, err
			}
			if !allowed {
				continue
			}
			version, err := h.files.GetLatestVersion(actor.WorkspaceID, node.ID)
			if err != nil {
				return nil, 0, err
			}
			if version == nil {
				continue
			}
			if version.ScanStatus == "infected" || version.ScanStatus == "pending_scan" || version.ScanStatus == "scan_error" {
				return nil, 0, errBatchDownloadUnsafe
			}
			if storageRequiresRestore(version.StorageClass) {
				return nil, 0, errBatchDownloadUnsafe
			}
			if len(items)+1 > limits.maxFiles {
				return nil, 0, errBatchDownloadFileLimit
			}
			if version.Size > limits.maxBytes-totalBytes {
				return nil, 0, errBatchDownloadSizeLimit
			}
			relativePath := node.Name
			if root.Type == "folder" {
				relativePath = path.Join(root.Name, batchRelativePath(root, node, nodeMap))
			}
			relativePath = uniqueArchivePath(relativePath, usedPaths)
			items = append(items, model.BatchDownloadItem{
				NodeID: node.ID, VersionID: version.ID, RelativePath: relativePath,
				StorageKey: version.StorageKey, StorageClass: normalizedVersionStorageClass(version.StorageClass), Size: version.Size, SHA256: version.SHA256,
			})
			seenFiles[node.ID] = struct{}{}
			totalBytes += version.Size
		}
	}
	if len(items) == 0 {
		return nil, 0, errBatchDownloadEmpty
	}
	return items, totalBytes, nil
}

func batchRelativePath(root *model.Node, node model.Node, nodes map[uint]model.Node) string {
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

func uniqueArchivePath(candidate string, used map[string]struct{}) string {
	if _, exists := used[candidate]; !exists {
		used[candidate] = struct{}{}
		return candidate
	}
	extension := path.Ext(candidate)
	stem := strings.TrimSuffix(candidate, extension)
	for index := 2; ; index++ {
		value := fmt.Sprintf("%s (%d)%s", stem, index, extension)
		if _, exists := used[value]; !exists {
			used[value] = struct{}{}
			return value
		}
	}
}
