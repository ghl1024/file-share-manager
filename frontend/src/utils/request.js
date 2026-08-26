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
import { ElMessage } from 'element-plus'
import router from '../router'

const request = axios.create({
  baseURL: '/api/fileshare/v1',
  timeout: 30000,
  withCredentials: true,
  headers: { 'X-Requested-With': 'XMLHttpRequest' }
})

// 请求拦截器
request.interceptors.request.use(
  config => {
		if (config.url && !config.url.startsWith('/auth/')) {
			config.url = `/management${config.url.startsWith('/') ? config.url : `/${config.url}`}`
		}
    return config
  },
  error => Promise.reject(error)
)

// 响应拦截器
request.interceptors.response.use(
  response => {
    if (response.config.responseType === 'blob' || response.config.responseType === 'arraybuffer') {
      return response
    }
    const res = response.data
    if (res.code !== 0) {
      if (!response.config.silent) ElMessage.error(res.message || '请求失败')
      if (res.code === 401) {
        router.push('/login')
      }
      const businessError = new Error(res.message || '请求失败')
      businessError.presented = !response.config.silent
      return Promise.reject(businessError)
    }
    return res
  },
  error => {
    if (axios.isCancel(error)) {
      return Promise.reject(error)
    }
    const serverResponse = error.response?.data
    const message = serverResponse?.message || error.message || '网络错误'
    if (error.response?.status === 401 || serverResponse?.code === 401) {
      router.push('/login')
    }
    if (!error.config?.silent) ElMessage.error(message)
    const normalizedError = new Error(message)
    normalizedError.response = error.response
    normalizedError.code = serverResponse?.code
    normalizedError.cause = error
    normalizedError.presented = !error.config?.silent
    return Promise.reject(normalizedError)
  }
)

export default request
