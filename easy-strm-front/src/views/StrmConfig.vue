<template>
  <div class="strm-config-container">
    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><Setting /></el-icon>
            <span>STRM 文件配置中心</span>
          </div>
          <el-button type="primary" @click="handleAdd">
            <el-icon><Plus /></el-icon>
            新增配置
          </el-button>
        </div>
      </template>

      <section class="overview-panel">
        <div class="overview-copy">
          <h2>配置与任务总览</h2>
          <p>STRM 配置、Cron 状态、全量生成任务和即时执行入口都保留原有后端流程，这里只重构首屏组织方式。</p>
        </div>
        <div class="overview-grid">
          <article v-for="card in configOverviewCards" :key="card.label" class="overview-card">
            <span class="overview-card__label">{{ card.label }}</span>
            <strong class="overview-card__value">{{ card.value }}</strong>
            <p class="overview-card__hint">{{ card.hint }}</p>
          </article>
        </div>
      </section>

      <section class="status-rail">
        <div class="status-rail__item">
          <span>Cron 已启用</span>
          <strong>{{ enabledCronCount }} / {{ cronTaskList.length || 0 }}</strong>
        </div>
        <div class="status-rail__item">
          <span>下一次计划执行</span>
          <strong>{{ nextRunSnapshot }}</strong>
        </div>
        <div class="status-rail__item">
          <span>最近聚焦账号</span>
          <strong>{{ primaryCloudAccountText }}</strong>
        </div>
      </section>
      
      <el-table :data="strmConfigList" style="width: 100%" border stripe class="custom-table" row-key="id" @sort-change="handleSortChange" :default-sort="{ prop: 'id', order: 'ascending' }">
        <el-table-column prop="id" label="ID" width="60" align="center" sortable="custom" />
        <el-table-column label="115账号" min-width="120" align="center" sortable="custom" prop="cloud115_id">
          <template #default="scope">
            <el-tag type="info">{{ getCloud115Name(scope.row.cloud115_id) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="net_disk_path" label="网盘目录" min-width="180" show-overflow-tooltip />
        <el-table-column prop="local_path" label="本地目录" min-width="180" show-overflow-tooltip />
        <el-table-column prop="cron" label="Cron 配置" width="120" align="center">
          <template #default="scope">
            <el-tag v-if="scope.row.cron" type="warning">{{ scope.row.cron }}</el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="定时任务" width="100" align="center">
          <template #default="scope">
            <el-tag v-if="getCronTask(scope.row.id)" :type="getCronTask(scope.row.id).status === 'enabled' ? 'success' : 'info'">
              {{ getCronTask(scope.row.id).status === 'enabled' ? '已启用' : '已禁用' }}
            </el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="下次执行" width="160" align="center">
          <template #default="scope">
            <span v-if="getCronTask(scope.row.id) && getCronTask(scope.row.id).next_run_time">
              {{ formatTime(getCronTask(scope.row.id).next_run_time) }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="extension" label="后缀名" min-width="200" show-overflow-tooltip>
          <template #default="scope">
            <span class="extension-text">{{ scope.row.extension || '-' }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="create_time" label="创建时间" width="160" align="center" sortable="custom" />
        <el-table-column prop="update_time" label="更新时间" width="160" align="center" sortable="custom" />
        <el-table-column label="操作" min-width="320" fixed="right" align="center">
          <template #default="scope">
            <div class="action-buttons">
              <el-button size="small" type="primary" @click="handleEdit(scope.row)">
                <el-icon><Edit /></el-icon>
                编辑
              </el-button>
              <el-button size="small" type="danger" @click="handleDelete(scope.row.id)">
                <el-icon><Delete /></el-icon>
                删除
              </el-button>
              <el-button size="small" type="warning" @click="handleFullGenerate(scope.row.id)"
                :loading="isGenerating(scope.row.id)" :disabled="isGenerating(scope.row.id)">
                <el-icon v-if="!isGenerating(scope.row.id)"><Refresh /></el-icon>
                {{ isGenerating(scope.row.id) ? '生成中...' : '全量生成' }}
              </el-button>
              <el-dropdown v-if="getCronTask(scope.row.id)" trigger="click" @command="(cmd) => handleCronCommand(cmd, scope.row)">
                <el-button size="small" type="info">
                  <el-icon><Timer /></el-icon>
                  定时任务
                  <el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :command="'toggle'" :disabled="cronTaskLoading">
                      {{ getCronTask(scope.row.id).status === 'enabled' ? '禁用定时任务' : '启用定时任务' }}
                    </el-dropdown-item>
                    <el-dropdown-item :command="'run'" :disabled="cronTaskLoading">
                      立即执行
                    </el-dropdown-item>
                    <el-dropdown-item :command="'status'" divided>
                      查看详情
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 任务进度卡片 -->
    <transition name="slide-fade">
      <el-card v-if="taskInfo" shadow="hover" class="task-card">
        <template #header>
          <div class="task-header">
            <div class="task-title">
              <el-icon class="task-icon" :class="{ 'spin': taskInfo.status === 'running' }">
                <component :is="taskStatusIcon" />
              </el-icon>
              <span>{{ taskInfo.status === 'running' ? '正在生成 STRM 文件...' : taskStatusLabel }}</span>
            </div>
            <el-button circle text @click="clearTask">
              <el-icon><Close /></el-icon>
            </el-button>
          </div>
        </template>

        <div class="task-body">
          <!-- 进度条 -->
          <el-progress
            :percentage="taskProgress"
            :status="taskProgressStatus"
            :stroke-width="14"
            class="task-progress"
            :format="progressFormat"
          />

          <!-- 统计数据 -->
          <div class="task-stats">
            <div class="stat-item">
              <div class="stat-value total">{{ taskInfo.total_files || 0 }}</div>
              <div class="stat-label">总文件数</div>
            </div>
            <div class="stat-item">
              <div class="stat-value success">{{ taskInfo.success_files || 0 }}</div>
              <div class="stat-label">成功</div>
            </div>
            <div class="stat-item">
              <div class="stat-value failed">{{ taskInfo.failed_files || 0 }}</div>
              <div class="stat-label">失败</div>
            </div>
            <div class="stat-item">
              <div class="stat-value pending">{{ pendingCount }}</div>
              <div class="stat-label">待处理</div>
            </div>
          </div>

          <!-- 错误信息 -->
          <el-alert v-if="taskInfo.error_message" type="error" :title="taskInfo.error_message" show-icon :closable="false" class="task-error" />
        </div>
      </el-card>
    </transition>

    <!-- 新增/编辑配置对话框 -->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" append-to-body>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="115账号" prop="cloud115_id">
          <el-select v-model="form.cloud115_id" placeholder="请选择115账号">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        
        <el-form-item label="网盘目录" prop="net_disk_path">
          <el-input v-model="form.net_disk_path" placeholder="请输入网盘目录路径" />
        </el-form-item>
        
        <el-form-item label="本地目录" prop="local_path">
          <el-input v-model="form.local_path" placeholder="请输入本地目录路径" />
        </el-form-item>
        
        <el-form-item label="Cron表达式" prop="cron">
          <el-input v-model="form.cron" placeholder="请输入 Cron 表达式，例如：0 0 * * *" />
          <el-button type="text" @click="showCronPicker = true">快捷生成</el-button>
        </el-form-item>
        
        <el-form-item label="后缀名" prop="extension">
          <el-input v-model="form.extension" placeholder="请输入文件后缀名，多个用逗号分隔，如：.mp4,.mkv,.avi" />
        </el-form-item>
      </el-form>
      
      <!-- Cron 表达式快捷生成器 -->
      <el-dialog v-model="showCronPicker" title="Cron表达式快捷生成" width="400px">
        <el-form :model="cronForm">
          <el-form-item label="执行周期">
            <el-radio-group v-model="cronForm.period">
              <el-radio label="daily">每天</el-radio>
              <el-radio label="weekly">每周</el-radio>
              <el-radio label="monthly">每月</el-radio>
            </el-radio-group>
          </el-form-item>
          
          <el-form-item label="执行时间">
            <el-time-picker v-model="cronForm.time" type="time" format="HH:mm" value-format="HH:mm" placeholder="请选择执行时间" />
          </el-form-item>
        </el-form>
        <template #footer>
          <span class="dialog-footer">
            <el-button @click="showCronPicker = false">取消</el-button>
            <el-button type="primary" @click="generateCronExpression">生成</el-button>
          </span>
        </template>
      </el-dialog>
      
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, computed, onBeforeUnmount } from 'vue'
import { ElMessage } from 'element-plus'
import { Setting, Plus, Edit, Delete, Refresh, Close, CircleCheck, CircleClose, Loading, Timer, ArrowDown } from '@element-plus/icons-vue'
import { request } from '../utils/api'
import { showAlertDialog, showConfirmDialog } from '../utils/ui/messageBox'

const DEFAULT_EXTENSION = '.mp4,.avi,.mkv,.mov,.wmv,.flv,.webm,.m4v,.mpeg,.mpg,.3gp,.rmvb,.rm,.vob,.ts,.m2ts,.divx,.asf'

// STRM 配置列表
const strmConfigList = ref([])

// 115 账号列表
const cloud115List = ref([])

// Cron 任务列表
const cronTaskList = ref([])
const cronTaskLoading = ref(false)

// 排序状态
const sortField = ref('id')
const sortOrder = ref('asc')

// 对话框状态
const dialogVisible = ref(false)
const dialogTitle = ref('新增配置')
const showCronPicker = ref(false)

// 当前任务信息
const taskInfo = ref(null)
const currentTaskId = ref(null)
const generatingConfigId = ref(null)
let pollTimer = null

// 判断某个配置是否正在生成
const isGenerating = (configId) => {
  return generatingConfigId.value === configId && taskInfo.value?.status === 'running'
}

// 任务进度，直接使用后端返回的 progress 字段（0-100）
const taskProgress = computed(() => {
  if (!taskInfo.value) return 0
  if (taskInfo.value.status === 'completed') return 100
  if (taskInfo.value.status === 'failed') return taskInfo.value.progress || 0
  // 后端 progress 字段已经是 0-100 的值
  const p = taskInfo.value.progress || 0
  // running 状态至少显示 5%，表示任务已经开始
  return p === 0 && taskInfo.value.status === 'running' ? 5 : p
})

// Element Plus progress status
const taskProgressStatus = computed(() => {
  if (!taskInfo.value) return ''
  if (taskInfo.value.status === 'completed') return 'success'
  if (taskInfo.value.status === 'failed') return 'exception'
  return ''
})

// 待处理数量
const pendingCount = computed(() => {
  if (!taskInfo.value) return 0
  const total = taskInfo.value.total_files || 0
  const processed = taskInfo.value.processed_files || 0
  const pending = total - processed
  return pending > 0 ? pending : 0
})

// 任务状态图标
const taskStatusIcon = computed(() => {
  if (!taskInfo.value) return Loading
  if (taskInfo.value.status === 'completed') return CircleCheck
  if (taskInfo.value.status === 'failed') return CircleClose
  return Loading
})

// 任务状态标签
const taskStatusLabel = computed(() => {
  if (!taskInfo.value) return ''
  if (taskInfo.value.status === 'completed') return '生成完成'
  if (taskInfo.value.status === 'failed') return '生成失败'
  return '生成中...'
})

// 进度文本格式化
const progressFormat = (percentage) => {
  if (!taskInfo.value) return `${percentage}%`
  if (taskInfo.value.status === 'completed') return '完成'
  if (taskInfo.value.status === 'running' && taskInfo.value.total_files === 0) return '准备中...'
  return `${percentage}%`
}

const enabledCronCount = computed(() => cronTaskList.value.filter(task => task.status === 'enabled').length)
const nextRunSnapshot = computed(() => {
  const candidates = cronTaskList.value
    .filter(task => task.next_run_time)
    .sort((a, b) => new Date(a.next_run_time).getTime() - new Date(b.next_run_time).getTime())
  return candidates.length ? formatTime(candidates[0].next_run_time) : '暂无计划'
})
const primaryCloudAccountText = computed(() => {
  const config = strmConfigList.value[0]
  if (!config) return '暂无配置'
  return getCloud115Name(config.cloud115_id)
})
const configOverviewCards = computed(() => {
  const accounts = new Set(strmConfigList.value.map(item => item.cloud115_id).filter(Boolean))
  const runningTaskLabel = taskInfo.value?.status === 'running' ? '运行中' : taskInfo.value?.status === 'completed' ? '已完成' : taskInfo.value?.status === 'failed' ? '失败' : '空闲'
  return [
    {
      label: '配置总数',
      value: strmConfigList.value.length,
      hint: `${accounts.size} 个 115 账号参与 STRM 生成`
    },
    {
      label: 'Cron 任务',
      value: cronTaskList.value.length,
      hint: `${enabledCronCount.value} 个处于启用状态`
    },
    {
      label: '当前生成状态',
      value: runningTaskLabel,
      hint: taskInfo.value ? `成功 ${taskInfo.value.success_files || 0}，失败 ${taskInfo.value.failed_files || 0}` : '暂无正在跟踪的生成任务'
    },
    {
      label: '默认后缀示例',
      value: strmConfigList.value[0]?.extension || DEFAULT_EXTENSION,
      hint: '沿用原有扩展名配置提交到后端'
    }
  ]
})

// 开始轮询任务状态
const startPolling = (taskId) => {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const resp = await request(`/strm/task/${taskId}`)
      taskInfo.value = resp.data.data
      if (taskInfo.value.status === 'completed') {
        ElMessage.success('STRM 文件全量生成完成')
        generatingConfigId.value = null
        stopPolling()
      } else if (taskInfo.value.status === 'failed') {
        ElMessage.error(`STRM 文件生成失败：${taskInfo.value.error_message || '未知错误'}`)
        generatingConfigId.value = null
        stopPolling()
      }
    } catch (e) {
      // 如果任务不存在，则自动关闭进度状态
      const errorMsg = e.response?.data?.error || e.message || ''
      if (errorMsg.includes('Task not found')) {
        clearTask()
        generatingConfigId.value = null
      }
    }
  }, 2000)
}

