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
	"fmt"
	"runtime"
	"strings"
	"time"
)

var (
	Version   = "v0.0.0"
	GitCommit = "unknown"
	BuildTime = "unknown"
	GoVersion = "unknown"
)

func printBanner(address, configPath string) {
	version := normalizeVersion(Version)
	goVersion := normalizeGoVersion(GoVersion)
	buildTime := normalizeBannerValue(BuildTime, time.Now().Format("2006-01-02 15:04:05"))
	commit := normalizeBannerValue(GitCommit, "none")

	banner := `
**************************************************************************
**************************************************************************
  _____ _ _      _____ _                    __  __
 |  ___(_) | ___/  ___| |__   __ _ _ __ ___|  \/  | __ _ _ __
 | |_  | | |/ _ \___ \| '_ \ / _' | '__/ _ \ |\/| |/ _' | '_ |
 |  _| | | |  __/___) | | | | (_| | | |  __/ |  | | (_| | | | |
 |_|   |_|_|\___|____/|_| |_|\__,_|_|  \___|_|  |_|\__,_|_| |_|

  File Share Manager - 开源工作空间文件共享服务
  Version:   %s
  Go:        %s
  Build:     %s
  Commit:    %s
  Author:    HaydenGuo
  Blog:      https://hayden.pub
  GitHub:    https://github.com/ghl1024/file-share-manager
  Gitee:     https://gitee.com/ghl1024/file-share-manager
  CNB:       https://cnb.cool/ghl1024/file-share-manager
  License:   Apache-2.0
  Listen:    %s
  Config:    %s
**************************************************************************
**************************************************************************
`
	fmt.Printf(banner, version, goVersion, buildTime, commit, address, configPath)
}

func normalizeBannerValue(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || strings.EqualFold(trimmed, "unknown") {
		return fallback
	}
	return trimmed
}

func normalizeVersion(value string) string {
	version := normalizeBannerValue(value, "v0.0.0")
	if version == "none" {
		return "v0.0.0"
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func normalizeGoVersion(value string) string {
	goVersion := normalizeBannerValue(value, runtime.Version())
	if strings.HasPrefix(goVersion, "go") {
		return goVersion
	}
	return "go" + goVersion
}
