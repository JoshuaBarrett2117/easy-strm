<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-center justify-between gap-4">
        <div>
          <div class="text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">Cloud 115</div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">115云账号管理</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            扫码登录、转存链路、状态管理和连通性测试都继续沿用原有后端接口。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <n-button type="success" @click="handleQRCodeLogin">
            <template #icon>
              <n-icon :component="QrCodeOutline" />
            </template>
            扫码登录
          </n-button>
          <n-button type="primary" @click="handleAdd">
            <template #icon>
              <n-icon :component="AddOutline" />
            </template>
            新增账号
          </n-button>
        </div>
      </div>
    </div>

    <!-- 账号池总览 -->
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        v-for="card in cloudOverviewCards"
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

    <!-- 账号列表 -->
    <PageCard title="账号列表" subtitle="支持排序、Cookie 查看、连接测试与扫码更新">
      <div class="overflow-x-auto">
        <n-data-table
          :columns="columns"
          :data="cloud115List"
          :row-key="(row) => row.id"
          :scroll-x="1720"
          size="small"
          @update:sorter="handleSorterChange"
        >
          <template #empty>
            <EmptyState title="暂无 115 云账号" description="点击右上角「扫码登录」或「新增账号」开始接入">
              <n-button type="primary" size="small" @click="handleQRCodeLogin">扫码登录</n-button>
            </EmptyState>
          </template>
        </n-data-table>
      </div>
      <div class="mt-4 flex justify-end">
        <n-pagination
          v-model:page="currentPage"
          v-model:page-size="pageSize"
          :item-count="total"
          :page-sizes="[5, 10, 20, 50]"
          show-size-picker
          @update:page="handleCurrentChange"
          @update:page-size="handleSizeChange"
        />
      </div>
    </PageCard>

    <!-- 新增/编辑对话框 -->
    <n-modal v-model:show="dialogVisible" preset="card" :title="dialogTitle" class="w-[92vw] max-w-2xl">
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="left" label-width="100">
        <n-form-item label="名称" path="name">
          <n-input v-model:value="form.name" placeholder="请输入 115 云账号名称" />
        </n-form-item>
        <n-form-item label="Cookie" path="cookie">
          <n-input v-model:value="form.cookie" type="textarea" placeholder="请输入 115 云账号 Cookie" :rows="3" />
        </n-form-item>
        <n-form-item label="账号类型" path="account_type">
          <div>
            <n-radio-group v-model:value="form.account_type">
              <n-radio value="resource">资源号</n-radio>
              <n-radio value="vip">VIP观影号</n-radio>
              <n-radio value="both">兼顾</n-radio>
            </n-radio-group>
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">设置账号在同步任务中的角色类型。</p>
          </div>
        </n-form-item>
        <n-form-item label="状态" path="status">
          <n-radio-group v-model:value="form.status">
            <n-radio value="active">正常</n-radio>
            <n-radio value="cooling">冷却中</n-radio>
            <n-radio value="disabled">禁用</n-radio>
          </n-radio-group>
        </n-form-item>
        <n-form-item label="优先级" path="priority">
          <div>
            <n-input-number v-model:value="form.priority" :min="1" :max="10" :step="1" />
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">1-10，数字越大优先级越高，用于同步调度排序。</p>
          </div>
        </n-form-item>
        <n-divider title-placement="left">文件转存配置</n-divider>
        <n-form-item label="转存账号" path="transfer_account_id">
          <div class="w-full">
            <n-select
              v-model:value="form.transfer_account_id"
              placeholder="请选择转存目标账号"
              clearable
              :options="transferAccountOptions"
            />
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">选择后会将文件转存到该账号下以获取直链。</p>
          </div>
        </n-form-item>
        <n-form-item label="转存目录" path="transfer_directory">
          <div class="w-full">
            <TargetFolderPicker
              :cloud-115-id="form.transfer_account_id || 0"
              :default-path="form.transfer_directory"
              placeholder="请选择转存目录，留空则使用根目录"
              :disabled="transferDisabled"
              @update:path="form.transfer_directory = $event"
            />
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">文件转存的目标目录路径，例如：/视频/转存文件。</p>
          </div>
        </n-form-item>
        <n-form-item label="秒传方式" path="transfer_method">
          <div class="w-full">
            <n-select
              v-model:value="form.transfer_method"
              placeholder="请选择秒传方式"
              :disabled="transferDisabled"
              :options="[{ label: 'alist', value: 'alist' }]"
            />
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">选择失败时自动回退到直链获取。</p>
          </div>
        </n-form-item>
      </n-form>
      <div class="mt-2 flex justify-end gap-2">
        <n-button @click="dialogVisible = false">取消</n-button>
        <n-button type="primary" :loading="loading" @click="handleSubmit">确定</n-button>
      </div>
    </n-modal>

    <!-- 扫码登录对话框 -->
    <n-modal
      v-model:show="qrcodeDialogVisible"
      preset="card"
      :title="qrcodeDialogTitle"
      class="w-[92vw] max-w-md"
      :mask-closable="false"
    >
      <div class="flex flex-col items-center">
        <!-- 登录方式选择 -->
        <n-tabs :value="loginMode" type="segment" size="small" class="mb-4" @update:value="selectLoginMode">
          <n-tab name="cookie" tab="Cookie 登录" />
          <n-tab name="open" tab="开放平台登录" />
        </n-tabs>

        <!-- 渠道选择（仅 Cookie 登录） -->
        <div v-if="loginMode === 'cookie'" class="mb-4 w-full">
          <p class="mb-2 text-center text-xs font-medium text-slate-400 dark:text-slate-500">扫码渠道</p>
          <div class="flex flex-wrap justify-center gap-2">
            <button
              v-for="channel in loginChannels"
              :key="channel.value"
              type="button"
              class="rounded-lg border px-3 py-1.5 text-xs transition-colors"
              :class="selectedChannel === channel.value
                ? 'border-cyan-500 bg-cyan-500/10 font-semibold text-cyan-600 dark:text-cyan-400'
                : 'border-slate-200 text-slate-500 hover:border-cyan-400 hover:text-cyan-600 dark:border-white/10 dark:text-slate-400 dark:hover:text-cyan-400'"
              @click="selectChannel(channel.value)"
            >
              {{ channel.label }}
            </button>
          </div>
        </div>

        <!-- 加载中 -->
        <div v-if="qrcodeLoading" class="flex flex-col items-center gap-3 py-10 text-cyan-600 dark:text-cyan-400">
          <n-spin size="large" />
          <span class="text-sm">正在获取二维码...</span>
        </div>

        <!-- 错误 -->
        <div v-else-if="qrcodeError" class="flex flex-col items-center gap-4 py-8">
          <n-icon size="40" class="text-red-500" :component="WarningOutline" />
          <span class="text-sm text-red-500">{{ qrcodeError }}</span>
          <n-button type="primary" @click="refreshQRCode">重新获取</n-button>
        </div>

        <!-- 二维码 -->
        <div v-else class="flex flex-col items-center gap-4">
          <div class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-white/10">
            <img :src="qrcodeDataUrl" alt="115登录二维码" class="h-[200px] w-[200px]" />
          </div>
          <n-tag :type="loginStatusType" size="large" round>{{ loginStatusText }}</n-tag>
          <div class="text-center text-sm text-slate-400 dark:text-slate-500">
            <p>{{ channelTip }}</p>
            <p v-if="qrcodeExpireTime > 0" class="mt-1 font-bold text-amber-500">
              二维码有效期：{{ formatExpireTime(qrcodeExpireTime) }}
            </p>
          </div>
        </div>
      </div>
      <div class="mt-6 flex justify-end gap-2">
        <n-button @click="qrcodeDialogVisible = false">关闭</n-button>
        <n-button type="primary" :disabled="qrcodeLoading" @click="refreshQRCode">刷新二维码</n-button>
      </div>
    </n-modal>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted, onBeforeUnmount, h } from 'vue'
