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

export const userApi = {
  list: (params) => request.get('/system/users', { params }),
  get: (id) => request.get(`/system/users/${id}`),
  create: (data) => request.post('/system/users', data),
  update: (id, data) => request.put(`/system/users/${id}`, data),
  delete: (id) => request.delete(`/system/users/${id}`),
  updateStatus: (id, data) => request.put(`/system/users/${id}/status`, data),
  assignRoles: (id, data) => request.put(`/users/${id}/roles`, data),
  batchAssignRoles: (data) => request.put('/users/batch/roles', data),
  resetPassword: (id, data) => request.put(`/system/users/${id}/password`, data)
}
