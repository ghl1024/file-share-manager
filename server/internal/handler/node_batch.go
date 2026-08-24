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
	"errors"
	"strconv"

	"file-share-manager/server/internal/dao"
	"file-share-manager/server/internal/model"
	"file-share-manager/server/internal/pkg/logger"
	"file-share-manager/server/internal/pkg/request"
	"file-share-manager/server/internal/pkg/response"
	"file-share-manager/server/internal/service/authorization"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const maxBatchNodeOperations = 100

type batchNodeResult struct {
	NodeID  uint   `json:"node_id"`
	Name    string `json:"name,omitempty"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type batchNodeResponse struct {
	Results      []batchNodeResult `json:"results"`
	SuccessCount int               `json:"success_count"`
	FailedCount  int               `json:"failed_count"`
}

type batchNodeIDsRequest struct {
	NodeIDs []uint `json:"node_ids" binding:"required,min=1,max=100,dive,gt=0"`
}

func (h *NodeHandler) BatchMove(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		NodeIDs  []uint `json:"node_ids" binding:"required,min=1,max=100,dive,gt=0"`
		ParentID *uint  `json:"parent_id"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	req.NodeIDs = uniqueNodeIDs(req.NodeIDs)
	if !h.authorizeBatchMoveTarget(c, actor, req.ParentID) {
		return
	}
	results := make([]batchNodeResult, 0, len(req.NodeIDs))
	for _, nodeID := range req.NodeIDs {
		node, result := h.batchWritableNode(c, actor, nodeID, "active", false)
		if !result.Success {
			results = append(results, result)
			continue
		}
		exists, err := h.nodeDAO.NameExists(actor.WorkspaceID, req.ParentID, node.NormalizedName, &node.ID)
		if err != nil {
			results = append(results, failedBatchNode(node, "检查目标目录失败"))
			logBatchNodeFailure("move", actor.WorkspaceID, node.ID, err)
			continue
		}
		if exists {
			results = append(results, failedBatchNode(node, "目标目录中已存在同名项目"))
			continue
		}
		workspaceID := actor.WorkspaceID
		err = h.nodeDAO.MoveWithAudit(actor.WorkspaceID, node.ID, actor.UserID, req.ParentID,
			newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:move", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name))
		if err != nil {
			message := "移动失败"
			switch {
			case errors.Is(err, dao.ErrInvalidMove):
				message = "目录不能移动到自身或其后代目录"
			case errors.Is(err, dao.ErrNodeState), errors.Is(err, gorm.ErrRecordNotFound):
				message = "节点或目标目录状态已变化"
			default:
				logBatchNodeFailure("move", actor.WorkspaceID, node.ID, err)
			}
			results = append(results, failedBatchNode(node, message))
			continue
		}
		results = append(results, successfulBatchNode(node, "已移动"))
	}
	respondBatchNodes(c, results)
}

func (h *NodeHandler) BatchTrash(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		batchNodeIDsRequest
		Confirm bool `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "批量移入回收站必须明确确认")
		return
	}
	results := h.runBatchWrite(c, actor, uniqueNodeIDs(req.NodeIDs), "active", false, "trash", func(node *model.Node) error {
		workspaceID := actor.WorkspaceID
		return h.nodeDAO.TrashSubtreeWithAudit(actor.WorkspaceID, node.ID, actor.UserID,
			newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:trash", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name))
	})
	respondBatchNodes(c, results)
}

func (h *NodeHandler) BatchRestore(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		batchNodeIDsRequest
		Confirm bool `json:"confirm" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	if !req.Confirm {
		response.BadRequest(c, "批量恢复必须明确确认")
		return
	}
	results := h.runBatchWrite(c, actor, uniqueNodeIDs(req.NodeIDs), "trashed", true, "restore", func(node *model.Node) error {
		workspaceID := actor.WorkspaceID
		return h.nodeDAO.RestoreSubtreeWithAudit(actor.WorkspaceID, node.ID, actor.UserID,
			newBusinessAuditEvent(c, actor.UserID, &workspaceID, "node:restore", "node", strconv.FormatUint(uint64(node.ID), 10), node.Name))
	})
	respondBatchNodes(c, results)
}

func (h *NodeHandler) BatchFavorite(c *gin.Context) {
	actor, ok := actorFromContext(c)
	if !ok {
		return
	}
	var req struct {
		batchNodeIDsRequest
		Favorite *bool `json:"favorite" binding:"required"`
	}
	if !request.BindJSON(c, &req) {
		return
	}
	results := make([]batchNodeResult, 0, len(req.NodeIDs))
	for _, nodeID := range uniqueNodeIDs(req.NodeIDs) {
		node, result := h.batchReadableNode(c, actor, nodeID)
		if !result.Success {
			results = append(results, result)
			continue
		}
		if err := h.favoriteDAO.Set(actor.WorkspaceID, actor.UserID, node.ID, *req.Favorite); err != nil {
			results = append(results, failedBatchNode(node, "更新收藏失败"))
			logBatchNodeFailure("favorite", actor.WorkspaceID, node.ID, err)
			continue
		}
		message := "已收藏"
		if !*req.Favorite {
			message = "已取消收藏"
		}
		results = append(results, successfulBatchNode(node, message))
	}
	respondBatchNodes(c, results)
}

func (h *NodeHandler) authorizeBatchMoveTarget(c *gin.Context, actor authorization.Actor, parentID *uint) bool {
	if parentID == nil {
		allowed := actor.IsSuperAdmin || actor.WorkspaceRole == "workspace_admin"
		recordDataAuthorization(c, allowed, "node:move_to_root", "workspace", actor.WorkspaceID)
		if !allowed {
			response.Forbidden(c, "只有工作空间管理员可以移动到根目录")
		}
		return allowed
	}
	parent, err := h.nodeDAO.GetByID(actor.WorkspaceID, *parentID)
	if err != nil {
		response.InternalError(c, "读取目标目录失败", err)
		return false
	}
	if parent == nil || parent.Type != "folder" || parent.Status != "active" {
		response.NotFound(c, "目标目录不存在")
		return false
	}
	allowed, err := h.authz.CanWrite(actor, parent.ID)
	if err != nil {
		h.handleAuthorizationError(c, err)
		return false
	}
	recordDataAuthorization(c, allowed, "node:write", "folder", parent.ID)
	if !allowed {
		response.Forbidden(c, "无权移动到目标目录")
	}
	return allowed
}

func (h *NodeHandler) runBatchWrite(c *gin.Context, actor authorization.Actor, nodeIDs []uint, expectedStatus string, restore bool, action string, operation func(*model.Node) error) []batchNodeResult {
	results := make([]batchNodeResult, 0, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		node, result := h.batchWritableNode(c, actor, nodeID, expectedStatus, restore)
		if !result.Success {
			results = append(results, result)
			continue
		}
		if err := operation(node); err != nil {
			message := "操作失败"
			switch {
			case errors.Is(err, gorm.ErrDuplicatedKey):
				message = "原目录中已存在同名项目"
			case errors.Is(err, dao.ErrNodeState), errors.Is(err, gorm.ErrRecordNotFound):
				message = "节点或父目录状态已变化"
			default:
				logBatchNodeFailure(action, actor.WorkspaceID, node.ID, err)
			}
			results = append(results, failedBatchNode(node, message))
			continue
		}
		message := "已移入回收站"
		if restore {
			message = "已恢复"
		}
		results = append(results, successfulBatchNode(node, message))
	}
	return results
}

func (h *NodeHandler) batchReadableNode(c *gin.Context, actor authorization.Actor, nodeID uint) (*model.Node, batchNodeResult) {
	node, err := h.nodeDAO.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		logBatchNodeFailure("read", actor.WorkspaceID, nodeID, err)
		return nil, unavailableBatchNode(nodeID)
	}
	if node == nil || node.Status != "active" {
		return nil, unavailableBatchNode(nodeID)
	}
	allowed, err := h.authz.CanRead(actor, node.ID)
	if err != nil {
		logBatchNodeFailure("authorize_read", actor.WorkspaceID, nodeID, err)
		return nil, unavailableBatchNode(nodeID)
	}
	recordDataAuthorization(c, allowed, "node:read", node.Type, node.ID)
	if !allowed {
		return nil, unavailableBatchNode(nodeID)
	}
	return node, successfulBatchNode(node, "")
}

