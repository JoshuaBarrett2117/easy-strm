<template>
  <div class="share-link-input">
    <!-- default: 输入框 + 解析按钮 -->
    <div v-if="state === 'default'" class="input-row">
      <n-input-group>
        <n-input
          ref="inputRef"
          v-model:value="urlValue"
          placeholder="粘贴115分享链接，如 https://115.com/s/swz5jtn33fw?password=abc123"
          clearable
          size="large"
          :disabled="false"
          @paste="onPaste"
          @keyup.enter="handleParse"
        />
        <n-button
          type="primary"
          size="large"
          :loading="false"
          :disabled="!urlValue.trim()"
          @click="handleParse"
        >
          解析
        </n-button>
      </n-input-group>
    </div>

    <!-- parsing: 解析进度 -->
    <div v-else-if="state === 'parsing'" class="parsing-state">
      <n-progress
        type="line"
        :percentage="parseProgress"
        :indicator-placement="'inside'"
        :height="20"
        :processing="true"
        :border-radius="4"
      />
      <p class="parsing-hint">正在解析分享链接，请稍候...</p>
      <n-button text type="primary" size="small" @click="handleCancelParse">
        取消
      </n-button>
    </div>

    <!-- parsed: 显示解析结果 -->
    <div v-else-if="state === 'parsed' && parsedData" class="parsed-state">
      <div class="parsed-card">
        <div class="parsed-header">
          <span class="parsed-icon">✅</span>
          <span class="parsed-title">解析完成</span>
        </div>
        <div class="parsed-info">
          <span class="info-item">{{ parsedData.folder_name || '未命名分享' }}</span>
          <span class="info-separator">·</span>
          <span class="info-item">{{ parsedData.total_files }} 个文件</span>
          <span class="info-separator">·</span>
          <span class="info-item">{{ formatSize(parsedData.total_size) }}</span>
        </div>
        <div class="parsed-actions">
          <n-button size="small" secondary @click="handleReset">
            重新解析
          </n-button>
          <n-button size="small" text @click="handleClear">
            清空
          </n-button>
        </div>
      </div>
    </div>

    <!-- error: 错误状态 -->
    <div v-else-if="state === 'error'" class="error-state">
      <n-alert type="error" :title="errorMessage" :closable="false">
        <template #header>
          <span class="error-title">解析失败</span>
        </template>
        <div class="error-actions">
          <n-button size="small" type="error" secondary @click="handleParse">
            重试
          </n-button>
          <n-button size="small" text @click="handleReset">
            重新输入
          </n-button>
        </div>
      </n-alert>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onUnmounted, nextTick } from 'vue'
import {
  NInput,
  NInputGroup,
  NButton,
  NProgress,
  NAlert
} from 'naive-ui'
import { parseShareLink } from '../../utils/api/resource'

const emit = defineEmits(['parsed', 'reset'])

// ===== 状态 =====
const state = ref('default') // default | parsing | parsed | error
const urlValue = ref('')
const password = ref('')
const errorMessage = ref('')
const parsedData = ref(null)
const inputRef = ref(null)

// 解析进度动画（模拟）
const parseProgress = ref(0)
let progressTimer = null
let parseTimeout = null

// ===== 公共方法 =====

/** 格式化文件大小 */
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

