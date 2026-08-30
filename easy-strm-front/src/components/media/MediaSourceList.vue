<!--
  MediaSourceList - 媒体源管理卡片
  包含媒体源列表表格和新增/编辑媒体源对话框
  支持本地存储和 115 云盘两种类型
-->
<template>
  <PageCard title="媒体源管理" subtitle="本地目录和 115 云盘媒体源统一在这里管理">
    <template #action>
      <n-button type="primary" @click="handleAdd">
        <template #icon>
          <n-icon :component="AddOutline" />
        </template>
        新增媒体源
      </n-button>
    </template>

    <section class="mb-4 grid gap-4 lg:grid-cols-[minmax(260px,1fr)_minmax(0,2fr)]">
      <div class="rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
        <h2 class="text-lg font-bold text-slate-800 dark:text-white">媒体源编排区</h2>
        <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
          本地目录和 115 云盘媒体源统一在这里管理。新增、编辑、浏览和自动整理配置仍然沿用现有后端接口。
        </p>
      </div>
      <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
        <StatCard
          v-for="card in sourceOverviewCards"
          :key="card.label"
          :label="card.label"
          :value="card.value"
          :hint="card.hint"
        />
      </div>
    </section>

    <section v-if="mediaSources.length" class="mb-4">
      <div>
        <h3 class="text-base font-bold text-slate-800 dark:text-white">最近媒体源</h3>
        <span class="mt-1 block text-xs text-slate-400 dark:text-slate-500">优先从这里进入浏览和整理</span>
      </div>
      <div class="mt-3 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        <button
          v-for="source in highlightedSources"
          :key="source.id"
          type="button"
          class="rounded-2xl border border-slate-200 bg-white p-4 text-left shadow-sm transition-all hover:-translate-y-0.5 hover:shadow-md dark:border-white/5 dark:bg-ink-800"
          @click="emit('browse', source)"
        >
          <div class="flex items-start justify-between gap-3">
            <strong class="text-sm font-bold text-slate-800 dark:text-white">{{ source.name }}</strong>
            <n-tag :type="source.source_type === 'local' ? 'success' : 'primary'" size="small" round>
              {{ source.source_type === 'local' ? '本地存储' : '115 云盘' }}
            </n-tag>
          </div>
          <div class="mt-3 break-all font-mono text-xs text-slate-500 dark:text-slate-400">{{ source.path || '/' }}</div>
          <div class="mt-3 flex flex-wrap justify-between gap-2 text-xs text-slate-400 dark:text-slate-500">
            <span>{{ getOrganizeDefaultsSummary(source) }}</span>
            <span>{{ getWatchStatusLabel(source) }}</span>
          </div>
        </button>
      </div>
    </section>

    <div class="overflow-x-auto">
      <n-data-table
        :columns="columns"
        :data="mediaSources"
        :striped="true"
        :row-key="(row) => row.id"
        :scroll-x="1640"
      />
    </div>

    <!-- 新增/编辑媒体源对话框 -->
    <n-modal
      v-model:show="dialogVisible"
      preset="card"
      :title="dialogTitle"
      class="w-[92vw] max-w-2xl"
      :mask-closable="false"
    >
      <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
        <div class="grid gap-x-4 sm:grid-cols-2">
          <n-form-item label="名称" path="name">
            <n-input v-model:value="form.name" placeholder="请输入媒体源名称" />
          </n-form-item>
          <n-form-item label="类型" path="source_type">
            <n-radio-group v-model:value="form.source_type" @update:value="handleSourceTypeChange">
              <n-radio value="local">本地存储</n-radio>
              <n-radio value="cloud115">115云盘</n-radio>
            </n-radio-group>
          </n-form-item>
          <n-form-item label="媒体源状态">
            <div class="w-full">
              <n-switch v-model:value="form.enabled">
                <template #checked>启用</template>
                <template #unchecked>停用</template>
              </n-switch>
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">停用后不会启动目录监控或自动整理。</p>
            </div>
          </n-form-item>
          <n-form-item label="路径" path="path" class="sm:col-span-2">
            <TargetFolderPicker
              v-if="form.source_type === 'cloud115'"
              :cloud-115-id="form.cloud115_id || 0"
              :default-path="form.path"
              placeholder="请选择媒体源根目录"
              @update:path="form.path = $event"
            />
            <n-input v-else v-model:value="form.path" placeholder="请输入本地绝对路径" />
          </n-form-item>
          <n-form-item v-if="form.source_type === 'cloud115'" label="关联账号" path="cloud115_id">
            <n-select
              v-model:value="form.cloud115_id"
              placeholder="请选择115账号"
              :options="cloud115Options"
              @update:value="handleCloud115AccountChange"
            />
          </n-form-item>
          <n-form-item v-if="form.source_type === 'cloud115'" label="监控目录" path="watch_path">
            <div class="w-full">
              <TargetFolderPicker
                :cloud-115-id="form.cloud115_id || 0"
                :default-path="form.watch_path"
                placeholder="请选择要轮询的 115 目录"
                @update:path="form.watch_path = $event"
              />
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">保存为绝对路径，例如 /影视资源；可以和媒体源根目录不同。</p>
            </div>
          </n-form-item>
          <n-form-item v-if="form.source_type !== 'cloud115'" label="整理目标目录" path="organize_target_path" class="sm:col-span-2">
            <div class="w-full">
              <n-input
                v-model:value="form.organize_target_path"
                placeholder="如 /已整理，留空则使用媒体源路径作为默认目标"
              />
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">整理时文件将以此路径为根目录进行分类分发。</p>
            </div>
          </n-form-item>
        </div>

        <n-divider title-placement="left">整理默认配置</n-divider>

        <div class="grid gap-x-4 sm:grid-cols-2">
          <n-form-item label="媒体类型" path="media_type">
            <div class="w-full">
              <n-select v-model:value="form.media_type" :options="mediaTypeOptions" />
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">自动整理时默认采用的媒体类型筛选条件。</p>
            </div>
          </n-form-item>
          <n-form-item label="冲突策略" path="conflict_policy">
            <div class="w-full">
              <n-select v-model:value="form.conflict_policy" :options="conflictPolicyOptions" />
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">目标已存在同名文件时的默认处理方式。</p>
            </div>
          </n-form-item>
          <n-form-item label="整理方式" path="operation_mode" class="sm:col-span-2">
            <div class="w-full">
              <n-select v-model:value="form.operation_mode" :options="operationModeOptions" />
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">
                {{ form.source_type === 'cloud115' ? '115 云盘不支持硬链接和软链接，保存时会自动回退为安全模式。' : '本地源可选硬链接或软链接以节省磁盘空间。' }}
              </p>
            </div>
          </n-form-item>
        </div>

        <template v-if="form.source_type === 'cloud115'">
          <div class="mb-4 rounded-2xl border border-cyan-500/20 bg-cyan-500/5 p-4">
            <div class="mb-3 flex flex-wrap items-start justify-between gap-3">
              <div>
                <div class="text-sm font-bold text-slate-800 dark:text-white">115自动监控整理</div>
                <div class="mt-1 text-xs leading-relaxed text-slate-500 dark:text-slate-400">监听 115 媒体源中的新增文件，并自动进入整理流程。</div>
              </div>
              <n-tag :type="cloud115FeatureStatus.type" size="small">
                {{ cloud115FeatureStatus.label }}
              </n-tag>
            </div>
            <n-alert
              type="info"
              :closable="false"
              :show-icon="true"
              title="开启后会按轮询间隔扫描 115 目录，只处理新增文件，不影响已有文件。"
              class="mb-4"
            />
            <n-form-item label="整理目标目录" path="organize_target_path">
              <div class="w-full">
                <TargetFolderPicker
                  :cloud-115-id="form.cloud115_id || 0"
                  :default-path="form.organize_target_path"
                  placeholder="请选择整理目标目录"
                  @update:path="form.organize_target_path = $event"
                />
                <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">建议为 115 自动监控整理单独设置归档目录，便于后续浏览和复查。</p>
              </div>
            </n-form-item>
            <n-form-item label="目录监控">
              <div class="w-full">
                <n-switch v-model:value="form.watch_enabled" @update:value="handleWatchEnabledChange">
                  <template #checked>开启</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">开启后会按设定间隔轮询 115 云盘目录，发现新增文件后继续执行自动整理。</p>
              </div>
            </n-form-item>
            <n-form-item label="自动整理">
              <div class="w-full">
                <n-switch v-model:value="form.auto_organize" :disabled="!form.watch_enabled">
                  <template #checked>开启</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">
                  {{ form.watch_enabled ? '发现新增文件后自动创建整理任务，结果可在任务列表中查看。' : '请先开启目录监控，自动整理才会生效。' }}
                </p>
              </div>
            </n-form-item>
            <n-form-item v-if="form.watch_enabled" label="轮询间隔">
              <div class="w-full">
                <div class="flex items-center gap-2">
                  <n-input-number
                    v-model:value="form.watch_interval"
                    :min="60"
                    :max="86400"
                    :step="60"
                    class="w-[200px]"
                  />
                  <span class="text-[13px] text-slate-400 dark:text-slate-500">秒</span>
                </div>
                <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">建议 10 到 30 分钟之间，兼顾及时性和 115 轮询开销。</p>
              </div>
            </n-form-item>
          </div>
        </template>
        <template v-else>
          <n-form-item label="自动整理">
            <div class="w-full">
              <n-switch v-model:value="form.auto_organize">
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </n-switch>
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">开启后，监控到新文件时会自动触发整理流程。</p>
            </div>
          </n-form-item>
          <n-form-item label="目录监控">
            <div class="w-full">
              <n-switch v-model:value="form.watch_enabled">
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </n-switch>
              <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">本地目录使用 fsnotify 实时监控，115 云盘使用轮询监控。</p>
            </div>
          </n-form-item>
        </template>

        <n-form-item v-if="embyLibraries.length > 0" label="Emby媒体库">
          <div class="w-full">
            <n-select
              v-model:value="form.emby_library_id"
              placeholder="可选：绑定Emby媒体库"
              clearable
              :options="embyLibraryOptions"
            />
            <p class="mt-1 text-xs leading-relaxed text-slate-400 dark:text-slate-500">绑定后，该媒体源整理完成时会自动刷新对应的Emby媒体库。</p>
          </div>
        </n-form-item>
      </n-form>

      <template #action>
        <div class="flex flex-wrap justify-end gap-2">
          <n-button @click="dialogVisible = false">取消</n-button>
          <n-button type="primary" :loading="submitLoading" @click="handleSubmit">确定</n-button>
        </div>
      </template>
    </n-modal>
  </PageCard>
