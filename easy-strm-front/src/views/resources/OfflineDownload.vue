<template>
  <div class="offline-download-page">
    <!-- 提交区 -->
    <div class="submit-section">
      <h3 class="section-title">新建云下载任务</h3>
      <div class="submit-grid">
        <!-- 账号选择 -->
        <div class="field">
          <label class="field-label">115 账号</label>
          <n-select
            v-model:value="cloud115Id"
            :options="accountOptions"
            placeholder="选择执行云下载的账号"
            style="width:100%"
            @update:value="directory = ''"
          />
        </div>

        <!-- 保存目录 -->
        <div class="field">
          <label class="field-label">保存目录</label>
          <TargetFolderPicker
            :cloud-115-id="cloud115Id"
            :default-path="directory"
            placeholder="请选择保存目录，留空默认保存到 /云下载"
            class="folder-picker"
            @update:path="directory = $event"
          />
        </div>
      </div>

      <!-- 链接输入 -->
      <div class="field">
        <label class="field-label">下载链接（每行一个，支持 ed2k、磁力、http/https、ftp）</label>
        <n-input
          v-model:value="urlText"
          type="textarea"
          :autosize="{ minRows: 5, maxRows: 12 }"
          placeholder="ed2k://|file|xxx.mkv|123456|HASH|/&#10;magnet:?xt=urn:btih:xxxx&#10;https://example.com/file.zip"
        />
        <div class="url-hint">
          已识别 {{ parsedUrlCount }} 个链接（超过 100 个时由后台队列分批提交，自动去重）
        </div>
      </div>

      <div class="submit-row">
        <n-button
          type="primary"
          :loading="submitting"
          :disabled="!canSubmit"
          @click="handleSubmit"
        >
          提交云下载 ({{ parsedUrlCount }})
        </n-button>
      </div>

      <!-- 提交结果反馈：逐链接受理情况 -->
      <div v-if="submitResults.length > 0" class="submit-results">
        <n-alert
          :type="submitRejectedCount > 0 ? 'warning' : 'success'"
          :closable="false"
        >
          已受理 {{ submitResults.length - submitRejectedCount }} / {{ submitResults.length }} 个链接，
          任务ID：{{ lastTaskId || '—' }}
        </n-alert>
        <div v-if="submitRejectedCount > 0" class="rejected-list">
          <div
            v-for="item in submitResults.filter(r => !r.accepted)"
            :key="item.url"
            class="rejected-item"
          >
            <span class="rejected-url">{{ item.url }}</span>
            <span class="rejected-msg">{{ item.message || '未受理' }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 记录列表 -->
    <div class="records-section">
      <div class="records-header">
        <h3 class="section-title records-title">云下载记录</h3>
        <div class="records-toolbar">
          <n-select
            v-model:value="filterStatus"
            :options="statusFilterOptions"
            size="small"
            style="width:130px"
            @update:value="loadRecords"
          />
          <n-button size="small" secondary :loading="loadingRecords" @click="loadRecords">
            刷新
          </n-button>
        </div>
      </div>

      <n-data-table
        :columns="columns"
        :data="records"
        :loading="loadingRecords"
        :row-key="rowKey"
        :bordered="false"
        size="small"
      />

      <div class="pagination-row">
        <n-pagination
          v-model:page="page"
          :page-size="pageSize"
          :item-count="total"
          show-size-picker
          :page-sizes="[20, 50, 100]"
          @update:page="loadRecords"
          @update:page-size="onPageSizeChange"
        />
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, h, onMounted, onBeforeUnmount } from 'vue'
import {
  NSelect,
  NButton,
  NInput,
  NDataTable,
  NAlert,
  NTag,
  NProgress,
  NPagination,
  NEllipsis
} from 'naive-ui'
import TargetFolderPicker from '../../components/resource/TargetFolderPicker.vue'
import { getCloud115List } from '../../utils/api/cloud115'
import {
  submitOfflineDownload,
  getOfflineDownloadTasks,
  deleteOfflineDownloadTask
} from '../../utils/api/offline'
import { message } from '../../utils/ui/feedback'
import { showConfirmDialog } from '../../utils/ui/messageBox'

// ===== 状态 =====
const cloud115Id = ref(0)
const directory = ref('')
const urlText = ref('')
const submitting = ref(false)
const accountOptions = ref([])

const submitResults = ref([])
const lastTaskId = ref('')
const awaitingQueuedRecords = ref(false)

const records = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const filterStatus = ref('')
const loadingRecords = ref(false)

let pollTimer = null

