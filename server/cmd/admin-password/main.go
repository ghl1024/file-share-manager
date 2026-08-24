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
	"os"
	"strings"

	"file-share-manager/server/internal/config"
	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/pkg/database"
	"file-share-manager/server/internal/pkg/security"
)

func main() {
	configPath := flag.String("config", "configs/config-dev.toml", "configuration file path")
	username := flag.String("username", "admin", "super administrator username")
	flag.Parse()

	password := os.Getenv("FILESHARE_ADMIN_PASSWORD")
	if err := security.ValidatePassword(password); err != nil {
		log.Fatalf("invalid FILESHARE_ADMIN_PASSWORD: %v", err)
	}
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

	user, err := dao.NewUserDAO().GetByUsername(strings.TrimSpace(*username))
	if err != nil {
		log.Fatalf("load administrator: %v", err)
	}
	if user == nil || !user.IsSuperAdmin {
		log.Fatalf("active super administrator %q was not found", strings.TrimSpace(*username))
	}
	if err := dao.NewUserDAO().UpdatePassword(user.ID, password); err != nil {
		log.Fatalf("update administrator password: %v", err)
	}
	log.Printf("administrator password updated for %s; existing sessions were invalidated", user.Username)
}
