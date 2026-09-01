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
  <div class="notification-center">
    <el-badge :value="unreadCount" :hidden="unreadCount < 1" :max="99" class="notification-badge">
      <button type="button" class="notification-trigger" title="通知中心" aria-label="通知中心" @click="openDrawer">
        <el-icon :size="18"><Bell /></el-icon>
      </button>
    </el-badge>

    <el-drawer v-model="visible" size="min(460px, 100vw)" class="user-notification-drawer" :with-header="false" append-to-body>
      <div class="drawer-layout">
        <header class="drawer-header">
          <div>
            <strong>通知中心</strong>
            <span>{{ unreadCount ? `${unreadCount} 条未读` : '暂无未读' }}</span>
          </div>
          <div class="drawer-header-actions">
            <el-tooltip content="通知偏好" placement="bottom">
              <button type="button" class="drawer-icon-button" aria-label="通知偏好" @click="preferenceVisible = !preferenceVisible">
                <el-icon><Setting /></el-icon>
              </button>
            </el-tooltip>
            <el-tooltip content="刷新" placement="bottom">
              <button type="button" class="drawer-icon-button" aria-label="刷新通知" :disabled="loading" @click="refreshAll">
                <el-icon><Refresh /></el-icon>
              </button>
            </el-tooltip>
          </div>
        </header>

        <section v-if="preferenceVisible" class="preference-panel">
          <div class="preference-heading">
            <strong>接收类型</strong>
            <ActionButton action="save" :loading="preferenceSaving" @click="savePreferences" />
          </div>
          <label v-for="option in preferenceOptions" :key="option.key" class="preference-row">
            <span><strong>{{ option.label }}</strong><small>{{ option.description }}</small></span>
            <el-switch v-model="preferences[option.key]" />
          </label>
        </section>

        <div class="notification-filters">
          <el-segmented v-model="readFilter" :options="readFilterOptions" @change="reloadList" />
          <el-select v-model="category" clearable placeholder="全部类型" class="category-filter" @change="reloadList">
            <el-option v-for="option in categoryOptions" :key="option.value" :label="option.label" :value="option.value" />
          </el-select>
        </div>

        <div class="notification-summary">
          <span>共 {{ total }} 条</span>
          <button v-if="unreadCount" type="button" class="mark-all-button" :disabled="markingAll" @click="markAllRead">
            <el-icon><CircleCheck /></el-icon>
            全部已读
          </button>
        </div>

        <el-scrollbar class="notification-scroll" v-loading="loading">
          <div v-if="items.length" class="notification-list">
            <article
              v-for="item in items"
              :key="item.id"
              :class="['notification-item', `severity-${item.severity}`, { 'is-unread': !item.is_read }]"
              @click="openNotification(item)"
            >
              <span class="severity-indicator" aria-hidden="true"></span>
              <div class="notification-copy">
                <div class="notification-title-row">
                  <strong>{{ item.title }}</strong>
                  <span>{{ formatTime(item.created_at) }}</span>
                </div>
                <p>{{ item.content }}</p>
                <div class="notification-meta">
                  <span>{{ categoryLabel(item.category) }}</span>
                  <span v-if="item.workspace_name">{{ item.workspace_name }}</span>
                </div>
              </div>
              <el-tooltip v-if="!item.is_read" content="标记已读" placement="left">
                <button type="button" class="read-button" aria-label="标记已读" @click.stop="markRead(item)">
                  <el-icon><Check /></el-icon>
                </button>
              </el-tooltip>
            </article>
          </div>
          <el-empty v-else-if="!loading" :description="readFilter === 'unread' ? '暂无未读通知' : '暂无通知'" :image-size="96" />
        </el-scrollbar>

        <el-pagination
          v-if="total > pageSize"
          class="notification-pagination"
          layout="prev, pager, next"
          :current-page="page"
          :page-size="pageSize"
          :total="total"
          @current-change="changePage"
        />
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Bell, Check, CircleCheck, Refresh, Setting } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { useRouter } from 'vue-router'
import { notificationApi } from '@/api/notification'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()
const visible = ref(false)
const loading = ref(false)
const markingAll = ref(false)
const preferenceVisible = ref(false)
const preferenceSaving = ref(false)
const unreadCount = ref(0)
const items = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = 20
const readFilter = ref('all')
const category = ref('')
const preferences = reactive({
  collaboration_enabled: true,
  task_enabled: true,
  security_enabled: true,
  share_enabled: true
})
let pollTimer = null

