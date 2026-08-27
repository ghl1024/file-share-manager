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
  <section class="files-page">
    <header class="page-toolbar files-toolbar">
      <div class="page-toolbar__group files-toolbar__filters">
        <span class="workspace-label">{{ workspaceName }}</span>
        <ModeSwitch v-model="viewMode" :options="viewOptions" @change="switchView" />
      </div>
      <div class="page-actions">
        <ActionButton v-if="viewMode !== 'trash'" action="batchDownload" :disabled="selectedNodes.length === 0" :loading="batchCreating" @click="createSelectedBatchDownload" />
        <el-dropdown trigger="click" :disabled="selectedNodes.length === 0 || batchActionLoading" @command="handleBatchNodeCommand">
          <ActionButton action="batchActions" :disabled="selectedNodes.length === 0" :loading="batchActionLoading" />
          <template #dropdown>
            <el-dropdown-menu>
              <el-dropdown-item v-if="viewMode === 'files'" command="move" :icon="Rank">批量移动</el-dropdown-item>
              <el-dropdown-item v-if="viewMode !== 'trash' && viewMode !== 'favorites'" command="favorite" :icon="Star">批量收藏</el-dropdown-item>
              <el-dropdown-item v-if="viewMode !== 'trash'" command="unfavorite" :icon="StarFilled">取消收藏</el-dropdown-item>
              <el-dropdown-item v-if="viewMode === 'files'" command="trash" :icon="Delete" divided>移入回收站</el-dropdown-item>
              <el-dropdown-item v-if="viewMode === 'trash'" command="restore" :icon="RefreshLeft">批量恢复</el-dropdown-item>
            </el-dropdown-menu>
          </template>
        </el-dropdown>
        <ActionButton action="tasks" @click="openBatchDownloads" />
        <ActionButton v-if="viewMode === 'files'" action="upload" @click="chooseNewFile" />
        <ActionButton v-if="viewMode === 'files'" action="folderAdd" :plain="false" @click="showCreateDialog = true" />
      </div>
      <input
        ref="fileInput"
        class="file-input"
        type="file"
        aria-label="选择上传文件"
        :multiple="fileSelectionMode === 'new'"
        @change="selectFile"
      />
    </header>

    <div v-if="secondaryView" class="favorites-search">
      <el-input
        v-model="keyword"
        class="file-search"
        clearable
        :prefix-icon="Search"
        :placeholder="secondarySearchPlaceholder"
        @keyup.enter="searchCurrent"
        @clear="searchCurrent"
      />
    </div>

    <div v-if="viewMode === 'files'" class="breadcrumb-bar">
      <el-breadcrumb separator="/">
        <el-breadcrumb-item v-for="(item, index) in breadcrumbs" :key="item.id || 'root'">
          <button type="button" class="breadcrumb-button" :disabled="index === breadcrumbs.length - 1" @click="goToBreadcrumb(index)">
            {{ item.name }}
          </button>
        </el-breadcrumb-item>
      </el-breadcrumb>
      <div class="breadcrumb-actions">
        <el-input
          v-model="keyword"
          class="file-search"
          clearable
          :prefix-icon="Search"
          placeholder="搜索文件或目录"
          @keyup.enter="searchCurrent"
          @clear="searchCurrent"
        />
		<ActionButton action="search" text="搜索" @click="searchCurrent" />
		<ActionButton action="filter" :text="searchFiltersVisible ? '收起筛选' : '高级筛选'" @click="searchFiltersVisible = !searchFiltersVisible" />
		<ActionButton action="refresh" :loading="loading" title="刷新目录" @click="loadCurrent" />
      </div>
    </div>

	<el-collapse-transition>
	  <div v-if="viewMode === 'files' && searchFiltersVisible" class="file-search-panel">
		<el-form :inline="true" class="file-search-form" @submit.prevent="searchCurrent">
		  <el-form-item label="类型">
			<el-select v-model="searchFilters.type" clearable placeholder="全部" class="search-filter-short">
			  <el-option label="文件" value="file" />
			  <el-option label="目录" value="folder" />
			</el-select>
		  </el-form-item>
		  <el-form-item label="扩展名">
			<el-input v-model.trim="searchFilters.extension" clearable placeholder="如 pdf" class="search-filter-short" :disabled="searchFilters.type === 'folder'" />
		  </el-form-item>
		  <el-form-item label="创建人">
			<el-input v-model.trim="searchFilters.createdBy" clearable placeholder="用户名或姓名前缀" class="search-filter-user" />
		  </el-form-item>
		  <el-form-item label="修改人">
			<el-input v-model.trim="searchFilters.updatedBy" clearable placeholder="用户名或姓名前缀" class="search-filter-user" />
		  </el-form-item>
		  <el-form-item label="时间">
			<el-select v-model="searchFilters.timeField" class="search-filter-time-kind">
			  <el-option label="最近修改" value="updated" />
			  <el-option label="创建时间" value="created" />
			</el-select>
			<el-date-picker v-model="searchFilters.timeRange" type="daterange" value-format="YYYY-MM-DD" range-separator="至" start-placeholder="开始日期" end-placeholder="结束日期" class="search-filter-date" />
		  </el-form-item>
		  <el-form-item label="大小 (MB)">
			<div class="search-size-range">
			  <el-input-number v-model="searchFilters.minSizeMB" :min="0" :precision="2" :controls="false" placeholder="最小" :disabled="searchFilters.type === 'folder'" />
			  <span>至</span>
			  <el-input-number v-model="searchFilters.maxSizeMB" :min="0" :precision="2" :controls="false" placeholder="最大" :disabled="searchFilters.type === 'folder'" />
			</div>
		  </el-form-item>
		  <el-form-item label="排序">
			<el-select v-model="searchFilters.sort" class="search-filter-sort">
			  <el-option label="相关度" value="relevance" />
			  <el-option label="最近修改" value="updated_desc" />
			  <el-option label="最近创建" value="created_desc" />
			  <el-option label="名称" value="name_asc" />
			  <el-option label="大小从小到大" value="size_asc" />
			  <el-option label="大小从大到小" value="size_desc" />
			</el-select>
		  </el-form-item>
		  <div class="search-filter-actions">
			<ActionButton action="search" text="应用筛选" @click="searchCurrent" />
			<ActionButton action="refresh" text="清空" @click="clearSearchFilters" />
		  </div>
		</el-form>
	  </div>
	</el-collapse-transition>

	<div v-if="viewMode === 'files' && isSearchActive" class="search-result-summary">
	  <span>全空间搜索结果</span>
	  <strong>{{ total }}</strong>
	</div>

    <section v-if="uploadTasks.length" class="upload-queue" aria-label="上传队列">
      <header class="upload-queue__header">
        <div>
          <strong>上传队列</strong>
          <span class="muted">{{ uploadQueueSummary }} · 最多 2 个任务并发</span>
        </div>
        <ActionButton v-if="hasFinishedUploadTasks" action="clearCompleted" @click="clearFinishedUploads" />
      </header>
      <div class="upload-queue__list">
        <article v-for="task in uploadTasks" :key="task.clientId" class="upload-task">
          <div class="upload-task__identity">
            <span class="upload-task__name" :title="task.name">{{ task.name }}</span>
            <span class="muted">{{ formatBytes(task.size) }} · {{ task.statusText }}</span>
          </div>
          <el-progress
            class="upload-task__progress"
            :percentage="task.progress"
            :status="task.status === 'error' ? 'exception' : (task.status === 'completed' ? 'success' : undefined)"
          />
          <div class="upload-task__actions">
            <ActionButton v-if="task.status === 'uploading'" action="pauseUpload" :disabled="task.pauseRequested" @click="pauseUploadTask(task)" />
            <ActionButton v-else-if="task.status === 'paused'" action="continueUpload" @click="continueUploadTask(task)" />
            <ActionButton v-else-if="task.status === 'waiting_file'" action="resumeUpload" @click="chooseResumeFile(task)" />
            <ActionButton v-else-if="task.status === 'error'" action="retryUpload" @click="retryUploadTask(task)" />
            <ActionButton v-if="canCancelUploadTask(task)" action="cancelUpload" @click="cancelUploadTask(task)" />
          </div>
        </article>
      </div>
    </section>

    <div class="files-list-panel content-panel">
      <div
        class="files-drop-zone"
        :class="{ 'is-dragging': dragging }"
        @dragenter.prevent="handleDragEnter"
        @dragover.prevent
        @dragleave.prevent="handleDragLeave"
        @drop.prevent="handleDrop"
      >
        <div v-if="dragging" class="drop-overlay"><el-icon :size="24"><Upload /></el-icon><span>释放文件以上传</span></div>
        <el-table
		ref="filesTableRef"
        :data="nodes"
        v-loading="loading"
        class="files-table"
        row-key="id"
        :row-class-name="nodeRowClassName"
        @row-dblclick="openNode"
		@selection-change="handleNodeSelection"
      >
	  <el-table-column type="selection" width="48" :selectable="isNodeSelectable" />
      <el-table-column label="名称" min-width="300">
        <template #default="scope">
		  <div class="node-name-cell">
			<button v-if="scope.row.type === 'folder' && viewMode !== 'trash'" type="button" class="node-name" @click="openNode(scope.row)">
			  <el-icon><Folder /></el-icon>
			  <span>{{ scope.row.name }}</span>
			</button>
			<button v-else-if="scope.row.type === 'file' && viewMode !== 'trash'" type="button" class="node-name" @click="openVersions(scope.row)">
			  <el-icon><Document /></el-icon>{{ scope.row.name }}
			</button>
			<span v-else class="node-name is-file"><el-icon><Folder v-if="scope.row.type === 'folder'" /><Document v-else /></el-icon>{{ scope.row.name }}</span>
			<span v-if="isSearchActive" class="search-node-meta">创建：{{ scope.row.created_by_name || '-' }} · 修改：{{ scope.row.updated_by_name || '-' }}</span>
		  </div>
        </template>
      </el-table-column>
      <el-table-column label="类型" width="100">
        <template #default="scope">{{ scope.row.type === 'folder' ? '目录' : '文件' }}</template>
      </el-table-column>
	  <el-table-column v-if="viewMode === 'shared'" label="授权来源" min-width="220">
		<template #default="scope">
		  <div class="permission-source-list">
			<el-tag v-for="source in scope.row.permission_sources" :key="`${source.type}-${source.id}`" class="permission-source-tag" :title="permissionSourceLabel(source)" :type="source.type === 'group' ? 'success' : 'primary'" effect="plain">
			  {{ permissionSourceLabel(source) }}
			</el-tag>
		  </div>
		</template>
	  </el-table-column>
	  <el-table-column v-if="viewMode === 'shared' || viewMode === 'recent'" label="我的权限" width="110">
		<template #default="scope"><el-tag :type="accessLevelMeta(scope.row.effective_access_level).type" effect="plain">{{ accessLevelMeta(scope.row.effective_access_level).label }}</el-tag></template>
	  </el-table-column>
	  <el-table-column v-if="viewMode === 'recent'" label="最近访问" width="180">
		<template #default="scope">{{ formatDate(scope.row.recent_accessed_at) }}</template>
	  </el-table-column>
	  <el-table-column v-if="isSearchActive" label="大小" width="120">
		<template #default="scope">{{ scope.row.size == null ? '-' : formatBytes(scope.row.size) }}</template>
	  </el-table-column>
      <el-table-column prop="updated_at" label="最近修改" width="180">
        <template #default="scope">{{ formatDate(scope.row.updated_at) }}</template>
      </el-table-column>
      <el-table-column label="状态" width="120">
        <template #default="scope">
		  <el-tag v-if="viewMode === 'trash'" type="warning" effect="plain">回收站</el-tag>
          <el-tooltip v-else-if="scope.row.type === 'file' && scope.row.archive_error" :content="scope.row.archive_error" placement="top">
            <el-tag type="danger" effect="plain">归档失败</el-tag>
          </el-tooltip>
          <el-tag v-else-if="scope.row.type === 'file'" :type="storageClassMeta(scope.row.storage_class).type" effect="plain">{{ storageClassMeta(scope.row.storage_class).label }}</el-tag>
          <el-tag v-else :type="scope.row.status === 'active' ? 'success' : 'warning'" effect="plain">{{ scope.row.status === 'active' ? '正常' : '回收站' }}</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="操作" width="250" fixed="right" class-name="common-operation-column" header-class-name="common-operation-column">
        <template #default="scope">
          <div class="common-action-group">
            <template v-if="viewMode !== 'trash'">
              <ActionButton v-if="scope.row.type === 'file'" action="preview" @click="openPreview(scope.row)" />
              <el-tooltip v-if="scope.row.type === 'file'" :content="canDownloadStorage(scope.row.storage_class) ? '下载文件' : '文件需先在对象存储中解冻'" placement="top">
                <span><ActionButton action="download" :disabled="!canDownloadStorage(scope.row.storage_class)" @click="download(scope.row)" /></span>
              </el-tooltip>
              <el-dropdown trigger="click" @command="(command) => handleNodeCommand(command, scope.row)">
                <ActionButton action="more" title="更多操作" />
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item v-if="scope.row.type === 'file'" command="preview" :icon="View">在线预览</el-dropdown-item>
                    <el-dropdown-item v-if="scope.row.type === 'file'" command="versions" :icon="Clock">版本</el-dropdown-item>
					<el-dropdown-item command="collaboration" :icon="ChatDotRound">协作详情</el-dropdown-item>
                    <el-dropdown-item command="access" :icon="Lock">权限详情</el-dropdown-item>
                    <el-dropdown-item v-if="canModifyNode(scope.row)" command="share" :icon="Share">创建分享</el-dropdown-item>
                    <el-dropdown-item command="favorite" :icon="scope.row.is_favorite ? StarFilled : Star">{{ scope.row.is_favorite ? '取消收藏' : '收藏' }}</el-dropdown-item>
                    <el-dropdown-item v-if="scope.row.type === 'folder' && canManageNode(scope.row) && userStore.hasPermission('acl:manage')" command="acl" :icon="Lock">目录权限</el-dropdown-item>
                    <el-dropdown-item v-if="canModifyNode(scope.row)" command="rename" :icon="EditPen">重命名</el-dropdown-item>
                    <el-dropdown-item v-if="canModifyNode(scope.row)" command="move" :icon="Rank">移动</el-dropdown-item>
                    <el-dropdown-item v-if="canModifyNode(scope.row)" command="trash" :icon="Delete" divided>移入回收站</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </template>
            <ActionButton v-else action="restore" @click="restore(scope.row)" />
          </div>
        </template>
      </el-table-column>
      <template #empty>
        <el-empty :description="emptyDescription" />
      </template>
        </el-table>
      </div>
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
    </div>

    <el-dialog v-model="showCreateDialog" title="新建目录" width="min(420px, calc(100vw - 32px))" @closed="resetCreateForm">
      <el-form ref="formRef" :model="createForm" :rules="rules" label-position="top" @submit.prevent="submitCreate">
        <el-form-item label="目录名称" prop="name">
          <el-input v-model="createForm.name" maxlength="255" show-word-limit @keyup.enter="submitCreate" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreateDialog = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="renameDialogVisible" title="重命名" width="min(420px, calc(100vw - 32px))">
      <el-form label-position="top" @submit.prevent="submitRename">
        <el-form-item label="名称">
          <el-input v-model="renameName" maxlength="255" show-word-limit @keyup.enter="submitRename" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="renameDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="actionLoading" @click="submitRename">保存</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="moveDialogVisible" :title="batchMoveMode ? `移动 ${selectedNodes.length} 个项目` : '移动到'" width="min(480px, calc(100vw - 32px))">
      <el-tree-select
        v-model="moveParentId"
        :data="folderTree"
        node-key="value"
        check-strictly
        default-expand-all
        style="width: 100%"
      />
      <template #footer>
        <el-button @click="moveDialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="actionLoading || batchActionLoading" @click="submitMove">移动</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="batchResultDialogVisible" :title="`${batchOperationLabel}结果`" width="min(640px, calc(100vw - 32px))">
      <el-alert
        :title="`成功 ${batchResultSummary.success} 项，失败 ${batchResultSummary.failed} 项`"
        :type="batchResultSummary.failed ? 'warning' : 'success'"
        :closable="false"
        show-icon
      />
      <el-table :data="batchNodeResults" row-key="node_id" class="batch-result-table">
        <el-table-column prop="name" label="名称" min-width="220">
          <template #default="scope">{{ scope.row.name || `节点 #${scope.row.node_id}` }}</template>
        </el-table-column>
        <el-table-column label="结果" width="90">
          <template #default="scope"><el-tag :type="scope.row.success ? 'success' : 'danger'" effect="plain">{{ scope.row.success ? '成功' : '失败' }}</el-tag></template>
        </el-table-column>
        <el-table-column prop="message" label="说明" min-width="180" />
      </el-table>
    </el-dialog>

    <el-dialog
      v-model="previewDialogVisible"
      class="file-preview-dialog"
      width="min(1120px, calc(100vw - 32px))"
      :fullscreen="isMobile"
      destroy-on-close
      @closed="resetPreview"
    >
      <template #header>
        <div class="preview-dialog-header">
          <span class="preview-dialog-icon"><el-icon><View /></el-icon></span>
          <div>
            <strong>{{ previewNode?.name || '在线预览' }}</strong>
            <span v-if="previewMeta">版本 {{ previewMeta.version_no }} · {{ formatBytes(previewMeta.size) }}</span>
          </div>
        </div>
      </template>
      <div v-loading="previewLoading" class="preview-viewport" :aria-busy="previewLoading">
        <el-result v-if="previewError" icon="warning" title="无法在线预览" :sub-title="previewError" />
        <img v-else-if="previewMeta?.kind === 'image' && previewObjectURL" class="preview-image" :src="previewObjectURL" :alt="previewNode?.name || '文件预览'" />
        <div v-else-if="previewMeta?.kind === 'pdf'" class="preview-pdf">
          <div class="preview-pdf-toolbar" aria-label="PDF 预览工具栏">
            <div class="preview-pdf-controls">
              <el-tooltip content="上一页" placement="top">
                <el-button circle :icon="ArrowLeft" aria-label="上一页" :disabled="previewPdfPage <= 1 || previewPdfRendering" @click="changePreviewPDFPage(-1)" />
              </el-tooltip>
              <span class="preview-pdf-page">{{ previewPdfPage }} / {{ previewPdfPages || 1 }}</span>
              <el-tooltip content="下一页" placement="top">
                <el-button circle :icon="ArrowRight" aria-label="下一页" :disabled="previewPdfPage >= previewPdfPages || previewPdfRendering" @click="changePreviewPDFPage(1)" />
              </el-tooltip>
            </div>
            <div class="preview-pdf-controls">
              <el-tooltip content="缩小" placement="top">
                <el-button circle :icon="ZoomOut" aria-label="缩小" :disabled="previewPdfZoom <= 0.5 || previewPdfRendering" @click="changePreviewPDFZoom(-0.25)" />
              </el-tooltip>
              <span class="preview-pdf-zoom">{{ Math.round(previewPdfZoom * 100) }}%</span>
              <el-tooltip content="放大" placement="top">
                <el-button circle :icon="ZoomIn" aria-label="放大" :disabled="previewPdfZoom >= 2 || previewPdfRendering" @click="changePreviewPDFZoom(0.25)" />
              </el-tooltip>
            </div>
          </div>
          <div v-loading="previewPdfRendering" class="preview-pdf-stage">
            <canvas ref="previewPdfCanvas" class="preview-pdf-canvas" :aria-label="`${previewNode?.name || 'PDF'} 第 ${previewPdfPage} 页`" />
          </div>
        </div>
        <pre v-else-if="previewMeta?.kind === 'text' && !previewLoading" class="preview-text">{{ previewText }}</pre>
      </div>
      <template #footer>
        <el-button @click="previewDialogVisible = false">关闭</el-button>
        <ActionButton v-if="previewNode && !previewError" action="download" :plain="false" @click="download(previewNode, previewVersion)" />
      </template>
    </el-dialog>

    <el-dialog v-model="versionDialogVisible" :title="versionNode?.name || '文件版本'" width="min(980px, calc(100vw - 32px))">
      <el-table :data="versions" v-loading="versionLoading" row-key="id">
        <el-table-column prop="version_no" label="版本" width="90" />
        <el-table-column label="大小" width="130"><template #default="scope">{{ formatBytes(scope.row.size) }}</template></el-table-column>
        <el-table-column label="安全状态" width="150">
          <template #default="scope">
            <el-tag :type="scanStatusMeta(scope.row.scan_status).type" effect="plain">
              {{ scanStatusMeta(scope.row.scan_status).label }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="存储状态" width="120">
          <template #default="scope"><el-tag :type="storageClassMeta(scope.row.storage_class).type" effect="plain">{{ storageClassMeta(scope.row.storage_class).label }}</el-tag></template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="180"><template #default="scope">{{ formatDate(scope.row.created_at) }}</template></el-table-column>
        <el-table-column label="操作" width="320" fixed="right" class-name="common-operation-column" header-class-name="common-operation-column">
          <template #default="scope">
            <div class="common-action-group">
              <ActionButton action="preview" title="预览此版本" @click="openPreview(versionNode, scope.row.version_no)" />
              <el-tooltip :content="canDownloadStorage(scope.row.storage_class) ? '下载此版本' : '文件需先在对象存储中解冻'" placement="top">
                <span><ActionButton action="download" :disabled="!canDownloadStorage(scope.row.storage_class)" @click="download(versionNode, scope.row.version_no)" /></span>
              </el-tooltip>
              <ActionButton action="restore" title="恢复此版本" @click="restoreVersion(scope.row)" />
              <ActionButton
                v-if="canRescanVersion(scope.row)"
                action="rescan"
                title="重新扫描此版本"
                @click="rescanVersion(scope.row)"
              />
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-dialog>

    <el-dialog v-model="batchDialogVisible" title="下载任务" width="min(980px, calc(100vw - 32px))" @open="loadBatchDownloads" @closed="stopBatchPolling">
      <el-table :data="batchJobs" v-loading="batchLoading" row-key="id" class="batch-table">
        <el-table-column label="任务名称" :min-width="isMobile ? 170 : 190" show-overflow-tooltip>
          <template #default="scope">
            <span>{{ scope.row.name }}</span>
            <span v-if="isMobile" class="batch-mobile-meta">{{ scope.row.processed_files }}/{{ scope.row.total_files }} · {{ formatBytes(scope.row.total_bytes) }} · {{ batchProgress(scope.row) }}%</span>
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="进度" min-width="150">
          <template #default="scope">
            <el-progress :percentage="batchProgress(scope.row)" :status="scope.row.status === 'failed' ? 'exception' : (scope.row.status === 'completed' ? 'success' : undefined)" />
          </template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="文件" width="90">
          <template #default="scope">{{ scope.row.processed_files }} / {{ scope.row.total_files }}</template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="大小" width="110">
          <template #default="scope">{{ formatBytes(scope.row.total_bytes) }}</template>
        </el-table-column>
        <el-table-column label="状态" :width="isMobile ? 80 : 90">
          <template #default="scope"><el-tag :type="batchStatus(scope.row.status).type" effect="plain">{{ batchStatus(scope.row.status).label }}</el-tag></template>
        </el-table-column>
        <el-table-column v-if="!isMobile" label="有效期" width="165">
          <template #default="scope">{{ scope.row.expires_at ? formatDate(scope.row.expires_at) : '-' }}</template>
        </el-table-column>
        <el-table-column label="操作" :width="isMobile ? 96 : 130" fixed="right" align="center">
          <template #default="scope">
            <ActionButton v-if="scope.row.status === 'completed'" action="download" title="下载压缩包" @click="downloadBatch(scope.row)" />
            <ActionButton v-else-if="scope.row.status === 'failed'" action="retryDownload" :title="scope.row.error_message || '重试任务'" @click="retryBatch(scope.row)" />
            <span v-else class="action-placeholder">{{ batchActionText(scope.row.status) }}</span>
          </template>
        </el-table-column>
        <template #empty><el-empty description="暂无下载任务" /></template>
      </el-table>
      <el-pagination
        v-if="batchTotal > 0"
        class="table-pagination"
        :current-page="batchPage"
        :page-size="batchPageSize"
        :total="batchTotal"
        layout="total, prev, pager, next"
        @current-change="handleBatchPageChange"
      />
    </el-dialog>

    <el-dialog v-model="shareDialogVisible" :title="createdShare ? '分享已创建' : `创建“${shareNode?.name || ''}”的分享`" width="min(520px, calc(100vw - 32px))" @closed="resetShareDialog">
      <template v-if="createdShare">
        <el-alert title="分享链接仅展示这一次" description="关闭窗口后无法再次查看原链接，请先复制并妥善发送给接收人。" type="warning" :closable="false" show-icon />
        <div class="share-result-field"><span>分享链接</span><el-input :model-value="createdShare.url" readonly><template #append><el-button :icon="CopyDocument" :type="copiedShareField === 'url' ? 'success' : 'primary'" @click="copyShareValue(createdShare.url, 'url')">{{ copiedShareField === 'url' ? '已复制' : '复制链接' }}</el-button></template></el-input></div>
        <div class="share-result-field"><span>访问密码</span><el-input :model-value="shareForm.password || '未设置'" readonly><template #append><el-button :icon="CopyDocument" :disabled="!shareForm.password" :type="copiedShareField === 'password' ? 'success' : 'default'" @click="copyShareValue(shareForm.password, 'password')">{{ copiedShareField === 'password' ? '已复制' : '复制密码' }}</el-button></template></el-input></div>
      </template>
      <el-form v-else label-position="top">
        <el-form-item label="有效期" required><el-date-picker v-model="shareForm.expiresAt" type="datetime" :disabled-date="disablePastDate" style="width: 100%" /></el-form-item>
        <el-form-item label="访问密码"><el-input v-model="shareForm.password" type="password" show-password maxlength="128" placeholder="可选，至少 8 个字符" /></el-form-item>
        <el-form-item label="最大下载次数"><el-input-number v-model="shareForm.maxDownloads" :min="1" :max="1000000" :step="1" controls-position="right" placeholder="不限制" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="shareDialogVisible = false">{{ createdShare ? '关闭' : '取消' }}</el-button>
        <el-button v-if="!createdShare" type="primary" :loading="shareSubmitting" @click="submitShare">创建分享</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="aclDialogVisible" :title="aclNode ? `目录权限：${aclNode.name}` : '目录权限'" width="min(980px, calc(100vw - 32px))">
      <div v-loading="aclLoading" class="acl-dialog-body">
        <div class="acl-toolbar">
          <el-select v-model="aclForm.subject_type" aria-label="授权主体类型" @change="aclForm.subject_id = null">
            <el-option label="用户" value="user" />
            <el-option label="用户组" value="group" />
          </el-select>
          <el-tree-select
            v-if="aclForm.subject_type === 'group'"
            v-model="aclForm.subject_id"
            :data="aclGroupTree"
            node-key="value"
            filterable
            check-strictly
            :default-expanded-keys="['group-root:ldap', 'group-root:local']"
            aria-label="授权主体"
            placeholder="按部门检索用户组"
          />
          <el-select v-else v-model="aclForm.subject_id" filterable aria-label="授权主体" placeholder="选择工作空间成员">
            <el-option v-for="subject in aclSubjects" :key="subject.id" :label="subject.label" :value="subject.id" />
          </el-select>
          <el-select v-model="aclForm.effect" aria-label="授权效果">
            <el-option label="允许" value="allow" />
            <el-option label="拒绝" value="deny" />
          </el-select>
          <el-select v-model="aclForm.access_level" aria-label="访问级别">
            <el-option label="读取" value="read" />
            <el-option label="读写" value="read_write" />
            <el-option label="管理员" value="admin" />
          </el-select>
          <el-switch v-model="aclForm.inherit_to_children" inline-prompt active-text="继承" inactive-text="本级" />
          <ActionButton action="add" text="添加" :plain="false" :disabled="!aclForm.subject_id" @click="addACL" />
        </div>

        <el-table :data="aclEntries" stripe border row-key="subject_key" class="acl-table">
          <el-table-column label="授权主体" min-width="220">
            <template #default="scope">{{ subjectLabel(scope.row) }}</template>
          </el-table-column>
          <el-table-column label="主体类型" width="110">
            <template #default="scope">{{ aclSubjectTypeLabel(scope.row) }}</template>
          </el-table-column>
          <el-table-column label="效果" width="100">
            <template #default="scope"><el-tag :type="scope.row.effect === 'allow' ? 'success' : 'danger'" effect="plain">{{ scope.row.effect === 'allow' ? '允许' : '拒绝' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="访问级别" width="120">
            <template #default="scope">{{ accessLevelLabel(scope.row.access_level) }}</template>
          </el-table-column>
          <el-table-column label="继承" width="100">
            <template #default="scope"><el-tag :type="scope.row.inherit_to_children ? 'info' : 'warning'" effect="plain">{{ scope.row.inherit_to_children ? '向下继承' : '仅本级' }}</el-tag></template>
          </el-table-column>
          <el-table-column label="操作" width="100" fixed="right">
            <template #default="scope"><ActionButton action="delete" title="删除授权" @click="removeACL(scope.$index)" /></template>
          </el-table-column>
          <template #empty><el-empty description="暂无直接授权，请添加一名管理员" /></template>
        </el-table>

        <div class="acl-inheritance-row">
          <div>
            <strong>目录继承</strong>
            <span class="muted">控制上级目录的授权是否继续生效</span>
          </div>
          <el-radio-group v-model="inheritanceMode">
            <el-radio value="inherit">继承上级权限</el-radio>
            <el-radio value="break">中断继承</el-radio>
          </el-radio-group>
        </div>
      </div>
      <template #footer>
        <el-button @click="aclDialogVisible = false">取消</el-button>
        <ActionButton action="save" text="保存权限" :plain="false" :loading="aclSaving" @click="saveACL" />
      </template>
    </el-dialog>

	<el-drawer
	  v-model="collaborationVisible"
	  class="collaboration-drawer"
	  :size="isMobile ? '100%' : '620px'"
	  append-to-body
	  destroy-on-close
	  @closed="resetCollaboration"
	>
	  <template #header>
		<div class="collaboration-header">
		  <div class="collaboration-header__icon"><el-icon><ChatDotRound /></el-icon></div>
		  <div>
			<h2>协作详情</h2>
			<span>{{ collaborationNode?.name || '文件或目录' }}</span>
		  </div>
		</div>
	  </template>

	  <el-tabs v-model="collaborationTab" class="collaboration-tabs" @tab-change="handleCollaborationTabChange">
		<el-tab-pane name="comments">
		  <template #label><span>评论 <strong>{{ collaborationCommentTotal }}</strong></span></template>
		  <div class="collaboration-pane">
			<section class="comment-composer" aria-label="发表评论">
			  <el-mention
				v-model="commentDraft"
				:options="mentionOptions"
				:loading="mentionLoading"
				:filter-option="false"
				popper-class="collaboration-mention-popper"
				:popper-style="{ maxWidth: 'calc(100vw - 28px)' }"
				show-arrow
				type="textarea"
				:autosize="{ minRows: 3, maxRows: 6 }"
				maxlength="2000"
				show-word-limit
				whole
				placeholder="写下评论，输入 @ 提及有权访问此项目的成员"
				@search="searchMentionCandidates"
			  />
			  <div class="comment-composer__footer">
				<span>提及通知会遵循成员的站内通知偏好</span>
				<ActionButton action="comment" text="发表评论" :plain="false" :loading="commentSubmitting" :disabled="!commentDraft.trim()" @click="submitComment" />
			  </div>
			</section>

			<div v-loading="commentsLoading" class="comment-feed">
			  <article v-for="comment in collaborationComments" :key="comment.id" class="comment-item">
				<el-avatar :size="36" class="comment-avatar">{{ avatarInitial(comment.author_display_name || comment.author_username) }}</el-avatar>
				<div class="comment-item__body">
				  <header class="comment-item__header">
					<div>
					  <strong>{{ comment.author_display_name || comment.author_username }}</strong>
					  <span>@{{ comment.author_username }}</span>
					</div>
					<span :title="formatDate(comment.created_at)">{{ relativeTime(comment.created_at) }}<template v-if="isCommentEdited(comment)"> · 已编辑</template></span>
				  </header>

				  <template v-if="editingCommentId === comment.id">
					<el-mention
					  v-model="editingCommentDraft"
					  :options="mentionOptions"
					  :loading="mentionLoading"
					  :filter-option="false"
					  popper-class="collaboration-mention-popper"
					  :popper-style="{ maxWidth: 'calc(100vw - 28px)' }"
					  show-arrow
					  type="textarea"
					  :autosize="{ minRows: 3, maxRows: 6 }"
					  maxlength="2000"
					  show-word-limit
					  whole
					  @search="searchMentionCandidates"
					/>
					<div class="comment-edit-actions">
					  <el-button size="small" @click="cancelEditComment">取消</el-button>
					  <ActionButton action="save" text="保存" :plain="false" :loading="commentUpdating" :disabled="!editingCommentDraft.trim()" @click="saveComment(comment)" />
					</div>
				  </template>
				  <template v-else>
					<p class="comment-content"><template v-for="(segment, index) in commentSegments(comment)" :key="index"><span :class="{ 'is-mention': segment.mention }">{{ segment.text }}</span></template></p>
					<div v-if="comment.can_edit || comment.can_delete" class="comment-actions common-action-group">
					  <ActionButton v-if="comment.can_edit" action="edit" text="编辑" @click="startEditComment(comment)" />
					  <ActionButton v-if="comment.can_delete" action="delete" text="删除" @click="removeComment(comment)" />
					</div>
				  </template>
				</div>
			  </article>
			  <el-empty v-if="!commentsLoading && !collaborationComments.length" description="暂无评论，开始第一次讨论" :image-size="88" />
			</div>
			<div v-if="collaborationComments.length < collaborationCommentTotal" class="collaboration-load-more">
			  <ActionButton action="more" text="加载更多评论" :loading="commentsLoading" @click="loadComments(false)" />
			</div>
		  </div>
		</el-tab-pane>

		<el-tab-pane name="activity">
		  <template #label><span>活动 <strong>{{ collaborationActivityTotal }}</strong></span></template>
		  <div v-loading="activityLoading" class="collaboration-pane collaboration-activity-pane">
			<el-timeline v-if="collaborationActivities.length" class="activity-timeline">
			  <el-timeline-item v-for="activity in collaborationActivities" :key="activity.id" :timestamp="formatDate(activity.occurred_at)" placement="top" type="primary" hollow>
				<div class="activity-item">
				  <el-avatar :size="32">{{ avatarInitial(activity.actor_display_name || activity.actor_username) }}</el-avatar>
				  <div>
					<strong>{{ activity.actor_display_name || activity.actor_username }}</strong>
					<span>{{ activity.summary }}</span>
				  </div>
				</div>
			  </el-timeline-item>
			</el-timeline>
			<el-empty v-else-if="!activityLoading" description="暂无可展示的协作活动" :image-size="88" />
			<div v-if="collaborationActivities.length < collaborationActivityTotal" class="collaboration-load-more">
			  <ActionButton action="more" text="加载更多活动" :loading="activityLoading" @click="loadActivity(false)" />
			</div>
		  </div>
		</el-tab-pane>
	  </el-tabs>
	</el-drawer>

    <el-drawer
      v-model="accessDetailVisible"
      class="access-detail-drawer"
      :size="isMobile ? '100%' : '540px'"
      append-to-body
      destroy-on-close
    >
      <template #header>
        <div class="access-detail-header">
          <div class="access-detail-icon"><el-icon><Lock /></el-icon></div>
          <div>
            <h2>访问权限</h2>
            <span>{{ accessDetail?.node?.name || '文件或目录' }}</span>
          </div>
        </div>
      </template>
      <div v-loading="accessDetailLoading" class="access-detail-body">
        <template v-if="accessDetail?.access">
          <div class="access-level-summary">
            <span>我的有效权限</span>
            <el-tag :type="accessLevelMeta(accessDetail.access.effective_access_level).type" effect="plain" size="large">
              {{ accessLevelMeta(accessDetail.access.effective_access_level).label }}
            </el-tag>
          </div>
          <el-descriptions :column="1" border>
            <el-descriptions-item label="对象类型">{{ accessDetail.node.type === 'folder' ? '目录' : '文件' }}</el-descriptions-item>
            <el-descriptions-item label="最近修改">{{ formatDate(accessDetail.node.updated_at) }}</el-descriptions-item>
          </el-descriptions>
          <section class="access-source-section">
            <div class="access-source-title">
              <h3>权限来源</h3>
              <span>{{ accessDetail.access.sources?.length || 0 }} 项生效授权</span>
            </div>
            <div v-for="source in accessDetail.access.sources" :key="`${source.type}-${source.id || source.name}-${source.source_node_id || 0}`" class="access-source-row">
              <div class="access-source-main">
                <strong>{{ accessSourceName(source) }}</strong>
                <span v-if="accessSourcePath(source)">{{ accessSourcePath(source) }}</span>
              </div>
              <div class="access-source-tags">
                <el-tag :type="accessSourceType(source).type" effect="plain">{{ accessSourceType(source).label }}</el-tag>
                <el-tag :type="source.inherited ? 'warning' : 'info'" effect="plain">{{ source.inherited ? '继承' : '直接' }}</el-tag>
                <el-tag :type="accessLevelMeta(source.granted_level).type" effect="plain">{{ accessLevelMeta(source.granted_level).label }}</el-tag>
              </div>
              <span class="access-source-scope">{{ accessSourceScope(source) }}</span>
            </div>
            <el-empty v-if="!accessDetail.access.sources?.length" description="暂无可展示的权限来源" :image-size="72" />
          </section>
        </template>
      </div>
    </el-drawer>
  </section>
</template>

<script setup>
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft, ArrowRight, ChatDotRound, Clock, CopyDocument, Delete, Document, Download, EditPen, Folder, Lock, Plus, Rank, RefreshLeft, Search, Share, Star, StarFilled, Upload, View, ZoomIn, ZoomOut } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import pdfWorkerURL from 'pdfjs-dist/build/pdf.worker.min.mjs?url'
import {
  batchMoveNodes, batchRestoreNodes, batchSetFavorite, batchTrashNodes, cancelUpload, completeUpload, createBatchDownload, createFolder, createNodeComment, createShare, deleteNodeComment, downloadBatchArchive, downloadFile, getFilePreview, getFilePreviewContent, getNodeDetail, getShareDetail, getUploadStatus, initUpload,
  listBatchDownloads,
  listChildren, listFavorites, listFileVersions, listFolderACL, listFolderTree, listNodeActivity, listNodeComments, listNodeMentionCandidates, listRecentNodes, listRoots, listSharedWithMe, listTrash, moveNode,
  renameNode, replaceFolderACL, rescanFileVersion, restoreFileVersion, restoreNode, retryBatchDownload, searchNodes, setFavorite, setFolderInheritance, trashNode, updateNodeComment, uploadPart
} from '@/api/files'
import { getWorkspaceGroupDirectory, getWorkspaceMembers } from '@/api/workspace'
import { useUserStore } from '@/stores/user'
import { usePagination } from '@/composables/usePagination'
import { copyText } from '@/utils/copy'
import ActionButton from '@/components/common/ActionButton.vue'
import ModeSwitch from '@/components/common/ModeSwitch.vue'

