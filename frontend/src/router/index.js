/*
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 */

import { createRouter, createWebHistory } from 'vue-router'
import { useUserStore } from '../stores/user'

const routes = [
  {
    path: '/login',
    name: 'Login',
    component: () => import('../views/Login.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/share/:token',
    name: 'SharePage',
    component: () => import('../views/share/SharePage.vue'),
    meta: { requiresAuth: false }
  },
  {
    path: '/',
    name: 'Layout',
    component: () => import('../views/Layout.vue'),
    redirect: '/dashboard',
    meta: { requiresAuth: true },
    children: [
      {
        path: 'dashboard',
        name: 'Dashboard',
        component: () => import('../views/dashboard/Dashboard.vue'),
        meta: { title: '工作台', description: '空间概览与常用入口' }
      },
      {
        path: 'workspaces',
        name: 'WorkspaceList',
        component: () => import('../views/workspace/WorkspaceList.vue'),
        meta: { title: '工作空间', description: '管理空间、成员与用户组' }
      },
      {
        path: 'files',
        name: 'Files',
        component: () => import('../views/files/Files.vue'),
        meta: { title: '文件目录', description: '浏览、上传和管理文件', requiresWorkspace: true, permission: 'file:list' }
      },
      {
        path: 'shares',
        name: 'ShareList',
        component: () => import('../views/share/ShareList.vue'),
        meta: { title: '外链分享', description: '查看与撤销对外分享', requiresWorkspace: true, permission: 'file:share:create' }
      },
      // 系统管理
      {
        path: 'system/user',
        name: 'UserManage',
        component: () => import('../views/system/UserManage.vue'),
        meta: { title: '用户管理', description: '维护账号与登录状态', superAdmin: true }
      },
      {
        path: 'system/config',
        name: 'SystemConfig',
        component: () => import('../views/system/SystemConfig.vue'),
        meta: { title: '系统配置', description: '检查依赖与存储运行状态', superAdmin: true }
      },
      {
        path: 'system/menu',
        name: 'MenuManage',
        component: () => import('../views/system/MenuManage.vue'),
        meta: { title: '菜单权限', description: '维护导航、页面和按钮权限映射', superAdmin: true }
      },
      {
        path: 'system/ldap',
        name: 'SystemLDAP',
        component: () => import('../views/system/SystemConfig.vue'),
        meta: { title: 'LDAP', description: '查看目录服务配置与连接状态', superAdmin: true, systemSection: 'ldap' }
      },
      {
        path: 'system/clamav',
        name: 'SystemClamAV',
        component: () => import('../views/system/SystemConfig.vue'),
        meta: { title: 'ClamAV', description: '查看病毒扫描配置与病毒库状态', superAdmin: true, systemSection: 'clamav' }
      },
      {
        path: 'system/backup-storage',
        name: 'SystemBackupStorage',
        component: () => import('../views/system/SystemConfig.vue'),
        meta: { title: '备份存储', description: '查看备份出口与对象存储状态', superAdmin: true, systemSection: 'backup' }
      },
      {
        path: 'system/notifications',
        name: 'NotificationManage',
        component: () => import('../views/system/NotificationManage.vue'),
        meta: { title: '通知告警', description: '管理通知渠道与可靠投递记录', superAdmin: true }
      },
      {
        path: 'system/backups',
        name: 'BackupList',
        component: () => import('../views/system/BackupList.vue'),
        meta: { title: '备份恢复', description: '管理备份链与文件恢复', requiresWorkspace: true, permission: 'backup:manage' }
      },
      {
        path: 'system/role',
        name: 'RoleManage',
        component: () => import('../views/system/RoleManage.vue'),
        meta: { title: '角色管理', description: '配置空间角色与权限', requiresWorkspace: true, permission: 'workspace:role:manage' }
      },
      // 安全审计
      {
        path: 'audit/history',
        name: 'OperationHistory',
        component: () => import('../views/audit/OperationHistory.vue'),
        meta: { title: '审计中心', description: '查询操作、访问与安全审计事件', requiresWorkspace: true, permission: 'audit:list' }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  routes
})

// 路由守卫
router.beforeEach(async (to, from, next) => {
  const userStore = useUserStore()

	if (to.meta.requiresAuth === false) {
	  if (to.path === '/login' && userStore.isLoggedIn) return next('/')
	  return next()
	}

	try {
	  await userStore.ensureSession()
	  if (to.meta.superAdmin && !userStore.user?.is_super_admin) return next('/dashboard')
	  if (to.meta.requiresWorkspace && !userStore.currentWorkspaceId) return next('/workspaces')
	  if (to.meta.permission && !userStore.hasPermission(to.meta.permission)) return next('/dashboard')
	  next()
	} catch (e) {
	  next('/login')
	}
})

export default router
