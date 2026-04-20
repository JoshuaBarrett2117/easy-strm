<template>
  <div class="strm-config-container">
    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><Setting /></el-icon>
            <span>STRM鏂囦欢閰嶇疆绠＄悊</span>
          </div>
          <el-button type="primary" @click="handleAdd">
            <el-icon><Plus /></el-icon>
            鏂板閰嶇疆
          </el-button>
        </div>
      </template>
      
      <el-table :data="strmConfigList" style="width: 100%" border stripe class="custom-table" row-key="id" @sort-change="handleSortChange" :default-sort="{ prop: 'id', order: 'ascending' }">
        <el-table-column prop="id" label="ID" width="60" align="center" sortable="custom" />
        <el-table-column label="115璐﹀彿" min-width="120" align="center" sortable="custom" prop="cloud115_id">
          <template #default="scope">
            <el-tag type="info">{{ getCloud115Name(scope.row.cloud115_id) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="net_disk_path" label="缃戠洏鐩綍" min-width="180" show-overflow-tooltip />
        <el-table-column prop="local_path" label="鏈湴鐩綍" min-width="180" show-overflow-tooltip />
        <el-table-column prop="cron" label="Cron閰嶇疆" width="120" align="center">
          <template #default="scope">
            <el-tag v-if="scope.row.cron" type="warning">{{ scope.row.cron }}</el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="瀹氭椂浠诲姟" width="100" align="center">
          <template #default="scope">
            <el-tag v-if="getCronTask(scope.row.id)" :type="getCronTask(scope.row.id).status === 'enabled' ? 'success' : 'info'">
              {{ getCronTask(scope.row.id).status === 'enabled' ? '已启用' : '已禁用' }}
            </el-tag>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="涓嬫鎵ц" width="160" align="center">
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
        <el-table-column prop="create_time" label="鍒涘缓鏃堕棿" width="160" align="center" sortable="custom" />
        <el-table-column prop="update_time" label="鏇存柊鏃堕棿" width="160" align="center" sortable="custom" />
        <el-table-column label="鎿嶄綔" min-width="320" fixed="right" align="center">
          <template #default="scope">
            <div class="action-buttons">
              <el-button size="small" type="primary" @click="handleEdit(scope.row)">
                <el-icon><Edit /></el-icon>
                缂栬緫
              </el-button>
              <el-button size="small" type="danger" @click="handleDelete(scope.row.id)">
                <el-icon><Delete /></el-icon>
                鍒犻櫎
              </el-button>
              <el-button size="small" type="warning" @click="handleFullGenerate(scope.row.id)"
                :loading="isGenerating(scope.row.id)" :disabled="isGenerating(scope.row.id)">
                <el-icon v-if="!isGenerating(scope.row.id)"><Refresh /></el-icon>
                {{ isGenerating(scope.row.id) ? '鐢熸垚涓?..' : '鍏ㄩ噺鐢熸垚' }}
              </el-button>
              <el-dropdown v-if="getCronTask(scope.row.id)" trigger="click" @command="(cmd) => handleCronCommand(cmd, scope.row)">
                <el-button size="small" type="info">
                  <el-icon><Timer /></el-icon>
                  瀹氭椂浠诲姟
                  <el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item :command="'toggle'" :disabled="cronTaskLoading">
                      {{ getCronTask(scope.row.id).status === 'enabled' ? '绂佺敤瀹氭椂浠诲姟' : '鍚敤瀹氭椂浠诲姟' }}
                    </el-dropdown-item>
                    <el-dropdown-item :command="'run'" :disabled="cronTaskLoading">
                      绔嬪嵆鎵ц
                    </el-dropdown-item>
                    <el-dropdown-item :command="'status'" divided>
                      鏌ョ湅璇︽儏
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <!-- 浠诲姟杩涘害鍗＄墖 -->
    <transition name="slide-fade">
      <el-card v-if="taskInfo" shadow="hover" class="task-card">
        <template #header>
          <div class="task-header">
            <div class="task-title">
              <el-icon class="task-icon" :class="{ 'spin': taskInfo.status === 'running' }">
                <component :is="taskStatusIcon" />
              </el-icon>
              <span>{{ taskInfo.status === 'running' ? '姝ｅ湪鐢熸垚 STRM 鏂囦欢...' : taskStatusLabel }}</span>
            </div>
            <el-button circle text @click="clearTask">
              <el-icon><Close /></el-icon>
            </el-button>
          </div>
        </template>

        <div class="task-body">
          <!-- 杩涘害鏉?-->
          <el-progress
            :percentage="taskProgress"
            :status="taskProgressStatus"
            :stroke-width="14"
            class="task-progress"
            :format="progressFormat"
          />

          <!-- 缁熻鏁版嵁 -->
          <div class="task-stats">
            <div class="stat-item">
              <div class="stat-value total">{{ taskInfo.total_files || 0 }}</div>
              <div class="stat-label">鎬绘枃浠舵暟</div>
            </div>
            <div class="stat-item">
              <div class="stat-value success">{{ taskInfo.success_files || 0 }}</div>
              <div class="stat-label">鎴愬姛</div>
            </div>
            <div class="stat-item">
              <div class="stat-value failed">{{ taskInfo.failed_files || 0 }}</div>
              <div class="stat-label">澶辫触</div>
            </div>
            <div class="stat-item">
              <div class="stat-value pending">{{ pendingCount }}</div>
              <div class="stat-label">待处理</div>
            </div>
          </div>

          <!-- 閿欒淇℃伅 -->
          <el-alert v-if="taskInfo.error_message" type="error" :title="taskInfo.error_message" show-icon :closable="false" class="task-error" />
        </div>
      </el-card>
    </transition>

    <!-- 鏂板/缂栬緫瀵硅瘽妗?-->
    <el-dialog v-model="dialogVisible" :title="dialogTitle" width="600px" append-to-body>
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="115璐﹀彿" prop="cloud115_id">
          <el-select v-model="form.cloud115_id" placeholder="璇烽€夋嫨115璐﹀彿">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        
        <el-form-item label="缃戠洏鐩綍" prop="net_disk_path">
          <el-input v-model="form.net_disk_path" placeholder="请输入网盘目录路径" />
        </el-form-item>
        
        <el-form-item label="鏈湴鐩綍" prop="local_path">
          <el-input v-model="form.local_path" placeholder="请输入本地目录路径" />
        </el-form-item>
        
        <el-form-item label="Cron表达式" prop="cron">
          <el-input v-model="form.cron" placeholder="璇疯緭鍏ron琛ㄨ揪寮忥紝濡傦細0 0 * * *" />
          <el-button type="text" @click="showCronPicker = true">蹇嵎鐢熸垚</el-button>
        </el-form-item>
        
        <el-form-item label="后缀名" prop="extension">
          <el-input v-model="form.extension" placeholder="请输入文件后缀名，多个用逗号分隔，如：.mp4,.mkv,.avi" />
        </el-form-item>
      </el-form>
      
      <!-- cron琛ㄨ揪寮忓揩鎹风敓鎴愬櫒 -->
      <el-dialog v-model="showCronPicker" title="Cron表达式快捷生成" width="400px">
        <el-form :model="cronForm">
          <el-form-item label="鎵ц鍛ㄦ湡">
            <el-radio-group v-model="cronForm.period">
              <el-radio label="daily">姣忓ぉ</el-radio>
              <el-radio label="weekly">姣忓懆</el-radio>
              <el-radio label="monthly">姣忔湀</el-radio>
            </el-radio-group>
          </el-form-item>
          
          <el-form-item label="鎵ц鏃堕棿">
            <el-time-picker v-model="cronForm.time" type="time" format="HH:mm" value-format="HH:mm" placeholder="璇烽€夋嫨鎵ц鏃堕棿" />
          </el-form-item>
        </el-form>
        <template #footer>
          <span class="dialog-footer">
            <el-button @click="showCronPicker = false">鍙栨秷</el-button>
            <el-button type="primary" @click="generateCronExpression">鐢熸垚</el-button>
          </span>
        </template>
      </el-dialog>
      
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">鍙栨秷</el-button>
          <el-button type="primary" @click="handleSubmit">纭畾</el-button>
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

// STRM閰嶇疆鍒楄〃
const strmConfigList = ref([])

// 115璐﹀彿鍒楄〃
const cloud115List = ref([])

// Cron浠诲姟鍒楄〃
const cronTaskList = ref([])
const cronTaskLoading = ref(false)

// 鎺掑簭鐘舵€?
const sortField = ref('id')
const sortOrder = ref('asc')

// 瀵硅瘽妗嗙姸鎬?
const dialogVisible = ref(false)
const dialogTitle = ref('鏂板閰嶇疆')
const showCronPicker = ref(false)

// 褰撳墠浠诲姟淇℃伅
const taskInfo = ref(null)
const currentTaskId = ref(null)
const generatingConfigId = ref(null)
let pollTimer = null

// 鍒ゆ柇鏌愪釜閰嶇疆鏄惁姝ｅ湪鐢熸垚
const isGenerating = (configId) => {
  return generatingConfigId.value === configId && taskInfo.value?.status === 'running'
}

// 浠诲姟杩涘害 鈥斺€?鐩存帴浣跨敤鍚庣杩斿洖鐨?progress 瀛楁锛?-100锛?
const taskProgress = computed(() => {
  if (!taskInfo.value) return 0
  if (taskInfo.value.status === 'completed') return 100
  if (taskInfo.value.status === 'failed') return taskInfo.value.progress || 0
  // 鍚庣 progress 瀛楁宸茬粡鏄?0-100 鐨勫€?
  const p = taskInfo.value.progress || 0
  // running 鐘舵€佽嚦灏戞樉绀?5% 琛ㄧず宸茬粡寮€濮嬩簡
  return p === 0 && taskInfo.value.status === 'running' ? 5 : p
})

// Element Plus progress status
const taskProgressStatus = computed(() => {
  if (!taskInfo.value) return ''
  if (taskInfo.value.status === 'completed') return 'success'
  if (taskInfo.value.status === 'failed') return 'exception'
  return ''
})

// 寰呭鐞嗘暟閲?
const pendingCount = computed(() => {
  if (!taskInfo.value) return 0
  const total = taskInfo.value.total_files || 0
  const processed = taskInfo.value.processed_files || 0
  const pending = total - processed
  return pending > 0 ? pending : 0
})

// 浠诲姟鐘舵€佸浘鏍?
const taskStatusIcon = computed(() => {
  if (!taskInfo.value) return Loading
  if (taskInfo.value.status === 'completed') return CircleCheck
  if (taskInfo.value.status === 'failed') return CircleClose
  return Loading
})

// 浠诲姟鐘舵€佹爣绛?
const taskStatusLabel = computed(() => {
  if (!taskInfo.value) return ''
  if (taskInfo.value.status === 'completed') return '生成完成'
  if (taskInfo.value.status === 'failed') return '鐢熸垚澶辫触'
  return '鐢熸垚涓?..'
})

// 杩涘害鏍煎紡鍖?
const progressFormat = (percentage) => {
  if (!taskInfo.value) return `${percentage}%`
  if (taskInfo.value.status === 'completed') return '瀹屾垚'
  if (taskInfo.value.status === 'running' && taskInfo.value.total_files === 0) return '鍑嗗涓?..'
  return `${percentage}%`
}

// 寮€濮嬭疆璇换鍔＄姸鎬?
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
      // 濡傛灉浠诲姟涓嶅瓨鍦紝鑷姩鍏抽棴杩涘害鏉?
      const errorMsg = e.response?.data?.error || e.message || ''
      if (errorMsg.includes('Task not found')) {
        clearTask()
        generatingConfigId.value = null
      }
    }
  }, 2000)
}

