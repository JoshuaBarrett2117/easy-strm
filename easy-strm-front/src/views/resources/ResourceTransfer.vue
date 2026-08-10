<template>
  <div class="resource-transfer-page">
    <!-- 预设配置（始终显示，方便用户一次性配置后复用） -->
    <div class="preset-section">
      <h3 class="section-title">转存默认配置</h3>
      <div class="preset-grid">
        <!-- 目标账号选择 -->
        <div class="preset-field">
          <label class="target-label">目标 115 账号</label>
          <n-select
            v-model:value="targetCloud115Id"
            :options="accountOptions"
            placeholder="选择 115 账号"
            style="width:100%"
            @update:value="onAccountChange"
          />
        </div>

        <!-- 默认保存路径 -->
        <div class="preset-field">
          <label class="target-label">默认保存路径</label>
          <TargetFolderPicker
            :cloud-115-id="targetCloud115Id"
            :default-path="targetDirectory"
            :selected-size="selectedSize"
            :available-space="availableSpace"
            @select="onTargetSelect"
          />
        </div>

        <!-- 冲突策略 -->
        <div class="preset-field">
          <label class="target-label">冲突策略</label>
          <n-radio-group v-model:value="conflictStrategy">
            <n-radio value="skip">跳过</n-radio>
            <n-radio value="overwrite">覆盖</n-radio>
            <n-radio value="rename">自动重命名</n-radio>
          </n-radio-group>
        </div>

        <!-- 开关项 -->
        <div class="preset-field preset-toggles">
          <label class="target-label">转存后操作</label>
          <div class="toggle-group">
            <n-switch v-model:value="autoOrganize">
              <template #checked>自动整理</template>
              <template #unchecked>自动整理</template>
            </n-switch>
            <n-switch v-model:value="autoScrape" :disabled="!autoOrganize">
              <template #checked>自动刮削</template>
              <template #unchecked>自动刮削</template>
            </n-switch>
          </div>
        </div>
      </div>
      <div class="save-preset-row">
        <n-button type="primary" size="small" @click="savePresetConfig">
          <template #icon><n-icon :component="SaveOutline" /></template>
          保存配置（下次自动填充）
        </n-button>
        <span v-if="presetSaved" class="preset-saved-hint">配置已保存</span>
      </div>
    </div>

    <!-- Step 1: 分享链接输入 -->
    <ShareLinkInput
      @parsed="onParsed"
      @reset="onReset"
    />

    <!-- Step 2: 文件选择器（解析完成后显示） -->
    <div v-if="parsedData" class="file-section">
      <h3 class="section-title">选择要转存的文件</h3>
      <FileSelector
        :files="parsedData.files || []"
        @selection-change="onSelectionChange"
      />
    </div>

    <!-- Step 3: 目标确认（选中文件后显示） -->
    <div v-if="selectedFiles.length > 0" class="target-section">
      <h3 class="section-title">转存确认</h3>
      <div class="confirm-info">
        <span>目标账号：<strong>{{ accountName }}</strong></span>
        <span>保存路径：<strong>{{ targetDirectory || '（账号默认根目录）' }}</strong></span>
        <span>文件数：<strong>{{ selectedFiles.length }}</strong> 个，共 {{ formatSize(selectedSize) }}</span>
      </div>

      <!-- 提交按钮 -->
      <div class="submit-row">
        <n-button
          type="primary"
          size="large"
          :loading="submitting"
          :disabled="!canSubmit"
          @click="handleSubmit"
        >
          开始转存 ({{ selectedFiles.length }} 个文件, {{ formatSize(selectedSize) }})
        </n-button>
        <div v-if="autoOrganize" class="submit-hint">
          <n-tag type="info" size="small">转存后将自动整理{{ autoScrape ? '+刮削' : '' }}</n-tag>
        </div>
      </div>
    </div>

    <!-- Step 4: 转存进度 -->
    <div v-if="transferTaskId" class="progress-section">
      <h3 class="section-title">转存进度</h3>
      <TransferProgress
        ref="progressRef"
        :task-id="transferTaskId"
        @cancel="handleCancelTransfer"
        @close="handleCloseProgress"
        @retry-all="handleRetryAll"
        @retry-single="handleRetrySingle"
        @go-to-files="handleGoToFiles"
        @restart="handleRestart"
        @status-change="onTransferStatusChange"
      />

      <!-- 转存完成后显示日志摘要 -->
      <div
        v-if="transferResult"
        class="result-summary"
      >
        <n-alert
          :type="transferResult.type"
          :title="transferResult.title"
          :closable="false"
        />
        <!-- 失败文件的错误原因明细 -->
        <div v-if="transferResult.failedItems && transferResult.failedItems.length > 0" class="error-detail">
          <p class="error-detail-title">失败原因：</p>
          <div
            v-for="(item, idx) in transferResult.failedItems"
            :key="idx"
            class="error-item"
          >
            <span class="error-item-name">{{ item.name }}</span>
            <span class="error-item-msg">{{ item.error }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 冲突预览弹窗 -->
    <ConflictPreviewModal
      v-model:visible="showConflictModal"
      :conflicts="conflictFiles"
      @confirm="onConflictConfirm"
      @cancel="onConflictCancel"
    />
  </div>
</template>

<script setup>
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import { useRouter } from 'vue-router'
import {
  NSelect,
  NButton,
  NRadioGroup,
  NRadio,
  NAlert,
  NSwitch,
  NTag,
  NIcon
} from 'naive-ui'
import { SaveOutline } from '@vicons/ionicons5'
import ShareLinkInput from '../../components/resource/ShareLinkInput.vue'
import FileSelector from '../../components/resource/FileSelector.vue'
import TargetFolderPicker from '../../components/resource/TargetFolderPicker.vue'
import TransferProgress from '../../components/resource/TransferProgress.vue'
import ConflictPreviewModal from '../../components/resource/ConflictPreviewModal.vue'
import { submitTransfer, getTransferProgress, cancelTransfer, retryTransfer } from '../../utils/api/resource'
import { getCloud115List } from '../../utils/api/cloud115'
import { message } from '../../utils/ui/feedback'

const router = useRouter()

// ===== 核心状态 =====
const parsedData = ref(null) // ParseShareResponse
const selectedFiles = ref([]) // ShareTransferFileItem[]
const targetCloud115Id = ref(0)
const targetDirectory = ref('')
const conflictStrategy = ref('skip')
const autoOrganize = ref(false)
const autoScrape = ref(false)
const presetSaved = ref(false)
const transferTaskId = ref('')
const submitting = ref(false)

// 账号相关
const accountOptions = ref([])
const availableSpace = ref(0)

// 冲突弹窗
const showConflictModal = ref(false)
const conflictFiles = ref([])

// 进度组件引用
const progressRef = ref(null)

// 转存结果
const transferResult = ref(null)

// ===== 计算属性 =====

const selectedSize = computed(() => {
  return selectedFiles.value.reduce((sum, f) => sum + (f.size || 0), 0)
})

const accountName = computed(() => {
  const acc = accountOptions.value.find(a => a.value === targetCloud115Id.value)
  return acc ? acc.label.replace(/\(ID:.*\)/, '').trim() : '未选择'
})

const canSubmit = computed(() => {
  return (
    targetCloud115Id.value > 0 &&
    selectedFiles.value.length > 0 &&
    !submitting.value
    // targetDirectory 允许为空（表示使用账号根目录）
  )
})

// ===== 方法 =====

const formatSize = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let idx = 0
  let size = bytes
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  return `${size.toFixed(idx > 0 ? 1 : 0)} ${units[idx]}`
}

