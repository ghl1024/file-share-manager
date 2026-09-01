/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

package auditcontext

import "github.com/gin-gonic/gin"

const authorizationChecksKey = "audit_authorization_checks"

type AuthorizationCheck struct {
	Decision   string `json:"decision"`
	Permission string `json:"permission"`
	Scope      string `json:"scope"`
	TargetType string `json:"target_type,omitempty"`
	TargetID   string `json:"target_id,omitempty"`
}

func RecordAuthorization(c *gin.Context, check AuthorizationCheck) {
	checks := AuthorizationChecks(c)
	checks = append(checks, check)
	c.Set(authorizationChecksKey, checks)
}

func AuthorizationChecks(c *gin.Context) []AuthorizationCheck {
	value, exists := c.Get(authorizationChecksKey)
	if !exists {
		return nil
	}
	checks, _ := value.([]AuthorizationCheck)
	return checks
}

func LastAuthorizationCheck(c *gin.Context) (AuthorizationCheck, bool) {
	checks := AuthorizationChecks(c)
	if len(checks) == 0 {
		return AuthorizationCheck{}, false
	}
	return checks[len(checks)-1], true
}

func LastAuthorizationTargetCheck(c *gin.Context) (AuthorizationCheck, bool) {
	checks := AuthorizationChecks(c)
	for index := len(checks) - 1; index >= 0; index-- {
		if checks[index].TargetType != "" || checks[index].TargetID != "" {
			return checks[index], true
		}
	}
	return AuthorizationCheck{}, false
}
