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
	"strings"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/pagination"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/clamav"
	ldapservice "file-share-manager/server/internal/service/ldap"
	"file-share-manager/server/internal/service/ldapsync"
	"file-share-manager/server/internal/service/storagehealth"

	"github.com/gin-gonic/gin"
)

type SystemHandler struct {
	ldapConfigDAO *dao.LDAPConfigDAO
	ldap          *ldapservice.Service
	ldapSync      *ldapsync.Service
}

func NewSystemHandler() *SystemHandler {
	return &SystemHandler{ldapConfigDAO: dao.NewLDAPConfigDAO(), ldap: ldapservice.NewService(), ldapSync: ldapsync.NewService()}
}

// @Summary Config
// @Description Handles GET /api/fileshare/v1/management/system/configs.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/configs [get]
func (h *SystemHandler) Config(c *gin.Context) {
	cfg := config.GetConfig()
	if cfg == nil {
		response.ServiceUnavailable(c, "系统配置尚未加载")
		return
	}
	ldapConfig, err := h.ldapConfigDAO.Get()
	if err != nil {
		response.InternalError(c, "查询 LDAP 配置失败", err)
		return
	}
	if ldapConfig == nil {
		ldapConfig = dao.DefaultLDAPConfig()
	}
	clamavRetry, err := clamav.CurrentRetryStatus()
	if err != nil {
		response.InternalError(c, "查询病毒扫描重试队列失败", err)
		return
	}
	response.Success(c, gin.H{
		"ldap":         ldapConfigResponse(ldapConfig),
		"backup":       gin.H{"type": cfg.Backup.Type, "local_path": cfg.Backup.LocalPath, "endpoint": cfg.Backup.Endpoint, "bucket": cfg.Backup.Bucket, "region": cfg.Backup.Region, "prefix": cfg.Backup.Prefix, "configured": backupConfigured(cfg), "manifest_encryption_enabled": strings.TrimSpace(cfg.Backup.ManifestEncryptionKey) != "", "manifest_format": "gzip + AES-256-GCM", "compaction_enabled": cfg.Backup.CompactionEnabled, "compaction_interval_minutes": cfg.Backup.CompactionIntervalMin, "compaction_incremental_threshold": cfg.Backup.CompactionThreshold},
		"archive":      gin.H{"enabled": cfg.Archive.Enabled, "primary_mode": cfg.Storage.Mode, "after_days": cfg.Archive.AfterDays, "batch_size": cfg.Archive.BatchSize, "prefix": cfg.Archive.Prefix},
		"clamav":       gin.H{"enabled": cfg.ClamAV.Enabled(), "host": cfg.ClamAV.Host, "port": cfg.ClamAV.Port, "timeout_seconds": cfg.ClamAV.TimeoutSeconds, "virus_db_max_age_hours": cfg.ClamAV.VirusDBMaxAgeHours, "retry": clamavRetry},
		"notification": gin.H{"credential_encryption_configured": strings.TrimSpace(cfg.Notification.CredentialEncryptionKey) != "", "worker_count": cfg.Notification.WorkerCount, "poll_interval_seconds": cfg.Notification.PollIntervalSeconds, "max_attempts": cfg.Notification.MaxAttempts},
		"lifecycle":    gin.H{"quarantine_retention_days": cfg.Lifecycle.QuarantineRetentionDays, "reconcile_batch_size": cfg.Lifecycle.ReconcileBatchSize},
	})
}

// @Summary Get L D A P
// @Description Handles GET /api/fileshare/v1/management/system/ldap.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/ldap [get]
func (h *SystemHandler) GetLDAP(c *gin.Context) {
	cfg, err := h.ldapConfigDAO.Get()
	if err != nil {
		response.InternalError(c, "查询 LDAP 配置失败", err)
		return
	}
	if cfg == nil {
		cfg = dao.DefaultLDAPConfig()
	}
	response.Success(c, ldapConfigResponse(cfg))
}