// 鍋滄杞
const stopPolling = () => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

// 娓呴櫎浠诲姟淇℃伅
const clearTask = () => {
  taskInfo.value = null
  currentTaskId.value = null
  stopPolling()
}

// 琛ㄥ崟鏁版嵁
const formRef = ref(null)
const form = ref({
  id: '',
  cloud115_id: 0,
  net_disk_path: '',
  local_path: '',
  cron: '',
  extension: '.strm'
})

// cron鐢熸垚鍣ㄨ〃鍗?
const cronForm = ref({
  period: 'daily',
  time: ''
})

// 琛ㄥ崟楠岃瘉瑙勫垯
const rules = {
  cloud115_id: [{ required: true, message: '璇烽€夋嫨115璐﹀彿', trigger: 'change' }],
  net_disk_path: [{ required: true, message: '请输入网盘目录', trigger: 'blur' }],
  local_path: [{ required: true, message: '请输入本地目录', trigger: 'blur' }],
  cron: [{ required: true, message: '请输入 Cron 表达式', trigger: 'blur' }],
  extension: [{ required: true, message: '请输入后缀名', trigger: 'blur' }]
}

// 鑾峰彇115璐﹀彿鍒楄〃
const fetchCloud115List = async () => {
  try {
    const response = await request('/cloud115')
    cloud115List.value = response.data.data || []
  } catch (error) {
    ElMessage.error('鑾峰彇115璐﹀彿鍒楄〃澶辫触')
  }
}

