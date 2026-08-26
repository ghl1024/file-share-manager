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
  <div v-if="error" class="error-boundary fade-in">
    <el-result
      icon="error"
      title="页面暂时无法加载"
      sub-title="请重试当前页面；如果问题持续出现，请返回工作台。"
    >
      <template #extra>
        <el-button type="primary" @click="reload">重新加载</el-button>
        <el-button @click="goHome">返回首页</el-button>
      </template>
    </el-result>
  </div>
  <slot v-else></slot>
</template>

<script setup>
import { ref, onErrorCaptured } from 'vue'
import { useRouter } from 'vue-router'

const error = ref(null)
const router = useRouter()

onErrorCaptured((err, instance, info) => {
  // Request errors are normalized and presented by the Axios interceptor.
  // Vue also reports rejected async event handlers here; treating those as a
  // render crash would replace the login/error feedback with a dead-end page.
  if (typeof err?.presented === 'boolean') {
    return false
  }
  console.error('Error Boundary Captured:', err, info)
  error.value = err
  return false // prevent the error from propagating further
})

const reload = () => {
  error.value = null
  window.location.reload()
}

const goHome = () => {
  error.value = null
  router.push('/')
}
</script>

<style scoped>
.error-boundary {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  padding: 24px;
  width: 100%;
  background: var(--bg-primary);
}
</style>