func (h *NodeHandler) batchWritableNode(c *gin.Context, actor authorization.Actor, nodeID uint, expectedStatus string, restore bool) (*model.Node, batchNodeResult) {
	node, err := h.nodeDAO.GetByID(actor.WorkspaceID, nodeID)
	if err != nil {
		logBatchNodeFailure("read", actor.WorkspaceID, nodeID, err)
		return nil, unavailableBatchNode(nodeID)
	}
	if node == nil || node.Status != expectedStatus {
		return nil, unavailableBatchNode(nodeID)
	}
	var allowed bool
	if restore {
		allowed, err = h.authz.CanRestore(actor, node.ID)
	} else {
		allowed, err = h.authz.CanWrite(actor, node.ID)
	}
	if err != nil {
		logBatchNodeFailure("authorize_write", actor.WorkspaceID, nodeID, err)
		return nil, unavailableBatchNode(nodeID)
	}
	recordDataAuthorization(c, allowed, "node:write", node.Type, node.ID)
	if !allowed {
		return nil, unavailableBatchNode(nodeID)
	}
	return node, successfulBatchNode(node, "")
}

func uniqueNodeIDs(nodeIDs []uint) []uint {
	result := make([]uint, 0, len(nodeIDs))
	seen := make(map[uint]struct{}, len(nodeIDs))
	for _, nodeID := range nodeIDs {
		if _, exists := seen[nodeID]; exists {
			continue
		}
		seen[nodeID] = struct{}{}
		result = append(result, nodeID)
	}
	return result
}

func successfulBatchNode(node *model.Node, message string) batchNodeResult {
	return batchNodeResult{NodeID: node.ID, Name: node.Name, Success: true, Message: message}
}

func failedBatchNode(node *model.Node, message string) batchNodeResult {
	return batchNodeResult{NodeID: node.ID, Name: node.Name, Success: false, Message: message}
}

func unavailableBatchNode(nodeID uint) batchNodeResult {
	return batchNodeResult{NodeID: nodeID, Success: false, Message: "节点不可用或无权操作"}
}

func respondBatchNodes(c *gin.Context, results []batchNodeResult) {
	result := batchNodeResponse{Results: results}
	for _, item := range results {
		if item.Success {
			result.SuccessCount++
		} else {
			result.FailedCount++
		}
	}
	response.Success(c, result)
}

func logBatchNodeFailure(action string, workspaceID, nodeID uint, err error) {
	logger.Error("batch_node_operation_failed", "action", action, "workspace_id", workspaceID, "node_id", nodeID, "error", err)
}