import {
  NButton,
  NDataTable,
  NDivider,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NInputNumber,
  NModal,
  NPagination,
  NRadio,
  NRadioGroup,
  NSelect,
  NSpin,
  NTab,
  NTabs,
  NTag,
  useMessage
} from 'naive-ui'
import {
  AddOutline,
  AlbumsOutline,
  CheckmarkCircleOutline,
  CreateOutline,
  EyeOffOutline,
  EyeOutline,
  KeyOutline,
  QrCodeOutline,
  RefreshOutline,
  SwapHorizontalOutline,
  TrashOutline,
  TrendingUpOutline,
  WarningOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import StatCard from '../components/common/StatCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import { showConfirmDialog } from '../utils/ui/messageBox'
import {
  channelTips,
  formatExpireTime,
  getAccountTypeName,
  getAccountTypeTag,
  getPriorityTag,
  getStatusName,
  getStatusTag,
  getTransferMethodName,
  getTransferMethodTag,
  isDialogCancelAction,
  loginStatusMap,
  maskSensitive,
  sortChannelsWithWechatFirst
} from '../utils/cloud115Display'
import {
  check115LoginStatus,
  check115OpenLoginStatus,
  confirm115Login,
  confirm115OpenLogin,
  createCloud115,
  deleteCloud115,
  get115LoginChannels,
  get115OpenQRCode,
  get115QRCode,
  getCloud115List,
  testCloud115Connection,
  updateCloud115
} from '../utils/api/cloud115'
import TargetFolderPicker from '../components/resource/TargetFolderPicker.vue'

const message = useMessage()

// 旧工具函数返回 Element Plus 的 tag 类型，danger 需映射为 Naive 的 error
const NAIVE_TAG_TYPE = { primary: 'primary', success: 'success', warning: 'warning', danger: 'error', info: 'info' }
const toTagType = (tag) => NAIVE_TAG_TYPE[tag] || 'default'

/**
 * 切换 Cookie 显示状态
 * @param {Object} row - 行数据
 */
const toggleShowCookie = (row) => {
  row._showCookie = !row._showCookie
}

// 表格数据
const cloud115List = ref([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(10)

// 排序状态
const sortField = ref('id')
const sortOrder = ref('asc')

// 对话框状态
const dialogVisible = ref(false)
const dialogTitle = ref('新增115云账号')
const formRef = ref(null)
const loading = ref(false)
const testingAccountId = ref(null)

// 转存相关字段禁用状态
const transferDisabled = ref(false)

// 表单数据
const form = ref({
  id: null,
  name: '',
  cookie: '',
  access_token: '',
  refresh_token: '',
  transfer_account_id: null,
  transfer_directory: '',
  account_type: 'resource',
  status: 'active',
  priority: 5,
  transfer_method: ''
})

// 表单验证规则
const rules = {
  name: [{ required: true, message: '请输入账号名称', trigger: ['blur', 'input'] }]
}

/**
 * 获取转存账号名称
 * @param {number} accountId - 账号 ID
 * @returns {string} 账号名称
 */
const getTransferAccountName = (accountId) => {
  if (!accountId) return '-'
  const account = cloud115List.value.find(item => item.id === accountId)
  return account ? account.name : `账号ID: ${accountId}`
}

/**
 * 可用的转存账号选项（排除当前编辑的账号）
 */
const transferAccountOptions = computed(() => {
  return cloud115List.value
    .filter(item => item.id !== form.value.id)
    .map(item => ({ label: item.name, value: item.id }))
})

// 扫码登录相关状态
const qrcodeDialogVisible = ref(false)
const qrcodeDialogTitle = ref('扫码登录115账号')
const qrcodeLoading = ref(false)
const qrcodeError = ref('')
const qrcodeDataUrl = ref('')
const qrcodeSession = ref({
  uid: '',
  time: 0,
  sign: ''
})
const loginStatus = ref(0)
const qrcodeExpireTime = ref(0)
const pollTimer = ref(null)
const expireTimer = ref(null)
const updateCloudId = ref(null)

// 登录方式：cookie（扫码取 Cookie）/ open（115 开放平台，取 access_token）
const loginMode = ref('cookie')
// 开放平台登录会话标识
const openLoginState = ref('')

// 渠道选择相关
const loginChannels = ref([])
const selectedChannel = ref('wechatmini')

// 计算属性
const loginStatusText = computed(() => loginStatusMap[loginStatus.value]?.text || '未知状态')
const loginStatusType = computed(() => toTagType(loginStatusMap[loginStatus.value]?.type || 'info'))
const channelTip = computed(() => {
  if (loginMode.value === 'open') return '请使用 115 APP 扫描二维码授权开放平台登录'
  return channelTips[selectedChannel.value] || '请扫描二维码登录'
})
const activeAccounts = computed(() => cloud115List.value.filter(item => item.status === 'active'))
const coolingAccounts = computed(() => cloud115List.value.filter(item => item.status === 'cooling'))
const disabledAccounts = computed(() => cloud115List.value.filter(item => item.status === 'disabled'))
const transferEnabledAccounts = computed(() => cloud115List.value.filter(item => item.transfer_account_id))
const resourceAccounts = computed(() => cloud115List.value.filter(item => item.account_type === 'resource'))
const vipAccounts = computed(() => cloud115List.value.filter(item => item.account_type === 'vip'))
const hybridAccounts = computed(() => cloud115List.value.filter(item => item.account_type === 'both'))

const cloudOverviewCards = computed(() => {
  const highestPriority = cloud115List.value.reduce((max, item) => Math.max(max, Number(item.priority || 0)), 0)
  return [
    {
      label: '账号总数',
      value: cloud115List.value.length,
      hint: `活跃 ${activeAccounts.value.length} 个，冷却 ${coolingAccounts.value.length} 个`,
      icon: AlbumsOutline,
      tone: 'cyan'
    },
    {
      label: '可用账号',
      value: activeAccounts.value.length,
      hint: disabledAccounts.value.length ? `${disabledAccounts.value.length} 个账号处于禁用状态` : '当前没有被禁用的账号',
      icon: CheckmarkCircleOutline,
      tone: 'green'
    },
    {
      label: '已配置转存',
      value: transferEnabledAccounts.value.length,
      hint: transferEnabledAccounts.value.length ? '可直接参与直链转存链路' : '还没有账号配置转存目标',
      icon: SwapHorizontalOutline,
      tone: 'violet'
    },
    {
      label: '最高优先级',
      value: highestPriority || '-',
      hint: highestPriority ? '用于同步调度时的优先选择' : '暂无优先级配置',
      icon: TrendingUpOutline,
      tone: 'amber'
    }
  ]
})

const accountStructureText = computed(() => {
  return `资源号 ${resourceAccounts.value.length} / VIP ${vipAccounts.value.length} / 兼顾 ${hybridAccounts.value.length}`
})
const priorityLeadersText = computed(() => {
  const leaders = [...cloud115List.value]
    .sort((a, b) => Number(b.priority || 0) - Number(a.priority || 0))
    .slice(0, 3)
    .map(item => item.name)
    .filter(Boolean)
  return leaders.length ? leaders.join('、') : '暂无'
})
const transferCoverageText = computed(() => {
  if (!cloud115List.value.length) return '暂无账号'
  return `${transferEnabledAccounts.value.length}/${cloud115List.value.length} 已接入转存`
})
const statusRailItems = computed(() => [
  { label: '当前账号结构', value: accountStructureText.value },
  { label: '高优先级账号', value: priorityLeadersText.value },
  { label: '转存配置覆盖', value: transferCoverageText.value }
])

// 表格列定义
const columns = computed(() => [
  { title: 'ID', key: 'id', width: 64, align: 'center', sorter: true, defaultSortOrder: 'ascend' },
  { title: '名称', key: 'name', minWidth: 120, sorter: true },
  {
    title: '账号类型',
    key: 'account_type',
    width: 110,
    align: 'center',
    render: (row) => h(
      NTag,
      { type: toTagType(getAccountTypeTag(row.account_type)), size: 'small' },
      { default: () => getAccountTypeName(row.account_type) }
    )
  },
  {
    title: '状态',
    key: 'status',
    width: 90,
    align: 'center',
    render: (row) => h(
      NTag,
      { type: toTagType(getStatusTag(row.status)), size: 'small' },
      { default: () => getStatusName(row.status) }
    )
  },
  {
    title: '优先级',
    key: 'priority',
    width: 90,
    align: 'center',
    sorter: true,
    render: (row) => h(
      NTag,
      { type: toTagType(getPriorityTag(row.priority)), size: 'small' },
      { default: () => String(row.priority || 5) }
    )
  },
  {
    title: 'Cookie',
    key: 'cookie',
    minWidth: 200,
    render: (row) => h('div', { class: 'flex items-center gap-2' }, [
      h(
        'span',
        { class: 'break-all font-mono text-xs text-slate-500 dark:text-slate-400' },
        row._showCookie ? (row.cookie || '-') : maskSensitive(row.cookie)
      ),
      h(
        NButton,
        { text: true, type: 'primary', size: 'small', onClick: () => toggleShowCookie(row) },
        { icon: () => h(NIcon, { component: row._showCookie ? EyeOffOutline : EyeOutline }) }
      )
    ])
  },
  {
    title: '转存账号',
    key: 'transfer_account_id',
    minWidth: 110,
    align: 'center',
    render: (row) => row.transfer_account_id
      ? getTransferAccountName(row.transfer_account_id)
      : h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '未配置')
  },
  {
    title: '转存目录',
    key: 'transfer_directory',
    minWidth: 130,
    render: (row) => row.transfer_account_id
      ? (row.transfer_directory || '根目录')
      : h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '-')
  },
  {
    title: '秒传方式',
    key: 'transfer_method',
    width: 110,
    align: 'center',
    render: (row) => row.transfer_method
      ? h(
          NTag,
          { type: toTagType(getTransferMethodTag(row.transfer_method)), size: 'small' },
          { default: () => getTransferMethodName(row.transfer_method) }
        )
      : h('span', { class: 'text-xs text-slate-400 dark:text-slate-500' }, '-')
  },
  { title: '创建时间', key: 'create_time', width: 160, align: 'center', sorter: true },
  { title: '更新时间', key: 'update_time', width: 160, align: 'center', sorter: true },
  {
    title: '操作',
    key: 'actions',
    width: 320,
    align: 'center',
    fixed: 'right',
    render: (row) => h('div', { class: 'flex flex-wrap items-center justify-center gap-1.5' }, [
      h(
        NButton,
        { type: 'primary', size: 'small', onClick: () => handleEdit(row) },
        { icon: () => h(NIcon, { component: CreateOutline }), default: () => '编辑' }
      ),
      h(
        NButton,
        { type: 'success', size: 'small', onClick: () => handleQRCodeUpdate(row) },
        { icon: () => h(NIcon, { component: KeyOutline }), default: () => '扫码更新' }
      ),
      h(
        NButton,
        {
          type: 'warning',
          size: 'small',
          loading: testingAccountId.value === row.id,
          onClick: () => handleTest(row)
        },
        { icon: () => h(NIcon, { component: RefreshOutline }), default: () => '测试' }
      ),
      h(
        NButton,
        { type: 'error', size: 'small', onClick: () => handleDelete(row) },
        { icon: () => h(NIcon, { component: TrashOutline }), default: () => '删除' }
      )
    ])
  }
])

/**
 * 获取 115 云账号列表
 */
const fetchCloud115List = async () => {
  try {
    const response = await getCloud115List({
      sort_field: sortField.value,
      sort_order: sortOrder.value
    })
    const apiData = response.data.data
    cloud115List.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
    total.value = apiData?.total || cloud115List.value.length
  } catch (error) {
    message.error('获取 115 云账号列表失败')
  }
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
  fetchCloud115List()
}

/**
 * 获取支持的登录渠道列表
 */
const fetchLoginChannels = async () => {
  try {
    const response = await get115LoginChannels()
    const channels = response.data.data || []
    // 将微信渠道放在第一位
    loginChannels.value = sortChannelsWithWechatFirst(channels)
  } catch (error) {
    console.error('获取登录渠道列表失败:', error)
    // 使用默认渠道列表，微信放在第一位
    loginChannels.value = [
      { value: 'wechatmini', label: '微信小程序', description: '使用微信小程序扫码登录' },
      { value: 'web', label: '网页版', description: '使用115网页版扫码登录' },
      { value: 'android', label: '安卓APP', description: '使用115安卓APP扫码登录' },
      { value: 'ios', label: 'iOS APP', description: '使用115 iOS APP扫码登录' },
      { value: 'tv', label: '电视版', description: '使用115电视版扫码登录' },
      { value: 'alipaymini', label: '支付宝小程序', description: '使用支付宝小程序扫码登录' },
      { value: 'qandroid', label: '安卓Q版', description: '使用115安卓Q版扫码登录' }
    ]
  }
}

/**
 * 新增账号
 */
const handleAdd = () => {
  dialogTitle.value = '新增115云账号'
  resetForm()
  dialogVisible.value = true
}

/**
 * 新增或编辑提交
 */
const handleSubmit = () => {
  if (!formRef.value) return

  formRef.value.validate((errors) => {
    if (!errors) {
      loading.value = true

      // 准备提交数据，将 null 转换为 0
      const submitData = {
        ...form.value,
        transfer_account_id: form.value.transfer_account_id || 0
      }

      const submitRequest = form.value.id
        ? updateCloud115(form.value.id, submitData)
        : createCloud115(submitData)

      submitRequest.then(() => {
        message.success(form.value.id ? '编辑成功' : '新增成功')
        dialogVisible.value = false
        fetchCloud115List()
        resetForm()
      }).catch(() => {
        message.error(form.value.id ? '编辑失败' : '新增失败')
      }).finally(() => {
        loading.value = false
      })
    }
  })
}

/**
 * 编辑账号
 * @param {Object} row - 账号数据
 */
const handleEdit = (row) => {
  dialogTitle.value = '编辑115云账号'
  form.value = {
    id: row.id,
    name: row.name || '',
    cookie: row.cookie || '',
    access_token: row.access_token || '',
    refresh_token: row.refresh_token || '',
    transfer_account_id: row.transfer_account_id || null,
    transfer_directory: row.transfer_directory || '',
    account_type: row.account_type || 'resource',
    status: row.status || 'active',
    priority: row.priority || 5,
    transfer_method: row.transfer_method || ''
  }
  // 根据转存账号是否配置来设置禁用状态
  transferDisabled.value = !row.transfer_account_id
  dialogVisible.value = true
}

/**
 * 删除账号
 * @param {Object} row - 账号数据
 */
const handleDelete = (row) => {
  showConfirmDialog(
    '确定要删除这个 115 云账号吗？',
    '删除确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(() => {
    return deleteCloud115(row.id)
  }).then(() => {
    message.success('删除成功')
    fetchCloud115List()
  }).catch((error) => {
    if (isDialogCancelAction(error)) {
      return
    }
    message.error('删除失败')
  })
}

