/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package ldapdn

import (
	"reflect"
	"testing"
)

func TestOrganizationalPath(t *testing.T) {
	tests := []struct {
		name string
		dn   string
		want []string
	}{
		{name: "nested organizational units", dn: "CN=Platform Team,OU=Platform,OU=Engineering,DC=example,DC=com", want: []string{"Engineering", "Platform"}},
		{name: "escaped value", dn: `CN=Approvers,OU=Research\, Development,DC=example,DC=com`, want: []string{"Research, Development"}},
		{name: "no organizational unit", dn: "CN=Users,DC=example,DC=com", want: []string{}},
		{name: "invalid", dn: "not-a-dn", want: []string{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := OrganizationalPath(test.dn); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("OrganizationalPath(%q) = %#v, want %#v", test.dn, got, test.want)
			}
		})
	}
}
