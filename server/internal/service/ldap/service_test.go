/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldap

import "testing"

func TestBuildUserFilterRestrictsAndEscapesUsername(t *testing.T) {
	got := buildUserFilter("(&(objectClass=user)(sAMAccountName=*))", "sAMAccountName", "admin*)(uid=*)")
	want := "(&(&(objectClass=user)(sAMAccountName=*))(sAMAccountName=admin\\2a\\29\\28uid=\\2a\\29))"
	if got != want {
		t.Fatalf("filter = %q, want %q", got, want)
	}
}
