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

import "testing"

func TestIsManagedGroupSource(t *testing.T) {
	tests := []struct {
		source  string
		managed bool
	}{
		{source: "", managed: false},
		{source: "local", managed: false},
		{source: " LOCAL ", managed: false},
		{source: "ldap", managed: true},
		{source: "external-directory", managed: true},
	}
	for _, test := range tests {
		if got := IsManagedGroupSource(test.source); got != test.managed {
			t.Fatalf("IsManagedGroupSource(%q) = %v, want %v", test.source, got, test.managed)
		}
	}
}
