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
	"strings"

	ldap "github.com/go-ldap/ldap/v3"
)

// OrganizationalPath returns organizational units from the directory root to
// the group leaf. Invalid or non-hierarchical DNs intentionally return empty.
func OrganizationalPath(raw string) []string {
	parsed, err := ldap.ParseDN(strings.TrimSpace(raw))
	if err != nil || parsed == nil {
		return []string{}
	}

	path := make([]string, 0, len(parsed.RDNs))
	for index := len(parsed.RDNs) - 1; index >= 0; index-- {
		for _, attribute := range parsed.RDNs[index].Attributes {
			if !strings.EqualFold(attribute.Type, "ou") {
				continue
			}
			value := strings.TrimSpace(attribute.Value)
			if value != "" && (len(path) == 0 || path[len(path)-1] != value) {
				path = append(path, value)
			}
		}
	}
	return path
}
