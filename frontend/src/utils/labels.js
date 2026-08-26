/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

export function labelsToText(labels = {}) {
  return Object.entries(labels)
    .map(([key, value]) => `${key}=${value}`)
    .join('\n')
}

export function textToLabels(text = '') {
  return String(text)
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .reduce((acc, line) => {
      const index = line.indexOf('=')
      if (index > 0) {
        acc[line.slice(0, index).trim()] = line.slice(index + 1).trim()
      }
      return acc
    }, {})
}

import { formatDateTime } from './date'

export function formatTime(value) {
  return formatDateTime(value)
}