let pdfLibraryPromise = null

function loadPDFLibrary() {
  if (!pdfLibraryPromise) {
    pdfLibraryPromise = import('pdfjs-dist').then((library) => {
      library.GlobalWorkerOptions.workerSrc = pdfWorkerURL
      return library
    })
  }
  return pdfLibraryPromise
}

const router = useRouter()
const route = useRoute()
const userStore = useUserStore()
const creating = ref(false)
const showCreateDialog = ref(false)
const formRef = ref(null)
const fileInput = ref(null)
const filesTableRef = ref(null)
const selectedNodes = ref([])
const fileSelectionMode = ref('new')
const resumeUploadTaskId = ref('')
const uploadTasks = ref([])
const activeUploadCount = ref(0)
const requestedView = typeof route.query.view === 'string' ? route.query.view : ''
const viewMode = ref(['files', 'shared', 'recent', 'favorites', 'trash'].includes(requestedView) ? requestedView : 'files')
const viewOptions = [
  { label: '文件', value: 'files', action: 'files', icon: Folder },
  { label: '与我共享', value: 'shared', action: 'shared', icon: Share },
  { label: '最近使用', value: 'recent', action: 'recent', icon: Clock },
  { label: '收藏', value: 'favorites', action: 'favorites', icon: Star },
  { label: '回收站', value: 'trash', action: 'trash', icon: Delete }
]
const renameDialogVisible = ref(false)
const renameName = ref('')
const selectedNode = ref(null)
const moveDialogVisible = ref(false)
const moveParentId = ref(0)
const batchMoveMode = ref(false)
const folderTree = ref([])
const actionLoading = ref(false)
const batchActionLoading = ref(false)
const batchResultDialogVisible = ref(false)
const batchOperationLabel = ref('批量操作')
const batchNodeResults = ref([])
const versionDialogVisible = ref(false)
const versionNode = ref(null)
const versions = ref([])
const versionLoading = ref(false)
const previewDialogVisible = ref(false)
const previewLoading = ref(false)
const previewNode = ref(null)
const previewVersion = ref(null)
const previewMeta = ref(null)
const previewText = ref('')
const previewObjectURL = ref('')
const previewError = ref('')
const previewPdfCanvas = ref(null)
const previewPdfPage = ref(1)
const previewPdfPages = ref(0)
const previewPdfZoom = ref(1)
const previewPdfRendering = ref(false)
let previewPdfLoadingTask = null
let previewPdfDocument = null
let previewPdfRenderTask = null
let previewRequestID = 0
const dragging = ref(false)
const shareDialogVisible = ref(false)
const shareNode = ref(null)
const shareSubmitting = ref(false)
const createdShare = ref(null)
const copiedShareField = ref('')
const shareForm = reactive({ expiresAt: '', password: '', maxDownloads: null })
const batchCreating = ref(false)
const batchDialogVisible = ref(false)
const batchLoading = ref(false)
const batchJobs = ref([])
const batchPage = ref(1)
const batchPageSize = 10
const batchTotal = ref(0)
const mobileMediaQuery = window.matchMedia('(max-width: 640px)')
const isMobile = ref(mobileMediaQuery.matches)
let batchPollTimer = null
const aclDialogVisible = ref(false)
const aclLoading = ref(false)
const aclSaving = ref(false)
const aclNode = ref(null)
const aclEntries = ref([])
const workspaceMembers = ref([])
const workspaceGroups = ref([])
const inheritanceMode = ref('inherit')
const aclForm = reactive({ subject_type: 'user', subject_id: null, effect: 'allow', access_level: 'read', inherit_to_children: true })
const accessDetailVisible = ref(false)
const accessDetailLoading = ref(false)
const accessDetail = ref(null)
const collaborationVisible = ref(false)
const collaborationNode = ref(null)
const collaborationTab = ref('comments')
const collaborationComments = ref([])
const collaborationCommentTotal = ref(0)
const collaborationCommentPage = ref(1)
const commentsLoading = ref(false)
const collaborationActivities = ref([])
const collaborationActivityTotal = ref(0)
const collaborationActivityPage = ref(1)
const activityLoading = ref(false)
const commentDraft = ref('')
const commentSubmitting = ref(false)
const editingCommentId = ref(null)
const editingCommentDraft = ref('')
const commentUpdating = ref(false)
const mentionOptions = ref([])
const mentionLoading = ref(false)
let mentionSearchTimer = null
let mentionSearchSequence = 0
const breadcrumbs = ref([{ id: null, name: '根目录' }])
const locatedNodeId = ref(null)
const createForm = reactive({ name: '' })
const rules = { name: [{ required: true, message: '请输入目录名称', trigger: 'blur' }] }
const searchFiltersVisible = ref(false)
const searchFilters = reactive({
	type: '', extension: '', createdBy: '', updatedBy: '', timeField: 'updated',
	timeRange: [], minSizeMB: null, maxSizeMB: null, sort: 'relevance'
})