</template>

<script setup>
import { computed, h, onMounted, onUnmounted, ref } from 'vue'
import {
  NButton,
  NDataTable,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NRadioGroup,
  NRadio,
  NTag,
  NAlert,
  NDivider,
  NIcon,
  NDropdown,
  useMessage
} from 'naive-ui'
import {
  AddOutline,
  FolderOpenOutline,
  CreateOutline,
  TrashOutline,
  ChevronDownOutline
} from '@vicons/ionicons5'
import PageCard from '../common/PageCard.vue'
import StatCard from '../common/StatCard.vue'
import TargetFolderPicker from '../resource/TargetFolderPicker.vue'
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

const message = useMessage()

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
  enabled: true,
  auto_organize: false,
  watch_enabled: false,
  watch_interval: 1800,
  emby_library_id: ''
})

const rules = {
  name: [{ required: true, message: '请输入媒体源名称', trigger: 'blur' }],
  source_type: [{ required: true, message: '请选择类型', trigger: 'change' }],
  path: [{ required: true, message: '请选择或输入路径', trigger: ['blur', 'change'] }]
}

const mediaTypeOptions = [
  { label: '全部', value: 'all' },
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' }
]

const conflictPolicyOptions = [
  { label: '跳过', value: 'skip' },
  { label: '覆盖', value: 'overwrite' },
  { label: '追加序号', value: 'suffix' }
]

