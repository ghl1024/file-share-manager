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
	"errors"
	"testing"
)

func TestValidateWorkspaceRoleTransition(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		next       string
		adminCount int64
		wantErr    error
	}{
		{name: "member can remain member", current: "member", next: "member", adminCount: 1},
		{name: "member can become admin", current: "member", next: "workspace_admin", adminCount: 1},
		{name: "admin can be downgraded when another admin exists", current: "workspace_admin", next: "member", adminCount: 2},
		{name: "last admin cannot be downgraded", current: "workspace_admin", next: "member", adminCount: 1, wantErr: ErrLastWorkspaceAdmin},
		{name: "invalid role rejected", current: "member", next: "owner", adminCount: 1, wantErr: ErrInvalidWorkspaceRole},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateWorkspaceRoleTransition(test.current, test.next, test.adminCount)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