// 停止轮询
const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// 清除任务信息
const clearTask = () => {
  taskInfo.value = null
  currentTaskId.value = null
  stopPolling()
}

// 表单数据
const formRef = ref(null)
const form = ref({
  id: '',
  cloud115_id: 0,
  net_disk_path: '',
  local_path: '',
  cron: '',
  extension: '.strm'
})

// Cron 生成器表单
const cronForm = ref({
  period: 'daily',
  time: ''
})

// 表单验证规则
const rules = {
  cloud115_id: [{ required: true, message: '请选择115账号', trigger: 'change' }],
  net_disk_path: [{ required: true, message: '请输入网盘目录', trigger: 'blur' }],
  local_path: [{ required: true, message: '请输入本地目录', trigger: 'blur' }],
  cron: [{ required: true, message: '请输入 Cron 表达式', trigger: 'blur' }],
  extension: [{ required: true, message: '请输入后缀名', trigger: 'blur' }]
}

// 获取 115 账号列表
const fetchCloud115List = async () => {
  try {
    const response = await request('/cloud115')
    cloud115List.value = response.data.data || []
  } catch (error) {
    ElMessage.error('获取115账号列表失败')
  }
}

// 获取 STRM 配置列表
const fetchStrmConfigList = async () => {
  try {
    const params = new URLSearchParams()
    params.append('sort_field', sortField.value)
    params.append('sort_order', sortOrder.value)
    const response = await request(`/strm/config?${params.toString()}`)
    const apiData = response.data.data
    strmConfigList.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (error) {
    ElMessage.error('获取 STRM 配置列表失败')
  }
}

// 获取 Cron 任务列表
const fetchCronTaskList = async () => {
  try {
    const response = await request('/cron/tasks')
    cronTaskList.value = response.data.data || []
  } catch (error) {
    console.error('获取 Cron 任务列表失败', error)
  }
}

// 根据配置 ID 获取对应的 Cron 任务
const getCronTask = (configId) => {
  return cronTaskList.value.find(task => task.strm_config_id === configId)
}

// 格式化时间
const formatTime = (time) => {
  if (!time) return '-'
  const date = new Date(time)
  return date.toLocaleString('zh-CN', {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit'
  })
}

/**
 * 处理表格排序变化
 * @param {Object} column - 列信息
 * @param {string} prop - 排序字段
 * @param {string} order - 排序方式
 */
const handleSortChange = ({ prop, order }) => {
  if (prop && order) {
    sortField.value = prop
    sortOrder.value = order === 'ascending' ? 'asc' : 'desc'
  } else {
    sortField.value = 'id'
    sortOrder.value = 'asc'
  }
  fetchStrmConfigList()
}

// 根据 ID 获取 115 账号名称
const getCloud115Name = (id) => {
  const account = cloud115List.value.find(item => item.id === id)
  return account ? account.name : '未知账号'
}

// 新增配置
const handleAdd = () => {
  dialogTitle.value = '新增配置'
  form.value = {
    id: '',
    cloud115_id: 0,
    net_disk_path: '',
    local_path: '',
    cron: '',
    extension: DEFAULT_EXTENSION
  }
  dialogVisible.value = true
}

// 编辑配置
const handleEdit = (row) => {
  dialogTitle.value = '编辑配置'
  form.value = {
    id: row.id,
    cloud115_id: parseInt(row.cloud115_id),
    net_disk_path: row.net_disk_path,
    local_path: row.local_path,
    cron: row.cron,
    extension: row.extension || '.strm'
  }
  dialogVisible.value = true
}

// 提交表单
const handleSubmit = () => {
  formRef.value.validate((valid) => {
    if (valid) {
      void submitConfig()
    }
  })
}

// 提交配置后等待列表刷新，保证表格和定时任务状态同步回显
const submitConfig = async () => {
  const requestData = {
    cloud115_id: form.value.cloud115_id,
    net_disk_path: form.value.net_disk_path,
    local_path: form.value.local_path,
    cron: form.value.cron,
    extension: form.value.extension
  }

  try {
    if (form.value.id) {
      await request(`/strm/config/${form.value.id}`, {
        method: 'PUT',
        data: requestData
      })
      ElMessage.success('配置更新成功')
    } else {
      await request('/strm/config', {
        method: 'POST',
        data: requestData
      })
      ElMessage.success('配置创建成功')
    }

    dialogVisible.value = false
    await fetchStrmConfigList()
    await fetchCronTaskList()
  } catch (error) {
    ElMessage.error(form.value.id ? '配置更新失败' : '配置创建失败')
  }
}

// 删除配置
const handleDelete = (id) => {
  showConfirmDialog('确定要删除这个配置吗？', '警告', {
    confirmButtonText: '确定',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(async () => {
    try {
      await request(`/strm/config/${id}`, {
        method: 'DELETE'
      })
      ElMessage.success('配置删除成功')
      await fetchStrmConfigList()
      await fetchCronTaskList()
    } catch (error) {
      ElMessage.error('配置删除失败')
    }
  }).catch(() => {})
}

// 全量生成 STRM 文件
const handleFullGenerate = (id) => {
  showConfirmDialog('确定要全量生成 STRM 文件吗？这将清除并重建全部 STRM 文件。', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消'
  }).then(() => {
    // 先清除之前的任务
    clearTask()
    generatingConfigId.value = id
    // 初始化任务状态为 running
    taskInfo.value = { status: 'running', total_files: 0, processed_files: 0, success_files: 0, failed_files: 0 }

    request(`/strm/config/${id}/generate/full`, {
      method: 'POST'
    }).then((resp) => {
      const taskId = resp.data.task_id
      if (taskId) {
        currentTaskId.value = taskId
        startPolling(taskId)
      } else {
        ElMessage.success('全量生成 STRM 文件成功')
        clearTask()
      }
    }).catch(() => {
      ElMessage.error('全量生成 STRM 文件失败')
      clearTask()
    })
  }).catch(() => {})
}

// 处理定时任务下拉菜单命令
const handleCronCommand = (command, row) => {
  const task = getCronTask(row.id)
  if (!task) return

  switch (command) {
    case 'toggle':
      handleToggleCronTask(task)
      break
    case 'run':
      handleRunCronTask(task)
      break
    case 'status':
      showCronTaskStatus(task)
      break
  }
}

// 切换定时任务状态
const handleToggleCronTask = async (task) => {
  const newStatus = task.status === 'enabled' ? 'disabled' : 'enabled'
  const actionText = newStatus === 'enabled' ? '启用' : '禁用'
  
  try {
    cronTaskLoading.value = true
    await request(`/cron/task/${task.id}`, {
      method: 'PUT',
      data: {
        cron_expr: task.cron_expr,
        status: newStatus
      }
    })
    ElMessage.success(`定时任务已${actionText}`)
    await fetchCronTaskList()
  } catch (error) {
    ElMessage.error(`${actionText}定时任务失败`)
  } finally {
    cronTaskLoading.value = false
  }
}

// 立即执行定时任务
const handleRunCronTask = async (task) => {
  try {
    cronTaskLoading.value = true
    await request(`/cron/task/${task.id}/run`, {
      method: 'POST'
    })
    ElMessage.success('定时任务已触发执行，请查看任务进度')
    // 刷新任务列表
    await fetchCronTaskList()
  } catch (error) {
    ElMessage.error('执行定时任务失败')
  } finally {
    cronTaskLoading.value = false
  }
}

// 显示定时任务详情
const showCronTaskStatus = (task) => {
  const statusText = task.status === 'enabled' ? '已启用' : '已禁用'
  const lastRunTime = task.last_run_time ? formatTime(task.last_run_time) : '从未执行'
  const nextRunTime = task.next_run_time ? formatTime(task.next_run_time) : '-'
  const lastRunStatus = task.last_run_status || '-'
  const lastRunMessage = task.last_run_message || '-'
  
  showAlertDialog(
    `<div style="line-height: 2;">
      <p><strong>任务名称：</strong>${task.task_name}</p>
      <p><strong>任务状态：</strong>${statusText}</p>
      <p><strong>Cron表达式：</strong>${task.cron_expr}</p>
      <p><strong>上次执行时间：</strong>${lastRunTime}</p>
      <p><strong>上次执行状态：</strong>${lastRunStatus}</p>
      <p><strong>上次执行结果：</strong>${lastRunMessage}</p>
      <p><strong>下次执行时间：</strong>${nextRunTime}</p>
    </div>`,
    '定时任务详情',
    {
      dangerouslyUseHTMLString: true,
      confirmButtonText: '关闭'
    }
  )
}

// 生成 Cron 表达式
const generateCronExpression = () => {
  const time = cronForm.value.time
  if (!time || typeof time !== 'string') {
    ElMessage.error('请选择执行时间')
    return
  }
  
  try {
    const parts = time.split(':')
    if (parts.length !== 2) {
      ElMessage.error('时间格式错误')
      return
    }
    const [hour, minute] = parts
    let cronExpression = ''
    
    switch (cronForm.value.period) {
      case 'daily':
        cronExpression = `${minute} ${hour} * * *`
        break
      case 'weekly':
        cronExpression = `${minute} ${hour} * * 0`
        break
      case 'monthly':
        cronExpression = `${minute} ${hour} 1 * *`
        break
      default:
        cronExpression = `${minute} ${hour} * * *`
        break
    }
    
    form.value.cron = cronExpression
    showCronPicker.value = false
  } catch (error) {
    ElMessage.error('生成 Cron 表达式失败')
    console.error('Error generating cron expression:', error)
  }
}

// 初始化
onMounted(() => {
  fetchCloud115List()
  fetchStrmConfigList()
  fetchCronTaskList()
})

// 卸载时清理定时器
onBeforeUnmount(() => {
  stopPolling()
})
</script>

<style scoped>
.strm-config-container {
  padding: 8px 0 0;
  min-height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.main-card {
  border-radius: 24px;
  overflow: hidden;
  border: 1px solid rgba(120, 101, 72, 0.12);
  background: rgba(255, 252, 247, 0.84);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

.main-card :deep(.el-card__header) {
  background:
    radial-gradient(circle at top right, rgba(242, 166, 90, 0.28), transparent 32%),
    linear-gradient(135deg, #1f6f78 0%, #24535f 55%, #17313a 100%);
  padding: 20px 24px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-size: 22px;
  font-weight: 700;
}

.header-icon {
  font-size: 24px;
}

.custom-table {
  border-radius: 18px;
  overflow: hidden;
}

.overview-panel {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(0, 2fr);
  gap: 18px;
  margin-bottom: 22px;
}

.overview-copy {
  padding: 20px;
  border-radius: 22px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.overview-copy h2 {
  margin: 0;
  font-size: 24px;
  color: #17313a;
}

.overview-copy p {
  margin: 12px 0 0;
  line-height: 1.7;
  color: #6c6259;
}

.overview-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.overview-card {
  padding: 18px;
  border-radius: 20px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(247, 241, 231, 0.92));
  border: 1px solid rgba(120, 101, 72, 0.1);
}

.overview-card__label {
  display: block;
  color: #8a7b6d;
  font-size: 13px;
}

.overview-card__value {
  display: block;
  margin-top: 12px;
  color: #17313a;
  font-size: 28px;
  line-height: 1.2;
  word-break: break-all;
}

.overview-card__hint {
  margin: 10px 0 0;
  color: #73675d;
  line-height: 1.6;
}

.status-rail {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  margin-bottom: 22px;
}

.status-rail__item {
  padding: 16px 18px;
  border-radius: 18px;
  background: rgba(244, 239, 231, 0.88);
  border: 1px solid rgba(120, 101, 72, 0.08);
}

.status-rail__item span {
  display: block;
  color: #8a7b6d;
  font-size: 12px;
}

.status-rail__item strong {
  display: block;
  margin-top: 8px;
  color: #17313a;
  font-size: 18px;
  line-height: 1.5;
}

.custom-table :deep(.el-table__header th) {
  background-color: #f7f1e7 !important;
  color: #4d453d;
  font-weight: 600;
}

.custom-table :deep(.el-table__row:hover > td) {
  background-color: #f8f2e8 !important;
}

.extension-text {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  color: #606266;
}

.action-buttons {
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  justify-content: center;
  white-space: nowrap;
}

.dialog-footer {
  width: 100%;
  display: flex;
  justify-content: flex-end;
}

/* 任务进度卡片 */
.task-card {
  border-radius: 20px;
  overflow: hidden;
  border: 1px solid rgba(120, 101, 72, 0.12);
  background: rgba(255, 252, 247, 0.84);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

.task-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #17313a 0%, #24535f 100%);
  padding: 14px 20px;
}

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.task-title {
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-size: 16px;
  font-weight: 600;
}

.task-icon {
  font-size: 20px;
  color: #38ef7d;
}

.task-icon.spin {
  animation: spin 1.2s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.task-body {
  padding: 20px 10px 8px;
}

.task-progress {
  margin-bottom: 24px;
}

.task-progress :deep(.el-progress-bar__outer) {
  border-radius: 8px;
  background-color: #eef0f5;
}

.task-progress :deep(.el-progress-bar__inner) {
  border-radius: 8px;
  background: linear-gradient(90deg, #11998e, #38ef7d);
  transition: width 0.5s ease;
}

.task-stats {
  display: flex;
  justify-content: space-around;
  padding: 12px 0 4px;
}

.stat-item {
  text-align: center;
}

.stat-value {
  font-size: 28px;
  font-weight: 700;
  line-height: 1.1;
}

.stat-value.total { color: #409eff; }
.stat-value.success { color: #67c23a; }
.stat-value.failed { color: #f56c6c; }
.stat-value.pending { color: #e6a23c; }

.stat-label {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
}

.task-error {
  margin-top: 16px;
}

/* 过渡动画 */
.slide-fade-enter-active {
  transition: all 0.4s cubic-bezier(0.22, 1, 0.36, 1);
}
.slide-fade-leave-active {
  transition: all 0.25s ease;
}
.slide-fade-enter-from {
  opacity: 0;
  transform: translateY(-16px);
}
.slide-fade-leave-to {
  opacity: 0;
  transform: translateY(-8px);
}

:global(.dark) .main-card,
:global(.dark) .task-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

:global(.dark) .overview-copy,
:global(.dark) .overview-card,
:global(.dark) .status-rail__item {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .overview-copy h2,
:global(.dark) .overview-card__value,
:global(.dark) .status-rail__item strong,
:global(.dark) .extension-text {
  color: #e8edf4;
}

:global(.dark) .overview-copy p,
:global(.dark) .overview-card__hint,
:global(.dark) .overview-card__label,
:global(.dark) .status-rail__item span,
:global(.dark) .stat-label {
  color: #9faebb;
}

:global(.dark) .custom-table :deep(.el-table__header th) {
  background-color: #182231 !important;
  color: #d6deea;
}

:global(.dark) .custom-table :deep(.el-table__row:hover > td) {
  background-color: #14202d !important;
}

@media (max-width: 960px) {
  .overview-panel,
  .overview-grid,
  .status-rail {
    grid-template-columns: 1fr;
  }
}
</style>

