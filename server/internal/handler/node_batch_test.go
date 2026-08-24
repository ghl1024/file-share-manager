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

func TestUniqueNodeIDsPreservesOrder(t *testing.T) {
	result := uniqueNodeIDs([]uint{7, 2, 7, 3, 2})
	want := []uint{7, 2, 3}
	if len(result) != len(want) {
		t.Fatalf("uniqueNodeIDs() = %v", result)
	}
	for index := range want {
		if result[index] != want[index] {
			t.Fatalf("uniqueNodeIDs() = %v", result)
		}
	}
}
