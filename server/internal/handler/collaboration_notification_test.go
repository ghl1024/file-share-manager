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

	"file-share-manager/server/internal/model"
)

func TestACLRecipientStatesPreferDirectUserEntries(t *testing.T) {
	handler := &ACLHandler{}
	states, err := handler.aclRecipientStates(1, []model.NodeACL{
		{SubjectType: "user", SubjectID: 7, Effect: "allow", AccessLevel: "read_write"},
		{SubjectType: "user", SubjectID: 8, Effect: "deny", AccessLevel: "read"},
	})
	if err != nil {
		t.Fatalf("aclRecipientStates() error = %v", err)
	}
	if states[7].Effect != "allow" || states[7].Level != "read_write" || states[7].Source != "个人授权" {
		t.Fatalf("user 7 state = %#v", states[7])
	}
	if states[8].Effect != "deny" {
		t.Fatalf("user 8 state = %#v", states[8])
	}
}
