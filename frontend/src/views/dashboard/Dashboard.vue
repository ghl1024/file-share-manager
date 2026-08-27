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
  <div class="dashboard-container" v-loading="loading">
    <section class="workspace-overview">
      <div class="workspace-overview__copy">
        <div class="workspace-eyebrow-row">
          <span class="workspace-eyebrow">当前工作空间</span>
          <el-tag v-if="currentWorkspace" :type="workspaceRoleType" effect="plain" size="small">{{ workspaceRoleLabel }}</el-tag>
          <el-tag v-if="stats.workspace_count > 1" type="info" effect="plain" size="small">可访问 {{ stats.workspace_count }} 个空间</el-tag>
        </div>
        <div class="workspace-title-row">
          <h2>{{ currentWorkspace?.name || '尚未选择工作空间' }}</h2>
          <el-tag v-if="currentWorkspace" type="success" effect="plain" size="small">服务正常</el-tag>
        </div>
        <p>{{ overviewDescription }}</p>
      </div>
      <div class="workspace-controls">
        <ModeSwitch v-if="canRequestWorkspaceScope" v-model="dashboardScope" :options="scopeOptions" @change="loadStats" />
        <ActionButton action="enter" :text="userStore.currentWorkspaceId ? '切换空间' : '选择空间'" @click="router.push('/workspaces')" />
      </div>
    </section>

    <el-alert
      v-if="quotaWarning"
      class="quota-alert"
      :title="quotaWarning"
      type="warning"
      :closable="false"
      show-icon
    />

    <div class="stats-grid">
      <el-card shadow="never" class="stat-card stat-card--blue">
        <div class="stat-top"><span class="stat-icon"><el-icon><Document /></el-icon></span><span class="stat-trend">{{ stats.folder_count }} 个目录</span></div>
        <div class="stat-label">{{ dashboardScope === 'workspace' ? '全空间文件' : '可访问文件' }}</div>
        <div class="stat-value">{{ stats.file_count }}</div>
        <div class="stat-desc">{{ stats.can_browse_files ? '统计会实时应用当前目录权限' : '当前账号没有文件浏览权限' }}</div>
      </el-card>

      <el-card shadow="never" class="stat-card stat-card--green">
        <div class="stat-top"><span class="stat-icon"><el-icon><DataLine /></el-icon></span><span class="stat-trend">{{ quotaPercentLabel }}</span></div>
        <div class="stat-label">{{ dashboardScope === 'workspace' ? '空间已用容量' : '个人已用容量' }}</div>
        <div class="stat-value storage-value">{{ formatBytes(stats.used_bytes) }}</div>
        <el-progress class="quota-progress" :percentage="quotaPercent" :show-text="false" :stroke-width="5" :status="quotaPercent >= 90 ? 'exception' : undefined" />
        <div class="stat-desc">{{ quotaDescription }}</div>
      </el-card>

      <el-card shadow="never" class="stat-card stat-card--amber">
        <div class="stat-top"><span class="stat-icon"><el-icon><Share /></el-icon></span><span class="stat-trend">我的分享 {{ stats.my_active_share_count }}</span></div>
        <div class="stat-label">{{ dashboardScope === 'workspace' ? '全空间有效分享' : '我的有效分享' }}</div>
        <div class="stat-value">{{ stats.active_share_count }}</div>
        <div class="stat-desc">已撤销、过期或耗尽次数的外链不计入</div>
      </el-card>

      <el-card shadow="never" class="stat-card stat-card--violet">
        <div class="stat-top"><span class="stat-icon"><el-icon><Clock /></el-icon></span><span class="stat-trend">{{ taskTrend }}</span></div>
        <div class="stat-label">我的进行中任务</div>
        <div class="stat-value">{{ inProgressTaskCount }}</div>
        <div class="stat-desc">上传 {{ stats.tasks.upload_in_progress }} 个，下载 {{ stats.tasks.download_in_progress }} 个</div>
      </el-card>
    </div>

    <section v-if="userStore.currentWorkspaceId" class="personal-overview">
      <div class="summary-panel">
        <div class="summary-panel__header">
          <div><h3>最近使用</h3><p>继续处理最近访问的文件和目录</p></div>
          <ActionButton action="recent" text="查看全部" @click="goToFileView('recent')" />
        </div>
        <div v-if="stats.recent_nodes.length" class="summary-list">
          <button v-for="item in stats.recent_nodes" :key="item.id" type="button" class="summary-item" @click="goToFileView('recent')">
            <span class="summary-item__icon"><el-icon><Folder v-if="item.type === 'folder'" /><Document v-else /></el-icon></span>
            <span class="summary-item__body"><strong>{{ item.name }}</strong><small>{{ formatDate(item.activity_at) }}</small></span>
            <el-icon class="summary-item__arrow"><ArrowRight /></el-icon>
          </button>
        </div>
        <el-empty v-else :image-size="62" description="暂无最近使用记录" />
      </div>

      <div class="summary-panel">
        <div class="summary-panel__header">
          <div><h3>我的收藏</h3><p>{{ stats.favorite_count }} 个仍有权限访问的收藏</p></div>
          <ActionButton action="favorites" text="查看全部" @click="goToFileView('favorites')" />
        </div>
        <div v-if="stats.favorite_nodes.length" class="summary-list">
          <button v-for="item in stats.favorite_nodes" :key="item.id" type="button" class="summary-item" @click="goToFileView('favorites')">
            <span class="summary-item__icon summary-item__icon--favorite"><el-icon><Star /></el-icon></span>
            <span class="summary-item__body"><strong>{{ item.name }}</strong><small>{{ item.type === 'folder' ? '目录' : '文件' }} · 更新于 {{ formatDate(item.updated_at) }}</small></span>
            <el-icon class="summary-item__arrow"><ArrowRight /></el-icon>
          </button>
        </div>
        <el-empty v-else :image-size="62" description="暂无可访问的收藏" />
      </div>
    </section>

    <section v-if="userStore.currentWorkspaceId" class="quick-actions">
      <div class="quick-actions__heading">
        <h3>快捷操作</h3>
        <p>{{ dashboardScope === 'workspace' && stats.active_user_count ? `${stats.active_user_count} 位活跃成员正在使用当前空间` : '进入最常用的文件协作流程' }}</p>
      </div>
      <div class="action-grid">
        <button v-permission="'file:list'" type="button" class="action-tile" @click="router.push('/files')">
          <span class="action-tile__icon"><el-icon><Folder /></el-icon></span><span><strong>浏览文件</strong><small>查看目录和文件详情</small></span><el-icon class="action-arrow"><ArrowRight /></el-icon>
        </button>
        <button v-permission="'file:upload'" type="button" class="action-tile action-tile--upload" @click="router.push('/files')">
          <span class="action-tile__icon"><el-icon><Upload /></el-icon></span><span><strong>上传文件</strong><small>添加文件或创建目录</small></span><el-icon class="action-arrow"><ArrowRight /></el-icon>
        </button>
        <button v-permission="'file:download'" type="button" class="action-tile action-tile--tasks" @click="goToDownloadTasks">
          <span class="action-tile__icon"><el-icon><Clock /></el-icon></span><span><strong>下载任务</strong><small>{{ stats.tasks.download_ready }} 个任务可下载</small></span><el-icon class="action-arrow"><ArrowRight /></el-icon>
        </button>
        <button v-permission="'file:share:create'" type="button" class="action-tile action-tile--share" @click="router.push('/shares')">
          <span class="action-tile__icon"><el-icon><Share /></el-icon></span><span><strong>我的分享</strong><small>{{ stats.my_active_share_count }} 个外链仍然有效</small></span><el-icon class="action-arrow"><ArrowRight /></el-icon>
        </button>
      </div>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { ArrowRight, Clock, DataLine, Document, Folder, Share, Star, Upload } from '@element-plus/icons-vue'
