<!--
  MediaSourceList - 媒体源管理卡片
  包含媒体源列表表格和新增/编辑媒体源对话框
  支持本地存储和 115 云盘两种类型
-->
<template>
  <el-card shadow="hover" class="source-card">
    <template #header>
      <div class="card-header">
        <div class="header-title">
          <el-icon class="header-icon"><FolderOpened /></el-icon>
          <span>媒体源管理</span>
        </div>
        <el-button type="primary" @click="handleAdd">
          <el-icon><Plus /></el-icon>
          新增媒体源
        </el-button>
      </div>
    </template>

    <section class="source-overview">
      <div class="source-overview__copy">
        <h2>媒体源编排区</h2>
        <p>本地目录和 115 云盘媒体源统一在这里管理。新增、编辑、浏览和自动整理配置仍然沿用现有后端接口。</p>
      </div>
      <div class="source-overview__grid">
        <article v-for="card in sourceOverviewCards" :key="card.label" class="overview-card">
          <span class="overview-card__label">{{ card.label }}</span>
          <strong class="overview-card__value">{{ card.value }}</strong>
          <p class="overview-card__hint">{{ card.hint }}</p>
        </article>
      </div>
    </section>

    <section v-if="mediaSources.length" class="source-highlight">
      <div class="source-highlight__header">
        <div>
          <h3>最近媒体源</h3>
          <span>优先从这里进入浏览和整理</span>
        </div>
      </div>
      <div class="source-highlight__grid">
        <button
          v-for="source in highlightedSources"
          :key="source.id"
          type="button"
          class="source-spotlight"
          @click="emit('browse', source)"
        >
          <div class="source-spotlight__top">
            <strong>{{ source.name }}</strong>
            <el-tag :type="source.source_type === 'local' ? 'success' : 'primary'" size="small" round>
              {{ source.source_type === 'local' ? '本地存储' : '115 云盘' }}
            </el-tag>
          </div>
          <div class="source-spotlight__path">{{ source.path || '/' }}</div>
          <div class="source-spotlight__meta">
            <span>{{ getOrganizeDefaultsSummary(source) }}</span>
            <span>{{ getWatchStatusLabel(source) }}</span>
          </div>
        </button>
      </div>
    </section>

    <div class="table-wrapper">
      <el-table :data="mediaSources" border style="width: 100%" stripe class="custom-table">
        <el-table-column prop="id" label="ID" width="60" align="center" />
        <el-table-column prop="name" label="名称" min-width="120" />
        <el-table-column prop="source_type" label="类型" width="120" align="center">
          <template #default="scope">
            <el-tag :type="scope.row.source_type === 'local' ? 'success' : 'primary'" size="small">
              {{ scope.row.source_type === 'local' ? '本地存储' : '115云盘' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="path" label="路径" min-width="200" show-overflow-tooltip />
        <el-table-column prop="watch_path" label="监控目录" min-width="200" show-overflow-tooltip>
          <template #default="scope">
            <span v-if="scope.row.source_type === 'cloud115'">
              {{ scope.row.watch_path || scope.row.path || '未配置' }}
            </span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="organize_target_path" label="整理目标目录" min-width="150" show-overflow-tooltip>
          <template #default="scope">
            <span v-if="scope.row.organize_target_path">{{ scope.row.organize_target_path }}</span>
            <span v-else class="text-muted">未配置</span>
          </template>
        </el-table-column>
        <el-table-column label="整理默认" min-width="180" show-overflow-tooltip>
          <template #default="scope">
            <span class="organize-default-summary">{{ getOrganizeDefaultsSummary(scope.row) }}</span>
          </template>
        </el-table-column>
        <el-table-column label="监控状态" min-width="220" align="center">
          <template #default="scope">
            <div class="watch-status-cell">
              <el-tag :type="getWatchStatusType(scope.row)" size="small">
                {{ getWatchStatusLabel(scope.row) }}
              </el-tag>
              <span class="watch-status-tip">
                {{ getWatchStatusDescription(scope.row) }}
              </span>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="cloud115_name" label="关联账号" width="120" align="center">
          <template #default="scope">
            <span v-if="scope.row.cloud115_id">{{ scope.row.cloud115_name || '-' }}</span>
            <span v-else class="text-muted">-</span>
          </template>
        </el-table-column>
        <el-table-column prop="create_time" label="创建时间" width="160" align="center" />
        <el-table-column label="操作" width="240" fixed="right" align="center">
          <template #default="scope">
            <!-- 桌面端：平铺按钮 -->
            <div class="action-buttons-desktop">
              <el-button type="primary" size="small" @click="emit('browse', scope.row)">
                <el-icon><Folder /></el-icon>
                浏览
              </el-button>
              <el-button type="warning" size="small" @click="handleEdit(scope.row)">
                <el-icon><Edit /></el-icon>
                编辑
              </el-button>
              <el-button type="danger" size="small" @click="handleDelete(scope.row)">
                <el-icon><Delete /></el-icon>
                删除
              </el-button>
            </div>
            <!-- 移动端：下拉菜单 -->
            <div class="action-buttons-mobile">
              <el-dropdown trigger="click">
                <el-button type="primary" size="small">
                  操作 <el-icon class="el-icon--right"><ArrowDown /></el-icon>
                </el-button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item @click="emit('browse', scope.row)">
                      <el-icon><Folder /></el-icon> 浏览
                    </el-dropdown-item>
                    <el-dropdown-item @click="handleEdit(scope.row)">
                      <el-icon><Edit /></el-icon> 编辑
                    </el-dropdown-item>
                    <el-dropdown-item @click="handleDelete(scope.row)">
                      <el-icon><Delete /></el-icon> 删除
                    </el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <!-- 新增/编辑媒体源对话框 -->
    <el-dialog
      v-model="dialogVisible"
      :title="dialogTitle"
      width="640px"
      append-to-body
    >
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="名称" prop="name">
          <el-input v-model="form.name" placeholder="请输入媒体源名称" />
        </el-form-item>
        <el-form-item label="类型" prop="source_type">
          <el-radio-group v-model="form.source_type" @change="handleSourceTypeChange">
            <el-radio label="local">本地存储</el-radio>
            <el-radio label="cloud115">115云盘</el-radio>
          </el-radio-group>
        </el-form-item>
        <el-form-item label="路径" prop="path">
          <el-input v-model="form.path" placeholder="请输入路径" />
        </el-form-item>
        <el-form-item v-if="form.source_type === 'cloud115'" label="关联账号" prop="cloud115_id">
          <el-select v-model="form.cloud115_id" placeholder="请选择115账号" style="width: 100%">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item v-if="form.source_type === 'cloud115'" label="监控目录" prop="watch_path">
          <el-input
            v-model="form.watch_path"
            placeholder="请输入要轮询的 115 目录 CID"
          />
          <div class="form-tip">这里填写自动监控的目标目录，和上面的“路径”可以不同。</div>
        </el-form-item>
        <el-form-item v-if="form.source_type !== 'cloud115'" label="整理目标目录" prop="organize_target_path">
          <el-input
            v-model="form.organize_target_path"
            placeholder="如 /已整理，留空则使用媒体源路径作为默认目标"
          />
          <div class="form-tip">整理时文件将以此路径为根目录进行分类分发。</div>
        </el-form-item>
        <el-divider content-position="left">整理默认配置</el-divider>
        <el-form-item label="媒体类型" prop="media_type">
          <el-select v-model="form.media_type" style="width: 100%">
            <el-option label="全部" value="all" />
            <el-option label="电影" value="movie" />
            <el-option label="剧集" value="tv" />
          </el-select>
          <div class="form-tip">自动整理时默认采用的媒体类型筛选条件。</div>
        </el-form-item>
        <el-form-item label="冲突策略" prop="conflict_policy">
          <el-select v-model="form.conflict_policy" style="width: 100%">
            <el-option label="跳过" value="skip" />
            <el-option label="覆盖" value="overwrite" />
            <el-option label="追加序号" value="suffix" />
          </el-select>
          <div class="form-tip">目标已存在同名文件时的默认处理方式。</div>
        </el-form-item>
        <el-form-item label="整理方式" prop="operation_mode">
          <el-select v-model="form.operation_mode" style="width: 100%">
            <el-option label="移动文件" value="move" />
            <el-option label="复制文件" value="copy" />
            <el-option label="硬链接" value="hardlink" :disabled="form.source_type === 'cloud115'" />
            <el-option label="软链接" value="symlink" :disabled="form.source_type === 'cloud115'" />
          </el-select>
          <div class="form-tip">
            {{ form.source_type === 'cloud115' ? '115 云盘不支持硬链接和软链接，保存时会自动回退为安全模式。' : '本地源可选硬链接或软链接以节省磁盘空间。' }}
          </div>
        </el-form-item>
        <template v-if="form.source_type === 'cloud115'">
          <div class="feature-panel">
            <div class="feature-panel__header">
              <div>
                <div class="feature-panel__title">115自动监控整理</div>
                <div class="feature-panel__subtitle">监听 115 媒体源中的新增文件，并自动进入整理流程。</div>
              </div>
              <el-tag :type="cloud115FeatureStatus.type" effect="light">
                {{ cloud115FeatureStatus.label }}
              </el-tag>
            </div>
            <el-alert
              type="info"
              :closable="false"
              show-icon
              title="开启后会按轮询间隔扫描 115 目录，只处理新增文件，不影响已有文件。"
              class="feature-alert"
            />
            <el-form-item label="整理目标目录" prop="organize_target_path" class="feature-form-item">
              <el-input
                v-model="form.organize_target_path"
                placeholder="如 /电影库，留空则默认整理回当前媒体源路径"
              />
              <div class="form-tip">建议为 115 自动监控整理单独设置归档目录，便于后续浏览和复查。</div>
            </el-form-item>
            <el-form-item label="目录监控" class="feature-form-item">
              <el-switch
                v-model="form.watch_enabled"
                active-text="开启"
                inactive-text="关闭"
                @change="handleWatchEnabledChange"
              />
              <div class="form-tip">开启后会按设定间隔轮询 115 云盘目录，发现新增文件后继续执行自动整理。</div>
            </el-form-item>
            <el-form-item label="自动整理" class="feature-form-item">
              <el-switch
                v-model="form.auto_organize"
                active-text="开启"
                inactive-text="关闭"
                :disabled="!form.watch_enabled"
              />
              <div class="form-tip">
                {{ form.watch_enabled ? '发现新增文件后自动创建整理任务，结果可在任务列表中查看。' : '请先开启目录监控，自动整理才会生效。' }}
              </div>
            </el-form-item>
            <el-form-item v-if="form.watch_enabled" label="轮询间隔" class="feature-form-item">
              <el-input-number
                v-model="form.watch_interval"
                :min="60"
                :max="86400"
                :step="60"
                style="width: 200px"
              />
              <span style="margin-left: 8px; color: #909399; font-size: 13px">秒</span>
              <div class="form-tip">建议 10 到 30 分钟之间，兼顾及时性和 115 轮询开销。</div>
            </el-form-item>
          </div>
        </template>
        <template v-else>
          <el-form-item label="自动整理">
            <el-switch v-model="form.auto_organize" active-text="开启" inactive-text="关闭" />
            <div class="form-tip">开启后，监控到新文件时会自动触发整理流程。</div>
          </el-form-item>
          <el-form-item label="目录监控">
            <el-switch v-model="form.watch_enabled" active-text="开启" inactive-text="关闭" />
            <div class="form-tip">本地目录使用 fsnotify 实时监控，115 云盘使用轮询监控。</div>
          </el-form-item>
        </template>
        <el-form-item v-if="embyLibraries.length > 0" label="Emby媒体库">
          <el-select v-model="form.emby_library_id" placeholder="可选：绑定Emby媒体库" clearable style="width: 100%">
            <el-option label="不绑定" value="" />
            <el-option
              v-for="lib in embyLibraries"
              :key="lib.ItemId"
              :label="lib.Name"
              :value="lib.ItemId"
            />
          </el-select>
          <div class="form-tip">绑定后，该媒体源整理完成时会自动刷新对应的Emby媒体库。</div>
        </el-form-item>
      </el-form>
      <template #footer>
        <span class="dialog-footer">
          <el-button @click="dialogVisible = false">取消</el-button>
          <el-button type="primary" @click="handleSubmit" :loading="submitLoading">确定</el-button>
        </span>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup>
import { computed, ref, onMounted, onUnmounted } from 'vue'
import {
  FolderOpened,
  Plus,
  Edit,
  Delete,
  Folder,
  ArrowDown
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { showConfirmDialog } from '../../utils/ui/messageBox'
import {
  getMediaSources,
  createMediaSource,
  updateMediaSource,
  deleteMediaSource
} from '../../utils/api/media'
import { getCloud115List } from '../../utils/api/cloud115'
import { getEmbyLibraries } from '../../utils/api/emby'

const emit = defineEmits(['browse'])

// --- 响应式布局状态 ---
const isMobile = ref(window.innerWidth < 768)

let resizeTimer = null
const handleResize = () => {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    isMobile.value = window.innerWidth < 768
  }, 150)
}

// --- 列表状态 ---
const mediaSources = ref([])
const cloud115List = ref([])
const embyLibraries = ref([])

// --- 对话框状态 ---
const dialogVisible = ref(false)
const dialogTitle = ref('新增媒体源')
const formRef = ref(null)
const submitLoading = ref(false)

const form = ref({
  id: null,
  name: '',
  source_type: 'local',
  path: '',
  cloud115_id: null,
  watch_path: '',
  organize_target_path: '',
  media_type: 'all',
  conflict_policy: 'skip',
  operation_mode: 'move',
  auto_organize: false,
  watch_enabled: false,
  watch_interval: 1800,
  emby_library_id: ''
})

const rules = {
  name: [{ required: true, message: '请输入媒体源名称', trigger: 'blur' }],
  source_type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  path: [{ required: true, message: '请输入路径', trigger: 'blur' }]
}

const getWatchStatusType = (source) => {
  if (!source?.watch_enabled) return 'info'
  if (source.auto_organize) return 'success'
  return 'warning'
}

const getWatchStatusLabel = (source) => {
  if (!source?.watch_enabled) return '未开启'
  if (source.auto_organize) return '自动整理中'
  return source?.source_type === 'local' ? '自动监控中' : '仅监控'
}

const getWatchStatusDescription = (source) => {
  if (!source?.watch_enabled) return '未监控新增文件'
  if (source?.source_type === 'local') {
    if (source.auto_organize) return '本地目录实时监控并自动整理'
    return '本地目录实时监控，当前仅监控不整理'
  }
  if (source.auto_organize) {
    const interval = source.watch_interval || 1800
    if (interval >= 3600 && interval % 3600 === 0) {
      return `每 ${interval / 3600} 小时扫描新增文件并自动整理`
    }
    if (interval >= 60 && interval % 60 === 0) {
      return `每 ${interval / 60} 分钟扫描新增文件并自动整理`
    }
    return `每 ${interval} 秒扫描新增文件并自动整理`
  }
  const interval = source.watch_interval || 1800
  if (interval >= 3600 && interval % 3600 === 0) {
    return `每 ${interval / 3600} 小时扫描新增文件，仅监控不整理`
  }
  if (interval >= 60 && interval % 60 === 0) {
    return `每 ${interval / 60} 分钟扫描新增文件，仅监控不整理`
  }
  return `每 ${interval} 秒扫描新增文件，仅监控不整理`
}

const getMediaTypeName = (mediaType) => {
  const map = {
    all: '全部',
    movie: '电影',
    tv: '剧集'
  }
  return map[mediaType] || '全部'
}

const getConflictPolicyName = (policy) => {
  const map = {
    skip: '跳过',
    overwrite: '覆盖',
    suffix: '追加序号'
  }
  return map[policy] || '跳过'
}

const getOperationModeName = (mode) => {
  const map = {
    move: '移动',
    copy: '复制',
    hardlink: '硬链接',
    symlink: '软链接'
  }
  return map[mode] || '移动'
}

const getOrganizeDefaultsSummary = (source) => {
  if (!source) return '未配置'
  return `${getMediaTypeName(source.media_type)} / ${getConflictPolicyName(source.conflict_policy)} / ${getOperationModeName(source.operation_mode)}`
}

const cloud115FeatureStatus = computed(() => {
  if (!form.value.watch_enabled) {
    return {
      type: 'info',
      label: '未开启'
    }
  }
  if (form.value.auto_organize) {
    return {
      type: 'success',
      label: '自动整理中'
    }
  }
  return {
    type: 'warning',
    label: '仅监控'
  }
})

const localSourceCount = computed(() => mediaSources.value.filter(item => item.source_type === 'local').length)
const cloudSourceCount = computed(() => mediaSources.value.filter(item => item.source_type === 'cloud115').length)
const watchEnabledCount = computed(() => mediaSources.value.filter(item => item.watch_enabled).length)
const autoOrganizeCount = computed(() => mediaSources.value.filter(item => item.auto_organize).length)
const sourceOverviewCards = computed(() => [
  {
    label: '媒体源总数',
    value: mediaSources.value.length,
    hint: `${localSourceCount.value} 个本地源，${cloudSourceCount.value} 个云源`
  },
  {
    label: '已开启监控',
    value: watchEnabledCount.value,
    hint: autoOrganizeCount.value ? `${autoOrganizeCount.value} 个会继续自动整理` : '当前没有自动整理中的媒体源'
  },
  {
    label: '115 关联账号',
    value: cloud115List.value.length,
    hint: cloud115List.value.length ? '可用于创建 115 云盘媒体源' : '当前没有可选的 115 账号'
  },
  {
    label: 'Emby 媒体库',
    value: embyLibraries.value.length || '-',
    hint: embyLibraries.value.length ? '可选绑定到整理后的刷新动作' : '打开编辑弹窗时会静默加载'
  }
])
const highlightedSources = computed(() => mediaSources.value.slice(0, 3))

// --- 方法 ---

const fetchMediaSources = async () => {
  try {
    const response = await getMediaSources()
    const apiData = response.data.data
    const sources = Array.isArray(apiData) ? apiData : (apiData?.data || [])
    mediaSources.value = [...sources].sort((a, b) => (b.id || 0) - (a.id || 0))
  } catch (error) {
    console.error('[MediaSourceList] 获取媒体源列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取媒体源列表失败'
    ElMessage.error(errorMsg)
  }
}

const fetchCloud115List = async () => {
  try {
    const response = await getCloud115List()
    const apiData = response.data.data
    cloud115List.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (error) {
    console.error('[MediaSourceList] 获取115账号列表失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '获取115账号列表失败'
    ElMessage.error(errorMsg)
  }
}

/**
 * 加载 Emby 媒体库列表
 * 静默加载，失败时不阻塞媒体源编辑
 */
const loadEmbyLibraries = async () => {
  try {
    const response = await getEmbyLibraries()
    const apiData = response.data.data
    embyLibraries.value = apiData?.data || []
  } catch (_error) {
    embyLibraries.value = []
  }
}

const handleAdd = () => {
  dialogTitle.value = '新增媒体源'
  resetForm()
  dialogVisible.value = true
  loadEmbyLibraries()
}

const handleEdit = (row) => {
  dialogTitle.value = '编辑媒体源'
  form.value = {
    id: row.id,
    name: row.name || '',
    source_type: row.source_type || 'local',
    path: row.path || '',
    cloud115_id: row.cloud115_id || null,
    watch_path: row.watch_path || row.path || '',
    organize_target_path: row.organize_target_path || '',
    media_type: row.media_type || 'all',
    conflict_policy: row.conflict_policy || 'skip',
    operation_mode: row.operation_mode || 'move',
    auto_organize: row.auto_organize || false,
    watch_enabled: row.watch_enabled || false,
    watch_interval: row.watch_interval || 1800,
    emby_library_id: row.emby_library_id || ''
  }
  dialogVisible.value = true
  loadEmbyLibraries()
}

const handleDelete = (row) => {
  showConfirmDialog(
    '确定要删除这个媒体源吗？',
    '删除确认',
    {
      confirmButtonText: '确定',
      cancelButtonText: '取消',
      type: 'warning'
    }
  ).then(async () => {
    try {
      await deleteMediaSource(row.id)
      ElMessage.success('删除成功')
      fetchMediaSources()
    } catch (error) {
      console.error('[MediaSourceList] 删除媒体源失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      ElMessage.error(errorMsg)
    }
  }).catch(() => {})
}

const handleSubmit = async () => {
  if (!formRef.value) return

  await formRef.value.validate(async (valid) => {
    if (valid) {
      if (!form.value.watch_enabled) {
        form.value.auto_organize = false
      }
      if (form.value.source_type === 'cloud115' && form.value.watch_enabled && !String(form.value.watch_path || '').trim()) {
        ElMessage.error('请先填写监控目录')
        submitLoading.value = false
        return
      }
      submitLoading.value = true
      try {
        if (form.value.id) {
          await updateMediaSource(form.value.id, form.value)
          ElMessage.success('编辑成功')
        } else {
          await createMediaSource(form.value)
          ElMessage.success('新增成功')
        }
        dialogVisible.value = false
        fetchMediaSources()
        resetForm()
      } catch (error) {
        console.error('[MediaSourceList] 提交媒体源失败:', error)
        const errorMsg = error.response?.data?.error || error.message || (form.value.id ? '编辑失败' : '新增失败')
        ElMessage.error(errorMsg)
      } finally {
        submitLoading.value = false
      }
    }
  })
}

const resetForm = () => {
  form.value = {
    id: null,
    name: '',
    source_type: 'local',
    path: '',
    cloud115_id: null,
    watch_path: '',
    organize_target_path: '',
    media_type: 'all',
    conflict_policy: 'skip',
    operation_mode: 'move',
    auto_organize: false,
    watch_enabled: false,
    watch_interval: 1800,
    emby_library_id: ''
  }
  if (formRef.value) {
    formRef.value.resetFields()
  }
}

const handleSourceTypeChange = () => {
  form.value.cloud115_id = null
  if (form.value.source_type !== 'cloud115') {
    return
  }
  if (!form.value.watch_interval || form.value.watch_interval < 60) {
    form.value.watch_interval = 1800
  }
  if (!['move', 'copy'].includes(form.value.operation_mode)) {
    form.value.operation_mode = 'move'
  }
}

const handleWatchEnabledChange = (enabled) => {
  if (!enabled) {
    form.value.auto_organize = false
  }
}

onMounted(() => {
  window.addEventListener('resize', handleResize)
  fetchMediaSources()
  fetchCloud115List()
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  if (resizeTimer) {
    clearTimeout(resizeTimer)
  }
})

/**
 * 暴露刷新方法和列表状态供父组件构建工作台摘要
 */
defineExpose({
  fetchMediaSources,
  mediaSources,
  handleAdd
})
</script>

<style scoped>
.form-tip {
  font-size: 12px;
  color: #909399;
  line-height: 1.4;
  margin-top: 4px;
}

.source-card {
  margin-bottom: 20px;
  border-radius: 24px;
  overflow: hidden;
  border: 1px solid rgba(120, 101, 72, 0.12);
  background: rgba(255, 252, 247, 0.86);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

.source-card :deep(.el-card__header) {
  background:
    radial-gradient(circle at top right, rgba(242, 166, 90, 0.24), transparent 30%),
    linear-gradient(135deg, #17313a 0%, #24535f 55%, #1f6f78 100%);
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
  font-size: 18px;
  font-weight: 600;
}

.header-icon {
  font-size: 22px;
}

.source-overview {
  display: grid;
  grid-template-columns: minmax(260px, 1fr) minmax(0, 2fr);
  gap: 18px;
  margin-bottom: 22px;
}

.source-overview__copy {
  padding: 20px;
  border-radius: 22px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.source-overview__copy h2 {
  margin: 0;
  font-size: 24px;
  color: #17313a;
}

.source-overview__copy p {
  margin: 12px 0 0;
  color: #6c6259;
  line-height: 1.7;
}

.source-overview__grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 14px;
}

.overview-card {
  padding: 18px;
  border-radius: 20px;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.94), rgba(247, 241, 231, 0.94));
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
  font-size: 28px;
  color: #17313a;
  line-height: 1.1;
}

.overview-card__hint {
  margin: 10px 0 0;
  color: #73675d;
  line-height: 1.6;
}

.source-highlight {
  margin-bottom: 22px;
}

.source-highlight__header h3 {
  margin: 0;
  font-size: 18px;
  color: #17313a;
}

.source-highlight__header span {
  display: block;
  margin-top: 6px;
  color: #7b6e63;
}

.source-highlight__grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 14px;
  margin-top: 14px;
}

.source-spotlight {
  border: 1px solid rgba(31, 111, 120, 0.12);
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.92), rgba(244, 239, 231, 0.92));
  border-radius: 20px;
  padding: 18px;
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, box-shadow 0.2s ease;
}

.source-spotlight:hover {
  transform: translateY(-2px);
  box-shadow: 0 12px 24px rgba(30, 55, 62, 0.1);
}

.source-spotlight__top {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: flex-start;
}

.source-spotlight__top strong {
  color: #17313a;
  font-size: 16px;
}

.source-spotlight__path {
  margin-top: 12px;
  font-family: 'Courier New', monospace;
  color: #6b6258;
  word-break: break-all;
  font-size: 13px;
}

.source-spotlight__meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-top: 14px;
  color: #87786b;
  font-size: 12px;
}

/* 表格水平滚动容器 */
.table-wrapper {
  overflow-x: auto;
  -webkit-overflow-scrolling: touch;
}

.custom-table {
  border-radius: 18px;
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

.text-muted {
  color: #909399;
  font-size: 13px;
}

.watch-status-cell {
  display: flex;
  flex-direction: column;
  gap: 4px;
  align-items: center;
}

.watch-status-tip {
  color: #909399;
  font-size: 12px;
  line-height: 1.4;
}

.feature-panel {
  margin-bottom: 18px;
  padding: 16px;
  border: 1px solid #d9ecff;
  border-radius: 12px;
  background: linear-gradient(180deg, #f7fbff 0%, #fdfefe 100%);
}

.feature-panel__header {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.feature-panel__title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}

.feature-panel__subtitle {
  margin-top: 4px;
  color: #606266;
  font-size: 13px;
  line-height: 1.5;
}

.feature-alert {
  margin-bottom: 16px;
}

.feature-form-item:last-child {
  margin-bottom: 0;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

/* --- 操作按钮：桌面端/移动端切换 --- */
.action-buttons-mobile {
  display: none;
}

.action-buttons-desktop {
  display: flex;
  justify-content: center;
  gap: 4px;
  flex-wrap: nowrap;
}

:global(.dark) .source-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

:global(.dark) .source-overview__copy,
:global(.dark) .overview-card,
:global(.dark) .source-spotlight {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .source-overview__copy h2,
:global(.dark) .overview-card__value,
:global(.dark) .source-highlight__header h3,
:global(.dark) .source-spotlight__top strong {
  color: #e8edf4;
}

:global(.dark) .source-overview__copy p,
:global(.dark) .overview-card__hint,
:global(.dark) .overview-card__label,
:global(.dark) .source-highlight__header span,
:global(.dark) .source-spotlight__path,
:global(.dark) .source-spotlight__meta,
:global(.dark) .feature-panel__subtitle,
:global(.dark) .form-tip {
  color: #9faebb;
}

/* ===== 响应式：移动端(< 768px) ===== */
@media (max-width: 768px) {
  .source-overview,
  .source-overview__grid,
  .source-highlight__grid {
    grid-template-columns: 1fr;
  }

  .source-card :deep(.el-card__header) {
    padding: 12px 16px;
  }

  .header-title {
    font-size: 15px;
    gap: 6px;
  }

  .card-header {
    flex-wrap: wrap;
    gap: 8px;
  }

  /* 移动端：操作按钮切换为下拉菜单 */
  .action-buttons-desktop {
    display: none;
  }

  .action-buttons-mobile {
    display: block;
  }

  /* 表格最小宽度确保可横向滚动 */
  .custom-table {
    min-width: 800px;
  }

  /* 表单标签宽度适配 */
  :deep(.el-form-item__label) {
    width: 100px !important;
  }

  .feature-panel__header {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

