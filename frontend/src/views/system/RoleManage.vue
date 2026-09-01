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
    <div class="page-toolbar role-toolbar">
      <span class="page-count">共 {{ total }} 个角色</span>
      <ActionButton action="create" text="新增角色" :plain="false" v-permission="'workspace:role:manage'" @click="openDialog()" />
    </div>

    <!-- 列表栏 -->
    <el-card class="table-card" shadow="never">

        <el-table :data="list" v-loading="loading" stripe border>
          <el-table-column prop="id" label="ID" width="72" />

          <el-table-column prop="name" label="角色名称" min-width="120" show-overflow-tooltip />
          <el-table-column prop="code" label="角色编码" min-width="150" show-overflow-tooltip>
            <template #default="{ row }"><span class="role-code">{{ row.code }}</span></template>
          </el-table-column>

          <el-table-column prop="description" label="描述" min-width="220">
            <template #default="{ row }">
              <span>{{ row.description || '-' }}</span>
            </template>
          </el-table-column>

          <el-table-column prop="sort_order" label="排序" width="90" />

          <el-table-column prop="status" label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">
                {{ row.status === 1 ? '启用' : '禁用' }}
              </el-tag>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="340" fixed="right">
            <template #default="{ row }">
              <div class="admin-action-group">
                <ActionButton action="edit" v-permission="'workspace:role:manage'" @click="openDialog(row)" />
                <ActionButton action="config" text="分配权限" v-permission="'workspace:role:manage'" @click="openPermissionDialog(row)" />
                <ActionButton action="delete" v-permission="'workspace:role:manage'" @click="handleDelete(row)" />
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
          layout="total, prev, pager, next, jumper"
          @change="fetchList"
        />
      </div>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑角色' : '新增角色'"
      width="min(480px, calc(100vw - 32px))"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px">
        <el-form-item label="名称" prop="name"><el-input v-model="form.name" /></el-form-item>
        <el-form-item label="编码" prop="code"><el-input v-model="form.code" :disabled="!!editingId" /></el-form-item>
        <el-form-item label="描述"><el-input v-model="form.description" type="textarea" /></el-form-item>
        <el-form-item label="排序"><el-input-number v-model="form.sort_order" :min="0" class="dialog-number" /></el-form-item>
        <el-form-item v-if="editingId" label="状态"><el-switch v-model="form.statusBool" active-text="启用" inactive-text="禁用" /></el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="permissionDialogVisible" title="分配角色权限" width="min(560px, calc(100vw - 32px))">
      <div v-loading="permissionLoading" class="permission-panel">
        <div class="permission-summary">
          <span>{{ permissionRoleName }}</span>
          <el-tag effect="plain">已选择 {{ checkedPermissionCodes.length }} 项</el-tag>
        </div>
        <el-checkbox-group v-model="checkedPermissionCodes" class="permission-grid">
          <el-checkbox v-for="permission in permissionList" :key="permission.code" :value="permission.code">
            <span class="permission-label">{{ permission.name }}</span>
            <span class="permission-code">{{ permission.code }}</span>
          </el-checkbox>
        </el-checkbox-group>
        <el-empty v-if="!permissionLoading && !permissionList.length" description="暂无可分配权限" />
      </div>

      <template #footer>
        <el-button @click="permissionDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="permissionSubmitting" @click="handleAssignPermissions">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { roleApi } from '../../api/role'
import ActionButton from '@/components/common/ActionButton.vue'

const list = ref([])
const loading = ref(false)
const total = ref(0)
const page = ref(1)
const pageSize = ref(10)
const dialogVisible = ref(false)
const editingId = ref(null)
const formRef = ref(null)
const permissionDialogVisible = ref(false)
const permissionList = ref([])
const checkedPermissionCodes = ref([])
const permissionRoleId = ref(null)
const permissionRole = ref(null)
const submitting = ref(false)
const permissionLoading = ref(false)
const permissionSubmitting = ref(false)
const permissionRoleName = computed(() => permissionRole.value?.name || '当前角色')

