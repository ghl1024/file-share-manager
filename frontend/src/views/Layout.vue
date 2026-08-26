<!--
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 -->

<template>
  <el-container class="layout-container">
    <div v-if="showMobileMask" class="mobile-mask" @click="isCollapse = true"></div>

    <el-aside
      :width="sidebarWidth"
      :class="['layout-sidebar', { 'is-mobile-collapsed': isMobile && isCollapse, 'is-collapsed': isCollapse && !isMobile }]"
    >
      <div class="sidebar-top">
        <div class="sidebar-logo" @click="$router.push('/')">
          <img :src="logoUrl" alt="File Share Manager Logo" class="logo-svg" />
          <div v-show="!isCollapse" class="logo-copy">
            <span class="logo-text">File Share Manager</span>
            <span class="logo-subtitle">文件协作控制台</span>
          </div>
        </div>
      </div>

      <el-scrollbar class="sidebar-scroll">
        <el-menu
          :default-active="currentPath"
          :collapse="isCollapse && !isMobile"
          router
          class="sidebar-menu"
          @select="handleMenuSelect"
        >

          <template v-for="menu in sidebarMenus" :key="menu.id">
            <el-sub-menu v-if="menu.children && menu.children.length" :index="menu.path">
              <template #title>
                <el-icon><component :is="resolveAppIcon(menu.icon, 'Folder')" /></el-icon>
                <span>{{ menu.name }}</span>
              </template>

              <el-menu-item v-for="child in menu.children" :key="child.id" :index="child.path">
                <el-icon><component :is="resolveAppIcon(child.icon)" /></el-icon>
                <template #title>{{ child.name }}</template>
              </el-menu-item>
            </el-sub-menu>

            <el-menu-item v-else :index="menu.path">
              <el-icon><component :is="resolveAppIcon(menu.icon)" /></el-icon>
              <template #title>{{ menu.name }}</template>
            </el-menu-item>
          </template>
        </el-menu>
      </el-scrollbar>

      <div v-if="!isMobile" class="sidebar-footer">
        <button
          type="button"
          class="sidebar-toggle"
          :aria-label="isCollapse ? '展开侧边栏' : '收起侧边栏'"
          :title="isCollapse ? '展开侧边栏' : '收起侧边栏'"
          @click="toggleSidebar"
        >
          <el-icon :size="18">
            <Expand v-if="isCollapse" />
            <Fold v-else />
          </el-icon>
          <span v-show="!isCollapse">收起导航</span>
        </button>
      </div>
    </el-aside>

    <el-container class="layout-content-wrapper">
      <el-header class="layout-header">
        <div class="header-left">
          <button
            v-if="isMobile"
            type="button"
            class="collapse-btn"
            :aria-label="isCollapse ? '打开导航菜单' : '关闭导航菜单'"
            :title="isCollapse ? '打开导航菜单' : '关闭导航菜单'"
            @click="toggleSidebar"
          >
            <el-icon :size="20">
              <Fold v-if="!isCollapse" />
              <Expand v-else />
            </el-icon>
          </button>

          <div class="page-heading">
            <h1 class="page-title">{{ currentTitle }}</h1>
            <p class="page-description">{{ currentDescription }}</p>
          </div>
        </div>

        <div class="header-right">
          <el-select
            v-if="userStore.workspaces.length"
            class="workspace-switcher"
            :model-value="userStore.currentWorkspaceId"
            placeholder="选择工作空间"
            :loading="workspaceSwitching"
            @change="handleWorkspaceChange"
          >
            <template #prefix><el-icon><OfficeBuilding /></el-icon></template>
            <el-option
              v-for="workspace in userStore.workspaces"
              :key="workspace.id"
              :label="workspaceOptionLabel(workspace)"
              :value="workspace.id"
            />
          </el-select>

          <UserNotificationCenter />

          <el-dropdown trigger="click" @command="switchTheme">
            <span class="header-icon-btn" title="切换主题" aria-label="切换主题">
              <span class="theme-swatch" :style="{ backgroundColor: themeStore.theme.vars['--accent-primary'] }"></span>
              <el-icon class="theme-dropdown-arrow"><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item
                  v-for="theme in themeStore.themeList"
                  :key="theme.key"
                  :command="theme.key"
                  :class="{ 'is-active-theme': theme.key === themeStore.currentTheme }"
                >
                  <span class="theme-option-swatch" :style="{ backgroundColor: theme.vars['--accent-primary'] }"></span>
                  {{ theme.name }}
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>

          <el-dropdown @command="handleCommand">
            <span class="user-info">
              <el-avatar :size="34" class="user-avatar">
                {{ userStore.user?.username?.charAt(0)?.toUpperCase() }}
              </el-avatar>
              <div class="user-meta">
                <span class="username">{{ userStore.user?.real_name || userStore.user?.username }}</span>
                <small>{{ userStore.user?.is_super_admin ? '超级管理员' : '空间成员' }}</small>
              </div>
              <el-icon><ArrowDown /></el-icon>
            </span>
            <template #dropdown>
              <el-dropdown-menu>
				<el-dropdown-item command="profile">
				  <el-icon><User /></el-icon>
				  个人中心
				</el-dropdown-item>
                <el-dropdown-item command="logout" class="text-danger">
                  <el-icon><SwitchButton /></el-icon>
                  退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </el-header>

	  <ProfileCenterDrawer ref="profileCenterRef" />

      <el-main class="layout-main">
        <div class="page-wrapper">
          <router-view v-slot="{ Component }">
            <keep-alive :include="['Dashboard']">
              <component :is="Component" />
            </keep-alive>
          </router-view>

          <footer class="layout-footer">
            <span>File Share Manager</span>
            <span class="footer-status"><i></i>服务已连接</span>
            <a href="https://hayden.pub" target="_blank">© 2026 HaydenGuo</a>
          </footer>
        </div>
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Fold, Expand, ArrowDown, OfficeBuilding, SwitchButton, User } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useThemeStore } from '../stores/theme'
import { useUserStore } from '../stores/user'
import { resolveAppIcon } from '../utils/icons'
import UserNotificationCenter from '../components/notifications/UserNotificationCenter.vue'
import ProfileCenterDrawer from '../components/profile/ProfileCenterDrawer.vue'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()
const themeStore = useThemeStore()
const logoUrl = `${import.meta.env.BASE_URL}logo.svg`

