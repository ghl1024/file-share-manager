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
  <div class="login-container">
    <div class="login-backdrop"></div>

    <section class="login-card">
      <div class="login-brand">
        <div class="login-logo">
          <img :src="logoUrl" alt="File Share Manager Logo" class="logo-svg" />
        </div>
        <div class="login-copy">
          <span class="login-kicker">File Share Manager</span>
          <h1 class="login-title">分布式文件共享系统</h1>
        </div>
      </div>

      <el-form ref="formRef" :model="form" :rules="rules" class="login-form" @submit.prevent="handleLogin">
        <el-form-item prop="username">
          <el-input v-model="form.username" placeholder="用户名" :prefix-icon="User" size="large" />
        </el-form-item>
        <el-form-item prop="password">
          <el-input
            v-model="form.password"
            type="password"
            placeholder="密码"
            :prefix-icon="Lock"
            size="large"
            show-password
            @keyup.enter="handleLogin"
          />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" size="large" :loading="loading" class="login-btn" @click="handleLogin">
            进入工作台
          </el-button>
        </el-form-item>
      </el-form>

      <div class="login-footer">
        © 2026 <a href="https://hayden.pub" target="_blank">HaydenGuo</a>
        <div class="footer-divider"></div>
        <div class="footer-brands">
          <a href="https://cnb.cool/ghl1024/file-share-manager.git" target="_blank" class="brand-link" title="CNB">
            <svg viewBox="0 0 1024 1024" width="18" height="18">
              <path d="M853.3 256L512 64 170.7 256v384l341.3 192 341.3-192V256zM512 733.9L242.1 581.3V322.7L512 170.1l269.9 152.6v258.6L512 733.9z" fill="currentColor"></path>
              <path d="M512 320c-106 0-192 86-192 192s86 192 192 192 192-86 192-192-86-192-192-192z m0 320c-70.7 0-128-57.3-128-128s57.3-128 128-128 128 57.3 128 128-57.3 128-128 128z" fill="currentColor"></path>
            </svg>
          </a>
          <a href="https://gitcode.com/haydenguo/FileShareManager" target="_blank" class="brand-link gitcode-link" title="GitCode">
            <svg viewBox="0 0 1024 1024" width="18" height="18">
              <path d="M985.536 517.888c-25.12-59.936-73.856-111.424-163.296-118.528-78.08-6.176-165.44 6.176-222.72 18.56-89.056 12.384-114.496 61.92-105.984 105.28 10.464 53.152 92.896 49.44 147.904 41.472 23.072-3.328 87.68-14.56 104.448-16.736 95.456-12.384 102.176 45.472 89.088 80.48-25.472 68.128-94.08 117.056-140 136.224-48.736 20.32-114.656 38.752-183.04 41.632-0.032 0-332.352 32.64-287.808-301.664 4.256-31.776 11.648-59.52 21.6-83.744 13.6-33.184 1.792-88.608-10.88-122.144-1.696-4.48 2.24-9.056 7.04-8.256l3.296 0.576c36.992 6.176 87.424 18.656 121.312 3.008 38.56-17.824 80.544-26.112 120.512-28.928 42.24-3.008 88.576-44.352 120.096-71.84 3.648-3.2 9.472-1.344 10.464 3.328l5.76 26.56c6.4 28.352 41.824 65.728 67.584 67.104 28.224 1.536 57.984-5.6 77.76-21.248 63.04-49.856 51.52-143.232-17.696-179.232C563.776-7.584 337.152 16.576 167.552 173.856c-189.76 192.384-193.024 493.12 0 680.896 80.256 78.08 181.632 121.6 286.56 132.768 4.096 0.512 8.256 0.8 12.352 1.248l6.464 0.64c14.176 1.376 28.48 2.4 42.976 2.592 3.264-0.096 6.464-0.32 9.664-0.416 29.696-0.416 59.36-3.52 88.768-9.344 151.52-26.336 247.264-104.48 328.288-227.072 21.6-32.64 37.12-68.096 48.16-104.672 13.216-43.648 12.384-90.496-5.248-132.608z" fill="currentColor"></path>
            </svg>
          </a>
          <a href="https://gitee.com/ghl1024/file-share-manager.git" target="_blank" class="brand-link" title="Gitee">
            <svg viewBox="0 0 1024 1024" width="18" height="18">
              <path d="M512 1024C230.4 1024 0 793.6 0 512S230.4 0 512 0s512 230.4 512 512-230.4 512-512 512z m259.2-569.6H502.4c-19.2 0-35.2 16-35.2 35.2v192c0 19.2 16 35.2 35.2 35.2h172.8c19.2 0 35.2-16 35.2-35.2v-25.6c0-19.2-16-35.2-35.2-35.2h-134.4v-64h192c19.2 0 35.2-16 35.2-35.2v-32c0-19.2-16-35.2-35.2-35.2z m-352 0H262.4c-19.2 0-35.2 16-35.2 35.2v192c0 19.2 16 35.2 35.2 35.2h156.8c19.2 0 35.2-16 35.2-35.2V508.8c0-12.8-12.8-19.2-19.2-19.2h-12.8c-12.8 0-19.2 12.8-19.2 19.2v150.4h-102.4V508.8h121.6c12.8 0 19.2-12.8 19.2-19.2v-32c0-3.2 0-16-19.2-16z" fill="currentColor"></path>
            </svg>
          </a>
          <a href="https://github.com/ghl1024/file-share-manager.git" target="_blank" class="brand-link" title="GitHub">
            <svg viewBox="0 0 16 16" width="18" height="18" fill="currentColor">
              <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z"></path>
            </svg>
          </a>
        </div>
      </div>
    </section>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { Lock, User } from '@element-plus/icons-vue'
