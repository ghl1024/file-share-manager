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

const crossWorkspaceAccessKey = "audit_cross_workspace_access"

type CrossWorkspaceAccess struct {
	SourceWorkspaceID *uint  `json:"source_workspace_id,omitempty"`
	TargetWorkspaceID uint   `json:"target_workspace_id"`
	Reason            string `json:"reason"`
}

func RecordCrossWorkspaceAccess(c *gin.Context, access CrossWorkspaceAccess) {
	c.Set(crossWorkspaceAccessKey, access)
}

func CrossWorkspaceAccessFrom(c *gin.Context) (CrossWorkspaceAccess, bool) {
	value, exists := c.Get(crossWorkspaceAccessKey)
	if !exists {
		return CrossWorkspaceAccess{}, false
	}
	access, valid := value.(CrossWorkspaceAccess)
	return access, valid && access.TargetWorkspaceID > 0 && access.Reason != ""
}
