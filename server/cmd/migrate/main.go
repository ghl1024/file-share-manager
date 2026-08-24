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
	"encoding/json"
	"flag"
	"log"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/migration"
	"file-share-manager/server/internal/pkg/database"
)

func main() {
	configPath := flag.String("config", "configs/config-dev.toml", "configuration file path")
	verifyOnly := flag.Bool("verify", false, "verify that the database is at the schema version required by this release")
	flag.Parse()
	if err := config.LoadConfig(*configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}
	if err := database.InitDB(); err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() {
		if sqlDB, err := database.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()
	if *verifyOnly {
		if err := migration.VerifyCurrent(database.DB); err != nil {
			log.Fatalf("verify schema: %v", err)
		}
		log.Printf("schema verification completed: version=%s", migration.CurrentVersion)
		return
	}
	report, err := migration.RunVersioned(database.DB)
	if err != nil {
		log.Fatalf("migrate schema: %v", err)
	}
	if err := dao.NewRoleDAO().EnsurePermissionDefinitions(); err != nil {
		log.Fatalf("seed permission definitions: %v", err)
	}
	if err := dao.NewMenuDAO().EnsureBuiltinMenus(); err != nil {
		log.Fatalf("seed builtin menus: %v", err)
	}
	encoded, _ := json.Marshal(report)
	log.Printf("migration completed: %s", encoded)
}
