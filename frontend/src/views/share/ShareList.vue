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
  <section class="share-list-page">
    <header class="page-toolbar share-toolbar">
      <div class="page-toolbar__group share-toolbar__scope">
        <ModeSwitch v-if="canViewWorkspace" v-model="viewScope" :options="scopeOptions" @change="changeScope" />
        <div class="share-summary">
          <strong>{{ total }}</strong>
          <span>{{ viewScope === 'workspace' ? '条全空间外链' : '条我的外链' }}</span>
          <span class="share-summary__hint">外链固定为创建时的文件快照</span>
        </div>
      </div>
      <ActionButton action="refresh" :loading="loading" @click="load" />
    </header>

    <div class="content-panel share-filter-panel">
      <el-form :inline="true" class="share-filter-form" @submit.prevent="applyFilters">
        <el-form-item label="分享名称">
          <el-input v-model.trim="filters.name" clearable placeholder="输入名称关键词" :prefix-icon="Search" class="share-filter-name" @keyup.enter="applyFilters" />
        </el-form-item>
        <el-form-item label="状态">
          <el-select v-model="filters.status" clearable placeholder="全部状态" class="share-filter-status">
            <el-option label="有效" value="active" />
            <el-option label="已撤销" value="revoked" />
            <el-option label="已过期" value="expired" />
            <el-option label="次数用尽" value="exhausted" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="viewScope === 'workspace'" label="创建人">
          <el-input v-model.trim="filters.creator" clearable placeholder="用户名或姓名前缀" class="share-filter-creator" @keyup.enter="applyFilters" />
        </el-form-item>
        <el-form-item label="到期时间">
          <el-date-picker
            v-model="filters.expiryRange"
            type="datetimerange"
            range-separator="至"
            start-placeholder="开始时间"
            end-placeholder="结束时间"
            class="share-filter-expiry"
          />
        </el-form-item>
        <div class="share-filter-actions">
          <ActionButton action="search" @click="applyFilters" />
          <ActionButton action="refresh" text="重置" @click="resetFilters" />
        </div>
      </el-form>
    </div>

    <div class="content-panel share-list-panel">
      <el-table :data="list" v-loading="loading" class="share-management-table" row-key="id" border>
        <el-table-column prop="name" label="分享名称" :min-width="isMobile ? 180 : 210">
          <template #default="scope">
            <div class="share-name-cell">
              <strong>{{ scope.row.name }}</strong>
              <span>{{ scope.row.root_type === 'folder' ? '目录快照' : '文件快照' }} · {{ scope.row.item_count || 0 }} 个文件</span>
              <el-tag v-if="isMobile" :type="statusMeta(scope.row).type" effect="plain" size="small">{{ statusMeta(scope.row).label }}</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="创建信息" min-width="160">
          <template #default="scope">
            <div class="creator-cell">
              <span>{{ scope.row.creator_name }}</span>
              <small v-if="scope.row.creator_username">{{ scope.row.creator_username }}</small>
              <small>{{ formatDate(scope.row.created_at) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="访问控制" min-width="130">
          <template #default="scope">
            <div class="access-cell">
              <el-tag :type="scope.row.requires_password ? 'warning' : 'info'" effect="plain" size="small">
                {{ scope.row.requires_password ? '密码保护' : '免密码' }}
              </el-tag>
              <small>下载 {{ downloadCountLabel(scope.row) }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="有效期" min-width="180">
          <template #default="scope">
            <div class="expiry-cell">
              <span>{{ formatDate(scope.row.expires_at) }}</span>
              <small :class="{ 'expiry-cell__warning': remainingTime(scope.row).warning }">{{ remainingTime(scope.row).label }}</small>
            </div>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="状态" width="96">
          <template #default="scope"><el-tag :type="statusMeta(scope.row).type" effect="plain">{{ statusMeta(scope.row).label }}</el-tag></template>
        </el-table-column>
        <el-table-column label="操作" :width="isMobile ? 170 : 178" fixed="right" align="center" class-name="common-operation-column" header-class-name="common-operation-column">
          <template #default="scope">
            <div class="common-action-group">
              <ActionButton action="view" text="详情" @click="openDetail(scope.row)" />
              <ActionButton v-if="scope.row.can_revoke && statusMeta(scope.row).value === 'active'" action="revoke" @click="revoke(scope.row)" />
            </div>
          </template>
        </el-table-column>
        <template #empty><el-empty :description="hasFilters ? '没有符合筛选条件的外链' : '暂无外链分享'" /></template>
      </el-table>
      <el-pagination
        v-if="total > 0"
        class="table-pagination"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        :total="total"
        layout="total, sizes, prev, pager, next"
        @current-change="changePage"
        @size-change="changePageSize"
      />
    </div>

    <el-drawer v-model="drawerVisible" append-to-body size="min(620px, 92vw)" title="外链详情" destroy-on-close>
      <div v-loading="detailLoading" class="share-detail">
        <template v-if="detail">
          <header class="share-detail__header">
            <div><span class="share-detail__eyebrow">{{ detail.root_type === 'folder' ? '目录快照' : '文件快照' }}</span><h3>{{ detail.name }}</h3></div>
            <el-tag :type="statusMeta(detail).type" effect="plain">{{ statusMeta(detail).label }}</el-tag>
          </header>

          <el-descriptions :column="2" border class="share-detail__facts">
            <el-descriptions-item label="快照文件">{{ detail.item_count }} 个</el-descriptions-item>
            <el-descriptions-item label="下载次数">{{ downloadCountLabel(detail) }}</el-descriptions-item>
            <el-descriptions-item label="创建人">{{ detail.creator_name }}<span v-if="detail.creator_username">（{{ detail.creator_username }}）</span></el-descriptions-item>
            <el-descriptions-item label="访问密码">{{ detail.requires_password ? '已设置' : '未设置' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatDate(detail.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="有效期至">{{ formatDate(detail.expires_at) }}</el-descriptions-item>
            <el-descriptions-item label="剩余有效期" :span="2">{{ remainingTime(detail).label }}</el-descriptions-item>
            <el-descriptions-item v-if="detail.revoked_at" label="撤销时间" :span="2">{{ formatDate(detail.revoked_at) }}</el-descriptions-item>
          </el-descriptions>

          <el-alert
            v-if="!detail.can_locate_source"
            title="源文件当前不可定位"
            description="源文件可能已删除、移入回收站，或你已不再拥有完整路径的读取权限。为避免泄露目录结构，系统不会展示源路径。"
            type="info"
            :closable="false"
            show-icon
          />

          <section class="share-files-preview">
            <div class="share-files-preview__title"><strong>快照内容</strong><span>最多展示前 20 个文件</span></div>
            <el-table :data="detail.items || []" size="small" max-height="300">
              <el-table-column prop="relative_path" label="文件" min-width="280" show-overflow-tooltip />
              <el-table-column label="大小" width="100"><template #default="scope">{{ formatBytes(scope.row.size) }}</template></el-table-column>
              <template #empty><el-empty description="没有可展示的快照文件" :image-size="64" /></template>
            </el-table>
          </section>
        </template>
      </div>
      <template #footer>
        <div class="share-detail__actions">
          <el-button @click="drawerVisible = false">关闭</el-button>
          <ActionButton v-if="detail?.can_locate_source" action="enter" text="定位源文件" @click="locateSource" />
          <ActionButton v-if="detail?.can_revoke && statusMeta(detail).value === 'active'" action="revoke" @click="revoke(detail)" />
        </div>
      </template>
    </el-drawer>
  </section>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getShareDetail, listShares, revokeShare } from '@/api/files'
import { usePagination } from '@/composables/usePagination'
import ActionButton from '@/components/common/ActionButton.vue'
import ModeSwitch from '@/components/common/ModeSwitch.vue'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const userStore = useUserStore()
const mobileMedia = window.matchMedia('(max-width: 760px)')
const isMobile = ref(mobileMedia.matches)
const viewScope = ref('mine')
const scopeOptions = [
  { label: '我的分享', value: 'mine', action: 'myShares' },
  { label: '全部分享', value: 'workspace', action: 'allShares' }
]
const filters = reactive({ name: '', status: '', creator: '', expiryRange: [] })
const currentWorkspace = computed(() => userStore.workspaces.find(item => item.id === userStore.currentWorkspaceId))
const canViewWorkspace = computed(() => Boolean(userStore.user?.is_super_admin || currentWorkspace.value?.current_role === 'workspace_admin'))
const hasFilters = computed(() => Boolean(filters.name || filters.status || filters.creator || filters.expiryRange?.length))

function requestFilters() {
  const params = { scope: viewScope.value }
  if (filters.name) params.name = filters.name
  if (filters.status) params.status = filters.status
  if (viewScope.value === 'workspace' && filters.creator) params.creator = filters.creator
  if (filters.expiryRange?.length === 2) {
    params.expires_from = new Date(filters.expiryRange[0]).toISOString()
    params.expires_to = new Date(filters.expiryRange[1]).toISOString()
  }
  return params
}

const fetchShares = (params) => listShares({ ...params, ...requestFilters() })
const { list, loading, page, pageSize, total, load } = usePagination(fetchShares, { pageSize: 20 })
const drawerVisible = ref(false)
const detailLoading = ref(false)
const detail = ref(null)

async function openDetail(row) {
  drawerVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const result = await getShareDetail(row.id)
    detail.value = result.data
  } finally {
    detailLoading.value = false
  }
}

async function revoke(row) {
  await ElMessageBox.confirm(`确定撤销“${row.name}”的分享链接？撤销后外部访问者将立即无法继续下载。`, '撤销分享', { type: 'warning', confirmButtonText: '确认撤销' })
  await revokeShare(row.id)
  ElMessage.success('分享已撤销')
  drawerVisible.value = false
  detail.value = null
  await load()
}

async function locateSource() {
  const location = detail.value?.source_location
  if (!location) return
  await router.push({ name: 'Files', query: { locateShare: detail.value.id } })
}

function applyFilters() {
  page.value = 1
  load()
}

function resetFilters() {
  Object.assign(filters, { name: '', status: '', creator: '', expiryRange: [] })
  page.value = 1
  load()
}

function changeScope() {
  filters.creator = ''
  page.value = 1
  load()
}

function changePage(value) {
  page.value = value
  load()
}

function changePageSize(value) {
  pageSize.value = value
  page.value = 1
  load()
}

function formatDate(value) {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-'
}

function formatBytes(value = 0) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

function downloadCountLabel(row = {}) {
  return `${row.download_count || 0}${row.max_downloads ? ` / ${row.max_downloads}` : ' / 不限'}`
}

function remainingTime(row = {}) {
  const status = statusMeta(row).value
  if (status === 'revoked') return { label: '已撤销', warning: false }
  if (status === 'expired') return { label: '已过期', warning: true }
  if (status === 'exhausted') return { label: '下载次数已用尽', warning: true }
  const milliseconds = new Date(row.expires_at).getTime() - Date.now()
  if (!Number.isFinite(milliseconds) || milliseconds <= 0) return { label: '已过期', warning: true }
  const minutes = Math.max(1, Math.floor(milliseconds / 60000))
  const days = Math.floor(minutes / 1440)
  const hours = Math.floor((minutes % 1440) / 60)
  const label = days > 0 ? `剩余 ${days} 天${hours ? ` ${hours} 小时` : ''}` : hours > 0 ? `剩余 ${hours} 小时` : `剩余 ${minutes} 分钟`
  return { label, warning: minutes <= 1440 }
}

function statusMeta(row = {}) {
  let value = row.effective_status || row.status || 'active'
  if (value === 'active' && row.max_downloads && row.download_count >= row.max_downloads) value = 'exhausted'
  if (value === 'active' && row.expires_at && new Date(row.expires_at) <= new Date()) value = 'expired'
  return {
    active: { value, label: '有效', type: 'success' },
    revoked: { value, label: '已撤销', type: 'danger' },
    expired: { value, label: '已过期', type: 'info' },
    exhausted: { value, label: '次数用尽', type: 'warning' }
  }[value] || { value, label: value, type: 'info' }
}

watch(() => userStore.currentWorkspaceId, () => {
  viewScope.value = 'mine'
  Object.assign(filters, { name: '', status: '', creator: '', expiryRange: [] })
  page.value = 1
  load()
})

function syncMobileLayout(event) {
  isMobile.value = event.matches
}

onMounted(() => {
  mobileMedia.addEventListener('change', syncMobileLayout)
  load()
})
onBeforeUnmount(() => mobileMedia.removeEventListener('change', syncMobileLayout))
</script>

<style scoped>
.share-list-page { display: flex; flex: 1; flex-direction: column; min-height: 0; gap: 12px; padding: 0 0 8px; }
.share-toolbar__scope { min-width: 0; }
.share-summary { display: flex; align-items: baseline; min-width: 0; gap: 6px; color: var(--text-secondary); font-size: 13px; }
.share-summary strong { color: var(--text-primary); font-size: 18px; }
.share-summary__hint { margin-left: 8px; padding-left: 14px; border-left: 1px solid var(--border-color); color: var(--text-tertiary); }
.share-filter-panel { flex: 0 0 auto; padding: 13px 16px 3px; box-shadow: none !important; }
.share-filter-form { display: flex; align-items: center; flex-wrap: wrap; gap: 0 14px; }
.share-filter-form :deep(.el-form-item) { margin-right: 0; margin-bottom: 10px; }
.share-filter-name { width: 210px; }
.share-filter-status { width: 130px; }
.share-filter-creator { width: 190px; }
.share-filter-expiry { width: 350px; }
.share-filter-actions { display: flex; align-items: center; gap: 8px; margin-bottom: 10px; }
.share-list-panel { display: flex; flex: 1; flex-direction: column; min-height: 280px; }
.share-management-table { flex: 1; min-height: 260px; border: 0; border-radius: 0 !important; }
.share-list-panel .table-pagination { margin-top: 0; padding: 12px 14px; border-top: 1px solid var(--border-color); }
.share-name-cell, .creator-cell, .access-cell, .expiry-cell { display: flex; flex-direction: column; align-items: center; gap: 3px; min-width: 0; }
.share-name-cell strong { max-width: 100%; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.share-name-cell span, .creator-cell small, .access-cell small, .expiry-cell small { color: var(--text-secondary); font-size: 12px; }
.expiry-cell__warning { color: var(--el-color-warning) !important; font-weight: 600; }
.share-detail { min-height: 260px; }
.share-detail__header { display: flex; align-items: flex-start; justify-content: space-between; gap: 16px; margin-bottom: 18px; }
.share-detail__header h3 { margin: 4px 0 0; color: var(--text-primary); font-size: 20px; }
.share-detail__eyebrow { color: var(--text-secondary); font-size: 12px; }
.share-detail__facts { margin-bottom: 16px; }
.share-files-preview { margin-top: 20px; }
.share-files-preview__title { display: flex; align-items: baseline; justify-content: space-between; margin-bottom: 10px; }
.share-files-preview__title span { color: var(--text-secondary); font-size: 12px; }
.share-detail__actions { display: flex; justify-content: flex-end; gap: 8px; }
@media (max-width: 900px) { .share-filter-expiry { width: 300px; } }
@media (max-width: 760px) {
  .share-toolbar { align-items: flex-start; }
  .share-toolbar__scope { align-items: flex-start; flex-direction: column; }
  .share-summary__hint { display: none; }
  .share-filter-form { align-items: stretch; flex-direction: column; }
  .share-filter-name, .share-filter-status, .share-filter-creator, .share-filter-expiry { width: 100%; }
  .share-filter-form :deep(.el-form-item) { display: block; width: 100%; }
  .share-filter-form :deep(.el-form-item__label) { display: block; width: auto; height: 24px; padding: 0; line-height: 24px; text-align: left; }
  .share-filter-form :deep(.el-form-item__content) { display: block; width: 100%; min-width: 0; }
  .share-filter-form :deep(.share-filter-expiry) { width: 100% !important; min-width: 0; max-width: 100%; }
}
</style>
