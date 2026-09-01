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
  <section class="notification-page">
    <header class="page-toolbar notification-toolbar">
      <div class="page-toolbar__group">
        <span class="page-toolbar__summary">{{ toolbarSummary }}</span>
        <el-select v-if="activeTab === 'outbox'" v-model="outboxStatus" clearable placeholder="全部状态" class="status-filter" @change="reloadOutbox">
          <el-option v-for="item in statusOptions" :key="item.value" :label="item.label" :value="item.value" />
        </el-select>
      </div>
      <div class="page-actions">
        <ActionButton action="refresh" :loading="activeTab === 'channels' ? channelLoading : outboxLoading" @click="refreshCurrent" />
        <ActionButton v-if="activeTab === 'channels'" action="create" text="新增渠道" :plain="false" @click="openChannelDialog()" />
      </div>
    </header>

    <div class="content-panel notification-panel">
      <el-tabs v-model="activeTab" class="notification-tabs" @tab-change="handleTabChange">
        <el-tab-pane name="channels">
          <template #label><span>通知渠道 <el-tag size="small" type="info" effect="plain">{{ channelTotal }}</el-tag></span></template>
          <div class="panel-heading">
            <div><strong>通知渠道</strong><p>凭据加密保存；测试消息同样通过可靠队列发送</p></div>
          </div>
          <el-table :data="channels" v-loading="channelLoading" row-key="id" class="notification-table">
            <el-table-column prop="name" label="渠道名称" min-width="135" show-overflow-tooltip />
            <el-table-column label="类型" width="72"><template #default="scope"><el-tag :type="channelTypeMeta(scope.row.type).type" effect="plain">{{ channelTypeMeta(scope.row.type).label }}</el-tag></template></el-table-column>
            <el-table-column prop="endpoint_summary" label="投递端点" min-width="150" show-overflow-tooltip />
            <el-table-column label="凭据" width="72"><template #default="scope"><el-tag :type="scope.row.credential_configured ? 'success' : 'danger'" effect="plain">{{ scope.row.credential_configured ? '已配置' : '未配置' }}</el-tag></template></el-table-column>
            <el-table-column label="状态" width="68"><template #default="scope"><el-tag :type="scope.row.status === 1 ? 'success' : 'info'" effect="plain">{{ scope.row.status === 1 ? '启用' : '停用' }}</el-tag></template></el-table-column>
            <el-table-column prop="remark" label="备注" min-width="110" show-overflow-tooltip><template #default="scope">{{ scope.row.remark || '-' }}</template></el-table-column>
            <el-table-column label="更新时间" width="135"><template #default="scope">{{ formatDate(scope.row.updated_at) }}</template></el-table-column>
            <el-table-column label="操作" width="220" fixed="right">
              <template #default="scope">
                <div class="common-action-group">
                  <ActionButton action="test" :disabled="scope.row.status !== 1" @click="testChannel(scope.row)" />
                  <ActionButton action="edit" @click="openChannelDialog(scope.row)" />
                  <ActionButton action="delete" @click="deleteChannel(scope.row)" />
                </div>
              </template>
            </el-table-column>
            <template #empty><el-empty description="暂无通知渠道" /></template>
          </el-table>
          <el-pagination v-if="channelTotal > 0" class="table-pagination" :current-page="channelPage" :page-size="pageSize" :total="channelTotal" layout="total, prev, pager, next" @current-change="changeChannelPage" />
        </el-tab-pane>

        <el-tab-pane name="outbox">
          <template #label><span>投递记录 <el-tag size="small" type="info" effect="plain">{{ outboxTotal }}</el-tag></span></template>
          <div class="panel-heading">
            <div><strong>投递记录</strong><p>失败任务按指数退避自动重试，耗尽后可由管理员重新入队</p></div>
          </div>
          <el-table :data="outbox" v-loading="outboxLoading" row-key="id" class="notification-table">
            <el-table-column prop="title" label="事件" min-width="175" show-overflow-tooltip>
              <template #default="scope"><div class="event-cell"><strong>{{ scope.row.title }}</strong><small>{{ scope.row.event_type }}</small></div></template>
            </el-table-column>
            <el-table-column prop="channel_name" label="渠道" min-width="110" show-overflow-tooltip>
              <template #default="scope"><span>{{ scope.row.channel_name }}</span><small class="inline-meta">{{ channelTypeMeta(scope.row.channel_type).label }}</small></template>
            </el-table-column>
            <el-table-column label="级别" width="68"><template #default="scope"><el-tag :type="severityMeta(scope.row.severity).type" effect="plain">{{ severityMeta(scope.row.severity).label }}</el-tag></template></el-table-column>
            <el-table-column label="状态" width="82"><template #default="scope"><el-tag :type="outboxStatusMeta(scope.row.status).type" effect="plain">{{ outboxStatusMeta(scope.row.status).label }}</el-tag></template></el-table-column>
            <el-table-column label="尝试" width="72"><template #default="scope">{{ scope.row.attempts }} / {{ scope.row.max_attempts }}</template></el-table-column>
            <el-table-column label="错误/下次重试" min-width="170" show-overflow-tooltip>
              <template #default="scope"><span v-if="scope.row.error_message">{{ scope.row.error_message }}</span><span v-else-if="scope.row.status === 'failed'">{{ formatDate(scope.row.next_attempt_at) }}</span><span v-else>-</span></template>
            </el-table-column>
            <el-table-column label="创建时间" width="140"><template #default="scope">{{ formatDate(scope.row.created_at) }}</template></el-table-column>
            <el-table-column label="操作" width="112" fixed="right">
              <template #default="scope">
                <ActionButton v-if="['failed', 'exhausted'].includes(scope.row.status)" action="retryNotification" @click="retryOutbox(scope.row)" />
                <span v-else class="action-placeholder">{{ outboxStatusMeta(scope.row.status).label }}</span>
              </template>
            </el-table-column>
            <template #empty><el-empty description="暂无投递记录" /></template>
          </el-table>
          <el-pagination v-if="outboxTotal > 0" class="table-pagination" :current-page="outboxPage" :page-size="pageSize" :total="outboxTotal" layout="total, prev, pager, next" @current-change="changeOutboxPage" />
        </el-tab-pane>
      </el-tabs>
    </div>

    <el-dialog v-model="channelDialogVisible" :title="editingChannel ? '编辑通知渠道' : '新增通知渠道'" width="min(620px, calc(100vw - 32px))" @closed="resetChannelForm">
      <el-alert v-if="editingChannel" title="连接凭据不会回显；敏感字段留空将保留原值。" type="info" :closable="false" show-icon />
      <el-form ref="channelFormRef" :model="channelForm" :rules="channelRules" label-position="top" class="channel-form" autocomplete="off">
        <div class="form-grid">
          <el-form-item label="渠道名称" prop="name"><el-input v-model="channelForm.name" maxlength="80" /></el-form-item>
          <el-form-item label="渠道类型" prop="type">
            <el-select v-model="channelForm.type" :disabled="Boolean(editingChannel)" @change="applyChannelDefaults">
              <el-option label="邮件（SMTP）" value="smtp" /><el-option label="企业微信" value="wecom" /><el-option label="钉钉" value="dingtalk" /><el-option label="飞书" value="feishu" />
            </el-select>
          </el-form-item>
        </div>

        <template v-if="channelForm.type === 'smtp'">
          <div class="form-grid">
            <el-form-item label="SMTP 主机" prop="smtp_host"><el-input v-model="channelForm.smtp_host" placeholder="smtp.example.com" /></el-form-item>
            <el-form-item label="端口" prop="smtp_port"><el-input-number v-model="channelForm.smtp_port" :min="1" :max="65535" controls-position="right" /></el-form-item>
          </div>
          <div class="form-grid">
            <el-form-item label="传输加密" prop="smtp_encryption"><el-select v-model="channelForm.smtp_encryption"><el-option label="STARTTLS" value="starttls" /><el-option label="TLS" value="tls" /></el-select></el-form-item>
            <el-form-item label="登录用户名"><el-input v-model="channelForm.smtp_username" name="notification-smtp-username" autocomplete="off" :placeholder="editingChannel && editingChannel.smtp_username_configured ? '留空保留已配置用户名' : '选填'" /></el-form-item>
          </div>
          <el-form-item label="登录密码"><el-input v-model="channelForm.smtp_password" name="notification-smtp-password" autocomplete="new-password" type="password" show-password :placeholder="editingChannel ? '留空保留原密码' : '用户名已配置时必填'" /></el-form-item>
          <div class="form-grid smtp-address-grid">
            <el-form-item label="发件人" prop="smtp_from"><el-input v-model="channelForm.smtp_from" placeholder="notify@example.com" /></el-form-item>
            <el-form-item label="收件人" prop="smtp_recipients"><el-select v-model="channelForm.smtp_recipients" multiple filterable allow-create default-first-option placeholder="输入邮箱后回车，可添加多个" /></el-form-item>
          </div>
        </template>
        <template v-else>
          <el-form-item label="Webhook" prop="webhook_url"><el-input v-model="channelForm.webhook_url" name="notification-webhook-url" autocomplete="off" type="textarea" :rows="3" :placeholder="editingChannel ? '留空保留原 Webhook' : webhookPlaceholder" /></el-form-item>
          <el-form-item label="签名密钥"><el-input v-model="channelForm.secret" name="notification-webhook-secret" autocomplete="new-password" type="password" show-password :placeholder="editingChannel ? '留空保留原密钥' : '选填'" /></el-form-item>
        </template>
        <el-form-item label="状态" class="channel-status-row">
          <el-select v-model="channelForm.status" class="channel-status-select" aria-label="状态">
            <el-option label="开启" :value="1" />
            <el-option label="关闭" :value="0" />
          </el-select>
        </el-form-item>
        <el-form-item label="备注"><el-input v-model="channelForm.remark" type="textarea" :rows="2" maxlength="255" show-word-limit /></el-form-item>
      </el-form>
      <template #footer><el-button @click="channelDialogVisible = false">取消</el-button><el-button type="primary" :loading="channelSaving" @click="saveChannel">保存</el-button></template>
    </el-dialog>
  </section>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { notificationApi } from '@/api/notification'