const operationModeOptions = computed(() => [
  { label: '移动文件', value: 'move' },
  { label: '复制文件', value: 'copy' },
  { label: '硬链接', value: 'hardlink', disabled: form.value.source_type === 'cloud115' },
  { label: '软链接', value: 'symlink', disabled: form.value.source_type === 'cloud115' }
])

const cloud115Options = computed(() => cloud115List.value.map((account) => ({
  label: account.name,
  value: account.id
})))

const getCloud115Name = (source) => {
  if (!source?.cloud115_id) return '-'
  return cloud115List.value.find(account => account.id === source.cloud115_id)?.name || '-'
}

const embyLibraryOptions = computed(() => [
  { label: '不绑定', value: '' },
  ...embyLibraries.value.map((lib) => ({
    label: lib.Name,
    value: lib.ItemId
  }))
])

const getWatchStatusType = (source) => {
  if (!source?.enabled) return 'default'
  if (!source?.watch_enabled) return 'info'
  if (source.watch_running === false) return 'error'
  if (source.auto_organize) return 'success'
  return 'warning'
}

const getWatchStatusLabel = (source) => {
  if (!source?.enabled) return '媒体源已停用'
  if (!source?.watch_enabled) return '未开启'
  if (source.watch_running === false) return '监控未运行'
  if (source.auto_organize) return '自动整理中'
  return source?.source_type === 'local' ? '自动监控中' : '仅监控'
}

