<template>
  <div class="system-logs">
    <section class="logs-toolbar">
      <div>
        <div class="page-kicker">Logs</div>
        <h2>系统日志中心</h2>
      </div>

      <div class="logs-actions">
        <el-select v-model="selectedLogFile" placeholder="选择日志文件" style="width: 260px" @change="loadLogContent">
          <el-option v-for="file in logFiles" :key="file.name" :label="file.name" :value="file.name">
            <span>{{ file.name }}</span>
            <span style="float: right; color: #909399; font-size: 12px;">{{ formatFileSize(file.size) }}</span>
          </el-option>
        </el-select>
        <el-button :icon="Refresh" @click="refreshLogContent" :loading="logLoading">刷新</el-button>
        <el-switch v-model="autoRefresh" active-text="自动刷新" />
        <div class="logs-config">
          <span>保留天数</span>
          <el-input-number v-model="logSaveDayLimit" :min="1" :max="365" @change="handleLogConfigChange" />
        </div>
      </div>
    </section>

    <section class="logs-content" v-loading="logLoading">
      <pre v-if="logContent">{{ logContent }}</pre>
      <el-empty v-else description="暂无日志内容" />
    </section>
  </div>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getLogConfig, getLogFileContent, getLogFiles, updateLogConfig } from '../../utils/api/strm'

const logFiles = ref([])
const selectedLogFile = ref('')
const logContent = ref('')
const logLoading = ref(false)
const autoRefresh = ref(false)
const logSaveDayLimit = ref(1)
let timer = null

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
    ElMessage.error('加载日志文件列表失败')
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
    ElMessage.error('加载日志内容失败')
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
    ElMessage.error('加载日志配置失败')
  }
}

const handleLogConfigChange = async (value) => {
  try {
    await updateLogConfig(value)
    ElMessage.success('日志保留天数已更新')
  } catch (error) {
    ElMessage.error('更新日志配置失败')
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
.system-logs {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.logs-toolbar,
.logs-content {
  padding: 24px;
  border-radius: 24px;
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

:global(.dark) .logs-toolbar,
:global(.dark) .logs-content {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

.page-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.logs-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.logs-toolbar h2 {
  margin-top: 10px;
  font-size: 30px;
}

.logs-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}

.logs-config {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #6f6457;
}

.logs-content {
  min-height: calc(100vh - 240px);
  overflow: auto;
  background: linear-gradient(180deg, #111822 0%, #0b1118 100%);
}

.logs-content pre {
  color: #d5e5f4;
  font-family: Consolas, 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.7;
  white-space: pre-wrap;
  word-break: break-word;
}

@media (max-width: 960px) {
  .logs-toolbar {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
