/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { ElMessage } from 'element-plus'
import { copyText } from '../utils/copy'

function bindCopyHandler(el, binding) {
  if (el._copyHandler) {
    el.removeEventListener('click', el._copyHandler)
  }

  el._copyHandler = async () => {
    // 优先取绑定的值 (v-copy="val")，没有绑定值则取 innerText
    const text = binding.value !== undefined ? binding.value : el.innerText
    if (!text || text === '-') return

    const copied = await copyText(text)
    if (copied) {
      ElMessage.success('已复制: ' + text)
    } else {
      ElMessage.error('复制失败，请手动选择复制')
    }
  }

  el.addEventListener('click', el._copyHandler)
}

export default {
  mounted(el, binding) {
    el.classList.add('copyable')
    bindCopyHandler(el, binding)
  },

  updated(el, binding) {
    // 组件更新时重新绑定回调，确保获取到最新的 value
    bindCopyHandler(el, binding)
  },

  unmounted(el) {
    if (el._copyHandler) {
      el.removeEventListener('click', el._copyHandler)
      delete el._copyHandler
    }
  }
}