const readFilterOptions = [{ label: '全部', value: 'all' }, { label: '未读', value: 'unread' }]
const categoryOptions = [
  { label: '协作动态', value: 'collaboration' },
  { label: '任务进度', value: 'task' },
  { label: '安全提醒', value: 'security' },
  { label: '外链动态', value: 'share' }
]
const preferenceOptions = [
  { key: 'collaboration_enabled', label: '协作动态', description: '评论提及、目录授权和用户组变更' },
  { key: 'task_enabled', label: '任务进度', description: '批量下载等异步任务完成状态' },
  { key: 'security_enabled', label: '安全提醒', description: '上传扫描失败和风险文件' },
  { key: 'share_enabled', label: '外链动态', description: '外链过期和下载次数状态' }
]
onMounted(() => {
  loadUnreadCount()
  pollTimer = window.setInterval(loadUnreadCount, 60000)
})

onBeforeUnmount(() => {
  if (pollTimer) window.clearInterval(pollTimer)
})

async function openDrawer() {
  visible.value = true
  await Promise.all([loadItems(), loadPreferences(), loadUnreadCount()])
}

async function loadUnreadCount() {
  try {
    const result = await notificationApi.getUnreadCount()
    unreadCount.value = Number(result.data?.unread_count || 0)
  } catch {
    // Background polling remains silent; the drawer refresh exposes errors.
  }
}

async function loadItems() {
  loading.value = true
  try {
    const result = await notificationApi.listUserNotifications({
      page: page.value,
      page_size: pageSize,
      category: category.value || undefined,
      unread_only: readFilter.value === 'unread' || undefined
    })
    items.value = result.data?.list || []
    total.value = Number(result.data?.total || 0)
  } finally {
    loading.value = false
  }
}

async function loadPreferences() {
  const result = await notificationApi.getPreferences()
  Object.assign(preferences, result.data || {})
}

async function refreshAll() {
  await Promise.all([loadItems(), loadUnreadCount()])
}

function reloadList() {
  page.value = 1
  loadItems()
}

function changePage(value) {
  page.value = value
  loadItems()
}

async function markRead(item) {
  if (!item || item.is_read) return
  await notificationApi.markRead(item.id)
  item.is_read = true
  unreadCount.value = Math.max(0, unreadCount.value - 1)
  if (readFilter.value === 'unread') await loadItems()
}

async function markAllRead() {
  markingAll.value = true
  try {
    await notificationApi.markAllRead()
    unreadCount.value = 0
    await loadItems()
    ElMessage.success('通知已全部标记为已读')
  } finally {
    markingAll.value = false
  }
}

async function savePreferences() {
  preferenceSaving.value = true
  try {
    await notificationApi.savePreferences({ ...preferences })
    ElMessage.success('通知偏好已保存')
  } finally {
    preferenceSaving.value = false
  }
}

async function openNotification(item) {
  if (!item) return
  try {
    const result = await notificationApi.openUserNotification(item.id)
    item.is_read = true
    await loadUnreadCount()
    const workspaceID = result.data?.workspace_id
    if (workspaceID && Number(workspaceID) !== Number(userStore.currentWorkspaceId)) {
      await userStore.switchWorkspace(Number(workspaceID))
    }
    visible.value = false
    await router.push(result.data?.path || '/dashboard')
  } catch {
    item.is_read = true
    await Promise.all([loadUnreadCount(), loadItems()])
  }
}

function categoryLabel(value) {
  return categoryOptions.find(option => option.value === value)?.label || '系统通知'
}

function formatTime(value) {
  if (!value) return '-'
  const date = new Date(value)
  const now = new Date()
  if (date.toDateString() === now.toDateString()) {
    return date.toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' })
  }
  return date.toLocaleDateString('zh-CN', { month: '2-digit', day: '2-digit' })
}
</script>