/** 解析完成回调 */
const onParsed = (data) => {
  parsedData.value = data
  selectedFiles.value = []
  transferTaskId.value = ''
  transferResult.value = null
}

/** 重置 */
const onReset = () => {
  parsedData.value = null
  selectedFiles.value = []
  transferTaskId.value = ''
  transferResult.value = null
  if (progressRef.value) {
    progressRef.value.reset()
  }
}

/** 文件选择变更 */
const onSelectionChange = (files) => {
  selectedFiles.value = files
}

/** 账号切换 */
const onAccountChange = (id) => {
  const acc = accountOptions.value.find(a => a.value === id)
  if (acc) {
    targetDirectory.value = acc.transferDirectory || ''
  }
}

/** 目标目录选择 */
const onTargetSelect = (path) => {
  targetDirectory.value = path
}

/** 提交转存 */
const handleSubmit = async () => {
  if (!canSubmit.value) return

  submitting.value = true
  try {
    const reqData = {
      share_code: parsedData.value.share_code,
      password: '',
      target_cloud115_id: targetCloud115Id.value,
      target_directory: targetDirectory.value,
      files: selectedFiles.value,
      conflict_strategy: conflictStrategy.value,
      auto_organize: autoOrganize.value,
      auto_scrape: autoScrape.value && autoOrganize.value
    }

    const response = await submitTransfer(reqData)
    const result = response.data?.data || response.data
    transferTaskId.value = result.task_id

    // 启动进度轮询
    await nextTick()
    if (progressRef.value) {
      progressRef.value.startPolling(getTransferProgress)
    }
    message.success('转存任务已提交')
  } catch (err) {
    const errMsg = err?.response?.data?.error || '提交转存失败'
    message.error(errMsg)
  }
  submitting.value = false
}

