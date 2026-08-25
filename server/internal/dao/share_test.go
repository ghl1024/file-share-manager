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

func TestShareItemSafeForExternalDownload(t *testing.T) {
	for _, test := range []struct {
		status string
		want   bool
	}{
		{status: "clean", want: true},
		{status: "unscanned"},
		{status: "pending_scan"},
		{status: "scan_error"},
		{status: "infected"},
		{status: ""},
	} {
		t.Run(test.status, func(t *testing.T) {
			if got := shareItemSafeForExternalDownload(model.ShareItem{ScanStatus: test.status}); got != test.want {
				t.Fatalf("shareItemSafeForExternalDownload(%q) = %v, want %v", test.status, got, test.want)
			}
		})
	}
}
