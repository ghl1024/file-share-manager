/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import 'element-plus/theme-chalk/dark/css-vars.css'
// Service-style components are created imperatively, so their styles are not
// discovered by the template-based Element Plus auto-import plugin.
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/notification/style/css'
import './style.css'
import { setupPermissionDirective } from './directives/permission'
import copyDirective from './directives/copy'
import { appIcons } from './utils/icons'
import ActionButton from './components/common/ActionButton.vue'

const app = createApp(App)

// 注册 Element Plus 常用图标组件
for (const [key, component] of Object.entries(appIcons)) {
  app.component(key, component)
}

app.component('action-button', ActionButton)

app.use(createPinia())
app.use(router)

app.directive('copy', copyDirective)

// 注册权限指令
setupPermissionDirective(app)

app.mount('#app')
