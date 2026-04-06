<template>
  <div class="cloud115-container">
    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><Cloudy /></el-icon>
            <span>115云账号管理</span>
          </div>
          <div class="header-buttons">
            <el-button type="success" @click="handleQRCodeLogin">
              <el-icon><Key /></el-icon>
              扫码登录
            </el-button>
            <el-button type="primary" @click="handleAdd">
              <el-icon><Plus /></el-icon>
              新增账号
            </el-button>
          </div>
        </div>
      </template>
      
      <el-table :data="cloud115List" border style="width: 100%" stripe class="custom-table" row-key="id" @sort-change="handleSortChange" :default-sort="{ prop: 'id', order: 'ascending' }">
        <el-table-column prop="id" label="ID" width="60" align="center" sortable="custom" />
        <el-table-column prop="name" label="名称" min-width="120" sortable="custom" />
        <el-table-column prop="account_type" label="账号类型" width="100" align="center">
          <template #default="scope">
            <el-tag :type="getAccountTypeTag(scope.row.account_type)" size="small">
              {{ getAccountTypeName(scope.row.account_type) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="90" align="center">
          <template #default="scope">
            <el-tag :type="getStatusTag(scope.row.status)" size="small">
              {{ getStatusName(scope.row.status) }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="priority" label="优先级" width="80" align="center" sortable="custom">
          <template #default="scope">
            <el-tag :type="getPriorityTag(scope.row.priority)" size="small">
              {{ scope.row.priority || 5 }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="Cookie" min-width="180">
          <template #default="scope">
            <div class="sensitive-cell">
              <span v-if="!scope.row._showCookie" class="masked-text">
                {{ maskSensitive(scope.row.cookie) }}
              </span>
              <span v-else class="full-text">{{ scope.row.cookie || '-' }}</span>
              <el-button 
                type="primary" 
                link 
                size="small" 
                @click="toggleShowCookie(scope.row)"
                class="toggle-btn"
              >
                <el-icon><View v-if="!scope.row._showCookie" /><Hide v-else /></el-icon>
              </el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="转存账号" min-width="100" align="center">
          <template #default="scope">
            <span v-if="scope.row.transfer_account_id">
              {{ getTransferAccountName(scope.row.transfer_account_id) }}
            </span>
            <span v-else class="text-muted">未配置</span>
          </template>
        </el-table-column>
        <el-table-column label="转存目录" min-width="120">
          <template #default="scope">
            <span v-if="scope.row.transfer_account_id">
              {{ scope.row.transfer_directory || '根目录' }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column label="秒传方式" width="110" align="center">
          <template #default="scope">
            <el-tag v-if="scope.row.transfer_method" :type="getTransferMethodTag(scope.row.transfer_method)" size="small">
              {{ getTransferMethodName(scope.row.transfer_method) }}
            </el-tag>
            <span v-else style="color: #909399">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="create_time" label="创建时间" width="160" align="center" sortable="custom" />
        <el-table-column prop="update_time" label="更新时间" width="160" align="center" sortable="custom" />
        <el-table-column label="操作" min-width="340" fixed="right" align="center">
          <template #default="scope">
            <div class="action-buttons">
              <el-button type="primary" size="small" @click="handleEdit(scope.row)">
                <el-icon><Edit /></el-icon>
                编辑
              </el-button>
              <el-button type="success" size="small" @click="handleQRCodeUpdate(scope.row)">
                <el-icon><Key /></el-icon>
                扫码更新
              </el-button>
              <el-button type="warning" size="small" @click="handleTest(scope.row)">
                <el-icon><RefreshRight /></el-icon>
                测试
              </el-button>
              <el-button type="danger" size="small" @click="handleDelete(scope.row)">
                <el-icon><Delete /></el-icon>
                删除
              </el-button>
            </div>
          </template>
        </el-table-column>
      </el-table>
      
      <div class="pagination-container">
        <el-pagination
          v-model:current-page="currentPage"
          v-model:page-size="pageSize"
          :page-sizes="[5, 10, 20, 50]"
          layout="total, sizes, prev, pager, next, jumper"
          :total="total"
          @size-change="handleSizeChange"
          @current-change="handleCurrentChange"
        />
      </div>
    </el-card>
    
    <!-- 新增/编辑对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="600px"
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入115云账号名称" />
        </el-form-item>
        <el-form-item label="Cookie" prop="cookie">
          <el-input v-model="form.cookie" type="textarea" placeholder="请输入115云账号Cookie" :rows="3" />
        </el-form-item>
        <el-form-item label="账号类型" prop="account_type">
          <el-radio-group v-model="form.account_type">
            <el-radio label="resource">资源号</el-radio>
            <el-radio label="vip">VIP观影号</el-radio>
            <el-radio label="both">兼顾</el-radio>
          </el-radio-group>
          <div class="form-tip">设置账号在同步任务中的角色类型</div>
        </el-form-item>
        <el-form-item label="状态" prop="status">
          <el-radio-group v-model="form.status">
            <el-radio label="active">正常</el-radio>
            <el-radio label="cooling">冷却中</el-radio>
            <el-radio label="disabled">禁用</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="优先级" prop="priority">
          <el-input-number v-model="form.priority" :min="1" :max="10" :step="1" />
          <div class="form-tip">1-10，数字越大优先级越高，用于同步调度排序</div>
        </el-form-item>
        <el-divider content-position="left">文件转存配置</el-divider>
        <el-form-item label="转存账号" prop="transfer_account_id">
          <el-select v-model="form.transfer_account_id" placeholder="请选择转存目标账号" clearable style="width: 100%">
            <el-option
              v-for="account in availableTransferAccounts"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
          <div class="form-tip">选择后将文件转存到该账号下获取直链</div>
        </el-form-item>
        <el-form-item label="转存目录" prop="transfer_directory" :disabled="transferDisabled">
          <el-input v-model="form.transfer_directory" placeholder="留空则转存到根目录" :disabled="transferDisabled" />
          <div class="form-tip">文件转存的目标目录路径，如：/视频/转存文件</div>
        </el-form-item>
        <el-form-item label="秒传方式" prop="transfer_method" :disabled="transferDisabled">
          <el-select v-model="form.transfer_method" placeholder="请选择秒传方式" style="width: 100%" :disabled="transferDisabled">
            <el-option label="alist" value="alist" />
          </el-select>
          <div class="form-tip">选择失败时自动回退到直链获取</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit" :loading="loading">确定</el-button>
        </span>
      </template>
    </el-dialog>

    <!-- 扫码登录对话框 -->
    <el-dialog
      v-model="qrcodeDialogVisible"
      :title="qrcodeDialogTitle"
      width="450px"
      :close-on-click-modal="false"
      @close="handleQRCodeDialogClose"
    >
      <div class="qrcode-container">
        <!-- 渠道选择 - 按钮样式 -->
        <div class="channel-selector">
          <div class="channel-label">扫码渠道</div>
          <div class="channel-buttons">
            <button
              v-for="channel in loginChannels"
              :key="channel.value"
              type="button"
              class="channel-btn"
              :class="{ active: selectedChannel === channel.value }"
              @click="selectChannel(channel.value)"
            >
              {{ channel.label }}
            </button>
          </div>
        </div>

        <div v-if="qrcodeLoading" class="qrcode-loading">
          <el-icon class="is-loading"><Loading /></el-icon>
          <span>正在获取二维码...</span>
        </div>
        <div v-else-if="qrcodeError" class="qrcode-error">
          <el-icon><WarningFilled /></el-icon>
          <span>{{ qrcodeError }}</span>
          <el-button type="primary" @click="refreshQRCode">重新获取</el-button>
        </div>
        <div v-else class="qrcode-content">
          <div class="qrcode-image">
            <img :src="qrcodeDataUrl" alt="115登录二维码" />
          </div>
          <div class="qrcode-status">
            <el-tag :type="loginStatusType" size="large">
              {{ loginStatusText }}
            </el-tag>
          </div>
          <div class="qrcode-tips">
            <p>{{ channelTip }}</p>
            <p class="expire-tip" v-if="qrcodeExpireTime > 0">
              二维码有效期: {{ formatExpireTime(qrcodeExpireTime) }}
            </p>
          </div>
        </div>
      </div>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="qrcodeDialogVisible = false">关闭</el-button>
          <el-button type="primary" @click="refreshQRCode" :disabled="qrcodeLoading">
            刷新二维码
          </el-button>
        </span>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, computed, watch } from 'vue'
import { Plus, Edit, Delete, RefreshRight, Key, Loading, WarningFilled, View, Hide, Cloudy } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { request } from '../utils/api'
import { get115QRCode, check115LoginStatus, confirm115Login, get115LoginChannels } from '../utils/api/cloud115'

/**
 * 敏感信息脱敏处理
 * @param {string} text - 原始文本
 * @returns {string} 脱敏后的文本
 */
const maskSensitive = (text) => {
  if (!text) return '-'
  if (text.length <= 20) return '******'
  return text.substring(0, 8) + '...' + text.substring(text.length - 8)
}

/**
 * 切换Cookie显示状态
 * @param {Object} row - 行数据
 */
const toggleShowCookie = (row) => {
  row._showCookie = !row._showCookie
}

/**
 * 切换AccessToken显示状态
 * @param {Object} row - 行数据
 */
const toggleShowAccessToken = (row) => {
  row._showAccessToken = !row._showAccessToken
}

/**
 * 切换RefreshToken显示状态
 * @param {Object} row - 行数据
 */
const toggleShowRefreshToken = (row) => {
  row._showRefreshToken = !row._showRefreshToken
}

// 账号类型映射
const accountTypeMap = {
  'resource': { name: '资源号', type: 'primary' },
  'vip': { name: 'VIP观影号', type: 'success' },
  'both': { name: '兼顾', type: 'warning' }
}

// 状态映射
const statusMap = {
  'active': { name: '正常', type: 'success' },
  'cooling': { name: '冷却中', type: 'warning' },
  'disabled': { name: '禁用', type: 'danger' }
}

/**
 * 获取账号类型显示名称
 * @param {string} type - 账号类型
 * @returns {string} 显示名称
 */
const getAccountTypeName = (type) => {
  return accountTypeMap[type]?.name || '资源号'
}

/**
 * 获取账号类型标签类型
 * @param {string} type - 账号类型
 * @returns {string} 标签类型
 */
const getAccountTypeTag = (type) => {
  return accountTypeMap[type]?.type || 'primary'
}

/**
 * 获取状态显示名称
 * @param {string} status - 状态
 * @returns {string} 显示名称
 */
const getStatusName = (status) => {
  return statusMap[status]?.name || '正常'
}

/**
 * 获取状态标签类型
 * @param {string} status - 状态
 * @returns {string} 标签类型
 */
const getStatusTag = (status) => {
  return statusMap[status]?.type || 'success'
}

/**
 * 获取优先级标签类型
 * @param {number} priority - 优先级
 * @returns {string} 标签类型
 */
const getPriorityTag = (priority) => {
  if (priority >= 8) return 'danger'
  if (priority >= 6) return 'warning'
  if (priority >= 4) return 'info'
  return 'success'
}

// 秒传方式映射
const transferMethodMap = {
  '115driver': { name: '115driver', type: 'primary' },
  'go115': { name: 'go115', type: 'success' },
  'alist': { name: 'alist', type: 'warning' }
}

/**
 * 获取秒传方式显示名称
 * @param {string} method - 秒传方式
 * @returns {string} 显示名称
 */
const getTransferMethodName = (method) => {
  if (!method || method === '') {
    return ''
  }
  return transferMethodMap[method]?.name || ''
}

/**
 * 获取秒传方式标签类型
 * @param {string} method - 秒传方式
 * @returns {string} 标签类型
 */
const getTransferMethodTag = (method) => {
  if (!method || method === '') {
    return 'info'
  }
  return transferMethodMap[method]?.type || 'info'
}

// 表格数据
const cloud115List = ref([])
const total = ref(0)
const currentPage = ref(1)
const pageSize = ref(10)

// 排序状态
const sortField = ref('id')
const sortOrder = ref('asc')

// 对话框
const dialogVisible = ref(false)
const dialogTitle = ref('新增115云账号')
const formRef = ref(null)
const loading = ref(false)

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
  name: [{ required: true, message: '请输入账号名称', trigger: 'blur' }],
  cookie: [{ required: false, message: '请输入Cookie', trigger: 'blur' }],
  access_token: [{ required: false, message: '请输入Access Token', trigger: 'blur' }],
  refresh_token: [{ required: false, message: '请输入Refresh Token', trigger: 'blur' }]
}

/**
 * 获取转存账号名称
 * @param {number} accountId - 账号ID
 * @returns {string} 账号名称
 */
const getTransferAccountName = (accountId) => {
  if (!accountId) return '-'
  const account = cloud115List.value.find(item => item.id === accountId)
  return account ? account.name : `账号ID: ${accountId}`
}

/**
 * 获取可用的转存账号列表（排除当前编辑的账号）
 */
const availableTransferAccounts = computed(() => {
  return cloud115List.value.filter(item => item.id !== form.value.id)
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

// 渠道选择相关
const loginChannels = ref([])
const selectedChannel = ref('wechatmini')

// 渠道提示映射
const channelTips = {
  web: '请使用115网页版扫描二维码登录',
  android: '请使用115安卓APP扫描二维码登录',
  ios: '请使用115 iOS APP扫描二维码登录',
  tv: '请使用115电视版扫描二维码登录',
  alipaymini: '请使用支付宝小程序扫描二维码登录',
  wechatmini: '请使用微信小程序扫描二维码登录',
  qandroid: '请使用115安卓Q版扫描二维码登录'
}

// 登录状态文本映射
const loginStatusMap = {
  0: { text: '等待扫码', type: 'info' },
  1: { text: '已扫码，等待确认', type: 'warning' },
  2: { text: '登录成功', type: 'success' },
  3: { text: '二维码已过期', type: 'danger' },
  4: { text: '登录失败', type: 'danger' }
}

// 计算属性
const loginStatusText = computed(() => loginStatusMap[loginStatus.value]?.text || '未知状态')
const loginStatusType = computed(() => loginStatusMap[loginStatus.value]?.type || 'info')
const channelTip = computed(() => channelTips[selectedChannel.value] || '请扫描二维码登录')

/**
 * 格式化过期时间倒计时
 * @param {number} seconds - 剩余秒数
 * @returns {string} 格式化后的时间字符串
 */
const formatExpireTime = (seconds) => {
  if (seconds <= 0) return '已过期'
  const minutes = Math.floor(seconds / 60)
  const secs = seconds % 60
  return `${minutes}分${secs}秒`
}

/**
 * 获取115云账号列表
 */
const fetchCloud115List = async () => {
  try {
    const params = new URLSearchParams()
    params.append('sort_field', sortField.value)
    params.append('sort_order', sortOrder.value)
    const response = await request(`/cloud115?${params.toString()}`)
    const apiData = response.data.data
    cloud115List.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
    total.value = apiData?.total || cloud115List.value.length
  } catch (error) {
    ElMessage.error('获取115云账号列表失败')
  }
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
 * 将微信渠道放在第一位
 * @param {Array} channels - 渠道列表
 * @returns {Array} 排序后的渠道列表
 */
const sortChannelsWithWechatFirst = (channels) => {
  const wechatIndex = channels.findIndex(c => c.value === 'wechatmini')
  if (wechatIndex > 0) {
    const wechat = channels.splice(wechatIndex, 1)[0]
    channels.unshift(wechat)
  }
  return channels
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
const handleSubmit = async () => {
  if (!formRef.value) return
  
  await formRef.value.validate((valid) => {
    if (valid) {
      loading.value = true
      
      const apiUrl = form.value.id ? `/cloud115/${form.value.id}` : '/cloud115'
      const method = form.value.id ? 'PUT' : 'POST'
      
      // 准备提交数据，将 null 转换为 0
      const submitData = {
        ...form.value,
        transfer_account_id: form.value.transfer_account_id || 0
      }
      
      request(apiUrl, {
        method,
        data: submitData
      }).then(() => {
        ElMessage.success(form.value.id ? '编辑成功' : '新增成功')
        dialogVisible.value = false
        fetchCloud115List()
        resetForm()
      }).catch(() => {
        ElMessage.error(form.value.id ? '编辑失败' : '新增失败')
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
  ElMessageBox.confirm(
    '确定要删除这个115云账号吗？',
    '删除确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(() => {
    return request(`/cloud115/${row.id}`, {
      method: 'DELETE'
    })
  }).then(() => {
    ElMessage.success('删除成功')
    fetchCloud115List()
  }).catch(() => {
    ElMessage.error('删除失败')
  })
}

/**
 * 测试账号连接
 * @param {Object} row - 账号数据
 */
const handleTest = (row) => {
  ElMessage.info('正在测试115云账号连接...')
  request(`/auth/cloud115/${row.id}`).then((response) => {
    console.log('115云账号测试结果:', response)
    const data = response.data
    if (data.state && data.data) {
      ElMessage.success(`测试成功！账号: ${data.data.name}`)
    } else {
      ElMessage.error(`测试失败: ${data.message || data.error || '未知错误'}`)
    }
  }).catch((error) => {
    console.error('115云账号测试失败:', error)
    ElMessage.error('测试失败，错误信息已打印到控制台')
  })
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
    formRef.value.resetFields()
  }
}

/**
 * 分页大小改变
 * @param {number} size - 每页数量
 */
const handleSizeChange = (size) => {
  pageSize.value = size
  fetchCloud115List()
}

/**
 * 当前页改变
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
  fetchQRCode()
}

/**
 * 处理扫码更新（更新现有账号）
 * @param {Object} row - 账号数据
 */
const handleQRCodeUpdate = (row) => {
  updateCloudId.value = row.id
  qrcodeDialogTitle.value = `扫码更新账号: ${row.name}`
  qrcodeDialogVisible.value = true
  fetchQRCode()
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
 * 获取登录二维码
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
    
    // 生成二维码图片URL
    qrcodeDataUrl.value = generateQRCodeDataUrl(data.qrcode)
    
    // 设置二维码过期时间（默认120秒）
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
 * 生成二维码Data URL
 * @param {string} content - 二维码内容
 * @returns {string} 二维码图片的Data URL
 */
const generateQRCodeDataUrl = (content) => {
  // 使用第三方API生成二维码图片
  return `https://api.qrserver.com/v1/create-qr-code/?size=200x200&data=${encodeURIComponent(content)}`
}

/**
 * 刷新二维码
 */
const refreshQRCode = () => {
  stopPolling()
  stopExpireTimer()
  fetchQRCode()
}

/**
 * 开始轮询检查登录状态
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
      
      // 状态2表示登录成功
      if (status === 2) {
        stopPolling()
        stopExpireTimer()
        await handleLoginConfirm()
      }
      // 状态3表示二维码过期
      else if (status === 3) {
        stopPolling()
        stopExpireTimer()
        qrcodeError.value = '二维码已过期，请刷新'
      }
      // 状态4表示登录失败
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
 * 确认登录并保存凭据
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
    
    const response = await confirm115Login(confirmData)
    
    ElMessage.success(updateCloudId.value ? '账号Cookie更新成功' : '扫码登录成功，账号已创建')
    qrcodeDialogVisible.value = false
    fetchCloud115List()
  } catch (error) {
    console.error('确认登录失败:', error)
    ElMessage.error('保存登录凭据失败')
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
}

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
onUnmounted(() => {
  stopPolling()
  stopExpireTimer()
})
</script>

<style scoped>
.cloud115-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
}

.main-card {
  border-radius: 12px;
  overflow: hidden;
}

.main-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
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

.header-buttons {
  display: flex;
  gap: 12px;
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
  background-color: #e8f4fd !important;
}

.sensitive-cell {
  display: flex;
  align-items: center;
  gap: 8px;
}

.masked-text {
  font-family: 'Courier New', monospace;
  color: #909399;
  font-size: 13px;
  word-break: break-all;
}

.full-text {
  font-family: 'Courier New', monospace;
  font-size: 12px;
  word-break: break-all;
  max-width: 300px;
  color: #303133;
}

.toggle-btn {
  padding: 2px 6px;
}

.action-buttons {
  display: flex;
  flex-wrap: nowrap;
  gap: 6px;
  justify-content: center;
  white-space: nowrap;
}

.pagination-container {
  display: flex;
  justify-content: flex-end;
  margin-top: 20px;
  padding: 10px 0;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

.qrcode-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 20px;
}

.channel-selector {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-bottom: 20px;
  width: 100%;
}

.channel-label {
  font-size: 14px;
  color: #606266;
  margin-bottom: 12px;
  font-weight: 500;
}

.channel-buttons {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: center;
}

.channel-btn {
  padding: 8px 16px;
  border: 1px solid #dcdfe6;
  border-radius: 6px;
  background-color: #fff;
  color: #606266;
  font-size: 13px;
  cursor: pointer;
  transition: all 0.2s ease;
  outline: none;
}

.channel-btn:hover {
  border-color: #409eff;
  color: #409eff;
  background-color: #ecf5ff;
}

.channel-btn.active {
  border-color: #409eff;
  background-color: #409eff;
  color: #fff;
}

.channel-option {
  display: flex;
  flex-direction: column;
  line-height: 1.2;
}

.channel-desc {
  font-size: 12px;
  color: #909399;
}

.qrcode-loading {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 10px;
  color: #409eff;
}

.qrcode-loading .el-icon {
  font-size: 40px;
}

.qrcode-error {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
  color: #f56c6c;
}

.qrcode-error .el-icon {
  font-size: 40px;
}

.qrcode-content {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 15px;
}

.qrcode-image {
  padding: 15px;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  background-color: #fff;
  box-shadow: 0 2px 12px rgba(0, 0, 0, 0.1);
}

.qrcode-image img {
  width: 200px;
  height: 200px;
}

.qrcode-status {
  margin-top: 10px;
}

.qrcode-tips {
  text-align: center;
  color: #909399;
  font-size: 14px;
}

.qrcode-tips p {
  margin: 5px 0;
}

.expire-tip {
  color: #e6a23c;
  font-weight: bold;
}

.text-muted {
  color: #909399;
  font-size: 13px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}
</style>