import { getDashboardStats } from '@/api/dashboard'
import ActionButton from '@/components/common/ActionButton.vue'
import ModeSwitch from '@/components/common/ModeSwitch.vue'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()
const loading = ref(false)
const dashboardScope = ref('mine')
const scopeOptions = [
  { label: '我的数据', value: 'mine', action: 'members', icon: Star },
  { label: '全空间', value: 'workspace', action: 'dashboard', icon: DataLine }
]
const emptyTasks = () => ({ upload_in_progress: 0, upload_failed: 0, download_in_progress: 0, download_ready: 0, download_failed: 0 })
const stats = reactive({
  view_scope: 'mine', can_view_workspace: false, can_browse_files: false,
  workspace_count: 0, file_count: 0, folder_count: 0, active_user_count: 0,
  active_share_count: 0, my_active_share_count: 0, favorite_count: 0,
  used_bytes: 0, reserved_bytes: 0, quota_bytes: null, quota_source: 'unlimited',
  tasks: emptyTasks(), recent_nodes: [], favorite_nodes: []
})

const currentWorkspace = computed(() => userStore.workspaces.find(item => item.id === userStore.currentWorkspaceId))
const canRequestWorkspaceScope = computed(() => Boolean(userStore.user?.is_super_admin || currentWorkspace.value?.current_role === 'workspace_admin'))
const workspaceRoleLabel = computed(() => userStore.user?.is_super_admin ? '超级管理员' : currentWorkspace.value?.current_role === 'workspace_admin' ? '空间管理员' : '空间成员')
const workspaceRoleType = computed(() => canRequestWorkspaceScope.value ? 'warning' : 'info')
const overviewDescription = computed(() => {
  if (!currentWorkspace.value) return '选择工作空间后即可查看个人文件和协作数据'
  return dashboardScope.value === 'workspace' ? '正在查看全空间汇总，个人任务和最近使用仍只属于当前账号' : '只展示当前账号有权访问的文件、个人配额和本人创建的分享'
})
const quotaPercent = computed(() => {
  if (!stats.quota_bytes) return 0
  return Math.min(100, Math.round(((Number(stats.used_bytes) + Number(stats.reserved_bytes)) / Number(stats.quota_bytes)) * 100))
})
const quotaPercentLabel = computed(() => stats.quota_bytes ? `占用 ${quotaPercent.value}%` : stats.quota_source === 'workspace_shared' ? '跟随空间配额' : '未限制')
const quotaDescription = computed(() => {
  const reserved = stats.reserved_bytes ? `，上传预留 ${formatBytes(stats.reserved_bytes)}` : ''
  if (stats.quota_bytes) return `上限 ${formatBytes(stats.quota_bytes)}${reserved}`
  if (stats.quota_source === 'workspace_shared') return `未设置个人上限，仍受空间总配额约束${reserved}`
  if (stats.quota_source === 'unavailable') return '当前账号不是该空间的直接成员'
  return `当前未设置容量上限${reserved}`
})
const quotaWarning = computed(() => {
  if (!stats.quota_bytes || quotaPercent.value < 80) return ''
  return quotaPercent.value >= 100 ? '容量已达到上限，请清理文件或联系空间管理员调整配额。' : `容量已使用 ${quotaPercent.value}%，建议及时清理文件或申请调整配额。`
})
const inProgressTaskCount = computed(() => Number(stats.tasks.upload_in_progress) + Number(stats.tasks.download_in_progress))
const taskTrend = computed(() => {
  const failed = Number(stats.tasks.upload_failed) + Number(stats.tasks.download_failed)
  if (failed) return `${failed} 个失败待处理`
  if (stats.tasks.download_ready) return `${stats.tasks.download_ready} 个可下载`
  return '任务状态正常'
})