const viewportWidth = ref(typeof window !== 'undefined' ? window.innerWidth : 1440)
const isCollapse = ref(false)

const currentPath = computed(() => route.path)
const currentTitle = computed(() => route.meta?.title || '导航首页')
const currentDescription = computed(() => route.meta?.description || '文件共享管理控制台')
const workspaceSwitching = ref(false)
const profileCenterRef = ref(null)
const builtinMenuLabels = { audit: '审计中心' }

function menuLabel(menu) {
  return builtinMenuLabels[menu?.code] || builtinMenuLabels[menu?.id] || menu?.name
}

const sidebarMenus = computed(() => {
  return userStore.menus
    .filter((menu) => menu.path !== '/home' && menu.type !== 2)
    .map((menu) => {
      if (menu.children && menu.children.length > 0) {
        return {
          ...menu,
          name: menuLabel(menu),
          children: menu.children.filter(child => child.type !== 2).map(child => ({ ...child, name: menuLabel(child) }))
        }
      }
      return { ...menu, name: menuLabel(menu) }
    })
})
const isMobile = computed(() => viewportWidth.value < 960)

const sidebarWidth = computed(() => {
  if (isMobile.value) return '260px'
  return isCollapse.value ? '64px' : '224px'
})

const showMobileMask = computed(() => isMobile.value && !isCollapse.value)

watch(
  () => route.path,
  () => {
    if (isMobile.value) {
      isCollapse.value = true
    }
  }
)

onMounted(async () => {
  themeStore.applyTheme()
  syncViewport()
  window.addEventListener('resize', syncViewport)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', syncViewport)
})

function syncViewport() {
  viewportWidth.value = window.innerWidth

  if (viewportWidth.value < 960) {
    isCollapse.value = true
  }
}

function toggleSidebar() {
  isCollapse.value = !isCollapse.value
}

function handleMenuSelect() {
  if (isMobile.value) {
    isCollapse.value = true
  }
}

function switchTheme(themeKey) {
  themeStore.setTheme(themeKey)
}

function workspaceOptionLabel(workspace) {
  return userStore.user?.is_super_admin && workspace?.is_member === false
    ? `${workspace.name}（跨空间）`
    : workspace.name
}

async function handleWorkspaceChange(workspaceId) {
  if (!workspaceId || workspaceId === userStore.currentWorkspaceId) return
  workspaceSwitching.value = true
  try {
    const workspace = userStore.workspaces.find(item => item.id === workspaceId)
    const reason = await crossWorkspaceReason(workspace)
    if (reason === null) return
    await userStore.switchWorkspace(workspaceId, reason)
    ElMessage.success('工作空间已切换')
    if (route.meta.requiresWorkspace) await router.replace(route.fullPath)
  } finally {
    workspaceSwitching.value = false
  }
}

