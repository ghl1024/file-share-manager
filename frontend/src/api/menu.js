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

export const menuApi = {
  permissions: () => request.get('/system/permissions'),
  list: () => request.get('/system/menus'),
  get: (id) => request.get(`/system/menus/${id}`),
  create: (data) => request.post('/system/menus', data),
  update: (id, data) => request.put(`/system/menus/${id}`, data),
  delete: (id) => request.delete(`/system/menus/${id}`)
}
