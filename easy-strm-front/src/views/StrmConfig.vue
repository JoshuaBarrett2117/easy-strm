<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">STRM Center</div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">STRM 文件配置中心</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            STRM 配置、Cron 状态、全量生成任务和即时执行入口都保留原有后端流程。
          </p>
        </div>
        <n-button type="primary" @click="handleAdd">
          <template #icon>
            <n-icon :component="AddOutline" />
          </template>
          新增配置
        </n-button>
      </div>
    </div>

    <!-- 配置与任务总览 -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in configOverviewCards"
        :key="card.label"
        :label="card.label"
        :value="card.value"
        :hint="card.hint"
        :icon="card.icon"
        :tone="card.tone"
      />
    </div>

    <!-- 状态速览 -->
    <div class="grid grid-cols-1 gap-3 sm:grid-cols-3 lg:gap-4">
      <div
        v-for="item in statusRailItems"
        :key="item.label"
        class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-white/5 dark:bg-ink-900"
      >
        <p class="text-xs text-slate-400 dark:text-slate-500">{{ item.label }}</p>
        <p class="mt-1.5 text-sm font-bold text-slate-800 dark:text-white">{{ item.value }}</p>
      </div>
    </div>

    <!-- 任务进度卡片 -->
    <Transition
      enter-active-class="transition duration-300 ease-out"
      enter-from-class="-translate-y-4 opacity-0"
      leave-active-class="transition duration-200 ease-in"
      leave-to-class="-translate-y-2 opacity-0"
    >
      <div
        v-if="taskInfo"
        class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-5"
      >
        <div class="mb-4 flex items-center justify-between gap-3">
          <div class="flex items-center gap-2.5">
            <n-icon
              size="20"
              :component="taskStatusIcon"
              :class="[taskStatusIconClass, { 'animate-spin': taskInfo.status === 'running' }]"
            />
            <span class="text-sm font-bold text-slate-800 dark:text-white">
              {{ taskInfo.status === 'running' ? '正在生成 STRM 文件...' : taskStatusLabel }}
            </span>
          </div>
          <n-button text circle @click="clearTask">
            <template #icon>
              <n-icon :component="CloseOutline" />
            </template>
          </n-button>
        </div>

        <!-- 进度条 -->
        <n-progress type="line" :percentage="taskProgress" :status="taskProgressStatus" :height="14" processing>
          {{ progressText }}
        </n-progress>

        <!-- 统计数据 -->
        <div class="mt-5 grid grid-cols-4 gap-2 text-center">
          <div>
            <div class="text-2xl font-bold tabular-nums text-cyan-600 dark:text-cyan-400">{{ taskInfo.total_files || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">总文件数</div>
          </div>
          <div>
            <div class="text-2xl font-bold tabular-nums text-emerald-600 dark:text-emerald-400">{{ taskInfo.success_files || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">成功</div>
          </div>
          <div>
            <div class="text-2xl font-bold tabular-nums text-red-500">{{ taskInfo.failed_files || 0 }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">失败</div>
          </div>
          <div>
            <div class="text-2xl font-bold tabular-nums text-amber-500">{{ pendingCount }}</div>
            <div class="mt-1 text-xs text-slate-400 dark:text-slate-500">待处理</div>
          </div>
        </div>

        <!-- 错误信息 -->
        <n-alert v-if="taskInfo.error_message" type="error" :title="taskInfo.error_message" class="mt-4" />
      </div>
    </Transition>

    <!-- 配置列表 -->
    <PageCard title="配置列表" subtitle="支持编辑、全量生成与定时任务管理">
      <div class="overflow-x-auto">
        <n-data-table
          :columns="columns"
          :data="strmConfigList"
          :row-key="(row) => row.id"
          :scroll-x="1460"
          size="small"
          @update:sorter="handleSorterChange"
        >
          <template #empty>
            <EmptyState title="暂无 STRM 配置" description="点击右上角「新增配置」创建第一个生成任务">
              <n-button type="primary" size="small" @click="handleAdd">新增配置</n-button>
            </EmptyState>
          </template>
        </n-data-table>
      </div>
    </PageCard>

    <!-- 新增/编辑配置对话框 -->
    <n-modal v-model:show="dialogVisible" preset="card" :title="dialogTitle" class="w-[92vw] max-w-2xl">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="100">
        <n-form-item label="115账号" path="cloud115_id">
          <n-select
            v-model:value="form.cloud115_id"
            placeholder="请选择115账号"
            :options="cloud115Options"
            @update:value="form.net_disk_path = ''"
          />
        </n-form-item>

        <n-form-item label="网盘目录" path="net_disk_path">
          <TargetFolderPicker
            :cloud-115-id="form.cloud115_id || 0"
            :default-path="form.net_disk_path"
            placeholder="请选择网盘目录"
            @update:path="form.net_disk_path = $event"
          />
        </n-form-item>

        <n-form-item label="本地目录" path="local_path">
          <n-input v-model:value="form.local_path" placeholder="请输入本地目录路径" />
        </n-form-item>

        <n-form-item label="Cron表达式" path="cron">
          <div class="flex w-full items-center gap-2">
            <n-input v-model:value="form.cron" placeholder="请输入 Cron 表达式，例如：0 0 * * *" />
            <n-button text type="primary" @click="showCronPicker = true">快捷生成</n-button>
          </div>
        </n-form-item>

        <n-form-item label="后缀名" path="extension">
          <n-input v-model:value="form.extension" placeholder="请输入文件后缀名，多个用逗号分隔，如：.mp4,.mkv,.avi" />
        </n-form-item>
      </n-form>

      <div class="mt-2 flex justify-end gap-2">
        <n-button @click="dialogVisible = false">取消</n-button>
        <n-button type="primary" @click="handleSubmit">确定</n-button>
      </div>
    </n-modal>

    <!-- Cron 表达式快捷生成器 -->
    <n-modal v-model:show="showCronPicker" preset="card" title="Cron表达式快捷生成" class="w-[92vw] max-w-sm">
      <n-form :model="cronForm" label-placement="left" label-width="80">
        <n-form-item label="执行周期">
          <n-radio-group v-model:value="cronForm.period">
            <n-radio value="daily">每天</n-radio>
            <n-radio value="weekly">每周</n-radio>
            <n-radio value="monthly">每月</n-radio>
          </n-radio-group>
        </n-form-item>

        <n-form-item label="执行时间">
          <n-time-picker
            v-model:formatted-value="cronForm.time"
            format="HH:mm"
            value-format="HH:mm"
            placeholder="请选择执行时间"
            class="w-full"
          />
        </n-form-item>
      </n-form>
      <div class="mt-2 flex justify-end gap-2">
        <n-button @click="showCronPicker = false">取消</n-button>
        <n-button type="primary" @click="generateCronExpression">生成</n-button>
      </div>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, onMounted, onBeforeUnmount, h } from 'vue'
import {
  NAlert,
  NButton,
  NDataTable,
  NDropdown,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NModal,
  NProgress,
  NRadio,
  NRadioGroup,
  NSelect,
  NTag,
  NTimePicker,
  useMessage
} from 'naive-ui'
import {
  AddOutline,
  AlarmOutline,
  CheckmarkCircleOutline,
  ChevronDownOutline,
  CloseCircleOutline,
  CloseOutline,
  CreateOutline,
  LayersOutline,
  PulseOutline,
  RefreshOutline,
  SyncOutline,
  TimeOutline,
  TrashOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import TargetFolderPicker from '../components/resource/TargetFolderPicker.vue'
import { getCloud115List } from '../utils/api/cloud115'
import { getCronTasks, runCronTask, updateCronTask } from '../utils/api/cron'
import { createStrmConfig, deleteStrmConfig, generateFullStrmConfig, getStrmConfigList, getStrmTaskStatus, updateStrmConfig } from '../utils/api/strm'
import { showAlertDialog, showConfirmDialog } from '../utils/ui/messageBox'

const DEFAULT_EXTENSION = '.mp4,.avi,.mkv,.mov,.wmv,.flv,.webm,.m4v,.mpeg,.mpg,.3gp,.rmvb,.rm,.vob,.ts,.m2ts,.divx,.asf'

const message = useMessage()

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

// Naive UI progress status
const taskProgressStatus = computed(() => {
  if (!taskInfo.value) return 'default'
  if (taskInfo.value.status === 'completed') return 'success'
  if (taskInfo.value.status === 'failed') return 'error'
  return 'default'
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
  if (!taskInfo.value) return SyncOutline
  if (taskInfo.value.status === 'completed') return CheckmarkCircleOutline
  if (taskInfo.value.status === 'failed') return CloseCircleOutline
  return SyncOutline
})

// 任务状态图标颜色
const taskStatusIconClass = computed(() => {
  if (!taskInfo.value) return 'text-cyan-500'
  if (taskInfo.value.status === 'completed') return 'text-emerald-500'
  if (taskInfo.value.status === 'failed') return 'text-red-500'
  return 'text-cyan-500'
})

// 任务状态标签
const taskStatusLabel = computed(() => {
  if (!taskInfo.value) return ''
  if (taskInfo.value.status === 'completed') return '生成完成'
  if (taskInfo.value.status === 'failed') return '生成失败'
  return '生成中...'
})

// 进度文本
const progressText = computed(() => {
  if (!taskInfo.value) return `${taskProgress.value}%`
  if (taskInfo.value.status === 'completed') return '完成'
  if (taskInfo.value.status === 'running' && taskInfo.value.total_files === 0) return '准备中...'
  return `${taskProgress.value}%`
})

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
      hint: `${accounts.size} 个 115 账号参与 STRM 生成`,
      icon: LayersOutline,
      tone: 'cyan'
    },
    {
      label: 'Cron 任务',
      value: cronTaskList.value.length,
      hint: `${enabledCronCount.value} 个处于启用状态`,
      icon: AlarmOutline,
      tone: 'violet'
    },
    {
      label: '当前生成状态',
      value: runningTaskLabel,
      hint: taskInfo.value ? `成功 ${taskInfo.value.success_files || 0}，失败 ${taskInfo.value.failed_files || 0}` : '暂无正在跟踪的生成任务',
      icon: PulseOutline,
      tone: taskInfo.value?.status === 'failed' ? 'red' : taskInfo.value?.status === 'running' ? 'amber' : 'green'
    },
    {
      label: '默认后缀示例',
      value: strmConfigList.value[0]?.extension || DEFAULT_EXTENSION,
      hint: '沿用原有扩展名配置提交到后端',
      icon: TimeOutline,
      tone: 'slate'
    }
  ]
})
const statusRailItems = computed(() => [
  { label: 'Cron 已启用', value: `${enabledCronCount.value} / ${cronTaskList.value.length || 0}` },
  { label: '下一次计划执行', value: nextRunSnapshot.value },
  { label: '最近聚焦账号', value: primaryCloudAccountText.value }
])

// 115 账号下拉选项
const cloud115Options = computed(() => cloud115List.value.map(item => ({ label: item.name, value: item.id })))

// 定时任务下拉菜单选项
const cronDropdownOptions = (task) => [
  { key: 'toggle', label: task.status === 'enabled' ? '禁用定时任务' : '启用定时任务', disabled: cronTaskLoading.value },
  { key: 'run', label: '立即执行', disabled: cronTaskLoading.value },
  { type: 'divider', key: 'd1' },
  { key: 'status', label: '查看详情' }
]

// 表格列定义
const columns = computed(() => [
  { title: 'ID', key: 'id', width: 64, align: 'center', sorter: true, defaultSortOrder: 'ascend' },
  {
    title: '115账号',
    key: 'cloud115_id',
    minWidth: 120,
    align: 'center',
    sorter: true,
    render: (row) => h(NTag, { type: 'info', size: 'small' }, { default: () => getCloud115Name(row.cloud115_id) })
  },
  { title: '网盘目录', key: 'net_disk_path', minWidth: 180, ellipsis: { tooltip: true } },
  { title: '本地目录', key: 'local_path', minWidth: 180, ellipsis: { tooltip: true } },
  {
    title: 'Cron 配置',
    key: 'cron',
    width: 120,
    align: 'center',
    render: (row) => row.cron
      ? h(NTag, { type: 'warning', size: 'small' }, { default: () => row.cron })
      : h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '-')
  },
  {
    title: '定时任务',
    key: 'cron_status',
    width: 100,
    align: 'center',
    render: (row) => {
      const task = getCronTask(row.id)
      if (!task) return h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '-')
      return h(
        NTag,
        { type: task.status === 'enabled' ? 'success' : 'info', size: 'small' },
        { default: () => (task.status === 'enabled' ? '已启用' : '已禁用') }
      )
    }
  },
  {
    title: '下次执行',
    key: 'next_run_time',
    width: 160,
    align: 'center',
    render: (row) => {
      const task = getCronTask(row.id)
      if (task && task.next_run_time) return formatTime(task.next_run_time)
      return h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '-')
    }
  },
  {
    title: '后缀名',
    key: 'extension',
    minWidth: 200,
    ellipsis: { tooltip: true },
    render: (row) => h('span', { class: 'font-mono text-xs text-slate-500 dark:text-slate-400' }, row.extension || '-')
  },
  { title: '创建时间', key: 'create_time', width: 160, align: 'center', sorter: true },
  {
    title: '操作',
    key: 'actions',
    width: 320,
    align: 'center',
    fixed: 'right',
    render: (row) => {
      const task = getCronTask(row.id)
      const buttons = [
        h(
          NButton,
          { type: 'primary', size: 'tiny', onClick: () => handleEdit(row) },
          { icon: () => h(NIcon, { component: CreateOutline }), default: () => '编辑' }
        ),
        h(
          NButton,
          { type: 'error', size: 'tiny', onClick: () => handleDelete(row.id) },
          { icon: () => h(NIcon, { component: TrashOutline }), default: () => '删除' }
        ),
        h(
          NButton,
          {
            type: 'warning',
            size: 'tiny',
            loading: isGenerating(row.id),
            disabled: isGenerating(row.id),
            onClick: () => handleFullGenerate(row.id)
          },
          {
            icon: isGenerating(row.id) ? undefined : () => h(NIcon, { component: RefreshOutline }),
            default: () => (isGenerating(row.id) ? '生成中...' : '全量生成')
          }
        )
      ]
      if (task) {
        buttons.push(
          h(
            NDropdown,
            {
              trigger: 'click',
              options: cronDropdownOptions(task),
              onSelect: (key) => handleCronCommand(key, row)
            },
            {
              default: () => h(
                NButton,
                { type: 'info', size: 'tiny' },
                {
                  icon: () => h(NIcon, { component: AlarmOutline }),
                  default: () => [
                    '定时任务',
                    h(NIcon, { component: ChevronDownOutline, class: 'ml-1' })
                  ]
                }
              )
            }
          )
        )
      }
      return h('div', { class: 'flex flex-nowrap items-center justify-center gap-1 whitespace-nowrap' }, buttons)
    }
  }
])

