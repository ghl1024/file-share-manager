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

export const systemApi = {
  config: () => request.get('/system/configs'),
  ldapConfig: () => request.get('/system/ldap'),
  saveLDAP: (data) => request.post('/system/ldap', data),
  ldapTest: (data = {}) => request.post('/system/ldap/test', data),
  ldapSync: () => request.post('/system/ldap/sync'),
  ldapSyncHistory: (params = {}) => request.get('/system/ldap/history', { params }),
  clamavHealth: () => request.get('/system/clamav/health'),
  reconcile: () => request.get('/storage/reconcile'),
  quarantineOrphans: (data) => request.post('/storage/reconcile/quarantine', data),
  storageHealth: () => request.get('/system/storage/health')
}