/**
 * 测试账号连接
 * @param {Object} row - 账号数据
 */
const handleTest = async (row) => {
  if (testingAccountId.value === row.id) return

  testingAccountId.value = row.id
  const loadingMessage = message.info(`正在测试账号「${row.name}」连接状态...`, {
    duration: 0,
    closable: true
  })
  try {
    const response = await testCloud115Connection(row.id)
    const payload = response.data || {}
    const data = payload.data || {}
    if (payload.state === true || data?.id || data?.name) {
      const fileCount = Number(data.file_count || 0)
      const accountName = data.name || row.name || `账号 ${row.id}`
      message.success(`${accountName} 测试成功，连接正常，可访问 ${fileCount} 项内容。`, {
        duration: 4000,
        closable: true
      })
      return
    }
    message.error(`测试失败：${payload.message || payload.error || '未知错误'}`)
  } catch (error) {
    console.error('115云账号测试失败:', error)
    const errorMsg = error.response?.data?.message || error.response?.data?.error || error.message || '未知错误'
    message.error(`测试失败：${errorMsg}`)
  } finally {
    loadingMessage.destroy()
    testingAccountId.value = null
  }
}

/**
 * 重置表单
 */
const resetForm = () => {
  form.value = {
    id: null,
    name: '',
    cookie: '',
    access_token: '',
    refresh_token: '',
    transfer_account_id: null,
    transfer_directory: '',
    account_type: 'resource',
    status: 'active',
    priority: 5,
    transfer_method: ''
  }
  transferDisabled.value = false
  if (formRef.value) {
    formRef.value.restoreValidation()
  }
}

