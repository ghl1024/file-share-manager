/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import axios from 'axios'

const publicRequest = axios.create({
  baseURL: '/api/fileshare/v1/share',
  timeout: 30000,
  withCredentials: true,
  headers: { 'X-Requested-With': 'XMLHttpRequest' }
})

publicRequest.interceptors.response.use(
  (response) => {
    if (response.config.responseType === 'blob' || response.config.responseType === 'arraybuffer') return response
    const payload = response.data
    if (payload.code !== 0) throw new Error(payload.message || '请求失败')
    return payload
  },
  (error) => {
    const message = error.response?.data?.message || error.message || '网络错误'
    const normalized = new Error(message)
    normalized.response = error.response
    return Promise.reject(normalized)
  }
)

export function getShare(token) {
  return publicRequest.get(`/${encodeURIComponent(token)}`)
}

export function verifyShare(token, password) {
  return publicRequest.post(`/${encodeURIComponent(token)}/verify`, { password })
}

export function downloadShare(token, item) {
  return publicRequest.get(`/${encodeURIComponent(token)}/download`, {
    params: item ? { item } : undefined,
    responseType: 'blob'
  })
}
