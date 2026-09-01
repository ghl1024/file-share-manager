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
    <!-- 操作栏 -->
    <el-card class="search-card user-toolbar" shadow="never">
      <el-form :inline="true" class="search-form" @submit.prevent="applyFilters">
        <div class="page-toolbar__group user-filter-group">
          <el-form-item label="用户">
            <el-input
              v-model="keyword"
              placeholder="搜索用户名、姓名或邮箱"
              :prefix-icon="Search"
              clearable
              class="filter-input"
              @keyup.enter="applyFilters"
              @clear="applyFilters"
            />
          </el-form-item>
          <el-form-item>
            <ActionButton action="search" @click="applyFilters" />
          </el-form-item>
        </div>

        <div class="toolbar-actions user-toolbar-actions">
          <el-form-item>
            <el-tooltip
              :content="selectedIds.length ? '仅可关联已加入当前工作空间的用户' : '请先勾选当前工作空间成员'"
              placement="top"
              :disabled="selectedIds.length > 0"
            >
              <span>
                <ActionButton action="assignRole" text="批量关联角色" v-permission="'system:user:assign_role'" :disabled="!selectedIds.length" @click="openBatchRoleDialog" />
              </span>
            </el-tooltip>
          </el-form-item>
          <el-form-item>
            <ActionButton action="create" text="新增用户" :plain="false" v-permission="'system:user:create'" @click="openDialog()" />
          </el-form-item>
        </div>
      </el-form>
    </el-card>

    <!-- 列表栏 -->
    <el-card class="table-card" shadow="never">
        <el-table :data="list" v-loading="loading" stripe border @selection-change="handleSelectionChange">
          <el-table-column type="selection" width="55" :selectable="canSelectForRole" align="center" header-align="center" />
          <el-table-column prop="real_name" label="姓名" min-width="120">
            <template #default="{ row }"><span v-copy>{{ row.real_name || '-' }}</span></template>
          </el-table-column>
          <el-table-column prop="username" label="用户名" min-width="120" sortable>
            <template #default="{ row }"><span v-copy>{{ row.username || '-' }}</span></template>
          </el-table-column>
          <el-table-column prop="email" label="邮箱" min-width="180">
            <template #default="{ row }">
              <span v-copy>{{ row.email || '-' }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="phone" label="手机号" min-width="135">
            <template #default="{ row }"><span v-copy>{{ row.phone || '-' }}</span></template>
          </el-table-column>

          <el-table-column prop="is_super_admin" label="超管" width="80">
            <template #default="{ row }">
              <el-tag v-if="row.is_super_admin" type="danger" effect="dark" size="small">是</el-tag>
              <el-tag v-else type="info" effect="plain" size="small">否</el-tag>
            </template>
          </el-table-column>

          <el-table-column prop="source" label="来源" width="100">
            <template #default="{ row }">
              <el-tag :type="row.source === 'ldap' ? 'primary' : 'success'" effect="plain" size="small">
                {{ row.source === 'ldap' ? 'LDAP' : '本地' }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="角色" min-width="180">
            <template #default="{ row }">
              <el-tag v-if="!row.workspace_member" type="info" effect="plain" size="small">非当前空间成员</el-tag>
              <div class="admin-inline-tags" v-else-if="row.roles?.length">
                <el-tag v-for="role in row.roles" :key="role.id" type="info" effect="light" size="small">
                  {{ role.name }}
                </el-tag>
              </div>
              <span v-else class="admin-muted">未分配角色</span>
            </template>
          </el-table-column>

          <el-table-column prop="status" label="状态" width="100">
            <template #default="{ row }">
              <el-switch
                v-model="row.status"
                :active-value="1"
                :inactive-value="0"
                :disabled="row.is_super_admin"
                @change="handleStatusChange(row)"
              />
            </template>
          </el-table-column>

          <el-table-column prop="created_at" label="创建时间" width="170" sortable>
            <template #default="{ row }">
              <span class="admin-time">{{ formatDate(row.created_at) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="360" fixed="right">
            <template #default="{ row }">
              <div class="admin-action-group">
                <ActionButton action="edit" v-permission="'system:user:update'" @click="openDialog(row)" />
                <ActionButton action="resetPassword" v-permission="'system:user:update'" @click="handleResetPwd(row)" />
                <ActionButton action="delete" v-permission="'system:user:delete'" :disabled="row.is_super_admin" @click="handleDelete(row)" />
              </div>
            </template>
          </el-table-column>
        </el-table>

      <!-- 分页组件 -->
      <div class="pagination-wrapper">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="pageSize"
          :total="total"
          layout="total, sizes, prev, pager, next, jumper"
          :page-sizes="[20, 50, 100, 200]"
          @change="fetchList"
        />
      </div>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑用户' : '新增用户'"
      width="min(480px, calc(100vw - 32px))"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="100px" autocomplete="off">
        <el-form-item label="姓名" prop="real_name">
          <el-input v-model="form.real_name" placeholder="请输入真实姓名" />
        </el-form-item>
        <el-form-item label="用户名" prop="username">
          <el-input
            v-model="form.username"
            name="fileshare-user-username"
            autocomplete="off"
            :disabled="!!editingId"
            placeholder="用于系统登录的账号"
          />
        </el-form-item>
        <el-form-item label="密码" prop="password" v-if="!editingId">
		  <el-input
            v-model="form.password"
            name="fileshare-user-new-password"
            type="password"
            autocomplete="new-password"
            show-password
            placeholder="长度不少于 12 位，至少包含三类字符"
          />
        </el-form-item>
        <el-form-item label="邮箱" prop="email">
          <el-input v-model="form.email" placeholder="example@example.com" />
        </el-form-item>
        <el-form-item label="手机号" prop="phone">
          <el-input v-model="form.phone" maxlength="32" placeholder="选填，用于账号联系信息" />
        </el-form-item>
        <el-form-item label="账号状态" v-if="!editingId">
          <el-switch
            v-model="form.status"
            :active-value="1"
            :inactive-value="0"
            active-text="开启"
            inactive-text="禁用"
            inline-prompt
          />
        </el-form-item>
        <el-form-item v-if="editingId" label="空间角色">
          <div class="role-editor">
            <el-select
              v-if="canEditRoles"
              v-model="form.role_ids"
              multiple
              clearable
              collapse-tags
              collapse-tags-tooltip
              placeholder="选择当前工作空间角色"
            >
              <el-option v-for="role in allRoles" :key="role.id" :label="role.name" :value="role.id" />
            </el-select>
            <el-alert
              v-else
              type="info"
              :closable="false"
              show-icon
              :title="roleEditHint"
            />
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 批量关联角色弹窗 -->
    <el-dialog v-model="batchRoleVisible" title="批量关联角色" width="min(420px, calc(100vw - 32px))">
      <el-alert class="role-dialog-alert" title="角色只作用于当前工作空间，非空间成员需先在工作空间页面添加；本操作会覆盖所选用户当前角色。" type="warning" :closable="false" show-icon />
      <el-form label-position="top">
        <el-form-item :label="`已选用户 (${selectedIds.length} 人)`">
          <el-checkbox-group v-model="batchRoleForm.role_ids" class="role-checkbox-group">
            <el-checkbox v-for="role in allRoles" :key="role.id" :value="role.id" :label="role.id">
              {{ role.name }}
            </el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="batchRoleVisible = false">取消</el-button>
        <el-button type="primary" :loading="batchSubmitting" @click="handleBatchAssign">确认关联</el-button>
      </template>
    </el-dialog>

  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import { userApi } from '@/api/user'
import { roleApi } from '@/api/role'
import ActionButton from '@/components/common/ActionButton.vue'
import { useUserStore } from '@/stores/user'
import { strongPasswordError, validateStrongPassword } from '@/utils/validation'

const list = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const dialogVisible = ref(false)
const editingId = ref(null)
const formRef = ref(null)
const allRoles = ref([])
const selectedIds = ref([])
const batchRoleVisible = ref(false)
const submitting = ref(false)
const batchSubmitting = ref(false)
const userStore = useUserStore()
const editingUser = ref(null)
const batchRoleForm = reactive({
  role_ids: []
})

const form = reactive({
  username: '',
  password: '',
  real_name: '',
  email: '',
	phone: '',
	status: 1,
  role_ids: []
})

const rules = {
  real_name: [
    { required: true, message: '姓名必填', trigger: 'blur' },
    { max: 64, message: '姓名不能超过 64 个字符', trigger: 'blur' }
  ],
  username: [
    { required: true, message: '用户名必填', trigger: 'blur' },
    { min: 3, max: 64, message: '用户名长度必须在 3 到 64 个字符之间', trigger: 'blur' }
  ],
  password: [
    { required: true, message: '密码必填', trigger: 'blur' },
    { validator: validateStrongPassword, trigger: 'blur' }
  ],
  email: [{ type: 'email', message: '请输入有效的邮箱地址', trigger: 'blur' }],
  phone: [{ max: 32, message: '手机号不能超过 32 个字符', trigger: 'blur' }]
}

onMounted(() => {
  fetchList()
  if (userStore.currentWorkspaceId) fetchRoles()
})

const canEditRoles = computed(() => {
  return Boolean(editingId.value && editingUser.value?.workspace_member && !editingUser.value?.is_super_admin && userStore.currentWorkspaceId)
})

const roleEditHint = computed(() => {
  if (!userStore.currentWorkspaceId) return '请先在顶部选择工作空间，再为用户关联空间角色。'
  if (editingUser.value?.is_super_admin) return '超级管理员拥有全部权限，不需要关联空间角色。'
  if (!editingUser.value?.workspace_member) return '该用户不是当前工作空间成员，请先在工作空间页面添加成员。'
  return '当前用户暂不可分配角色。'
})

async function fetchList() {
  loading.value = true
  try {
    const res = await userApi.list({
      page: page.value,
      page_size: pageSize.value,
      keyword: keyword.value
    })
    list.value = res.data.list || []
    total.value = res.data.total
  } catch (error) {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

async function fetchRoles() {
  try {
    const res = await roleApi.listAll()
    allRoles.value = (res.data?.list || []).filter((role) => role.code !== 'super_admin')
  } catch (error) {
    allRoles.value = []
  }
}

function applyFilters() {
  page.value = 1
  fetchList()
}

function openDialog(row) {
  editingId.value = row ? row.id : null
  editingUser.value = row || null
  Object.assign(
    form,
    row
      ? {
          username: row.username,
          real_name: row.real_name,
          email: row.email,
          phone: row.phone,
          status: row.status,
          password: '',
          role_ids: (row.roles || []).map((role) => role.id)
        }
      : {
          username: '',
          password: '',
          real_name: '',
          email: '',
		  phone: '',
		  status: 1,
          role_ids: []
        }
  )
  dialogVisible.value = true
}

async function handleStatusChange(row) {
  const nextStatus = row.status
  const previousStatus = nextStatus === 1 ? 0 : 1
  const actionText = nextStatus === 1 ? '启用' : '禁用'
  try {
    await ElMessageBox.confirm(`确定${actionText}用户“${row.username}”？`, `${actionText}用户`, {
      type: nextStatus === 1 ? 'info' : 'warning',
      confirmButtonText: `确认${actionText}`
    })
    await userApi.updateStatus(row.id, { status: row.status })
    ElMessage.success('状态更新成功')
  } catch (error) {
    row.status = previousStatus
    if (error !== 'cancel') console.error(error)
  }
}

import { formatDateTime } from '../../utils/date'

function formatDate(date) {
  return formatDateTime(date)
}

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  submitting.value = true
  try {
    if (editingId.value) {
      await userApi.update(editingId.value, { real_name: form.real_name, email: form.email, phone: form.phone })
      if (canEditRoles.value) {
        await userApi.assignRoles(editingId.value, { role_ids: form.role_ids })
      }
    } else {
      await userApi.create({
        username: form.username,
        password: form.password,
        real_name: form.real_name,
        email: form.email,
        phone: form.phone
      })
    }

    ElMessage.success('操作成功')
    dialogVisible.value = false
    fetchList()
  } catch {
    // The request interceptor presents the API error.
  } finally {
    submitting.value = false
  }
}


async function handleResetPwd(row) {
  try {
    await ElMessageBox.confirm(`确定重置用户“${row.username}”的密码？`, '重置密码', {
      type: 'warning',
      confirmButtonText: '继续重置'
    })
    const { value } = await ElMessageBox.prompt('输入新密码', '重置密码', {
      inputType: 'password',
      inputPlaceholder: '12-128 位，至少包含三类字符',
      inputValidator: (password) => strongPasswordError(password) || true
    })
    if (value) {
      await userApi.resetPassword(row.id, { password: value })
      ElMessage.success('重置成功')
    }
  } catch (error) {
    if (error !== 'cancel') console.error(error)
  }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除用户“${row.username}”？删除后会移除其角色、空间成员和用户组关系。`, '删除用户', {
      type: 'warning',
      confirmButtonText: '确认删除',
      confirmButtonClass: 'el-button--danger'
    })
    await userApi.delete(row.id)
    ElMessage.success('删除成功')
    fetchList()
  } catch (error) {
    if (error !== 'cancel') console.error(error)
  }
}

function handleSelectionChange(selection) {
  selectedIds.value = selection.map((item) => item.id)
}

function canSelectForRole(row) {
  return Boolean(row.workspace_member)
}

function openBatchRoleDialog() {
  batchRoleForm.role_ids = []
  batchRoleVisible.value = true
}

async function handleBatchAssign() {
  if (!batchRoleForm.role_ids.length) {
    ElMessage.warning('请选择至少一个角色')
    return
  }
  batchSubmitting.value = true
  try {
    await ElMessageBox.confirm(`将覆盖 ${selectedIds.value.length} 个用户在当前工作空间的角色，是否继续？`, '批量关联角色', {
      type: 'warning',
      confirmButtonText: '确认关联'
    })
    await userApi.batchAssignRoles({
      user_ids: selectedIds.value,
      role_ids: batchRoleForm.role_ids
    })
    ElMessage.success('批量关联成功')
    batchRoleVisible.value = false
    fetchList()
  } catch {
    // The request interceptor presents the API error.
  } finally {
    batchSubmitting.value = false
  }
}
</script>

<style scoped>
.filter-input {
  width: 240px;
}

.user-toolbar :deep(.el-form) {
  justify-content: space-between;
  width: 100%;
}

.user-filter-group,
.user-toolbar-actions {
  min-width: 0;
}

.user-toolbar-actions {
  margin-left: auto;
}

.role-editor,
.role-editor .el-select {
  width: 100%;
}

.role-checkbox-group {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.role-dialog-alert {
  margin-bottom: 16px;
}

@media (max-width: 640px) {
  .filter-input {
    width: 100%;
  }

  .user-toolbar :deep(.el-form),
  .user-filter-group,
  .user-toolbar-actions {
    align-items: stretch;
    flex-direction: column;
    width: 100%;
  }

  .user-toolbar-actions {
    margin-left: 0;
  }
}
</style>