/** 取消转存 */
const handleCancelTransfer = async () => {
  if (!transferTaskId.value) return
  try {
    await cancelTransfer(transferTaskId.value)
    message.info('正在取消转存...')
  } catch (err) {
    message.error('取消失败')
  }
}

/** 关闭进度 */
const handleCloseProgress = () => {
  transferTaskId.value = ''
  transferResult.value = null
  if (progressRef.value) {
    progressRef.value.reset()
  }
}

/** 重试全部 */
const handleRetryAll = async () => {
  if (!transferTaskId.value) return
  try {
    const response = await retryTransfer(transferTaskId.value)
    const result = response.data?.data || response.data
    if (result.retried_files > 0) {
      message.success(`已重新提交 ${result.retried_files} 个文件`)
      // 重新开始轮询
      await nextTick()
      if (progressRef.value) {
        progressRef.value.startPolling(getTransferProgress)
      }
    } else {
      message.warning('没有可重试的文件')
    }
  } catch (err) {
    message.error('重试失败')
  }
}

/** 重试单个文件 */
const handleRetrySingle = async (item) => {
  if (!transferTaskId.value) return
  try {
    const response = await retryTransfer(transferTaskId.value)
    const result = response.data?.data || response.data
    if (result.retried_files > 0) {
      message.success(`已重新提交 ${item.name}`)
      await nextTick()
      if (progressRef.value) {
        progressRef.value.startPolling(getTransferProgress)
      }
    }
  } catch (err) {
    message.error(`重试 ${item.name} 失败`)
  }
}

/** 去文件工作台 */
const handleGoToFiles = () => {
  router.push('/dashboard/media-manager')
}

/** 重新开始 */
const handleRestart = () => {
  onReset()
}

/** 转存状态变化 */
const onTransferStatusChange = (status) => {
  const statusMap = {
    completed: { type: 'success', title: '✅ 所有文件转存完成' },
    partial_failed: { type: 'warning', title: '⚠️ 部分文件转存失败，请查看详情' },
    failed: { type: 'error', title: '❌ 全部文件转存失败' },
    cancelled: { type: 'info', title: '🛑 转存已取消' }
  }
  const result = statusMap[status] || null
  // 从进度组件读取失败文件明细
  if (result && progressRef.value) {
    const pd = progressRef.value.progressData
    if (pd?.failed_items?.length > 0) {
      result.failedItems = pd.failed_items
    }
  }
  transferResult.value = result
}

/** 冲突处理确认 */
const onConflictConfirm = (strategy) => {
  conflictStrategy.value = strategy
  showConflictModal.value = false
}

/** 冲突取消 */
const onConflictCancel = () => {
  showConflictModal.value = false
}