const getWatchStatusDescription = (source) => {
  if (!source?.enabled) return '停用状态下不会启动监控'
  if (!source?.watch_enabled) return '未监控新增文件'
  if (source.watch_running === false) return '配置已保存，但后台监听启动失败'
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
const watchEnabledCount = computed(() => mediaSources.value.filter(item => item.watch_running).length)
const autoOrganizeCount = computed(() => mediaSources.value.filter(item => item.watch_running && item.auto_organize).length)
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

// --- 表格列定义(render 中读取的 ref 会被表格渲染副作用跟踪，自动响应更新) ---
const mutedText = 'text-[13px] text-slate-400 dark:text-slate-500'

const rowActionOptions = [
  {
    label: '浏览',
    key: 'browse',
    icon: () => h(NIcon, { component: FolderOpenOutline })
  },
  {
    label: '编辑',
    key: 'edit',
    icon: () => h(NIcon, { component: CreateOutline })
  },
  {
    label: '删除',
    key: 'delete',
    icon: () => h(NIcon, { component: TrashOutline })
  }
]

const handleRowAction = (key, row) => {
  if (key === 'browse') {
    emit('browse', row)
  } else if (key === 'edit') {
    handleEdit(row)
  } else if (key === 'delete') {
    handleDelete(row)
  }
}

const columns = [
  { title: 'ID', key: 'id', width: 60, align: 'center' },
  { title: '名称', key: 'name', minWidth: 120 },
  {
    title: '类型',
    key: 'source_type',
    width: 110,
    align: 'center',
    render: (row) => h(
      NTag,
      { type: row.source_type === 'local' ? 'success' : 'primary', size: 'small' },
      { default: () => (row.source_type === 'local' ? '本地存储' : '115云盘') }
    )
  },
  { title: '路径', key: 'path', minWidth: 200, ellipsis: { tooltip: true } },
  {
    title: '监控目录',
    key: 'watch_path',
    minWidth: 200,
    ellipsis: { tooltip: true },
    render: (row) => (
      row.source_type === 'cloud115'
        ? h('span', row.watch_path || row.path || '未配置')
        : h('span', { class: mutedText }, '-')
    )
  },
  {
    title: '整理目标目录',
    key: 'organize_target_path',
    minWidth: 150,
    ellipsis: { tooltip: true },
    render: (row) => (
      row.organize_target_path
        ? h('span', row.organize_target_path)
        : h('span', { class: mutedText }, '未配置')
    )
  },
  {
    title: '整理默认',
    key: 'organize_defaults',
    minWidth: 180,
    ellipsis: { tooltip: true },
    render: (row) => getOrganizeDefaultsSummary(row)
  },
  {
    title: '监控状态',
    key: 'watch_status',
    minWidth: 220,
    align: 'center',
    render: (row) => h('div', { class: 'flex flex-col items-center gap-1' }, [
      h(
        NTag,
        { type: getWatchStatusType(row), size: 'small' },
        { default: () => getWatchStatusLabel(row) }
      ),
      h('span', { class: 'text-xs leading-snug text-slate-400 dark:text-slate-500' }, getWatchStatusDescription(row))
    ])
  },
  {
    title: '关联账号',
    key: 'cloud115_name',
    width: 120,
    align: 'center',
    render: (row) => (
      row.cloud115_id
        ? h('span', getCloud115Name(row))
        : h('span', { class: mutedText }, '-')
    )
  },
  { title: '创建时间', key: 'create_time', width: 160, align: 'center' },
  {
    title: '操作',
    key: 'actions',
    width: 240,
    align: 'center',
    fixed: 'right',
    render: (row) => {
      // 移动端：下拉菜单;桌面端：平铺按钮
      if (isMobile.value) {
        return h(
          NDropdown,
          {
            trigger: 'click',
            options: rowActionOptions,
            onSelect: (key) => handleRowAction(key, row)
          },
          {
            default: () => h(
              NButton,
              { type: 'primary', size: 'small' },
              {
                default: () => '操作',
                icon: () => h(NIcon, { component: ChevronDownOutline })
              }
            )
          }
        )
      }
      return h('div', { class: 'flex flex-nowrap justify-center gap-1' }, [
        h(
          NButton,
          { type: 'primary', size: 'small', onClick: () => emit('browse', row) },
          {
            default: () => '浏览',
            icon: () => h(NIcon, { component: FolderOpenOutline })
          }
        ),
        h(
          NButton,
          { type: 'warning', size: 'small', onClick: () => handleEdit(row) },
          {
            default: () => '编辑',
            icon: () => h(NIcon, { component: CreateOutline })
          }
        ),
        h(
          NButton,
          { type: 'error', size: 'small', onClick: () => handleDelete(row) },
          {
            default: () => '删除',
            icon: () => h(NIcon, { component: TrashOutline })
          }
        )
      ])
    }
  }
]

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
    message.error(errorMsg)
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
    message.error(errorMsg)
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
    enabled: row.enabled !== false,
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
      message.success('删除成功')
      fetchMediaSources()
    } catch (error) {
      console.error('[MediaSourceList] 删除媒体源失败:', error)
      const errorMsg = error.response?.data?.error || error.message || '删除失败'
      message.error(errorMsg)
    }
  }).catch(() => {})
}

