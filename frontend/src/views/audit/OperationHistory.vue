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
  <div class="page-container">
    <!-- 搜索栏 -->
    <el-card class="search-card audit-search-card" shadow="never">
      <el-form :inline="true" class="search-form" @submit.prevent="applyFilters">
        <div class="audit-filter-main">
          <el-form-item label="操作人">
            <el-input v-model="filters.username" placeholder="用户名" :prefix-icon="Search" clearable class="toolbar-input" @keyup.enter="applyFilters" />
          </el-form-item>
          <el-form-item label="事件类型">
            <el-select v-model="filters.category" placeholder="全部类型" clearable class="toolbar-select" @change="handleCategoryChange">
              <el-option label="操作" value="operation" />
              <el-option label="访问" value="access" />
              <el-option label="安全" value="security" />
            </el-select>
          </el-form-item>
          <el-form-item label="结果">
            <el-select v-model="filters.result" placeholder="全部结果" clearable class="toolbar-select">
              <el-option label="成功" value="success" />
              <el-option label="失败" value="failure" />
              <el-option label="拒绝" value="denied" />
            </el-select>
          </el-form-item>
          <div class="toolbar-actions audit-query-actions">
            <ActionButton action="search" @click="applyFilters" />
            <ActionButton action="refresh" text="重置" @click="resetFilters" />
            <ActionButton action="more" :text="advancedVisible ? '收起筛选' : '更多筛选'" @click="advancedVisible = !advancedVisible" />
          </div>
        </div>

        <div v-show="advancedVisible" class="audit-filter-advanced">
          <el-form-item label="风险级别">
            <el-select v-model="filters.severity" placeholder="全部级别" clearable class="toolbar-select">
              <el-option label="普通" value="info" />
              <el-option label="警告" value="warning" />
              <el-option label="高风险" value="high" />
            </el-select>
          </el-form-item>
          <el-form-item label="主体类型">
            <el-select v-model="filters.actor_type" placeholder="全部主体" clearable class="toolbar-select">
              <el-option label="登录用户" value="user" />
              <el-option label="系统任务" value="system" />
              <el-option label="外链访客" value="external_share" />
            </el-select>
          </el-form-item>
          <el-form-item label="请求方法">
            <el-select v-model="filters.method" placeholder="全部方法" clearable class="toolbar-select">
              <el-option v-for="item in ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']" :key="item" :label="item" :value="item" />
            </el-select>
          </el-form-item>
          <el-form-item label="目标类型"><el-input v-model="filters.target_type" placeholder="如 file" clearable class="toolbar-input" /></el-form-item>
          <el-form-item label="目标 ID"><el-input v-model="filters.target_id" placeholder="精确匹配" clearable class="toolbar-input" /></el-form-item>
          <el-form-item label="客户端 IP"><el-input v-model="filters.ip" placeholder="精确匹配" clearable class="toolbar-input" /></el-form-item>
          <el-form-item label="请求 ID"><el-input v-model="filters.request_id" placeholder="精确匹配" clearable class="request-id-input" /></el-form-item>
          <el-form-item label="发生时间">
            <el-date-picker v-model="timeRange" type="datetimerange" range-separator="至" start-placeholder="开始时间" end-placeholder="结束时间" class="time-range-picker" />
          </el-form-item>
        </div>

        <div class="audit-toolbar-footer">
          <div class="filter-summary">
            <span>共 {{ total }} 条事件</span>
            <span v-if="policy.hot_retention_days">热数据策略 {{ policy.hot_retention_days }} 天</span>
            <span v-if="policy.export_retention_hours">导出保留 {{ policy.export_retention_hours }} 小时</span>
            <span>归档清理 {{ policy.archive_enabled ? '已启用' : '未启用' }}</span>
          </div>
          <div class="toolbar-actions">
            <ActionButton v-permission="'audit:export'" action="download" text="导出当前筛选" :loading="exportLoading" @click="exportLogs" />
            <ActionButton v-permission="'audit:export'" action="exportHistory" @click="openExportJobs" />
            <ActionButton action="archive" @click="openArchives" />
            <ActionButton action="verify" text="校验审计链" :loading="verifyLoading" @click="verifyChain" />
          </div>
        </div>
      </el-form>
    </el-card>

    <!-- 列表栏 -->
    <el-card class="table-card audit-table-card" shadow="never">
      <div class="audit-view-tabs">
        <el-tabs v-model="activeAuditView" @tab-change="handleAuditViewChange">
          <el-tab-pane label="全部事件" name="all" />
          <el-tab-pane label="操作审计" name="operation" />
          <el-tab-pane label="文件访问" name="access" />
          <el-tab-pane label="安全事件" name="security" />
        </el-tabs>
      </div>
      <div class="audit-table-heading">
        <div><strong>{{ auditViewTitle }}</strong><small>{{ auditViewDescription }}</small></div>
      </div>
      <el-table :data="list" v-loading="loading" stripe border>
        <el-table-column prop="stream_seq" label="流序号" width="74">
          <template #default="{ row }">#{{ row.stream_seq || '-' }}</template>
        </el-table-column>

        <el-table-column label="操作主体" width="112">
          <template #default="{ row }">
            <div class="actor-cell"><span>{{ row.username || '-' }}</span><small>{{ actorTypeLabel(row.actor_type) }}</small></div>
          </template>
        </el-table-column>

        <el-table-column prop="category" label="类型" width="68">
          <template #default="{ row }">
            <el-tag size="small" effect="plain">{{ categoryLabel(row.category) }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column prop="result" label="结果" width="68">
          <template #default="{ row }"><el-tag :type="resultType(row.result)" size="small">{{ resultLabel(row.result) }}</el-tag></template>
        </el-table-column>

        <el-table-column prop="severity" label="风险" width="72">
          <template #default="{ row }"><el-tag :type="severityType(row.severity)" size="small" effect="plain">{{ severityLabel(row.severity) }}</el-tag></template>
        </el-table-column>

        <el-table-column label="目标" min-width="112" show-overflow-tooltip>
          <template #default="{ row }">{{ targetLabel(row) }}</template>
        </el-table-column>

        <el-table-column label="事件" min-width="230">
          <template #default="{ row }">
            <div class="event-cell">
              <span>{{ actionLabel(row.action || row.path) }}</span>
              <small><el-tag :type="methodType(row.method)" size="small" effect="plain">{{ row.method }}</el-tag>{{ row.path }}</small>
            </div>
          </template>
        </el-table-column>

        <el-table-column prop="created_at" label="时间" width="150">
          <template #default="{ row }">{{ formatTime(row.created_at) }}</template>
        </el-table-column>

        <el-table-column label="操作" width="100" class-name="common-operation-column" header-class-name="common-operation-column">
          <template #default="{ row }">
            <ActionButton action="view" text="详情" @click="showDetail(row)" />
          </template>
        </el-table-column>
      </el-table>
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          :page-sizes="[20, 50, 100, 200]"
          layout="total, sizes, prev, pager, next, jumper"
          @change="fetchList"
        />
      </div>
    </el-card>

    <el-dialog v-model="detailVisible" title="审计事件详情" width="min(720px, calc(100vw - 32px))">
      <div v-loading="detailLoading" class="audit-detail-content">
        <el-descriptions v-if="detail" :column="2" border size="small">
          <el-descriptions-item label="审计流序号">#{{ detail.stream_seq || '-' }}</el-descriptions-item>
          <el-descriptions-item label="主体类型">{{ actorTypeLabel(detail.actor_type) }}</el-descriptions-item>
          <el-descriptions-item label="操作人">{{ detail.username || '-' }}</el-descriptions-item>
          <el-descriptions-item label="事件类型">{{ categoryLabel(detail.category) }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.source_workspace_id || detail.target_workspace_id" label="源工作空间">{{ workspaceLabel(detail.source_workspace_id, '未选择') }}</el-descriptions-item>
          <el-descriptions-item v-if="detail.source_workspace_id || detail.target_workspace_id" label="目标工作空间">{{ workspaceLabel(detail.target_workspace_id || detail.workspace_id) }}</el-descriptions-item>
          <el-descriptions-item v-if="operationReason(detail)" label="访问原因" :span="2">{{ operationReason(detail) }}</el-descriptions-item>
          <el-descriptions-item label="动作">{{ actionLabel(detail.action) }}<small v-if="detail.action" class="detail-enum">{{ detail.action }}</small></el-descriptions-item>
          <el-descriptions-item label="结果">{{ resultLabel(detail.result) }}</el-descriptions-item>
          <el-descriptions-item label="风险级别">{{ severityLabel(detail.severity) }}</el-descriptions-item>
          <el-descriptions-item label="失败原因">{{ reasonCodeLabel(detail.reason_code) }}<small v-if="detail.reason_code" class="detail-enum">{{ detail.reason_code }}</small></el-descriptions-item>
          <el-descriptions-item label="操作目标" :span="2">{{ targetLabel(detail) }}</el-descriptions-item>
          <el-descriptions-item label="请求方法">{{ detail.method }}</el-descriptions-item>
          <el-descriptions-item label="状态码">{{ detail.status }}</el-descriptions-item>
          <el-descriptions-item label="请求路径" :span="2"><span class="detail-code">{{ detail.path }}</span></el-descriptions-item>
          <el-descriptions-item label="请求 ID"><span class="detail-code">{{ detail.request_id || '-' }}</span></el-descriptions-item>
          <el-descriptions-item label="Trace ID"><span class="detail-code">{{ detail.trace_id || '-' }}</span></el-descriptions-item>
          <el-descriptions-item label="客户端 IP">{{ detail.ip || '-' }}</el-descriptions-item>
          <el-descriptions-item label="耗时">{{ detail.latency }} ms</el-descriptions-item>
          <el-descriptions-item label="详情" :span="2"><pre class="detail-json">{{ formatDetails(detail.details) }}</pre></el-descriptions-item>
          <el-descriptions-item v-if="detail.before_json" label="变更前" :span="2"><pre class="detail-json">{{ formatDetails(detail.before_json) }}</pre></el-descriptions-item>
          <el-descriptions-item v-if="detail.after_json" label="变更后" :span="2"><pre class="detail-json">{{ formatDetails(detail.after_json) }}</pre></el-descriptions-item>
          <el-descriptions-item label="上一条哈希" :span="2"><span class="detail-code">{{ detail.prev_hash || '-' }}</span></el-descriptions-item>
          <el-descriptions-item label="当前哈希" :span="2"><span class="detail-code">{{ detail.current_hash || '-' }}</span></el-descriptions-item>
        </el-descriptions>
        <el-empty v-else description="暂无详情" />
      </div>
    </el-dialog>

    <el-dialog v-model="exportDialogVisible" title="审计导出记录" width="min(900px, calc(100vw - 32px))" @closed="stopExportPolling">
      <div class="dialog-toolbar">
        <span class="filter-summary">导出文件过期后会自动清理</span>
        <ActionButton action="refresh" :loading="exportJobsLoading" @click="loadExportJobs" />
      </div>
      <el-table :data="exportJobs" v-loading="exportJobsLoading" stripe border empty-text="暂无导出记录">
        <el-table-column prop="format" label="格式" width="80"><template #default="{ row }">{{ String(row.format || '').toUpperCase() }}</template></el-table-column>
        <el-table-column prop="status" label="状态" width="100"><template #default="{ row }"><el-tag :type="exportStatusType(row.status)" size="small">{{ exportStatusLabel(row.status) }}</el-tag></template></el-table-column>
        <el-table-column prop="record_count" label="记录数" width="100" />
        <el-table-column prop="file_size" label="文件大小" width="110"><template #default="{ row }">{{ formatBytes(row.file_size) }}</template></el-table-column>
        <el-table-column prop="created_at" label="创建时间" width="180"><template #default="{ row }">{{ formatTime(row.created_at) }}</template></el-table-column>
        <el-table-column prop="expires_at" label="过期时间" width="180"><template #default="{ row }">{{ formatTime(row.expires_at) }}</template></el-table-column>
        <el-table-column label="操作" width="112" fixed="right" class-name="common-operation-column" header-class-name="common-operation-column">
          <template #default="{ row }"><ActionButton action="download" :disabled="row.status !== 'completed'" @click="downloadExportJob(row)" /></template>
        </el-table-column>
      </el-table>
      <div class="pagination-wrapper dialog-pagination">
        <el-pagination v-model:current-page="exportPage" :page-size="10" :total="exportTotal" layout="total, prev, pager, next" @current-change="loadExportJobs" />
      </div>
    </el-dialog>

    <el-dialog v-model="archiveDialogVisible" title="审计归档记录" width="min(980px, calc(100vw - 32px))">
      <div class="dialog-toolbar">
        <span class="filter-summary">只有加密对象回读校验成功后，才会清理对应的过期热事件</span>
        <div class="toolbar-actions">
          <ActionButton action="refresh" :loading="archiveLoading" @click="loadArchives" />
          <ActionButton
            v-if="policy.archive_enabled"
            v-permission="'audit:archive'"
            action="archive"
            text="执行归档"
            :loading="archiveRunning"
            @click="runArchive"
          />
        </div>
      </div>
      <el-alert
        v-if="!policy.archive_enabled"
        class="archive-disabled-alert"
        type="info"
        :closable="false"
        show-icon
        title="当前未启用自动审计归档，系统不会删除任何热事件"
      />
      <el-table :data="archives" v-loading="archiveLoading" stripe border empty-text="暂无归档记录">
        <el-table-column prop="status" label="状态" width="90"><template #default="{ row }"><el-tag :type="archiveStatusType(row.status)" size="small">{{ archiveStatusLabel(row.status) }}</el-tag></template></el-table-column>
        <el-table-column label="序号范围" min-width="165"><template #default="{ row }">#{{ row.from_seq }} - #{{ row.to_seq }}</template></el-table-column>
        <el-table-column prop="event_count" label="事件数" width="90" />
        <el-table-column prop="object_size" label="归档大小" width="110"><template #default="{ row }">{{ formatBytes(row.object_size) }}</template></el-table-column>
        <el-table-column prop="failure_count" label="失败次数" width="90" />
        <el-table-column prop="verified_at" label="校验时间" width="180"><template #default="{ row }">{{ formatTime(row.verified_at) }}</template></el-table-column>
        <el-table-column prop="error_message" label="最近错误" min-width="180" show-overflow-tooltip><template #default="{ row }">{{ row.error_message || '-' }}</template></el-table-column>
      </el-table>
      <div class="pagination-wrapper dialog-pagination">
        <el-pagination v-model:current-page="archivePage" :page-size="10" :total="archiveTotal" layout="total, prev, pager, next" @current-change="loadArchives" />
      </div>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { auditApi } from '../../api/audit'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useUserStore } from '../../stores/user'
import ActionButton from '../../components/common/ActionButton.vue'

const list = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const defaultFilters = () => ({ username: '', method: '', category: '', severity: '', result: '', actor_type: '', target_type: '', target_id: '', ip: '', request_id: '' })
const filters = reactive(defaultFilters())
const timeRange = ref([])
const advancedVisible = ref(false)
const activeAuditView = ref('all')
const detailVisible = ref(false)
const detailLoading = ref(false)
const detail = ref(null)
const verifyLoading = ref(false)
const exportLoading = ref(false)
const policy = reactive({ hot_retention_days: 0, export_retention_hours: 0, archive_enabled: false })
const exportDialogVisible = ref(false)
const exportJobsLoading = ref(false)
const exportJobs = ref([])
const exportPage = ref(1)
const exportTotal = ref(0)
let exportPollTimer = null
const archiveDialogVisible = ref(false)
const archiveLoading = ref(false)
const archiveRunning = ref(false)
const archives = ref([])
const archivePage = ref(1)
const archiveTotal = ref(0)
const userStore = useUserStore()
const auditViewTitle = computed(() => ({ all: '全部审计事件', operation: '操作审计', access: '文件访问审计', security: '安全事件' }[activeAuditView.value]))
const auditViewDescription = computed(() => ({ all: '统一查看当前工作空间的操作、访问和安全事件', operation: '记录目录、权限、角色、配置等业务操作', access: '记录文件、版本、外链和批量下载访问链路', security: '聚焦权限拒绝、认证失败和跨空间高风险事件' }[activeAuditView.value]))

onMounted(async () => {
  await Promise.all([fetchList(), loadPolicy()])
})
onBeforeUnmount(stopExportPolling)

async function loadPolicy() {
  try {
    const res = await auditApi.getPolicy()
    Object.assign(policy, res.data || {})
  } catch {
    Object.assign(policy, { hot_retention_days: 0, export_retention_hours: 0, archive_enabled: false })
  }
}

async function fetchList() {
  loading.value = true
  try {
    const res = await auditApi.getLogs(filterParams({ page: page.value, page_size: pageSize.value }))
    list.value = res.data.list || []
    total.value = res.data.total
  } catch (error) {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function showDetail(row) {
  detailVisible.value = true
  detailLoading.value = true
  detail.value = null
  try {
    const res = await auditApi.getDetail(row.id)
    detail.value = res.data
  } finally {
    detailLoading.value = false
  }
}

async function verifyChain() {
  if (!userStore.currentWorkspaceId) return
  verifyLoading.value = true
  try {
    const res = await auditApi.verifyStream(userStore.currentWorkspaceId)
    const value = res.data
    if (value.valid) ElMessage.success(`审计链校验通过，共检查 ${value.checked} 条事件，末序号 #${value.last_seq || 0}`)
    else ElMessage.error(`审计链存在断点：事件 ${value.broken_id}（${value.broken_kind}）`)
  } finally {
    verifyLoading.value = false
  }
}

async function exportLogs() {
  exportLoading.value = true
  try {
    const res = await auditApi.createExport(filterParams(), 'csv')
    const jobId = res.data.id
    for (let attempt = 0; attempt < 30; attempt += 1) {
      await new Promise(resolve => setTimeout(resolve, 1000))
      const status = await auditApi.getExport(jobId)
      if (status.data.status === 'failed' || status.data.status === 'expired') throw new Error('审计导出任务失败或已过期')
      if (status.data.status !== 'completed') continue
      await downloadExportJob(status.data)
      ElMessage.success('审计导出已下载')
      return
    }
    throw new Error('审计导出任务处理超时')
  } catch (error) {
    if (!error?.presented) ElMessage.error(error?.message || '审计导出失败')
  } finally {
    exportLoading.value = false
  }
}

async function openExportJobs() {
  exportDialogVisible.value = true
  exportPage.value = 1
  await loadExportJobs()
}

async function loadExportJobs() {
  exportJobsLoading.value = true
  stopExportPolling()
  try {
    const res = await auditApi.listExports({ page: exportPage.value, page_size: 10 })
    exportJobs.value = res.data.list || []
    exportTotal.value = res.data.total || 0
    if (exportDialogVisible.value && exportJobs.value.some(item => item.status === 'queued' || item.status === 'running')) {
      exportPollTimer = window.setTimeout(loadExportJobs, 2000)
    }
  } finally {
    exportJobsLoading.value = false
  }
}

function stopExportPolling() {
  if (exportPollTimer) window.clearTimeout(exportPollTimer)
  exportPollTimer = null
}

async function downloadExportJob(job) {
  const fileResponse = await auditApi.downloadExport(job.id)
  const url = URL.createObjectURL(fileResponse.data)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `audit-export-${job.id}.${job.format || 'csv'}`
  anchor.click()
  URL.revokeObjectURL(url)
}

async function openArchives() {
  archiveDialogVisible.value = true
  archivePage.value = 1
  await loadArchives()
}

async function loadArchives() {
  archiveLoading.value = true
  try {
    const res = await auditApi.listArchives({ page: archivePage.value, page_size: 10 })
    archives.value = res.data.list || []
    archiveTotal.value = res.data.total || 0
  } finally {
    archiveLoading.value = false
  }
}

async function runArchive() {
  try {
    await ElMessageBox.confirm(
      `系统将归档超过 ${policy.hot_retention_days} 天的连续审计事件；只有加密对象回读校验成功后才会清理热数据。确定继续吗？`,
      '执行审计归档',
      { type: 'warning', confirmButtonText: '确认执行' }
    )
  } catch {
    return
  }
  archiveRunning.value = true
  try {
    await auditApi.runArchive()
    ElMessage.success('审计归档任务执行完成')
    await Promise.all([loadArchives(), fetchList()])
  } finally {
    archiveRunning.value = false
  }
}

function applyFilters() {
  page.value = 1
  fetchList()
}

function handleAuditViewChange(value) {
  filters.category = value === 'all' ? '' : value
  page.value = 1
  fetchList()
}

function handleCategoryChange(value) {
  activeAuditView.value = value || 'all'
  applyFilters()
}

function resetFilters() {
  Object.assign(filters, defaultFilters())
  timeRange.value = []
  applyFilters()
}

function filterParams(base = {}) {
  const params = { ...base }
  Object.entries(filters).forEach(([key, value]) => {
    if (String(value || '').trim()) params[key] = String(value).trim()
  })
  if (timeRange.value?.length === 2) {
    params.from = new Date(timeRange.value[0]).toISOString()
    params.to = new Date(timeRange.value[1]).toISOString()
  }
  return params
}

function methodType(methodValue) {
  return { GET: 'info', POST: 'success', PUT: 'warning', PATCH: 'warning', DELETE: 'danger' }[methodValue] || ''
}

function statusType(status) {
  if (Number(status) >= 500) return 'danger'
  if (Number(status) >= 400) return 'warning'
  return 'success'
}

function categoryLabel(value) {
  return { operation: '操作', access: '访问', security: '安全' }[value] || '操作'
}

function resultLabel(value) {
  return { success: '成功', failure: '失败', denied: '拒绝' }[value] || value || '-'
}

function resultType(value) {
  return { success: 'success', failure: 'danger', denied: 'warning' }[value] || 'info'
}

function severityLabel(value) {
  return { info: '普通', warning: '警告', high: '高风险' }[value] || value || '-'
}

function severityType(value) {
  return { info: 'info', warning: 'warning', high: 'danger' }[value] || 'info'
}

function actorTypeLabel(value) {
  return { user: '登录用户', system: '系统任务', external_share: '外链访客' }[value] || value || '-'
}

function actionLabel(value) {
  return {
    'permission:allowed': '权限允许',
    'permission:denied': '权限拒绝',
    'super_admin_cross_workspace_read': '超级管理员跨空间读取',
    'file:download_denied': '文件下载被拒绝'
  }[value] || value || '-'
}

function reasonCodeLabel(value) {
  return {
    permission_denied: '权限不足',
    authentication_required: '需要登录',
    unsafe_scan_status: '文件安全状态不允许',
    invalid_request: '请求参数无效',
    resource_not_found: '资源不存在',
    resource_gone: '资源已失效',
    conflict: '资源状态冲突',
    rate_limited: '请求过于频繁',
    internal_error: '服务内部错误'
  }[value] || value || '-'
}

function targetLabel(value) {
  if (!value?.target_type && !value?.target_id && !value?.target_name_snapshot) return '-'
  const identity = value.target_name_snapshot || value.target_id || '-'
  return value.target_type ? `${value.target_type} · ${identity}` : identity
}

function workspaceLabel(workspaceId, fallback = '-') {
  if (!workspaceId) return fallback
  const workspace = userStore.workspaces.find(item => Number(item.id) === Number(workspaceId))
  return workspace ? `${workspace.name}（${workspaceId}）` : `工作空间 ${workspaceId}`
}

function operationReason(value) {
  try {
    return JSON.parse(value?.details || '{}').operation_reason || ''
  } catch {
    return ''
  }
}

function exportStatusLabel(value) {
  return { queued: '排队中', running: '执行中', completed: '已完成', failed: '失败', expired: '已过期' }[value] || value || '-'
}

function exportStatusType(value) {
  return { queued: 'info', running: 'warning', completed: 'success', failed: 'danger', expired: 'info' }[value] || 'info'
}

function archiveStatusLabel(value) {
  return { queued: '排队中', running: '执行中', completed: '已完成', failed: '失败' }[value] || value || '-'
}

function archiveStatusType(value) {
  return { queued: 'info', running: 'warning', completed: 'success', failed: 'danger' }[value] || 'info'
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let size = bytes / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1 }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[index]}`
}

function formatDetails(value) {
  try {
    return JSON.stringify(JSON.parse(value || '{}'), null, 2)
  } catch {
    return value || '{}'
  }
}

import { formatDateTime } from '../../utils/date'

function formatTime(value) {
  return formatDateTime(value)
}
</script>

<style scoped>
.toolbar-input {
  width: 180px;
}

.toolbar-select {
  width: 150px;
}

.audit-search-card :deep(.el-card__body) {
  padding-bottom: 12px;
}

.audit-table-card :deep(.el-card__body) {
  overflow: hidden;
}

.audit-view-tabs {
  flex-shrink: 0;
  padding: 0 16px;
  border-bottom: 1px solid var(--border-color);
}

.audit-view-tabs :deep(.el-tabs__header) {
  margin: 0;
}

.audit-view-tabs :deep(.el-tabs__nav-wrap::after) {
  display: none;
}

.audit-table-heading {
  display: flex;
  min-height: 48px;
  align-items: center;
  justify-content: space-between;
  flex-shrink: 0;
  padding: 8px 16px;
}

.audit-table-heading > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.audit-table-heading strong {
  color: var(--text-primary);
  font-size: 14px;
}

.audit-table-heading small {
  color: var(--text-muted);
  font-size: 12px;
}

.audit-search-card .search-form {
  display: block;
}

.audit-filter-main,
.audit-filter-advanced,
.audit-toolbar-footer {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px 14px;
}

.audit-filter-advanced {
  margin-top: 2px;
  padding-top: 12px;
  border-top: 1px solid var(--border-color);
}

.audit-toolbar-footer {
  justify-content: space-between;
  margin-top: 2px;
  padding-top: 10px;
  border-top: 1px solid var(--border-color);
}

.audit-query-actions {
  margin-bottom: 10px;
}

.filter-summary {
  display: flex;
  flex-wrap: wrap;
  gap: 6px 14px;
  color: var(--text-secondary);
  font-size: 13px;
}

.dialog-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.dialog-pagination {
  display: flex;
  justify-content: flex-end;
  padding: 12px 0 0;
}

.archive-disabled-alert {
  margin-bottom: 12px;
}

.request-id-input {
  width: 220px;
}

.time-range-picker {
  width: 360px !important;
}

.actor-cell {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 2px;
}

.actor-cell small {
  color: var(--text-secondary);
  font-size: 11px;
}

:deep(.el-table__row td) {
  white-space: nowrap !important;
}

.event-cell {
  display: flex;
  min-width: 0;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  text-align: center;
}

.event-cell > span,
.event-cell > small {
  width: 100%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.event-cell > small {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  color: var(--text-secondary);
  font-size: 11px;
}

.event-cell :deep(.el-tag) {
  flex-shrink: 0;
}

.path-text {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-weight: 600;
  color: var(--text-primary);
}

.detail-code,
.detail-json {
  word-break: break-all;
  white-space: pre-wrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.audit-detail-content {
  min-height: 120px;
}

.detail-enum {
  display: block;
  margin-top: 3px;
  color: var(--text-muted);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
}

.detail-json {
  margin: 0;
  font-size: 12px;
}

@media (max-width: 640px) {
  .toolbar-input,
  .toolbar-select,
  .request-id-input,
  .time-range-picker {
    width: 100%;
  }

  .audit-filter-main,
  .audit-filter-advanced,
  .audit-toolbar-footer,
  .audit-query-actions {
    align-items: stretch;
  }

  .audit-toolbar-footer,
  .audit-toolbar-footer .toolbar-actions {
    width: 100%;
  }
}
</style>
