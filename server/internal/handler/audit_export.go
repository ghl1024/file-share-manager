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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	auditexport "file-share-manager/server/internal/service/auditexport"

	"github.com/gin-gonic/gin"
)

type AuditExportHandler struct{ service *auditexport.Service }

func NewAuditExportHandler() *AuditExportHandler {
	return &AuditExportHandler{service: auditexport.DefaultService()}
}

// @Summary Create
// @Description Handles POST /api/fileshare/v1/management/audit/exports.
// @Tags Audit
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
// @Router /management/audit/exports [post]
func (h *AuditExportHandler) Create(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		Format string `json:"format"`
	}
	if c.Request.ContentLength != 0 {
		if !request.BindJSON(c, &req) {
			return
		}
	}
	format := strings.ToLower(strings.TrimSpace(req.Format))
	if format == "" {
		format = "csv"
	}
	if format != "csv" && format != "json" {
		response.BadRequest(c, "format 只能是 csv 或 json")
		return
	}
	filters, ok := parseAuditFilters(c)
	if !ok {
		return
	}
	job, err := h.service.Create(actor.WorkspaceID, actor.UserID, format, filters)
	if err != nil {
		response.InternalError(c, "创建审计导出任务失败", err)
		return
	}
	response.Success(c, job)
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/audit/exports.
// @Tags Audit
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
// @Router /management/audit/exports [get]
func (h *AuditExportHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	jobs, err := h.service.List(actor.WorkspaceID, actor.UserID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取审计导出任务失败", err)
		return
	}
	response.SuccessWithPage(c, jobs.List, jobs.Total, jobs.Page, jobs.PageSize)
}

// @Summary Get
// @Description Handles GET /api/fileshare/v1/management/audit/exports/{id}.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/exports/{id} [get]
func (h *AuditExportHandler) Get(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	job, err := h.service.Get(actor.WorkspaceID, actor.UserID, c.Param("id"))
	if err != nil {
		response.InternalError(c, "读取审计导出任务失败", err)
		return
	}
	if job == nil {
		response.NotFound(c, "审计导出任务不存在")
		return
	}
	response.Success(c, job)
}

// @Summary Download
// @Description Handles GET /api/fileshare/v1/management/audit/exports/{id}/download.
// @Tags Audit
// @Accept json
// @Produce application/octet-stream
// @Security BearerAuth
// @Param id path string true "id"
// @Success 200 {file} binary
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/exports/{id}/download [get]
func (h *AuditExportHandler) Download(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	job, err := h.service.Get(actor.WorkspaceID, actor.UserID, c.Param("id"))
	if err != nil {
		response.InternalError(c, "读取审计导出任务失败", err)
		return
	}
	if job == nil {
		response.NotFound(c, "审计导出任务不存在")
		return
	}
	if job.Status != "completed" || job.FilePath == "" {
		response.Conflict(c, "审计导出任务尚未完成")
		return
	}
	if job.ExpiresAt == nil || !job.ExpiresAt.After(time.Now()) {
		response.Gone(c, "审计导出文件已过期")
		return
	}
	if filepath.Base(job.FilePath) != job.ID+"."+job.Format {
		response.NotFound(c, "导出文件不存在")
		return
	}
	file, err := os.Open(job.FilePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.NotFound(c, "导出文件不存在")
			return
		}
		response.InternalError(c, "读取导出文件失败", err)
		return
	}
	defer file.Close()
	c.Header("Content-Type", map[string]string{"csv": "text/csv; charset=utf-8", "json": "application/json; charset=utf-8"}[job.Format])
	c.Header("Content-Disposition", "attachment; filename=\""+auditexport.DownloadName(job)+"\"")
	http.ServeContent(c.Writer, c.Request, auditexport.DownloadName(job), job.CreatedAt, file)
}
