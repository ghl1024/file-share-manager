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
  <main class="public-share-page">
    <div class="share-brand">
      <img :src="logoUrl" alt="File Share Manager Logo" />
      <span>File Share Manager</span>
    </div>
    <section v-loading="loading" class="share-shell">
      <template v-if="share">
        <header class="share-header">
          <div>
            <p class="eyebrow">文件分享</p>
            <h1>{{ share.name }}</h1>
            <p class="muted">{{ share.root_name }} · 有效至 {{ formatDate(share.expires_at) }}</p>
          </div>
          <el-tag v-if="share.max_downloads" effect="plain">已下载 {{ share.download_count }} / {{ share.max_downloads }}</el-tag>
        </header>

        <el-alert v-if="share.requires_password && !verified" title="此分享需要密码验证" type="warning" :closable="false" show-icon />
        <el-table v-if="verified || !share.requires_password" :data="share.items" class="share-table" row-key="public_id">
          <el-table-column label="文件" min-width="320">
            <template #default="scope">
              <span class="share-item-name"><el-icon><Document /></el-icon>{{ scope.row.relative_path }}</span>
            </template>
          </el-table-column>
          <el-table-column label="大小" width="140"><template #default="scope">{{ formatBytes(scope.row.size) }}</template></el-table-column>
          <el-table-column prop="version_no" label="版本" width="90" />
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="scope">
              <el-button
                text
                type="primary"
                :icon="Download"
                :loading="downloadingItem === scope.row.public_id"
                :disabled="Boolean(downloadingItem)"
                @click="download(scope.row)"
              >下载</el-button>
            </template>
          </el-table-column>
          <template #empty><el-empty description="分享内容为空" /></template>
        </el-table>
      </template>
      <el-empty v-else-if="!loading" description="分享不存在或已失效" />
    </section>

    <el-dialog v-model="passwordDialog" title="验证分享密码" width="min(380px, calc(100vw - 32px))" :close-on-click-modal="false" :show-close="false">
      <el-form ref="passwordFormRef" :model="passwordForm" :rules="passwordRules" label-position="top" @submit.prevent="submitPassword">
        <el-form-item label="分享密码" prop="password">
          <el-input v-model="passwordForm.password" type="password" show-password autofocus placeholder="请输入分享密码" @keyup.enter="submitPassword" />
        </el-form-item>
      </el-form>
      <template #footer><el-button type="primary" :loading="verifying" @click="submitPassword">验证</el-button></template>
    </el-dialog>
  </main>
</template>

<script setup>
import { onMounted, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { Document, Download } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { downloadShare, getShare, verifyShare } from '@/api/share'

const route = useRoute()
const token = String(route.params.token || '')
const logoUrl = `${import.meta.env.BASE_URL}logo.svg`
const share = ref(null)
const loading = ref(true)
const verified = ref(false)
const passwordDialog = ref(false)
const passwordFormRef = ref(null)
const passwordForm = reactive({ password: '' })
const passwordRules = {
  password: [{ required: true, message: '请输入分享密码', trigger: 'blur' }]
}
const verifying = ref(false)
const downloadingItem = ref('')

async function loadShare() {
  loading.value = true
  try {
    const result = await getShare(token)
    share.value = result.data
    verified.value = !share.value.requires_password || verified.value
    passwordDialog.value = share.value.requires_password && !verified.value
  } catch (error) {
    share.value = null
    ElMessage.error(error.message)
  } finally {
    loading.value = false
  }
}

async function submitPassword() {
  const valid = await passwordFormRef.value?.validate().catch(() => false)
  if (!valid) return
  verifying.value = true
  try {
    await verifyShare(token, passwordForm.password)
    verified.value = true
    passwordDialog.value = false
    passwordForm.password = ''
    await loadShare()
    passwordDialog.value = false
    ElMessage.success('验证成功')
  } catch (error) {
    ElMessage.error(error.message)
  } finally {
    verifying.value = false
  }
}

async function download(item) {
  downloadingItem.value = item.public_id
  try {
    const result = await downloadShare(token, item.public_id)
    const url = URL.createObjectURL(result.data)
    const link = document.createElement('a')
    link.href = url
    link.download = item.name
    link.click()
    URL.revokeObjectURL(url)
  } catch (error) {
    ElMessage.error(error.message)
  } finally {
    downloadingItem.value = ''
  }
}

function formatDate(value) {
  return value ? new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value)) : '-'
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(1)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

onMounted(loadShare)
</script>

<style scoped>
.public-share-page {
  min-height: 100vh;
  padding: 32px 20px;
  background: var(--bg-viewport);
}

.share-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  width: min(1080px, 100%);
  margin: 0 auto 14px;
  color: var(--text-primary);
  font-size: 15px;
  font-weight: 700;
}

.share-brand img {
  width: 30px;
  height: 30px;
  padding: 4px;
  border-radius: var(--radius-sm);
  background: var(--accent-primary);
}

.share-shell {
  width: min(1080px, 100%);
  margin: 0 auto;
  min-height: 260px;
  padding: 24px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius);
  background: var(--panel-bg-strong);
  box-shadow: var(--shadow-card);
}

.share-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 20px;
}

.share-header h1 {
  margin: 0;
  font-size: 24px;
  letter-spacing: 0;
  overflow-wrap: anywhere;
}

.share-header .muted {
  margin: 8px 0 0;
}

.eyebrow {
  margin: 0 0 6px;
  color: var(--accent-primary);
  font-size: 12px;
  font-weight: 700;
}

.share-table {
  margin-top: 16px;
}

.share-item-name {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  overflow-wrap: anywhere;
}

@media (max-width: 640px) {
  .public-share-page {
    padding: 18px 12px;
  }

  .share-shell {
    padding: 18px 12px 12px;
  }

  .share-header {
    flex-direction: column;
  }

  .share-header h1 {
    font-size: 20px;
  }
}
</style>