// 开始轮询任务状态
const startPolling = (taskId) => {
  stopPolling()
  pollTimer = setInterval(async () => {
    try {
      const resp = await getStrmTaskStatus(taskId)
      taskInfo.value = resp.data.data
      if (taskInfo.value.status === 'completed') {
        message.success('STRM 文件全量生成完成')
        generatingConfigId.value = null
        stopPolling()
      } else if (taskInfo.value.status === 'failed') {
        message.error(`STRM 文件生成失败：${taskInfo.value.error_message || '未知错误'}`)
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
  cloud115_id: [{ required: true, type: 'number', message: '请选择115账号', trigger: 'change', validator: (rule, value) => !!value }],
  net_disk_path: [{ required: true, message: '请选择网盘目录', trigger: ['blur', 'change'] }],
  local_path: [{ required: true, message: '请输入本地目录', trigger: ['blur', 'input'] }],
  cron: [{ required: true, message: '请输入 Cron 表达式', trigger: ['blur', 'input'] }],
  extension: [{ required: true, message: '请输入后缀名', trigger: ['blur', 'input'] }]
}

// 获取 115 账号列表
const fetchCloud115List = async () => {
  try {
    const response = await getCloud115List()
    cloud115List.value = response.data.data || []
  } catch (error) {
    message.error('获取115账号列表失败')
  }
}

// 获取 STRM 配置列表
const fetchStrmConfigList = async () => {
  try {
    const response = await getStrmConfigList({
      sort_field: sortField.value,
      sort_order: sortOrder.value
    })
    const apiData = response.data.data
    strmConfigList.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (error) {
    message.error('获取 STRM 配置列表失败')
  }
}

// 获取 Cron 任务列表
const fetchCronTaskList = async () => {
  try {
    const response = await getCronTasks()
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
 * 处理表格排序变化（服务端排序）
 * @param {Object|null} sorter - Naive UI sorter 状态
 */
const handleSorterChange = (sorter) => {
  if (sorter && sorter.order) {
    sortField.value = sorter.columnKey
    sortOrder.value = sorter.order === 'ascend' ? 'asc' : 'desc'
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
  formRef.value.validate((errors) => {
    if (!errors) {
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
      await updateStrmConfig(form.value.id, requestData)
      message.success('配置更新成功')
    } else {
      await createStrmConfig(requestData)
      message.success('配置创建成功')
    }

    dialogVisible.value = false
    await fetchStrmConfigList()
    await fetchCronTaskList()
  } catch (error) {
    message.error(form.value.id ? '配置更新失败' : '配置创建失败')
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
      await deleteStrmConfig(id)
      message.success('配置删除成功')
      await fetchStrmConfigList()
      await fetchCronTaskList()
    } catch (error) {
      message.error('配置删除失败')
    }
  }).catch(() => {})
}

// 全量生成 STRM 文件
const handleFullGenerate = (id) => {
  showConfirmDialog('确定要全量生成 STRM 文件吗？这将清空目标目录内的所有内容并重新生成 STRM 文件。', '提示', {
    confirmButtonText: '确定',
    cancelButtonText: '取消'
  }).then(() => {
    // 先清除之前的任务
    clearTask()
    generatingConfigId.value = id
    // 初始化任务状态为 running
    taskInfo.value = { status: 'running', total_files: 0, processed_files: 0, success_files: 0, failed_files: 0 }

    generateFullStrmConfig(id).then((resp) => {
      const taskId = resp.data.task_id
      if (taskId) {
        currentTaskId.value = taskId
        startPolling(taskId)
      } else {
        message.success('全量生成 STRM 文件成功')
        clearTask()
      }
    }).catch(() => {
      message.error('全量生成 STRM 文件失败')
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
    await updateCronTask(task.id, {
      cron_expr: task.cron_expr,
      status: newStatus
    })
    message.success(`定时任务已${actionText}`)
    await fetchCronTaskList()
  } catch (error) {
    message.error(`${actionText}定时任务失败`)
  } finally {
    cronTaskLoading.value = false
  }
}

// 立即执行定时任务
const handleRunCronTask = async (task) => {
  try {
    cronTaskLoading.value = true
    await runCronTask(task.id)
    message.success('定时任务已触发执行，请查看任务进度')
    // 刷新任务列表
    await fetchCronTaskList()
  } catch (error) {
    message.error('执行定时任务失败')
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

  const detailRow = (label, value) => h('p', { class: 'leading-loose' }, [
    h('strong', null, `${label}：`),
    String(value)
  ])

  // Naive dialog 的 content 支持渲染函数，替代旧版 dangerouslyUseHTMLString
  showAlertDialog(
    () => h('div', null, [
      detailRow('任务名称', task.task_name),
      detailRow('任务状态', statusText),
      detailRow('Cron表达式', task.cron_expr),
      detailRow('上次执行时间', lastRunTime),
      detailRow('上次执行状态', lastRunStatus),
      detailRow('上次执行结果', lastRunMessage),
      detailRow('下次执行时间', nextRunTime)
    ]),
    '定时任务详情',
    {
      confirmButtonText: '关闭'
    }
  )
}

// 生成 Cron 表达式
const generateCronExpression = () => {
  const time = cronForm.value.time
  if (!time || typeof time !== 'string') {
    message.error('请选择执行时间')
    return
  }

  try {
    const parts = time.split(':')
    if (parts.length !== 2) {
      message.error('时间格式错误')
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
    message.error('生成 Cron 表达式失败')
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
