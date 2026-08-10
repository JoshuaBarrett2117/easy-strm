<template>
  <div v-if="!visible" style="display:none"></div>
  <div v-else class="transfer-progress">
    <!-- queued: 排队中 -->
    <div v-if="progressData.status === 'queued'" class="state-card queued">
      <div class="state-header">
        <n-icon size="20" :component="TimerOutline" />
        <span class="state-title">任务排队中</span>
      </div>
      <div class="state-body">
        <div class="stat-row">
          <span class="stat-label">任务ID</span>
          <span class="stat-value mono">{{ progressData.task_id }}</span>
        </div>
        <div class="stat-row">
          <span class="stat-label">文件数</span>
          <span class="stat-value">{{ progressData.total_files }} 个</span>
        </div>
      </div>
    </div>

    <!-- transferring: 转存中 -->
    <div v-else-if="progressData.status === 'transferring'" class="state-card transferring">
      <div class="state-header">
        <n-icon size="20" :component="SyncOutline" class="spin-icon" />
        <span class="state-title">正在转存...</span>
      </div>
      <div class="state-body">
        <n-progress
          type="line"
          :percentage="progressData.progress || 0"
          :height="18"
          :border-radius="4"
          indicator-placement="outside"
          :processing="true"
        />
        <div class="stat-grid">
          <div class="stat-item success">
            <span class="stat-num">{{ progressData.success_files }}</span>
            <span class="stat-label">成功</span>
          </div>
          <div class="stat-item failed">
            <span class="stat-num">{{ progressData.failed_files }}</span>
            <span class="stat-label">失败</span>
          </div>
          <div class="stat-item skipped">
            <span class="stat-num">{{ progressData.skipped_files }}</span>
            <span class="stat-label">跳过</span>
          </div>
          <div class="stat-item total">
            <span class="stat-num">{{ progressData.total_files }}</span>
            <span class="stat-label">总计</span>
          </div>
        </div>
        <div v-if="progressData.current_file" class="current-file">
          <span class="cf-label">当前文件：</span>
          <span class="cf-name">{{ progressData.current_file }}</span>
        </div>
        <div v-if="progressData.estimated_remaining" class="estimate">
          预计剩余：{{ progressData.estimated_remaining }}
        </div>
        <div class="action-row">
          <n-button type="error" secondary size="small" @click="$emit('cancel')">
            取消转存
          </n-button>
        </div>
      </div>
    </div>

    <!-- completed: 全部成功 -->
    <div v-else-if="progressData.status === 'completed'" class="state-card completed">
      <div class="state-header">
        <n-icon size="20" :component="CheckmarkCircleOutline" color="#059669" />
        <span class="state-title">转存完成</span>
      </div>
      <div class="state-body">
        <p class="complete-msg">
          ✅ 全部 {{ progressData.success_files }} 个文件已成功转存
        </p>
        <div class="action-row">
          <n-button type="primary" size="small" @click="$emit('go-to-files')">
            去文件工作台
          </n-button>
          <n-button size="small" @click="$emit('close')">
            关闭
          </n-button>
        </div>
      </div>
    </div>

    <!-- partial_failed: 部分失败 -->
    <div v-else-if="progressData.status === 'partial_failed'" class="state-card partial-failed">
      <div class="state-header">
        <n-icon size="20" :component="AlertCircleOutline" color="#d97706" />
        <span class="state-title">部分转存失败</span>
      </div>
      <div class="state-body">
        <p class="partial-msg">
          ⚠️ {{ progressData.success_files }} 成功，{{ progressData.failed_files }} 失败
        </p>
        <div v-if="progressData.failed_items && progressData.failed_items.length > 0" class="failed-list">
          <div
            v-for="(item, idx) in progressData.failed_items"
            :key="idx"
            class="failed-item"
          >
            <div class="fi-info">
              <span class="fi-name">{{ item.name }}</span>
              <span class="fi-error">{{ item.error }}</span>
            </div>
            <n-button
              v-if="item.retryable"
              size="tiny"
              type="warning"
              secondary
              @click="$emit('retry-single', item)"
            >
              重试
            </n-button>
          </div>
        </div>
        <div class="action-row">
          <n-button type="warning" secondary size="small" @click="$emit('retry-all')">
            重试全部
          </n-button>
          <n-button size="small" @click="$emit('close')">
            关闭
          </n-button>
        </div>
      </div>
    </div>

    <!-- failed: 全部失败 -->
    <div v-else-if="progressData.status === 'failed'" class="state-card failed">
      <div class="state-header">
        <n-icon size="20" :component="CloseCircleOutline" color="#dc2626" />
        <span class="state-title">转存失败</span>
      </div>
      <div class="state-body">
        <n-alert type="error" :closable="false" class="failed-alert">
          ❌ 全部 {{ progressData.total_files }} 个文件转存失败
        </n-alert>
        <div v-if="progressData.failed_items && progressData.failed_items.length > 0" class="failed-list">
          <div
            v-for="(item, idx) in progressData.failed_items"
            :key="idx"
            class="failed-item"
          >
            <span class="fi-name">{{ item.name }}</span>
            <span class="fi-error">{{ item.error }}</span>
          </div>
        </div>
        <div class="action-row">
          <n-button type="error" secondary size="small" @click="$emit('retry-all')">
            重试
          </n-button>
          <n-button size="small" @click="$emit('close')">
            关闭
          </n-button>
        </div>
      </div>
    </div>

    <!-- cancelled: 已取消 -->
    <div v-else-if="progressData.status === 'cancelled'" class="state-card cancelled">
      <div class="state-header">
        <n-icon size="20" :component="BanOutline" color="#6b7280" />
        <span class="state-title">已取消</span>
      </div>
      <div class="state-body">
        <p class="cancelled-msg">
          🛑 转存已取消，已完成 {{ progressData.processed_files || progressData.success_files || 0 }} 个文件
        </p>
        <div class="action-row">
          <n-button type="primary" size="small" @click="$emit('restart')">
            重新开始
          </n-button>
          <n-button size="small" @click="$emit('close')">
            关闭
          </n-button>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onUnmounted } from 'vue'