/**
 * 分页大小变化
 * @param {number} size - 每页数量
 */
const handleSizeChange = (size) => {
  pageSize.value = size
  fetchCloud115List()
}

/**
 * 当前页变化
 * @param {number} page - 当前页码
 */
const handleCurrentChange = (page) => {
  currentPage.value = page
  fetchCloud115List()
}

/**
 * 处理扫码登录（新增账号）
 */
const handleQRCodeLogin = () => {
  updateCloudId.value = null
  qrcodeDialogTitle.value = '扫码登录115账号'
  qrcodeDialogVisible.value = true
  fetchCurrentQRCode()
}

/**
 * 处理扫码更新（更新现有账号）
 * @param {Object} row - 账号数据
 */
const handleQRCodeUpdate = (row) => {
  updateCloudId.value = row.id
  qrcodeDialogTitle.value = `扫码更新账号：${row.name}`
  qrcodeDialogVisible.value = true
  fetchCurrentQRCode()
}

/**
 * 渠道选择处理
 * @param {string} channel - 选中的渠道值
 */
const selectChannel = (channel) => {
  if (selectedChannel.value !== channel) {
    selectedChannel.value = channel
    refreshQRCode()
  }
}

/**
 * 登录方式切换处理
 * @param {string} mode - cookie | open
 */