// @Summary Save L D A P
// @Description Handles POST /api/fileshare/v1/management/system/ldap.
// @Tags System management
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
// @Router /management/system/ldap [post]
func (h *SystemHandler) SaveLDAP(c *gin.Context) {
	var req ldapConfigRequest
	if !request.BindJSON(c, &req) {
		return
	}
	existing, err := h.ldapConfigDAO.Get()
	if err != nil {
		response.InternalError(c, "查询 LDAP 配置失败", err)
		return
	}
	cfg := &model.LDAPConfig{
		Host:             req.Host,
		Port:             req.Port,
		AdminDN:          req.AdminDN,
		Password:         req.Password,
		BaseDN:           req.BaseDN,
		UserFilter:       req.UserFilter,
		UsernameAttr:     req.UsernameAttr,
		EmailAttr:        req.EmailAttr,
		RealNameAttr:     req.RealNameAttr,
		SyncCron:         req.SyncCron,
		SyncWorkspaceID:  req.SyncWorkspaceID,
		GroupSyncEnabled: req.GroupSyncEnabled,
		GroupBaseDN:      req.GroupBaseDN,
		GroupFilter:      req.GroupFilter,
		GroupNameAttr:    req.GroupNameAttr,
		GroupMemberAttr:  req.GroupMemberAttr,
		Status:           req.Status,
	}
	if strings.TrimSpace(cfg.Password) == "" && existing != nil {
		cfg.Password = existing.Password
	}
	if strings.TrimSpace(cfg.SyncCron) == "" && existing != nil {
		cfg.SyncCron = existing.SyncCron
	}
	if strings.TrimSpace(cfg.SyncCron) == "" {
		cfg.SyncCron = ldapsync.DefaultCron
	}
	if cfg.Status == 1 && strings.TrimSpace(cfg.Password) == "" {
		response.BadRequest(c, "启用 LDAP 时管理员密码不能为空")
		return
	}
	if err := ldapsync.ValidateSpec(cfg.SyncCron); err != nil {
		response.BadRequest(c, "同步 Cron 表达式无效")
		return
	}
	if err := validateLDAPConfig(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if err := validateLDAPGroupSyncConfig(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	userValue, _ := c.Get("user_id")
	userID, _ := userValue.(uint)
	if err := h.ldapConfigDAO.SaveWithAudit(cfg,
		newBusinessAuditEvent(c, userID, nil, "system:ldap_update", "ldap_config", "1", "LDAP 配置")); err != nil {
		response.InternalError(c, "保存 LDAP 配置失败", err)
		return
	}
	saved, err := h.ldapConfigDAO.Get()
	if err != nil {
		response.InternalError(c, "查询 LDAP 配置失败", err)
		return
	}
	if err := ldapsync.UpdateGlobal(saved); err != nil {
		response.InternalError(c, "更新 LDAP 同步计划失败", err)
		return
	}
	response.SuccessWithMessage(c, "保存成功", ldapConfigResponse(saved))
}

// @Summary Clam A V Health
// @Description Handles GET /api/fileshare/v1/management/system/clamav/health.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/clamav/health [get]
func (h *SystemHandler) ClamAVHealth(c *gin.Context) {
	health, err := clamav.CheckHealth(c.Request.Context())
	if err != nil {
		response.BadGateway(c, "ClamAV 健康检查失败")
		return
	}
	response.Success(c, health)
}

// @Summary L D A P Test
// @Description Handles POST /api/fileshare/v1/management/system/ldap/test.
// @Tags System management
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
// @Router /management/system/ldap/test [post]
func (h *SystemHandler) LDAPTest(c *gin.Context) {
	var req ldapConfigRequest
	if !request.BindJSON(c, &req) {
		return
	}
	existing, err := h.ldapConfigDAO.Get()
	if err != nil {
		response.InternalError(c, "查询 LDAP 配置失败", err)
		return
	}
	if strings.TrimSpace(req.Password) == "" && existing != nil {
		req.Password = existing.Password
	}
	cfg := &model.LDAPConfig{
		Host:             req.Host,
		Port:             req.Port,
		AdminDN:          req.AdminDN,
		Password:         req.Password,
		BaseDN:           req.BaseDN,
		UserFilter:       req.UserFilter,
		UsernameAttr:     req.UsernameAttr,
		EmailAttr:        req.EmailAttr,
		RealNameAttr:     req.RealNameAttr,
		SyncCron:         req.SyncCron,
		SyncWorkspaceID:  req.SyncWorkspaceID,
		GroupSyncEnabled: req.GroupSyncEnabled,
		GroupBaseDN:      req.GroupBaseDN,
		GroupFilter:      req.GroupFilter,
		GroupNameAttr:    req.GroupNameAttr,
		GroupMemberAttr:  req.GroupMemberAttr,
		Status:           1,
	}
	if cfg.Host == "" && existing != nil {
		cfg = existing
	}
	if err := validateLDAPConfig(cfg); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	if strings.TrimSpace(cfg.Password) == "" {
		response.BadRequest(c, "管理员密码不能为空")
		return
	}
	if err := h.ldap.TestConnection(c.Request.Context(), dao.LDAPRuntimeConfig(cfg)); err != nil {
		response.BadGateway(c, "LDAP 连接测试失败")
		return
	}
	response.Success(c, gin.H{"reachable": true})
}

// @Summary L D A P Manual Sync
// @Description Handles POST /api/fileshare/v1/management/system/ldap/sync.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param X-Requested-With header string false "Set to XMLHttpRequest when using the session cookie"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/ldap/sync [post]
func (h *SystemHandler) LDAPManualSync(c *gin.Context) {
	err := h.ldapSync.StartAsync("manual")
	if err == nil {
		response.SuccessWithMessage(c, "LDAP 同步任务已启动，请稍后查看同步记录", nil)
		return
	}
	if errors.Is(err, ldapsync.ErrAlreadyRunning) {
		response.Conflict(c, "已有 LDAP 同步任务正在执行，请稍后再试")
		return
	}
	if errors.Is(err, ldapsync.ErrNotEnabled) {
		response.BadRequest(c, "请先完成并启用 LDAP 配置")
		return
	}
	response.InternalError(c, "启动 LDAP 同步任务失败", err)
}

// @Summary L D A P Sync History
// @Description Handles GET /api/fileshare/v1/management/system/ldap/history.
// @Tags System management
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
// @Router /management/system/ldap/history [get]
func (h *SystemHandler) LDAPSyncHistory(c *gin.Context) {
	page, pageSize, _ := pagination.ParseGinContextWithOptions(c, pagination.Options{
		DefaultPage:     1,
		DefaultPageSize: 10,
		MaxPageSize:     100,
	})
	result, err := dao.NewLDAPSyncHistoryDAO().ListPage(page, pageSize)
	if err != nil {
		response.InternalError(c, "查询 LDAP 同步记录失败", err)
		return
	}
	response.SuccessWithPage(c, result.List, result.Total, result.Page, result.PageSize)
}

// @Summary Storage Health
// @Description Handles GET /api/fileshare/v1/management/system/storage/health.
// @Tags System management
// @Accept json
// @Produce json
// @Security BearerAuth
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 401 {object} response.Response
// @Failure 403 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /management/system/storage/health [get]
func (h *SystemHandler) StorageHealth(c *gin.Context) {
	cfg := config.GetConfig()
	if cfg == nil {
		response.ServiceUnavailable(c, "系统配置尚未加载")
		return
	}
	response.Success(c, storagehealth.Check(cfg))
}

func backupConfigured(cfg *config.Config) bool {
	if !strings.EqualFold(cfg.Backup.Type, "local") {
		return cfg.Backup.Endpoint != "" && cfg.Backup.Bucket != "" && cfg.Backup.Region != "" && cfg.Backup.AccessKey != "" && cfg.Backup.SecretKey != ""
	}
	return cfg.Backup.LocalPath != ""
}

type ldapConfigRequest struct {
	Host             string `json:"host" binding:"omitempty,max=255"`
	Port             int    `json:"port" binding:"omitempty,min=1,max=65535"`
	AdminDN          string `json:"admin_dn" binding:"omitempty,max=255"`
	Password         string `json:"password" binding:"omitempty,max=255"`
	BaseDN           string `json:"base_dn" binding:"omitempty,max=255"`
	UserFilter       string `json:"user_filter" binding:"omitempty,max=255"`
	UsernameAttr     string `json:"username_attr" binding:"omitempty,max=64"`
	EmailAttr        string `json:"email_attr" binding:"omitempty,max=64"`
	RealNameAttr     string `json:"real_name_attr" binding:"omitempty,max=64"`
	SyncCron         string `json:"sync_cron" binding:"omitempty,max=64"`
	SyncWorkspaceID  uint   `json:"sync_workspace_id" binding:"omitempty"`
	GroupSyncEnabled int    `json:"group_sync_enabled" binding:"omitempty,oneof=0 1"`
	GroupBaseDN      string `json:"group_base_dn" binding:"omitempty,max=255"`
	GroupFilter      string `json:"group_filter" binding:"omitempty,max=255"`
	GroupNameAttr    string `json:"group_name_attr" binding:"omitempty,max=64"`
	GroupMemberAttr  string `json:"group_member_attr" binding:"omitempty,max=64"`
	Status           int    `json:"status" binding:"omitempty,oneof=0 1"`
}

func ldapConfigResponse(cfg *model.LDAPConfig) gin.H {
	if cfg == nil {
		cfg = dao.DefaultLDAPConfig()
	}
	runtimeConfig := dao.LDAPRuntimeConfig(cfg)
	return gin.H{
		"id":                 cfg.ID,
		"host":               cfg.Host,
		"port":               cfg.Port,
		"admin_dn":           cfg.AdminDN,
		"base_dn":            cfg.BaseDN,
		"user_filter":        cfg.UserFilter,
		"username_attr":      cfg.UsernameAttr,
		"email_attr":         cfg.EmailAttr,
		"real_name_attr":     cfg.RealNameAttr,
		"realname_attr":      cfg.RealNameAttr,
		"sync_cron":          cfg.SyncCron,
		"sync_workspace_id":  cfg.SyncWorkspaceID,
		"group_sync_enabled": cfg.GroupSyncEnabled,
		"group_base_dn":      cfg.GroupBaseDN,
		"group_filter":       cfg.GroupFilter,
		"group_name_attr":    cfg.GroupNameAttr,
		"group_member_attr":  cfg.GroupMemberAttr,
		"status":             cfg.Status,
		"enabled":            cfg.Status == 1 && runtimeConfig.Enabled(),
		"created_at":         cfg.CreatedAt,
		"updated_at":         cfg.UpdatedAt,
	}
}

func validateLDAPConfig(cfg *model.LDAPConfig) error {
	normalized := dao.LDAPRuntimeConfig(cfg)
	if cfg.Status != 0 && cfg.Status != 1 {
		return errString("status 只能是 0 或 1")
	}
	if cfg.Status == 0 {
		return nil
	}
	if normalized.Port < 1 || normalized.Port > 65535 {
		return errString("LDAP 端口必须在 1 到 65535 之间")
	}
	if strings.TrimSpace(normalized.Host) == "" {
		return errString("LDAP 主机地址不能为空")
	}
	if strings.TrimSpace(normalized.BaseDN) == "" {
		return errString("Base DN 不能为空")
	}
	if strings.TrimSpace(normalized.AdminDN) == "" {
		return errString("管理员 DN 不能为空")
	}
	if strings.TrimSpace(normalized.UsernameAttr) == "" {
		return errString("用户名属性不能为空")
	}
	return nil
}

func validateLDAPGroupSyncConfig(cfg *model.LDAPConfig) error {
	normalized := dao.LDAPRuntimeConfig(cfg)
	if cfg.Status != 1 || cfg.GroupSyncEnabled != 1 {
		return nil
	}
	if cfg.SyncWorkspaceID == 0 {
		return errString("启用用户组同步时必须选择目标工作空间")
	}
	workspace, err := dao.NewWorkspaceDAO().GetByID(cfg.SyncWorkspaceID)
	if err != nil {
		return errString("查询目标工作空间失败")
	}
	if workspace == nil || workspace.Status != 1 {
		return errString("目标工作空间不存在或已禁用")
	}
	if strings.TrimSpace(normalized.GroupFilter) == "" {
		return errString("启用用户组同步时组过滤器不能为空")
	}
	if strings.TrimSpace(normalized.GroupNameAttr) == "" {
		return errString("启用用户组同步时组名属性不能为空")
	}
	if strings.TrimSpace(normalized.GroupMemberAttr) == "" {
		return errString("启用用户组同步时成员属性不能为空")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }
