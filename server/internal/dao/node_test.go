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

func TestEscapeSearchLike(t *testing.T) {
	if got := escapeSearchLike(`50%_done\report`); got != `50\%\_done\\report` {
		t.Fatalf("escapeSearchLike() = %q", got)
	}
}