async function loadStats() {
  stats.workspace_count = userStore.workspaces.length
  if (!userStore.currentWorkspaceId) return
  if (dashboardScope.value === 'workspace' && !canRequestWorkspaceScope.value) dashboardScope.value = 'mine'
  loading.value = true
  try {
    const res = await getDashboardStats(dashboardScope.value)
    Object.assign(stats, res.data, {
      tasks: { ...emptyTasks(), ...(res.data?.tasks || {}) },
      recent_nodes: res.data?.recent_nodes || [],
      favorite_nodes: res.data?.favorite_nodes || []
    })
  } catch {
    Object.assign(stats, {
      file_count: 0, folder_count: 0, active_user_count: 0, active_share_count: 0,
      my_active_share_count: 0, favorite_count: 0, used_bytes: 0, reserved_bytes: 0,
      quota_bytes: null, quota_source: 'unlimited', tasks: emptyTasks(), recent_nodes: [], favorite_nodes: []
    })
  } finally {
    loading.value = false
  }
}

function goToFileView(view) {
  router.push({ name: 'Files', query: { view } })
}

function goToDownloadTasks() {
  router.push({ name: 'Files', query: { panel: 'downloads' } })
}

function formatBytes(bytes) {
  const value = Number(bytes || 0)
  if (value < 1024) return `${value} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let size = value / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[index]}`
}

