/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/middleware"
	"file-share-manager/server/internal/pkg/database"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestHealthRouteContractsMySQL(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run health route contract tests")
	}
	dsnConfig, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatal(err)
	}
	dsnConfig.ParseTime = true
	db, err := gorm.Open(mysql.Open(dsnConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB
	previousAuditDB := database.AuditArchiveDB
	previousConfig := config.GetConfig()
	database.DB = db
	database.AuditArchiveDB = nil
	config.SetTestConfig(&config.Config{})
	t.Cleanup(func() {
		database.DB = previousDB
		database.AuditArchiveDB = previousAuditDB
		config.SetTestConfig(previousConfig)
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(middleware.SecurityHeadersMiddleware())
	registerHealthRoutes(engine)
	assertPlainTextHealth(t, engine, "/healthz", http.StatusOK, "OK")
	assertPlainTextHealth(t, engine, "/readyz", http.StatusOK, "Ready")

	config.SetTestConfig(&config.Config{Audit: config.AuditConfig{ArchiveEnabled: true}})
	assertPlainTextHealth(t, engine, "/readyz", http.StatusServiceUnavailable, "Audit Archive Database Not Ready")
	database.AuditArchiveDB = db
	assertPlainTextHealth(t, engine, "/readyz", http.StatusOK, "Ready")
}

func assertPlainTextHealth(t *testing.T, handler http.Handler, path string, wantStatus int, wantBody string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != wantStatus || strings.TrimSpace(response.Body.String()) != wantBody {
		t.Fatalf("GET %s = %d %q, want %d %q", path, response.Code, response.Body.String(), wantStatus, wantBody)
	}
	if response.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("GET %s omitted security headers", path)
	}
}