const handleSubmit = () => {
  if (!formRef.value) return

  formRef.value.validate(async (errors) => {
    if (!errors) {
      if (!form.value.watch_enabled) {
        form.value.auto_organize = false
      }
      if (form.value.source_type === 'cloud115' && form.value.watch_enabled && !String(form.value.watch_path || '').trim()) {
        message.error('请先填写监控目录')
        submitLoading.value = false
        return
      }
      submitLoading.value = true
      try {
        let response
        if (form.value.id) {
          response = await updateMediaSource(form.value.id, form.value)
        } else {
          response = await createMediaSource(form.value)
        }
        const savedSource = response?.data?.data?.data
        if (form.value.enabled && form.value.watch_enabled && savedSource?.watch_running === false) {
          message.warning('配置已保存，但监控未启动，请检查目录、账号或后端日志')
        } else {
          message.success(form.value.id ? '编辑成功' : '新增成功')
        }
        dialogVisible.value = false
        fetchMediaSources()
        resetForm()
      } catch (error) {
        console.error('[MediaSourceList] 提交媒体源失败:', error)
        const errorMsg = error.response?.data?.error || error.message || (form.value.id ? '编辑失败' : '新增失败')
        message.error(errorMsg)
      } finally {
        submitLoading.value = false
      }
    }
  }).catch(() => {})
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
    enabled: true,
    auto_organize: false,
    watch_enabled: false,
    watch_interval: 1800,
    emby_library_id: ''
  }
  if (formRef.value) {
    formRef.value.restoreValidation()
  }
}

const handleSourceTypeChange = () => {
  form.value.cloud115_id = null
  form.value.path = ''
  form.value.watch_path = ''
  form.value.organize_target_path = ''
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

const handleCloud115AccountChange = () => {
  form.value.path = ''
  form.value.watch_path = ''
  form.value.organize_target_path = ''
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
