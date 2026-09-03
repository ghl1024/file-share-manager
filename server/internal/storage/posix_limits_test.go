/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package storage

import "testing"

func TestSizeProbeLimitAvoidsInt64Overflow(t *testing.T) {
	if got := sizeProbeLimit(0); got != 1 {
		t.Fatalf("sizeProbeLimit(0) = %d, want 1", got)
	}
	if got := sizeProbeLimit(maxInt64 - 1); got != maxInt64 {
		t.Fatalf("sizeProbeLimit(maxInt64-1) = %d, want maxInt64", got)
	}
	if got := sizeProbeLimit(maxInt64); got != maxInt64 {
		t.Fatalf("sizeProbeLimit(maxInt64) = %d, want maxInt64", got)
	}
}
