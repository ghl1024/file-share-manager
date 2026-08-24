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

import "testing"

func TestBuildMenuFromRequestRejectsBlankAndPlaceholderNames(t *testing.T) {
	for _, name := range []string{"", "   ", "...", "…"} {
		_, ok := buildMenuFromRequest(menuRequest{Code: "custom-menu", Name: name, Type: 1}, true)
		if ok {
			t.Fatalf("buildMenuFromRequest accepted invalid name %q", name)
		}
	}
}

func TestBuildMenuFromRequestTrimsValidName(t *testing.T) {
	menu, ok := buildMenuFromRequest(menuRequest{Code: "custom-menu", Name: "  自定义菜单  ", Type: 1}, true)
	if !ok {
		t.Fatal("buildMenuFromRequest rejected valid name")
	}
	if menu.Name != "自定义菜单" {
		t.Fatalf("menu name = %q, want trimmed value", menu.Name)
	}
}