const selectLoginMode = (mode) => {
  if (loginMode.value !== mode) {
    loginMode.value = mode
    refreshQRCode()
  }
}

/**
 * 按当前登录方式获取二维码
 */
const fetchCurrentQRCode = () => {
  if (loginMode.value === 'open') {
    fetchOpenQRCode()
  } else {
    fetchQRCode()
  }
}

/**
 * 获取登录二维码（Cookie 登录）
 */
const fetchQRCode = async () => {
  qrcodeLoading.value = true
  qrcodeError.value = ''
  qrcodeDataUrl.value = ''
  loginStatus.value = 0

  try {
    const response = await get115QRCode()
    const data = response.data.data

    qrcodeSession.value = {
      uid: data.uid,
      time: data.time,
      sign: data.sign
    }

    // 生成二维码图片 URL
    qrcodeDataUrl.value = generateQRCodeDataUrl(data.qrcode)

    // 设置二维码过期时间，默认 120 秒
    qrcodeExpireTime.value = 120
    startExpireTimer()
    startPolling()
  } catch (error) {
    console.error('获取二维码失败:', error)
    qrcodeError.value = '获取二维码失败，请重试'
  } finally {
    qrcodeLoading.value = false
  }
}

/**
 * 获取开放平台登录二维码
 */