const workspaceName = computed(() => userStore.workspaces.find((item) => item.id === userStore.currentWorkspaceId)?.name || '未选择工作空间')
const currentParentId = computed(() => breadcrumbs.value[breadcrumbs.value.length - 1].id)
const uploadQueueSummary = computed(() => {
  const active = uploadTasks.value.filter((task) => ['initializing', 'uploading'].includes(task.status)).length
  const waiting = uploadTasks.value.filter((task) => ['queued', 'paused', 'waiting_file'].includes(task.status)).length
  const failed = uploadTasks.value.filter((task) => task.status === 'error').length
  return [`${active} 个进行中`, waiting ? `${waiting} 个等待` : '', failed ? `${failed} 个失败` : ''].filter(Boolean).join('，')
})
const hasFinishedUploadTasks = computed(() => uploadTasks.value.some((task) => ['completed', 'cancelled'].includes(task.status)))
const batchResultSummary = computed(() => batchNodeResults.value.reduce((summary, item) => {
  if (item.success) summary.success += 1
  else summary.failed += 1
  return summary
}, { success: 0, failed: 0 }))
const secondaryView = computed(() => ['favorites', 'shared', 'recent'].includes(viewMode.value))
const secondarySearchPlaceholder = computed(() => ({
  favorites: '搜索收藏', shared: '搜索共享名称或授权来源', recent: '搜索最近使用'
}[viewMode.value] || '搜索'))
const emptyDescription = computed(() => {
  if (keyword.value.trim() && secondaryView.value) return '未找到符合条件的文件或目录'
  return {
    favorites: '暂无收藏', shared: '暂无直接分享给你的文件或目录', recent: '暂无最近使用记录', trash: '回收站为空'
  }[viewMode.value] || (isSearchActive.value ? '未找到符合条件的文件或目录' : '当前目录为空')
})
const aclSubjects = computed(() => {
  return workspaceMembers.value
    .filter((member) => member.status === undefined || member.status === 1)
    .map((member) => ({
      id: member.user_id,
      label: `${member.real_name || member.username} (${member.username})`
    }))
})
const aclGroupTree = computed(() => buildGroupDirectoryTree(workspaceGroups.value))