const form = reactive({
  name: '',
  code: '',
  description: '',
  sort_order: 0,
  statusBool: true
})

const rules = {
  name: [
    { required: true, message: '请输入角色名称', trigger: 'blur' },
    { max: 64, message: '角色名称不能超过 64 个字符', trigger: 'blur' }
  ],
  code: [
    { required: true, message: '请输入角色编码', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9:_-]{2,63}$/, message: '以小写字母开头，可包含小写字母、数字、冒号、下划线和连字符', trigger: 'blur' }
  ]
}

onMounted(fetchList)

async function fetchList() {
  loading.value = true
  try {
    const res = await roleApi.list({ page: page.value, page_size: pageSize.value })
    list.value = res.data.list || []
    total.value = res.data.total
  } catch (error) {
    list.value = []
    total.value = 0
  } finally {
    loading.value = false
  }
}

function openDialog(row) {
  editingId.value = row ? row.id : null
  Object.assign(
    form,
    row
      ? {
          name: row.name,
          code: row.code,
          description: row.description,
          sort_order: row.sort_order,
          statusBool: row.status === 1
        }
      : {
          name: '',
          code: '',
          description: '',
          sort_order: 0,
          statusBool: true
        }
  )
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  const data = editingId.value
    ? { name: form.name, description: form.description, sort_order: form.sort_order, status: form.statusBool ? 1 : 0 }
    : { name: form.name, code: form.code, description: form.description, sort_order: form.sort_order }

  submitting.value = true
  try {
    if (editingId.value) {
      await roleApi.update(editingId.value, data)
    } else {
      await roleApi.create(data)
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

async function openPermissionDialog(row) {
  permissionRoleId.value = row.id
  permissionRole.value = row
  permissionDialogVisible.value = true
  permissionLoading.value = true
  try {
    const [permissionsRes, roleRes] = await Promise.all([roleApi.permissions(), roleApi.get(row.id)])
    permissionList.value = permissionsRes.data || []
    checkedPermissionCodes.value = roleRes.data.permissions || []
  } catch (error) {
    permissionList.value = []
    checkedPermissionCodes.value = []
  } finally {
    permissionLoading.value = false
  }
}

async function handleAssignPermissions() {
  permissionSubmitting.value = true
  try {
    await roleApi.assignPermissions(permissionRoleId.value, checkedPermissionCodes.value)
    ElMessage.success('分配成功')
    permissionDialogVisible.value = false
  } catch {
    // The request interceptor presents the API error.
  } finally {
    permissionSubmitting.value = false
  }
}

async function handleDelete(row) {
  try {
    await ElMessageBox.confirm(`确定删除角色“${row.name}”？删除后已关联用户将失去该角色权限。`, '删除角色', {
      type: 'warning',
      confirmButtonText: '确认删除',
      confirmButtonClass: 'el-button--danger'
    })
    await roleApi.delete(row.id)
    ElMessage.success('删除成功')
    fetchList()
  } catch (error) {
    if (error !== 'cancel') console.error(error)
  }
}
</script>

<style scoped>
.dialog-number {
  width: 100%;
}
.role-toolbar {
  flex-shrink: 0;
}
.permission-panel {
  min-height: 120px;
}
.permission-summary {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
  color: var(--text-primary);
  font-weight: 700;
}
.role-code {
  color: var(--text-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
}
.permission-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 12px 20px;
}
.permission-grid :deep(.el-checkbox) {
  height: auto;
  margin-right: 0;
}
.permission-grid :deep(.el-checkbox__label) {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
  line-height: 1.35;
}
.permission-code {
  color: var(--el-text-color-secondary);
  font-size: 12px;
  overflow-wrap: anywhere;
}
@media (max-width: 640px) {
  .permission-grid {
    grid-template-columns: 1fr;
  }
}
</style>