import { useRouter } from 'vue-router'
import { useUserStore } from '../stores/user'

const router = useRouter()
const userStore = useUserStore()
const logoUrl = `${import.meta.env.BASE_URL}logo.svg`
const formRef = ref(null)
const loading = ref(false)

const form = reactive({
  username: '',
  password: ''
})

const rules = {
  username: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

async function handleLogin() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return

  loading.value = true
  try {
    await userStore.login(form.username, form.password)
    router.push('/')
  } catch (error) {
    // handled by interceptor
  } finally {
    loading.value = false
  }
}
</script>

<style scoped>
.login-container {
  position: relative;
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  overflow: hidden;
  isolation: isolate;
  background: var(--bg-primary);
}

.login-backdrop {
  position: absolute;
  inset: 0;
  pointer-events: none;
  background: var(--login-bg-gradient);
}

.login-card {
  position: relative;
  z-index: 1;
  width: min(100%, 430px);
  padding: 28px;
  border-radius: var(--radius);
  border: 1px solid color-mix(in srgb, var(--panel-border) 84%, rgba(255, 255, 255, 0.38));
  background: var(--panel-bg-strong);
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.16);
}

.login-brand {
  display: flex;
  align-items: center;
  gap: 16px;
}

.login-logo {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 52px;
  height: 52px;
  border-radius: var(--radius);
  background: var(--gradient-primary);
  box-shadow: var(--shadow-glow);
  flex-shrink: 0;
}

.logo-svg {
  width: 34px;
  height: 34px;
}

.login-copy {
  min-width: 0;
}

.login-kicker {
  display: inline-flex;
  align-items: center;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0;
  text-transform: uppercase;
  color: var(--accent-primary);
}

.login-title {
  margin-top: 8px;
  font-size: 27px;
  line-height: 1.2;
  letter-spacing: 0;
  color: var(--text-primary);
}

.login-form {
  margin-top: 24px;
}

.login-form :deep(.el-form-item) {
  margin-bottom: 16px;
}

.login-btn {
  width: 100%;
  height: 46px;
  font-weight: 700;
}

@media (max-width: 640px) {
  .login-container {
    padding: 16px;
  }

  .login-card {
    padding: 22px;
  }

  .login-title {
    font-size: 24px;
  }

  .login-footer {
    flex-wrap: wrap;
    row-gap: 10px;
  }
}

.login-footer {
  margin-top: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 12px;
  font-size: 11px;
  color: var(--text-muted);
  opacity: 0.8;
  text-shadow: none;
}

.login-footer a {
  color: inherit;
  text-decoration: none;
  transition: all 0.3s ease;
}

.login-footer a:hover {
  color: var(--accent-primary);
}

.footer-divider {
  width: 1px;
  height: 12px;
  background: var(--panel-border);
}

.footer-brands {
  display: flex;
  align-items: center;
  gap: 16px;
}

.brand-link {
  display: flex;
  align-items: center;
  opacity: 0.7;
}

.brand-link:hover {
  opacity: 1;
  color: var(--text-primary);
}

.gitcode-link:hover {
  color: #FC5533 !important;
}
</style>