/** 从URL中提取密码（URL query参数） */
const extractPassword = (url) => {
  if (!url) return ''
  const match = url.match(/[?&]password=([^&\s#]+)/i)
  return match ? decodeURIComponent(match[1]) : ''
}

/** 开始解析 */
const handleParse = async () => {
  const url = urlValue.value.trim()
  if (!url) return

  // 验证是否为115分享链接（兼容 115.com 和 115cdn.com）
  const shareCodeMatch = url.match(/(?:115cdn\.com|115\.com)\/s\/(\w+)/)
  if (!shareCodeMatch) {
    state.value = 'error'
    errorMessage.value = '无效的115分享链接，请确认链接格式正确（支持 115.com 和 115cdn.com）'
    return
  }

  // 提取密码
  password.value = extractPassword(url)

  // 进入解析状态
  state.value = 'parsing'
  errorMessage.value = ''
  parseProgress.value = 0

  // 启动进度动画（模拟30s超时）
  startProgressAnimation()

  // 设置30s超时
  parseTimeout = setTimeout(() => {
    if (state.value === 'parsing') {
      stopProgressAnimation()
      state.value = 'error'
      errorMessage.value = '解析超时，请检查网络后重试'
    }
  }, 30000)

  try {
    const response = await parseShareLink(url, password.value)
    stopProgressAnimation()
    clearTimeout(parseTimeout)

    if (response.data && response.data.data) {
      parsedData.value = response.data.data
    } else {
      parsedData.value = response.data
    }
    state.value = 'parsed'
    parseProgress.value = 100
    emit('parsed', parsedData.value)
  } catch (err) {
    stopProgressAnimation()
    clearTimeout(parseTimeout)
    state.value = 'error'
    const errMsg = err?.response?.data?.error || err?.message || '解析失败，请稍后重试'
    errorMessage.value = errMsg
  }
}

/** 启动进度动画 */
const startProgressAnimation = () => {
  progressTimer = setInterval(() => {
    if (parseProgress.value < 90) {
      parseProgress.value += Math.random() * 15 + 5
      if (parseProgress.value > 90) parseProgress.value = 90
    }
  }, 500)
}

/** 停止进度动画 */
const stopProgressAnimation = () => {
  if (progressTimer) {
    clearInterval(progressTimer)
    progressTimer = null
  }
}

/** 取消解析 */
const handleCancelParse = () => {
  stopProgressAnimation()
  clearTimeout(parseTimeout)
  state.value = 'default'
  parseProgress.value = 0
}

/** 重新输入（回到default） */
const handleReset = () => {
  stopProgressAnimation()
  clearTimeout(parseTimeout)
  state.value = 'default'
  urlValue.value = ''
  password.value = ''
  parsedData.value = null
  errorMessage.value = ''
  parseProgress.value = 0
  emit('reset')
}

/** 清空（回到default但保留URL） */
const handleClear = () => {
  state.value = 'default'
  parsedData.value = null
  emit('reset')
}

/** 粘贴处理 */
const onPaste = (e) => {
  // 让 n-input 自行处理粘贴
}

// 清理
onUnmounted(() => {
  stopProgressAnimation()
  clearTimeout(parseTimeout)
})
</script>

<style scoped>
.share-link-input {
  margin-bottom: 20px;
}

.input-row {
  max-width: 680px;
}

.parsing-state {
  max-width: 680px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
  padding: 24px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  background: var(--card-bg, #f9fafb);
}

.parsing-hint {
  margin: 0;
  font-size: 14px;
  color: var(--text-secondary, #6b7280);
}

.parsed-state {
  max-width: 680px;
}

.parsed-card {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 12px;
  padding: 14px 18px;
  border: 1px solid #10b98133;
  border-radius: 12px;
  background: #10b98108;
}

.parsed-header {
  display: flex;
  align-items: center;
  gap: 6px;
}

.parsed-icon {
  font-size: 18px;
}

.parsed-title {
  font-size: 14px;
  font-weight: 700;
  color: #059669;
}

.parsed-info {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
  margin-left: 4px;
}

.info-item {
  font-size: 13px;
  color: var(--text-primary, #374151);
  font-weight: 500;
}

.info-separator {
  font-size: 13px;
  color: var(--text-tertiary, #9ca3af);
}

.parsed-actions {
  display: flex;
  gap: 8px;
  margin-left: auto;
}

.error-state {
  max-width: 680px;
}

.error-title {
  font-weight: 700;
}

.error-actions {
  display: flex;
  gap: 8px;
  margin-top: 8px;
}
</style>
