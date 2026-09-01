/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldapsync

import "testing"

func TestValidateSpec(t *testing.T) {
	for _, spec := range []string{"0 0 2 * * *", "0 2 * * *", "@daily"} {
		if err := ValidateSpec(spec); err != nil {
			t.Fatalf("ValidateSpec(%q) error = %v", spec, err)
		}
	}
	if err := ValidateSpec("not a cron"); err == nil {
		t.Fatal("ValidateSpec(invalid) = nil, want error")
	}
}
