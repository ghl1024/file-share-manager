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

export const roleApi = {
  list: (params) => request.get('/roles', { params }),
  listAll: () => request.get('/roles', { params: { page: 1, page_size: 200 } }),
  get: (id) => request.get(`/roles/${id}`),
  permissions: () => request.get('/permissions'),
  create: (data) => request.post('/roles', data),
  update: (id, data) => request.put(`/roles/${id}`, data),
  delete: (id) => request.delete(`/roles/${id}`),
  assignPermissions: (id, permissions) => request.put(`/roles/${id}/permissions`, { permissions: permissions || [] })
}