// ===== 派生状态 =====
const parsedUrls = computed(() => {
  const seen = new Set()
  const urls = []
  urlText.value.split(/\r?\n/).forEach((line) => {
    const url = line.trim()
    if (url && !seen.has(url)) {
      seen.add(url)
      urls.push(url)
    }
  })
  return urls
})

const parsedUrlCount = computed(() => parsedUrls.value.length)

const canSubmit = computed(() => {
  return cloud115Id.value > 0 && parsedUrlCount.value > 0 && !submitting.value
})

const submitRejectedCount = computed(() => submitResults.value.filter(r => !r.accepted).length)

const statusMeta = {
  pending: { label: '等待下载', type: 'default' },
  downloading: { label: '下载中', type: 'info' },
  completed: { label: '已完成', type: 'success' },
  failed: { label: '失败', type: 'error' },
  cancelled: { label: '已取消', type: 'warning' },
  removed: { label: '已移除', type: 'default' }
}

const statusFilterOptions = [
  { label: '全部状态', value: '' },
  { label: '等待下载', value: 'pending' },
  { label: '下载中', value: 'downloading' },
  { label: '已完成', value: 'completed' },
  { label: '失败', value: 'failed' },
  { label: '已取消', value: 'cancelled' },
  { label: '已移除', value: 'removed' }
]

// ===== 表格列 =====
const rowKey = (row) => row.id

