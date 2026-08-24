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
	"testing"
	"time"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
)

func TestMergeSharedGrantsGroupsSourcesByNode(t *testing.T) {
	now := time.Now()
	grants := []dao.DirectSharedGrant{
		{Node: model.Node{ID: 7, Name: "共享目录"}, SourceType: "user", SourceID: 3, SourceName: "测试用户", GrantedLevel: "read", SharedAt: now},
		{Node: model.Node{ID: 7, Name: "共享目录"}, SourceType: "group", SourceID: 5, SourceName: "研发部", SourceDirectory: "ldap", SourceLDAPDN: "CN=研发部,OU=技术中心,DC=example,DC=com", GrantedLevel: "read_write", SharedAt: now.Add(-time.Minute)},
	}
	items, nodeIDs := mergeSharedGrants(grants)
	if len(items) != 1 || len(nodeIDs) != 1 || nodeIDs[0] != 7 {
		t.Fatalf("mergeSharedGrants() items=%#v nodeIDs=%#v", items, nodeIDs)
	}
	if len(items[0].PermissionSources) != 2 || items[0].PermissionSources[1].Name != "研发部" {
		t.Fatalf("permission sources = %#v", items[0].PermissionSources)
	}
	if got := items[0].PermissionSources[1].DirectoryPath; len(got) != 1 || got[0] != "技术中心" {
		t.Fatalf("LDAP directory path = %#v", got)
	}
	if items[0].SharedAt != now {
		t.Fatalf("shared_at = %v, want latest %v", items[0].SharedAt, now)
	}
}

func TestFilterSharedNodesMatchesNodeAndSource(t *testing.T) {
	items := []sharedNodeResponse{
		{Node: model.Node{ID: 1, Name: "季度报告"}, PermissionSources: []permissionSourceResponse{{Name: "财务部"}}},
		{Node: model.Node{ID: 2, Name: "产品素材"}, PermissionSources: []permissionSourceResponse{{Name: "市场部"}}},
	}
	if got := filterSharedNodes(items, "季度"); len(got) != 1 || got[0].ID != 1 {
		t.Fatalf("node keyword result = %#v", got)
	}
	if got := filterSharedNodes(items, "市场"); len(got) != 1 || got[0].ID != 2 {
		t.Fatalf("source keyword result = %#v", got)
	}
}
