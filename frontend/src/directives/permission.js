/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { useUserStore } from '../stores/user'

export function setupPermissionDirective(app) {
  app.directive('permission', {
    mounted(el, binding) {
      const userStore = useUserStore()
      const permission = binding.value
      if (permission && !userStore.hasPermission(permission)) {
        el.parentNode?.removeChild(el)
      }
    }
  })
}
