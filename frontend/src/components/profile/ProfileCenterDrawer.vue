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
  <el-drawer v-model="visible" class="profile-center-drawer" size="min(720px, 100%)" :with-header="false" append-to-body>
    <div class="profile-shell" v-loading="loading">
      <header class="profile-header">
        <div class="profile-identity">
          <el-avatar :size="52" class="profile-avatar">{{ avatarText }}</el-avatar>
          <div class="profile-heading">
            <div class="profile-title-line">
              <h2>{{ profile?.real_name || profile?.username || '个人中心' }}</h2>
              <el-tag :type="isLDAP ? 'primary' : 'success'" effect="plain">{{ isLDAP ? 'LDAP' : '本地账号' }}</el-tag>
              <el-tag v-if="profile?.is_super_admin" type="warning" effect="plain">超级管理员</el-tag>
            </div>
            <span class="profile-username">@{{ profile?.username || '-' }}</span>
          </div>
        </div>
        <div class="profile-header-actions">
          <el-button :icon="Refresh" circle title="刷新个人资料" aria-label="刷新个人资料" :loading="loading" @click="loadProfile" />
          <el-button :icon="Close" circle title="关闭个人中心" aria-label="关闭个人中心" @click="visible = false" />
        </div>
      </header>

      <div class="profile-session-strip">
        <div><span>账号状态</span><strong>{{ profile?.status === 1 ? '正常' : '已停用' }}</strong></div>
        <div><span>登录有效期</span><strong>{{ formatDateTime(profile?.session_expires_at) }}</strong></div>
        <div><span>加入时间</span><strong>{{ formatDateTime(profile?.created_at) }}</strong></div>
      </div>

      <el-tabs v-model="activeTab" class="profile-tabs">
        <el-tab-pane label="个人资料" name="profile">
          <section class="profile-section">
            <div class="section-heading">
              <div>
                <h3>基本信息</h3>
                <span>{{ isLDAP ? '由目录服务同步' : '用于团队成员识别与联系' }}</span>
              </div>
              <ActionButton v-if="!isLDAP && !editing" action="edit" text="修改资料" @click="startEditing" />
            </div>

            <el-alert v-if="isLDAP" type="info" :closable="false" show-icon title="LDAP 账号资料由目录服务统一维护" />

            <el-form v-if="editing" ref="profileFormRef" :model="profileForm" :rules="profileRules" label-position="top" class="profile-form" @submit.prevent="saveProfile">
              <div class="form-grid">
                <el-form-item label="姓名" prop="real_name">
                  <el-input v-model="profileForm.real_name" maxlength="64" show-word-limit />
                </el-form-item>
                <el-form-item label="用户名">
                  <el-input :model-value="profile?.username" disabled />
                </el-form-item>
                <el-form-item label="邮箱" prop="email">
                  <el-input v-model="profileForm.email" maxlength="128" placeholder="name@example.com" />
                </el-form-item>
                <el-form-item label="手机号" prop="phone">
                  <el-input v-model="profileForm.phone" maxlength="32" placeholder="请输入手机号" />
                </el-form-item>
              </div>
              <div class="form-actions">
                <el-button @click="cancelEditing">取消</el-button>
                <ActionButton action="save" text="保存资料" :plain="false" :loading="profileSaving" @click="saveProfile" />
              </div>
            </el-form>

            <el-descriptions v-else :column="compact ? 1 : 2" border class="profile-descriptions">
              <el-descriptions-item label="姓名">{{ profile?.real_name || '-' }}</el-descriptions-item>
              <el-descriptions-item label="用户名">{{ profile?.username || '-' }}</el-descriptions-item>
              <el-descriptions-item label="邮箱">{{ profile?.email || '未填写' }}</el-descriptions-item>
              <el-descriptions-item label="手机号">{{ profile?.phone || '未填写' }}</el-descriptions-item>
              <el-descriptions-item label="账号来源">{{ isLDAP ? 'LDAP 目录服务' : '本地账号' }}</el-descriptions-item>
              <el-descriptions-item label="最近更新">{{ formatDateTime(profile?.updated_at) }}</el-descriptions-item>
            </el-descriptions>
          </section>
        </el-tab-pane>

        <el-tab-pane :label="workspaceTabLabel" name="workspaces">
          <section class="profile-section workspace-section">
            <div class="section-heading">
              <div>
                <h3>工作空间权限</h3>
                <span>当前账号可访问的空间、角色和配额</span>
              </div>
            </div>
            <el-table :data="profile?.workspaces || []" row-key="workspace_id" stripe empty-text="暂未加入工作空间">
              <el-table-column label="工作空间" min-width="150">
                <template #default="{ row }">
                  <div class="workspace-name"><strong>{{ row.name }}</strong><span>{{ row.code }}</span></div>
                </template>
              </el-table-column>
              <el-table-column label="空间身份" min-width="110">
                <template #default="{ row }"><el-tag :type="membershipTag(row).type" effect="plain">{{ membershipTag(row).text }}</el-tag></template>
              </el-table-column>
              <el-table-column label="功能角色" min-width="150">
                <template #default="{ row }">
                  <div v-if="row.functional_roles?.length" class="role-tags">
                    <el-tag v-for="role in row.functional_roles" :key="role.id" type="info" effect="plain">{{ role.name }}</el-tag>
                  </div>
                  <span v-else class="muted-value">未分配</span>
                </template>
              </el-table-column>
              <el-table-column label="个人用量 / 配额" min-width="150">
                <template #default="{ row }">
                  <div class="quota-value"><strong>{{ formatBytes(row.used_bytes) }}</strong><span>/ {{ row.quota_bytes == null ? '无限制' : formatBytes(row.quota_bytes) }}</span></div>
                </template>
              </el-table-column>
            </el-table>
          </section>
        </el-tab-pane>

        <el-tab-pane label="账号安全" name="security">
          <section class="profile-section security-section">
            <div class="section-heading">
              <div>
                <h3>登录密码</h3>
                <span>{{ isLDAP ? '由目录服务验证登录凭据' : '修改后其他设备需要重新登录' }}</span>
              </div>
            </div>
            <el-alert v-if="isLDAP" type="info" :closable="false" show-icon title="请通过公司目录服务修改 LDAP 密码" />
            <el-form v-else ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-position="top" class="password-form" @submit.prevent="submitPassword">
              <el-form-item label="当前密码" prop="current_password">
                <el-input v-model="passwordForm.current_password" type="password" show-password autocomplete="current-password" />
              </el-form-item>
              <el-form-item label="新密码" prop="new_password">
                <el-input v-model="passwordForm.new_password" type="password" show-password autocomplete="new-password" placeholder="12-128 位，至少包含三类字符" />
              </el-form-item>
              <el-form-item label="确认新密码" prop="confirm_password">
                <el-input v-model="passwordForm.confirm_password" type="password" show-password autocomplete="new-password" />
              </el-form-item>
              <div class="form-actions">
                <ActionButton action="key" text="修改密码" :plain="false" :loading="passwordSaving" @click="submitPassword" />
              </div>
            </el-form>
          </section>
        </el-tab-pane>
      </el-tabs>
    </div>
  </el-drawer>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { Close, Refresh } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { changePassword, getProfile, updateProfile } from '../../api/auth'
