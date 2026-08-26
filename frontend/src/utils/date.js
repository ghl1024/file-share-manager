/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

/**
 * 全站统一时间格式化函数
 * 确保输出格式一律为：YYYY-MM-DD HH:mm:ss（如 2027-02-12 07:59:59）
 */
export function formatDateTime(val) {
  if (!val) return '-'
  
  const d = new Date(val)
  if (isNaN(d.getTime())) return '-'

  const pad = (num) => String(num).padStart(2, '0')

  const year = d.getFullYear()
  const month = pad(d.getMonth() + 1)
  const day = pad(d.getDate())
  const hours = pad(d.getHours())
  const minutes = pad(d.getMinutes())
  const seconds = pad(d.getSeconds())

  return `${year}-${month}-${day} ${hours}:${minutes}:${seconds}`
}
