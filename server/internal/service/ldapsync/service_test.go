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

import (
	"testing"

	"file-share-manager/server/internal/model"
	ldapservice "file-share-manager/server/internal/service/ldap"
)

func TestSyncEnabled(t *testing.T) {
	tests := []struct {
		name string
		cfg  *model.LDAPConfig
		want bool
	}{
		{name: "nil", cfg: nil, want: false},
		{name: "disabled", cfg: &model.LDAPConfig{Status: 0, Host: "ldap.local", BaseDN: "dc=example,dc=com", AdminDN: "cn=admin", Password: "secret"}, want: false},
		{name: "missing password", cfg: &model.LDAPConfig{Status: 1, Host: "ldap.local", BaseDN: "dc=example,dc=com", AdminDN: "cn=admin"}, want: false},
		{name: "enabled", cfg: &model.LDAPConfig{Status: 1, Host: "ldap.local", BaseDN: "dc=example,dc=com", AdminDN: "cn=admin", Password: "secret"}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := syncEnabled(test.cfg); got != test.want {
				t.Fatalf("syncEnabled() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestResolveLDAPGroupMemberIDsMatchesDNAndUsername(t *testing.T) {
	alice := &model.User{ID: 11, Username: "alice", Source: "ldap"}
	bob := &model.User{ID: 12, Username: "bob", Source: "ldap"}

	got := resolveLDAPGroupMemberIDs(
		[]string{
			"CN=Alice,OU=Users,DC=example,DC=com",
			"bob",
			"cn=alice,ou=users,dc=example,dc=com",
			"unknown",
		},
		map[string]*model.User{
			normalizeLDAPKey("cn=alice,ou=users,dc=example,dc=com"): alice,
		},
		map[string]*model.User{
			normalizeLDAPKey("bob"): bob,
		},
	)

	if len(got) != 2 || got[0] != alice.ID || got[1] != bob.ID {
		t.Fatalf("member IDs = %#v, want [%d %d]", got, alice.ID, bob.ID)
	}
}

func TestRegisterSyncedLDAPUserIgnoresLocalUsers(t *testing.T) {
	byDN := map[string]*model.User{}
	byUsername := map[string]*model.User{}

	registerSyncedLDAPUser(ldapservice.Identity{DN: "cn=local,dc=example", Username: "local"}, &model.User{ID: 1, Username: "local", Source: "local"}, byDN, byUsername)
	if len(byDN) != 0 || len(byUsername) != 0 {
		t.Fatalf("local users must not be registered for LDAP group matching")
	}

	registerSyncedLDAPUser(ldapservice.Identity{DN: "CN=Alice,DC=example", Username: "alice"}, &model.User{ID: 2, Username: "alice", Source: "ldap"}, byDN, byUsername)
	if byDN["cn=alice,dc=example"].ID != 2 || byUsername["alice"].ID != 2 {
		t.Fatalf("ldap user was not registered by both DN and username")
	}
}