const columns = [
  {
    title: '名称 / 链接',
    key: 'name',
    minWidth: 220,
    render(row) {
      return h('div', { class: 'cell-name' }, [
        h(NEllipsis, { class: 'cell-name-title' }, { default: () => row.name || '(未命名)' }),
        h(NEllipsis, { class: 'cell-name-url' }, { default: () => row.url })
      ])
    }
  },
  {
    title: '账号',
    key: 'account_name',
    width: 130,
    render(row) {
      return h(NEllipsis, null, { default: () => row.account_name || '—' })
    }
  },
  {
    title: '状态',
    key: 'status',
    width: 110,
    render(row) {
      const meta = statusMeta[row.status] || { label: row.status, type: 'default' }
      return h(NTag, { size: 'small', type: meta.type, round: true }, { default: () => meta.label })
    }
  },
  {
    title: '进度',
    key: 'percent',
    width: 150,
    render(row) {
      const percent = Math.min(100, Math.max(0, Number(row.percent) || 0))
      const status = row.status === 'completed'
        ? 'success'
        : (row.status === 'failed' ? 'error' : 'default')
      return h(NProgress, {
        type: 'line',
        percentage: Math.round(percent * 10) / 10,
        status,
        indicatorPlacement: 'inside',
        height: 16
      })
    }
  },
  {
    title: '大小',
    key: 'size',
    width: 90,
    render(row) {
      return formatSize(row.size)
    }
  },
  {
    title: '更新时间',
    key: 'update_time',
    width: 150,
    render(row) {
      return formatTime(row.update_time)
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 90,
    render(row) {
      return h(
        NButton,
        {
          size: 'tiny',
          quaternary: true,
          type: 'error',
          onClick: () => handleDelete(row)
        },
        { default: () => '删除' }
      )
    }
  }
]

// ===== 行为 =====
const formatSize = (bytes) => {
  if (!bytes || bytes <= 0) return '—'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let idx = 0
  let size = Number(bytes)
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  return `${size.toFixed(idx > 0 ? 1 : 0)} ${units[idx]}`
}

const formatTime = (raw) => {
  if (!raw) return '—'
  const normalized = String(raw).replace(' ', 'T')
  const date = new Date(normalized.includes('T') ? normalized : `${raw}T00:00:00`)
  if (Number.isNaN(date.getTime())) return String(raw)
  const pad = (n) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`
}

/** 提交云下载 */
const handleSubmit = async () => {
  if (!canSubmit.value) return

  submitting.value = true
  submitResults.value = []
  try {
    const response = await submitOfflineDownload({
      cloud115_id: cloud115Id.value,
      directory: directory.value,
      urls: parsedUrls.value
    })
    const result = response.data?.data || response.data || {}
    lastTaskId.value = result.task_id || ''
    submitResults.value = result.results || []
    if (result.queued) {
      awaitingQueuedRecords.value = true
      const invalidCount = Number(result.rejected) || 0
      const queuedMessage = `已加入后台队列，将分批提交 ${result.queued_count ?? parsedUrlCount.value} 个云下载链接`
      if (invalidCount > 0) {
        message.warning(`${queuedMessage}，另有 ${invalidCount} 个链接格式无效`)
      } else {
        message.success(queuedMessage)
      }
    } else if (submitRejectedCount.value === 0) {
      message.success(`已提交 ${result.accepted ?? parsedUrlCount.value} 个云下载任务`)
    } else {
      message.warning(`部分链接未受理（${submitRejectedCount.value} 个），请查看明细`)
    }
    urlText.value = ''
    page.value = 1
    await loadRecords()
    startPolling()
  } catch (err) {
    message.error(err?.response?.data?.error || '提交云下载失败')
  }
  submitting.value = false
}

/** 加载记录列表 */
const loadRecords = async () => {
  loadingRecords.value = true
  try {
    const params = { page: page.value, page_size: pageSize.value }
    if (filterStatus.value) params.status = filterStatus.value
    const response = await getOfflineDownloadTasks(params)
    const result = response.data?.data || response.data || {}
    records.value = result.data || []
    total.value = result.total || 0
    if (awaitingQueuedRecords.value && records.value.some(r => r.task_id === lastTaskId.value)) {
      awaitingQueuedRecords.value = false
    }
    adjustPolling()
  } catch (err) {
    message.error('加载云下载记录失败')
  }
  loadingRecords.value = false
}

const onPageSizeChange = (size) => {
  pageSize.value = size
  page.value = 1
  loadRecords()
}

/** 删除记录（含115侧离线任务） */
const handleDelete = (row) => {
  showConfirmDialog(
    `确定删除「${row.name || row.url}」吗？将同时移除 115 侧离线任务。`,
    '删除云下载记录',
    { confirmButtonText: '删除' }
  ).then(async () => {
    try {
      await deleteOfflineDownloadTask(row.id, false)
      message.success('已删除')
      await loadRecords()
    } catch (err) {
      message.error(err?.response?.data?.error || '删除失败')
    }
  }).catch(() => {
    // 用户取消删除
  })
}

// ===== 轮询：存在进行中任务时每 3 秒刷新 =====
const hasActiveRecords = () => {
  return awaitingQueuedRecords.value || records.value.some(r => r.status === 'pending' || r.status === 'downloading')
}

const adjustPolling = () => {
  if (hasActiveRecords() && !pollTimer) {
    pollTimer = setInterval(loadRecords, 3000)
  } else if (!hasActiveRecords() && pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

const startPolling = () => {
  if (pollTimer) return
  pollTimer = setInterval(loadRecords, 3000)
}

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})

// ===== 初始化 =====
const loadAccounts = async () => {
  try {
    const response = await getCloud115List({ status: 'active' })
    const accounts = response.data?.data || response.data || []
    if (Array.isArray(accounts)) {
      accountOptions.value = accounts.map(a => ({
        label: `${a.name} (ID: ${a.id})`,
        value: a.id
      }))
      if (!cloud115Id.value && accountOptions.value.length > 0) {
        cloud115Id.value = accountOptions.value[0].value
      }
    }
  } catch (_e) {
    // 账号加载失败静默处理
  }
}

onMounted(() => {
  loadAccounts()
  loadRecords()
})
</script>

<style scoped>
.offline-download-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.section-title {
  font-size: 15px;
  font-weight: 700;
  margin: 0 0 12px;
  color: var(--text-primary, #1f2937);
}

.submit-section,
.records-section {
  padding: 18px 20px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  background: var(--card-bg, #fff);
}

.submit-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: 16px;
  margin-bottom: 14px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.field-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary, #6b7280);
}

.folder-picker {
  margin-top: 6px;
  margin-bottom: 0;
}

.url-hint {
  font-size: 12px;
  color: var(--text-secondary, #6b7280);
}

.submit-row {
  margin-top: 14px;
}

.submit-results {
  margin-top: 14px;
}

.rejected-list {
  margin-top: 8px;
  padding: 10px 12px;
  border: 1px solid #fecaca;
  border-radius: 8px;
  background: #fef2f2;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.rejected-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  font-size: 12px;
}

.rejected-url {
  font-weight: 600;
  color: #374151;
  word-break: break-all;
}

.rejected-msg {
  color: #dc2626;
}

.records-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.records-title {
  margin-bottom: 0;
}

.records-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
}

.pagination-row {
  margin-top: 12px;
  display: flex;
  justify-content: flex-end;
}

:deep(.cell-name) {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

:deep(.cell-name-title) {
  font-weight: 600;
  max-width: 100%;
}

:deep(.cell-name-url) {
  font-size: 12px;
  color: var(--text-secondary, #6b7280);
  max-width: 100%;
}
</style>
