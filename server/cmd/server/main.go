/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

// @title File Share Manager API
// @version 1.0
// @description File Share Manager workspace file sharing and governance API.
// @termsOfService https://github.com/ghl1024/file-share-manager
// @contact.name HaydenGuo
// @contact.url https://hayden.pub
// @license.name Apache 2.0
// @license.url https://www.apache.org/licenses/LICENSE-2.0
// @BasePath /api/fileshare/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Enter "Bearer {token}". Browser sessions may also use the HttpOnly fileshare_session cookie.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/middleware"
	"file-share-manager/server/internal/migration"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/security"
	"file-share-manager/server/internal/router"
	"file-share-manager/server/internal/service/archive"
	"file-share-manager/server/internal/service/auditarchive"
	"file-share-manager/server/internal/service/auditexport"
	"file-share-manager/server/internal/service/backup"
	"file-share-manager/server/internal/service/batchdownload"
	"file-share-manager/server/internal/service/clamav"
	ldapservice "file-share-manager/server/internal/service/ldap"
	"file-share-manager/server/internal/service/ldapsync"
	"file-share-manager/server/internal/service/lifecycle"
	"file-share-manager/server/internal/service/notification"
	"file-share-manager/server/internal/service/storagehealth"

	"github.com/gin-gonic/gin"
)

func main() {
	environment := flag.String("env", "dev", "runtime environment: dev or prod")
	explicitConfig := flag.String("config", "", "explicit config file path")
	flag.Parse()

	configPath := *explicitConfig
	if configPath == "" {
		configPath = fmt.Sprintf("configs/config-%s.toml", *environment)
	}
	if err := config.LoadConfig(configPath); err != nil {
		log.Fatalf("load config %s: %v", configPath, err)
	}
	cfg := config.GetConfig()

	if err := logger.Init(logger.Config{
		Level: cfg.Log.Level, Directory: cfg.Log.Directory, Filename: cfg.Log.Filename,
		Console: cfg.Log.Console, Format: cfg.Log.Format, SplitByLevel: cfg.Log.SplitByLevel,
		RetentionDays: cfg.Log.RetentionDays, RotationTime: cfg.Log.RotationTime,
	}); err != nil {
		log.Fatalf("initialize logger: %v", err)
	}
	defer logger.Sync()

	if err := database.InitDB(); err != nil {
		logger.Fatalf("initialize database: %v", err)
	}
	if cfg.Database.AutoMigrate {
		if err := autoMigrate(); err != nil {
			logger.Fatalf("database auto migration failed: %v", err)
		}
		if err := dao.NewRoleDAO().EnsurePermissionDefinitions(); err != nil {
			logger.Fatalf("seed permission definitions: %v", err)
		}
		if err := dao.NewMenuDAO().EnsureBuiltinMenus(); err != nil {
			logger.Fatalf("seed builtin menus: %v", err)
		}
	} else if err := migration.VerifyCurrent(database.DB); err != nil {
		logger.Fatalf("database schema is not ready for release %s: %v", migration.CurrentVersion, err)
	}
	if err := validateLDAPStartupConfig(); err != nil {
		logger.Fatalf("LDAP security configuration is invalid: %v", err)
	}
	if err := ensureBootstrapAdmin(); err != nil {
		logger.Fatalf("initialize bootstrap administrator: %v", err)
	}
	if err := database.InitAuditArchiveDB(); err != nil {
		logger.Fatalf("initialize audit archive database: %v", err)
	}
	if err := os.MkdirAll(cfg.Storage.RootPath, 0o750); err != nil {
		logger.Fatalf("create storage root: %v", err)
	}
	if err := os.MkdirAll(cfg.Storage.StagingPath, 0o750); err != nil {
		logger.Fatalf("create storage staging directory: %v", err)
	}

	if cfg.Server.Mode == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	engine := gin.New()
	if err := engine.SetTrustedProxies(cfg.Server.TrustedProxies); err != nil {
		logger.Fatalf("configure trusted proxies: %v", err)
	}
	engine.Use(middleware.GlobalRequestSizeLimitMiddleware(cfg.Server.MaxRequestBodyBytes, cfg.Server.MaxUploadBodyBytes))
	engine.Use(middleware.LoggerMiddleware())
	engine.Use(middleware.RecoveryMiddleware())
	engine.Use(middleware.SecurityHeadersMiddleware())
	engine.Use(middleware.CORSMiddleware())
	engine.Use(middleware.RateLimitMiddleware(100, time.Minute))
	registerHealthRoutes(engine)
	router.RegisterRoutesWithConfig(engine, cfg)

	address := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	server := &http.Server{
		Addr:              address,
		Handler:           engine,
		ReadHeaderTimeout: time.Duration(cfg.Server.ReadHeaderTimeoutSeconds) * time.Second,
		ReadTimeout:       time.Duration(cfg.Server.ReadTimeoutSeconds) * time.Second,
		WriteTimeout:      time.Duration(cfg.Server.WriteTimeoutSeconds) * time.Second,
		IdleTimeout:       time.Duration(cfg.Server.IdleTimeoutSeconds) * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Fatalf("listen on %s: %v", address, err)
	}
	printBanner(address, configPath)

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("server_listening", "address", address, "config", configPath)
		serverErrors <- server.Serve(listener)
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	batchdownload.DefaultService().Start(ctx)
	auditexport.DefaultService().Start(ctx)
	auditarchive.DefaultService().Start(ctx)
	lifecycle.NewService().Start(ctx)
	notification.NewService().Start(ctx)
	storagehealth.NewMonitor().Start(ctx)
	backup.NewService().Start(ctx)
	if err := clamav.StartRetryWorker(ctx); err != nil {
		logger.Error("clamav_retry_worker_start_failed", "error", err)
	}
	if err := archive.StartGlobal(ctx); err != nil {
		logger.Error("archive_worker_start_failed", "error", err)
	}
	if err := ldapsync.StartGlobal(ctx); err != nil {
		logger.Error("ldap_sync_scheduler_start_failed", "error", err)
	}
	select {
	case <-ctx.Done():
		logger.Info("shutdown_signal")
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server_stopped_unexpectedly", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeoutSeconds)*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server_shutdown_failed", "error", err)
	}
	if sqlDB, err := database.DB.DB(); err == nil {
		_ = sqlDB.Close()
	}
	if database.AuditArchiveDB != nil {
		if sqlDB, err := database.AuditArchiveDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}
	logger.Info("server_exited")
}