import ActionButton from '@/components/common/ActionButton.vue'

const activeTab = ref('channels')
const pageSize = 20
const channels = ref([])
const channelLoading = ref(false)
const channelPage = ref(1)
const channelTotal = ref(0)
const outbox = ref([])
const outboxLoading = ref(false)
const outboxPage = ref(1)
const outboxTotal = ref(0)
const outboxStatus = ref('')
const channelDialogVisible = ref(false)
const channelSaving = ref(false)
const channelFormRef = ref()
const editingChannel = ref(null)

const blankChannel = () => ({ name: '', type: 'smtp', webhook_url: '', secret: '', smtp_host: '', smtp_port: 587, smtp_encryption: 'starttls', smtp_username: '', smtp_password: '', smtp_from: '', smtp_recipients: [], status: 1, remark: '' })
const channelForm = reactive(blankChannel())
const emailPattern = /^[^\s@]+@[^\s@]+\.[^\s@]+$/
const channelRules = {
  name: [{ required: true, message: '请输入渠道名称', trigger: 'blur' }],
  type: [{ required: true, message: '请选择渠道类型', trigger: 'change' }],
  smtp_host: [{ validator: (_, value, callback) => channelForm.type !== 'smtp' || value ? callback() : callback(new Error('请输入 SMTP 主机')), trigger: 'blur' }],
  smtp_from: [{ validator: (_, value, callback) => channelForm.type !== 'smtp' || emailPattern.test(value) ? callback() : callback(new Error('请输入有效发件人邮箱')), trigger: 'blur' }],
  smtp_recipients: [{ validator: (_, value, callback) => channelForm.type !== 'smtp' || (value?.length && value.every(item => emailPattern.test(item))) ? callback() : callback(new Error('请至少填写一个有效收件人')), trigger: 'change' }],
  webhook_url: [{ validator: (_, value, callback) => channelForm.type === 'smtp' || editingChannel.value || value ? callback() : callback(new Error('请输入 Webhook')), trigger: 'blur' }]
}
const statusOptions = [
  { label: '等待发送', value: 'pending' }, { label: '发送中', value: 'sending' }, { label: '已发送', value: 'sent' },
  { label: '等待重试', value: 'failed' }, { label: '重试耗尽', value: 'exhausted' }, { label: '已取消', value: 'cancelled' }
]
const toolbarSummary = computed(() => activeTab.value === 'channels' ? `共 ${channelTotal.value} 个通知渠道` : `共 ${outboxTotal.value} 条投递记录`)
const webhookPlaceholder = computed(() => ({ wecom: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...', dingtalk: 'https://oapi.dingtalk.com/robot/send?access_token=...', feishu: 'https://open.feishu.cn/open-apis/bot/v2/hook/...' }[channelForm.type] || ''))

async function loadChannels() {
  channelLoading.value = true
  try {
    const res = await notificationApi.listChannels({ page: channelPage.value, page_size: pageSize })
    channels.value = res.data?.list || []
    channelTotal.value = res.data?.total || 0
  } finally { channelLoading.value = false }
}

async function loadOutbox() {
  outboxLoading.value = true
  try {
    const res = await notificationApi.listOutbox({ page: outboxPage.value, page_size: pageSize, status: outboxStatus.value || undefined })
    outbox.value = res.data?.list || []
    outboxTotal.value = res.data?.total || 0
  } finally { outboxLoading.value = false }
}

function refreshCurrent() { return activeTab.value === 'channels' ? loadChannels() : loadOutbox() }
function handleTabChange(name) { if (name === 'outbox') loadOutbox() }
function reloadOutbox() { outboxPage.value = 1; loadOutbox() }
function changeChannelPage(value) { channelPage.value = value; loadChannels() }
function changeOutboxPage(value) { outboxPage.value = value; loadOutbox() }
function applyChannelDefaults(type) { if (type === 'smtp') { channelForm.smtp_port = 587; channelForm.smtp_encryption = 'starttls' } }

function openChannelDialog(row = null) {
  editingChannel.value = row
  Object.assign(channelForm, blankChannel(), row ? { name: row.name, type: row.type, smtp_host: row.smtp_host || '', smtp_port: row.smtp_port || 587, smtp_encryption: row.smtp_encryption || 'starttls', smtp_from: row.smtp_from || '', smtp_recipients: row.smtp_recipients || [], status: row.status, remark: row.remark || '' } : {})
  channelDialogVisible.value = true
}

function resetChannelForm() { editingChannel.value = null; Object.assign(channelForm, blankChannel()); channelFormRef.value?.clearValidate() }

async function saveChannel() {
  await channelFormRef.value.validate()
  channelSaving.value = true
  try {
    const payload = { ...channelForm, smtp_recipients: [...channelForm.smtp_recipients] }
    if (editingChannel.value) await notificationApi.updateChannel(editingChannel.value.id, payload)
    else await notificationApi.createChannel(payload)
    ElMessage.success(editingChannel.value ? '通知渠道已更新' : '通知渠道已创建')
    channelDialogVisible.value = false
    await loadChannels()
  } finally { channelSaving.value = false }
}

async function testChannel(row) {
  await notificationApi.testChannel(row.id)
  ElMessage.success('测试消息已加入发送队列')
  activeTab.value = 'outbox'
  outboxPage.value = 1
  await loadOutbox()
}

async function deleteChannel(row) {
  await ElMessageBox.confirm(`确定删除通知渠道“${row.name}”？等待发送的任务将被取消。`, '删除通知渠道', { type: 'warning', confirmButtonText: '确认删除' })
  await notificationApi.deleteChannel(row.id)
  ElMessage.success('通知渠道已删除')
  await loadChannels()
}

async function retryOutbox(row) {
  await notificationApi.retryOutbox(row.id)
  ElMessage.success('通知已重新入队')
  await loadOutbox()
}

function channelTypeMeta(value) { return ({ smtp: { label: '邮件', type: 'primary' }, wecom: { label: '企业微信', type: 'warning' }, dingtalk: { label: '钉钉', type: 'success' }, feishu: { label: '飞书', type: 'info' } }[value] || { label: value || '-', type: 'info' }) }
function severityMeta(value) { return ({ info: { label: '提示', type: 'info' }, warning: { label: '警告', type: 'warning' }, critical: { label: '严重', type: 'danger' } }[value] || { label: value || '-', type: 'info' }) }
function outboxStatusMeta(value) { return ({ pending: { label: '等待发送', type: 'info' }, sending: { label: '发送中', type: 'primary' }, sent: { label: '已发送', type: 'success' }, failed: { label: '等待重试', type: 'warning' }, exhausted: { label: '重试耗尽', type: 'danger' }, cancelled: { label: '已取消', type: 'info' } }[value] || { label: value || '-', type: 'info' }) }
function formatDate(value) { return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-' }

onMounted(loadChannels)
</script>

<style scoped>
.notification-page { display: flex; flex-direction: column; gap: 12px; padding-bottom: 8px; min-width: 0; }
.notification-toolbar { min-height: 64px; }
.status-filter { width: 150px; }
.notification-panel { min-width: 0; padding: 0 22px 14px; }
.notification-tabs :deep(.el-tabs__header) { margin-bottom: 0; }
.panel-heading { display: flex; align-items: center; justify-content: space-between; padding: 18px 0 14px; }
.panel-heading strong { display: block; font-size: 15px; color: var(--text-primary); }
.panel-heading p { margin: 4px 0 0; font-size: 12px; color: var(--text-muted); }
.notification-table { min-height: 280px; }
.event-cell { display: flex; flex-direction: column; gap: 3px; min-width: 0; }
.event-cell strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-weight: 600; }
.event-cell small, .inline-meta { color: var(--text-muted); font-size: 11px; }
.inline-meta { display: block; margin-top: 2px; }
.form-grid { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 1fr); gap: 12px; }
.channel-form { margin-top: 14px; }
.channel-form :deep(.el-select), .channel-form :deep(.el-input-number) { width: 100%; }
.channel-status-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: nowrap;
  gap: 16px;
  min-height: 46px;
  margin-bottom: 18px;
  padding: 0 14px;
  border: 1px solid var(--border-color);
  border-radius: 6px;
  background: var(--panel-bg-strong);
}
.channel-status-row:deep(.el-form-item__label-wrap) { margin-left: 0 !important; }
.channel-status-row:deep(.el-form-item__label) {
  flex: 0 0 auto;
  width: auto !important;
  height: auto;
  margin: 0;
  padding: 0;
  color: var(--text-primary);
  font-weight: 600;
  line-height: 1;
  white-space: nowrap;
}
.channel-status-row:deep(.el-form-item__content) {
  display: flex;
  justify-content: flex-end;
  min-height: auto;
  flex: 0 0 auto;
  width: auto !important;
  margin-left: auto !important;
  line-height: 1;
}
.channel-status-select { width: 132px !important; }
.channel-status-row:deep(.el-form-item__error) { display: none; }
.notification-panel .table-pagination { margin-top: 0; padding-top: 14px; border-top: 1px solid var(--border-color); }
@media (max-width: 720px) {
  .notification-panel { padding-inline: 12px; }
  .form-grid { grid-template-columns: 1fr; gap: 0; }
  .status-filter { width: 132px; }
}
</style>
