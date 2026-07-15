<template>
  <div class="space-y-4">
    <!-- 工具栏 -->
    <PageCard title="系统日志中心" subtitle="Logs">
      <div class="flex flex-wrap items-center gap-3">
        <n-select
          v-model:value="selectedLogFile"
          :options="logFileOptions"
          :render-label="renderLogFileLabel"
          placeholder="选择日志文件"
          class="w-full sm:w-64"
          @update:value="loadLogContent"
        />
        <n-button :loading="logLoading" @click="refreshLogContent">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新
        </n-button>
        <div class="flex items-center gap-2">
          <n-switch v-model:value="autoRefresh" />
          <span class="text-xs text-slate-500 dark:text-slate-400">自动刷新</span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-xs text-slate-500 dark:text-slate-400">保留天数</span>
          <n-input-number
            v-model:value="logSaveDayLimit"
            :min="1"
            :max="365"
            class="w-32"
            @update:value="handleLogConfigChange"
          />
        </div>
      </div>
    </PageCard>

    <!-- 日志内容 -->
    <PageCard>
      <n-spin :show="logLoading">
        <pre
          v-if="logContent"
          class="min-h-[60vh] whitespace-pre-wrap break-words rounded-xl bg-slate-950 p-4 font-mono text-xs leading-relaxed text-slate-300 overflow-auto"
        >{{ logContent }}</pre>
        <EmptyState v-else title="暂无日志内容" />
      </n-spin>
    </PageCard>
  </div>
</template>

<script setup>
import { computed, h, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInputNumber, NSelect, NSpin, NSwitch, useMessage } from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'
import PageCard from '../../components/common/PageCard.vue'
import EmptyState from '../../components/common/EmptyState.vue'
import { getLogConfig, getLogFileContent, getLogFiles, updateLogConfig } from '../../utils/api/strm'

const message = useMessage()

const logFiles = ref([])
const selectedLogFile = ref('')
const logContent = ref('')
const logLoading = ref(false)
const autoRefresh = ref(false)
const logSaveDayLimit = ref(1)
let timer = null

const logFileOptions = computed(() => {
  return logFiles.value.map(file => ({
    label: file.name,
    value: file.name,
    size: file.size
  }))
})

// 下拉项：文件名 + 右侧灰色文件大小
const renderLogFileLabel = (option) => {
  return h('div', { class: 'flex items-center justify-between gap-3' }, [
    h('span', null, option.label),
    h('span', { class: 'text-xs text-slate-400' }, formatFileSize(option.size))
  ])
}

const stopAutoRefresh = () => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const startAutoRefresh = () => {
  stopAutoRefresh()
  timer = setInterval(() => {
    loadLogContent()
  }, 3000)
}

const formatFileSize = (bytes) => {
  if (!bytes) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  const level = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1)
  return `${(bytes / (1024 ** level)).toFixed(level === 0 ? 0 : 2)} ${units[level]}`
}

const loadLogFiles = async () => {
  try {
    const response = await getLogFiles()
    logFiles.value = response.data.data || []
    if (!selectedLogFile.value && logFiles.value.length > 0) {
      const infoLog = logFiles.value.find(item => item.type === 'info')
      selectedLogFile.value = infoLog ? infoLog.name : logFiles.value[0].name
      await loadLogContent()
    }
  } catch (error) {
    message.error('加载日志文件列表失败')
  }
}

const loadLogContent = async () => {
  if (!selectedLogFile.value) return
  logLoading.value = true
  try {
    const response = await getLogFileContent(selectedLogFile.value, 500)
    logContent.value = response.data.data.content || ''
  } catch (error) {
    logContent.value = ''
    message.error('加载日志内容失败')
  } finally {
    logLoading.value = false
  }
}

const refreshLogContent = async () => {
  if (!selectedLogFile.value) {
    await loadLogFiles()
    return
  }
  await loadLogContent()
}

const loadLogConfig = async () => {
  try {
    const response = await getLogConfig()
    logSaveDayLimit.value = response.data.data.value || 1
  } catch (error) {
    message.error('加载日志配置失败')
  }
}

const handleLogConfigChange = async (value) => {
  try {
    await updateLogConfig(value)
    message.success('日志保留天数已更新')
  } catch (error) {
    message.error('更新日志配置失败')
  }
}

watch(autoRefresh, (value) => {
  if (value) startAutoRefresh()
  else stopAutoRefresh()
})

onBeforeUnmount(() => {
  stopAutoRefresh()
})

Promise.all([loadLogFiles(), loadLogConfig()])
</script>