function formatDate(value) {
  if (!value) return '时间未知'
  return new Date(value).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', hour12: false })
}

watch(() => userStore.currentWorkspaceId, () => {
  dashboardScope.value = 'mine'
  loadStats()
})
onMounted(loadStats)
</script>

<style scoped>
.dashboard-container { display: flex; flex-direction: column; gap: 14px; }
.workspace-overview { display: flex; align-items: center; justify-content: space-between; gap: 24px; padding: 18px 20px; border: 1px solid var(--border-color); border-left: 3px solid var(--accent-primary); border-radius: var(--radius); background: var(--panel-bg-strong); }
.workspace-overview__copy { min-width: 0; }
.workspace-eyebrow-row, .workspace-title-row, .workspace-controls { display: flex; align-items: center; gap: 9px; }
.workspace-eyebrow { color: var(--text-muted); font-size: 11px; font-weight: 700; }
.workspace-title-row { margin-top: 5px; }
.workspace-title-row h2 { min-width: 0; margin: 0; overflow: hidden; color: var(--text-primary); font-size: 20px; text-overflow: ellipsis; white-space: nowrap; }
.workspace-overview__copy p { margin: 5px 0 0; color: var(--text-secondary); font-size: 12px; }
.workspace-controls { flex: 0 0 auto; }
.quota-alert { border: 1px solid color-mix(in srgb, #d97706 24%, transparent); }
.stats-grid { display: grid; grid-template-columns: repeat(4, minmax(0, 1fr)); gap: 12px; }
.stat-card { min-width: 0; border-top: 2px solid color-mix(in srgb, var(--accent-primary) 55%, transparent); }
.stat-card--green { border-top-color: color-mix(in srgb, #059669 55%, transparent); }
.stat-card--amber { border-top-color: color-mix(in srgb, #d97706 55%, transparent); }
.stat-card--violet { border-top-color: color-mix(in srgb, #7c3aed 55%, transparent); }
.stat-card :deep(.el-card__body) { padding: 16px 17px; }
.stat-top { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.stat-icon { width: 34px; height: 34px; display: inline-flex; align-items: center; justify-content: center; border-radius: 7px; background: color-mix(in srgb, currentColor 11%, transparent); color: var(--accent-primary); font-size: 17px; }
.stat-card--green .stat-icon { color: #059669; }
.stat-card--amber .stat-icon { color: #d97706; }
.stat-card--violet .stat-icon { color: #7c3aed; }
.stat-trend { overflow: hidden; color: var(--text-muted); font-size: 11px; text-overflow: ellipsis; white-space: nowrap; }
.stat-label { margin-top: 12px; color: var(--text-secondary); font-size: 12px; font-weight: 600; }
.stat-value { margin: 4px 0 6px; color: var(--text-primary); font-size: 26px; font-weight: 700; }
.storage-value { font-size: 21px; }
.stat-desc { min-height: 18px; overflow-wrap: anywhere; color: var(--text-muted); font-size: 11px; line-height: 1.5; }
.quota-progress { margin: 3px 0 7px; }
.personal-overview { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; }
.summary-panel { min-width: 0; padding: 16px 17px; border: 1px solid var(--border-color); border-radius: var(--radius); background: var(--panel-bg-strong); }
.summary-panel__header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding-bottom: 12px; border-bottom: 1px solid var(--border-color); }
.summary-panel__header h3, .quick-actions__heading h3 { margin: 0; color: var(--text-primary); font-size: 15px; }
.summary-panel__header p, .quick-actions__heading p { margin: 4px 0 0; color: var(--text-muted); font-size: 11px; }
.summary-list { display: grid; gap: 2px; padding-top: 6px; }
.summary-item { width: 100%; min-width: 0; min-height: 46px; display: grid; grid-template-columns: 30px minmax(0, 1fr) 16px; align-items: center; gap: 9px; padding: 6px 7px; border: 0; border-radius: 6px; background: transparent; color: var(--text-primary); text-align: left; cursor: pointer; transition: var(--transition); }
.summary-item:hover { background: var(--surface-hover); }
.summary-item__icon { width: 30px; height: 30px; display: inline-flex; align-items: center; justify-content: center; border-radius: 6px; background: color-mix(in srgb, var(--accent-primary) 10%, transparent); color: var(--accent-primary); }
.summary-item__icon--favorite { background: color-mix(in srgb, #d97706 10%, transparent); color: #d97706; }
.summary-item__body, .summary-item__body strong, .summary-item__body small { display: block; min-width: 0; }
.summary-item__body strong { overflow: hidden; font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.summary-item__body small { margin-top: 3px; overflow: hidden; color: var(--text-muted); font-size: 10px; text-overflow: ellipsis; white-space: nowrap; }
.summary-item__arrow { color: var(--text-muted); }
.summary-panel :deep(.el-empty) { padding: 20px 0 6px; }
.quick-actions { display: flex; align-items: center; justify-content: space-between; gap: 20px; padding: 16px 18px; border: 1px solid var(--border-color); border-radius: var(--radius); background: var(--panel-bg-strong); }
.quick-actions__heading { flex: 0 0 190px; }
.action-grid { display: grid; grid-template-columns: repeat(4, minmax(150px, 1fr)); gap: 8px; flex: 1; }
.action-tile { min-width: 0; min-height: 60px; display: grid; grid-template-columns: 32px minmax(0, 1fr) 14px; align-items: center; gap: 9px; padding: 9px 10px; border: 1px solid var(--border-color); border-radius: var(--radius); background: var(--surface-soft); color: var(--text-primary); text-align: left; cursor: pointer; transition: var(--transition); }
.action-tile:hover { border-color: var(--border-hover); background: var(--surface-hover); }
.action-tile__icon { width: 32px; height: 32px; display: inline-flex; align-items: center; justify-content: center; border-radius: 7px; background: var(--panel-bg-strong); color: var(--accent-primary); }
.action-tile--upload .action-tile__icon { color: #059669; }
.action-tile--tasks .action-tile__icon { color: #7c3aed; }
.action-tile--share .action-tile__icon { color: #d97706; }
.action-tile strong, .action-tile small { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.action-tile strong { font-size: 12px; }
.action-tile small { margin-top: 3px; color: var(--text-muted); font-size: 10px; }
.action-arrow { color: var(--text-muted); }
@media (max-width: 1200px) { .stats-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .quick-actions { align-items: stretch; flex-direction: column; } .quick-actions__heading { flex-basis: auto; } .action-grid { width: 100%; } }
@media (max-width: 800px) { .personal-overview { grid-template-columns: 1fr; } .action-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } .workspace-overview { align-items: flex-start; flex-direction: column; } .workspace-controls { width: 100%; flex-wrap: wrap; } }
@media (max-width: 640px) { .stats-grid, .action-grid { grid-template-columns: 1fr; } .workspace-title-row h2 { white-space: normal; } .summary-panel__header { align-items: flex-start; } }
</style>