async function crossWorkspaceReason(workspace) {
  if (!userStore.user?.is_super_admin || workspace?.is_member !== false) return ''
  try {
    const { value } = await ElMessageBox.prompt(
      `你不是“${workspace?.name || '目标工作空间'}”的成员，本次进入及文件读取将记录高风险安全审计。`,
      '跨空间访问确认',
      {
        confirmButtonText: '确认进入',
        cancelButtonText: '取消',
        inputPlaceholder: '填写访问原因，例如：安全事件调查',
        inputValidator: value => Array.from(String(value || '').trim()).length >= 5 || '访问原因至少需要 5 个字符',
        type: 'warning'
      }
    )
    return String(value || '').trim()
  } catch {
    return null
  }
}

async function handleCommand(command) {
	if (command === 'profile') {
		profileCenterRef.value?.open()
		return
	}
  if (command === 'logout') {
	await userStore.logout()
	router.push('/login')
  }
}
</script>

<style scoped>
.layout-container {
  height: 100vh;
  overflow: hidden;
  position: relative;
  background: transparent;
}

.layout-sidebar {
  position: relative;
  z-index: 12;
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 0 10px 12px;
  background: var(--bg-sidebar);
  border-right: 1px solid var(--panel-border);
  transition: width 0.28s ease, transform 0.28s ease;
  box-shadow: none;
}

.sidebar-top {
  display: flex;
  flex-direction: column;
  min-height: 60px;
  justify-content: center;
  border-bottom: 1px solid var(--border-color);
}

.sidebar-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 8px;
  border-radius: var(--radius-sm);
  background: transparent;
  border: 0;
  cursor: pointer;
  transition: var(--transition);
}

.sidebar-logo:hover {
  background: var(--surface-muted);
}

.logo-svg {
  width: 30px;
  height: 30px;
  flex-shrink: 0;
}

.logo-copy {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 3px;
}

.logo-subtitle {
  color: var(--text-muted);
  font-size: 10px;
  line-height: 1;
  white-space: nowrap;
}

.logo-text {
  font-size: 15px;
  font-weight: 700;
  line-height: 1;
  color: var(--text-primary);
  white-space: nowrap;
}

.sidebar-scroll {
  flex: 1;
  min-height: 0;
}

.sidebar-menu {
  padding: 6px 2px;
  background: transparent !important;
  border-right: none !important;
}

.sidebar-footer {
  flex-shrink: 0;
  padding-top: 8px;
  border-top: 1px solid var(--border-color);
}

.sidebar-toggle {
  width: 100%;
  min-height: 38px;
  display: flex;
  align-items: center;
  justify-content: flex-start;
  gap: 10px;
  padding: 8px 12px;
  border: 1px solid transparent;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-secondary);
  cursor: pointer;
  transition: var(--transition);
}

.sidebar-toggle:hover,
.sidebar-toggle:focus-visible {
  color: var(--accent-primary);
  border-color: var(--border-hover);
  background: var(--surface-hover);
  outline: none;
}

/* ================= 折叠状态专用精准样式 ================= */
.layout-sidebar.is-collapsed {
  padding: 0 8px 12px;
}

.layout-sidebar.is-collapsed .sidebar-logo {
  padding: 8px 0;
  justify-content: center;
  border-radius: var(--radius-sm);
}

.layout-sidebar.is-collapsed .sidebar-menu {
  padding: 4px 0;
  width: 100% !important;
}

.layout-sidebar.is-collapsed .sidebar-menu :deep(> .el-menu-item),
.layout-sidebar.is-collapsed .sidebar-menu :deep(> .el-sub-menu),
.layout-sidebar.is-collapsed .sidebar-menu :deep(> .el-sub-menu > .el-sub-menu__title) {
  padding: 0 !important;
  width: 100% !important;
  height: 40px !important;
  line-height: 40px !important;
  display: flex !important;
  align-items: center !important;
  justify-content: center !important;
  margin-left: 0 !important;
  margin-right: 0 !important;
  border-radius: var(--radius-sm) !important;
}

.layout-sidebar.is-collapsed .sidebar-menu :deep(.el-icon) {
  margin: 0 !important;
  font-size: 19px;
}

.layout-sidebar.is-collapsed .sidebar-footer {
  padding-top: 8px;
}

.layout-sidebar.is-collapsed .sidebar-toggle {
  justify-content: center;
  min-height: 40px;
  padding: 8px 0;
}