const fetchNodes = (params) => {
  if (viewMode.value === 'trash') return listTrash(params)
  if (viewMode.value === 'favorites') return listFavorites(params)
  if (viewMode.value === 'shared') return listSharedWithMe(params)
  if (viewMode.value === 'recent') return listRecentNodes(params)
  if (params.keyword || hasAdvancedSearchCriteria()) return searchNodes({ ...params, ...searchFilterParams() })
  return currentParentId.value === null ? listRoots(params) : listChildren(currentParentId.value, params)
}
const {
  list: nodes, loading, page, pageSize, total, keyword, load: loadNodes,
  handleSearch: loadBySearch, handleSizeChange: loadBySize
} = usePagination(fetchNodes, { pageSize: 20 })
const isSearchActive = computed(() => viewMode.value === 'files' && (!!keyword.value.trim() || hasAdvancedSearchCriteria()))

function hasAdvancedSearchCriteria() {
	return !!(searchFilters.type || searchFilters.extension.trim() || searchFilters.createdBy.trim() || searchFilters.updatedBy.trim() ||
		searchFilters.timeRange?.length === 2 || searchFilters.minSizeMB != null || searchFilters.maxSizeMB != null)
}

function searchFilterParams() {
	const params = { sort: searchFilters.sort }
	if (searchFilters.type) params.type = searchFilters.type
	if (searchFilters.type !== 'folder' && searchFilters.extension.trim()) params.extension = searchFilters.extension.trim()
	if (searchFilters.createdBy.trim()) params.created_by = searchFilters.createdBy.trim()
	if (searchFilters.updatedBy.trim()) params.updated_by = searchFilters.updatedBy.trim()
	if (searchFilters.timeRange?.length === 2) {
		params[`${searchFilters.timeField}_from`] = searchFilters.timeRange[0]
		params[`${searchFilters.timeField}_to`] = searchFilters.timeRange[1]
	}
	if (searchFilters.type !== 'folder' && searchFilters.minSizeMB != null) params.min_size = Math.round(searchFilters.minSizeMB * 1024 * 1024)
	if (searchFilters.type !== 'folder' && searchFilters.maxSizeMB != null) params.max_size = Math.round(searchFilters.maxSizeMB * 1024 * 1024)
	return params
}

function resetSearchFilters() {
	Object.assign(searchFilters, {
		type: '', extension: '', createdBy: '', updatedBy: '', timeField: 'updated',
		timeRange: [], minSizeMB: null, maxSizeMB: null, sort: 'relevance'
	})
}

async function loadCurrent() {
  if (!userStore.currentWorkspaceId) {
    await router.replace('/workspaces')
    return
  }
  try {
    await loadNodes()
	selectedNodes.value = []
	filesTableRef.value?.clearSelection()
  } catch (error) {
    nodes.value = []
  }
}

async function openNode(node) {
  if (viewMode.value === 'trash' || node.type !== 'folder') return
  const openedFromSearch = isSearchActive.value
  if (secondaryView.value) {
    viewMode.value = 'files'
    breadcrumbs.value = [{ id: null, name: '根目录' }, { id: node.id, name: node.name }]
    keyword.value = ''
    page.value = 1
    await loadCurrent()
    return
  }
  keyword.value = ''
	if (openedFromSearch) {
		resetSearchFilters()
		breadcrumbs.value = [{ id: null, name: '根目录' }]
	}
  breadcrumbs.value.push({ id: node.id, name: node.name })
  page.value = 1
  await loadCurrent()
}

function accessLevelMeta(level) {
  return {
    read: { label: '只读', type: 'info', rank: 1 },
    read_write: { label: '可编辑', type: 'warning', rank: 2 },
    admin: { label: '可管理', type: 'success', rank: 3 }
  }[level] || { label: '-', type: 'info', rank: 0 }
}

function canModifyNode(node) {
  if (!['shared', 'recent'].includes(viewMode.value)) return true
  return accessLevelMeta(node.effective_access_level).rank >= 2
}

function canManageNode(node) {
  if (!['shared', 'recent'].includes(viewMode.value)) return true
  return accessLevelMeta(node.effective_access_level).rank >= 3
}

async function goToBreadcrumb(index) {
  if (index === breadcrumbs.value.length - 1) return
  breadcrumbs.value = breadcrumbs.value.slice(0, index + 1)
  page.value = 1
  await loadCurrent()
}

function handlePageChange(value) {
  page.value = value
  loadNodes()
}

function nodeRowClassName({ row }) {
  return locatedNodeId.value === row.id ? 'located-source-row' : ''
}

function handleSizeChange(value) {
  pageSize.value = value
  loadBySize()
}

function searchCurrent() {
  loadBySearch()
}

function clearSearchFilters() {
	keyword.value = ''
	resetSearchFilters()
	loadBySearch()
}

function switchView() {
  keyword.value = ''
	resetSearchFilters()
	searchFiltersVisible.value = false
  page.value = 1
  dragging.value = false
	selectedNodes.value = []
  loadCurrent()
}

function isNodeSelectable(node) {
  return viewMode.value === 'trash' ? node.status === 'trashed' : node.status === 'active'
}

function handleNodeSelection(selection) {
  selectedNodes.value = selection
}

function selectedNodeIDs() {
  return selectedNodes.value.map((node) => node.id)
}

async function handleBatchNodeCommand(command) {
  if (!selectedNodes.value.length) return
  if (command === 'move') {
    await openBatchMove()
    return
  }
  if (command === 'favorite') {
    await executeBatchNodeOperation('批量收藏', batchSetFavorite, { node_ids: selectedNodeIDs(), favorite: true })
    return
  }
  if (command === 'unfavorite') {
    await executeBatchNodeOperation('取消收藏', batchSetFavorite, { node_ids: selectedNodeIDs(), favorite: false })
    return
  }
  if (command === 'trash') {
    try {
      await ElMessageBox.confirm(`确定将选中的 ${selectedNodes.value.length} 个项目移入回收站？每个项目都会独立校验权限。`, '批量移入回收站', {
        type: 'warning', confirmButtonText: '确认移入回收站'
      })
    } catch (error) {
      if (error !== 'cancel') console.error(error)
      return
    }
    await executeBatchNodeOperation('批量移入回收站', batchTrashNodes, { node_ids: selectedNodeIDs(), confirm: true })
    return
  }
  if (command === 'restore') {
    try {
      await ElMessageBox.confirm(`确定恢复选中的 ${selectedNodes.value.length} 个项目？名称冲突的项目会保留在回收站。`, '批量恢复', {
        type: 'warning', confirmButtonText: '确认恢复'
      })
    } catch (error) {
      if (error !== 'cancel') console.error(error)
      return
    }
    await executeBatchNodeOperation('批量恢复', batchRestoreNodes, { node_ids: selectedNodeIDs(), confirm: true })
  }
}

async function executeBatchNodeOperation(label, requestFn, payload) {
  batchActionLoading.value = true
  try {
    const result = await requestFn(payload)
    const data = result.data || {}
    batchOperationLabel.value = label
    batchNodeResults.value = data.results || []
    batchResultDialogVisible.value = true
    if (data.failed_count) ElMessage.warning(`${label}完成：成功 ${data.success_count || 0} 项，失败 ${data.failed_count} 项`)
    else ElMessage.success(`${label}成功，共 ${data.success_count || 0} 项`)
    await loadCurrent()
    return data
  } finally {
    batchActionLoading.value = false
  }
}

async function createSelectedBatchDownload() {
  if (!selectedNodes.value.length) return
  batchCreating.value = true
  try {
    const result = await createBatchDownload({ node_ids: selectedNodes.value.map((node) => node.id) })
    ElMessage.success('批量下载任务已创建')
    selectedNodes.value = []
    filesTableRef.value?.clearSelection()
    batchDialogVisible.value = true
    batchPage.value = 1
    await loadBatchDownloads()
    startBatchPolling()
    return result.data
  } finally {
    batchCreating.value = false
  }
}

function openBatchDownloads() {
  batchPage.value = 1
  batchDialogVisible.value = true
}

async function loadBatchDownloads(options = {}) {
  batchLoading.value = !options.silent
  try {
    const result = await listBatchDownloads({ page: batchPage.value, page_size: batchPageSize }, { silent: options.silent })
    batchJobs.value = result.data?.list || []
    batchTotal.value = result.data?.total || 0
    if (batchJobs.value.some((job) => ['queued', 'running'].includes(job.status))) startBatchPolling()
    else stopBatchPolling()
  } finally {
    batchLoading.value = false
  }
}

function startBatchPolling() {
  stopBatchPolling()
  if (!batchDialogVisible.value) return
  batchPollTimer = window.setTimeout(async () => {
    try {
      await loadBatchDownloads({ silent: true })
    } catch {
      stopBatchPolling()
    }
  }, 2000)
}

function stopBatchPolling() {
  if (batchPollTimer) window.clearTimeout(batchPollTimer)
  batchPollTimer = null
}

function updateMobileBreakpoint(event) {
  isMobile.value = event.matches
}

function handleBatchPageChange(value) {
  batchPage.value = value
  loadBatchDownloads()
}

function batchProgress(job) {
  if (job.status === 'completed') return 100
  if (!job.total_bytes) return 0
  return Math.min(100, Math.round((job.processed_bytes / job.total_bytes) * 100))
}

function batchStatus(status) {
  return {
    queued: { label: '排队中', type: 'info' },
    running: { label: '生成中', type: 'primary' },
    completed: { label: '可下载', type: 'success' },
    failed: { label: '失败', type: 'danger' },
    expired: { label: '已过期', type: 'info' }
  }[status] || { label: status, type: 'info' }
}

function batchActionText(status) {
  return {
    queued: '等待生成',
    running: '生成中',
    expired: '已失效'
  }[status] || '不可用'
}

async function downloadBatch(job) {
  try {
    const result = await downloadBatchArchive(job.id)
    const url = URL.createObjectURL(result.data)
    const link = document.createElement('a')
    link.href = url
    link.download = job.name
    link.click()
    URL.revokeObjectURL(url)
  } catch {
    await loadBatchDownloads()
  }
}

async function retryBatch(job) {
  await retryBatchDownload(job.id)
  ElMessage.success('任务已重新排队')
  await loadBatchDownloads()
  startBatchPolling()
}

function handleDragEnter() {
  if (viewMode.value === 'files') dragging.value = true
}

function handleDragLeave(event) {
  if (!event.currentTarget.contains(event.relatedTarget)) dragging.value = false
}

function handleDrop(event) {
  dragging.value = false
  const files = Array.from(event.dataTransfer?.files || [])
  if (!files.length || viewMode.value !== 'files') return
  enqueueNewUploads(files)
}

async function submitCreate() {
  const valid = await formRef.value?.validate().catch(() => false)
  if (!valid) return
  creating.value = true
  try {
    await createFolder({ name: createForm.name, parent_id: currentParentId.value })
    ElMessage.success('目录已创建')
    showCreateDialog.value = false
    await loadCurrent()
  } finally {
    creating.value = false
  }
}

function resetCreateForm() {
  createForm.name = ''
  formRef.value?.clearValidate()
}

function formatDate(value) {
  if (!value) return '-'
  return new Intl.DateTimeFormat('zh-CN', { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value))
}

async function download(node, version) {
  try {
    const result = await downloadFile(node.id, version)
    const blob = result.data
    const url = URL.createObjectURL(blob)
    const link = document.createElement('a')
    link.href = url
    link.download = node.name
    link.click()
    URL.revokeObjectURL(url)
  } catch {
    // The request interceptor presents the server error.
  }
}

async function openPreview(node, version = null) {
  if (!node?.id) return
  const requestID = ++previewRequestID
  clearPreviewObjectURL()
  resetPreviewPDF()
  previewNode.value = node
  previewVersion.value = version
  previewMeta.value = null
  previewText.value = ''
  previewError.value = ''
  previewLoading.value = true
  previewDialogVisible.value = true
  if (version != null) versionDialogVisible.value = false
  try {
    const infoResult = await getFilePreview(node.id, version, { silent: true })
    if (requestID !== previewRequestID || !previewDialogVisible.value) return
    previewMeta.value = infoResult.data
    const contentResult = await getFilePreviewContent(node.id, version, { silent: true })
    if (requestID !== previewRequestID || !previewDialogVisible.value) return
    if (previewMeta.value.kind === 'text') {
      previewText.value = await contentResult.data.text()
      if (requestID !== previewRequestID || !previewDialogVisible.value) return
    } else if (previewMeta.value.kind === 'pdf') {
      await loadPreviewPDF(contentResult.data, requestID)
    } else {
      previewObjectURL.value = URL.createObjectURL(contentResult.data)
    }
  } catch (error) {
    if (requestID !== previewRequestID || !previewDialogVisible.value) return
    previewError.value = await previewErrorMessage(error)
  } finally {
    if (requestID === previewRequestID) previewLoading.value = false
  }
}