import { NIcon, NButton, NProgress, NAlert } from 'naive-ui'
import {
  TimerOutline,
  SyncOutline,
  CheckmarkCircleOutline,
  AlertCircleOutline,
  CloseCircleOutline,
  BanOutline
} from '@vicons/ionicons5'

const props = defineProps({
  taskId: {
    type: String,
    default: ''
  }
})

const emit = defineEmits([
  'cancel', 'close', 'retry-all', 'retry-single',
  'go-to-files', 'restart', 'status-change'
])

const visible = ref(false)
const progressData = ref({
  task_id: '',
  status: 'queued',
  progress: 0,
  total_files: 0,
  processed_files: 0,
  success_files: 0,
  failed_files: 0,
  skipped_files: 0,
  failed_items: [],
  current_file: '',
  estimated_remaining: '',
  create_time: '',
  update_time: ''
})

let pollTimer = null

/** 开始轮询 */
const startPolling = async (getProgressFn) => {
  visible.value = true
  if (pollTimer) clearInterval(pollTimer)

  const poll = async () => {
    try {
      const response = await getProgressFn(props.taskId)
      const data = response.data?.data || response.data
      if (data) {
        progressData.value = { ...progressData.value, ...data }
      }

      // 检测终态
      const terminalStatuses = ['completed', 'partial_failed', 'failed', 'cancelled']
      if (terminalStatuses.includes(progressData.value.status)) {
        stopPolling()
        emit('status-change', progressData.value.status)
      }
    } catch (err) {
      // 轮询错误静默处理
    }
  }

  // 立即执行一次
  await poll()
  // 每2s轮询
  pollTimer = setInterval(poll, 2000)
}

/** 停止轮询 */
const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

/** 重置状态 */
const reset = () => {
  stopPolling()
  visible.value = false
  progressData.value = {
    task_id: '',
    status: 'queued',
    progress: 0,
    total_files: 0,
    processed_files: 0,
    success_files: 0,
    failed_files: 0,
    skipped_files: 0,
    failed_items: [],
    current_file: '',
    estimated_remaining: '',
    create_time: '',
    update_time: ''
  }
}

// 暴露方法给父组件
defineExpose({ startPolling, stopPolling, reset, progressData })

onUnmounted(() => {
  stopPolling()
})
</script>

<style scoped>
.transfer-progress {
  max-width: 720px;
}

.state-card {
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  padding: 18px 20px;
  background: var(--card-bg, #fff);
}

.state-card.queued { border-color: #93c5fd66; background: #eff6ff; }
.state-card.transferring { border-color: #fbbf2466; background: #fffbeb; }
.state-card.completed { border-color: #6ee7b766; background: #f0fdf4; }
.state-card.partial-failed { border-color: #fcd34d66; background: #fffbeb; }
.state-card.failed { border-color: #fca5a566; background: #fef2f2; }
.state-card.cancelled { border-color: #d1d5db66; background: #f9fafb; }

.state-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
}

.state-title {
  font-size: 15px;
  font-weight: 700;
}

.state-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.stat-row {
  display: flex;
  justify-content: space-between;
  font-size: 13px;
}

.stat-label {
  color: var(--text-secondary, #6b7280);
}

.stat-value {
  font-weight: 600;
}

.stat-value.mono {
  font-family: 'JetBrains Mono', 'Fira Code', monospace;
  font-size: 12px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
}

.stat-item {
  text-align: center;
  padding: 10px 8px;
  border-radius: 10px;
  background: rgba(255,255,255,0.7);
  border: 1px solid var(--border-color, #e5e7eb);
}

.stat-item.success { background: #f0fdf4; border-color: #bbf7d0; }
.stat-item.failed { background: #fef2f2; border-color: #fecaca; }
.stat-item.skipped { background: #fffbeb; border-color: #fde68a; }
.stat-item.total { background: #eff6ff; border-color: #bfdbfe; }

.stat-num {
  display: block;
  font-size: 20px;
  font-weight: 800;
  line-height: 1.2;
}

.stat-item.success .stat-num { color: #059669; }
.stat-item.failed .stat-num { color: #dc2626; }
.stat-item.skipped .stat-num { color: #d97706; }
.stat-item.total .stat-num { color: #2563eb; }

.stat-label {
  display: block;
  font-size: 11px;
  color: var(--text-secondary, #6b7280);
  margin-top: 2px;
}

.current-file {
  font-size: 13px;
  color: var(--text-secondary, #6b7280);
}

.cf-label { font-weight: 500; }
.cf-name { color: var(--text-primary, #374151); }

.estimate {
  font-size: 12px;
  color: var(--text-tertiary, #9ca3af);
}

.action-row {
  display: flex;
  gap: 8px;
  margin-top: 4px;
}

.complete-msg, .partial-msg, .cancelled-msg {
  margin: 0;
  font-size: 14px;
}

.failed-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 200px;
  overflow-y: auto;
}

.failed-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 8px 12px;
  background: rgba(255,255,255,0.8);
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 8px;
  font-size: 13px;
}

.fi-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.fi-name {
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.fi-error {
  color: #dc2626;
  font-size: 12px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.failed-alert {
  margin: 0;
}

.spin-icon {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}
</style>