const fetchOpenQRCode = async () => {
  qrcodeLoading.value = true
  qrcodeError.value = ''
  qrcodeDataUrl.value = ''
  loginStatus.value = 0

  try {
    const response = await get115OpenQRCode()
    const data = response.data.data

    openLoginState.value = data.state
    qrcodeDataUrl.value = generateQRCodeDataUrl(data.qrcode_url)

    // 设置二维码过期时间，默认 120 秒
    qrcodeExpireTime.value = 120
    startExpireTimer()
    startOpenPolling()
  } catch (error) {
    console.error('获取开放平台二维码失败:', error)
    const errorMsg = error.response?.data?.error || error.response?.data?.message || ''
    qrcodeError.value = errorMsg || '获取开放平台二维码失败，请重试'
  } finally {
    qrcodeLoading.value = false
  }
}

/**
 * 生成二维码 Data URL
 * @param {string} content - 二维码内容
 * @returns {string} 二维码图片的 Data URL
 */
const generateQRCodeDataUrl = (content) => {
  // 使用第三方 API 生成二维码图片
  return `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(content)}`
}

/**
 * 刷新二维码
 */
const refreshQRCode = () => {
  stopPolling()
  stopExpireTimer()
  fetchCurrentQRCode()
}

/**
 * 开始轮询检查登录状态（Cookie 登录）
 */