async function previewErrorMessage(error) {
  const payload = error?.response?.data
  if (payload instanceof Blob && payload.type?.includes('json')) {
    try {
      const parsed = JSON.parse(await payload.text())
      if (parsed?.message) return parsed.message
    } catch {
      // Fall through to the normalized request error.
    }
  }
  return error?.message || '预览内容加载失败'
}

function clearPreviewObjectURL() {
  if (previewObjectURL.value) URL.revokeObjectURL(previewObjectURL.value)
  previewObjectURL.value = ''
}

async function loadPreviewPDF(blob, requestID) {
  const pdfLibrary = await loadPDFLibrary()
  if (requestID !== previewRequestID || !previewDialogVisible.value) return
  const loadingTask = pdfLibrary.getDocument({
    data: new Uint8Array(await blob.arrayBuffer()),
    enableXfa: false,
    isEvalSupported: false,
    useWorkerFetch: false
  })
  previewPdfLoadingTask = loadingTask
  const documentProxy = await loadingTask.promise
  if (requestID !== previewRequestID || !previewDialogVisible.value) {
    await loadingTask.destroy()
    return
  }
  previewPdfDocument = documentProxy
  previewPdfPages.value = documentProxy.numPages
  previewPdfPage.value = 1
  previewPdfZoom.value = 1
  await nextTick()
  await renderPreviewPDF()
}

async function renderPreviewPDF() {
  if (!previewPdfDocument || !previewPdfCanvas.value) return
  const pdfLibrary = await loadPDFLibrary()
  previewPdfRenderTask?.cancel()
  const page = await previewPdfDocument.getPage(previewPdfPage.value)
  await nextTick()
  const canvas = previewPdfCanvas.value
  if (!canvas) return
  const baseViewport = page.getViewport({ scale: 1 })
  const availableWidth = Math.max((canvas.parentElement?.clientWidth || baseViewport.width) - 64, 240)
  const fitScale = Math.min(Math.max(availableWidth / baseViewport.width, 0.45), 2)
  const viewport = page.getViewport({ scale: fitScale * previewPdfZoom.value })
  const outputScale = Math.min(window.devicePixelRatio || 1, 2)
  canvas.width = Math.floor(viewport.width * outputScale)
  canvas.height = Math.floor(viewport.height * outputScale)
  canvas.style.width = `${Math.floor(viewport.width)}px`
  canvas.style.height = `${Math.floor(viewport.height)}px`
  previewPdfRendering.value = true
  const renderTask = page.render({
    canvas,
    viewport,
    transform: outputScale === 1 ? undefined : [outputScale, 0, 0, outputScale, 0, 0],
    annotationMode: pdfLibrary.AnnotationMode.DISABLE,
    background: '#ffffff'
  })
  previewPdfRenderTask = renderTask
  try {
    await renderTask.promise
  } catch (error) {
    if (error?.name !== 'RenderingCancelledException') throw error
  } finally {
    if (previewPdfRenderTask === renderTask) {
      previewPdfRenderTask = null
      previewPdfRendering.value = false
    }
  }
}

async function changePreviewPDFPage(offset) {
  const nextPage = Math.min(Math.max(previewPdfPage.value + offset, 1), previewPdfPages.value)
  if (nextPage === previewPdfPage.value) return
  previewPdfPage.value = nextPage
  await renderPreviewPDF()
}

async function changePreviewPDFZoom(offset) {
  const nextZoom = Math.min(Math.max(previewPdfZoom.value + offset, 0.5), 2)
  if (nextZoom === previewPdfZoom.value) return
  previewPdfZoom.value = nextZoom
  await renderPreviewPDF()
}

function resetPreviewPDF() {
  previewPdfRenderTask?.cancel()
  previewPdfRenderTask = null
  const loadingTask = previewPdfLoadingTask
  previewPdfLoadingTask = null
  previewPdfDocument = null
  if (loadingTask) void loadingTask.destroy().catch(() => {})
  previewPdfCanvas.value = null
  previewPdfPage.value = 1
  previewPdfPages.value = 0
  previewPdfZoom.value = 1
  previewPdfRendering.value = false
}

function resetPreview() {
  previewRequestID += 1
  clearPreviewObjectURL()
  resetPreviewPDF()
  previewLoading.value = false
  previewNode.value = null
  previewVersion.value = null
  previewMeta.value = null
  previewText.value = ''
  previewError.value = ''
}

function handleNodeCommand(command, node) {
  if (command === 'preview') return openPreview(node)
  if (command === 'versions') return openVersions(node)
	if (command === 'collaboration') return openCollaboration(node)
  if (command === 'access') return openAccessDetail(node)
  if (command === 'share') return openShare(node)
  if (command === 'acl') return openACL(node)
  if (command === 'rename') return openRename(node)
  if (command === 'move') return openMove(node)
  if (command === 'trash') return moveToTrash(node)
  if (command === 'favorite') return toggleFavorite(node)
}

async function openCollaboration(node) {
	collaborationNode.value = node
	collaborationTab.value = 'comments'
	collaborationComments.value = []
	collaborationCommentTotal.value = 0
	collaborationCommentPage.value = 1
	collaborationActivities.value = []
	collaborationActivityTotal.value = 0
	collaborationActivityPage.value = 1
	commentDraft.value = ''
	editingCommentId.value = null
	editingCommentDraft.value = ''
	mentionOptions.value = []
	collaborationVisible.value = true
	await Promise.all([loadComments(true), loadActivity(true)])
}

async function loadComments(reset = false) {
	const nodeId = collaborationNode.value?.id
	if (!nodeId || commentsLoading.value) return
	const targetPage = reset ? 1 : collaborationCommentPage.value + 1
	commentsLoading.value = true
	try {
		const result = await listNodeComments(nodeId, { page: targetPage, page_size: 20 })
		if (collaborationNode.value?.id !== nodeId) return
		const incoming = result.data?.list || []
		collaborationComments.value = reset ? incoming : [...collaborationComments.value, ...incoming]
		collaborationCommentTotal.value = result.data?.total || 0
		collaborationCommentPage.value = targetPage
	} finally {
		commentsLoading.value = false
	}
}

async function loadActivity(reset = false) {
	const nodeId = collaborationNode.value?.id
	if (!nodeId || activityLoading.value) return
	const targetPage = reset ? 1 : collaborationActivityPage.value + 1
	activityLoading.value = true
	try {
		const result = await listNodeActivity(nodeId, { page: targetPage, page_size: 20 })
		if (collaborationNode.value?.id !== nodeId) return
		const incoming = result.data?.list || []
		collaborationActivities.value = reset ? incoming : [...collaborationActivities.value, ...incoming]
		collaborationActivityTotal.value = result.data?.total || 0
		collaborationActivityPage.value = targetPage
	} finally {
		activityLoading.value = false
	}
}

function handleCollaborationTabChange(name) {
	if (name === 'comments' && !collaborationComments.value.length && collaborationCommentTotal.value > 0) loadComments(true)
	if (name === 'activity' && !collaborationActivities.value.length && collaborationActivityTotal.value > 0) loadActivity(true)
}

function searchMentionCandidates(keyword) {
	window.clearTimeout(mentionSearchTimer)
	const sequence = ++mentionSearchSequence
	mentionLoading.value = true
	mentionSearchTimer = window.setTimeout(async () => {
		try {
			const nodeId = collaborationNode.value?.id
			if (!nodeId) return
			const result = await listNodeMentionCandidates(nodeId, keyword || '')
			if (sequence !== mentionSearchSequence) return
			mentionOptions.value = (result.data || []).map((member) => ({
				value: member.username,
				label: `${member.real_name || member.username} (@${member.username})`
			}))
		} catch (error) {
			if (sequence === mentionSearchSequence) mentionOptions.value = []
		} finally {
			if (sequence === mentionSearchSequence) mentionLoading.value = false
		}
	}, 250)
}

async function submitComment() {
	const content = commentDraft.value.trim()
	if (!collaborationNode.value || !content) return
	commentSubmitting.value = true
	try {
		await createNodeComment(collaborationNode.value.id, content)
		commentDraft.value = ''
		ElMessage.success('评论已发表')
		await Promise.all([loadComments(true), loadActivity(true)])
	} finally {
		commentSubmitting.value = false
	}
}

function startEditComment(comment) {
	editingCommentId.value = comment.id
	editingCommentDraft.value = comment.content
	mentionOptions.value = []
}

function cancelEditComment() {
	editingCommentId.value = null
	editingCommentDraft.value = ''
}

async function saveComment(comment) {
	const content = editingCommentDraft.value.trim()
	if (!collaborationNode.value || !content) return
	commentUpdating.value = true
	try {
		await updateNodeComment(collaborationNode.value.id, comment.id, { content, revision: comment.revision })
		cancelEditComment()
		ElMessage.success('评论已更新')
		await Promise.all([loadComments(true), loadActivity(true)])
	} finally {
		commentUpdating.value = false
	}
}

async function removeComment(comment) {
	if (!collaborationNode.value) return
	try {
		await ElMessageBox.confirm('删除后评论内容不可恢复，确定继续吗？', '删除评论', {
			type: 'warning', confirmButtonText: '确认删除', cancelButtonText: '取消'
		})
	} catch (error) {
		if (error !== 'cancel') console.error(error)
		return
	}
	await deleteNodeComment(collaborationNode.value.id, comment.id)
	ElMessage.success('评论已删除')
	await Promise.all([loadComments(true), loadActivity(true)])
}

function resetCollaboration() {
	window.clearTimeout(mentionSearchTimer)
	mentionSearchSequence += 1
	mentionLoading.value = false
	collaborationNode.value = null
	collaborationComments.value = []
	collaborationActivities.value = []
	commentDraft.value = ''
	mentionOptions.value = []
	cancelEditComment()
}

function avatarInitial(value) {
	return Array.from(String(value || '用').trim())[0]?.toUpperCase() || '用'
}

function relativeTime(value) {
	const time = new Date(value).getTime()
	if (!Number.isFinite(time)) return '-'
	const seconds = Math.round((time - Date.now()) / 1000)
	const formatter = new Intl.RelativeTimeFormat('zh-CN', { numeric: 'auto' })
	if (Math.abs(seconds) < 60) return formatter.format(seconds, 'second')
	const minutes = Math.round(seconds / 60)
	if (Math.abs(minutes) < 60) return formatter.format(minutes, 'minute')
	const hours = Math.round(minutes / 60)
	if (Math.abs(hours) < 24) return formatter.format(hours, 'hour')
	const days = Math.round(hours / 24)
	if (Math.abs(days) < 30) return formatter.format(days, 'day')
	return formatDate(value)
}

function isCommentEdited(comment) {
	return comment.revision > 1 || Math.abs(new Date(comment.updated_at).getTime() - new Date(comment.created_at).getTime()) > 1000
}

function commentSegments(comment) {
	const mentions = new Set((comment.mentions || []).map((item) => `@${item.username}`.toLowerCase()))
	return String(comment.content || '').split(/(@[\p{L}\p{N}._-]+)/gu).filter(Boolean).map((text) => ({
		text,
		mention: mentions.has(text.toLowerCase())
	}))
}

async function toggleFavorite(node) {
  const favorite = !node.is_favorite
  await setFavorite(node.id, favorite)
  node.is_favorite = favorite
  ElMessage.success(favorite ? '已收藏' : '已取消收藏')
  if (viewMode.value === 'favorites' && !favorite) await loadCurrent()
}

async function openACL(node) {
  aclNode.value = node
  inheritanceMode.value = node.inherit_mode || 'inherit'
  aclDialogVisible.value = true
  aclLoading.value = true
  try {
    const workspaceID = userStore.currentWorkspaceId
    const [aclResult, memberResult, groupResult] = await Promise.all([
      listFolderACL(node.id),
      getWorkspaceMembers(workspaceID, { page: 1, page_size: 200 }),
      getWorkspaceGroupDirectory(workspaceID)
    ])
    workspaceMembers.value = memberResult.data?.list || []
    workspaceGroups.value = groupResult.data || []
    aclEntries.value = (aclResult.data || []).map((entry) => ({
      subject_type: entry.subject_type,
      subject_id: entry.subject_id,
      effect: entry.effect,
      access_level: entry.access_level,
      inherit_to_children: entry.inherit_to_children,
      subject_key: `${entry.subject_type}:${entry.subject_id}`
    }))
    aclForm.subject_type = 'user'
    aclForm.subject_id = null
    aclForm.effect = 'allow'
    aclForm.access_level = 'read'
    aclForm.inherit_to_children = true
  } catch {
    aclDialogVisible.value = false
  } finally {
    aclLoading.value = false
  }
}

function buildGroupDirectoryTree(groups) {
  const roots = [
    { value: 'group-root:ldap', label: 'LDAP 组织', disabled: true, children: [] },
    { value: 'group-root:local', label: '本地用户组', disabled: true, children: [] }
  ]
  const ldapRoot = roots[0]
  const localRoot = roots[1]

  for (const group of groups) {
    const managed = group.source === 'ldap'
    let parent = managed ? ldapRoot : localRoot
    if (managed) {
      const path = Array.isArray(group.directory_path) ? group.directory_path : []
      const traversed = []
      for (const segment of path) {
        traversed.push(segment)
        const key = `group-branch:ldap:${JSON.stringify(traversed)}`
        let branch = parent.children.find((item) => item.value === key)
        if (!branch) {
          branch = { value: key, label: segment, disabled: true, children: [] }
          parent.children.push(branch)
        }
        parent = branch
      }
    }
    parent.children.push({
      value: group.id,
      label: group.name,
      disabled: false,
      source: group.source,
      directory_path: group.directory_path || []
    })
  }
  return roots.filter((root) => root.children.length > 0)
}

async function openAccessDetail(node) {
  accessDetail.value = null
  accessDetailVisible.value = true
  accessDetailLoading.value = true
  try {
    const result = await getNodeDetail(node.id)
    accessDetail.value = result.data
  } catch {
    accessDetailVisible.value = false
  } finally {
    accessDetailLoading.value = false
  }
}

function accessSourceType(source) {
  if (source.type === 'role') return { label: '管理角色', type: 'danger' }
  if (source.type === 'user') return { label: '个人授权', type: 'primary' }
  if (source.directory_source === 'ldap') return { label: 'LDAP 部门组', type: 'success' }
  return { label: '本地用户组', type: 'info' }
}

function accessSourceName(source) {
  if (source.type === 'role') return source.name
  if (source.type === 'user') return source.name || '当前用户'
  return source.name || '用户组'
}

function accessSourcePath(source) {
  if (source.type !== 'group') return ''
  const path = Array.isArray(source.directory_path) ? source.directory_path : []
  return path.length ? path.join(' / ') : (source.directory_source === 'ldap' ? 'LDAP 组织' : '')
}

function accessSourceScope(source) {
  if (source.type === 'role') return '管理角色在当前工作空间内生效'
  if (source.inherited) return `继承自“${source.source_node_name || '上级目录'}”`
  return '直接授予于当前对象'
}

