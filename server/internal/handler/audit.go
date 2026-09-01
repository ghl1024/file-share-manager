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
	"strconv"
	"strings"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"

	"github.com/gin-gonic/gin"
)

type AuditHandler struct {
	logs *dao.OperationLogDAO
}

func NewAuditHandler() *AuditHandler {
	return &AuditHandler{logs: dao.NewOperationLogDAO()}
}

// @Summary List
// @Description Handles GET /api/fileshare/v1/management/audit/logs and GET /api/fileshare/v1/management/audit/events.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param action query string false "action"
// @Param actor_type query string false "actor_type"
// @Param category query string false "category"
// @Param from query string false "from"
// @Param ip query string false "ip"
// @Param method query string false "method"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Param request_id query string false "request_id"
// @Param result query string false "result"
// @Param severity query string false "severity"
// @Param status query string false "status"
// @Param target_id query string false "target_id"
// @Param target_type query string false "target_type"
// @Param to query string false "to"
// @Param username query string false "username"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/logs [get]
// @Router /management/audit/events [get]
func (h *AuditHandler) List(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 200})
	filters, ok := parseAuditFilters(c)
	if !ok {
		return
	}
	logs, err := h.logs.ListPageWithFilters(actor.WorkspaceID, page, pageSize, filters)
	if err != nil {
		response.InternalError(c, "读取审计日志失败", err)
		return
	}
	response.SuccessWithPage(c, logs.List, logs.Total, logs.Page, logs.PageSize)
}

// @Summary Detail
// @Description Handles GET /api/fileshare/v1/management/audit/events/{id}.
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
// @Router /management/audit/events/{id} [get]
func (h *AuditHandler) Detail(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	log, err := h.logs.GetByID(actor.WorkspaceID, id)
	if err != nil {
		response.InternalError(c, "读取审计详情失败", err)
		return
	}
	if log == nil {
		response.NotFound(c, "审计事件不存在")
		return
	}
	response.Success(c, log)
}

// @Summary Security Events
// @Description Handles GET /api/fileshare/v1/management/audit/security-events.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param action query string false "action"
// @Param actor_type query string false "actor_type"
// @Param from query string false "from"
// @Param ip query string false "ip"
// @Param method query string false "method"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Param request_id query string false "request_id"
// @Param result query string false "result"
// @Param severity query string false "severity"
// @Param status query string false "status"
// @Param target_id query string false "target_id"
// @Param target_type query string false "target_type"
// @Param to query string false "to"
// @Param username query string false "username"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/security-events [get]
func (h *AuditHandler) SecurityEvents(c *gin.Context) {
	query := c.Request.URL.Query()
	query.Set("category", "security")
	c.Request.URL.RawQuery = query.Encode()
	h.List(c)
}

// @Summary Policy
// @Description Handles GET /api/fileshare/v1/management/audit/policy.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/policy [get]
func (h *AuditHandler) Policy(c *gin.Context) {
	if _, ok := actorFromContext(c); !ok {
		return
	}
	cfg := config.GetConfig()
	if cfg == nil {
		response.ServiceUnavailable(c, "审计策略尚未加载")
		return
	}
	response.Success(c, gin.H{
		"hot_retention_days":              cfg.Audit.HotRetentionDays,
		"export_retention_hours":          cfg.Audit.ExportRetentionHours,
		"archive_enabled":                 cfg.Audit.ArchiveEnabled,
		"archive_interval_minutes":        cfg.Audit.ArchiveIntervalMinutes,
		"archive_batch_size":              cfg.Audit.ArchiveBatchSize,
		"hot_cleanup_enabled":             cfg.Audit.ArchiveEnabled,
		"archive_required_before_cleanup": true,
	})
}

// @Summary Verify Stream
// @Description Handles GET /api/fileshare/v1/management/audit/streams/{workspace_id}/verify.
// @Tags Audit
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param workspace_id path string true "workspace_id"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/audit/streams/{workspace_id}/verify [get]
func (h *AuditHandler) VerifyStream(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	workspaceID, err := request.ParseUintParam(c, "workspace_id")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if !actor.IsSuperAdmin && workspaceID != actor.WorkspaceID {
		response.Forbidden(c, "无权校验其他工作空间审计链")
		return
	}
	result, err := h.logs.VerifyChain(workspaceID)
	if err != nil {
		response.InternalError(c, "校验审计链失败", err)
		return
	}
	response.Success(c, result)
}

func parseAuditFilters(c *gin.Context) (dao.AuditFilters, bool) {
	filters := dao.AuditFilters{
		Username: c.Query("username"), Method: c.Query("method"), Action: c.Query("action"),
		Category: c.Query("category"), Severity: c.Query("severity"), Result: c.Query("result"),
		ActorType: c.Query("actor_type"), TargetType: c.Query("target_type"), TargetID: c.Query("target_id"),
		IP: c.Query("ip"), RequestID: c.Query("request_id"),
	}
	if !validOptionalAuditFilter(filters.Category, model.ValidAuditCategory) {
		response.BadRequest(c, "category 不是受支持的审计事件类型")
		return dao.AuditFilters{}, false
	}
	if !validOptionalAuditFilter(filters.Severity, model.ValidAuditSeverity) {
		response.BadRequest(c, "severity 不是受支持的风险级别")
		return dao.AuditFilters{}, false
	}
	if !validOptionalAuditFilter(filters.Result, model.ValidAuditResult) {
		response.BadRequest(c, "result 不是受支持的审计结果")
		return dao.AuditFilters{}, false
	}
	if !validOptionalAuditFilter(filters.ActorType, model.ValidAuditActorType) {
		response.BadRequest(c, "actor_type 不是受支持的主体类型")
		return dao.AuditFilters{}, false
	}
	if method := strings.ToUpper(strings.TrimSpace(filters.Method)); method != "" {
		switch method {
		case "GET", "POST", "PUT", "PATCH", "DELETE":
			filters.Method = method
		default:
			response.BadRequest(c, "method 不是受支持的请求方法")
			return dao.AuditFilters{}, false
		}
	}
	if raw := strings.TrimSpace(c.Query("status")); raw != "" {
		status, err := strconv.Atoi(raw)
		if err != nil || status < 100 || status > 599 {
			response.BadRequest(c, "status 必须是 100 到 599 之间的整数")
			return dao.AuditFilters{}, false
		}
		filters.Status = &status
	}
	from, ok := parseAuditTime(c, "from")
	if !ok {
		return dao.AuditFilters{}, false
	}
	to, ok := parseAuditTime(c, "to")
	if !ok {
		return dao.AuditFilters{}, false
	}
	filters.From, filters.To = from, to
	if filters.From != nil && filters.To != nil && !filters.From.Before(*filters.To) {
		response.BadRequest(c, "from 必须早于 to")
		return dao.AuditFilters{}, false
	}
	return filters, true
}

func validOptionalAuditFilter(value string, validate func(string) bool) bool {
	value = strings.TrimSpace(value)
	return value == "" || validate(value)
}

func parseAuditTime(c *gin.Context, name string) (*time.Time, bool) {
	raw := strings.TrimSpace(c.Query(name))
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		response.BadRequest(c, name+" 必须是 RFC3339 时间")
		return nil, false
	}
	return &value, true
}
