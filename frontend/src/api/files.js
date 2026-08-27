/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import request from '@/utils/request'

export function listRoots(params) {
  return request.get('/folders/roots', { params })
}

export function listChildren(folderId, params) {
  return request.get(`/folders/${folderId}/children`, { params })
}

export function createFolder(data) {
  return request.post('/folders', data)
}

export function listFolderTree() {
  return request.get('/folders/tree')
}

export function renameNode(nodeId, data) {
  return request.put(`/nodes/${nodeId}`, data)
}

export function moveNode(nodeId, data) {
  return request.post(`/nodes/${nodeId}/move`, data)
}

export function trashNode(nodeId) {
  return request.delete(`/nodes/${nodeId}`)
}

export function restoreNode(nodeId) {
  return request.post(`/nodes/${nodeId}/restore`)
}

export function listTrash(params) {
  return request.get('/trash', { params })
}

export function searchNodes(params) {
  return request.get('/search', { params })
}

export function listFavorites(params) {
  return request.get('/favorites', { params })
}

export function listSharedWithMe(params) {
  return request.get('/collaboration/shared-with-me', { params })
}

export function listRecentNodes(params) {
  return request.get('/collaboration/recent', { params })
}

export function setFavorite(nodeId, favorite) {
  return request.put(`/nodes/${nodeId}/favorite`, { favorite })
}

export function batchSetFavorite(data) {
  return request.put('/nodes/batch/favorite', data)
}

export function batchMoveNodes(data) {
  return request.post('/nodes/batch/move', data)
}

export function batchTrashNodes(data) {
  return request.post('/nodes/batch/trash', data)
}

export function batchRestoreNodes(data) {
  return request.post('/nodes/batch/restore', data)
}

export function getNodeDetail(nodeId) {
  return request.get(`/nodes/${nodeId}/detail`)
}

export function listNodeActivity(nodeId, params) {
  return request.get(`/nodes/${nodeId}/activity`, { params })
}

export function listNodeComments(nodeId, params) {
  return request.get(`/nodes/${nodeId}/comments`, { params })
}

export function listNodeMentionCandidates(nodeId, keyword = '') {
  return request.get(`/nodes/${nodeId}/mention-candidates`, { params: { keyword } })
}

export function createNodeComment(nodeId, content) {
  return request.post(`/nodes/${nodeId}/comments`, { content })
}

export function updateNodeComment(nodeId, commentId, data) {
  return request.put(`/nodes/${nodeId}/comments/${commentId}`, data)
}

export function deleteNodeComment(nodeId, commentId) {
  return request.delete(`/nodes/${nodeId}/comments/${commentId}`)
}

export function listFileVersions(fileId) {
  return request.get(`/files/${fileId}/versions`)
}

export function restoreFileVersion(fileId, version) {
  return request.post(`/files/${fileId}/versions/${version}/restore`)
}

export function rescanFileVersion(fileId, version) {
  return request.post(`/files/${fileId}/versions/${version}/rescan`)
}

export function createShare(data) {
  return request.post('/shares', data)
}

export function listShares(params) {
  return request.get('/shares', { params })
}

export function getShareDetail(shareId) {
  return request.get(`/shares/${shareId}`)
}

export function revokeShare(shareId) {
  return request.post(`/shares/${shareId}/revoke`)
}

export function listFolderACL(folderId) {
  return request.get(`/folders/${folderId}/acl`)
}

export function replaceFolderACL(folderId, entries) {
  return request.put(`/folders/${folderId}/acl`, { entries })
}

export function setFolderInheritance(folderId, mode) {
  return request.put(`/folders/${folderId}/inheritance`, { mode })
}

export function downloadFile(fileId, version) {
  return request.get(`/files/${fileId}/download`, {
    params: version ? { version } : undefined,
    responseType: 'blob'
  })
}

export function getFilePreview(fileId, version, options = {}) {
  return request.get(`/files/${fileId}/preview`, {
    params: version ? { version } : undefined,
    ...options
  })
}

export function getFilePreviewContent(fileId, version, options = {}) {
  return request.get(`/files/${fileId}/preview/content`, {
    params: version ? { version } : undefined,
    responseType: 'blob',
    ...options
  })
}

export function createBatchDownload(data) {
  return request.post('/batch-downloads', data)
}

export function listBatchDownloads(params, options = {}) {
  return request.get('/batch-downloads', { params, ...options })
}

export function getBatchDownload(jobId, options = {}) {
  return request.get(`/batch-downloads/${jobId}`, options)
}

export function retryBatchDownload(jobId) {
  return request.post(`/batch-downloads/${jobId}/retry`)
}

export function downloadBatchArchive(jobId) {
  return request.get(`/batch-downloads/${jobId}/download`, { responseType: 'blob', timeout: 0 })
}

export function initUpload(data) {
  return request.post('/uploads/init', data)
}

export function uploadPart(uploadId, partNo, data, onUploadProgress) {
  return request.put(`/uploads/${uploadId}/parts/${partNo}`, data, {
    onUploadProgress,
    headers: { 'Content-Type': 'application/octet-stream' }
  })
}

export function getUploadStatus(uploadId, options = {}) {
  return request.get(`/uploads/${uploadId}/status`, options)
}

export function completeUpload(uploadId, data) {
  return request.post(`/uploads/${uploadId}/complete`, data)
}

export function cancelUpload(uploadId) {
  return request.delete(`/uploads/${uploadId}`)
}