import { useUserStore } from '../../stores/user'
import { formatDateTime } from '../../utils/date'
import { validateStrongPassword } from '../../utils/validation'

const userStore = useUserStore()
const visible = ref(false)
const loading = ref(false)
const profileSaving = ref(false)
const passwordSaving = ref(false)
const editing = ref(false)
const activeTab = ref('profile')
const profile = ref(null)
const profileFormRef = ref(null)
const passwordFormRef = ref(null)
const compact = ref(typeof window !== 'undefined' && window.innerWidth <= 640)
const profileForm = reactive({ real_name: '', email: '', phone: '' })
const passwordForm = reactive({ current_password: '', new_password: '', confirm_password: '' })

const isLDAP = computed(() => profile.value?.source === 'ldap')
const avatarText = computed(() => String(profile.value?.username || 'U').charAt(0).toUpperCase())
const workspaceTabLabel = computed(() => `工作空间 ${profile.value?.workspaces?.length || 0}`)

const profileRules = {
  real_name: [{ required: true, whitespace: true, message: '请输入姓名', trigger: 'blur' }],
  email: [{ type: 'email', message: '邮箱格式不正确', trigger: ['blur', 'change'] }],
  phone: [{ pattern: /^[0-9+()\-\s]*$/, message: '手机号只能包含数字及 + - ( )', trigger: ['blur', 'change'] }]
}