/* ================= 现代 SaaS 一体化菜单排版规范 ================= */

/* 一级菜单项与子菜单标题通用基础 */
.sidebar-menu :deep(> .el-menu-item),
.sidebar-menu :deep(> .el-sub-menu > .el-sub-menu__title) {
  height: 40px;
  line-height: 40px;
  margin-bottom: 2px;
  border-radius: var(--radius-sm) !important;
  background: transparent !important;
  border: none !important;
  color: var(--text-secondary) !important;
  font-size: 14px;
  font-weight: 600;
  transition: var(--transition);
}

/* 菜单 Icon 居中与间距 */
.sidebar-menu :deep(.el-icon) {
  font-size: 18px;
  margin-right: 10px;
  transition: transform 0.25s ease;
}

/* Hover 浮动微光胶囊效果 */
.sidebar-menu :deep(> .el-menu-item:hover),
.sidebar-menu :deep(> .el-sub-menu > .el-sub-menu__title:hover) {
  background: var(--surface-hover) !important;
  color: var(--text-primary) !important;
}

.sidebar-menu :deep(> .el-menu-item:hover .el-icon),
.sidebar-menu :deep(> .el-sub-menu > .el-sub-menu__title:hover .el-icon) {
  transform: none;
  color: var(--accent-primary);
}

/* 菜单项选中态 (Active Floating Pill) - 适用于所有选中的菜单 */
.sidebar-menu :deep(.el-menu-item.is-active) {
  background: var(--accent-primary) !important;
  color: #ffffff !important;
  font-weight: 700;
  box-shadow: none !important;
}

.sidebar-menu :deep(.el-menu-item.is-active .el-icon) {
  color: #ffffff !important;
  transform: none;
}

/* 带子菜单的一级菜单 展开/激活 状态标题 (Active Submenu Header Pill) */
.sidebar-menu :deep(.el-sub-menu.is-opened > .el-sub-menu__title),
.sidebar-menu :deep(.el-sub-menu.is-active > .el-sub-menu__title) {
  background: var(--surface-muted) !important;
  color: var(--accent-primary) !important;
  font-weight: 700;
  border-left: 2px solid var(--accent-primary) !important;
  box-shadow: none !important;
}

.sidebar-menu :deep(.el-sub-menu.is-opened > .el-sub-menu__title .el-icon),
.sidebar-menu :deep(.el-sub-menu.is-active > .el-sub-menu__title .el-icon) {
  color: var(--accent-primary) !important;
}

/* 子菜单内嵌容器与现代化左侧指示垂直轨迹线 */
.sidebar-menu :deep(.el-sub-menu .el-menu--inline) {
  background: transparent !important;
  position: relative;
  padding: 2px 0 4px 16px;
}

.sidebar-menu :deep(.el-sub-menu .el-menu--inline::before) {
  content: "";
  position: absolute;
  top: 6px;
  bottom: 8px;
  left: 10px;
  width: 2px;
  border-radius: 999px;
  background: color-mix(in srgb, var(--accent-primary) 24%, transparent);
}

/* 子菜单内嵌项目 */
.sidebar-menu :deep(.el-sub-menu .el-menu-item) {
  height: 40px;
  line-height: 40px;
  margin: 3px 0;
  padding-left: 16px !important;
  border-radius: var(--radius-sm) !important;
  background: transparent !important;
  border: none !important;
  color: var(--text-secondary) !important;
  font-size: 13.5px;
  font-weight: 500;
  transition: var(--transition);
}

.sidebar-menu :deep(.el-sub-menu .el-menu-item:not(.is-active):hover) {
  background: var(--surface-hover) !important;
  color: var(--text-primary) !important;
}

/* 子菜单选中态 (确保与一级菜单100%完全一致的渐变胶囊Pill与白字) */
.sidebar-menu :deep(.el-sub-menu .el-menu-item.is-active) {
  background: var(--accent-primary) !important;
  color: #ffffff !important;
  font-weight: 700;
  box-shadow: none !important;
}

.sidebar-menu :deep(.el-sub-menu .el-menu-item.is-active .el-icon),
.sidebar-menu :deep(.el-sub-menu .el-menu-item.is-active span) {
  color: #ffffff !important;
}

/* Sub-menu children hover */
.sidebar-menu :deep(.el-sub-menu .el-menu-item:not(.is-active):hover) {
  background: var(--surface-hover) !important;
  color: var(--text-primary) !important;
}

