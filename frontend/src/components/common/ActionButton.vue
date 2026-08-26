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
  <el-button
    :class="['common-action-button', `common-action-button--${action}`]"
    :type="btnConfig.type"
    :color="btnConfig.color"
    :icon="resolvedIcon"
    :plain="plain"
    :size="size"
    v-bind="$attrs"
  >
    <slot>{{ text || btnConfig.text }}</slot>
  </el-button>
</template>

<script setup>
import { computed } from 'vue'
import {
  Check,
  Clock,
  Close,
  CircleCheck,
	ChatDotRound,
  Box,
  DataAnalysis,
  Delete,
  Download,
  Edit,
	Filter,
  FolderAdd,
  FolderOpened,
  Key,
  Lock,
  MoreFilled,
  Minus,
  Plus,
  Position,
	Promotion,
  Refresh,
  RefreshLeft,
  RefreshRight,
  Search,
  Setting,
  Share,
  Star,
  Upload,
  User,
  VideoPause,
  VideoPlay,
  View,
} from '@element-plus/icons-vue'

const props = defineProps({
  action: {
    type: String,
    required: true,
  },
  plain: {
    type: Boolean,
    default: true
  },
  size: {
    type: String,
    default: 'small'
  },
  text: {
    type: String,
    default: ''
  },
  icon: {
    type: [String, Object, Function],
    default: ''
  }
})

// action -> config mapping
const actionMap = {
  view: { text: '查看', type: 'success', icon: 'View' },
  preview: { text: '预览', color: '#0d9488', icon: 'View' },
  search: { text: '查询', type: 'primary', icon: 'Search' },
	filter: { text: '筛选', type: 'info', icon: 'Filter' },
  dashboard: { text: '看板', color: '#06b6d4', icon: 'DataAnalysis' },
  enter: { text: '进入', type: 'primary', icon: 'Position' },
  members: { text: '成员', type: 'success', icon: 'User' },
  quota: { text: '配额', type: 'warning', icon: 'Setting' },
  edit: { text: '编辑', type: 'primary', icon: 'Edit' },
  delete: { text: '删除', type: 'danger', icon: 'Delete' },
  upgrade: { text: '升级', type: 'warning', icon: 'Upload' },
  config: { text: '配置', color: '#8b5cf6', icon: 'Setting' },
  key: { text: '凭证', type: 'warning', icon: 'Key' },
  add: { text: '添加', type: 'success', icon: 'Plus' },
  create: { text: '新增', type: 'primary', icon: 'Plus' },
  save: { text: '保存', type: 'primary', icon: 'Check' },
  assignRole: { text: '关联角色', type: 'info', icon: 'User' },
  exportHistory: { text: '导出记录', color: '#0ea5e9', icon: 'Clock' },
  archive: { text: '归档记录', color: '#8b5cf6', icon: 'Box' },
  refresh: { text: '刷新', color: '#0ea5e9', icon: 'Refresh' },
  test: { text: '测试', color: '#ec4899', icon: 'Position' },
  enable: { text: '启用', type: 'success', icon: 'Check' },
  disable: { text: '禁用', type: 'info', icon: 'Minus' },
  rotate: { text: '轮换', type: 'warning', icon: 'Refresh' },
  revoke: { text: '撤销', type: 'danger', icon: 'Lock' },
  files: { text: '文件', type: 'primary', icon: 'FolderOpened' },
  shared: { text: '与我共享', color: '#0d9488', icon: 'Share' },
  myShares: { text: '我的分享', type: 'primary', icon: 'User' },
  allShares: { text: '全部分享', color: '#0d9488', icon: 'Share' },
  recent: { text: '最近使用', color: '#7c3aed', icon: 'Clock' },
  favorites: { text: '收藏', type: 'warning', icon: 'Star' },
  trash: { text: '回收站', type: 'danger', icon: 'Delete' },
  batchDownload: { text: '批量下载', type: 'info', icon: 'Download' },
  tasks: { text: '下载任务', type: 'info', icon: 'Clock' },
  upload: { text: '上传文件', type: 'success', icon: 'Upload' },
  folderAdd: { text: '新建目录', type: 'primary', icon: 'FolderAdd' },
  download: { text: '下载', type: 'primary', icon: 'Download' },
  more: { text: '更多', type: 'info', icon: 'MoreFilled' },
	collaborate: { text: '协作', color: '#7c3aed', icon: 'ChatDotRound' },
	comment: { text: '评论', color: '#0d9488', icon: 'Promotion' },
  restore: { text: '恢复', type: 'success', icon: 'RefreshLeft' },
  resetPassword: { text: '重置密码', type: 'warning', icon: 'Key' },
  verify: { text: '校验', type: 'success', icon: 'CircleCheck' },
  drill: { text: '演练', type: 'success', icon: 'DataAnalysis' },
  retryBackup: { text: '重试', type: 'warning', icon: 'RefreshRight' },
  retryDownload: { text: '重试', type: 'warning', icon: 'RefreshLeft' },
  retryNotification: { text: '重新入队', type: 'warning', icon: 'RefreshRight' },
  rescan: { text: '重扫', type: 'warning', icon: 'Refresh' },
  baseline: { text: '创建基线备份', type: 'primary', icon: 'Plus' },
  incremental: { text: '创建增量备份', type: 'info', icon: 'Plus' },
  compact: { text: '压缩基线', color: '#8b5cf6', icon: 'DataAnalysis' },
  restoreWorkspace: { text: '恢复空间', type: 'warning', icon: 'FolderAdd' },
  pauseUpload: { text: '暂停', type: 'warning', icon: 'VideoPause' },
  continueUpload: { text: '继续', type: 'success', icon: 'VideoPlay' },
  resumeUpload: { text: '选择原文件', type: 'primary', icon: 'Upload' },
  retryUpload: { text: '重试', type: 'warning', icon: 'RefreshRight' },
  cancelUpload: { text: '取消', type: 'danger', icon: 'Close' },
  clearCompleted: { text: '清除已结束', type: 'info', icon: 'Delete' },
  batchActions: { text: '批量操作', color: '#7c3aed', icon: 'MoreFilled' },
}

const iconMap = {
  Check,
  Clock,
  Close,
  CircleCheck,
	ChatDotRound,
  Box,
  DataAnalysis,
  Delete,
  Download,
  Edit,
	Filter,
  FolderAdd,
  FolderOpened,
  Key,
  Lock,
  MoreFilled,
  Minus,
  Plus,
  Position,
	Promotion,
  Refresh,
  RefreshLeft,
  RefreshRight,
  Search,
  Setting,
  Share,
  Star,
  Upload,
  User,
  VideoPause,
  VideoPlay,
  View,
}

const btnConfig = computed(() => {
  return actionMap[props.action] || { text: '操作', type: 'default', icon: '' }
})

const resolvedIcon = computed(() => {
  const configuredIcon = props.icon || btnConfig.value.icon
  if (!configuredIcon || typeof configuredIcon !== 'string') return configuredIcon || null
  return iconMap[configuredIcon] || null
})
</script>