const startPolling = () => {
  stopPolling()

  pollTimer.value = setInterval(async () => {
    if (loginStatus.value === 2 || loginStatus.value === 3 || loginStatus.value === 4) {
      stopPolling()
      return
    }

    try {
      const response = await check115LoginStatus({
        uid: qrcodeSession.value.uid,
        time: qrcodeSession.value.time,
        sign: qrcodeSession.value.sign
      })

      const status = response.data.data.status
      loginStatus.value = status

      // 状态 2 表示登录成功
      if (status === 2) {
        stopPolling()
        stopExpireTimer()
        await handleLoginConfirm()
      }
      // 状态 3 表示二维码过期
      else if (status === 3) {
        stopPolling()
        stopExpireTimer()
        qrcodeError.value = '二维码已过期，请刷新'
      }
      // 状态 4 表示登录失败
      else if (status === 4) {
        stopPolling()
        stopExpireTimer()
        qrcodeError.value = '登录失败，请重试'
      }
    } catch (error) {
      console.error('检查登录状态失败:', error)
    }
  }, 2000)
}

/**
 * 开始轮询检查登录状态（开放平台登录）
 */
const startOpenPolling = () => {
  stopPolling()

  pollTimer.value = setInterval(async () => {
    if (loginStatus.value === 2 || loginStatus.value === 3 || loginStatus.value === 4) {
      stopPolling()
      return
    }

    try {
      const response = await check115OpenLoginStatus({ state: openLoginState.value })

      const status = response.data.data.status
      loginStatus.value = status

      // 状态 2 表示登录成功
      if (status === 2) {
        stopPolling()
        stopExpireTimer()
        await handleOpenLoginConfirm()
      }
      // 状态 3 表示二维码过期
      else if (status === 3) {
        stopPolling()
        stopExpireTimer()
        qrcodeError.value = '二维码已过期，请刷新'
      }
      // 状态 4 表示登录失败
      else if (status === 4) {
        stopPolling()
        stopExpireTimer()
        qrcodeError.value = '登录失败，请重试'
      }
    } catch (error) {
      console.error('检查开放平台登录状态失败:', error)
    }
  }, 2000)
}