func validateLDAPStartupConfig() error {
	ldapConfig, err := dao.NewLDAPConfigDAO().Get()
	if err != nil {
		return err
	}
	if ldapConfig == nil || ldapConfig.Status != 1 {
		return nil
	}
	runtimeConfig := dao.LDAPRuntimeConfig(ldapConfig)
	if !runtimeConfig.Enabled() || strings.TrimSpace(runtimeConfig.AdminDN) == "" || strings.TrimSpace(runtimeConfig.Password) == "" {
		return errors.New("enabled LDAP requires host, base DN, administrator DN and encrypted password")
	}
	allowPlaintext := true
	if cfg := config.GetConfig(); cfg != nil {
		allowPlaintext = cfg.Server.Mode != "release"
	}
	return ldapservice.ValidateConfig(runtimeConfig, allowPlaintext)
}

func autoMigrate() error {
	return migration.Run(database.DB)
}

func ensureBootstrapAdmin() error {
	userDAO := dao.NewUserDAO()
	hasAdmin, err := userDAO.HasSuperAdmin()
	if err != nil {
		return err
	}
	if hasAdmin {
		return nil
	}
	password := os.Getenv("FILESHARE_BOOTSTRAP_ADMIN_PASSWORD")
	if password == "" {
		return errors.New("no active super administrator exists; set FILESHARE_BOOTSTRAP_ADMIN_PASSWORD for the first start")
	}
	if config.IsPlaceholderSecret(password) {
		return errors.New("bootstrap password must not use a known placeholder")
	}
	if err := security.ValidatePassword(password); err != nil {
		return fmt.Errorf("bootstrap password: %w", err)
	}
	hash, err := security.HashPassword(password)
	if err != nil {
		return err
	}
	created, err := userDAO.EnsureSuperAdmin("admin", hash)
	if err != nil {
		return err
	}
	if created {
		logger.Info("bootstrap_admin_created", "username", "admin")
	}
	return nil
}

func registerHealthRoutes(engine *gin.Engine) {
	engine.GET("/healthz", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})
	engine.GET("/readyz", func(c *gin.Context) {
		sqlDB, err := database.DB.DB()
		if err != nil {
			c.String(http.StatusServiceUnavailable, "Database Not Ready")
			return
		}
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := sqlDB.PingContext(ctx); err != nil {
			c.String(http.StatusServiceUnavailable, "Database Not Ready")
			return
		}
		cfg := config.GetConfig()
		if cfg != nil && cfg.Audit.ArchiveEnabled {
			if database.AuditArchiveDB == nil {
				c.String(http.StatusServiceUnavailable, "Audit Archive Database Not Ready")
				return
			}
			auditSQLDB, err := database.AuditArchiveDB.DB()
			if err != nil || auditSQLDB.PingContext(ctx) != nil {
				c.String(http.StatusServiceUnavailable, "Audit Archive Database Not Ready")
				return
			}
		}
		c.String(http.StatusOK, "Ready")
	})
}