function permissionSourceLabel(source) {
  if (source.type === 'user') return `个人 · ${source.name || '-'}`
  const path = Array.isArray(source.directory_path) ? source.directory_path : []
  const prefix = source.directory_source === 'ldap' ? 'LDAP 部门组' : '本地用户组'
  const qualifiedName = [...path, source.name].filter(Boolean).join(' / ')
  return `${prefix} · ${qualifiedName || '-'}`
}

function subjectLabel(entry) {
  if (entry.subject_type === 'group') {
    const group = workspaceGroups.value.find((item) => item.id === entry.subject_id)
    if (!group) return `用户组 #${entry.subject_id}`
    const path = Array.isArray(group.directory_path) ? group.directory_path : []
    return path.length ? `${path.join(' / ')} / ${group.name}` : group.name
  }
  const member = workspaceMembers.value.find((item) => item.user_id === entry.subject_id)
  return member ? `${member.real_name || member.username} (${member.username})` : `用户 #${entry.subject_id}`
}

function aclSubjectTypeLabel(entry) {
  if (entry.subject_type !== 'group') return '用户'
  const group = workspaceGroups.value.find((item) => item.id === entry.subject_id)
  return group?.source === 'ldap' ? 'LDAP 组' : '本地组'
}

function accessLevelLabel(value) {
  return { read: '读取', read_write: '读写', admin: '管理员' }[value] || value
}

function addACL() {
  if (!aclForm.subject_id) return ElMessage.warning('请选择授权主体')
  const subjectKey = `${aclForm.subject_type}:${aclForm.subject_id}`
  if (aclEntries.value.some((entry) => entry.subject_key === subjectKey)) {
    return ElMessage.warning('同一授权主体不能重复添加')
  }
  aclEntries.value.push({ ...aclForm, subject_key: subjectKey })
  aclForm.subject_id = null
}

function removeACL(index) {
  ElMessageBox.confirm('确定删除这条目录授权？保存权限后会对访问控制生效。', '删除授权', {
    type: 'warning',
    confirmButtonText: '确认删除'
  }).then(() => {
    aclEntries.value.splice(index, 1)
  }).catch((error) => {
    if (error !== 'cancel') console.error(error)
  })
}

async function saveACL() {
  if (!aclNode.value) return
  if (!aclEntries.value.length) return ElMessage.warning('请至少添加一条目录授权')
  if (!aclEntries.value.some((entry) => entry.effect === 'allow' && entry.access_level === 'admin')) {
    return ElMessage.warning('目录必须至少保留一名直接管理员')
  }
  aclSaving.value = true
  try {
    const entries = aclEntries.value.map(({ subject_key, ...entry }) => entry)
    await replaceFolderACL(aclNode.value.id, entries)
    const previousMode = aclNode.value.inherit_mode || 'inherit'
    if (inheritanceMode.value !== previousMode) {
      await setFolderInheritance(aclNode.value.id, inheritanceMode.value)
      aclNode.value.inherit_mode = inheritanceMode.value
    }
    ElMessage.success('目录权限已保存')
    aclDialogVisible.value = false
  } finally {
    aclSaving.value = false
  }
}

function openShare(node) {
  shareNode.value = node
  createdShare.value = null
  copiedShareField.value = ''
  shareForm.expiresAt = new Date(Date.now() + 7 * 24 * 60 * 60 * 1000).toISOString()
  shareForm.password = ''
  shareForm.maxDownloads = null
  shareDialogVisible.value = true
}

function disablePastDate(date) {
  return date.getTime() < Date.now() - 24 * 60 * 60 * 1000
}

async function submitShare() {
  if (!shareNode.value || !shareForm.expiresAt) return
  if (shareForm.password && shareForm.password.length < 8) {
    ElMessage.warning('分享密码至少需要 8 个字符')
    return
  }
  shareSubmitting.value = true
  try {
    const result = await createShare({
      node_id: shareNode.value.id,
      expires_at: new Date(shareForm.expiresAt).toISOString(),
      password: shareForm.password || undefined,
      max_downloads: shareForm.maxDownloads || undefined
    })
    createdShare.value = result.data
    ElMessage.success('分享已创建')
  } finally {
    shareSubmitting.value = false
  }
}

async function copyShareValue(value, field) {
  if (await copyText(value)) {
    copiedShareField.value = field
    ElMessage.success(field === 'password' ? '访问密码已复制' : '分享链接已复制')
  }
  else ElMessage.error('复制失败，请手动复制')
}

function resetShareDialog() {
  shareNode.value = null
  createdShare.value = null
  copiedShareField.value = ''
  shareForm.expiresAt = ''
  shareForm.password = ''
  shareForm.maxDownloads = null
}

function openRename(node) {
  selectedNode.value = node
  renameName.value = node.name
  renameDialogVisible.value = true
}

async function submitRename() {
  const name = renameName.value.trim()
  if (!name || !selectedNode.value) return
  actionLoading.value = true
  try {
    await renameNode(selectedNode.value.id, { name })
    ElMessage.success('重命名成功')
    renameDialogVisible.value = false
    await loadCurrent()
  } finally {
    actionLoading.value = false
  }
}

async function openMove(node) {
  batchMoveMode.value = false
  selectedNode.value = node
  moveParentId.value = node.parent_id || 0
  const result = await listFolderTree()
  folderTree.value = buildFolderTree(result.data || [], [node.id])
  moveDialogVisible.value = true
}

async function openBatchMove() {
  batchMoveMode.value = true
  selectedNode.value = null
  const parentIDs = new Set(selectedNodes.value.map((node) => node.parent_id || 0))
  moveParentId.value = parentIDs.size === 1 ? [...parentIDs][0] : 0
  const result = await listFolderTree()
  folderTree.value = buildFolderTree(result.data || [], selectedNodes.value.filter((node) => node.type === 'folder').map((node) => node.id))
  moveDialogVisible.value = true
}

async function submitMove() {
  if (batchMoveMode.value) {
    if (!selectedNodes.value.length) return
    try {
      await executeBatchNodeOperation('批量移动', batchMoveNodes, { node_ids: selectedNodeIDs(), parent_id: moveParentId.value || null })
      moveDialogVisible.value = false
    } finally {
      batchMoveMode.value = false
    }
    return
  }
  if (!selectedNode.value) return
  actionLoading.value = true
  try {
    await moveNode(selectedNode.value.id, { parent_id: moveParentId.value || null })
    ElMessage.success('移动成功')
    moveDialogVisible.value = false
    await loadCurrent()
  } finally {
    actionLoading.value = false
  }
}

function buildFolderTree(folders, movingNodeIds = []) {
  const excluded = new Set(movingNodeIds)
  let changed = true
  while (changed) {
    changed = false
    for (const folder of folders) {
      if (folder.parent_id && excluded.has(folder.parent_id) && !excluded.has(folder.id)) {
        excluded.add(folder.id)
        changed = true
      }
    }
  }
  const map = new Map()
  for (const folder of folders) {
    if (!excluded.has(folder.id)) map.set(folder.id, { value: folder.id, label: folder.name, children: [] })
  }
  const roots = []
  for (const folder of folders) {
    const item = map.get(folder.id)
    if (!item) continue
    const parent = map.get(folder.parent_id)
    if (parent) parent.children.push(item)
    else roots.push(item)
  }
  return [{ value: 0, label: '根目录', children: roots }]
}

async function moveToTrash(node) {
  await ElMessageBox.confirm(`确定将“${node.name}”移入回收站？`, '移入回收站', { type: 'warning' })
  await trashNode(node.id)
  ElMessage.success('已移入回收站')
  await loadCurrent()
}

async function restore(node) {
  await ElMessageBox.confirm(`确定恢复“${node.name}”？`, '恢复文件', {
    type: 'warning',
    confirmButtonText: '确认恢复'
  })
  await restoreNode(node.id)
  ElMessage.success('恢复成功')
  await loadCurrent()
}

async function openVersions(node) {
  versionNode.value = node
  versionDialogVisible.value = true
  versionLoading.value = true
  try {
    const result = await listFileVersions(node.id)
    versions.value = result.data || []
  } finally {
    versionLoading.value = false
  }
}

async function restoreVersion(version) {
  await ElMessageBox.confirm(`将版本 ${version.version_no} 恢复为新版本？`, '恢复版本', { type: 'warning' })
  await restoreFileVersion(versionNode.value.id, version.version_no)
  ElMessage.success('版本已恢复')
  await openVersions(versionNode.value)
  await loadCurrent()
}

async function rescanVersion(version) {
  await ElMessageBox.confirm(`将重新扫描版本 ${version.version_no} 的文件对象，扫描期间该版本会短暂进入“扫描中”状态。确定继续吗？`, '重新扫描文件版本', {
    type: 'warning',
    confirmButtonText: '开始重扫',
    cancelButtonText: '取消'
  })
  versionLoading.value = true
  try {
    await rescanFileVersion(versionNode.value.id, version.version_no)
    ElMessage.success('重新扫描完成')
    await openVersions(versionNode.value)
    await loadCurrent()
  } finally {
    versionLoading.value = false
  }
}

function canRescanVersion(version) {
	return ['scan_error', 'unscanned'].includes(version?.scan_status) && (!version?.storage_class || version.storage_class === 'standard')
}

function scanStatusMeta(status) {
  if (status === 'clean') return { type: 'success', label: '安全' }
  if (status === 'infected') return { type: 'danger', label: '感染' }
  if (status === 'pending_scan') return { type: 'warning', label: '扫描中' }
  if (status === 'scan_error') return { type: 'danger', label: '扫描失败' }
  if (status === 'unscanned') return { type: 'info', label: '未扫描' }
  return { type: 'info', label: status || '-' }
}

function storageClassMeta(storageClass) {
  if (storageClass === 'archive') return { type: 'warning', label: '已归档' }
  if (storageClass === 'glacier') return { type: 'info', label: '待解冻' }
  if (storageClass === 'restoring') return { type: 'info', label: '解冻中' }
  return { type: 'success', label: '标准' }
}

function canDownloadStorage(storageClass) {
  return !['glacier', 'restoring'].includes(storageClass)
}

const uploadChunkSize = 8 * 1024 * 1024
const uploadConcurrency = 2
const maxUploadQueueSize = 50

async function chooseNewFile() {
  fileSelectionMode.value = 'new'
  resumeUploadTaskId.value = ''
  await nextTick()
  fileInput.value?.click()
}

async function chooseResumeFile(task) {
  if (!task?.uploadId) return
  fileSelectionMode.value = 'resume'
  resumeUploadTaskId.value = task.clientId
  await nextTick()
  fileInput.value?.click()
}

async function selectFile(event) {
  const files = Array.from(event.target.files || [])
  event.target.value = ''
  if (!files.length) return
  const mode = fileSelectionMode.value
  fileSelectionMode.value = 'new'
  if (mode === 'resume') {
    const task = uploadTasks.value.find((item) => item.clientId === resumeUploadTaskId.value)
    resumeUploadTaskId.value = ''
    if (task) await resumeSelectedUpload(task, files[0])
    return
  }
  enqueueNewUploads(files)
}