// 鑾峰彇STRM閰嶇疆鍒楄〃
const fetchStrmConfigList = async () => {
  try {
    const params = new URLSearchParams()
    params.append('sort_field', sortField.value)
    params.append('sort_order', sortOrder.value)
    const response = await request(`/strm/config?${params.toString()}`)
    const apiData = response.data.data
    strmConfigList.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (error) {
    ElMessage.error('鑾峰彇STRM閰嶇疆鍒楄〃澶辫触')
  }
}

// 鑾峰彇Cron浠诲姟鍒楄〃
const fetchCronTaskList = async () => {
  try {
    const response = await request('/cron/tasks')
    cronTaskList.value = response.data.data || []
  } catch (error) {
    console.error('鑾峰彇Cron浠诲姟鍒楄〃澶辫触', error)
  }
}

// 鏍规嵁閰嶇疆ID鑾峰彇瀵瑰簲鐨凜ron浠诲姟
const getCronTask = (configId) => {
  return cronTaskList.value.find(task => task.strm_config_id === configId)
}

// 鏍煎紡鍖栨椂闂?
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
 * 澶勭悊琛ㄦ牸鎺掑簭鍙樺寲
 * @param {Object} column - 鍒椾俊鎭?
 * @param {string} prop - 鎺掑簭瀛楁
 * @param {string} order - 鎺掑簭鏂瑰紡
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

// 鏍规嵁ID鑾峰彇115璐﹀彿鍚嶇О
const getCloud115Name = (id) => {
  const account = cloud115List.value.find(item => item.id === id)
  return account ? account.name : '鏈煡璐﹀彿'
}

// 鏂板閰嶇疆
const handleAdd = () => {
  dialogTitle.value = '鏂板閰嶇疆'
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

// 缂栬緫閰嶇疆
const handleEdit = (row) => {
  dialogTitle.value = '缂栬緫閰嶇疆'
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

// 鎻愪氦琛ㄥ崟
const handleSubmit = () => {
  formRef.value.validate((valid) => {
    if (valid) {
        const requestData = {
          cloud115_id: form.value.cloud115_id,
          net_disk_path: form.value.net_disk_path,
          local_path: form.value.local_path,
          cron: form.value.cron,
          extension: form.value.extension
        }
      
      if (form.value.id) {
        request(`/strm/config/${form.value.id}`, {
          method: 'PUT',
          data: requestData
        }).then(() => {
          ElMessage.success('閰嶇疆鏇存柊鎴愬姛')
          dialogVisible.value = false
          fetchStrmConfigList()
          fetchCronTaskList()
        }).catch(() => {
          ElMessage.error('閰嶇疆鏇存柊澶辫触')
        })
      } else {
        request('/strm/config', {
          method: 'POST',
          data: requestData
        }).then(() => {
          ElMessage.success('閰嶇疆鍒涘缓鎴愬姛')
          dialogVisible.value = false
          fetchStrmConfigList()
          fetchCronTaskList()
        }).catch(() => {
          ElMessage.error('閰嶇疆鍒涘缓澶辫触')
        })
      }
    }
  })
}

// 鍒犻櫎閰嶇疆
const handleDelete = (id) => {
  showConfirmDialog('确定要删除这个配置吗？', '警告', {
    confirmButtonText: '纭畾',
    cancelButtonText: '鍙栨秷',
    type: 'warning'
  }).then(() => {
    request(`/strm/config/${id}`, {
      method: 'DELETE'
    }).then(() => {
      ElMessage.success('閰嶇疆鍒犻櫎鎴愬姛')
      fetchStrmConfigList()
      fetchCronTaskList()
    }).catch(() => {
      ElMessage.error('閰嶇疆鍒犻櫎澶辫触')
    })
  }).catch(() => {})
}

// 鍏ㄩ噺鐢熸垚STRM鏂囦欢
const handleFullGenerate = (id) => {
  showConfirmDialog('确定要全量生成 STRM 文件吗？这将清除并重建全部 STRM 文件。', '提示', {
    confirmButtonText: '纭畾',
    cancelButtonText: '鍙栨秷'
  }).then(() => {
    // 鍏堟竻闄や箣鍓嶇殑浠诲姟
    clearTask()
    generatingConfigId.value = id
    // 鍒濆鍖栦换鍔＄姸鎬佷负 running
    taskInfo.value = { status: 'running', total_files: 0, processed_files: 0, success_files: 0, failed_files: 0 }

    request(`/strm/config/${id}/generate/full`, {
      method: 'POST'
    }).then((resp) => {
      const taskId = resp.data.task_id
      if (taskId) {
        currentTaskId.value = taskId
        startPolling(taskId)
      } else {
        ElMessage.success('鍏ㄩ噺鐢熸垚STRM鏂囦欢鎴愬姛')
        clearTask()
      }
    }).catch(() => {
      ElMessage.error('鍏ㄩ噺鐢熸垚STRM鏂囦欢澶辫触')
      clearTask()
    })
  }).catch(() => {})
}

// 澶勭悊瀹氭椂浠诲姟涓嬫媺鑿滃崟鍛戒护
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

// 鍒囨崲瀹氭椂浠诲姟鐘舵€?
const handleToggleCronTask = async (task) => {
  const newStatus = task.status === 'enabled' ? 'disabled' : 'enabled'
  const actionText = newStatus === 'enabled' ? '鍚敤' : '绂佺敤'
  
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
    fetchCronTaskList()
  } catch (error) {
    ElMessage.error(`${actionText}瀹氭椂浠诲姟澶辫触`)
  } finally {
    cronTaskLoading.value = false
  }
}

// 绔嬪嵆鎵ц瀹氭椂浠诲姟
const handleRunCronTask = async (task) => {
  try {
    cronTaskLoading.value = true
    await request(`/cron/task/${task.id}/run`, {
      method: 'POST'
    })
    ElMessage.success('定时任务已触发执行，请查看任务进度')
    // 鍒锋柊浠诲姟鍒楄〃
    fetchCronTaskList()
  } catch (error) {
    ElMessage.error('鎵ц瀹氭椂浠诲姟澶辫触')
  } finally {
    cronTaskLoading.value = false
  }
}

// 鏄剧ず瀹氭椂浠诲姟璇︽儏
const showCronTaskStatus = (task) => {
  const statusText = task.status === 'enabled' ? '已启用' : '已禁用'
  const lastRunTime = task.last_run_time ? formatTime(task.last_run_time) : '浠庢湭鎵ц'
  const nextRunTime = task.next_run_time ? formatTime(task.next_run_time) : '-'
  const lastRunStatus = task.last_run_status || '-'
  const lastRunMessage = task.last_run_message || '-'
  
  showAlertDialog(
    `<div style="line-height: 2;">
      <p><strong>任务名称：</strong>${task.task_name}</p>
      <p><strong>浠诲姟鐘舵€侊細</strong>${statusText}</p>
      <p><strong>Cron琛ㄨ揪寮忥細</strong>${task.cron_expr}</p>
      <p><strong>上次执行时间：</strong>${lastRunTime}</p>
      <p><strong>涓婃鎵ц鐘舵€侊細</strong>${lastRunStatus}</p>
      <p><strong>上次执行结果：</strong>${lastRunMessage}</p>
      <p><strong>下次执行时间：</strong>${nextRunTime}</p>
    </div>`,
    '瀹氭椂浠诲姟璇︽儏',
    {
      dangerouslyUseHTMLString: true,
      confirmButtonText: '鍏抽棴'
    }
  )
}

// 鐢熸垚cron琛ㄨ揪寮?
const generateCronExpression = () => {
  const time = cronForm.value.time
  if (!time || typeof time !== 'string') {
    ElMessage.error('璇烽€夋嫨鎵ц鏃堕棿')
    return
  }
  
  try {
    const parts = time.split(':')
    if (parts.length !== 2) {
      ElMessage.error('鏃堕棿鏍煎紡閿欒')
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

// 鍒濆鍖?
onMounted(() => {
  fetchCloud115List()
  fetchStrmConfigList()
  fetchCronTaskList()
})

// 閿€姣佹椂娓呯悊瀹氭椂鍣?
onBeforeUnmount(() => {
  stopPolling()
})
</script>

<style scoped>
.strm-config-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.main-card {
  border-radius: 12px;
  overflow: hidden;
}

.main-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
  padding: 16px 20px;
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
  font-size: 18px;
  font-weight: 600;
}

.header-icon {
  font-size: 22px;
}

.custom-table {
  border-radius: 8px;
  overflow: hidden;
}

.custom-table :deep(.el-table__header th) {
  background-color: #f8f9fa !important;
  color: #495057;
  font-weight: 600;
}

.custom-table :deep(.el-table__row:hover > td) {
  background-color: #e8f8f0 !important;
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

/* 浠诲姟杩涘害鍗＄墖 */
.task-card {
  border-radius: 12px;
  overflow: hidden;
  border: 1px solid #dde3e8;
}

.task-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 100%);
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

/* 杩囨浮鍔ㄧ敾 */
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
</style>

