/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package security

import "testing"

func TestValidatePassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		valid    bool
	}{
		{name: "strong", password: "Correct-Horse-42", valid: true},
		{name: "too short", password: "Admin123!", valid: false},
		{name: "too few classes", password: "alllowercasepassword", valid: false},
		{name: "whitespace", password: "Strong Password42!", valid: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidatePassword(test.password)
			if test.valid && err != nil {
				t.Fatalf("expected password to be valid: %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected password to be rejected")
			}
		})
	}
}
