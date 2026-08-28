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

export const backupApi = {
  list: (params) => request.get('/backups', { params }),
  health: () => request.get('/backups/health'),
  listRestoreDrills: (params) => request.get('/backup-restore-drills', { params }),
  detail: (id) => request.get(`/backups/${id}`),
  createBaseline: () => request.post('/backups/baseline'),
  createIncremental: () => request.post('/backups/incremental'),
  compact: () => request.post('/backups/compact', { confirm: true }),
  retry: (id) => request.post(`/backups/${id}/retry`),
  restoreDrill: (id) => request.post(`/backups/${id}/restore-drill`, { confirm: true }),
  verify: (id) => request.post(`/backups/${id}/verify`),
  restore: (id, data) => request.post(`/backups/${id}/restore`, data),
  restoreWorkspace: (id, data) => request.post(`/backups/${id}/restore-workspace`, data)
}
