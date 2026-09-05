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
  <div class="page-container" :class="{ 'ldap-page': activeSection === 'ldap' }" v-loading="loading">
    <template v-if="activeSection === 'ldap'">
      <el-card shadow="never" class="ldap-compact-card ldap-config-card">
        <div class="ldap-card-head">
          <div class="ldap-card-title">
            <span class="ldap-title-icon"><el-icon><Connection /></el-icon></span>
            <span>LDAP 配置</span>
            <span class="ldap-schedule-summary">
              定时同步 {{ config.ldap?.enabled ? '已启用' : '已停用' }}
              <b>{{ config.ldap?.sync_cron || '-' }}</b>
            </span>
          </div>
          <div class="ldap-card-actions">
            <el-tag :type="config.ldap?.enabled ? 'success' : 'info'" effect="plain" class="ldap-status-tag">
              {{ config.ldap?.enabled ? '已启用' : '未启用' }}
            </el-tag>
            <el-button type="warning" plain :loading="ldapTesting" :disabled="!hasLDAPConfig" @click="testLDAP()">测试连接</el-button>
            <el-button type="success" plain :loading="ldapSyncing" :disabled="!config.ldap?.enabled" @click="manualLDAPSync">手动全量同步</el-button>
            <el-button type="primary" plain @click="openLDAPDrawer">修改配置</el-button>
          </div>
        </div>
      </el-card>

      <el-card shadow="never" class="ldap-compact-card ldap-history-card">
        <template #header>
          <div class="ldap-card-head">
            <div class="ldap-card-title">
              <span class="ldap-title-icon"><el-icon><Clock /></el-icon></span>
              <span>同步历史记录</span>
            </div>
            <el-button :icon="Refresh" circle type="primary" plain :loading="ldapHistoryLoading" @click="loadLDAPHistory" />
          </div>
        </template>
        <div class="ldap-history-body" v-loading="ldapHistoryLoading">
          <el-table
            v-if="ldapHistory.length"
            :data="ldapHistory"
            stripe
            border
            height="350"
            class="center-table ldap-history-table"
          >
            <el-table-column prop="start_time" label="同步时间" min-width="168">
              <template #default="{ row }">{{ formatDateTime(row.start_time) }}</template>
            </el-table-column>
            <el-table-column prop="sync_type" label="触发方式" width="110">
              <template #default="{ row }">
                <el-tag :type="row.sync_type === 'manual' ? 'warning' : 'info'" effect="plain" size="small">
                  {{ syncTypeLabel(row.sync_type) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="status" label="状态" width="110">
              <template #default="{ row }">
                <el-tag :type="syncStatusType(row.status)" effect="light" size="small">
                  {{ syncStatusLabel(row.status) }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="total_users" label="拉取人数" width="110" />
            <el-table-column prop="success_count" label="新增入库" width="110" />
            <el-table-column prop="update_count" label="属性更新" width="110" />
            <el-table-column prop="skip_count" label="跳过" width="90" />
            <el-table-column prop="total_groups" label="拉取组数" width="110" />
            <el-table-column label="组变更" width="128">
              <template #default="{ row }">{{ groupSyncSummary(row) }}</template>
            </el-table-column>
            <el-table-column label="耗时" width="110">
              <template #default="{ row }">{{ syncDuration(row) }}</template>
            </el-table-column>
            <el-table-column prop="error_message" label="错误信息" min-width="220" show-overflow-tooltip>
              <template #default="{ row }">{{ row.error_message || '-' }}</template>
            </el-table-column>
          </el-table>

          <div v-else class="ldap-empty-state">
            <el-empty description="暂无同步记录" />
          </div>

          <div v-if="ldapHistoryTotal > 0" class="history-pagination">
            <el-pagination
              background
              layout="total, prev, pager, next, sizes"
              :total="ldapHistoryTotal"
              :current-page="ldapHistoryQuery.page"
              :page-size="ldapHistoryQuery.page_size"
              :page-sizes="[10, 20, 50]"
              @current-change="handleLDAPHistoryPageChange"
              @size-change="handleLDAPHistorySizeChange"
            />
          </div>
        </div>
      </el-card>
    </template>

    <template v-else>
    <div class="page-toolbar">
      <span class="page-count">运行环境与依赖状态</span>
      <el-button :icon="Refresh" @click="load">刷新</el-button>
    </div>

    <div class="stats-grid system-config-grid">
      <el-card v-for="item in visibleDependencyCards" :key="item.key" shadow="never" class="dependency-card">
        <div class="dependency-header">
          <div class="dependency-icon"><el-icon><component :is="item.icon" /></el-icon></div>
          <div class="dependency-title"><strong>{{ item.label }}</strong><span>{{ item.summary }}</span></div>
          <el-tag :type="item.enabled ? 'success' : 'info'" effect="plain">{{ item.enabled ? '已配置' : '未配置' }}</el-tag>
        </div>
        <div v-if="item.key === 'ldap' || (item.key === 'clamav' && item.enabled)" class="dependency-actions">
          <el-button v-if="item.key === 'ldap'" @click="openLDAPDrawer">配置</el-button>
          <el-button v-if="item.key === 'ldap'" :loading="ldapTesting" :disabled="!hasLDAPConfig" @click="testLDAP()">测试连接</el-button>
          <el-button v-if="item.key === 'clamav'" :loading="clamavTesting" @click="testClamAV">健康检查</el-button>
        </div>
      </el-card>
    </div>

    <el-card v-if="showSection('ldap')" shadow="never" class="table-card clamav-health-card">
      <template #header>
        <div class="card-header-row">
          <div class="ldap-title-block">
            <span>LDAP 目录服务</span>
            <small>配置保存到数据库，登录认证实时读取启用状态</small>
          </div>
          <div class="ldap-config-actions">
            <el-tag :type="config.ldap?.enabled ? 'success' : 'info'" size="small">{{ config.ldap?.enabled ? '已启用' : '未启用' }}</el-tag>
            <el-button :loading="ldapTesting" :disabled="!hasLDAPConfig" @click="testLDAP()">测试连接</el-button>
            <el-button :loading="ldapSyncing" :disabled="!config.ldap?.enabled" @click="manualLDAPSync">手动同步</el-button>
            <el-button type="primary" @click="openLDAPDrawer">修改配置</el-button>
          </div>
        </div>
      </template>
      <el-alert
        class="config-source-tip"
        type="success"
        :closable="false"
        show-icon
        title="LDAP 已改为页面维护"
        description="主机、管理员 DN、Base DN 和属性映射会保存到数据库；管理员密码不会返回前端，编辑时留空表示保留旧密码。"
      />
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item label="服务地址">{{ ldapConnectionText }}</el-descriptions-item>
        <el-descriptions-item label="管理员 DN">{{ config.ldap?.admin_dn || '-' }}</el-descriptions-item>
        <el-descriptions-item label="基础 DN">{{ config.ldap?.base_dn || '-' }}</el-descriptions-item>
        <el-descriptions-item label="用户过滤器">{{ config.ldap?.user_filter || '-' }}</el-descriptions-item>
        <el-descriptions-item label="用户名属性">{{ config.ldap?.username_attr || '-' }}</el-descriptions-item>
        <el-descriptions-item label="邮箱属性">{{ config.ldap?.email_attr || '-' }}</el-descriptions-item>
        <el-descriptions-item label="姓名属性">{{ config.ldap?.real_name_attr || config.ldap?.realname_attr || '-' }}</el-descriptions-item>
        <el-descriptions-item label="同步周期">{{ config.ldap?.sync_cron || '-' }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="!config.ldap?.enabled" class="clamav-warning" type="info" :closable="false" show-icon title="当前使用本地账号登录。启用 LDAP 后，新登录用户会按页面映射自动创建或更新资料。" />
    </el-card>

    <el-card v-if="activeSection === 'ldap'" shadow="never" class="table-card ldap-history-card">
      <template #header>
        <div class="card-header-row">
          <div class="ldap-title-block">
            <span>LDAP 同步记录</span>
            <small>展示手动同步和定时同步的执行结果</small>
          </div>
          <div class="ldap-config-actions">
            <el-button :loading="ldapHistoryLoading" @click="loadLDAPHistory">刷新记录</el-button>
            <el-button type="primary" :loading="ldapSyncing" :disabled="!config.ldap?.enabled" @click="manualLDAPSync">手动同步</el-button>
          </div>
        </div>
      </template>
      <el-table
        v-loading="ldapHistoryLoading"
        :data="ldapHistory"
        stripe
        border
        class="center-table ldap-history-table"
        empty-text="暂无同步记录"
      >
        <el-table-column prop="start_time" label="开始时间" min-width="168">
          <template #default="{ row }">{{ formatDateTime(row.start_time) }}</template>
        </el-table-column>
        <el-table-column prop="sync_type" label="类型" width="100">
          <template #default="{ row }">
            <el-tag :type="row.sync_type === 'manual' ? 'primary' : 'info'" effect="plain" size="small">
              {{ syncTypeLabel(row.sync_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="110">
          <template #default="{ row }">
            <el-tag :type="syncStatusType(row.status)" effect="light" size="small">
              {{ syncStatusLabel(row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="total_users" label="LDAP 用户" width="110" />
        <el-table-column prop="success_count" label="新增" width="90" />
        <el-table-column prop="update_count" label="更新" width="90" />
        <el-table-column prop="skip_count" label="跳过" width="90" />
        <el-table-column label="耗时" width="110">
          <template #default="{ row }">{{ syncDuration(row) }}</template>
        </el-table-column>
        <el-table-column prop="error_message" label="摘要" min-width="220" show-overflow-tooltip>
          <template #default="{ row }">{{ row.error_message || '-' }}</template>
        </el-table-column>
      </el-table>
      <div class="history-pagination">
        <el-pagination
          background
          layout="total, prev, pager, next, sizes"
          :total="ldapHistoryTotal"
          :current-page="ldapHistoryQuery.page"
          :page-size="ldapHistoryQuery.page_size"
          :page-sizes="[10, 20, 50]"
          @current-change="handleLDAPHistoryPageChange"
          @size-change="handleLDAPHistorySizeChange"
        />
      </div>
    </el-card>

    <el-card v-if="showSection('clamav') && !clamavEnabled" shadow="never" class="table-card clamav-health-card">
      <template #header>
        <div class="card-header-row">
          <span>ClamAV 病毒库</span>
          <el-tag type="info" size="small">未启用</el-tag>
        </div>
      </template>
      <el-alert
        class="clamav-warning"
        type="info"
        :closable="false"
        show-icon
        title="当前开发配置未启用 ClamAV，上传文件会保留 unscanned 状态；启用 ClamAV 后可在此查看病毒库健康。"
      />
    </el-card>

    <el-card v-else-if="showSection('clamav') && clamavHealth" shadow="never" class="table-card clamav-health-card">
      <template #header>
        <div class="card-header-row">
          <span>ClamAV 病毒库</span>
          <el-tag :type="clamavHealth.status === 'healthy' ? 'success' : (clamavHealth.status === 'stale' ? 'warning' : 'danger')" size="small">
            {{ clamavHealth.status === 'healthy' ? '健康' : (clamavHealth.status === 'stale' ? '病毒库可能过期' : '连接异常') }}
          </el-tag>
        </div>
      </template>
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item label="引擎版本">{{ clamavHealth.engine_version || '-' }}</el-descriptions-item>
        <el-descriptions-item label="病毒库版本">{{ clamavHealth.virus_db_version || '-' }}</el-descriptions-item>
        <el-descriptions-item label="病毒库时间">{{ formatDateTime(clamavHealth.virus_db_updated_at) }}</el-descriptions-item>
        <el-descriptions-item label="病毒库龄">{{ clamavHealth.virus_db_age_hours == null ? '-' : `${clamavHealth.virus_db_age_hours} 小时` }}</el-descriptions-item>
        <el-descriptions-item label="自动重试">最多 {{ clamavHealth.retry?.max_attempts || config.clamav?.retry?.max_attempts || 0 }} 次，基础间隔 {{ clamavHealth.retry?.base_interval_minutes || config.clamav?.retry?.base_interval_minutes || 0 }} 分钟</el-descriptions-item>
        <el-descriptions-item label="批次上限">{{ clamavHealth.retry?.batch_size || config.clamav?.retry?.batch_size || 0 }} 个文件</el-descriptions-item>
        <el-descriptions-item label="待重试 / 扫描中">{{ clamavHealth.retry?.retryable || 0 }} / {{ clamavHealth.retry?.pending || 0 }}</el-descriptions-item>
        <el-descriptions-item label="已耗尽 / 已感染">{{ clamavHealth.retry?.exhausted || 0 }} / {{ clamavHealth.retry?.infected || 0 }}</el-descriptions-item>
        <el-descriptions-item label="下次重试">{{ formatDateTime(clamavHealth.retry?.next_retry_at) }}</el-descriptions-item>
        <el-descriptions-item label="检查时间">{{ formatDateTime(clamavHealth.checked_at) }}</el-descriptions-item>
      </el-descriptions>
      <el-alert v-if="clamavHealth.virus_db_stale" class="clamav-warning" type="warning" :closable="false" show-icon :title="`病毒库已超过 ${clamavHealth.virus_db_max_age_hours} 小时，请尽快更新。`" />
      <el-alert v-if="clamavHealth.retry?.exhausted" class="clamav-warning" type="error" :closable="false" show-icon :title="`${clamavHealth.retry.exhausted} 个文件已耗尽自动重试次数，需要检查 ClamAV 或手动重扫。`" />
      <el-alert v-if="clamavHealth.retry?.infected" class="clamav-warning" type="error" :closable="false" show-icon :title="`${clamavHealth.retry.infected} 个文件被检出感染，下载和外链已被阻断。`" />
    </el-card>

    <el-card v-if="showSection('backup')" shadow="never" class="table-card clamav-health-card">
      <template #header>
        <div class="card-header-row">
          <span>备份存储</span>
          <el-tag :type="config.backup?.configured ? 'success' : 'info'" size="small">{{ config.backup?.configured ? '已配置' : '未配置' }}</el-tag>
        </div>
      </template>
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item label="存储类型">{{ config.backup?.type || '-' }}</el-descriptions-item>
        <el-descriptions-item label="前缀">{{ config.backup?.prefix || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="config.backup?.type !== 'local'" label="区域">{{ config.backup?.region || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="config.backup?.type !== 'local'" label="Bucket">{{ config.backup?.bucket || '-' }}</el-descriptions-item>
        <el-descriptions-item label="清单保护" :span="2">
          <el-tag :type="config.backup?.manifest_encryption_enabled ? 'success' : 'danger'" size="small">
            {{ config.backup?.manifest_encryption_enabled ? config.backup?.manifest_format : '未启用' }}
          </el-tag>
        </el-descriptions-item>
		<el-descriptions-item label="自动基线压缩"><el-tag :type="config.backup?.compaction_enabled ? 'success' : 'info'" size="small">{{ config.backup?.compaction_enabled ? '已启用' : '未启用' }}</el-tag></el-descriptions-item>
		<el-descriptions-item label="增量链阈值">{{ config.backup?.compaction_incremental_threshold || '-' }} 个增量</el-descriptions-item>
		<el-descriptions-item label="压缩检查周期" :span="2">{{ config.backup?.compaction_interval_minutes || '-' }} 分钟</el-descriptions-item>
        <el-descriptions-item v-if="config.backup?.type !== 'local'" label="Endpoint" :span="2">{{ config.backup?.endpoint || '-' }}</el-descriptions-item>
        <el-descriptions-item v-if="config.backup?.type === 'local'" label="本地路径" :span="2">{{ config.backup?.local_path || '-' }}</el-descriptions-item>
		<el-descriptions-item label="主存模式">{{ config.archive?.primary_mode === 'cloud_mount' ? '云存储挂载' : '本地 POSIX' }}</el-descriptions-item>
		<el-descriptions-item label="自动归档"><el-tag :type="config.archive?.enabled ? 'success' : 'info'" size="small">{{ config.archive?.enabled ? '已启用' : '未启用' }}</el-tag></el-descriptions-item>
		<el-descriptions-item label="冷数据阈值">{{ config.archive?.after_days ? `${config.archive.after_days} 天未访问` : '-' }}</el-descriptions-item>
		<el-descriptions-item label="单批上限">{{ config.archive?.batch_size || '-' }}</el-descriptions-item>
		<el-descriptions-item label="归档前缀" :span="2">{{ config.archive?.prefix || '-' }}</el-descriptions-item>
      </el-descriptions>
    </el-card>

    <el-card v-if="showStorageHealth" shadow="never" class="table-card">
      <template #header><span>存储健康</span></template>
      <el-descriptions :column="2" border size="small">
        <el-descriptions-item label="主存目录">{{ health.root?.path || '-' }}</el-descriptions-item>
        <el-descriptions-item label="状态">
          <el-tag :type="storageStatusType(health.status)" size="small">
            {{ storageStatusLabel(health.status) }}
          </el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="总容量">{{ formatBytes(health.root?.total_bytes) }}</el-descriptions-item>
        <el-descriptions-item label="主存可用">{{ formatBytes(health.root?.free_bytes) }}（{{ formatPercent(health.root?.free_percent) }}）</el-descriptions-item>
        <el-descriptions-item label="主存已用">{{ formatBytes(health.root?.used_bytes) }}</el-descriptions-item>
        <el-descriptions-item label="暂存目录">{{ health.staging?.path || '-' }}</el-descriptions-item>
        <el-descriptions-item label="暂存状态">
          <el-tag :type="pathStatusType(health.staging)" size="small">{{ pathStatusLabel(health.staging) }}</el-tag>
        </el-descriptions-item>
        <el-descriptions-item label="暂存可用">{{ formatBytes(health.staging?.free_bytes) }}（{{ formatPercent(health.staging?.free_percent) }}）</el-descriptions-item>
        <template v-if="health.backup">
          <el-descriptions-item label="本地备份目录">{{ health.backup.path || '-' }}</el-descriptions-item>
          <el-descriptions-item label="备份状态">
            <el-tag :type="pathStatusType(health.backup)" size="small">{{ pathStatusLabel(health.backup) }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label="备份可用">{{ formatBytes(health.backup.free_bytes) }}（{{ formatPercent(health.backup.free_percent) }}）</el-descriptions-item>
        </template>
        <el-descriptions-item label="告警阈值" :span="2">低于 {{ health.warn_free_percent || 0 }}% 预警；低于 {{ health.min_free_percent || 0 }}% 或 {{ formatBytes(health.min_free_bytes) }} 时高优先级</el-descriptions-item>
      </el-descriptions>
      <el-alert
        v-for="alert in health.alerts || []"
        :key="alert"
        :title="alert"
        :type="['critical', 'degraded'].includes(health.status) ? 'error' : 'warning'"
        :closable="false"
        class="reconcile-alert"
        show-icon
      />
      <div class="reconcile-actions">
        <el-button :icon="Search" :loading="reconcileLoading" @click="scanObjects">扫描对象引用</el-button>
        <el-button
          v-if="reconcile?.orphan_objects?.length"
          type="danger"
          plain
          :loading="reconcileQuarantineLoading"
          @click="quarantineOrphanObjects"
        >
          隔离孤儿对象
        </el-button>
        <span v-if="reconcile" class="reconcile-summary">扫描 {{ reconcile.scanned }} 个对象，孤儿 {{ reconcile.orphan_objects?.length || 0 }} 个，缺失 {{ reconcile.missing_objects?.length || 0 }} 个</span>
      </div>
      <el-alert
        v-if="reconcile?.orphan_objects?.length"
        title="发现孤儿对象"
        type="warning"
        :closable="false"
        class="reconcile-alert"
        show-icon
      >
        <div class="reconcile-alert-body">
          <span>隔离后保留 {{ config.lifecycle?.quarantine_retention_days || 7 }} 天；到期时系统会再次检查文件版本、外链和下载任务引用，仍被引用的对象会自动恢复。</span>
          <code v-for="key in previewOrphanObjects" :key="key">{{ key }}</code>
          <span v-if="(reconcile.orphan_objects?.length || 0) > previewOrphanObjects.length">还有 {{ reconcile.orphan_objects.length - previewOrphanObjects.length }} 个对象未展示。</span>
        </div>
      </el-alert>
      <el-alert
        v-if="reconcile?.quarantined_objects?.length"
        :title="`已隔离 ${reconcile.quarantined_objects.length} 个孤儿对象`"
        type="success"
        :closable="false"
        class="reconcile-alert"
        show-icon
      />
      <div v-if="reconcile?.quarantine_records?.length" class="quarantine-records">
        <div class="quarantine-records__header">
          <strong>隔离处理记录</strong>
          <span>失败任务会由生命周期任务自动重试</span>
        </div>
        <el-table :data="reconcile.quarantine_records" border stripe class="center-table">
          <el-table-column prop="storage_key" label="对象" min-width="260" show-overflow-tooltip />
          <el-table-column label="状态" width="110">
            <template #default="{ row }">
              <el-tag :type="quarantineStatusType(row.status)" effect="light" size="small">
                {{ quarantineStatusLabel(row.status) }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="隔离时间" min-width="168">
            <template #default="{ row }">{{ formatDateTime(row.quarantined_at) }}</template>
          </el-table-column>
          <el-table-column label="计划清理" min-width="168">
            <template #default="{ row }">{{ formatDateTime(row.purge_after) }}</template>
          </el-table-column>
          <el-table-column prop="retry_count" label="重试" width="80" />
          <el-table-column prop="last_error" label="最近错误" min-width="180" show-overflow-tooltip>
            <template #default="{ row }">{{ row.last_error || '-' }}</template>
          </el-table-column>
        </el-table>
      </div>
    </el-card>
    </template>

    <el-drawer v-model="ldapDrawerVisible" title="修改 LDAP 配置" size="560px" destroy-on-close class="ldap-config-drawer">
      <el-alert
        class="ldap-form-tip"
        type="info"
        :closable="false"
        show-icon
        title="配置会立即保存到数据库"
        description="启用后登录流程会实时读取此配置；密码不回显，留空保存时会保留数据库中的旧密码。"
      />
      <el-form ref="ldapFormRef" :model="ldapForm" :rules="ldapRules" label-width="120px" class="ldap-form">
        <el-form-item label="启用状态" prop="status">
          <el-switch v-model="ldapForm.status" :active-value="1" :inactive-value="0" active-text="启用" inactive-text="禁用" />
        </el-form-item>

        <el-divider content-position="left">服务器连接</el-divider>

        <el-form-item label="主机地址" prop="host">
          <el-input v-model="ldapForm.host" placeholder="ldap.example.com 或 ldaps://ldap.example.com" clearable />
        </el-form-item>
        <el-form-item label="端口" prop="port">
          <el-input-number v-model="ldapForm.port" :min="1" :max="65535" :controls="false" style="width: 100%" />
        </el-form-item>
        <el-form-item label="传输模式" prop="transport">
          <el-select v-model="ldapForm.transport" style="width: 100%">
            <el-option label="StartTLS（推荐，LDAP 389）" value="starttls" />
            <el-option label="LDAPS（TLS 636）" value="ldaps" />
            <el-option label="明文 LDAP（仅开发环境）" value="plain" />
          </el-select>
          <div class="form-help">生产模式禁止明文 LDAP；证书校验默认使用系统 CA。</div>
        </el-form-item>
        <el-form-item v-if="ldapForm.transport !== 'plain'" label="TLS 服务器名" prop="tls_server_name">
          <el-input v-model="ldapForm.tls_server_name" placeholder="留空则使用主机名" clearable />
        </el-form-item>
        <el-form-item v-if="ldapForm.transport !== 'plain'" label="最低 TLS 版本" prop="tls_min_version">
          <el-select v-model="ldapForm.tls_min_version" style="width: 100%">
            <el-option label="TLS 1.2" value="1.2" />
            <el-option label="TLS 1.3" value="1.3" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="ldapForm.transport !== 'plain'" label="自定义 CA" prop="tls_ca">
          <el-input v-model="ldapForm.tls_ca" type="textarea" :rows="4" placeholder="可选，粘贴 PEM 格式 CA 证书" />
        </el-form-item>
        <el-form-item label="管理员 DN" prop="admin_dn">
          <el-input v-model="ldapForm.admin_dn" placeholder="cn=admin,dc=example,dc=com" clearable />
        </el-form-item>
        <el-form-item label="管理员密码" prop="password">
          <el-input v-model="ldapForm.password" type="password" show-password placeholder="首次启用必填；不修改请留空" />
        </el-form-item>

        <el-divider content-position="left">搜索与属性映射</el-divider>

        <el-form-item label="Base DN" prop="base_dn">
          <el-input v-model="ldapForm.base_dn" placeholder="ou=users,dc=example,dc=com" clearable />
        </el-form-item>
        <el-form-item label="用户过滤器" prop="user_filter">
          <el-input v-model="ldapForm.user_filter" placeholder="(&(objectClass=user)(sAMAccountName=*))" clearable />
        </el-form-item>
        <el-form-item label="用户名属性" prop="username_attr">
          <el-input v-model="ldapForm.username_attr" placeholder="sAMAccountName" clearable />
        </el-form-item>
        <el-form-item label="姓名属性" prop="real_name_attr">
          <el-input v-model="ldapForm.real_name_attr" placeholder="displayName" clearable />
        </el-form-item>
        <el-form-item label="邮箱属性" prop="email_attr">
          <el-input v-model="ldapForm.email_attr" placeholder="mail" clearable />
        </el-form-item>
        <el-form-item label="同步周期" prop="sync_cron">
          <el-input v-model="ldapForm.sync_cron" placeholder="0 0 2 * * *" clearable>
            <template #append>cron</template>
          </el-input>
          <div class="form-help">支持 6 段 cron（秒 分 时 日 月 周），默认每天凌晨 2 点；也兼容常见 5 段表达式。</div>
        </el-form-item>

        <el-divider content-position="left">用户组同步</el-divider>

        <el-form-item label="同步用户组" prop="group_sync_enabled">
          <el-switch
            v-model="ldapForm.group_sync_enabled"
            :active-value="1"
            :inactive-value="0"
            active-text="启用"
            inactive-text="禁用"
          />
          <div class="form-help">启用后，同步任务会将 LDAP 组映射为目标工作空间内的用户组，并按 LDAP 当前成员替换成员关系。</div>
        </el-form-item>
        <el-form-item label="目标工作空间" prop="sync_workspace_id">
          <el-select
            v-model="ldapForm.sync_workspace_id"
            placeholder="请选择 LDAP 组同步到哪个工作空间"
            clearable
            filterable
            style="width: 100%"
            :disabled="ldapForm.group_sync_enabled !== 1"
          >
            <el-option
              v-for="workspace in workspaceOptions"
              :key="workspace.id"
              :label="workspace.name"
              :value="workspace.id"
            >
              <span>{{ workspace.name }}</span>
              <span class="workspace-option-code">{{ workspace.code }}</span>
            </el-option>
          </el-select>
          <div class="form-help">LDAP 用户组是工作空间内的权限主体，因此需要明确同步目标。</div>
        </el-form-item>
        <el-form-item label="组 Base DN" prop="group_base_dn">
          <el-input
            v-model="ldapForm.group_base_dn"
            placeholder="ou=groups,dc=example,dc=com；留空默认使用用户 Base DN"
            :disabled="ldapForm.group_sync_enabled !== 1"
            clearable
          />
        </el-form-item>
        <el-form-item label="组过滤器" prop="group_filter">
          <el-input
            v-model="ldapForm.group_filter"
            placeholder="(objectClass=group)"
            :disabled="ldapForm.group_sync_enabled !== 1"
            clearable
          />
        </el-form-item>
        <el-form-item label="组名属性" prop="group_name_attr">
          <el-input
            v-model="ldapForm.group_name_attr"
            placeholder="cn"
            :disabled="ldapForm.group_sync_enabled !== 1"
            clearable
          />
        </el-form-item>
        <el-form-item label="成员属性" prop="group_member_attr">
          <el-input
            v-model="ldapForm.group_member_attr"
            placeholder="member 或 memberUid"
            :disabled="ldapForm.group_sync_enabled !== 1"
            clearable
          />
          <div class="form-help">`member` 适合 DN 列表，`memberUid` 适合用户名列表；系统会自动按 DN 或用户名匹配本次同步到的 LDAP 用户。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <div class="drawer-footer">
          <el-button @click="ldapDrawerVisible = false">取消</el-button>
          <el-button :loading="ldapTesting" @click="testLDAP(ldapForm)">测试连接</el-button>
          <el-button type="primary" :loading="ldapSaving" @click="saveLDAP">保存配置</el-button>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Clock, Collection, Connection, Monitor, Refresh, Search } from '@element-plus/icons-vue'
import { systemApi } from '../../api/system'
import { ElMessage, ElMessageBox } from 'element-plus'
import { formatDateTime } from '../../utils/date'
import { useUserStore } from '../../stores/user'

const loading = ref(false)
const route = useRoute()
const userStore = useUserStore()
const config = ref({ ldap: {}, backup: {}, clamav: {}, lifecycle: {} })
const health = ref({})
const ldapFormRef = ref(null)
const ldapDrawerVisible = ref(false)
const ldapTesting = ref(false)
const ldapSaving = ref(false)
const clamavTesting = ref(false)
const clamavHealth = ref(null)
const reconcileLoading = ref(false)
const reconcileQuarantineLoading = ref(false)
const reconcile = ref(null)
const ldapForm = ref(defaultLDAPForm())
const ldapSyncing = ref(false)
const ldapHistoryLoading = ref(false)
const ldapHistory = ref([])
const ldapHistoryTotal = ref(0)
const ldapHistoryQuery = ref({ page: 1, page_size: 10 })

const dependencyCards = computed(() => [
  { key: 'ldap', label: 'LDAP', icon: Connection, enabled: !!config.value.ldap?.enabled, summary: config.value.ldap?.host ? ldapConnectionText.value : '使用本地账号登录' },
  { key: 'clamav', label: 'ClamAV', icon: Monitor, enabled: !!config.value.clamav?.enabled, summary: config.value.clamav?.enabled ? `${config.value.clamav.host}:${config.value.clamav.port}` : '上传后标记为 unscanned' },
  { key: 'backup', label: '备份存储', icon: Collection, enabled: !!config.value.backup?.configured, summary: config.value.backup?.configured ? `${config.value.backup.type} · ${config.value.backup.prefix || '-'}` : '尚未配置备份出口' }
])

const clamavEnabled = computed(() => !!config.value.clamav?.enabled)
const hasLDAPConfig = computed(() => {
  const ldap = config.value.ldap || {}
  return !!(String(ldap.host || '').trim() && Number(ldap.port || 0) > 0 && String(ldap.admin_dn || '').trim() && String(ldap.base_dn || '').trim() && String(ldap.username_attr || '').trim())
})
const ldapConnectionText = computed(() => {
  const ldap = config.value.ldap || {}
  const host = String(ldap.host || '').trim()
  if (!host) return '-'
  const transport = String(ldap.transport || 'starttls').toLowerCase()
  const label = transport === 'ldaps' ? 'LDAPS' : transport === 'plain' ? 'LDAP' : 'StartTLS'
  return `${label} · ${host.includes('://') ? host.replace(/^\w+:\/\//, '') : `${host}:${ldap.port || 389}`}`
})
const activeSection = computed(() => route.meta?.systemSection || 'overview')
const visibleDependencyCards = computed(() => {
  if (activeSection.value === 'overview') return dependencyCards.value
  return dependencyCards.value.filter((item) => item.key === activeSection.value)
})
const showStorageHealth = computed(() => activeSection.value === 'overview' || activeSection.value === 'backup')
const workspaceOptions = computed(() => userStore.workspaces || [])
const previewOrphanObjects = computed(() => (reconcile.value?.orphan_objects || []).slice(0, 5))

function showSection(section) {
  return activeSection.value === 'overview' || activeSection.value === section
}

const requiredWhenEnabled = (message) => (_rule, value, callback) => {
  if (ldapForm.value.status === 1 && !String(value || '').trim()) {
    callback(new Error(message))
    return
  }
  callback()
}

const passwordRequiredWhenFirstEnable = (_rule, value, callback) => {
  if (ldapForm.value.status === 1 && !config.value.ldap?.id && !String(value || '').trim()) {
    callback(new Error('首次启用 LDAP 时请输入管理员密码'))
    return
  }
  callback()
}

const ldapRules = {
  host: [{ validator: requiredWhenEnabled('请输入 LDAP 主机地址'), trigger: 'blur' }],
  port: [{ required: true, message: '请输入端口', trigger: 'blur' }],
  admin_dn: [{ validator: requiredWhenEnabled('请输入管理员 DN'), trigger: 'blur' }],
  password: [{ validator: passwordRequiredWhenFirstEnable, trigger: 'blur' }],
  base_dn: [{ validator: requiredWhenEnabled('请输入 Base DN'), trigger: 'blur' }],
  username_attr: [{ validator: requiredWhenEnabled('请输入用户名属性'), trigger: 'blur' }],
  sync_workspace_id: [{ validator: groupRequired('请选择目标工作空间'), trigger: 'change' }],
  group_filter: [{ validator: groupRequired('请输入组过滤器'), trigger: 'blur' }],
  group_name_attr: [{ validator: groupRequired('请输入组名属性'), trigger: 'blur' }],
  group_member_attr: [{ validator: groupRequired('请输入成员属性'), trigger: 'blur' }]
}

onMounted(load)

watch(activeSection, (section) => {
  if (section === 'ldap') loadLDAPHistory()
  if (section === 'clamav' && clamavEnabled.value) loadClamAVHealth(false)
})

async function load() {
  loading.value = true
  try {
    const [configRes, healthRes] = await Promise.all([systemApi.config(), systemApi.storageHealth()])
    config.value = normalizeSystemConfig(configRes.data || config.value)
    health.value = healthRes.data || {}
    if (activeSection.value === 'ldap') loadLDAPHistory()
    if (activeSection.value === 'clamav' && clamavEnabled.value) loadClamAVHealth(false)
  } finally {
    loading.value = false
  }
}

async function openLDAPDrawer() {
  const res = await systemApi.ldapConfig()
  config.value = normalizeSystemConfig({ ...config.value, ldap: res.data || {} })
  ldapForm.value = normalizeLDAPConfig(config.value.ldap)
  ldapDrawerVisible.value = true
}

async function saveLDAP() {
  if (!ldapFormRef.value) return
  await ldapFormRef.value.validate()
  ldapSaving.value = true
  try {
    const res = await systemApi.saveLDAP(prepareLDAPPayload(ldapForm.value))
    config.value = normalizeSystemConfig({ ...config.value, ldap: res.data || {} })
    ldapForm.value = normalizeLDAPConfig(config.value.ldap)
    ldapDrawerVisible.value = false
    ElMessage.success('LDAP 配置已保存')
    if (activeSection.value === 'ldap') loadLDAPHistory()
  } finally {
    ldapSaving.value = false
  }
}

async function manualLDAPSync() {
  if (!config.value.ldap?.enabled) {
    ElMessage.warning('请先完成并启用 LDAP 配置')
    return
  }
  const scopeText = config.value.ldap?.group_sync_enabled === 1 ? '用户、用户组和组成员关系' : '用户'
  try {
    await ElMessageBox.confirm(`将立即从 LDAP 拉取${scopeText}并同步到本地。与本地账号或本地用户组同名的 LDAP 条目会被跳过，确定继续吗？`, '确认手动同步 LDAP', {
      type: 'warning',
      confirmButtonText: '开始同步',
      cancelButtonText: '取消'
    })
  } catch (_e) {
    return
  }
  ldapSyncing.value = true
  try {
    await systemApi.ldapSync()
    ElMessage.success('LDAP 同步任务已启动')
    setTimeout(loadLDAPHistory, 900)
  } finally {
    ldapSyncing.value = false
  }
}

async function loadLDAPHistory() {
  if (activeSection.value !== 'ldap') return
  ldapHistoryLoading.value = true
  try {
    const res = await systemApi.ldapSyncHistory(ldapHistoryQuery.value)
    const data = res.data || {}
    ldapHistory.value = data.list || []
    ldapHistoryTotal.value = Number(data.total || 0)
    ldapHistoryQuery.value = {
      page: Number(data.page || ldapHistoryQuery.value.page),
      page_size: Number(data.page_size || ldapHistoryQuery.value.page_size)
    }
  } finally {
    ldapHistoryLoading.value = false
  }
}

function handleLDAPHistoryPageChange(page) {
  ldapHistoryQuery.value.page = page
  loadLDAPHistory()
}

function handleLDAPHistorySizeChange(pageSize) {
  ldapHistoryQuery.value.page = 1
  ldapHistoryQuery.value.page_size = pageSize
  loadLDAPHistory()
}

async function testLDAP(source) {
  const payload = prepareLDAPPayload(source || config.value.ldap)
  if (!payload.host || !payload.base_dn || !payload.admin_dn || !payload.username_attr) {
    ElMessage.warning('请先填写 LDAP 主机、管理员 DN、Base DN 和用户名属性')
    return
  }
  if (!config.value.ldap?.id && !payload.password) {
    ElMessage.warning('首次测试 LDAP 时请输入管理员密码')
    return
  }
  ldapTesting.value = true
  try {
    await systemApi.ldapTest(payload)
    ElMessage.success('LDAP 连接测试通过')
  } finally {
    ldapTesting.value = false
  }
}

async function testClamAV() {
  await loadClamAVHealth(true)
}

async function loadClamAVHealth(notify = false) {
  clamavTesting.value = true
  try {
    const res = await systemApi.clamavHealth()
    clamavHealth.value = res.data
    if (notify) {
      if (res.data?.virus_db_stale) ElMessage.warning('ClamAV 可连接，但病毒库已超过更新阈值')
      else ElMessage.success('ClamAV 健康检查通过')
    }
  } finally { clamavTesting.value = false }
}

async function scanObjects() {
  reconcileLoading.value = true
  try { const res = await systemApi.reconcile(); reconcile.value = res.data; ElMessage.success('对象引用扫描完成') } finally { reconcileLoading.value = false }
}

async function quarantineOrphanObjects() {
  const orphanObjects = reconcile.value?.orphan_objects || []
  if (!orphanObjects.length) {
    ElMessage.info('当前没有可隔离的孤儿对象')
    return
  }
  try {
    await ElMessageBox.prompt(
      `将把 ${orphanObjects.length} 个当前工作空间孤儿对象移入隔离区，保留 ${config.value.lifecycle?.quarantine_retention_days || 7} 天后复核引用再清理。请输入 QUARANTINE 确认。`,
      '确认隔离孤儿对象',
      {
        type: 'warning',
        confirmButtonText: '隔离',
        cancelButtonText: '取消',
        inputPattern: /^QUARANTINE$/,
        inputErrorMessage: '请输入 QUARANTINE'
      }
    )
  } catch (_e) {
    return
  }
  reconcileQuarantineLoading.value = true
  try {
    const res = await systemApi.quarantineOrphans({ storage_keys: orphanObjects, confirm: 'QUARANTINE' })
    reconcile.value = res.data
    const failed = reconcile.value?.failed_objects?.length || 0
    if (failed > 0) ElMessage.warning(`隔离完成，但 ${failed} 个对象处理失败`)
    else ElMessage.success(`已隔离 ${reconcile.value?.quarantined_objects?.length || 0} 个孤儿对象`)
  } finally {
    reconcileQuarantineLoading.value = false
  }
}

function formatBytes(value) {
  const bytes = Number(value || 0)
  if (bytes < 1024) return `${bytes} B`
  const units = ['KB', 'MB', 'GB', 'TB', 'PB']
  let size = bytes / 1024
  let index = 0
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1 }
  return `${size.toFixed(size >= 10 ? 0 : 1)} ${units[index]}`
}

function formatPercent(value) {
  const percent = Number(value)
  return Number.isFinite(percent) ? `${percent.toFixed(1)}%` : '-'
}

function storageStatusLabel(status) {
  return ({ ready: '健康', warning: '容量预警', critical: '容量严重不足', degraded: '不可写' })[status] || '未知'
}

function storageStatusType(status) {
  return ({ ready: 'success', warning: 'warning', critical: 'danger', degraded: 'danger' })[status] || 'info'
}

function pathStatusLabel(item) {
  if (!item?.available) return '不可用'
  if (!item?.writable) return '不可写'
  if (item?.critical_space) return '容量严重不足'
  if (item?.low_space) return '容量预警'
  return '可读写'
}

function pathStatusType(item) {
  if (!item?.available || !item?.writable) return 'danger'
  if (item?.critical_space) return 'danger'
  return item?.low_space ? 'warning' : 'success'
}

function quarantineStatusLabel(status) {
  return ({ quarantined: '隔离中', purged: '已清理', restored: '已恢复' })[status] || status || '-'
}

function quarantineStatusType(status) {
  return ({ quarantined: 'warning', purged: 'info', restored: 'success' })[status] || 'info'
}

function defaultLDAPForm() {
  return {
    id: 0,
    host: '',
    port: 389,
    admin_dn: '',
    password: '',
    transport: 'starttls',
    tls_ca: '',
    tls_server_name: '',
    tls_min_version: '1.2',
    base_dn: '',
    user_filter: '(&(objectClass=user)(sAMAccountName=*))',
    username_attr: 'sAMAccountName',
    real_name_attr: 'displayName',
    email_attr: 'mail',
    sync_cron: '0 0 2 * * *',
    sync_workspace_id: null,
    group_sync_enabled: 0,
    group_base_dn: '',
    group_filter: '(objectClass=group)',
    group_name_attr: 'cn',
    group_member_attr: 'member',
    status: 0
  }
}

function normalizeSystemConfig(value) {
  const next = value || {}
  return {
    ...next,
    ldap: normalizeLDAPConfig(next.ldap),
    backup: next.backup || {},
    clamav: next.clamav || {}
  }
}

function normalizeLDAPConfig(value = {}) {
  const defaults = defaultLDAPForm()
  return {
    ...defaults,
    ...value,
    port: Number(value.port || defaults.port),
    password: '',
    transport: String(value.transport || defaults.transport).toLowerCase(),
    tls_ca: String(value.tls_ca || ''),
    tls_server_name: String(value.tls_server_name || ''),
    tls_min_version: String(value.tls_min_version || defaults.tls_min_version),
    real_name_attr: value.real_name_attr || value.realname_attr || defaults.real_name_attr,
    sync_cron: value.sync_cron || defaults.sync_cron,
    sync_workspace_id: value.sync_workspace_id ? Number(value.sync_workspace_id) : null,
    group_sync_enabled: Number(value.group_sync_enabled || 0),
    group_base_dn: value.group_base_dn || value.base_dn || defaults.group_base_dn,
    group_filter: value.group_filter || defaults.group_filter,
    group_name_attr: value.group_name_attr || defaults.group_name_attr,
    group_member_attr: value.group_member_attr || defaults.group_member_attr,
    status: Number(value.status || 0)
  }
}

function prepareLDAPPayload(value = {}) {
  const defaults = defaultLDAPForm()
  const source = {
    ...defaults,
    ...value,
    real_name_attr: value.real_name_attr || value.realname_attr || defaults.real_name_attr
  }
  return {
    host: String(source.host || '').trim(),
    port: Number(source.port || 389),
    admin_dn: String(source.admin_dn || '').trim(),
    password: String(source.password || ''),
    transport: String(source.transport || defaults.transport).toLowerCase(),
    tls_ca: String(source.tls_ca || '').trim(),
    tls_server_name: String(source.tls_server_name || '').trim(),
    tls_min_version: String(source.tls_min_version || defaults.tls_min_version),
    base_dn: String(source.base_dn || '').trim(),
    user_filter: String(source.user_filter || '').trim(),
    username_attr: String(source.username_attr || '').trim(),
    real_name_attr: String(source.real_name_attr || source.realname_attr || '').trim(),
    email_attr: String(source.email_attr || '').trim(),
    sync_cron: String(source.sync_cron || '').trim(),
    sync_workspace_id: Number(source.sync_workspace_id || 0),
    group_sync_enabled: Number(source.group_sync_enabled || 0),
    group_base_dn: String(source.group_base_dn || source.base_dn || '').trim(),
    group_filter: String(source.group_filter || '').trim(),
    group_name_attr: String(source.group_name_attr || '').trim(),
    group_member_attr: String(source.group_member_attr || '').trim(),
    status: Number(source.status || 0)
  }
}

function syncTypeLabel(value) {
  return value === 'manual' ? '手动' : '定时'
}

function syncStatusLabel(value) {
  if (value === 'success') return '成功'
  if (value === 'failed') return '失败'
  if (value === 'running') return '执行中'
  return value || '-'
}

function syncStatusType(value) {
  if (value === 'success') return 'success'
  if (value === 'failed') return 'danger'
  if (value === 'running') return 'warning'
  return 'info'
}

function syncDuration(row) {
  if (!row?.start_time || !row?.end_time) return row?.status === 'running' ? '执行中' : '-'
  const start = new Date(row.start_time).getTime()
  const end = new Date(row.end_time).getTime()
  if (Number.isNaN(start) || Number.isNaN(end) || end < start) return '-'
  const seconds = Math.max(1, Math.round((end - start) / 1000))
  return seconds < 60 ? `${seconds}s` : `${Math.floor(seconds / 60)}m ${seconds % 60}s`
}

function groupRequired(message) {
  return (_rule, value, callback) => {
    if (ldapForm.value.status === 1 && ldapForm.value.group_sync_enabled === 1 && !String(value || '').trim()) {
      callback(new Error(message))
      return
    }
    callback()
  }
}

function groupSyncSummary(row) {
  const created = Number(row?.group_success_count || 0)
  const updated = Number(row?.group_update_count || 0)
  const skipped = Number(row?.group_skip_count || 0)
  if (!created && !updated && !skipped) return '-'
  return `新 ${created} / 更 ${updated} / 跳 ${skipped}`
}
</script>

<style scoped>
.ldap-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
}
.ldap-compact-card {
  overflow: hidden;
  border: 1px solid color-mix(in srgb, var(--accent-primary) 14%, var(--panel-border));
  border-radius: 18px;
  background: color-mix(in srgb, var(--panel-bg-strong) 96%, var(--accent-green) 4%);
  box-shadow: 0 18px 48px rgba(15, 23, 42, 0.06);
}
.ldap-compact-card :deep(.el-card__body) {
  padding: 26px 32px;
}
.ldap-config-card {
  min-height: 96px;
}
.ldap-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  min-width: 0;
}
.ldap-card-title {
  display: inline-flex;
  align-items: center;
  min-width: 0;
  gap: 12px;
  color: var(--text-primary);
  font-size: 18px;
  font-weight: 800;
  letter-spacing: 0.01em;
}
.ldap-title-icon {
  display: inline-grid;
  place-items: center;
  width: 30px;
  height: 30px;
  flex: 0 0 auto;
  border-radius: 10px;
  color: var(--accent-green);
  background: color-mix(in srgb, var(--accent-green) 11%, transparent);
}
.ldap-schedule-summary {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  margin-left: 4px;
  color: var(--text-muted);
  font-size: 13px;
  font-weight: 600;
}
.ldap-schedule-summary b {
  padding: 4px 8px;
  border-radius: 6px;
  background: var(--surface-muted);
  color: var(--text-secondary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  font-weight: 700;
}
.ldap-card-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 14px;
}
.ldap-card-actions .el-button {
  min-width: 116px;
  margin-left: 0 !important;
  border-radius: 16px;
  font-weight: 800;
}
.ldap-status-tag {
  min-width: 78px;
  justify-content: center;
  border-radius: 999px;
  font-weight: 800;
}
.ldap-history-card :deep(.el-card__header) {
  padding: 26px 32px;
  border-bottom: 1px solid color-mix(in srgb, var(--accent-green) 28%, var(--border-color));
}
.ldap-history-card :deep(.el-card__body) {
  padding: 0;
}
.ldap-history-body {
  display: flex;
  min-height: 430px;
  flex-direction: column;
  background: var(--panel-bg-strong);
}
.ldap-empty-state {
  display: grid;
  min-height: 390px;
  place-items: center;
}
.ldap-empty-state :deep(.el-empty__description p) {
  color: color-mix(in srgb, var(--accent-green) 50%, var(--text-secondary));
  font-size: 15px;
  font-weight: 800;
}
.ldap-history-body .history-pagination {
  margin: 18px 0 22px;
}
.system-config-grid { grid-template-columns: repeat(3, minmax(0, 1fr)); margin-bottom: 20px; }
.dependency-card :deep(.el-card__body) { padding: 18px; }
.dependency-header { display: flex; align-items: center; gap: 12px; min-width: 0; }
.dependency-icon { display: grid; place-items: center; width: 38px; height: 38px; flex: 0 0 auto; border-radius: var(--radius-sm); color: var(--accent-primary); background: var(--surface-muted); }
.dependency-title { display: flex; min-width: 0; flex: 1; flex-direction: column; gap: 3px; }
.dependency-title strong { color: var(--text-primary); font-size: 14px; }
.dependency-title span { overflow: hidden; color: var(--text-secondary); font-size: 12px; text-overflow: ellipsis; white-space: nowrap; }
.dependency-actions { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-top: 14px; padding-top: 12px; border-top: 1px solid var(--border-color); }
.clamav-health-card { margin-bottom: 20px; }
.card-header-row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.ldap-title-block { display: flex; min-width: 0; flex-direction: column; gap: 4px; }
.ldap-title-block span { color: var(--text-primary); font-weight: 700; }
.ldap-title-block small { color: var(--text-muted); font-size: 12px; font-weight: 500; }
.ldap-config-actions,
.drawer-footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: wrap;
  gap: 10px;
}
.ldap-config-actions .el-button,
.drawer-footer .el-button,
.dependency-actions .el-button {
  margin-left: 0 !important;
}
.ldap-form-tip { margin-bottom: 18px; }
.ldap-form :deep(.el-divider__text) {
  color: var(--text-secondary);
  font-weight: 700;
}
.form-help {
  margin-top: 6px;
  color: var(--text-muted);
  font-size: 12px;
  line-height: 1.5;
}
.workspace-option-code {
  float: right;
  margin-left: 16px;
  color: var(--text-muted);
  font-size: 12px;
}
.ldap-history-card { margin-bottom: 20px; }
.ldap-history-table :deep(.el-table__cell) {
  text-align: center;
}
.ldap-history-table :deep(.cell) {
  display: flex;
  align-items: center;
  justify-content: center;
  min-width: 0;
}
:deep(.ldap-config-drawer) {
  width: min(560px, 92vw) !important;
}
.clamav-warning { margin-top: 16px; }
.config-source-tip { margin-bottom: 16px; }
.reconcile-actions { display: flex; align-items: center; gap: 16px; margin-top: 18px; flex-wrap: wrap; }
.reconcile-summary { color: var(--text-secondary); font-size: 13px; }
.reconcile-alert { margin-top: 16px; }
.reconcile-alert-body {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.reconcile-alert-body code {
  display: block;
  padding: 6px 8px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--accent-warning) 10%, var(--surface-muted));
  color: var(--text-secondary);
  font-size: 12px;
  word-break: break-all;
}
.quarantine-records { margin-top: 18px; }
.quarantine-records__header {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 10px;
}
.quarantine-records__header strong { color: var(--text-primary); font-size: 14px; }
.quarantine-records__header span { color: var(--text-muted); font-size: 12px; }
@media (max-width: 760px) {
  .ldap-page {
    min-height: auto;
  }
  .ldap-page .ldap-compact-card {
    flex-shrink: 0;
  }
  .system-config-grid { grid-template-columns: 1fr; }
  .card-header-row { align-items: flex-start; flex-direction: column; }
  .ldap-card-head { align-items: flex-start; flex-direction: column; }
  .ldap-card-title { align-items: flex-start; flex-wrap: wrap; }
  .ldap-schedule-summary { width: 100%; margin-left: 42px; }
  .ldap-card-actions { justify-content: flex-start; width: 100%; }
  .ldap-card-actions .el-button { flex: 1 1 140px; }
  .ldap-history-body { min-height: 300px; }
  .ldap-empty-state { min-height: 260px; }
  .ldap-config-actions,
  .drawer-footer { justify-content: flex-start; width: 100%; }
}
</style>
