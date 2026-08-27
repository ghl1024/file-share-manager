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
  <div class="workspace-container">
    <div class="page-toolbar header-actions">
      <div class="workspace-toolbar-main">
        <span class="workspace-summary">共 {{ total }} 个工作空间</span>
        <el-input
          v-model="keyword"
          class="workspace-search"
          clearable
          :prefix-icon="Search"
          placeholder="搜索名称或空间代号"
          @keyup.enter="handleSearch"
          @clear="handleSearch"
        />
      </div>
      <div class="toolbar-actions">
        <ActionButton action="refresh" :loading="loading" title="刷新工作空间" @click="fetchWorkspaces" />
        <ActionButton v-if="userStore.user?.is_super_admin" action="create" text="新建工作空间" :plain="false" @click="showCreateDialog = true" />
      </div>
    </div>

    <el-card class="workspace-table-card" shadow="never">
      <el-table :data="workspaces" v-loading="loading" style="width: 100%">
        <el-table-column prop="id" label="ID" width="80" />
        <el-table-column prop="name" label="工作空间名称" min-width="190">
          <template #default="scope">
            <div class="workspace-name-cell">
              <span>{{ scope.row.name }}</span>
              <el-tag v-if="userStore.user?.is_super_admin && scope.row.is_member === false" type="danger" effect="plain" size="small">跨空间</el-tag>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="code" label="空间代号" width="150" />
        <el-table-column label="容量使用" min-width="210">
          <template #default="scope">
            <div class="usage-cell">
              <div class="usage-label">
                <span>{{ formatBytes(scope.row.used_bytes) }}</span>
                <span>{{ scope.row.quota_bytes === null ? '无限制' : formatBytes(scope.row.quota_bytes) }}</span>
              </div>
              <el-progress v-if="scope.row.quota_bytes" :percentage="quotaPercent(scope.row)" :show-text="false" :stroke-width="6" />
              <div v-else class="unlimited-track"><span /></div>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100">
          <template #default="scope">
            <el-tag :type="scope.row.status === 1 ? 'success' : 'danger'">
              {{ scope.row.status === 1 ? '活跃' : '停用' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="280" fixed="right">
          <template #default="scope">
            <div class="common-action-group">
              <ActionButton action="enter" @click="enterWorkspace(scope.row)" />
              <ActionButton v-if="canManageWorkspace(scope.row)" action="members" @click="openMemberDialog(scope.row)" />
              <ActionButton v-if="userStore.user?.is_super_admin" action="quota" title="设置配额" @click="openWorkspaceQuota(scope.row)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        v-if="total > 0"
        class="table-pagination"
        :current-page="page"
        :page-size="pageSize"
        :page-sizes="[20, 50, 100]"
        :total="total"
        layout="total, sizes, prev, pager, next"
        @current-change="handlePageChange"
        @size-change="handleSizeChange"
      />
    </el-card>

    <!-- 新建工作空间弹窗 -->
    <el-dialog v-model="showCreateDialog" title="创建工作空间" width="min(500px, calc(100vw - 32px))">
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" maxlength="128" show-word-limit placeholder="例如：研发部共享空间" />
        </el-form-item>
        <el-form-item label="空间代号" prop="code">
          <el-input v-model="form.code" maxlength="64" placeholder="全局唯一英文，如：rd-share" />
        </el-form-item>
        <el-form-item label="配额(GB)" prop="quota_gb">
          <el-input-number v-model="form.quota_gb" :min="0" :step="1" placeholder="留空为不限制" style="width: 100%" />
          <div class="form-tip">设置为 0 或留空代表无容量限制</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="showCreateDialog = false">取消</el-button>
          <el-button type="primary" @click="handleCreate" :loading="creating">确定创建</el-button>
        </span>
      </template>
    </el-dialog>

    <el-dialog v-model="quotaDialogVisible" :title="quotaDialogTitle" width="min(440px, calc(100vw - 32px))">
      <el-form label-position="top">
        <el-form-item label="容量配额 (GB)">
          <el-input-number v-model="quotaForm.quota_gb" :min="0" :max="8388607" :precision="2" :step="1" placeholder="无限制" style="width: 100%" />
          <div class="form-tip">设置为 0 或留空代表无限制，不能低于当前已用和预留容量。</div>
        </el-form-item>
      </el-form>
      <div v-if="quotaTarget" class="quota-summary">
        <span>当前已用 <strong>{{ formatBytes(quotaTarget.used_bytes || 0) }}</strong></span>
        <span>上传预留 <strong>{{ formatBytes(quotaTarget.reserved_bytes || 0) }}</strong></span>
      </div>
      <template #footer>
        <el-button @click="quotaDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="quotaSaving" @click="saveQuota">保存配额</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="memberDialogVisible" :title="`${selectedWorkspace?.name || ''} 成员与用户组`" width="min(960px, calc(100vw - 32px))">
      <el-tabs v-model="memberTab">
        <el-tab-pane label="成员" name="members">
          <div class="dialog-toolbar">
            <el-input v-model="memberKeyword" clearable placeholder="搜索用户名、姓名或邮箱" style="width: 280px" @keyup.enter="loadMembers" />
            <ActionButton action="add" text="添加成员" :plain="false" @click="openMemberForm" />
          </div>
          <el-table :data="members" v-loading="membersLoading" stripe border>
            <el-table-column prop="username" label="用户名" min-width="130" />
            <el-table-column prop="real_name" label="姓名" min-width="120" />
            <el-table-column prop="email" label="邮箱" min-width="180" />
            <el-table-column prop="role" label="空间角色" width="130">
              <template #default="{ row }"><el-tag :type="row.role === 'workspace_admin' ? 'warning' : 'info'">{{ row.role === 'workspace_admin' ? '空间管理员' : '成员' }}</el-tag></template>
            </el-table-column>
            <el-table-column label="配额" width="130">
              <template #default="{ row }">{{ row.quota_bytes === null ? '跟随空间' : formatBytes(row.quota_bytes) }}</template>
            </el-table-column>
            <el-table-column label="操作" width="180" fixed="right">
              <template #default="{ row }">
                <div class="common-action-group">
                  <ActionButton action="quota" @click="openMemberQuota(row)" />
                  <ActionButton action="delete" text="移除" @click="removeMember(row)" />
                </div>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination v-if="memberTotal > 0" class="table-pagination" v-model:current-page="memberPage" v-model:page-size="memberPageSize" :total="memberTotal" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @change="loadMembers" />
        </el-tab-pane>

        <el-tab-pane label="用户组" name="groups">
          <div class="dialog-toolbar">
            <el-input v-model="groupKeyword" clearable placeholder="搜索用户组" style="width: 280px" @keyup.enter="loadGroups" />
            <ActionButton action="add" text="新建用户组" :plain="false" @click="openGroupForm()" />
          </div>
          <el-table :data="groups" v-loading="groupsLoading" stripe border>
            <el-table-column prop="name" label="用户组名称" min-width="180" />
            <el-table-column label="来源" width="90">
              <template #default="{ row }"><el-tag :type="isManagedGroup(row) ? 'primary' : 'success'" effect="plain">{{ isManagedGroup(row) ? 'LDAP' : '本地' }}</el-tag></template>
            </el-table-column>
            <el-table-column prop="description" label="描述" min-width="260" show-overflow-tooltip />
            <el-table-column label="操作" width="285" fixed="right">
              <template #default="{ row }">
                <div class="common-action-group">
                  <ActionButton action="members" @click="openGroupMembers(row)" />
                  <ActionButton v-if="!isManagedGroup(row)" action="edit" @click="openGroupForm(row)" />
                  <ActionButton v-if="!isManagedGroup(row)" action="delete" @click="removeGroup(row)" />
                  <el-tag v-else type="info" effect="plain">同步管理</el-tag>
                </div>
              </template>
            </el-table-column>
          </el-table>
          <el-pagination v-if="groupTotal > 0" class="table-pagination" v-model:current-page="groupPage" v-model:page-size="groupPageSize" :total="groupTotal" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @change="loadGroups" />
        </el-tab-pane>
      </el-tabs>
    </el-dialog>

    <el-dialog v-model="memberFormVisible" title="添加工作空间成员" width="min(460px, calc(100vw - 32px))">
      <el-form :model="memberForm" label-width="90px">
        <el-form-item label="用户" required><el-select v-model="memberForm.user_id" filterable placeholder="选择用户" style="width: 100%"><el-option v-for="user in availableUsers" :key="user.id" :label="`${user.real_name || user.username} (${user.username})`" :value="user.id" /></el-select></el-form-item>
        <el-form-item label="角色"><el-radio-group v-model="memberForm.role"><el-radio value="member">成员</el-radio><el-radio value="workspace_admin">空间管理员</el-radio></el-radio-group></el-form-item>
        <el-form-item label="配额(GB)"><el-input-number v-model="memberForm.quota_gb" :min="0" :step="1" style="width: 100%" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="memberFormVisible = false">取消</el-button><el-button type="primary" :loading="memberSaving" @click="saveMember">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="groupFormVisible" :title="editingGroup ? '编辑用户组' : '新建用户组'" width="min(460px, calc(100vw - 32px))" @closed="resetGroupForm">
      <el-form :model="groupForm" label-width="90px">
        <el-form-item label="名称" required><el-input v-model="groupForm.name" maxlength="128" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="groupForm.description" type="textarea" maxlength="1000" /></el-form-item>
      </el-form>
      <template #footer><el-button @click="groupFormVisible = false">取消</el-button><el-button type="primary" :loading="groupSaving" @click="saveGroup">保存</el-button></template>
    </el-dialog>

    <el-dialog v-model="groupMembersVisible" :title="`${selectedGroup?.name || ''} 成员`" width="min(720px, calc(100vw - 32px))">
      <div class="dialog-toolbar">
        <el-tag v-if="isManagedGroup(selectedGroup)" type="info" effect="plain">LDAP 同步管理</el-tag>
        <ActionButton v-else action="add" text="添加成员" :plain="false" @click="openGroupMemberForm" />
      </div>
      <el-table :data="groupMembers" v-loading="groupMembersLoading" stripe border>
        <el-table-column prop="username" label="用户名" min-width="140" /><el-table-column prop="real_name" label="姓名" min-width="120" /><el-table-column prop="email" label="邮箱" min-width="180" />
        <el-table-column label="操作" width="110"><template #default="{ row }"><ActionButton v-if="!isManagedGroup(selectedGroup)" action="delete" text="移除" @click="removeGroupMemberRow(row)" /><el-tag v-else type="info" effect="plain">同步管理</el-tag></template></el-table-column>
      </el-table>
      <el-pagination v-if="groupMemberTotal > 0" class="table-pagination" v-model:current-page="groupMemberPage" v-model:page-size="groupMemberPageSize" :total="groupMemberTotal" :page-sizes="[20, 50, 100]" layout="total, sizes, prev, pager, next" @change="loadGroupMembers" />
    </el-dialog>

    <el-dialog v-model="groupMemberFormVisible" title="添加用户组成员" width="min(420px, calc(100vw - 32px))">
      <el-select v-model="groupMemberUserID" filterable placeholder="选择工作空间成员" style="width: 100%"><el-option v-for="user in members" :key="user.user_id" :label="`${user.real_name || user.username} (${user.username})`" :value="user.user_id" /></el-select>
      <template #footer><el-button @click="groupMemberFormVisible = false">取消</el-button><el-button type="primary" @click="saveGroupMember">保存</el-button></template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, ref, reactive, onMounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { useRouter } from 'vue-router'
import { getWorkspaces, createWorkspace, updateWorkspaceQuota, addWorkspaceMember, getWorkspaceMembers, getWorkspaceAvailableUsers, removeWorkspaceMember, updateWorkspaceMemberQuota, getWorkspaceGroups, createWorkspaceGroup, updateWorkspaceGroup, deleteWorkspaceGroup, addGroupMember, getGroupMembers, removeGroupMember } from '@/api/workspace'
import { useUserStore } from '@/stores/user'
import { usePagination } from '@/composables/usePagination'
import ActionButton from '@/components/common/ActionButton.vue'

const userStore = useUserStore()
const router = useRouter()
const showCreateDialog = ref(false)
const creating = ref(false)
const formRef = ref(null)
const memberDialogVisible = ref(false)
const memberFormVisible = ref(false)
const groupFormVisible = ref(false)
const groupMembersVisible = ref(false)
const groupMemberFormVisible = ref(false)
const quotaDialogVisible = ref(false)
const quotaSaving = ref(false)
const quotaDialogMode = ref('workspace')
const quotaTarget = ref(null)
const memberTab = ref('members')
const selectedWorkspace = ref(null)
const selectedGroup = ref(null)
const editingGroup = ref(null)
const members = ref([])
const groups = ref([])
const groupMembers = ref([])
const availableUsers = ref([])
const membersLoading = ref(false)
const groupsLoading = ref(false)
const groupMembersLoading = ref(false)
const memberSaving = ref(false)
const groupSaving = ref(false)
const memberKeyword = ref('')
const groupKeyword = ref('')
const memberPage = ref(1)
const memberPageSize = ref(20)
const memberTotal = ref(0)
const groupPage = ref(1)
const groupPageSize = ref(20)
const groupTotal = ref(0)
const groupMemberPage = ref(1)
const groupMemberPageSize = ref(20)
const groupMemberTotal = ref(0)
const groupMemberUserID = ref(null)
const memberForm = reactive({ user_id: null, role: 'member', quota_gb: null })
const groupForm = reactive({ name: '', description: '' })
const quotaForm = reactive({ quota_gb: null })

const form = ref({
  name: '',
  code: '',
  quota_gb: null
})

const rules = {
  name: [
    { required: true, message: '请输入工作空间名称', trigger: 'blur' },
    { max: 128, message: '名称不能超过 128 个字符', trigger: 'blur' }
  ],
  code: [
    { required: true, message: '请输入空间代号', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9-]{2,63}$/, message: '以小写字母开头，仅包含小写字母、数字和连字符', trigger: 'blur' }
  ]
}

const { list: workspaces, loading, page, pageSize, total, keyword, load, handleSearch, handleSizeChange: loadBySize } = usePagination(getWorkspaces, { pageSize: 20 })

const fetchWorkspaces = () => load()
const handlePageChange = (value) => {
  page.value = value
  load()
}
const handleSizeChange = (value) => {
  pageSize.value = value
  loadBySize()
}

const canManageWorkspace = (workspace) => userStore.user?.is_super_admin || workspace.current_role === 'workspace_admin'
const isManagedGroup = (group) => Boolean(group?.source && group.source !== 'local')
const quotaDialogTitle = computed(() => {
  if (quotaDialogMode.value === 'workspace') return `工作空间配额：${quotaTarget.value?.name || ''}`
  return `成员配额：${quotaTarget.value?.real_name || quotaTarget.value?.username || ''}`
})

const bytesToGigabytes = (bytes) => bytes === null || bytes === undefined ? null : Number((bytes / (1024 ** 3)).toFixed(2))

const openWorkspaceQuota = (workspace) => {
  quotaDialogMode.value = 'workspace'
  quotaTarget.value = workspace
  quotaForm.quota_gb = bytesToGigabytes(workspace.quota_bytes)
  quotaDialogVisible.value = true
}

const openMemberQuota = (member) => {
  quotaDialogMode.value = 'member'
  quotaTarget.value = member
  quotaForm.quota_gb = bytesToGigabytes(member.quota_bytes)
  quotaDialogVisible.value = true
}

const saveQuota = async () => {
  if (!quotaTarget.value) return
  const gigabytes = Number(quotaForm.quota_gb || 0)
  if (!Number.isFinite(gigabytes) || gigabytes < 0) return ElMessage.warning('请输入有效的配额')
  const quotaBytes = gigabytes > 0 ? Math.round(gigabytes * (1024 ** 3)) : null
  quotaSaving.value = true
  try {
    if (quotaDialogMode.value === 'workspace') {
      await updateWorkspaceQuota(quotaTarget.value.id, quotaBytes)
      quotaTarget.value.quota_bytes = quotaBytes
      const sessionWorkspace = userStore.workspaces.find((item) => item.id === quotaTarget.value.id)
      if (sessionWorkspace) sessionWorkspace.quota_bytes = quotaBytes
    } else {
      await updateWorkspaceMemberQuota(selectedWorkspace.value.id, quotaTarget.value.user_id, quotaBytes)
      quotaTarget.value.quota_bytes = quotaBytes
    }
    ElMessage.success('配额已更新')
    quotaDialogVisible.value = false
  } finally {
    quotaSaving.value = false
  }
}

const openMemberDialog = async (workspace) => {
  selectedWorkspace.value = workspace
  memberTab.value = 'members'
  memberPage.value = 1
  groupPage.value = 1
  memberDialogVisible.value = true
  await Promise.all([loadMembers(), loadGroups(), loadAvailableUsers()])
}

const loadMembers = async () => {
  if (!selectedWorkspace.value) return
  membersLoading.value = true
  try {
    const res = await getWorkspaceMembers(selectedWorkspace.value.id, { page: memberPage.value, page_size: memberPageSize.value, keyword: memberKeyword.value })
    members.value = res.data.list || []
    memberTotal.value = res.data.total || 0
  } finally { membersLoading.value = false }
}

const loadGroups = async () => {
  if (!selectedWorkspace.value) return
  groupsLoading.value = true
  try {
    const res = await getWorkspaceGroups(selectedWorkspace.value.id, { page: groupPage.value, page_size: groupPageSize.value, keyword: groupKeyword.value })
    groups.value = res.data.list || []
    groupTotal.value = res.data.total || 0
  } finally { groupsLoading.value = false }
}

const loadAvailableUsers = async () => {
  if (!selectedWorkspace.value) return
  const res = await getWorkspaceAvailableUsers(selectedWorkspace.value.id, { page: 1, page_size: 200 })
  availableUsers.value = res.data.list || []
}

const openMemberForm = () => {
  Object.assign(memberForm, { user_id: null, role: 'member', quota_gb: null })
  memberFormVisible.value = true
}

const saveMember = async () => {
  if (!selectedWorkspace.value || !memberForm.user_id) return ElMessage.warning('请选择用户')
  memberSaving.value = true
  try {
    const quotaBytes = memberForm.quota_gb > 0 ? memberForm.quota_gb * 1024 * 1024 * 1024 : null
    await addWorkspaceMember(selectedWorkspace.value.id, { user_id: memberForm.user_id, role: memberForm.role, quota_bytes: quotaBytes })
    ElMessage.success('成员已保存')
    memberFormVisible.value = false
    await loadMembers()
  } finally { memberSaving.value = false }
}

const removeMember = async (row) => {
  await ElMessageBox.confirm(`确定移除成员 ${row.username} 吗？`, '确认操作', { type: 'warning' })
  await removeWorkspaceMember(selectedWorkspace.value.id, row.user_id)
  ElMessage.success('成员已移除')
  await loadMembers()
}

const openGroupForm = (group = null) => {
  editingGroup.value = group
  Object.assign(groupForm, { name: group?.name || '', description: group?.description || '' })
  groupFormVisible.value = true
}

const resetGroupForm = () => {
  editingGroup.value = null
  Object.assign(groupForm, { name: '', description: '' })
}

const saveGroup = async () => {
  if (!selectedWorkspace.value || !groupForm.name.trim()) return ElMessage.warning('请输入用户组名称')
  groupSaving.value = true
  try {
    const payload = { name: groupForm.name.trim(), description: groupForm.description.trim() }
    if (editingGroup.value) await updateWorkspaceGroup(selectedWorkspace.value.id, editingGroup.value.id, payload)
    else await createWorkspaceGroup(selectedWorkspace.value.id, payload)
    ElMessage.success(editingGroup.value ? '用户组已更新' : '用户组已创建')
    groupFormVisible.value = false
    await loadGroups()
  } finally { groupSaving.value = false }
}

const removeGroup = async (row) => {
  await ElMessageBox.confirm(`确定删除用户组“${row.name}”吗？组成员关系和引用该组的目录授权将同步移除。`, '删除用户组', { type: 'warning', confirmButtonText: '确认删除' })
  await deleteWorkspaceGroup(selectedWorkspace.value.id, row.id)
  ElMessage.success('用户组已删除')
  await loadGroups()
}

const openGroupMembers = async (group) => {
  selectedGroup.value = group
  groupMemberPage.value = 1
  groupMembersVisible.value = true
  await loadGroupMembers()
}

const loadGroupMembers = async () => {
  if (!selectedWorkspace.value || !selectedGroup.value) return
  groupMembersLoading.value = true
  try {
    const res = await getGroupMembers(selectedWorkspace.value.id, selectedGroup.value.id, { page: groupMemberPage.value, page_size: groupMemberPageSize.value })
    groupMembers.value = res.data.list || []
    groupMemberTotal.value = res.data.total || 0
  } finally { groupMembersLoading.value = false }
}

const openGroupMemberForm = () => { groupMemberUserID.value = null; groupMemberFormVisible.value = true }
const saveGroupMember = async () => {
  if (!groupMemberUserID.value) return ElMessage.warning('请选择工作空间成员')
  await addGroupMember(selectedWorkspace.value.id, selectedGroup.value.id, { user_id: groupMemberUserID.value })
  ElMessage.success('成员已加入用户组')
  groupMemberFormVisible.value = false
  await loadGroupMembers()
}
const removeGroupMemberRow = async (row) => {
  await ElMessageBox.confirm(`确定移除 ${row.username} 吗？`, '确认操作', { type: 'warning' })
  await removeGroupMember(selectedWorkspace.value.id, selectedGroup.value.id, row.user_id)
  ElMessage.success('成员已移除')
  await loadGroupMembers()
}

const handleCreate = async () => {
  if (!formRef.value) return
  await formRef.value.validate(async (valid) => {
    if (valid) {
      creating.value = true
      try {
        let quotaBytes = null
        if (form.value.quota_gb && form.value.quota_gb > 0) {
          quotaBytes = form.value.quota_gb * 1024 * 1024 * 1024
        }
        
        await createWorkspace({
          name: form.value.name,
          code: form.value.code,
          quota_bytes: quotaBytes,
          owner_id: userStore.user?.id
        })
        
        ElMessage.success('创建成功')
        showCreateDialog.value = false
        formRef.value.resetFields()
        fetchWorkspaces()
      } catch (error) {
        console.error('创建失败', error)
      } finally {
        creating.value = false
      }
    }
  })
}

const enterWorkspace = async (row) => {
  try {
    const reason = await crossWorkspaceReason(row)
    if (reason === null) return
    await userStore.switchWorkspace(row.id, reason)
    await router.push('/files')
  } catch (error) {
    console.error('切换工作空间失败', error)
  }
}

const crossWorkspaceReason = async (workspace) => {
  if (!userStore.user?.is_super_admin || workspace?.is_member !== false) return ''
  try {
    const { value } = await ElMessageBox.prompt(
      `你不是“${workspace.name}”的成员，本次进入及文件读取将记录高风险安全审计。`,
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

const formatBytes = (bytes) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

const quotaPercent = (workspace) => {
  const quota = Number(workspace.quota_bytes || 0)
  if (!quota) return 0
  return Math.min(100, Math.round((Number(workspace.used_bytes || 0) / quota) * 100))
}

onMounted(() => {
  fetchWorkspaces()
})
</script>

<style scoped>
.workspace-container {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 0;
}
.header-actions {
  flex-shrink: 0;
}
.workspace-toolbar-main {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 14px;
}
.workspace-search {
  width: 260px;
}
.workspace-summary {
  color: var(--text-secondary);
  font-size: 13px;
}

.workspace-name-cell {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  max-width: 100%;
}
.workspace-table-card :deep(.el-card__body) {
  padding: 0;
}
.workspace-table-card :deep(.el-table) {
  border: 0;
  border-radius: 0 !important;
}
.workspace-table-card :deep(.table-pagination) {
  margin-top: 0;
  padding: 12px 14px;
  border-top: 1px solid var(--border-color);
}
.usage-cell {
  max-width: 230px;
}
.usage-label {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 7px;
  color: var(--text-secondary);
  font-size: 12px;
}
.unlimited-track {
  height: 6px;
  overflow: hidden;
  border-radius: 3px;
  background: var(--surface-muted);
}
.unlimited-track span {
  display: block;
  width: 28%;
  height: 100%;
  background: var(--accent-green);
}
.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}
.dialog-toolbar {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}
.quota-summary {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  padding-top: 12px;
  border-top: 1px solid var(--panel-border);
  color: var(--text-secondary);
  font-size: 13px;
}
.quota-summary strong {
  margin-left: 4px;
  color: var(--text-primary);
}

@media (max-width: 640px) {
  .workspace-toolbar-main {
    width: 100%;
    align-items: stretch;
    flex-direction: column;
    gap: 8px;
  }

  .workspace-search {
    width: 100%;
  }

  .dialog-toolbar {
    align-items: stretch;
    flex-direction: column;
  }

  .dialog-toolbar :deep(.el-input) {
    width: 100% !important;
  }

  .quota-summary {
    flex-direction: column;
    gap: 8px;
  }
}
</style>
