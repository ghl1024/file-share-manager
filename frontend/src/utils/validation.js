/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

export function strongPasswordError(password) {
  const value = String(password || '')
  if (value.length < 12 || value.length > 128) return '密码长度必须在 12 到 128 个字符之间'
  if (/\s/.test(value)) return '密码不能包含空白字符'

  const classes = [/[a-z]/.test(value), /[A-Z]/.test(value), /\d/.test(value), /[^A-Za-z\d\s]/.test(value)]
  if (classes.filter(Boolean).length < 3) return '密码必须包含大小写字母、数字、特殊字符中的至少三类'
  return ''
}

export function validateStrongPassword(rule, value, callback) {
  if (!value) return callback()
  const message = strongPasswordError(value)
  callback(message ? new Error(message) : undefined)
}
