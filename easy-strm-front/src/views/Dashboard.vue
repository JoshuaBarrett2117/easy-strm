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
    >
      <div class="task-dialog-container">
        <div class="task-toolbar">
          <el-button :icon="Refresh" @click="loadTaskList" :loading="taskLoading">刷新</el-button>
          <el-checkbox v-model="autoRefreshTasks" @change="toggleTaskAutoRefresh" style="margin-left: 15px;">自动刷新</el-checkbox>
        </div>
        <div class="task-list" v-loading="taskLoading">
          <div v-if="taskList.length > 0" class="task-items">
            <TaskCard v-for="task in taskList" :key="task.task_id" :task="task" />
          </div>
          <el-empty v-else description="暂无运行中的任务" />
        </div>
      </div>
      <template #footer>
        <el-button @click="taskDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="networkDialogVisible"
      title="网络连通性测试"
      width="72%"
      top="8vh"
      :close-on-click-modal="false"
      :destroy-on-close="true"
    >
      <div class="network-toolbar">
        <el-button :icon="Refresh" @click="loadNetworkResults" :loading="networkLoading">刷新</el-button>
      </div>
      <el-table :data="networkResults" v-loading="networkLoading" stripe border>
        <el-table-column prop="name" label="站点" min-width="130" />
        <el-table-column prop="url" label="地址" min-width="220" />
        <el-table-column label="连通状态" width="110">
          <template #default="{ row }">
            <el-tag :type="row.ok ? 'success' : 'danger'">
              {{ row.ok ? '成功' : '失败' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="代理路径" width="120">
          <template #default="{ row }">
            <el-tag :type="row.via_proxy ? 'warning' : 'info'">
              {{ row.via_proxy ? '代理' : '直连' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status_code" label="HTTP" width="90" />
        <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
        <el-table-column prop="error" label="错误信息" min-width="200" />
      </el-table>
      <template #footer>
        <el-button @click="networkDialogVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </el-container>
</template>

<script setup>
import { computed, ref, onUnmounted, watch } from 'vue'
import { useRouter } from 'vue-router'
import { User, Cloudy, Setting, SwitchButton, Document, Refresh, List, Tools, FolderOpened, CollectionTag, Connection } from '@element-plus/icons-vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import { getLogFiles, getLogFileContent, getLogConfig, updateLogConfig, getTaskList, testNetworkConnectivity } from '../utils/api'
import TaskCard from '../components/TaskCard.vue'

const router = useRouter()

// 判断是否为admin用户
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
    // 默认选择最新的INFO日志文件（后端已按日期降序排列）
    if (logFiles.value.length > 0) {
      // 优先选择INFO类型的日志文件
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
    }, 3000) // 每3秒刷新一次
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
  ElMessageBox.confirm('确定要退出登录吗？', '提示', {
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

const networkDialogVisible = ref(false)
const networkLoading = ref(false)
const networkResults = ref([])

/**
 * 显示任务弹窗
 */
const showTaskDialog = async () => {
  taskDialogVisible.value = true
  await loadTaskList()
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
  networkLoading.value = true
  try {
    const response = await testNetworkConnectivity()
    networkResults.value = response.data.data || []
  } catch (error) {
    console.error('加载网络测试结果失败:', error)
    ElMessage.error('加载网络测试结果失败')
    networkResults.value = []
  } finally {
    networkLoading.value = false
  }
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
 * 监听弹窗关闭，停止自动刷新
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