const passwordRules = {
  current_password: [{ required: true, message: '请输入当前密码', trigger: 'blur' }],
  new_password: [
    { required: true, message: '请输入新密码', trigger: 'blur' },
    { validator: validateStrongPassword, trigger: ['blur', 'change'] }
  ],
  confirm_password: [
    { required: true, message: '请再次输入新密码', trigger: 'blur' },
    { validator: (_rule, value, callback) => callback(value === passwordForm.new_password ? undefined : new Error('两次输入的新密码不一致')), trigger: ['blur', 'change'] }
  ]
}

function open() {
  visible.value = true
  activeTab.value = 'profile'
  editing.value = false
  resetPasswordForm()
  loadProfile()
}

async function loadProfile() {
  loading.value = true
  try {
    const response = await getProfile()
    profile.value = response.data
    syncProfileForm()
  } finally {
    loading.value = false
  }
}

function syncProfileForm() {
  profileForm.real_name = profile.value?.real_name || ''
  profileForm.email = profile.value?.email || ''
  profileForm.phone = profile.value?.phone || ''
}

function startEditing() {
  syncProfileForm()
  editing.value = true
}

function cancelEditing() {
  editing.value = false
  syncProfileForm()
  profileFormRef.value?.clearValidate()
}

async function saveProfile() {
  if (!await profileFormRef.value?.validate().catch(() => false)) return
  profileSaving.value = true
  try {
    const response = await updateProfile({
      real_name: profileForm.real_name.trim(),
      email: profileForm.email.trim(),
      phone: profileForm.phone.trim()
    })
    profile.value = { ...profile.value, ...response.data, updated_at: new Date().toISOString() }
    userStore.applyProfile(response.data)
    editing.value = false
    ElMessage.success(response.message || '个人资料已更新')
  } finally {
    profileSaving.value = false
  }
}

async function submitPassword() {
  if (!await passwordFormRef.value?.validate().catch(() => false)) return
  try {
    await ElMessageBox.confirm('修改密码后，除当前浏览器外的其他登录会话将立即失效。', '确认修改密码', {
      confirmButtonText: '确认修改', cancelButtonText: '取消', type: 'warning'
    })
  } catch {
    return
  }
  passwordSaving.value = true
  try {
    const response = await changePassword({
      current_password: passwordForm.current_password,
      new_password: passwordForm.new_password
    })
    userStore.applySession(response.data)
    resetPasswordForm()
    passwordFormRef.value?.clearValidate()
    await loadProfile()
    ElMessage.success(response.message || '密码已修改')
  } finally {
    passwordSaving.value = false
  }
}

function resetPasswordForm() {
  passwordForm.current_password = ''
  passwordForm.new_password = ''
  passwordForm.confirm_password = ''
}

