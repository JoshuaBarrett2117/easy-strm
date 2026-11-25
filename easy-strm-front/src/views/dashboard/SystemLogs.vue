<template>
  <div class="system-logs-panel space-y-4">
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
        <n-select
          v-model:value="levelFilter"
          :options="levelOptions"
          class="w-32"
          @update:value="loadLogContent"
        />
        <n-input
          v-model:value="keyword"
          clearable
          placeholder="关键字 / req=… / task=…（空格分隔，需全部命中）"
          class="w-full sm:w-80"
          @keyup.enter="loadLogContent"
          @clear="onKeywordClear"
        />
        <n-select
          v-model:value="lineLimit"
          :options="lineOptions"
          class="w-28"
          @update:value="loadLogContent"
        />
        <n-button type="primary" :loading="logLoading" @click="refreshLogContent">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          查询
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
      <div class="mb-2 flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <span>{{ summaryText }}</span>
        <n-tag v-if="keyword" size="small" closable @close="onKeywordClear">筛选：{{ keyword }}</n-tag>
        <span class="text-slate-400">debug 文件是包含所有级别的完整时间线；点击 req / task 可只看该请求或任务的全部日志。</span>
      </div>
      <n-spin :show="logLoading" class="system-logs-content">
        <div
          v-if="parsedLines.length"
          class="system-logs-text rounded-xl bg-slate-950 p-4 font-mono text-xs leading-relaxed text-slate-300 overflow-auto"
        >
          <div v-for="(line, index) in parsedLines" :key="index" class="whitespace-pre-wrap break-words">
            <template v-if="line.parsed">
              <span class="text-slate-500">{{ line.time }} </span>
              <span :class="levelClass(line.level)">{{ line.level.padEnd(5) }} </span>
              <span v-if="line.req !== '-'" class="log-token text-sky-400" title="只看该请求" @click="filterBy('req=' + line.req)">req={{ shortId(line.req) }} </span>
              <span v-if="line.task !== '-'" class="log-token text-emerald-400" title="只看该任务" @click="filterBy('task=' + line.task)">task={{ line.task }} </span>
              <span class="log-token text-violet-300" title="只看该模块" @click="filterBy('module=' + line.module)">[{{ line.module }}] </span>
              <span :class="messageClass(line.level)">{{ line.message }}</span>
            </template>
            <span v-else>{{ line.raw }}</span>
          </div>
        </div>
        <EmptyState v-else :title="keyword || levelFilter ? '没有匹配的日志' : '暂无日志内容'" />
      </n-spin>
    </PageCard>
  </div>
</template>

<script setup>
import { computed, h, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInput, NInputNumber, NSelect, NSpin, NSwitch, NTag, useMessage } from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'
import PageCard from '../../components/common/PageCard.vue'
import EmptyState from '../../components/common/EmptyState.vue'
import { getLogConfig, getLogFileContent, getLogFiles, updateLogConfig } from '../../utils/api/strm'
import { formatFileSize, parseLogLine, shortLogId, sortLogFiles } from '../../utils/logs'

const message = useMessage()

const logFiles = ref([])
const selectedLogFile = ref('')
const logLines = ref([])
const logMeta = ref({ matched: 0, truncated: false })
const logLoading = ref(false)
const autoRefresh = ref(false)
const logSaveDayLimit = ref(1)
const levelFilter = ref('')
const keyword = ref('')
const lineLimit = ref(500)
let timer = null

const levelOptions = [
  { label: '全部级别', value: '' },
  { label: 'INFO 及以上', value: 'INFO' },
  { label: 'WARN 及以上', value: 'WARN' },
  { label: '仅 ERROR', value: 'ERROR' }
]
const lineOptions = [500, 2000, 5000].map(value => ({ label: `${value} 行`, value }))

const logFileOptions = computed(() => {
  return logFiles.value.map(file => ({
    label: file.name,
    value: file.name,
    size: file.size
  }))
})

const parsedLines = computed(() => logLines.value.map(parseLogLine))

const summaryText = computed(() => {
  const base = `${selectedLogFile.value || '未选择文件'}：显示最近 ${logMeta.value.matched} 条匹配日志，最新在上方`
  return logMeta.value.truncated ? `${base}（文件较大，仅检索了末尾 64MB）` : base
})

const levelClass = (level) => ({
  'text-slate-500': level === 'DEBUG',
  'text-sky-300': level === 'INFO',
  'text-amber-400 font-bold': level === 'WARN',
  'text-rose-400 font-bold': level === 'ERROR'
})
const messageClass = (level) => ({
  'text-slate-400': level === 'DEBUG',
  'text-amber-200': level === 'WARN',
  'text-rose-300': level === 'ERROR'
})
const shortId = shortLogId

const filterBy = (token) => {
  keyword.value = token
  levelFilter.value = ''
  loadLogContent()
}

const onKeywordClear = () => {
  keyword.value = ''
  loadLogContent()
}

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
    loadLogContent(true)
  }, 3000)
}

const loadLogFiles = async () => {
  try {
    const response = await getLogFiles()
    const files = sortLogFiles(response.data.data || [])
    logFiles.value = files
    if (!selectedLogFile.value && files.length > 0) {
      selectedLogFile.value = files[0].name
      await loadLogContent()
    }
  } catch (error) {
    message.error('加载日志文件列表失败')
  }
}

const loadLogContent = async (silent = false) => {
  if (!selectedLogFile.value) return
  if (!silent) logLoading.value = true
  try {
    const response = await getLogFileContent(selectedLogFile.value, lineLimit.value, {
      level: levelFilter.value,
      q: keyword.value.trim()
    })
    const data = response.data.data || {}
    logLines.value = data.content ? data.content.split('\n') : []
    logMeta.value = { matched: data.matched ?? logLines.value.length, truncated: !!data.truncated }
  } catch (error) {
    logLines.value = []
    if (!silent) message.error('加载日志内容失败')
  } finally {
    logLoading.value = false
  }
}

const refreshLogContent = async () => {
  await loadLogFiles()
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

<style scoped>
.log-token {
  cursor: pointer;
}
.log-token:hover {
  text-decoration: underline;
}
</style>
