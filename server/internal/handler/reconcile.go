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
	"strings"

	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/reconcile"

	"github.com/gin-gonic/gin"
)

type ReconcileHandler struct{}

func NewReconcileHandler() *ReconcileHandler { return &ReconcileHandler{} }

// @Summary Scan
// @Description Handles GET /api/fileshare/v1/management/storage/reconcile.
// @Tags Storage governance
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/storage/reconcile [get]
func (h *ReconcileHandler) Scan(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	report, err := reconcile.ScanWorkspace(c.Request.Context(), actor.WorkspaceID)
	if err != nil {
		response.InternalError(c, "扫描存储对象失败", err)
		return
	}
	response.Success(c, report)
}

type quarantineOrphansRequest struct {
	StorageKeys []string `json:"storage_keys" binding:"required,min=1,max=200,dive,required,max=512"`
	Confirm     string   `json:"confirm" binding:"required"`
}

// @Summary Quarantine
// @Description Handles POST /api/fileshare/v1/management/storage/reconcile/quarantine.
// @Tags Storage governance
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
// @Router /management/storage/reconcile/quarantine [post]
func (h *ReconcileHandler) Quarantine(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req quarantineOrphansRequest
	if !request.BindJSON(c, &req) {
		return
	}
	if strings.TrimSpace(req.Confirm) != "QUARANTINE" {
		response.BadRequest(c, "请输入确认词 QUARANTINE")
		return
	}
	report, err := reconcile.QuarantineWorkspaceOrphans(c.Request.Context(), actor.WorkspaceID, actor.UserID, req.StorageKeys)
	if err != nil {
		response.InternalError(c, "隔离孤儿对象失败", err)
		return
	}
	response.SuccessWithMessage(c, "孤儿对象隔离完成", report)
}
