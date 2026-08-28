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
  <div class="page-container backup-page">
    <div class="page-toolbar">
      <span class="page-count">共 {{ total }} 个备份任务</span>
      <div class="toolbar-actions">
        <ActionButton action="refresh" @click="fetchList" />
        <ActionButton action="baseline" :plain="false" :loading="creating" @click="createBaseline" />
        <ActionButton action="incremental" :loading="creatingIncremental" @click="createIncremental" />
      </div>
    </div>
    <el-alert
      v-if="backupHealth"
      class="backup-health-alert"
      :type="healthAlertType"
      :closable="false"
      show-icon
      :title="healthTitle"
    >
      <template #default>
        <span>{{ healthDescription }}</span>
        <span v-if="backupHealth.recommended_next" class="health-next">建议：{{ backupHealth.recommended_next }}</span>
      </template>
    </el-alert>
    <div v-if="backupHealth?.compaction" class="compaction-strip">
      <div class="compaction-copy">
        <strong>增量链压缩</strong>
        <span>把当前增量链重建为已校验的独立基线，原备份任务和对象仍会保留。</span>
      </div>
      <div class="compaction-status">
        <el-tag :type="backupHealth.compaction.enabled ? 'success' : 'info'" size="small">
          自动压缩{{ backupHealth.compaction.enabled ? '已启用' : '未启用' }}
        </el-tag>
        <span class="compaction-depth">
          当前 <strong>{{ backupHealth.compaction.current_incrementals }}</strong> / {{ backupHealth.compaction.incremental_threshold }} 个增量
        </span>
        <span v-if="backupHealth.compaction.last_compaction_at" class="compaction-last">
          最近压缩 {{ formatDateTime(backupHealth.compaction.last_compaction_at) }}
        </span>
        <ActionButton
          action="compact"
          :loading="compacting"
          :disabled="!backupHealth.compaction.manual_available"
          :title="backupHealth.compaction.manual_available ? '立即生成独立压缩基线' : '当前没有可压缩的增量任务'"
          @click="compactBaseline"
        />
      </div>
    </div>
    <el-alert class="backup-tip" type="info" :closable="false" show-icon title="增量备份按变更游标连续生成；压缩会先校验完整源链，再生成并校验新的独立基线。" />
    <el-card shadow="never" class="table-card backup-tabs-card">
      <el-tabs v-model="activeTab" class="backup-tabs">
        <el-tab-pane name="jobs">
          <template #label><span class="backup-tab-label">备份任务 <em>{{ total }}</em></span></template>
          <div class="backup-tab-panel">
            <div class="backup-section-heading">
              <div><strong>备份任务</strong><small>按时间倒序展示当前工作空间的完整备份链</small></div>
            </div>
            <el-table class="backup-job-table" :data="list" v-loading="loading" stripe border>
              <el-table-column prop="id" label="任务 ID" min-width="165" show-overflow-tooltip />
              <el-table-column prop="kind" label="类型" min-width="72">
                <template #default="{ row }"><el-tag :type="backupTypeTag(row)" size="small">{{ backupTypeLabel(row) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="status" label="状态" min-width="80">
                <template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="verify_status" label="校验" min-width="80">
                <template #default="{ row }"><el-tag :type="verifyStatusType(row.verify_status)" effect="plain" size="small">{{ verifyStatusLabel(row.verify_status) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="object_count" label="对象数" min-width="68" />
              <el-table-column label="变更游标" min-width="105">
                <template #default="{ row }">
                  <span v-if="row.kind === 'incremental'">{{ row.change_log_start }} → {{ row.change_log_end }}</span>
                  <span v-else>截止 {{ row.change_log_end }}</span>
                </template>
              </el-table-column>
              <el-table-column prop="total_bytes" label="数据量" min-width="80"><template #default="{ row }">{{ formatBytes(row.total_bytes) }}</template></el-table-column>
              <el-table-column prop="created_at" label="创建时间" min-width="125"><template #default="{ row }">{{ formatDateTime(row.created_at) }}</template></el-table-column>
              <el-table-column label="操作" width="220" fixed="right">
                <template #default="{ row }">
                  <div class="admin-action-group">
                    <ActionButton v-if="row.status === 'complete'" action="files" text="文件" @click="openDetail(row)" />
                    <ActionButton v-if="row.status === 'complete'" action="verify" @click="verify(row)" />
                    <ActionButton v-if="row.status === 'complete'" action="drill" :loading="drillingId === row.id" @click="runRestoreDrill(row)" />
                    <ActionButton v-if="row.status === 'failed'" action="retryBackup" :loading="retryingId === row.id" @click="retry(row)" />
                    <span v-if="row.status !== 'complete' && row.status !== 'failed'" class="action-placeholder">处理中</span>
                    <span v-if="row.status === 'failed'" class="action-placeholder">原任务已保留</span>
                  </div>
                </template>
              </el-table-column>
            </el-table>
            <div class="pagination-wrapper"><el-pagination v-model:current-page="page" v-model:page-size="pageSize" :total="total" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next, jumper" @change="fetchList" /></div>
          </div>
        </el-tab-pane>

        <el-tab-pane name="drills">
          <template #label><span class="backup-tab-label">恢复演练 <em>{{ drillTotal }}</em></span></template>
          <div class="backup-tab-panel">
            <div class="backup-section-heading">
              <div><strong>恢复演练记录</strong><small>隔离写入并校验整条备份链中的文件版本，不修改当前工作空间</small></div>
              <ActionButton action="refresh" :loading="drillLoading" @click="fetchDrills" />
            </div>
            <el-table class="backup-drill-table" :data="drills" v-loading="drillLoading" stripe border empty-text="暂无恢复演练记录">
              <el-table-column prop="id" label="演练 ID" min-width="185" show-overflow-tooltip />
              <el-table-column prop="backup_job_id" label="恢复点" min-width="185" show-overflow-tooltip />
              <el-table-column prop="status" label="状态" min-width="80">
                <template #default="{ row }"><el-tag :type="statusType(row.status)" size="small">{{ statusLabel(row.status) }}</el-tag></template>
              </el-table-column>
              <el-table-column prop="object_count" label="对象数" min-width="70" />
              <el-table-column prop="total_bytes" label="写入量" min-width="90"><template #default="{ row }">{{ formatBytes(row.total_bytes) }}</template></el-table-column>
              <el-table-column prop="created_at" label="开始时间" min-width="130"><template #default="{ row }">{{ formatDateTime(row.started_at || row.created_at) }}</template></el-table-column>
              <el-table-column prop="completed_at" label="完成时间" min-width="130"><template #default="{ row }">{{ formatDateTime(row.completed_at) }}</template></el-table-column>
            </el-table>
            <div class="pagination-wrapper"><el-pagination v-model:current-page="drillPage" v-model:page-size="drillPageSize" :total="drillTotal" :page-sizes="[10, 20, 50]" layout="total, sizes, prev, pager, next" @change="fetchDrills" /></div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <el-drawer v-model="detailVisible" title="备份文件" size="min(760px, 92vw)">
      <el-alert
        v-if="detail"
        class="metadata-alert"
        :type="detail.metadata_available ? 'success' : 'warning'"
        :closable="false"
        show-icon
        :title="detail.metadata_available ? '该恢复点包含完整工作空间元数据' : '该历史恢复点仅支持文件级恢复'"
      >
        <template #default>
          <span v-if="detail.metadata_available">包含 {{ detail.metadata_node_count }} 个节点、{{ detail.metadata_version_count }} 个文件版本；可恢复为一个新的独立工作空间。</span>
          <span v-else>请先创建新的基线或增量备份，再执行完整工作空间恢复。</span>
        </template>
      </el-alert>
      <el-descriptions v-if="detail" class="backup-summary" :column="2" border>
        <el-descriptions-item label="备份类型">{{ backupTypeLabel(detail) }}</el-descriptions-item>
        <el-descriptions-item label="元数据变更">{{ detail.change_count || 0 }} 条</el-descriptions-item>
        <el-descriptions-item label="起始游标">{{ detail.change_log_start || 0 }}</el-descriptions-item>
        <el-descriptions-item label="截止游标">{{ detail.change_log_end || 0 }}</el-descriptions-item>
        <el-descriptions-item label="父任务" :span="2"><span class="mono-value">{{ detail.parent_id || '无（基线）' }}</span></el-descriptions-item>
        <el-descriptions-item v-if="detail.compacted_from_id" label="压缩来源" :span="2"><span class="mono-value">{{ detail.compacted_from_id }}</span></el-descriptions-item>
      </el-descriptions>
      <div v-if="detail?.metadata_available && isSuperAdmin" class="workspace-restore-entry">
        <div><strong>完整工作空间恢复</strong><span>创建独立空间，不覆盖当前数据；历史分享恢复后默认撤销。</span></div>
        <ActionButton action="restoreWorkspace" @click="openWorkspaceRestore" />
      </div>
      <el-table :data="objects" v-loading="detailLoading" stripe border>
        <el-table-column prop="name" label="文件名" min-width="180" show-overflow-tooltip />
        <el-table-column prop="version_id" label="原版本 ID" width="110" />
        <el-table-column prop="size" label="大小" width="110"><template #default="{ row }">{{ formatBytes(row.size) }}</template></el-table-column>
        <el-table-column prop="scan_status" label="安全状态" width="110" />
        <el-table-column label="操作" width="110" fixed="right" align="center"><template #default="{ row }"><ActionButton action="restore" @click="restore(row)" /></template></el-table-column>
      </el-table>
      <el-empty v-if="!detailLoading && !objects.length" description="该备份没有文件对象" />
    </el-drawer>

    <el-dialog v-model="workspaceRestoreVisible" title="恢复为独立工作空间" width="min(520px, calc(100vw - 32px))" :close-on-click-modal="false" @closed="resetWorkspaceRestore">
      <el-alert type="warning" :closable="false" show-icon title="这是高风险且耗时的恢复操作">
        <template #default>系统会完整校验备份链，重建目录、版本、成员、权限与角色。恢复期间新空间不可见，失败会自动清理。</template>
      </el-alert>
      <el-form ref="workspaceRestoreFormRef" class="workspace-restore-form" :model="workspaceRestoreForm" :rules="workspaceRestoreRules" label-position="top" @submit.prevent="submitWorkspaceRestore">
        <el-form-item label="新工作空间名称" prop="name"><el-input v-model="workspaceRestoreForm.name" maxlength="128" show-word-limit placeholder="例如：项目资料恢复副本" /></el-form-item>
        <el-form-item label="新工作空间代号" prop="code"><el-input v-model="workspaceRestoreForm.code" maxlength="64" placeholder="例如：project-restore-20260813" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="workspaceRestoreVisible = false">取消</el-button>
        <ActionButton action="restoreWorkspace" text="确认恢复" :plain="false" :loading="restoringWorkspace" @click="submitWorkspaceRestore" />
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import ActionButton from '../../components/common/ActionButton.vue'
import { backupApi } from '../../api/backup'
import { useUserStore } from '../../stores/user'
import { formatDateTime } from '../../utils/date'