function membershipTag(row) {
  if (row.membership_role === 'super_admin') return { text: '平台管理员', type: 'warning' }
  if (row.membership_role === 'workspace_admin') return { text: '空间管理员', type: 'success' }
  if (row.is_member) return { text: '空间成员', type: 'primary' }
  return { text: '平台访问', type: 'info' }
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let size = bytes / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[index]}`
}

function syncViewport() {
  compact.value = window.innerWidth <= 640
}

onMounted(() => window.addEventListener('resize', syncViewport))
onBeforeUnmount(() => window.removeEventListener('resize', syncViewport))

defineExpose({ open })
</script>

<style scoped>
.profile-shell { min-height: 100%; display: flex; flex-direction: column; }
.profile-header { display: flex; align-items: center; justify-content: space-between; gap: 16px; padding: 22px 24px 18px; border-bottom: 1px solid var(--border-color); }
.profile-identity, .profile-title-line, .profile-header-actions { display: flex; align-items: center; }
.profile-identity { min-width: 0; gap: 14px; }
.profile-avatar { flex: 0 0 auto; background: var(--accent-primary); color: #fff; font-size: 20px; font-weight: 700; }
.profile-heading { min-width: 0; display: flex; flex-direction: column; gap: 5px; }
.profile-title-line { flex-wrap: wrap; gap: 8px; }
.profile-title-line h2 { max-width: 300px; overflow: hidden; color: var(--text-primary); font-size: 20px; line-height: 1.3; text-overflow: ellipsis; white-space: nowrap; }
.profile-username { color: var(--text-muted); font-size: 13px; }
.profile-header-actions { flex: 0 0 auto; gap: 8px; }
.profile-header-actions .el-button { margin-left: 0; }
.profile-session-strip { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); border-bottom: 1px solid var(--border-color); background: var(--surface-soft); }
.profile-session-strip > div { min-width: 0; padding: 13px 18px; border-right: 1px solid var(--border-color); }
.profile-session-strip > div:last-child { border-right: 0; }
.profile-session-strip span, .profile-session-strip strong { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.profile-session-strip span { margin-bottom: 4px; color: var(--text-muted); font-size: 11px; }
.profile-session-strip strong { color: var(--text-secondary); font-size: 12px; font-weight: 650; }
.profile-tabs { flex: 1; min-height: 0; padding: 0 24px 24px; }
.profile-tabs :deep(.el-tabs__nav-wrap::after) { background: var(--border-color); }
.profile-section { display: flex; flex-direction: column; gap: 18px; padding-top: 4px; }
.section-heading { min-height: 42px; display: flex; align-items: center; justify-content: space-between; gap: 16px; }
.section-heading h3 { margin-bottom: 4px; color: var(--text-primary); font-size: 16px; }
.section-heading span { color: var(--text-muted); font-size: 12px; }
.form-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 0 16px; }
.profile-form, .password-form { max-width: 620px; }
.password-form { width: 100%; max-width: 460px; }
.form-actions { display: flex; align-items: center; justify-content: flex-end; gap: 10px; padding-top: 4px; }
.form-actions .el-button { margin-left: 0; }
.profile-descriptions :deep(.el-descriptions__cell) { padding: 12px 14px !important; }
.workspace-section { min-width: 0; }
.workspace-name, .quota-value { display: flex; flex-direction: column; align-items: center; gap: 3px; }
.workspace-name strong, .quota-value strong { color: var(--text-primary); font-weight: 650; }
.workspace-name span, .quota-value span, .muted-value { color: var(--text-muted); font-size: 12px; }
.role-tags { display: flex; flex-wrap: wrap; justify-content: center; gap: 5px; }
.security-section { align-items: flex-start; }
.security-section .section-heading { width: 100%; }
.security-section .el-alert { width: 100%; }

@media (max-width: 640px) {
  .profile-header { align-items: flex-start; padding: 18px 16px 14px; }
  .profile-avatar { width: 42px !important; height: 42px !important; font-size: 17px; }
  .profile-title-line h2 { max-width: 180px; font-size: 18px; }
  .profile-header-actions .el-button:first-child { display: none; }
  .profile-session-strip { grid-template-columns: 1fr; }
  .profile-session-strip > div { padding: 10px 16px; border-right: 0; border-bottom: 1px solid var(--border-color); }
  .profile-session-strip > div:last-child { border-bottom: 0; }
  .profile-tabs { padding: 0 16px 20px; }
  .profile-tabs :deep(.el-tabs__item) { padding: 0 12px; }
  .form-grid { grid-template-columns: 1fr; }
  .section-heading { align-items: flex-start; }
  .profile-descriptions :deep(.el-descriptions__body table) { table-layout: fixed; }
}
</style>