/**
 * 停止轮询
 */
const stopPolling = () => {
  if (pollTimer.value) {
    clearInterval(pollTimer.value)
    pollTimer.value = null
  }
}

/**
 * 开始过期倒计时
 */
const startExpireTimer = () => {
  stopExpireTimer()

  expireTimer.value = setInterval(() => {
    if (qrcodeExpireTime.value > 0) {
      qrcodeExpireTime.value--
      if (qrcodeExpireTime.value <= 0) {
        stopExpireTimer()
        loginStatus.value = 3
        qrcodeError.value = '二维码已过期，请刷新'
      }
    }
  }, 1000)
}

/**
 * 停止过期倒计时
 */
const stopExpireTimer = () => {
  if (expireTimer.value) {
    clearInterval(expireTimer.value)
    expireTimer.value = null
  }
}

/**
 * 确认登录并保存凭据（Cookie 登录）
 */
const handleLoginConfirm = async () => {
  try {
    const confirmData = {
      uid: qrcodeSession.value.uid,
      time: qrcodeSession.value.time,
      sign: qrcodeSession.value.sign,
      app: selectedChannel.value,
      name: updateCloudId.value ? undefined : `115账号_${Date.now()}`,
      cloud_id: updateCloudId.value
    }

    await confirm115Login(confirmData)

    message.success(updateCloudId.value ? '账号 Cookie 更新成功' : '扫码登录成功，账号已创建')
    qrcodeDialogVisible.value = false
    fetchCloud115List()
  } catch (error) {
    console.error('确认登录失败:', error)
    message.error('保存登录凭据失败')
    loginStatus.value = 4
    qrcodeError.value = '保存登录凭据失败，请重试'
  }
}

/**
 * 确认登录并保存 Token（开放平台登录）
 */
const handleOpenLoginConfirm = async () => {
  try {
    const confirmData = {
      state: openLoginState.value,
      name: updateCloudId.value ? undefined : `115账号_${Date.now()}`,
      cloud_id: updateCloudId.value
    }

    await confirm115OpenLogin(confirmData)

    message.success(updateCloudId.value ? '账号 Token 更新成功' : '开放平台登录成功，账号已创建')
    qrcodeDialogVisible.value = false
    fetchCloud115List()
  } catch (error) {
    console.error('确认开放平台登录失败:', error)
    message.error('保存登录凭据失败')
    loginStatus.value = 4
    qrcodeError.value = '保存登录凭据失败，请重试'
  }
}

/**
 * 扫码对话框关闭处理
 */
const handleQRCodeDialogClose = () => {
  stopPolling()
  stopExpireTimer()
  qrcodeLoading.value = false
  qrcodeError.value = ''
  qrcodeDataUrl.value = ''
  loginStatus.value = 0
  qrcodeExpireTime.value = 0
  updateCloudId.value = null
  openLoginState.value = ''
}

// 对话框关闭时清理轮询与倒计时
watch(qrcodeDialogVisible, (visible) => {
  if (!visible) {
    handleQRCodeDialogClose()
  }
})

// 监听转存账号变化，控制秒传方式和转存目录的禁用状态
watch(() => form.value.transfer_account_id, (newVal) => {
  if (!newVal || newVal === 0) {
    transferDisabled.value = true
    form.value.transfer_method = ''
    form.value.transfer_directory = ''
  } else {
    transferDisabled.value = false
  }
})

// 组件挂载
onMounted(() => {
  fetchCloud115List()
  fetchLoginChannels()
})

// 组件卸载时清理定时器
onBeforeUnmount(() => {
  stopPolling()
  stopExpireTimer()
})
</script>