<style scoped>
:global(.user-notification-drawer) {
  isolation: isolate;
  background: var(--panel-bg-strong) !important;
}
:global(.user-notification-drawer .el-drawer__body) {
  position: relative;
  z-index: 1;
  overflow: hidden;
  background: var(--panel-bg-strong) !important;
}
.notification-center { display: inline-flex; align-items: center; }
.notification-badge { display: inline-flex; }
.notification-trigger,
.drawer-icon-button,
.read-button {
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
.notification-trigger { width: 36px; height: 36px; }
.notification-trigger:hover,
.drawer-icon-button:hover,
.read-button:hover { color: var(--accent-primary); border-color: var(--border-hover); background: var(--surface-hover); }
.drawer-icon-button:disabled { cursor: wait; opacity: 0.55; }
.drawer-layout { height: 100%; min-height: 100%; min-width: 0; display: flex; flex-direction: column; background: var(--panel-bg-strong); }
.drawer-header { min-height: 58px; display: flex; align-items: center; justify-content: space-between; gap: 12px; padding-bottom: 14px; border-bottom: 1px solid var(--border-color); }
.drawer-header > div:first-child { min-width: 0; display: flex; flex-direction: column; gap: 4px; }
.drawer-header strong { color: var(--text-primary); font-size: 18px; }
.drawer-header span { color: var(--text-muted); font-size: 12px; }
.drawer-header-actions { display: flex; gap: 8px; }
.drawer-icon-button { width: 34px; height: 34px; }
.preference-panel { padding: 14px 0; border-bottom: 1px solid var(--border-color); }
.preference-heading { display: flex; align-items: center; justify-content: space-between; margin-bottom: 8px; }
.preference-heading > strong { font-size: 14px; color: var(--text-primary); }
.preference-row { min-height: 50px; display: flex; align-items: center; justify-content: space-between; gap: 16px; cursor: pointer; }
.preference-row > span { min-width: 0; display: flex; flex-direction: column; gap: 3px; }
.preference-row strong { font-size: 13px; color: var(--text-secondary); }
.preference-row small { color: var(--text-muted); font-size: 11px; line-height: 1.35; }
.notification-filters { display: grid; grid-template-columns: minmax(150px, 1fr) 148px; gap: 10px; padding: 14px 0 10px; }
.notification-filters :deep(.el-segmented) { width: 100%; }
.category-filter { width: 100%; }
.notification-summary { min-height: 34px; display: flex; align-items: center; justify-content: space-between; gap: 12px; color: var(--text-muted); font-size: 12px; }
.mark-all-button { display: inline-flex; align-items: center; gap: 5px; padding: 5px 0; border: 0; background: transparent; color: var(--accent-primary); cursor: pointer; }
.mark-all-button:disabled { opacity: 0.55; cursor: wait; }
.notification-scroll { flex: 1; min-height: 240px; margin: 0 -20px; background: var(--panel-bg-strong); }
.notification-list { display: flex; flex-direction: column; min-height: 100%; background: var(--panel-bg-strong); }
.notification-item { position: relative; display: grid; grid-template-columns: 5px minmax(0, 1fr) 30px; align-items: start; gap: 12px; padding: 14px 20px; border-bottom: 1px solid var(--border-color); background: var(--panel-bg-strong); cursor: pointer; transition: var(--transition); }
.notification-item:hover { background: var(--surface-hover); }
.notification-item.is-unread { background: color-mix(in srgb, var(--accent-primary) 5%, var(--panel-bg-strong)); }
.severity-indicator { width: 5px; height: 34px; border-radius: 3px; background: var(--text-muted); }
.severity-warning .severity-indicator { background: var(--accent-yellow); }
.severity-critical .severity-indicator { background: var(--accent-red); }
.severity-info .severity-indicator { background: var(--accent-primary); }
.notification-copy { min-width: 0; display: flex; flex-direction: column; gap: 7px; }
.notification-title-row { display: flex; align-items: flex-start; justify-content: space-between; gap: 10px; }
.notification-title-row strong { min-width: 0; overflow-wrap: anywhere; color: var(--text-primary); font-size: 14px; line-height: 1.35; }
.notification-title-row span { flex: 0 0 auto; color: var(--text-muted); font-size: 11px; }
.notification-copy p { margin: 0; color: var(--text-secondary); font-size: 12px; line-height: 1.55; overflow-wrap: anywhere; }
.notification-meta { display: flex; align-items: center; flex-wrap: wrap; gap: 6px 10px; color: var(--text-muted); font-size: 11px; }
.read-button { width: 28px; height: 28px; padding: 0; }
.notification-pagination { flex: 0 0 auto; justify-content: center; padding-top: 12px; border-top: 1px solid var(--border-color); }
@media (max-width: 520px) {
  .notification-filters { grid-template-columns: 1fr; }
  .notification-scroll { margin-inline: -16px; }
  .notification-item { padding-inline: 16px; }
}
</style>
