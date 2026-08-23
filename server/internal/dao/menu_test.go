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
	"testing"

	"file-share-manager/server/internal/model"
)

func TestRepairBuiltinMenuNames(t *testing.T) {
	menus := []model.Menu{
		{Code: "files-upload", Name: "..."},
		{Code: "files-download", Name: " "},
		{Code: "files-delete", Name: "删除文件"},
		{Code: "custom-action", Name: ""},
	}

	repairBuiltinMenuNames(menus)

	if menus[0].Name != "上传文件" {
		t.Fatalf("placeholder builtin name = %q, want %q", menus[0].Name, "上传文件")
	}
	if menus[1].Name != "下载文件" {
		t.Fatalf("blank builtin name = %q, want %q", menus[1].Name, "下载文件")
	}
	if menus[2].Name != "删除文件" {
		t.Fatalf("valid builtin name changed to %q", menus[2].Name)
	}
	if menus[3].Name != "" {
		t.Fatalf("custom name unexpectedly changed to %q", menus[3].Name)
	}
}
