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
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/auditarchive"

	"github.com/gin-gonic/gin"
)

type AuditArchiveHandler struct{ service *auditarchive.Service }

func NewAuditArchiveHandler() *AuditArchiveHandler {
	return &AuditArchiveHandler{service: auditarchive.DefaultService()}
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/audit/archives.
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
// @Router /management/audit/archives [get]
func (h *AuditArchiveHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 10, MaxPageSize: 100})
	archives, err := h.service.List(actor.WorkspaceID, page, pageSize)
	if err != nil {
		response.InternalError(c, "读取审计归档记录失败", err)
		return
	}
	response.SuccessWithPage(c, archives.List, archives.Total, archives.Page, archives.PageSize)
}

// @Summary Run
// @Description Handles POST /api/fileshare/v1/management/audit/archives/run.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/archives/run [post]
func (h *AuditArchiveHandler) Run(c *gin.Context) {
	if _, ok := actorFromContext(c); !ok {
		return
	}
	cfg := config.GetConfig()
	if cfg == nil || !cfg.Audit.ArchiveEnabled {
		response.Conflict(c, "审计归档尚未启用")
		return
	}
	if err := h.service.RunOnce(c.Request.Context(), time.Now()); err != nil {
		response.InternalError(c, "执行审计归档失败", err)
		return
	}
	response.SuccessWithMessage(c, "审计归档任务执行完成", nil)
}
