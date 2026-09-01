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
	"net/url"
	"strings"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/notification"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type NotificationHandler struct{ dao *dao.NotificationDAO }

func NewNotificationHandler() *NotificationHandler {
	return &NotificationHandler{dao: dao.NewNotificationDAO()}
}

type notificationChannelRequest struct {
	Name              string   `json:"name" binding:"required,max=80"`
	Type              string   `json:"type" binding:"required,oneof=smtp dingtalk feishu wecom"`
	WebhookURL        string   `json:"webhook_url" binding:"omitempty,max=2048"`
	Secret            string   `json:"secret" binding:"omitempty,max=512"`
	ClearSecret       bool     `json:"clear_secret"`
	SMTPHost          string   `json:"smtp_host" binding:"omitempty,max=255"`
	SMTPPort          int      `json:"smtp_port" binding:"omitempty,min=1,max=65535"`
	SMTPEncryption    string   `json:"smtp_encryption" binding:"omitempty,oneof=tls starttls"`
	SMTPUsername      string   `json:"smtp_username" binding:"omitempty,max=255"`
	SMTPPassword      string   `json:"smtp_password" binding:"omitempty,max=512"`
	ClearSMTPPassword bool     `json:"clear_smtp_password"`
	SMTPFrom          string   `json:"smtp_from" binding:"omitempty,max=320"`
	SMTPRecipients    []string `json:"smtp_recipients" binding:"omitempty,max=20,dive,max=320"`
	Status            int      `json:"status" binding:"oneof=0 1"`
	Remark            string   `json:"remark" binding:"omitempty,max=255"`
}