// ===== 初始化：加载账号列表 + 恢复保存的配置 =====
const PRESET_STORAGE_KEY = 'easy-strm-115-transfer-preset'

/** 保存预设配置到 localStorage */
const savePresetConfig = () => {
  try {
    const preset = {
      targetCloud115Id: targetCloud115Id.value,
      targetDirectory: targetDirectory.value,
      conflictStrategy: conflictStrategy.value,
      autoOrganize: autoOrganize.value,
      autoScrape: autoScrape.value,
      savedAt: new Date().toISOString()
    }
    localStorage.setItem(PRESET_STORAGE_KEY, JSON.stringify(preset))
    presetSaved.value = true
    setTimeout(() => { presetSaved.value = false }, 2000)
  } catch (_e) {
    // localStorage 不可用时静默失败
  }
}

/** 从 localStorage 恢复预设配置 */
const loadPresetConfig = () => {
  try {
    const raw = localStorage.getItem(PRESET_STORAGE_KEY)
    if (!raw) return
    const preset = JSON.parse(raw)
    if (preset.targetCloud115Id) targetCloud115Id.value = preset.targetCloud115Id
    if (preset.targetDirectory !== undefined) targetDirectory.value = preset.targetDirectory
    if (preset.conflictStrategy) conflictStrategy.value = preset.conflictStrategy
    if (preset.autoOrganize !== undefined) autoOrganize.value = preset.autoOrganize
    if (preset.autoScrape !== undefined) autoScrape.value = preset.autoScrape
  } catch (_e) {
    // 解析失败时忽略
  }
}

const loadAccounts = async () => {
  try {
    const response = await getCloud115List({ status: 'active' })
    const accounts = response.data?.data || response.data || []
    if (Array.isArray(accounts)) {
      accountOptions.value = accounts.map(a => ({
        label: `${a.name} (ID: ${a.id})`,
        value: a.id,
        transferDirectory: a.transfer_directory || ''
      }))
    }
  } catch (err) {
    // 账号加载失败静默处理
  }
}

loadAccounts()
loadPresetConfig()
</script>

<style scoped>
.resource-transfer-page {
  display: flex;
  flex-direction: column;
  gap: 24px;
}

.section-title {
  font-size: 15px;
  font-weight: 700;
  margin: 0 0 12px;
  color: var(--text-primary, #1f2937);
}

/* 预设配置区 */
.preset-section {
  padding: 18px 20px;
  border: 1px solid solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  background: var(--card-bg, #fff);
}

.preset-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 16px;
}

.preset-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.preset-toggles {
  align-items: flex-end;
}

.toggle-group {
  display: flex;
  gap: 16px;
  align-items: center;
}

.save-preset-row {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 10px;
}

.preset-saved-hint {
  font-size: 12px;
  color: #10b981;
  font-weight: 600;
}

.file-section,
.target-section,
.progress-section {
  padding: 18px 20px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  background: var(--card-bg, #fff);
}

.target-row,
.strategy-row {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 14px;
}

.target-label {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-secondary, #6b7280);
  min-width: 80px;
}

.submit-row {
  margin-top: 18px;
}

.result-summary {
  margin-top: 16px;
}

.error-detail {
  margin-top: 10px;
  padding: 12px;
  border-radius: 8px;
  background: #fef2f2;
  border: 1px solid #fecaca;
}

.error-detail-title {
  font-size: 13px;
  font-weight: 700;
  color: #991b1b;
  margin: 0 0 8px;
}

.error-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 6px 8px;
  margin-bottom: 4px;
  border-radius: 6px;
  background: #fff;
  font-size: 12px;
}

.error-item-name {
  font-weight: 600;
  color: #374151;
  word-break: break-all;
}

.error-item-msg {
  color: #dc2626;
  word-break: break-all;
}

.confirm-info {
  display: flex;
  flex-wrap: wrap;
  gap: 12px 24px;
  font-size: 13px;
  color: var(--text-secondary, #6b7280);
  margin-bottom: 14px;
}

.confirm-info strong {
  color: var(--text-primary, #1f2937);
}

.submit-hint {
  margin-top: 8px;
}
</style>
