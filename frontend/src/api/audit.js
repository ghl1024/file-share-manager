/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import request from '../utils/request'

export const auditApi = {
  getLogs: (params) => request.get('/audit/logs', { params }),
  getDetail: (id) => request.get(`/audit/events/${id}`),
  getPolicy: () => request.get('/audit/policy'),
  verifyStream: (workspaceId) => request.get(`/audit/streams/${workspaceId}/verify`),
  createExport: (params, format = 'csv') => request.post('/audit/exports', { format }, { params }),
  listExports: (params = {}) => request.get('/audit/exports', { params }),
  getExport: (id) => request.get(`/audit/exports/${id}`),
  downloadExport: (id) => request.get(`/audit/exports/${id}/download`, { responseType: 'blob' }),
  listArchives: (params = {}) => request.get('/audit/archives', { params }),
  runArchive: () => request.post('/audit/archives/run')
}