function newUploadClientId() {
  return window.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function createUploadTask(file) {
  return {
    clientId: newUploadClientId(),
    name: file.name,
    size: file.size,
    lastModified: file.lastModified,
    file,
    progress: 0,
    status: 'initializing',
    statusText: '正在创建上传会话',
    pauseRequested: false,
    cancelled: false,
    uploadId: null,
    chunkSize: uploadChunkSize,
    totalChunks: 0,
    targetParentId: currentParentId.value,
    expiresAt: null
  }
}

function enqueueNewUploads(files) {
  const availableSlots = Math.max(0, maxUploadQueueSize - uploadTasks.value.filter((task) => !['completed', 'cancelled'].includes(task.status)).length)
  if (!availableSlots) {
    ElMessage.warning(`上传队列最多保留 ${maxUploadQueueSize} 个未结束任务`)
    return
  }
  const accepted = []
  const pendingNames = new Set(uploadTasks.value
    .filter((task) => !['completed', 'cancelled'].includes(task.status))
    .map((task) => normalizeFileName(task.name)))
  for (const file of files.slice(0, availableSlots)) {
    const normalizedName = normalizeFileName(file.name)
    if (pendingNames.has(normalizedName)) {
      ElMessage.warning(`“${file.name}”已在上传队列中`)
      continue
    }
    pendingNames.add(normalizedName)
    const task = createUploadTask(file)
    uploadTasks.value.push(task)
    accepted.push(task)
  }
  if (files.length > availableSlots) ElMessage.warning(`本次仅加入前 ${availableSlots} 个文件，上传队列上限为 ${maxUploadQueueSize}`)
  for (const task of accepted) prepareNewUpload(task)
}

async function prepareNewUpload(task) {
  try {
    const target = await resolveUploadTarget(task.name, task.targetParentId)
    if (task.cancelled) return
    const payload = {
      display_name: task.name,
      total_size: task.size,
      chunk_size: task.chunkSize,
      target_parent_id: task.targetParentId
    }
    if (target) {
      payload.node_id = target.nodeId
      payload.base_version_no = target.baseVersionNo
    }
    const initialized = await initUpload(payload)
    task.uploadId = initialized.data.upload_id
    task.totalChunks = initialized.data.total_chunks
    task.expiresAt = initialized.data.expires_at
    if (task.cancelled) {
      try {
        await cancelUpload(task.uploadId)
      } catch {
        // The session may already have expired or been cancelled.
      }
      task.uploadId = null
      task.status = 'cancelled'
      task.statusText = '已取消'
      persistPendingUploads()
      return
    }
    task.status = 'queued'
    task.statusText = '等待上传'
    persistPendingUploads()
    processUploadQueue()
  } catch (error) {
    markUploadFailed(task, error)
  }
}

async function resolveUploadTarget(fileName, parentId) {
  const params = { page: 1, page_size: 200, keyword: fileName }
  const result = parentId === null
    ? await listRoots(params)
    : await listChildren(parentId, params)
  const normalizedName = normalizeFileName(fileName)
  const existing = (result.data?.list || []).find((node) => normalizeFileName(node.name) === normalizedName)
  if (!existing) return null
  if (existing.type !== 'file') throw new Error('当前目录已存在同名目录，无法上传该文件')
  const versionResult = await listFileVersions(existing.id)
  const latest = versionResult.data?.[0]
  if (!latest) throw new Error('同名文件缺少有效版本，请刷新后重试')
  return { nodeId: existing.id, baseVersionNo: latest.version_no }
}

function normalizeFileName(value) {
  return String(value || '').trim().toLocaleLowerCase()
}

async function resumeSelectedUpload(task, file) {
  if (!task?.uploadId) return
  if (normalizeFileName(file.name) !== normalizeFileName(task.name) || file.size !== task.size || (task.lastModified && file.lastModified !== task.lastModified)) {
    ElMessage.error('所选文件与原上传文件不一致，请重新选择')
    return
  }
  task.file = file
  task.cancelled = false
  task.pauseRequested = false
  task.status = 'queued'
  task.statusText = '等待继续上传'
  persistPendingUploads()
  processUploadQueue()
}

function processUploadQueue() {
  while (activeUploadCount.value < uploadConcurrency) {
    const task = uploadTasks.value.find((item) => item.status === 'queued' && item.file && item.uploadId && !item.cancelled)
    if (!task) return
    activeUploadCount.value += 1
    task.status = 'uploading'
    uploadFileParts(task.file, task)
      .catch((error) => markUploadFailed(task, error))
      .finally(() => {
        activeUploadCount.value = Math.max(0, activeUploadCount.value - 1)
        persistPendingUploads()
        processUploadQueue()
      })
  }
}

async function uploadFileParts(file, task) {
  task.status = 'uploading'
  task.pauseRequested = false
  task.statusText = '读取断点状态'
  const statusResult = await getUploadStatus(task.uploadId)
  const status = statusResult.data
  if (!['initialized', 'uploading'].includes(status.status)) {
    task.uploadId = null
    persistPendingUploads()
    throw new Error('上传会话已结束，请重新上传')
  }
  if (new Date(status.expires_at).getTime() <= Date.now()) {
    task.uploadId = null
    persistPendingUploads()
    throw new Error('上传会话已过期，请重新上传')
  }
  if (normalizeFileName(file.name) !== normalizeFileName(status.display_name) || file.size !== status.total_size) {
    throw new Error('所选文件与服务端上传会话不一致')
  }
  task.chunkSize = status.chunk_size
  task.totalChunks = status.total_chunks
  task.expiresAt = status.expires_at
  task.targetParentId = status.target_parent_id
  const received = new Set(status.received_parts || [])
  task.progress = Math.round((received.size / task.totalChunks) * 100)
  task.statusText = received.size ? `继续上传，已完成 ${received.size}/${task.totalChunks} 个分片` : '上传中'
  persistPendingUploads()

  for (let partNo = 0; partNo < task.totalChunks; partNo += 1) {
    if (task.cancelled) return
    if (task.pauseRequested) {
      task.status = 'paused'
      task.statusText = `已暂停，完成 ${received.size}/${task.totalChunks} 个分片`
      persistPendingUploads()
      return
    }
    if (!received.has(partNo)) {
      const start = partNo * task.chunkSize
      const end = Math.min(file.size, start + task.chunkSize)
      await uploadPartWithRetry(task.uploadId, partNo, file.slice(start, end))
      received.add(partNo)
    }
    task.progress = Math.round((received.size / task.totalChunks) * 100)
    task.statusText = `上传中，已完成 ${received.size}/${task.totalChunks} 个分片`
    persistPendingUploads()
  }
  if (task.cancelled) return
  if (task.pauseRequested) {
    task.status = 'paused'
    task.statusText = `已暂停，完成 ${received.size}/${task.totalChunks} 个分片`
    return
  }
  task.statusText = '正在校验并生成文件版本'
  await completeUpload(task.uploadId, {})
  task.progress = 100
  task.status = 'completed'
  task.statusText = '上传完成'
  task.file = null
  task.uploadId = null
  persistPendingUploads()
  ElMessage.success(`“${task.name}”上传完成`)
  await loadCurrent()
}

function markUploadFailed(task, error) {
  if (task.cancelled) return
  task.status = 'error'
  task.statusText = task.uploadId
    ? (task.file ? '上传中断，可重试' : '上传中断，请选择原文件继续')
    : (error.message || '上传失败')
  persistPendingUploads()
  if (!error.presented) ElMessage.error(error.message || '上传失败')
}

function pauseUploadTask(task) {
  if (task.status !== 'uploading') return
  task.pauseRequested = true
  task.statusText = '正在暂停，将在当前分片完成后停止'
}

function continueUploadTask(task) {
  if (task.status !== 'paused' || !task.file) return
  task.pauseRequested = false
  task.status = 'queued'
  task.statusText = '等待继续上传'
  processUploadQueue()
}

function retryUploadTask(task) {
  if (task.status !== 'error') return
  if (!task.uploadId) {
    task.status = 'initializing'
    task.statusText = '正在重新创建上传会话'
    prepareNewUpload(task)
    return
  }
  if (!task.file) {
    chooseResumeFile(task)
    return
  }
  task.status = 'queued'
  task.statusText = '等待重试'
  processUploadQueue()
}

function canCancelUploadTask(task) {
  return !['completed', 'cancelled', 'cancelling'].includes(task.status)
}

async function cancelUploadTask(task) {
  if (!task) return
  try {
    await ElMessageBox.confirm(`确定取消“${task.name}”的上传任务？已上传但未完成的分片会被清理。`, '取消上传', {
      type: 'warning',
      confirmButtonText: '确认取消'
    })
  } catch (error) {
    if (error !== 'cancel') console.error(error)
    return
  }
  task.cancelled = true
  task.pauseRequested = false
  task.status = 'cancelling'
  task.statusText = '正在取消'
  if (task.uploadId) {
    try {
      await cancelUpload(task.uploadId)
    } catch {
      // The local task is cleared even when the server already expired it.
    }
  }
  task.file = null
  task.uploadId = null
  task.status = 'cancelled'
  task.statusText = '已取消'
  persistPendingUploads()
}

function pendingUploadStorageKey() {
  const userID = userStore.user?.id
  const workspaceID = userStore.currentWorkspaceId
  return userID && workspaceID ? `fileshare.pending-upload:${userID}:${workspaceID}` : null
}

function persistPendingUploads() {
  const key = pendingUploadStorageKey()
  if (!key) return
  const pending = uploadTasks.value
    .filter((task) => task.uploadId && !['completed', 'cancelled'].includes(task.status))
    .map((task) => ({
      uploadId: task.uploadId,
      name: task.name,
      size: task.size,
      lastModified: task.lastModified,
      targetParentId: task.targetParentId,
      expiresAt: task.expiresAt
    }))
  if (pending.length) localStorage.setItem(key, JSON.stringify(pending))
  else localStorage.removeItem(key)
}

function readPendingUploads() {
  const key = pendingUploadStorageKey()
  if (!key) return []
  try {
    const parsed = JSON.parse(localStorage.getItem(key) || '[]')
    if (Array.isArray(parsed)) return parsed
    return parsed?.uploadId ? [parsed] : []
  } catch {
    localStorage.removeItem(key)
    return []
  }
}

async function restorePendingUploads() {
  const pendingUploads = readPendingUploads()
  if (!pendingUploads.length) return
  const restored = await Promise.all(pendingUploads.map(async (pending) => {
    try {
      const result = await getUploadStatus(pending.uploadId, { silent: true })
      const status = result.data
      if (!['initialized', 'uploading'].includes(status.status) || new Date(status.expires_at).getTime() <= Date.now()) return null
      const receivedCount = (status.received_parts || []).length
      return {
        clientId: pending.uploadId,
        uploadId: status.upload_id,
        name: status.display_name,
        size: status.total_size,
        lastModified: pending.lastModified,
        file: null,
        chunkSize: status.chunk_size,
        totalChunks: status.total_chunks,
        targetParentId: status.target_parent_id,
        expiresAt: status.expires_at,
        progress: status.total_chunks ? Math.round((receivedCount / status.total_chunks) * 100) : 0,
        status: 'waiting_file',
        statusText: `检测到未完成上传，已接收 ${receivedCount}/${status.total_chunks} 个分片`,
        pauseRequested: false,
        cancelled: false
      }
    } catch {
      return null
    }
  }))
  uploadTasks.value.push(...restored.filter(Boolean))
  persistPendingUploads()
}

function clearFinishedUploads() {
  uploadTasks.value = uploadTasks.value.filter((task) => !['completed', 'cancelled'].includes(task.status))
  persistPendingUploads()
}

async function uploadPartWithRetry(uploadId, partNo, chunk) {
  let lastError
  for (let attempt = 0; attempt < 3; attempt += 1) {
    try {
      return await uploadPart(uploadId, partNo, chunk)
    } catch (error) {
      lastError = error
      if (attempt < 2) await new Promise((resolve) => setTimeout(resolve, 500 * 2 ** attempt))
    }
  }
  throw lastError
}

function formatBytes(value) {
  if (value < 1024) return `${value} B`
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`
  return `${(value / (1024 * 1024 * 1024)).toFixed(1)} GB`
}

async function consumeSourceLocation() {
  const shareId = Number(route.query.locateShare)
  if (!Number.isInteger(shareId) || shareId < 1) return false
  let location
  try {
    const result = await getShareDetail(shareId)
    location = result.data?.source_location
  } catch {
    return false
  } finally {
    await router.replace({ name: 'Files' })
  }
  if (!location || !Number.isInteger(location.node_id) || !Array.isArray(location.breadcrumbs)) {
    ElMessage.info('源文件当前不可定位')
    return false
  }
  breadcrumbs.value = [
    { id: null, name: '根目录' },
    ...location.breadcrumbs.filter((item) => Number.isInteger(item.id) && item.name).map((item) => ({ id: item.id, name: item.name }))
  ]
  locatedNodeId.value = location.node_id
  return true
}

async function consumeCollaborationLocation() {
	const nodeId = Number(route.query.node_id)
	if (route.query.panel !== 'collaboration' || !Number.isInteger(nodeId) || nodeId < 1) return null
	let detail
	try {
		const result = await getNodeDetail(nodeId)
		detail = result.data
	} catch {
		return null
	} finally {
		await router.replace({ name: 'Files' })
	}
	if (!detail?.node) return null
	const location = detail.location
	if (location && Array.isArray(location.breadcrumbs)) {
		breadcrumbs.value = [
			{ id: null, name: '根目录' },
			...location.breadcrumbs.filter((item) => Number.isInteger(item.id) && item.name).map((item) => ({ id: item.id, name: item.name }))
		]
		locatedNodeId.value = detail.node.id
	}
	return detail.node
}

onMounted(async () => {
  mobileMediaQuery.addEventListener('change', updateMobileBreakpoint)
	const collaborationTarget = await consumeCollaborationLocation()
  const locatingSource = collaborationTarget ? false : await consumeSourceLocation()
  await loadCurrent()
	if (collaborationTarget) {
		await openCollaboration(collaborationTarget)
		ElMessage.success('已打开相关文件的协作详情')
	}
  if (locatingSource) {
    await nextTick()
    if (nodes.value.some((node) => node.id === locatedNodeId.value)) ElMessage.success('已定位到外链源文件')
    else ElMessage.warning('源文件位置已变化，请刷新外链详情后重试')
  }
  await restorePendingUploads()
  if (route.query.panel === 'downloads') await openBatchDownloads()
})

onBeforeUnmount(stopBatchPolling)
onBeforeUnmount(() => mobileMediaQuery.removeEventListener('change', updateMobileBreakpoint))
onBeforeUnmount(resetPreview)
onBeforeUnmount(resetCollaboration)
</script>

<style scoped>
.files-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
  padding: 0 0 8px;
}

.files-toolbar__filters {
  min-width: 0;
}

.files-toolbar__filters :deep(.common-mode-switch) {
  max-width: 100%;
  justify-content: flex-start;
  overflow-x: auto;
}

.file-input {
  position: absolute;
  width: 1px;
  height: 1px;
  padding: 0;
  margin: -1px;
  overflow: hidden;
  opacity: 0;
  white-space: nowrap;
  border: 0;
}

.page-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.workspace-label {
  display: inline-flex;
  align-items: center;
  min-height: 30px;
  padding-right: 12px;
  border-right: 1px solid var(--border-color);
  color: var(--text-secondary);
  font-size: 13px;
  font-weight: 600;
}

.file-search {
  width: min(280px, 100%);
}

.favorites-search {
  display: flex;
  justify-content: flex-end;
  padding: 10px 14px;
  border: 1px solid var(--border-color);
  border-radius: var(--radius);
  background: var(--panel-bg-strong);
}

.permission-source-list {
  display: flex;
  flex-wrap: wrap;
  justify-content: center;
  gap: 6px;
}

.permission-source-tag {
  max-width: 260px;
}

.permission-source-tag :deep(.el-tag__content) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-search-panel {
	padding: 12px 14px 2px;
	border: 1px solid var(--panel-border);
	border-radius: 8px;
	background: var(--panel-bg-strong);
}

.file-search-form {
	display: flex;
	align-items: flex-end;
	flex-wrap: wrap;
	gap: 0 14px;
}

.file-search-form :deep(.el-form-item) {
	margin-right: 0;
	margin-bottom: 10px;
}

.search-filter-short {
	width: 128px;
}

.search-filter-user {
	width: 190px;
}

.search-filter-time-kind {
	width: 112px;
	margin-right: 6px;
}

.search-filter-date {
	width: 252px;
}

.search-filter-sort {
	width: 148px;
}

.search-size-range {
	display: flex;
	align-items: center;
	gap: 6px;
}

.search-size-range :deep(.el-input-number) {
	width: 92px;
}

.search-size-range > span {
	color: var(--text-muted);
	font-size: 12px;
}

.search-filter-actions {
	display: flex;
	align-items: center;
	gap: 8px;
	margin-bottom: 10px;
}

.search-result-summary {
	display: flex;
	align-items: center;
	gap: 8px;
	min-height: 28px;
	padding: 0 4px;
	color: var(--text-secondary);
	font-size: 13px;
}

.search-result-summary strong {
	min-width: 24px;
	padding: 2px 8px;
	border-radius: 999px;
	background: var(--accent-soft);
	color: var(--accent-primary);
	text-align: center;
}

.upload-queue {
  overflow: hidden;
  border: 1px solid var(--panel-border);
  border-radius: 8px;
  background: var(--panel-bg-strong);
}

.upload-queue__header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  min-height: 48px;
  padding: 8px 12px;
  border-bottom: 1px solid var(--panel-border);
}

.upload-queue__header > div {
  display: flex;
  align-items: baseline;
  min-width: 0;
  gap: 10px;
}

.upload-queue__list {
  display: flex;
  flex-direction: column;
  max-height: 280px;
  overflow-y: auto;
}

.upload-task {
  display: grid;
  grid-template-columns: minmax(180px, 1fr) minmax(220px, 1.5fr) minmax(190px, auto);
  align-items: center;
  gap: 14px;
  min-height: 62px;
  padding: 8px 12px;
}

.upload-task + .upload-task {
  border-top: 1px solid var(--panel-border);
}

.upload-task__identity {
  display: flex;
  flex-direction: column;
  min-width: 0;
  gap: 3px;
}

.upload-task__name {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 650;
}

.upload-task__progress {
  min-width: 0;
}

.upload-task__actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  min-width: 190px;
}

@media (max-width: 640px) {
  .page-actions {
    width: 100%;
  }

  .page-actions .el-button {
    flex: 1;
  }

  .file-search {
    width: 100%;
  }

  .breadcrumb-bar {
    align-items: stretch;
    flex-direction: column;
  }

  .breadcrumb-actions {
    width: 100%;
  }

  .breadcrumb-actions .file-search {
    flex: 1;
  }

  .files-toolbar__filters {
    align-items: stretch;
    flex-direction: column;
  }

	.file-search-form {
		display: grid;
		grid-template-columns: 1fr;
	}

	.file-search-form :deep(.el-form-item),
	.search-filter-short,
	.search-filter-user,
	.search-filter-date,
	.search-filter-sort {
		width: 100%;
	}

	.file-search-form :deep(.el-form-item) {
		display: flex;
		align-items: stretch;
		flex-direction: column;
	}

	.file-search-form :deep(.el-form-item__label) {
		width: 100%;
		height: auto;
		padding: 0 0 5px;
		justify-content: flex-start;
		line-height: 20px;
	}

	.file-search-form :deep(.el-form-item__content) {
		display: flex;
		align-items: stretch;
		flex-direction: column;
		width: 100%;
		min-width: 0;
		margin-left: 0 !important;
		gap: 6px;
	}

	.search-filter-time-kind {
		width: 100%;
		margin: 0 0 6px;
	}

	.search-size-range,
	.search-size-range :deep(.el-input-number) {
		width: 100%;
	}

	.search-filter-actions .el-button {
		flex: 1;
	}

  .workspace-label {
    min-height: auto;
    padding: 0 0 8px;
    border-right: 0;
    border-bottom: 1px solid var(--border-color);
  }

  .upload-queue__header {
    align-items: flex-start;
  }

  .upload-queue__header > div {
    align-items: flex-start;
    flex-direction: column;
    gap: 2px;
  }

  .upload-task {
    grid-template-columns: minmax(0, 1fr);
    gap: 8px;
    padding: 10px 12px;
  }

  .upload-task__actions {
    min-width: 0;
    justify-content: flex-start;
    flex-wrap: wrap;
  }
}

.breadcrumb-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin: 0;
  padding: 8px 12px;
  border: 1px solid var(--panel-border);
  border-radius: 8px;
  background: var(--panel-bg-strong);
}

.breadcrumb-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 4px;
  flex: 0 0 auto;
}

.breadcrumb-button {
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--accent-primary);
  cursor: pointer;
}

.breadcrumb-button:disabled {
  color: var(--text-primary);
  cursor: default;
}

.node-name {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: var(--text-primary);
  font-weight: 650;
  cursor: pointer;
}

.node-name-cell {
	display: flex;
	align-items: center;
	justify-content: center;
	flex-direction: column;
	min-width: 0;
	gap: 3px;
}

.search-node-meta {
	max-width: 100%;
	overflow: hidden;
	color: var(--text-muted);
	font-size: 12px;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.node-name:hover {
  color: var(--accent-primary);
}

.node-name.is-file {
  cursor: default;
}

.files-table {
  min-height: 260px;
  border: 0;
  border-radius: 0 !important;
}

.files-list-panel .table-pagination {
  margin-top: 0;
  padding: 12px 14px;
  border-top: 1px solid var(--border-color);
}

.preview-dialog-header {
  display: flex;
  align-items: center;
  min-width: 0;
  gap: 10px;
}

.preview-dialog-icon {
  display: grid;
  width: 36px;
  height: 36px;
  flex: 0 0 36px;
  place-items: center;
  border: 1px solid color-mix(in srgb, #0d9488 30%, var(--panel-border));
  border-radius: 8px;
  background: color-mix(in srgb, #0d9488 9%, var(--panel-bg));
  color: #0d9488;
}

.preview-dialog-header > div {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 2px;
}

.preview-dialog-header strong,
.preview-dialog-header span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.preview-dialog-header strong {
  color: var(--text-primary);
  font-size: 16px;
}

.preview-dialog-header span {
  color: var(--text-secondary);
  font-size: 12px;
}

.preview-viewport {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: clamp(360px, 66vh, 760px);
  overflow: hidden;
  border: 1px solid var(--panel-border);
  border-radius: 8px;
  background: var(--surface-soft);
}

.preview-image {
  display: block;
  max-width: 100%;
  max-height: 100%;
  object-fit: contain;
}

.preview-pdf {
  display: flex;
  min-width: 0;
  width: 100%;
  height: 100%;
  flex-direction: column;
  background: var(--panel-bg-strong);
}

.preview-pdf-toolbar {
  display: flex;
  min-height: 48px;
  flex: 0 0 48px;
  align-items: center;
  justify-content: space-between;
  padding: 0 16px;
  border-bottom: 1px solid var(--panel-border);
  background: var(--panel-bg-strong);
}

.preview-pdf-controls {
  display: flex;
  align-items: center;
  gap: 8px;
}

.preview-pdf-controls .el-button {
  width: 30px;
  height: 30px;
  margin: 0;
}

.preview-pdf-page,
.preview-pdf-zoom {
  min-width: 52px;
  color: var(--text-secondary);
  font-size: 12px;
  font-variant-numeric: tabular-nums;
  text-align: center;
}

.preview-pdf-stage {
  display: flex;
  min-height: 0;
  flex: 1;
  align-items: flex-start;
  justify-content: center;
  padding: 20px;
  overflow: auto;
  background: #e9edf2;
}

.preview-pdf-canvas {
  display: block;
  max-width: none;
  flex: 0 0 auto;
  background: #fff;
  box-shadow: 0 4px 18px rgb(15 23 42 / 14%);
}

.preview-text {
  width: 100%;
  height: 100%;
  margin: 0;
  padding: 20px 24px;
  overflow: auto;
  background: var(--panel-bg-strong);
  color: var(--text-primary);
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 13px;
  line-height: 1.65;
  text-align: left;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
  tab-size: 2;
}

@media (max-width: 640px) {
  .preview-viewport {
    height: calc(100vh - 156px);
    min-height: 320px;
    border-right: 0;
    border-left: 0;
    border-radius: 0;
  }

  .preview-text {
    padding: 16px;
  }

  .preview-pdf-toolbar {
    padding: 0 10px;
  }

  .preview-pdf-stage {
    padding: 12px;
  }
}

.batch-table {
  min-height: 260px;
}

.batch-mobile-meta {
  display: block;
  margin-top: 4px;
  color: var(--text-secondary);
  font-size: 12px;
}

.files-drop-zone {
  position: relative;
}

.files-drop-zone.is-dragging {
  outline: 2px dashed var(--accent-primary);
  outline-offset: 4px;
}

:deep(.files-table .located-source-row > td.el-table__cell) {
  background: color-mix(in srgb, var(--accent-primary) 10%, var(--panel-bg)) !important;
}

:deep(.files-table .located-source-row .node-name) {
  color: var(--accent-primary);
  font-weight: 700;
}

.drop-overlay {
  position: absolute;
  z-index: 5;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 10px;
  border-radius: 8px;
  background: color-mix(in srgb, var(--panel-bg-strong) 92%, var(--accent-primary));
  color: var(--accent-primary);
  font-weight: 700;
  pointer-events: none;
}

.share-result-field {
  margin-top: 16px;
}

.share-result-field > span {
  display: block;
  margin-bottom: 6px;
  color: var(--text-secondary);
  font-size: 13px;
}

.acl-dialog-body {
  min-height: 220px;
}

.acl-toolbar {
  display: grid;
  grid-template-columns: 120px minmax(220px, 1fr) 110px 120px 100px auto;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.acl-table {
  min-height: 180px;
}

.acl-inheritance-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  margin-top: 18px;
  padding-top: 16px;
  border-top: 1px solid var(--panel-border);
}

.acl-inheritance-row > div {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

:global(.collaboration-drawer .el-drawer__body) {
	min-height: 0;
	padding: 0 20px 20px;
	overflow: hidden;
}

:global(.collaboration-mention-popper) {
	max-width: calc(100vw - 28px);
}

.collaboration-header {
	display: flex;
	align-items: center;
	min-width: 0;
	gap: 12px;
}

.collaboration-header__icon {
	display: grid;
	width: 40px;
	height: 40px;
	flex: 0 0 40px;
	place-items: center;
	border: 1px solid color-mix(in srgb, #7c3aed 28%, var(--panel-border));
	border-radius: 8px;
	background: color-mix(in srgb, #7c3aed 9%, var(--panel-bg));
	color: #7c3aed;
	font-size: 19px;
}

.collaboration-header > div:last-child {
	display: flex;
	min-width: 0;
	flex-direction: column;
	gap: 3px;
}

.collaboration-header h2 {
	margin: 0;
	color: var(--text-primary);
	font-size: 18px;
	letter-spacing: 0;
}

.collaboration-header span {
	overflow: hidden;
	color: var(--text-secondary);
	font-size: 13px;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.collaboration-tabs {
	display: flex;
	height: 100%;
	min-height: 0;
	flex-direction: column;
}

.collaboration-tabs :deep(.el-tabs__header) {
	margin-bottom: 0;
}

.collaboration-tabs :deep(.el-tabs__item strong) {
	margin-left: 4px;
	color: var(--text-muted);
	font-size: 12px;
}

.collaboration-tabs :deep(.el-tabs__content),
.collaboration-tabs :deep(.el-tab-pane) {
	min-height: 0;
	flex: 1;
	height: 100%;
}

.collaboration-tabs :deep(.el-tabs__content) {
	overflow: hidden;
}

.collaboration-pane {
	height: 100%;
	min-height: 0;
	padding-top: 16px;
	overflow-y: auto;
}

.comment-composer {
	padding: 14px;
	border: 1px solid var(--panel-border);
	border-radius: 8px;
	background: var(--panel-bg-strong);
}

.comment-composer :deep(.el-textarea__inner),
.comment-item :deep(.el-textarea__inner) {
	line-height: 1.65;
}

.comment-composer__footer,
.comment-edit-actions {
	display: flex;
	align-items: center;
	justify-content: space-between;
	gap: 12px;
	margin-top: 10px;
}

.comment-composer__footer > span {
	color: var(--text-muted);
	font-size: 12px;
}

.comment-feed {
	min-height: 180px;
	margin-top: 8px;
}

.comment-item {
	display: grid;
	grid-template-columns: 36px minmax(0, 1fr);
	gap: 12px;
	padding: 18px 2px;
	border-bottom: 1px solid var(--panel-border);
}

.comment-avatar,
.activity-item :deep(.el-avatar) {
	background: color-mix(in srgb, #7c3aed 78%, #fff);
	color: #fff;
	font-weight: 700;
}

.comment-item__body {
	min-width: 0;
}

.comment-item__header {
	display: flex;
	align-items: flex-start;
	justify-content: space-between;
	gap: 12px;
}

.comment-item__header > div {
	display: flex;
	min-width: 0;
	align-items: baseline;
	gap: 6px;
}

.comment-item__header strong {
	overflow: hidden;
	color: var(--text-primary);
	font-size: 14px;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.comment-item__header span {
	color: var(--text-muted);
	font-size: 12px;
	white-space: nowrap;
}

.comment-content {
	margin: 9px 0 0;
	color: var(--text-secondary);
	font-size: 14px;
	line-height: 1.7;
	white-space: pre-wrap;
	word-break: break-word;
}

.comment-content .is-mention {
	color: var(--accent-primary);
	font-weight: 650;
}

.comment-actions {
	justify-content: flex-start;
	margin-top: 10px;
}

.comment-actions :deep(.el-button),
.comment-edit-actions :deep(.el-button) {
	height: 28px;
}

.comment-edit-actions {
	justify-content: flex-end;
}

.collaboration-load-more {
	display: flex;
	justify-content: center;
	padding: 16px 0 8px;
}

.collaboration-activity-pane {
	padding-top: 22px;
}

.activity-timeline {
	padding: 2px 4px 0 6px;
}

.activity-timeline :deep(.el-timeline-item__timestamp) {
	color: var(--text-muted);
	font-size: 12px;
}

.activity-item {
	display: flex;
	align-items: center;
	gap: 10px;
	padding: 10px 12px;
	border: 1px solid var(--panel-border);
	border-radius: 8px;
	background: var(--panel-bg-strong);
}

.activity-item > div {
	display: flex;
	min-width: 0;
	flex-direction: column;
	gap: 2px;
}

.activity-item strong {
	overflow: hidden;
	color: var(--text-primary);
	font-size: 13px;
	text-overflow: ellipsis;
	white-space: nowrap;
}

.activity-item span {
	color: var(--text-secondary);
	font-size: 13px;
}

.access-detail-header {
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
}

.access-detail-icon {
  display: grid;
  width: 38px;
  height: 38px;
  flex: 0 0 38px;
  place-items: center;
  border: 1px solid color-mix(in srgb, var(--accent-primary) 28%, var(--panel-border));
  border-radius: 8px;
  background: color-mix(in srgb, var(--accent-primary) 9%, var(--panel-bg));
  color: var(--accent-primary);
  font-size: 18px;
}

.access-detail-header h2,
.access-source-title h3 {
  margin: 0;
  color: var(--text-primary);
  letter-spacing: 0;
}

.access-detail-header h2 {
  font-size: 18px;
}

.access-detail-header span,
.access-source-title span,
.access-source-main span,
.access-source-scope {
  color: var(--text-secondary);
  font-size: 13px;
}

.access-detail-header > div:last-child,
.access-source-main {
  display: flex;
  min-width: 0;
  flex-direction: column;
  gap: 3px;
}

.access-detail-header span,
.access-source-main strong,
.access-source-main span {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.access-detail-body {
  min-height: 260px;
}

.access-level-summary,
.access-source-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.access-level-summary {
  margin-bottom: 16px;
  padding: 14px 16px;
  border: 1px solid var(--panel-border);
  border-radius: 8px;
  background: var(--panel-bg-strong);
  color: var(--text-primary);
  font-weight: 650;
}

.access-source-section {
  margin-top: 24px;
}

.access-source-title {
  margin-bottom: 8px;
}

.access-source-title h3 {
  font-size: 16px;
}

.access-source-row {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 8px 12px;
  padding: 16px 0;
  border-bottom: 1px solid var(--panel-border);
}

.access-source-tags {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 6px;
  flex-wrap: wrap;
}

.access-source-scope {
  grid-column: 1 / -1;
}

@media (max-width: 760px) {
	:global(.collaboration-drawer .el-drawer__body) {
		padding: 0 14px 14px;
	}

	.comment-composer__footer,
	.comment-item__header {
		align-items: flex-start;
		flex-direction: column;
	}

	.comment-composer__footer :deep(.el-button) {
		width: 100%;
	}

  .acl-toolbar {
    grid-template-columns: 1fr 1fr;
  }

  .acl-toolbar > :nth-child(2) {
    grid-column: span 2;
  }

  .acl-inheritance-row {
    align-items: flex-start;
    flex-direction: column;
  }

  .access-source-row {
    grid-template-columns: minmax(0, 1fr);
  }

  .access-source-tags {
    justify-content: flex-start;
  }

  .access-source-scope {
    grid-column: auto;
  }
}
</style>
