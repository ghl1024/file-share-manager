/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package dao

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/database"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestNodeSearchActiveMySQLNgramAndMetadataFilters(t *testing.T) {
	db := openNodeSearchTestDB(t)
	if err := db.AutoMigrate(&model.User{}, &model.Node{}, &model.NodeClosure{}, &model.FileVersion{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("ALTER TABLE nodes ADD FULLTEXT INDEX idx_nodes_fulltext_name (normalized_name) WITH PARSER ngram").Error; err != nil {
		t.Fatal(err)
	}
	creator := model.User{Username: "search_admin", RealName: "搜索管理员", PasswordHash: "not-used", Status: 1}
	if err := db.Create(&creator).Error; err != nil {
		t.Fatal(err)
	}
	nodes := []model.Node{
		{WorkspaceID: 1, Name: "季度回归报告.pdf", NormalizedName: "季度回归报告.pdf", Type: "file", Status: "active", InheritMode: "inherit", CreatedBy: creator.ID, UpdatedBy: creator.ID},
		{WorkspaceID: 1, Name: "回归资料", NormalizedName: "回归资料", Type: "folder", Status: "trashed", InheritMode: "inherit", CreatedBy: creator.ID, UpdatedBy: creator.ID},
		{WorkspaceID: 2, Name: "季度回归报告.pdf", NormalizedName: "季度回归报告.pdf", Type: "file", Status: "active", InheritMode: "inherit", CreatedBy: creator.ID, UpdatedBy: creator.ID},
	}
	if err := db.Create(&nodes).Error; err != nil {
		t.Fatal(err)
	}
	closures := []model.NodeClosure{
		{AncestorID: nodes[1].ID, DescendantID: nodes[1].ID, Depth: 0},
		{AncestorID: nodes[1].ID, DescendantID: nodes[0].ID, Depth: 1},
	}
	if err := db.Create(&closures).Error; err != nil {
		t.Fatal(err)
	}
	version := model.FileVersion{WorkspaceID: 1, NodeID: nodes[0].ID, VersionNo: 1, StorageKey: "search/object", Size: 2048, SHA256: strings.Repeat("a", 64), Extension: ".pdf", CreatedBy: creator.ID}
	if err := db.Create(&version).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&model.Node{}).Where("id = ?", nodes[0].ID).Update("active_version", version.ID).Error; err != nil {
		t.Fatal(err)
	}

	previousDB := database.DB
	database.DB = db
	t.Cleanup(func() { database.DB = previousDB })
	searchDAO := NewNodeDAO()
	results, err := searchDAO.SearchActive(1, NodeSearchFilter{Keyword: "回归", Sort: "relevance"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != nodes[0].ID {
		t.Fatalf("workspace/status filtered ngram results = %#v", results)
	}
	if results[0].SearchSize == nil || *results[0].SearchSize != 2048 || results[0].SearchExtension != ".pdf" || results[0].SearchCreatedBy != "搜索管理员" {
		t.Fatalf("search metadata was not projected: %#v", results[0])
	}
	results, err = searchDAO.SearchActive(1, NodeSearchFilter{Extension: ".pdf", CreatedBy: "search", MinSize: int64Pointer(1024), MaxSize: int64Pointer(4096), Sort: "size_desc"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].ID != nodes[0].ID {
		t.Fatalf("metadata filtered results = %#v", results)
	}
	descendants, err := searchDAO.ListAllDescendants(1, nodes[1].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(descendants) != 1 || descendants[0].ID != nodes[0].ID {
		t.Fatalf("descendant projection results = %#v", descendants)
	}
}

func int64Pointer(value int64) *int64 { return &value }

func openNodeSearchTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("FILESHARE_TEST_MYSQL_ADMIN_DSN"))
	if dsn == "" {
		t.Skip("set FILESHARE_TEST_MYSQL_ADMIN_DSN to run node search integration tests")
	}
	adminConfig, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse test MySQL DSN: %v", err)
	}
	adminConfig.ParseTime = true
	adminConfig.DBName = ""
	adminDB, err := gorm.Open(mysql.Open(adminConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect test MySQL: %v", err)
	}
	databaseName := "fs_search_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:12]
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci", databaseName)).Error; err != nil {
		t.Fatalf("create temporary search database: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", databaseName)).Error
		if sqlDB, sqlErr := adminDB.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	testConfig := *adminConfig
	testConfig.DBName = databaseName
	db, err := gorm.Open(mysql.Open(testConfig.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("connect temporary search database: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, sqlErr := db.DB(); sqlErr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