.layout-content-wrapper {
  min-width: 0;
  height: 100vh;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.layout-header {
  margin: 0;
  height: 60px;
  padding: 0 20px;
  border-radius: 0;
  border: 0;
  border-bottom: 1px solid var(--border-color);
  background: var(--panel-bg-strong);
  box-shadow: none;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  position: relative;
  overflow: hidden;
}

.layout-header::before {
  display: none;
}

.header-left, .header-right {
  position: relative;
  z-index: 1;
}

.page-heading { display: flex; flex-direction: column; gap: 3px; margin-left: 4px; }
.page-title { margin: 0; }
.page-description { margin: 0; color: var(--text-muted); font-size: 11px; line-height: 1; }

.header-left,
.header-right,
.header-icon-btn,
.user-info {
  display: flex;
  align-items: center;
}

.header-left,
.header-right {
  gap: 14px;
}

.collapse-btn {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--panel-border);
  border-radius: var(--radius-sm);
  background: var(--surface-soft);
  color: var(--text-secondary);
  cursor: pointer;
  transition: var(--transition);
}

.collapse-btn:hover {
  color: var(--accent-primary);
  border-color: var(--border-hover);
}

.page-title {
  font-size: 18px;
  line-height: 1.2;
  font-weight: 700;
  color: var(--text-primary);
}

.header-icon-btn,
.user-info {
  gap: 8px;
  padding: 6px 10px;
  border-radius: var(--radius-sm);
  border: 1px solid var(--panel-border);
  background: var(--surface-soft);
  color: var(--text-secondary);
  cursor: pointer;
  transition: all 0.3s;
}

.theme-dropdown-arrow {
  margin-left: 4px;
  font-size: 12px;
  color: var(--text-secondary);
}

.header-icon-btn:hover,
.user-info:hover {
  border-color: var(--border-hover);
  color: var(--text-primary);
}

.username {
  font-size: 14px;
  font-weight: 600;
}

.theme-swatch, .theme-option-swatch { display: inline-block; width: 14px; height: 14px; border: 2px solid color-mix(in srgb, currentColor 14%, transparent); border-radius: 50%; }
.theme-option-swatch { margin-right: 8px; }

.workspace-switcher { width: 210px; }
.workspace-switcher :deep(.el-select__wrapper) { min-height: 36px; background: var(--surface-soft) !important; }

.is-active-theme {
  color: var(--accent-primary) !important;
  font-weight: 700;
}

.user-avatar {
  background: var(--accent-primary);
  color: #fff;
  font-weight: 700;
}

.user-meta {
  display: flex;
  flex-direction: column;
  gap: 2px;
  line-height: 1.1;
}

.user-meta small {
  color: var(--text-muted);
  font-size: 11px;
}

.layout-main {
  flex: 1;
  padding: 16px 20px 24px;
  background: transparent;
  overflow: auto;
  display: flex;
  flex-direction: column;
}

.page-wrapper {
  width: 100%;
  max-width: 100%;
  margin: 0 auto;
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.layout-footer {
  margin-top: auto;
  padding: 20px 0 2px;
  display: flex;
  flex-direction: row;
  align-items: center;
  justify-content: center;
  gap: 14px;
  color: var(--text-muted);
  font-size: 11px;
  opacity: 0.75;
}

.layout-footer:hover {
  opacity: 1;
}

.footer-status { display: inline-flex; align-items: center; gap: 5px; }
.footer-status i { width: 6px; height: 6px; border-radius: 50%; background: var(--accent-green); }

.mobile-mask {
  position: fixed;
  inset: 0;
  z-index: 11;
  background: rgba(2, 6, 23, 0.42);
}

@media (max-width: 960px) {
  .layout-sidebar {
    position: fixed;
    top: 0;
    left: 0;
    bottom: 0;
    z-index: 20;
    box-shadow: 0 24px 60px rgba(15, 23, 42, 0.18);
  }

  .layout-sidebar.is-mobile-collapsed {
    transform: translateX(calc(-100% - 20px));
  }

  .layout-header {
    min-height: 56px;
    height: 56px;
    padding: 0 14px;
  }

  .layout-main {
    padding: 14px;
  }

  .workspace-switcher { display: none; }
}

@media (max-width: 720px) {
  .page-description,
  .user-meta small,
  .username {
    display: none;
  }

  .user-info,
  .header-icon-btn {
    padding: 6px 8px;
  }

  .page-title {
    font-size: 16px;
  }

  .layout-footer { flex-wrap: wrap; gap: 8px 12px; }
}
</style>