const userStore = useUserStore()
const activeTab = ref('jobs')
const list = ref([]); const total = ref(0); const page = ref(1); const pageSize = ref(20); const loading = ref(false); const creating = ref(false); const creatingIncremental = ref(false); const compacting = ref(false)
const retryingId = ref('')
const drillingId = ref(''); const drills = ref([]); const drillLoading = ref(false); const drillTotal = ref(0); const drillPage = ref(1); const drillPageSize = ref(10)
const backupHealth = ref(null)
const detailVisible = ref(false); const detailLoading = ref(false); const detailJob = ref(null); const detail = ref(null); const objects = ref([])
const workspaceRestoreVisible = ref(false); const workspaceRestoreFormRef = ref(null); const restoringWorkspace = ref(false)
const workspaceRestoreForm = ref({ name: '', code: '' })
const workspaceRestoreRules = {
  name: [{ required: true, message: '请输入新工作空间名称', trigger: 'blur' }],
  code: [{ required: true, message: '请输入新工作空间代号', trigger: 'blur' }, { pattern: /^[a-z][a-z0-9-]{2,63}$/, message: '以小写字母开头，仅包含小写字母、数字和连字符', trigger: 'blur' }]
}
const isSuperAdmin = computed(() => !!userStore.user?.is_super_admin)
const healthAlertType = computed(() => backupHealth.value?.status === 'critical' ? 'error' : backupHealth.value?.status === 'warning' ? 'warning' : 'success')
const healthTitle = computed(() => backupHealth.value?.status === 'critical' ? '备份恢复状态异常' : backupHealth.value?.status === 'warning' ? '备份恢复需要关注' : '备份链与恢复演练正常')
const healthDescription = computed(() => (backupHealth.value?.alerts || []).join('；') || '最新完整备份链校验通过，最近一次恢复演练成功。')
onMounted(() => { fetchList(); fetchDrills(); fetchHealth() })
async function fetchList() { loading.value = true; try { const res = await backupApi.list({ page: page.value, page_size: pageSize.value }); list.value = res.data.list || []; total.value = res.data.total || 0 } finally { loading.value = false } }
async function fetchHealth() { const res = await backupApi.health(); backupHealth.value = res.data || null }
async function createBaseline() { creating.value = true; try { await backupApi.createBaseline(); ElMessage.success('基线备份已创建') } finally { creating.value = false; await Promise.all([fetchList(), fetchHealth()]) } }
async function createIncremental() { creatingIncremental.value = true; try { await backupApi.createIncremental(); ElMessage.success('增量备份已创建') } finally { creatingIncremental.value = false; await Promise.all([fetchList(), fetchHealth()]) } }
async function compactBaseline() {
  const depth = backupHealth.value?.compaction?.current_incrementals || 0
  if (!await confirmAction(`将校验当前完整备份链，并把 ${depth} 个增量任务重建为新的独立基线。原备份任务和对象不会删除，是否继续？`, '压缩备份基线', { type: 'warning', confirmButtonText: '开始压缩' })) return
  compacting.value = true
  try { await backupApi.compact(); ElMessage.success('备份链已压缩为新的独立基线') } finally { compacting.value = false; await Promise.all([fetchList(), fetchHealth()]) }
}
async function retry(row) {
	if (!await confirmAction(`将为失败的${row.kind === 'baseline' ? '基线' : '增量'}备份创建一个新的重试任务，原失败记录会保留。是否继续？`, '重试备份', { type: 'warning', confirmButtonText: '确认重试' })) return
  retryingId.value = row.id
  try { await backupApi.retry(row.id); ElMessage.success('备份重试任务已完成') } finally { retryingId.value = ''; await Promise.all([fetchList(), fetchHealth()]) }
}
async function fetchDrills() { drillLoading.value = true; try { const res = await backupApi.listRestoreDrills({ page: drillPage.value, page_size: drillPageSize.value }); drills.value = res.data.list || []; drillTotal.value = res.data.total || 0 } finally { drillLoading.value = false } }
async function runRestoreDrill(row) {
	if (!await confirmAction(`将对备份 ${row.id} 的完整父链执行隔离恢复写入和 SHA-256 校验，不会修改当前工作空间。是否继续？`, '执行恢复演练', { type: 'warning', confirmButtonText: '确认演练' })) return
  drillingId.value = row.id
  try { await backupApi.restoreDrill(row.id); ElMessage.success('恢复演练通过') } finally { drillingId.value = ''; await Promise.all([fetchDrills(), fetchHealth()]) }
}
async function verify(row) { if (!await confirmAction(`将校验备份 ${row.id} 的清单和对象，是否继续？`, '校验备份', { type: 'info' })) return; try { await backupApi.verify(row.id); ElMessage.success('备份校验通过') } finally { await Promise.all([fetchList(), fetchHealth()]) } }
async function openDetail(row) { detailVisible.value = true; detailLoading.value = true; detailJob.value = row; detail.value = null; objects.value = []; try { const res = await backupApi.detail(row.id); detail.value = res.data; objects.value = res.data.objects || [] } finally { detailLoading.value = false } }
async function restore(row) { if (!await confirmAction(`将“${row.name || `版本 ${row.version_id}`}”恢复到工作空间根目录。不会覆盖同名文件，是否继续？`, '恢复文件', { type: 'warning', confirmButtonText: '确认恢复' })) return; await backupApi.restore(detailJob.value.id, { version_id: row.version_id, parent_id: null, confirm: true }); ElMessage.success('文件已恢复到工作空间根目录') }
function openWorkspaceRestore() {
  const suffix = new Date().toISOString().slice(0, 10).replaceAll('-', '')
  workspaceRestoreForm.value = { name: `${userStore.workspaces.find(item => item.id === userStore.currentWorkspaceId)?.name || '工作空间'}恢复副本`, code: `restore-${suffix}` }
  workspaceRestoreVisible.value = true
}
function resetWorkspaceRestore() { workspaceRestoreFormRef.value?.resetFields(); workspaceRestoreForm.value = { name: '', code: '' } }
async function submitWorkspaceRestore() {
  const valid = await workspaceRestoreFormRef.value?.validate().catch(() => false)
  if (!valid) return
  if (!await confirmAction(`将备份 ${detailJob.value.id} 完整恢复为“${workspaceRestoreForm.value.name}”（${workspaceRestoreForm.value.code}）。该操作会写入全部文件与元数据，是否继续？`, '确认完整恢复', { type: 'warning', confirmButtonText: '开始恢复', cancelButtonText: '取消' })) return
  restoringWorkspace.value = true
  try {
    const res = await backupApi.restoreWorkspace(detailJob.value.id, { ...workspaceRestoreForm.value, confirm: true })
    workspaceRestoreVisible.value = false
    userStore.invalidateSession(); await userStore.ensureSession()
    ElMessage.success(`工作空间“${res.data.name}”已完整恢复`)
  } finally { restoringWorkspace.value = false }
}
async function confirmAction(message, title, options = {}) {
  try { await ElMessageBox.confirm(message, title, options); return true } catch { return false }
}
function statusLabel(value) { return ({ running: '执行中', complete: '已完成', failed: '失败' }[value] || value || '-') }
function statusType(value) { return value === 'complete' ? 'success' : value === 'failed' ? 'danger' : 'warning' }
function verifyStatusLabel(value) { return ({ valid: '已通过', invalid: '未通过', unknown: '待校验' }[value] || '待校验') }
function verifyStatusType(value) { return value === 'valid' ? 'success' : value === 'invalid' ? 'danger' : 'info' }
function backupTypeLabel(row) {
  if (row?.trigger === 'manual_compaction') return '手动压缩'
  if (row?.trigger === 'scheduled_compaction') return '自动压缩'
  return row?.kind === 'baseline' ? '基线' : '增量'
}
function backupTypeTag(row) {
  if (row?.trigger === 'manual_compaction' || row?.trigger === 'scheduled_compaction') return 'warning'
  return row?.kind === 'baseline' ? 'primary' : 'info'
}
function formatBytes(value) { const bytes = Number(value || 0); if (bytes < 1024) return `${bytes} B`; const units = ['KB', 'MB', 'GB', 'TB']; let size = bytes / 1024; let i = 0; while (size >= 1024 && i < units.length - 1) { size /= 1024; i += 1 } return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[i]}` }
</script>

<style scoped>
.toolbar-actions { display: flex; gap: 10px; }
.backup-tip { margin-bottom: 0; }
.backup-health-alert { margin-bottom: 12px; }
.backup-health-alert :deep(.el-alert__description) { display: flex; flex-wrap: wrap; gap: 8px 18px; }
.health-next { color: var(--text-secondary); font-weight: 700; }
.compaction-strip { display: flex; align-items: center; justify-content: space-between; gap: 20px; margin-bottom: 12px; padding: 12px 16px; border-left: 3px solid #8b5cf6; background: #f8f7ff; }
.compaction-copy { display: flex; min-width: 240px; flex-direction: column; gap: 3px; }
.compaction-copy strong { color: var(--text-primary); font-size: 14px; }
.compaction-copy span, .compaction-last { color: var(--text-muted); font-size: 12px; }
.compaction-status { display: flex; align-items: center; justify-content: flex-end; gap: 12px; flex-wrap: wrap; }
.compaction-depth { color: var(--text-secondary); font-size: 13px; white-space: nowrap; }
.compaction-depth strong { color: #7c3aed; }
.backup-summary { margin-bottom: 16px; }
.metadata-alert { margin-bottom: 16px; }
.workspace-restore-entry { display: flex; align-items: center; justify-content: space-between; gap: 16px; margin-bottom: 16px; padding: 12px 14px; border: 1px solid var(--el-color-warning-light-7); border-radius: var(--radius-sm); background: var(--el-color-warning-light-9); }
.workspace-restore-entry > div { display: flex; min-width: 0; flex-direction: column; gap: 3px; }
.workspace-restore-entry strong { color: var(--text-primary); font-size: 14px; }
.workspace-restore-entry span { color: var(--text-secondary); font-size: 12px; }
.workspace-restore-form { margin-top: 18px; }
.backup-tabs-card { flex: 0 0 auto !important; min-height: 0; }
.backup-tabs-card :deep(.el-card__body) { display: block; overflow: visible; padding: 0 16px 16px; }
.backup-tabs :deep(.el-tabs__header) { margin-bottom: 16px; }
.backup-tabs :deep(.el-tabs__content) { overflow: visible; }
.backup-tab-label { display: inline-flex; align-items: center; gap: 6px; }
.backup-tab-label em { min-width: 20px; padding: 1px 6px; border-radius: 999px; background: var(--surface-soft); color: var(--text-muted); font-size: 11px; font-style: normal; text-align: center; }
.backup-tab-panel { display: flex; min-height: 0; flex-direction: column; gap: 12px; }
.backup-section-heading { display: flex; align-items: center; justify-content: space-between; gap: 16px; min-height: 36px; }
.backup-section-heading > div { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.backup-section-heading strong { color: var(--text-primary); }
.backup-section-heading small { color: var(--text-muted); font-size: 12px; }
.backup-tab-panel :deep(.el-table) { display: table !important; flex: none !important; height: auto !important; }
.backup-tab-panel :deep(.el-table__inner-wrapper) { display: block !important; height: auto !important; }
.backup-tab-panel :deep(.el-table__body-wrapper) { flex: none !important; overflow-y: visible !important; }
.backup-tab-panel .pagination-wrapper { flex-shrink: 0; }
.mono-value { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; overflow-wrap: anywhere; }
.backup-page { flex: 0 0 auto; min-height: 100%; overflow: visible; padding-right: 2px; padding-bottom: 24px; }
.backup-job-table :deep(.el-table__body-wrapper), .backup-job-table :deep(.el-scrollbar__wrap), .backup-drill-table :deep(.el-table__body-wrapper), .backup-drill-table :deep(.el-scrollbar__wrap) { overflow-x: auto; }
@media (max-width: 900px) { .compaction-strip { align-items: flex-start; flex-direction: column; } .compaction-status { justify-content: flex-start; } }
@media (max-width: 760px) { .page-toolbar, .backup-section-heading { align-items: flex-start; flex-direction: column; gap: 12px; } .toolbar-actions { flex-wrap: wrap; } }
</style>
