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
    <div class="page-toolbar">
      <div class="page-toolbar__summary">
        <span class="page-count">共 {{ flatMenus.length }} 个菜单/按钮</span>
        <el-tag effect="plain">菜单 {{ menuCount }}</el-tag>
        <el-tag effect="plain" type="warning">按钮 {{ buttonCount }}</el-tag>
      </div>
      <div class="toolbar-actions">
        <ActionButton action="refresh" @click="fetchList" />
        <ActionButton action="create" text="新增菜单" @click="openDialog(null, 1)" />
        <ActionButton action="create" text="新增按钮" :plain="false" @click="openDialog(null, 2)" />
      </div>
    </div>

    <el-alert
      class="menu-tip"
      type="info"
      :closable="false"
      show-icon
      title="菜单只负责导航展示，按钮节点用于说明页面操作对应的 permission code；真实权限仍由后端接口统一校验。"
    />

    <el-card class="table-card" shadow="never">
      <el-table
        :data="menuTree"
        v-loading="loading"
        row-key="id"
        stripe
        border
        default-expand-all
        :tree-props="{ children: 'children' }"
      >
        <el-table-column label="名称" min-width="300" align="center" header-align="center" show-overflow-tooltip>
          <template #default="{ row }">
            <div class="menu-name-cell">
              <el-icon v-if="row.type !== 2"><component :is="resolveAppIcon(row.icon)" /></el-icon>
              <span class="menu-name">{{ menuDisplayName(row) }}</span>
              <span class="menu-code">{{ row.code }}</span>
            </div>
          </template>
        </el-table-column>

        <el-table-column label="类型" width="100" align="center" header-align="center">
          <template #default="{ row }">
            <el-tag :type="typeTag(row.type)" size="small">{{ typeText(row.type) }}</el-tag>
          </template>
        </el-table-column>

        <el-table-column label="路由" min-width="180" align="center" header-align="center" show-overflow-tooltip>
          <template #default="{ row }">
            <span class="mono-value">{{ row.type === 2 ? '-' : row.path || '无路由' }}</span>
          </template>
        </el-table-column>

        <el-table-column label="权限点" min-width="260" align="center" header-align="center">
          <template #default="{ row }">
            <div v-if="row.permissions?.length" class="admin-inline-tags">
              <el-tag v-for="permission in row.permissions" :key="permission" effect="plain" size="small">
                {{ permissionName(permission) }}
                <span class="permission-code"> {{ permission }}</span>
              </el-tag>
            </div>
            <span v-else class="admin-muted">无需功能权限</span>
          </template>
        </el-table-column>

        <el-table-column prop="sort_order" label="排序" width="90" align="center" header-align="center" />

        <el-table-column prop="status" label="状态" width="100" align="center" header-align="center">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'danger'" size="small">
              {{ row.status === 1 ? '启用' : '禁用' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="显示" width="90" align="center" header-align="center">
          <template #default="{ row }">
            <el-tag :type="row.hidden ? 'info' : 'success'" effect="plain" size="small">
              {{ row.hidden ? '隐藏' : '显示' }}
            </el-tag>
          </template>
        </el-table-column>

        <el-table-column label="操作" width="220" fixed="right" align="center" header-align="center">
          <template #default="{ row }">
            <div class="admin-action-group">
              <ActionButton action="edit" @click="openDialog(row)" />
              <ActionButton action="delete" :disabled="Boolean(row.children?.length)" @click="handleDelete(row)" />
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog
      v-model="dialogVisible"
      :title="editingId ? '编辑菜单权限' : (form.type === 2 ? '新增按钮权限' : '新增菜单权限')"
      width="min(680px, calc(100vw - 32px))"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="98px">
        <el-form-item label="父级菜单">
          <el-tree-select
            v-model="form.parent_id"
            :data="menuTreeForSelect"
            :props="{ label: 'name', value: 'id', children: 'children' }"
            check-strictly
            clearable
            placeholder="顶级菜单"
            class="dialog-full"
          />
        </el-form-item>

        <el-form-item label="类型" prop="type">
          <el-radio-group v-model="form.type">
            <el-radio :value="0">目录</el-radio>
            <el-radio :value="1">菜单</el-radio>
            <el-radio :value="2">按钮</el-radio>
          </el-radio-group>
        </el-form-item>

        <el-form-item label="编码" prop="code">
          <el-input v-model="form.code" :disabled="Boolean(editingId)" placeholder="例如 files-upload" />
        </el-form-item>

        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="展示在侧边栏或按钮权限说明中的名称" />
        </el-form-item>

        <el-form-item v-if="form.type !== 2" label="路由" prop="path">
          <el-input v-model="form.path" placeholder="/system/menu" />
        </el-form-item>

        <el-form-item v-if="form.type !== 2" label="组件">
          <el-input v-model="form.component" placeholder="system/MenuManage，仅做配置备注" />
        </el-form-item>

        <el-form-item v-if="form.type !== 2" label="图标">
          <el-input v-model="form.icon" placeholder="Element Plus 图标名，例如 Menu" />
        </el-form-item>

        <el-form-item label="权限点">
          <el-select
            v-model="form.permissions"
            multiple
            filterable
            clearable
            collapse-tags
            collapse-tags-tooltip
            placeholder="选择该菜单/按钮需要的功能权限；不选则仅依赖内置可见性规则"
            class="dialog-full"
          >
            <el-option
              v-for="permission in permissionOptions"
              :key="permission.code"
              :label="`${permission.name} · ${permission.code}`"
              :value="permission.code"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="排序">
          <el-input-number v-model="form.sort_order" :min="0" :max="9999" class="dialog-number" />
        </el-form-item>

        <el-form-item label="状态">
          <el-switch
            v-model="form.status"
            :active-value="1"
            :inactive-value="0"
            active-text="启用"
            inactive-text="禁用"
          />
        </el-form-item>

        <el-form-item v-if="form.type !== 2" label="隐藏">
          <el-switch v-model="form.hidden" active-text="隐藏" inactive-text="显示" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="submitting" @click="handleSubmit">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { menuApi } from '@/api/menu'
import ActionButton from '@/components/common/ActionButton.vue'
import { resolveAppIcon } from '@/utils/icons'

const menuTree = ref([])
const permissionOptions = ref([])
const loading = ref(false)
const submitting = ref(false)
const dialogVisible = ref(false)
const editingId = ref(null)
const formRef = ref(null)

const form = reactive({
  code: '',
  parent_id: 0,
  name: '',
  path: '',
  component: '',
  type: 1,
  icon: '',
  sort_order: 0,
  hidden: false,
  status: 1,
  permissions: []
})

const rules = {
  code: [
    { required: true, message: '请输入菜单编码', trigger: 'blur' },
    { pattern: /^[a-z][a-z0-9:_-]{1,63}$/, message: '以小写字母开头，可包含小写字母、数字、冒号、下划线和连字符', trigger: 'blur' }
  ],
  name: [
    { required: true, message: '请输入名称', trigger: 'blur' },
    { max: 64, message: '名称不能超过 64 个字符', trigger: 'blur' }
  ],
  path: [{ max: 255, message: '路由不能超过 255 个字符', trigger: 'blur' }]
}

const flatMenus = computed(() => flattenMenus(menuTree.value))
const menuCount = computed(() => flatMenus.value.filter((item) => item.type !== 2).length)
const buttonCount = computed(() => flatMenus.value.filter((item) => item.type === 2).length)
const permissionNameMap = computed(() => Object.fromEntries(permissionOptions.value.map((item) => [item.code, item.name])))
const menuTreeForSelect = computed(() => [{ id: 0, name: '顶级菜单', children: nonButtonTree(menuTree.value, editingId.value) }])
const builtinMenuNames = {
  dashboard: '工作台',
  workspaces: '工作空间',
  files: '文件目录',
  'files-upload': '上传文件',
  'files-download': '下载文件',
  'files-delete': '删除文件',
  'files-restore': '恢复文件',
  'files-acl': '目录权限',
  shares: '外链分享',
  audit: '审计中心',
  'audit-export': '导出审计',
  'audit-archive': '归档审计',
  system: '系统管理',
  'system-users': '用户管理',
  'system-menu': '菜单权限',
  'system-ldap': 'LDAP',
  'system-clamav': 'ClamAV',
  'system-backup-storage': '备份存储',
  roles: '角色管理',
  backups: '备份恢复'
}

onMounted(async () => {
  await Promise.all([fetchList(), fetchPermissions()])
})

async function fetchList() {
  loading.value = true
  try {
    const res = await menuApi.list()
    menuTree.value = normalizeMenuTree(res.data || [])
  } finally {
    loading.value = false
  }
}

async function fetchPermissions() {
  const res = await menuApi.permissions()
  permissionOptions.value = res.data || []
}

function menuDisplayName(row) {
  const name = String(row?.name || '').trim()
  return builtinMenuNames[row?.code] || (name && name !== '...' && name !== '…' ? name : '未命名菜单')
}

function normalizeMenuTree(items) {
  return (items || []).map((item) => ({
    ...item,
    name: menuDisplayName(item),
    children: normalizeMenuTree(item.children)
  }))
}

function openDialog(row, defaultType = 1) {
  editingId.value = row?.id || null
  Object.assign(
    form,
    row
      ? {
          code: row.code,
          parent_id: row.parent_id || 0,
          name: row.name,
          path: row.path || '',
          component: row.component || '',
          type: row.type,
          icon: row.icon || '',
          sort_order: row.sort_order || 0,
          hidden: Boolean(row.hidden),
          status: row.status ?? 1,
          permissions: [...(row.permissions || [])]
        }
      : {
          code: '',
          parent_id: 0,
          name: '',
          path: '',
          component: '',
          type: defaultType,
          icon: '',
          sort_order: flatMenus.value.length * 10 + 10,
          hidden: defaultType === 2,
          status: 1,
          permissions: []
        }
  )
  dialogVisible.value = true
}

async function handleSubmit() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  const payload = {
    code: form.code,
    parent_id: form.parent_id || 0,
    name: form.name,
    path: form.type === 2 ? '' : form.path,
    component: form.type === 2 ? '' : form.component,
    type: form.type,
    icon: form.type === 2 ? '' : form.icon,
    sort_order: form.sort_order,
    hidden: form.type === 2 ? true : form.hidden,
    status: form.status,
    permissions: form.permissions || []
  }
  submitting.value = true
  try {
    if (editingId.value) {
      await menuApi.update(editingId.value, payload)
    } else {
      await menuApi.create(payload)
    }
    ElMessage.success('保存成功')
    dialogVisible.value = false
    await fetchList()
  } finally {
    submitting.value = false
  }
}

async function handleDelete(row) {
  await ElMessageBox.confirm(`确定删除“${row.name}”吗？删除后相关菜单权限映射会同步移除。`, '删除菜单权限', {
    type: 'warning',
    confirmButtonText: '确认删除',
    cancelButtonText: '取消'
  })
  await menuApi.delete(row.id)
  ElMessage.success('删除成功')
  await fetchList()
}

function typeText(type) {
  return ['目录', '菜单', '按钮'][type] || '未知'
}

function typeTag(type) {
  if (type === 0) return 'info'
  if (type === 2) return 'warning'
  return 'success'
}

function permissionName(code) {
  return permissionNameMap.value[code] || code
}

function flattenMenus(items, result = []) {
  for (const item of items || []) {
    result.push(item)
    flattenMenus(item.children, result)
  }
  return result
}

function nonButtonTree(items, excludedId) {
  return (items || [])
    .filter((item) => item.type !== 2 && item.id !== excludedId)
    .map((item) => ({
      ...item,
      children: nonButtonTree(item.children, excludedId)
    }))
}
</script>

<style scoped>
.menu-tip {
  margin-bottom: 0;
}

.menu-name-cell {
  display: flex;
  align-items: center;
  justify-content: center;
  width: max-content;
  max-width: none;
  gap: 8px;
  margin: 0 auto;
}

.menu-name {
  flex: 0 0 auto;
  font-weight: 700;
  color: var(--text-primary);
}

.menu-code,
.permission-code,
.mono-value {
  flex: 0 0 auto;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  color: var(--text-muted);
  font-size: 12px;
}

.dialog-full,
.dialog-number {
  width: 100%;
}

@media (max-width: 760px) {
  .page-toolbar {
    align-items: flex-start;
    flex-direction: column;
  }
}
</style>
