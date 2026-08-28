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
	"flag"
	"log"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/database"
)

func main() {
	configPath := flag.String("config", "configs/config-dev.toml", "configuration file path")
	workspaceID := flag.Uint("workspace", 0, "workspace whose audit chain will be rebuilt")
	confirm := flag.Bool("confirm", false, "confirm the maintenance operation")
	flag.Parse()

	if *workspaceID == 0 {
		log.Fatal("workspace must be greater than zero")
	}
	if !*confirm {
		log.Fatal("refusing to rebuild audit hashes without --confirm")
	}
	if err := config.LoadConfig(*configPath); err != nil {
		log.Fatalf("load config: %v", err)
	}
	if config.GetConfig().Server.Mode == "prod" {
		log.Fatal("audit chain repair is disabled in production mode")
	}
	if err := database.InitDB(); err != nil {
		log.Fatalf("initialize database: %v", err)
	}
	defer func() {
		if sqlDB, err := database.DB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	count, err := dao.NewOperationLogDAO().RebuildChain(uint(*workspaceID))
	if err != nil {
		log.Fatalf("rebuild audit chain: %v", err)
	}
	log.Printf("rebuilt audit chain for workspace %d (%d events)", *workspaceID, count)
}
