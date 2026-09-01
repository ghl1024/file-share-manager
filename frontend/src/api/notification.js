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

export const notificationApi = {
  listUserNotifications: (params = {}) => request.get('/notifications', { params }),
  getUnreadCount: () => request.get('/notifications/unread-count', { silent: true }),
  markRead: (id) => request.put(`/notifications/${id}/read`),
  markAllRead: () => request.put('/notifications/read-all'),
  getPreferences: () => request.get('/notifications/preferences'),
  savePreferences: (data) => request.put('/notifications/preferences', data),
  openUserNotification: (id) => request.post(`/notifications/${id}/open`),
  listChannels: (params = {}) => request.get('/system/notifications', { params }),
  createChannel: (data) => request.post('/system/notifications', data),
  updateChannel: (id, data) => request.put(`/system/notifications/${id}`, data),
  deleteChannel: (id) => request.delete(`/system/notifications/${id}`),
  testChannel: (id) => request.post(`/system/notifications/${id}/test`),
  listOutbox: (params = {}) => request.get('/system/notifications/outbox', { params }),
  retryOutbox: (id) => request.post(`/system/notifications/outbox/${id}/retry`)
}