type notificationChannelResponse struct {
	ID                     uint      `json:"id"`
	Name                   string    `json:"name"`
	Type                   string    `json:"type"`
	EndpointSummary        string    `json:"endpoint_summary"`
	CredentialConfigured   bool      `json:"credential_configured"`
	SMTPHost               string    `json:"smtp_host,omitempty"`
	SMTPPort               int       `json:"smtp_port,omitempty"`
	SMTPEncryption         string    `json:"smtp_encryption,omitempty"`
	SMTPFrom               string    `json:"smtp_from,omitempty"`
	SMTPRecipients         []string  `json:"smtp_recipients,omitempty"`
	SMTPUsernameConfigured bool      `json:"smtp_username_configured"`
	Status                 int       `json:"status"`
	Remark                 string    `json:"remark"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

// @Summary List Channels
// @Description Handles GET /api/fileshare/v1/management/system/notifications.
// @Tags Notifications
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
// @Router /management/system/notifications [get]
func (h *NotificationHandler) ListChannels(c *gin.Context) {
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	result, err := h.dao.ListChannels(page, pageSize)
	if err != nil {
		response.InternalError(c, "查询通知渠道失败", err)
		return
	}
	items := make([]notificationChannelResponse, 0, len(result.List))
	for i := range result.List {
		item, err := notificationChannelDTO(&result.List[i])
		if err != nil {
			response.InternalError(c, "通知渠道凭据无法解密", err)
			return
		}
		items = append(items, item)
	}
	response.SuccessWithPage(c, items, result.Total, result.Page, result.PageSize)
}

// @Summary Create Channel
// @Description Handles POST /api/fileshare/v1/management/system/notifications.
// @Tags Notifications
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
// @Router /management/system/notifications [post]
func (h *NotificationHandler) CreateChannel(c *gin.Context) {
	var req notificationChannelRequest
	if !request.BindJSON(c, &req) {
		return
	}
	settings, err := notificationSettingsFromRequest(req, nil, "")
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := notification.ValidateSettings(req.Type, &settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ciphertext, err := notification.EncryptSettings(settings)
	if errors.Is(err, notification.ErrCredentialKeyMissing) {
		response.ServiceUnavailable(c, "请先配置通知凭据加密密钥")
		return
	}
	if err != nil {
		response.InternalError(c, "加密通知渠道凭据失败", err)
		return
	}
	channel := &model.NotificationChannel{Name: strings.TrimSpace(req.Name), Type: strings.TrimSpace(req.Type), ConfigCiphertext: ciphertext, Status: req.Status, Remark: strings.TrimSpace(req.Remark)}
	if err := h.dao.CreateChannel(channel); err != nil {
		response.InternalError(c, "创建通知渠道失败", err)
		return
	}
	item, err := notificationChannelDTO(channel)
	if err != nil {
		response.InternalError(c, "读取通知渠道失败", err)
		return
	}
	response.SuccessWithMessage(c, "创建成功", item)
}

// @Summary Update Channel
// @Description Handles PUT /api/fileshare/v1/management/system/notifications/{id}.
// @Tags Notifications
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
// @Router /management/system/notifications/{id} [put]
func (h *NotificationHandler) UpdateChannel(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, "通知渠道 ID 无效")
		return
	}
	var req notificationChannelRequest
	if !request.BindJSON(c, &req) {
		return
	}
	existing, err := h.dao.GetChannel(id)
	if err != nil {
		response.InternalError(c, "查询通知渠道失败", err)
		return
	}
	if existing == nil {
		response.NotFound(c, "通知渠道不存在")
		return
	}
	previous, err := notification.DecryptSettings(existing.ConfigCiphertext)
	if err != nil {
		response.InternalError(c, "通知渠道凭据无法解密", err)
		return
	}
	settings, err := notificationSettingsFromRequest(req, &previous, existing.Type)
	if err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := notification.ValidateSettings(req.Type, &settings); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	ciphertext, err := notification.EncryptSettings(settings)
	if err != nil {
		response.InternalError(c, "加密通知渠道凭据失败", err)
		return
	}
	existing.Name, existing.Type, existing.ConfigCiphertext = strings.TrimSpace(req.Name), strings.TrimSpace(req.Type), ciphertext
	existing.Status, existing.Remark = req.Status, strings.TrimSpace(req.Remark)
	if err := h.dao.UpdateChannel(existing); err != nil {
		response.InternalError(c, "更新通知渠道失败", err)
		return
	}
	response.SuccessWithMessage(c, "保存成功", nil)
}

// @Summary Delete Channel
// @Description Handles DELETE /api/fileshare/v1/management/system/notifications/{id}.
// @Tags Notifications
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
// @Router /management/system/notifications/{id} [delete]
func (h *NotificationHandler) DeleteChannel(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, "通知渠道 ID 无效")
		return
	}
	if err := h.dao.DeleteChannel(id); errors.Is(err, gorm.ErrRecordNotFound) {
		response.NotFound(c, "通知渠道不存在")
		return
	} else if err != nil {
		response.InternalError(c, "删除通知渠道失败", err)
		return
	}
	response.SuccessWithMessage(c, "删除成功", nil)
}

// @Summary Test Channel
// @Description Handles POST /api/fileshare/v1/management/system/notifications/{id}/test.
// @Tags Notifications
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
// @Router /management/system/notifications/{id}/test [post]
func (h *NotificationHandler) TestChannel(c *gin.Context) {
	id, err := request.ParseUintParam(c, "id")
	if err != nil {
		response.BadRequest(c, "通知渠道 ID 无效")
		return
	}
	channel, err := h.dao.GetChannel(id)
	if err != nil {
		response.InternalError(c, "查询通知渠道失败", err)
		return
	}
	if channel == nil {
		response.NotFound(c, "通知渠道不存在")
		return
	}
	if channel.Status != 1 {
		response.Conflict(c, "请先启用通知渠道")
		return
	}
	if _, err := notification.EnqueueTest(channel); err != nil {
		response.InternalError(c, "创建测试通知失败", err)
		return
	}
	response.SuccessWithMessage(c, "测试消息已加入发送队列", nil)
}

// @Summary List Outbox
// @Description Handles GET /api/fileshare/v1/management/system/notifications/outbox.
// @Tags Notifications
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param status query string false "status"
// @Param page query string false "page"
// @Param page_size query string false "page_size"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/notifications/outbox [get]
func (h *NotificationHandler) ListOutbox(c *gin.Context) {
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{DefaultPage: 1, DefaultPageSize: 20, MaxPageSize: 100})
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !map[string]bool{"pending": true, "sending": true, "sent": true, "failed": true, "exhausted": true, "cancelled": true}[status] {
		response.BadRequest(c, "通知状态无效")
		return
	}
	result, err := h.dao.ListOutbox(page, pageSize, status)
	if err != nil {
		response.InternalError(c, "查询通知记录失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

// @Summary Retry Outbox
// @Description Handles POST /api/fileshare/v1/management/system/notifications/outbox/{id}/retry.
// @Tags Notifications
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
// @Router /management/system/notifications/outbox/{id}/retry [post]
func (h *NotificationHandler) RetryOutbox(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" || len(id) > 64 {
		response.BadRequest(c, "通知任务 ID 无效")
		return
	}
	retried, err := h.dao.Retry(id, time.Now())
	if err != nil {
		response.InternalError(c, "重试通知任务失败", err)
		return
	}
	if !retried {
		response.Conflict(c, "只有失败或重试耗尽的通知可以重新入队")
		return
	}
	response.SuccessWithMessage(c, "通知已重新入队", nil)
}

func notificationSettingsFromRequest(req notificationChannelRequest, previous *notification.ChannelSettings, previousType string) (notification.ChannelSettings, error) {
	settings := notification.ChannelSettings{}
	if previous != nil && strings.EqualFold(strings.TrimSpace(previousType), strings.TrimSpace(req.Type)) {
		settings = *previous
	}
	if req.Type == "smtp" {
		if strings.TrimSpace(req.SMTPHost) != "" {
			settings.SMTPHost = req.SMTPHost
		}
		if req.SMTPPort > 0 {
			settings.SMTPPort = req.SMTPPort
		}
		if strings.TrimSpace(req.SMTPEncryption) != "" {
			settings.SMTPEncryption = req.SMTPEncryption
		}
		if strings.TrimSpace(req.SMTPUsername) != "" {
			settings.SMTPUsername = req.SMTPUsername
		}
		if req.ClearSMTPPassword {
			settings.SMTPPassword = ""
		} else if req.SMTPPassword != "" {
			settings.SMTPPassword = req.SMTPPassword
		}
		if strings.TrimSpace(req.SMTPFrom) != "" {
			settings.SMTPFrom = req.SMTPFrom
		}
		if len(req.SMTPRecipients) > 0 {
			settings.SMTPRecipients = req.SMTPRecipients
		}
		settings.WebhookURL, settings.Secret = "", ""
	} else {
		if strings.TrimSpace(req.WebhookURL) != "" {
			settings.WebhookURL = req.WebhookURL
		}
		if req.ClearSecret {
			settings.Secret = ""
		} else if req.Secret != "" {
			settings.Secret = req.Secret
		}
		settings.SMTPHost, settings.SMTPUsername, settings.SMTPPassword, settings.SMTPFrom = "", "", "", ""
		settings.SMTPPort, settings.SMTPRecipients, settings.SMTPEncryption = 0, nil, ""
	}
	if previous == nil && ((req.Type == "smtp" && settings.SMTPHost == "") || (req.Type != "smtp" && settings.WebhookURL == "")) {
		return settings, errors.New("请填写完整的通知渠道连接信息")
	}
	return settings, nil
}

func notificationChannelDTO(channel *model.NotificationChannel) (notificationChannelResponse, error) {
	settings, err := notification.DecryptSettings(channel.ConfigCiphertext)
	if err != nil {
		return notificationChannelResponse{}, err
	}
	item := notificationChannelResponse{
		ID: channel.ID, Name: channel.Name, Type: channel.Type, Status: channel.Status, Remark: channel.Remark,
		CreatedAt: channel.CreatedAt, UpdatedAt: channel.UpdatedAt,
	}
	if channel.Type == "smtp" {
		item.EndpointSummary = settings.SMTPHost + ":" + stringInt(settings.SMTPPort)
		item.CredentialConfigured = settings.SMTPUsername == "" || settings.SMTPPassword != ""
		item.SMTPHost, item.SMTPPort, item.SMTPEncryption = settings.SMTPHost, settings.SMTPPort, settings.SMTPEncryption
		item.SMTPFrom, item.SMTPRecipients = settings.SMTPFrom, settings.SMTPRecipients
		item.SMTPUsernameConfigured = settings.SMTPUsername != ""
	} else {
		item.CredentialConfigured = settings.WebhookURL != ""
		if parsed, parseErr := url.Parse(settings.WebhookURL); parseErr == nil {
			item.EndpointSummary = parsed.Scheme + "://" + parsed.Host + "/***"
		}
	}
	return item, nil
}

func stringInt(value int) string {
	if value == 0 {
		return "-"
	}
	const digits = "0123456789"
	buffer := make([]byte, 0, 5)
	for value > 0 {
		buffer = append(buffer, digits[value%10])
		value /= 10
	}
	for left, right := 0, len(buffer)-1; left < right; left, right = left+1, right-1 {
		buffer[left], buffer[right] = buffer[right], buffer[left]
	}
	return string(buffer)
}
