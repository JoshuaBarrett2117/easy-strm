<template>
  <el-container class="dashboard-container">
    <el-aside width="220px" class="sidebar">
      <div class="sidebar-header">
        <div class="logo-wrapper">
          <svg class="logo-svg" viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg">
            <defs>
              <linearGradient id="sidebarBgGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" style="stop-color:#667eea"/>
                <stop offset="100%" style="stop-color:#764ba2"/>
              </linearGradient>
              <linearGradient id="sidebarPlayGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                <stop offset="0%" style="stop-color:#38ef7d"/>
                <stop offset="100%" style="stop-color:#11998e"/>
              </linearGradient>
            </defs>
            <circle cx="32" cy="32" r="30" fill="url(#sidebarBgGrad)"/>
            <circle cx="32" cy="32" r="26" fill="none" stroke="rgba(255,255,255,0.2)" stroke-width="2"/>
            <polygon points="26,20 26,44 46,32" fill="url(#sidebarPlayGrad)" stroke="white" stroke-width="1.5" stroke-linejoin="round"/>
            <circle cx="50" cy="14" r="8" fill="#ffd93d" opacity="0.9"/>
            <path d="M50 10 L51 13 L54 13.5 L52 15.5 L52.5 18.5 L50 17 L47.5 18.5 L48 15.5 L46 13.5 L49 13 Z" fill="white"/>
          </svg>
          <span class="logo-text">Easy Stream</span>
        </div>
      </div>
      <el-menu
        :default-active="$route.path"
        router
        background-color="transparent"
        text-color="#a0aec0"
        active-text-color="#ffffff"
        class="sidebar-menu"
      >
        <el-menu-item index="/dashboard/user-info" class="menu-item">
          <el-icon><User /></el-icon>
          <span>用户信息</span>
        </el-menu-item>
        <el-menu-item index="/dashboard/cloud115" class="menu-item">
          <el-icon><Cloudy /></el-icon>
          <span>115云管理</span>
        </el-menu-item>
        <el-menu-item index="/dashboard/strm-config" class="menu-item">
          <el-icon><Setting /></el-icon>
          <span>STRM配置管理</span>
        </el-menu-item>
        <el-menu-item index="/dashboard/media-manager" class="menu-item">
          <el-icon><FolderOpened /></el-icon>
          <span>媒体管理</span>
        </el-menu-item>
        <el-menu-item index="/dashboard/category-strategy" class="menu-item">
          <el-icon><CollectionTag /></el-icon>
          <span>分类策略</span>
        </el-menu-item>
        <el-menu-item index="/dashboard/settings" class="menu-item">
          <el-icon><Tools /></el-icon>
          <span>系统配置</span>
        </el-menu-item>
      </el-menu>
      <div class="sidebar-footer">
        <div class="version-info">v1.0.0</div>
      </div>
    </el-aside>
    
    <el-container>
      <el-header class="content-header">
        <div class="header-left">
          <h1>{{ currentTitle }}</h1>
        </div>
        <div class="header-right">
          <el-button v-if="isAdmin" class="task-btn" @click="showTaskDialog">
            <el-icon><List /></el-icon>
            查看任务
          </el-button>
          <el-button v-if="isAdmin" class="log-btn" @click="showLogDialog">
            <el-icon><Document /></el-icon>
            查看日志
          </el-button>
          <el-button v-if="isAdmin" class="network-btn" @click="showNetworkDialog">
            <el-icon><Connection /></el-icon>
            网络测试
          </el-button>
          <el-button class="logout-btn" @click="handleLogout">
            <el-icon><SwitchButton /></el-icon>
            退出登录
          </el-button>
        </div>
      </el-header>
      <el-main class="content-body">
        <router-view :key="$route.fullPath" />
      </el-main>
    </el-container>

    <!-- 日志查看弹窗 -->
    <el-dialog
      v-model="logDialogVisible"
      title="系统日志"
      width="80%"
      top="5vh"
      :close-on-click-modal="false"
      :destroy-on-close="true"
      append-to-body
    >
      <div class="log-container">
        <div class="log-toolbar">
          <el-select v-model="selectedLogFile" placeholder="选择日志文件" @change="loadLogContent" style="width: 250px;">
            <el-option
              v-for="file in logFiles"
              :key="file.name"
              :label="file.name"
              :value="file.name"
            >
              <span>
                <el-tag :type="file.type === 'info' ? 'success' : 'warning'" size="small" style="margin-right: 8px;">{{ file.type?.toUpperCase() }}</el-tag>
                {{ file.name }}
              </span>
              <span style="float: right; color: #8492a6; font-size: 12px;">{{ formatFileSize(file.size) }}</span>
            </el-option>
          </el-select>
          <el-button :icon="Refresh" @click="refreshLogContent" :loading="logLoading" style="margin-left: 10px;">刷新</el-button>
          <el-checkbox v-model="autoRefresh" @change="toggleAutoRefresh" style="margin-left: 15px;">自动刷新</el-checkbox>
          <div class="log-config">
            <span class="config-label">日志保留天数：</span>
            <el-input-number 
              v-model="logSaveDayLimit" 
              :min="1" 
              :max="365" 
              size="small" 
              style="width: 100px;"
              @change="handleLogConfigChange"
            />
            <span class="config-unit">天</span>
          </div>
        </div>
        <div class="log-content" v-loading="logLoading">
          <pre v-if="logContent">{{ logContent }}</pre>
          <el-empty v-else description="暂无日志内容" />
        </div>
      </div>
      <template #footer>
        <el-button @click="logDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 任务列表弹窗 -->
    <el-dialog
      v-model="taskDialogVisible"
      title="任务列表"
      width="70%"
      top="5vh"
      :close-on-click-modal="false"
      :destroy-on-close="true"
      append-to-body
    >
      <div class="task-dialog-container">
        <div class="task-toolbar">
          <el-button :icon="Refresh" @click="loadTaskList" :loading="taskLoading">刷新</el-button>
          <el-checkbox v-model="autoRefreshTasks" @change="toggleTaskAutoRefresh" style="margin-left: 15px;">自动刷新</el-checkbox>
        </div>
        <div class="task-list" v-loading="taskLoading">
          <div v-if="taskList.length > 0" class="task-items">
            <TaskCard
              v-for="task in taskList"
              :key="task.task_id"
              :task="task"
              @cancel="handleCancelTask"
              @resume="handleResumeTask"
              @detail="handleTaskDetail"
            />
          </div>
          <el-empty v-else description="暂无运行中的任务" />
        </div>
      </div>
      <template #footer>
        <el-button @click="taskDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-drawer
      v-model="taskDetailVisible"
      title="任务详情"
      size="42%"
      :destroy-on-close="true"
      append-to-body
    >
      <div class="task-detail-drawer" v-loading="taskDetailLoading">
        <el-alert
          v-if="taskDetailError"
          type="error"
          :title="taskDetailError"
          show-icon
          :closable="false"
          class="task-detail-error"
        />

        <template v-if="taskDetailData">
          <el-descriptions :column="1" border class="task-detail-summary">
            <el-descriptions-item label="任务ID">
              {{ taskDetailData.task_id || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="任务名称">
              {{ taskDetailData.task_name || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="任务类型">
              {{ taskDetailData.task_type || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="状态">
              {{ taskDetailData.status || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="创建时间">
              {{ taskDetailData.create_time || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="更新时间">
              {{ taskDetailData.update_time || '-' }}
            </el-descriptions-item>
            <el-descriptions-item label="进度">
              {{ taskDetailData.progress ?? 0 }}%
            </el-descriptions-item>
            <el-descriptions-item label="文件统计">
              成功 {{ taskDetailData.success_files || 0 }} / 总数 {{ taskDetailData.total_files || 0 }} / 失败 {{ taskDetailData.failed_files || 0 }}
            </el-descriptions-item>
            <el-descriptions-item label="错误信息">
              {{ taskDetailData.error_message || '-' }}
            </el-descriptions-item>
          </el-descriptions>

          <div class="task-detail-section">
            <div class="task-detail-section-title">任务元数据</div>
            <div v-if="taskDetailMetadataRows.length > 0" class="task-detail-meta-grid">
              <div
                v-for="item in taskDetailMetadataRows"
                :key="item.key"
                class="task-detail-meta-item"
              >
                <div class="task-detail-meta-label">{{ item.label }}</div>
                <div class="task-detail-meta-value">{{ item.value }}</div>
              </div>
            </div>
            <el-empty v-else description="暂无任务元数据" />
          </div>

          <div v-if="taskDetailFailedItems.length > 0" class="task-detail-section">
            <div class="task-detail-section-title">失败文件</div>
            <div v-if="taskDetailFailureGroups.length > 0" class="task-detail-failure-summary">
              <div
                v-for="group in taskDetailFailureGroups"
                :key="group.key"
                class="task-detail-failure-group"
              >
                <div class="task-detail-failure-group-header">
                  <div class="task-detail-failure-group-title">{{ group.label }}</div>
                  <el-tag :type="group.tagType" size="small">{{ group.items.length }}</el-tag>
                </div>
                <div class="task-detail-failure-group-reason">
                  {{ group.reason || '未提供失败原因' }}
                </div>
              </div>
            </div>
            <div class="task-detail-failed-list">
              <div
                v-for="item in taskDetailFailedItems"
                :key="`${item.file_id || item.file_name}-${item.reason}`"
                class="task-detail-failed-item"
              >
                <div class="task-detail-failed-main">
                  <span class="task-detail-failed-name">{{ item.file_name }}</span>
                  <span v-if="item.file_id" class="task-detail-failed-id">{{ item.file_id }}</span>
                  <el-tag v-if="item.category" :type="taskDetailFailureTagType(item.category)" size="small">
                    {{ taskDetailFailureTagLabel(item.category) }}
                  </el-tag>
                </div>
                <div class="task-detail-failed-reason">{{ item.reason || '未提供失败原因' }}</div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </el-drawer>

    <el-dialog
      v-model="networkDialogVisible"
      title="网络连通性测试"
      width="72%"
      top="8vh"
      :close-on-click-modal="false"
      :destroy-on-close="true"
      append-to-body
    >
      <div class="network-toolbar">
        <el-button :icon="Refresh" @click="loadNetworkResults" :loading="networkLoading">刷新</el-button>
      </div>
      <el-table :data="networkResults" v-loading="networkLoading" stripe border>
        <el-table-column prop="name" label="站点" min-width="130" />
        <el-table-column prop="url" label="地址" min-width="220" />
        <el-table-column label="连通状态" width="110">
          <template #default="{ row }">
            <el-tag :type="getNetworkStatusTag(row)">
              {{ getNetworkStatusText(row) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="代理路径" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.via_proxy !== null && row.via_proxy !== undefined" :type="row.via_proxy ? 'warning' : 'info'">
              {{ row.via_proxy ? '代理' : '直连' }}
            </el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status_code" label="HTTP" width="90" />
        <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
        <el-table-column label="错误信息" min-width="200">
          <template #default="{ row }">
            {{ row.error || row.status_message || '-' }}
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="networkDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup>
import { computed, nextTick, ref, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { User, Cloudy, Setting, SwitchButton, Document, Refresh, List, Tools, FolderOpened, CollectionTag, Connection } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { showConfirmDialog } from '../utils/ui/messageBox'
import { getLogFiles, getLogFileContent, getLogConfig, updateLogConfig, getTaskList, getTaskDetail, cancelTask, resumeTask, getNetworkProbeSites, testNetworkConnectivity } from '../utils/api'
import TaskCard from '../components/TaskCard.vue'

const router = useRouter()

// 判断是否为 admin 用户
const isAdmin = computed(() => {
  const userName = localStorage.getItem('user_name')
  return userName === 'admin'
})

const currentTitle = computed(() => {
  const path = router.currentRoute.value.path
  if (path === '/dashboard/user-info') {
    return '用户信息管理'
  } else if (path === '/dashboard/cloud115') {
    return '115云账号管理'
  } else if (path === '/dashboard/strm-config') {
    return 'STRM配置管理'
  } else if (path === '/dashboard/media-manager') {
    return '媒体管理'
  } else if (path === '/dashboard/category-strategy') {
    return '分类策略'
  } else if (path === '/dashboard/settings') {
    return '系统配置'
  }
  return '后台管理'
})

// 日志相关状态
const logDialogVisible = ref(false)
const logFiles = ref([])
const selectedLogFile = ref('')
const logContent = ref('')
const logLoading = ref(false)
const autoRefresh = ref(false)
const logSaveDayLimit = ref(1)
let autoRefreshTimer = null

/**
 * 显示日志弹窗
 */
const showLogDialog = async () => {
  logDialogVisible.value = true
  await Promise.all([loadLogFiles(), loadLogConfig()])
}

/**
 * 加载日志文件列表
 */
const loadLogFiles = async () => {
  try {
    const response = await getLogFiles()
    logFiles.value = response.data.data || []
    // 默认选择最新的 INFO 日志文件，后端已按日期降序排列
    if (logFiles.value.length > 0) {
      // 优先选择 INFO 类型的日志文件
      const infoLogFile = logFiles.value.find(f => f.type === 'info')
      selectedLogFile.value = infoLogFile ? infoLogFile.name : logFiles.value[0].name
      await loadLogContent()
    } else {
      // 没有日志文件时清空内容并提示
      logContent.value = ''
      ElMessage.warning('暂无日志文件，请稍后再试')
    }
  } catch (error) {
    console.error('加载日志文件列表失败:', error)
    ElMessage.error('加载日志文件列表失败')
  }
}

/**
 * 加载日志内容
 */
const loadLogContent = async () => {
  if (!selectedLogFile.value) {
    ElMessage.warning('请先选择日志文件')
    return
  }
  
  logLoading.value = true
  try {
    const response = await getLogFileContent(selectedLogFile.value, 500)
    logContent.value = response.data.data.content || ''
  } catch (error) {
    console.error('加载日志内容失败:', error)
    ElMessage.error('加载日志内容失败')
    logContent.value = ''
  } finally {
    logLoading.value = false
  }
}

/**
 * 刷新日志内容
 */
const refreshLogContent = async () => {
  if (!selectedLogFile.value) {
    await loadLogFiles()
    return
  }
  await loadLogContent()
}

/**
 * 切换自动刷新
 */
const toggleAutoRefresh = (value) => {
  if (value) {
    autoRefreshTimer = setInterval(() => {
      loadLogContent()
    }, 3000) // 每 3 秒刷新一次
  } else {
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer)
      autoRefreshTimer = null
    }
  }
}

/**
 * 加载日志配置
 */
const loadLogConfig = async () => {
  try {
    const response = await getLogConfig()
    logSaveDayLimit.value = response.data.data.value || 1
  } catch (error) {
    console.error('加载日志配置失败:', error)
  }
}

/**
 * 处理日志配置变更
 */
const handleLogConfigChange = async (value) => {
  try {
    await updateLogConfig(value)
    ElMessage.success('日志保留天数已更新')
  } catch (error) {
    console.error('更新日志配置失败:', error)
    ElMessage.error('更新日志配置失败')
  }
}

/**
 * 格式化文件大小
 */
const formatFileSize = (bytes) => {
  if (bytes === 0) return '0 B'
  const k = 1024
  const sizes = ['B', 'KB', 'MB', 'GB']
  const i = Math.floor(Math.log(bytes) / Math.log(k))
  return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i]
}

/**
 * 处理退出登录
 */
const handleLogout = () => {
  showConfirmDialog('确定要退出登录吗？', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(() => {
    localStorage.removeItem('token')
    localStorage.removeItem('user_id')
    localStorage.removeItem('user_name')
    router.push('/login')
  }).catch(() => {})
}

// 任务相关状态
const taskDialogVisible = ref(false)
const taskList = ref([])
const taskLoading = ref(false)
const autoRefreshTasks = ref(false)
let taskRefreshTimer = null
const taskDetailVisible = ref(false)
const taskDetailLoading = ref(false)
const taskDetailData = ref(null)
const taskDetailError = ref('')

const taskDetailMetadataRows = computed(() => {
  const metadata = taskDetailData.value?.metadata
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) {
    return []
  }

  const labelMap = {
    source_id: '来源ID',
    source_name: '来源媒体源',
    source_type: '来源类型',
    source_path: '来源路径',
    organize_target_path: '整理目标',
    media_type: '媒体类型',
    conflict_policy: '冲突策略',
    operation_mode: '整理方式',
    watch_interval: '监控间隔',
    trigger_mode: '触发方式',
    detected_files: '检测文件数',
    success_files: '成功数',
    failed_files: '失败数',
    result_summary: '结果概览',
    failure_category: '失败分类',
    failure_reason: '失败原因',
    failed_item_count: '失败文件数'
  }

  return Object.entries(metadata)
    .filter(([key, value]) => key !== 'failed_items' && value !== null && value !== undefined && value !== '')
    .map(([key, value]) => {
      let formatted = value
      if (key === 'trigger_mode') {
        formatted = value === 'polling' ? '115 轮询监控' : value === 'fsnotify' ? '本地实时监控' : value
      } else if (key === 'media_type') {
        const mediaTypeMap = { all: '全部', movie: '电影', tv: '剧集' }
        formatted = mediaTypeMap[value] || value
      } else if (key === 'conflict_policy') {
        const conflictPolicyMap = { skip: '跳过', overwrite: '覆盖', suffix: '追加序号' }
        formatted = conflictPolicyMap[value] || value
      } else if (key === 'operation_mode') {
        const operationModeMap = { move: '移动', copy: '复制', hardlink: '硬链接', symlink: '软链接' }
        formatted = operationModeMap[value] || value
      } else if (key === 'watch_interval' && Number.isFinite(Number(value))) {
        formatted = `${value} 秒`
      } else if (Array.isArray(value)) {
        formatted = `${value.length} 项`
      } else if (typeof value === 'object') {
        formatted = JSON.stringify(value)
      }

      return {
        key,
        label: labelMap[key] || key,
        value: String(formatted)
      }
    })
})

const taskDetailFailedItems = computed(() => {
  const items = taskDetailData.value?.metadata?.failed_items
  if (!Array.isArray(items)) {
    return []
  }

  return items
    .map((item) => ({
      file_id: item?.file_id || item?.fileID || '',
      file_name: item?.file_name || item?.fileName || item?.file_id || '',
      category: item?.category || '',
      reason: item?.reason || ''
    }))
    .filter(item => item.file_id || item.file_name)
})

const taskDetailFailureGroups = computed(() => {
  const items = taskDetailFailedItems.value
  if (items.length === 0) {
    return []
  }

  const groupOrder = ['identify_failed', 'organize_failed', 'cloud115_auth_failed', 'cloud115_failed', 'scan_failed', 'target_path', 'partial_failed', 'conflict_skipped', 'panic', 'other']
  const groupMap = new Map()

  const getGroupKey = (category, reason) => {
    const normalizedCategory = String(category || '').trim()
    if (normalizedCategory) {
      return normalizedCategory
    }
    const text = String(reason || '').toLowerCase()
    if (text.includes('identify') || text.includes('tmdb') || text.includes('recogniz')) return 'identify_failed'
    if (text.includes('organize') || text.includes('move') || text.includes('copy')) return 'organize_failed'
    if (text.includes('cookie') || text.includes('auth') || text.includes('unauthorized')) return 'cloud115_auth_failed'
    if (text.includes('cloud115') || text.includes('115')) return 'cloud115_failed'
    if (text.includes('scan') || text.includes('list')) return 'scan_failed'
    if (text.includes('target') && text.includes('path')) return 'target_path'
    if (text.includes('partial') || text.includes('部分')) return 'partial_failed'
    if (text.includes('conflict') || text.includes('冲突')) return 'conflict_skipped'
    if (text.includes('panic') || text.includes('exception')) return 'panic'
    return 'other'
  }

  const getGroupMeta = (key) => {
    const meta = {
      identify_failed: { label: '识别失败', tagType: 'warning' },
      organize_failed: { label: '整理失败', tagType: 'danger' },
      cloud115_auth_failed: { label: '115 账号失效', tagType: 'danger' },
      cloud115_failed: { label: '115 操作失败', tagType: 'danger' },
      scan_failed: { label: '扫描失败', tagType: 'warning' },
      target_path: { label: '目标路径异常', tagType: 'warning' },
      partial_failed: { label: '部分失败', tagType: 'warning' },
      conflict_skipped: { label: '冲突跳过', tagType: 'warning' },
      panic: { label: '异常中断', tagType: 'danger' },
      other: { label: '其他失败', tagType: 'info' }
    }
    return meta[key] || meta.other
  }

  for (const item of items) {
    const key = getGroupKey(item.category, item.reason)
    if (!groupMap.has(key)) {
      const meta = getGroupMeta(key)
      groupMap.set(key, {
        key,
        label: meta.label,
        tagType: meta.tagType,
        reason: item.reason || '',
        items: []
      })
    }
    groupMap.get(key).items.push(item)
  }

  return groupOrder
    .filter(key => groupMap.has(key))
    .map(key => groupMap.get(key))
})

const taskDetailFailureTagLabel = (category) => {
  const map = {
    identify_failed: '识别失败',
    organize_failed: '整理失败',
    cloud115_auth_failed: '115 账号失效',
    cloud115_failed: '115 操作失败',
    scan_failed: '扫描失败',
    target_path: '目标路径异常',
    panic: '异常中断',
    partial_failed: '部分失败',
    conflict_skipped: '冲突跳过',
    other: '其他失败'
  }
  return map[String(category || '').trim()] || '其他失败'
}

const taskDetailFailureTagType = (category) => {
  const key = String(category || '').trim()
  if (key === 'identify_failed' || key === 'scan_failed' || key === 'target_path') return 'warning'
  if (key === 'organize_failed' || key === 'cloud115_auth_failed' || key === 'cloud115_failed' || key === 'panic') return 'danger'
  if (key === 'partial_failed') return 'warning'
  return 'info'
}

const networkDialogVisible = ref(false)
const networkLoading = ref(false)
const networkResults = ref([])
let networkProbeRunId = 0

const normalizeNetworkSites = (payload) => {
  if (Array.isArray(payload)) {
    return payload
  }
  if (Array.isArray(payload?.data)) {
    return payload.data
  }
  return []
}

const normalizeNetworkResult = (payload) => {
  if (Array.isArray(payload)) {
    return payload[0] || null
  }
  if (Array.isArray(payload?.data)) {
    return payload.data[0] || null
  }
  if (payload?.data && typeof payload.data === 'object') {
    return payload.data
  }
  if (payload && typeof payload === 'object') {
    return payload
  }
  return null
}

/**
 * 显示任务弹窗
 */
const showTaskDialog = async () => {
  taskDialogVisible.value = true
  await loadTaskList()
}

const handleTaskDetail = async (taskId) => {
  if (!taskId) {
    return
  }

  taskDetailVisible.value = true
  taskDetailLoading.value = true
  taskDetailError.value = ''
  taskDetailData.value = null

  try {
    const response = await getTaskDetail(taskId)
    taskDetailData.value = response.data.data || response.data || null
  } catch (error) {
    console.error('加载任务详情失败:', error)
    taskDetailError.value = error.response?.data?.error || error.message || '加载任务详情失败'
  } finally {
    taskDetailLoading.value = false
  }
}

/**
 * 加载任务列表
 */
const loadTaskList = async () => {
  taskLoading.value = true
  try {
    const response = await getTaskList()
    taskList.value = response.data.data || []
  } catch (error) {
    console.error('加载任务列表失败:', error)
    ElMessage.error('加载任务列表失败')
  } finally {
    taskLoading.value = false
  }
}

const handleCancelTask = async (taskId) => {
  if (!taskId) {
    return
  }

  try {
    await cancelTask(taskId)
    ElMessage.success('任务已取消')
    await loadTaskList()
  } catch (error) {
    console.error('取消任务失败:', error)
  }
}

const handleResumeTask = async (taskId) => {
  if (!taskId) {
    return
  }

  try {
    await resumeTask(taskId)
    ElMessage.success('任务已重新执行')
    await loadTaskList()
  } catch (error) {
    console.error('恢复任务失败:', error)
  }
}

/**
 * 切换任务自动刷新
 */
const toggleTaskAutoRefresh = (value) => {
  if (value) {
    taskRefreshTimer = setInterval(() => {
      loadTaskList()
    }, 3000)
  } else {
    if (taskRefreshTimer) {
      clearInterval(taskRefreshTimer)
      taskRefreshTimer = null
    }
  }
}

const showNetworkDialog = async () => {
  networkDialogVisible.value = true
  await loadNetworkResults()
}

const loadNetworkResults = async () => {
  const runId = ++networkProbeRunId
  networkLoading.value = true
  try {
    const siteResponse = await getNetworkProbeSites()
    const sites = normalizeNetworkSites(siteResponse.data.data)
    networkResults.value = sites.map(site => ({
      ...site,
      ok: null,
      status_code: null,
      duration_ms: null,
      via_proxy: null,
      error: '',
      status_message: '待测试'
    }))
    await nextTick()

    for (const site of sites) {
      if (runId !== networkProbeRunId) {
        return
      }

      const currentItem = networkResults.value.find(item => item.name === site.name && item.url === site.url)
      if (currentItem) {
        currentItem.status_message = '测试中...'
      }

      try {
        const response = await testNetworkConnectivity({ name: site.name, url: site.url })
        const result = normalizeNetworkResult(response.data.data)
        if (currentItem && result) {
          Object.assign(currentItem, result, {
            status_message: result.ok ? '成功' : '失败'
          })
        }
      } catch (error) {
        const errorMsg = error.response?.data?.error || error.message || '测试失败'
        if (currentItem) {
          Object.assign(currentItem, {
            ok: false,
            error: errorMsg,
            status_message: '失败'
          })
        }
      }
    }
  } catch (error) {
    console.error('加载网络测试结果失败:', error)
    ElMessage.error('加载网络测试结果失败')
    networkResults.value = []
  } finally {
    if (runId === networkProbeRunId) {
      networkLoading.value = false
    }
  }
}

const getNetworkStatusText = (row) => {
  if (row.ok === true) return '成功'
  if (row.ok === false) return '失败'
  return row.status_message || '待测试'
}

const getNetworkStatusTag = (row) => {
  if (row.ok === true) return 'success'
  if (row.ok === false) return 'danger'
  return 'info'
}
// 组件卸载时清理定时器
onUnmounted(() => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
  }
  if (taskRefreshTimer) {
    clearInterval(taskRefreshTimer)
  }
})

/**
 * 监听日志弹窗关闭，停止自动刷新
 */
watch(logDialogVisible, (newVal) => {
  if (!newVal) {
    // 弹窗关闭时停止自动刷新
    autoRefresh.value = false
    if (autoRefreshTimer) {
      clearInterval(autoRefreshTimer)
      autoRefreshTimer = null
    }
  }
})

/**
 * 监听任务弹窗关闭，停止自动刷新
 */
watch(taskDialogVisible, (newVal) => {
  if (!newVal) {
    autoRefreshTasks.value = false
    if (taskRefreshTimer) {
      clearInterval(taskRefreshTimer)
      taskRefreshTimer = null
    }
  }
})

watch(taskDetailVisible, (newVal) => {
  if (!newVal) {
    taskDetailError.value = ''
    taskDetailData.value = null
  }
})
</script>

<style scoped>
.dashboard-container {
  height: 100vh;
  background-color: #f0f2f5;
}

.sidebar {
  background: linear-gradient(180deg, #1a1a2e 0%, #16213e 100%);
  display: flex;
  flex-direction: column;
  box-shadow: 2px 0 10px rgba(0, 0, 0, 0.1);
}

.sidebar-header {
  padding: 20px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.1);
}

.logo-wrapper {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-svg {
  width: 36px;
  height: 36px;
}

.logo-text {
  font-size: 20px;
  font-weight: 700;
  color: #ffffff;
  letter-spacing: 1px;
}

.sidebar-menu {
  flex: 1;
  border-right: none;
  padding: 10px 0;
}

.menu-item {
  margin: 4px 12px;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.menu-item:hover {
  background-color: rgba(102, 126, 234, 0.2) !important;
}

:deep(.el-menu-item.is-active) {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%) !important;
  color: white !important;
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
}

.sidebar-footer {
  padding: 15px;
  border-top: 1px solid rgba(255, 255, 255, 0.1);
  text-align: center;
}

.version-info {
  color: #606266;
  font-size: 12px;
  background: rgba(255, 255, 255, 0.05);
  padding: 6px 12px;
  border-radius: 12px;
  display: inline-block;
}

.content-header {
  background-color: white;
  border-bottom: 1px solid #e8e8e8;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  box-shadow: 0 1px 4px rgba(0, 0, 0, 0.08);
}

.header-left h1 {
  font-size: 18px;
  color: #303133;
  margin: 0;
  font-weight: 600;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 16px;
}

.logout-btn {
  color: #f56c6c;
  border: 1px solid #f56c6c;
  background-color: transparent;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.logout-btn:hover {
  color: #fff;
  background-color: #f56c6c;
  border-color: #f56c6c;
}

.content-body {
  background-color: #f0f2f5;
  padding: 0;
  overflow-y: auto;
}

.log-btn {
  color: #409eff;
  border: 1px solid #409eff;
  background-color: transparent;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.log-btn:hover {
  color: #fff;
  background-color: #409eff;
  border-color: #409eff;
}

.network-btn {
  color: #e6a23c;
  border: 1px solid #e6a23c;
  background-color: transparent;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.network-btn:hover {
  color: #fff;
  background-color: #e6a23c;
  border-color: #e6a23c;
}

.log-container {
  height: 60vh;
  display: flex;
  flex-direction: column;
}

.log-toolbar {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
  margin-bottom: 15px;
  padding-bottom: 15px;
  border-bottom: 1px solid #ebeef5;
}

.log-config {
  display: flex;
  align-items: center;
  margin-left: auto;
}

.config-label {
  font-size: 14px;
  color: #606266;
  margin-right: 8px;
}

.config-unit {
  font-size: 14px;
  color: #606266;
  margin-left: 5px;
}

.log-content {
  flex: 1;
  overflow: auto;
  background-color: #1e1e1e;
  border-radius: 8px;
  padding: 15px;
}

.log-content pre {
  margin: 0;
  color: #d4d4d4;
  font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
  font-size: 13px;
  line-height: 1.5;
  white-space: pre-wrap;
  word-wrap: break-word;
}

.task-btn {
  color: #67c23a;
  border: 1px solid #67c23a;
  background-color: transparent;
  border-radius: 8px;
  transition: all 0.3s ease;
}

.task-btn:hover {
  color: #fff;
  background-color: #67c23a;
  border-color: #67c23a;
}

.task-dialog-container {
  height: 60vh;
  display: flex;
  flex-direction: column;
}

.task-detail-drawer {
  padding-right: 8px;
}

.task-detail-error {
  margin-bottom: 16px;
}

.task-detail-summary {
  margin-bottom: 16px;
}

.task-detail-section {
  margin-bottom: 20px;
}

.task-detail-section-title {
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.task-detail-meta-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}

.task-detail-meta-item {
  padding: 10px 12px;
  border-radius: 10px;
  background: #f7f8fa;
  border: 1px solid #ebeef5;
}

.task-detail-meta-label {
  margin-bottom: 4px;
  font-size: 12px;
  color: #909399;
}

.task-detail-meta-value {
  font-size: 13px;
  color: #303133;
  word-break: break-word;
  line-height: 1.5;
}

.task-detail-failure-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.task-detail-failure-group {
  padding: 12px 14px;
  border: 1px solid #ebeef5;
  border-radius: 10px;
  background: #fff;
}

.task-detail-failure-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.task-detail-failure-group-title {
  font-weight: 600;
  color: #303133;
}

.task-detail-failure-group-reason {
  margin-top: 8px;
  font-size: 13px;
  line-height: 1.6;
  color: #606266;
  word-break: break-word;
}

.task-detail-failed-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-detail-failed-item {
  padding: 12px 14px;
  border: 1px solid #fde2e2;
  border-radius: 10px;
  background: #fffafa;
}

.task-detail-failed-main {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  margin-bottom: 4px;
}

.task-detail-failed-name {
  font-weight: 600;
  color: #303133;
  word-break: break-all;
}

.task-detail-failed-id {
  font-size: 12px;
  color: #909399;
  word-break: break-all;
}

.task-detail-failed-reason {
  font-size: 12px;
  line-height: 1.5;
  color: #f56c6c;
  word-break: break-word;
}

.task-toolbar {
  display: flex;
  align-items: center;
  margin-bottom: 15px;
  padding-bottom: 15px;
  border-bottom: 1px solid #ebeef5;
}

.task-list {
  flex: 1;
  overflow-y: auto;
}

.task-items {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.network-toolbar {
  display: flex;
  align-items: center;
  margin-bottom: 12px;
}
</style>





