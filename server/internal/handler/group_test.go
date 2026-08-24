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
	"reflect"
	"testing"

	"file-share-manager/server/internal/model"
)

func TestGroupDirectoryItemsHidesRawDNAndBuildsPath(t *testing.T) {
	items := groupDirectoryItems([]model.UserGroup{
		{ID: 1, Name: "研发平台", Source: "ldap", LDAPDN: "CN=研发平台,OU=平台部,OU=研发中心,DC=example,DC=com"},
		{ID: 2, Name: "项目成员", Source: "local"},
	})
	if len(items) != 2 {
		t.Fatalf("groupDirectoryItems() len = %d, want 2", len(items))
	}
	if got, want := items[0].DirectoryPath, []string{"研发中心", "平台部"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LDAP directory path = %#v, want %#v", got, want)
	}
	if items[1].DirectoryPath == nil || len(items[1].DirectoryPath) != 0 {
		t.Fatalf("local directory path = %#v, want empty non-nil slice", items[1].DirectoryPath)
	}
}
