<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="整理结果"
    class="w-[96vw] max-w-[900px]"
    :mask-closable="false"
  >
    <section class="mb-4 rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
      <h3 class="text-lg font-bold text-slate-800 dark:text-white">整理执行回执</h3>
      <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
        这里展示本轮整理的成功、跳过与失败明细；后续还能继续重试失败项、刷新 Emby 或生成 STRM。
      </p>
    </section>

    <div v-if="normalizedSummary" class="mb-4 grid grid-cols-2 gap-3 sm:grid-cols-4">
      <div class="flex flex-col items-center rounded-xl bg-slate-500/10 px-4 py-3">
        <span class="text-2xl font-bold tabular-nums text-slate-600 dark:text-slate-300">{{ normalizedSummary.total || 0 }}</span>
        <span class="mt-1 text-xs text-slate-400 dark:text-slate-500">总计</span>
      </div>
      <div class="flex flex-col items-center rounded-xl bg-emerald-500/10 px-4 py-3">
        <span class="text-2xl font-bold tabular-nums text-emerald-600 dark:text-emerald-400">{{ normalizedSummary.success || 0 }}</span>
        <span class="mt-1 text-xs text-slate-400 dark:text-slate-500">成功</span>
      </div>
      <div class="flex flex-col items-center rounded-xl bg-amber-500/10 px-4 py-3">
        <span class="text-2xl font-bold tabular-nums text-amber-600 dark:text-amber-400">{{ normalizedSummary.skipped || 0 }}</span>
        <span class="mt-1 text-xs text-slate-400 dark:text-slate-500">跳过</span>
      </div>
      <div class="flex flex-col items-center rounded-xl bg-red-500/10 px-4 py-3">
        <span class="text-2xl font-bold tabular-nums text-red-600 dark:text-red-400">{{ normalizedSummary.failed || 0 }}</span>
        <span class="mt-1 text-xs text-slate-400 dark:text-slate-500">失败</span>
      </div>
    </div>

    <div v-if="displayResultList.length > 0" class="overflow-x-auto">
      <n-data-table
        :columns="resultColumns"
        :data="displayResultList"
        :max-height="420"
        :striped="true"
        :scroll-x="840"
      />
    </div>
    <EmptyState v-else title="暂无整理结果" />

    <template #action>
      <div class="flex flex-wrap justify-end gap-2">
        <n-button
          v-if="failedItems.length > 0"
          type="warning"
          :loading="executing"
          @click="handleRetryFailed"
        >
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          重试失败项 ({{ failedItems.length }})
        </n-button>
        <n-button
          v-if="canRefreshEmby"
          type="warning"
          :loading="embyRefreshing"
          @click="handleRefreshEmby"
        >
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新 Emby
        </n-button>
        <n-button
          v-if="canGenerateStrm"
          type="success"
          :loading="strmGenerating"
          @click="handleGenerateStrm"
        >
          <template #icon>
            <n-icon :component="VideocamOutline" />
          </template>
          生成 STRM
        </n-button>
        <n-button type="primary" @click="visible = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { computed, h, ref } from 'vue'
import { NModal, NDataTable, NButton, NTag, NIcon, useMessage } from 'naive-ui'
import { RefreshOutline, VideocamOutline } from '@vicons/ionicons5'
import EmptyState from '../common/EmptyState.vue'
import { generateStrmFromOrganize } from '../../utils/api/strm'
import { refreshEmbyLibrary } from '../../utils/api/emby'

const props = defineProps({
  summary: {
    type: Object,
    default: null
  },
  resultList: {
    type: [Array, Object],
    default: () => []
  },
  currentSource: {
    type: Object,
    default: null
  },
  executing: {
    type: Boolean,
    default: false
  }
})

const emit = defineEmits(['retry-failed', 'refresh-file-list'])

const visible = defineModel('visible', { type: Boolean, default: false })
const message = useMessage()

const normalizedResultList = computed(() => {
  if (Array.isArray(props.resultList)) {
    return props.resultList
  }
  if (Array.isArray(props.resultList?.data)) {
    return props.resultList.data
  }
  return []
})

const normalizedSummary = computed(() => {
  if (props.summary && typeof props.summary === 'object') {
    return props.summary
  }
  if (props.resultList?.summary && typeof props.resultList.summary === 'object') {
    return props.resultList.summary
  }
  if (normalizedResultList.value.length === 0) {
    return null
  }
  return {
    total: normalizedResultList.value.length,
    success: normalizedResultList.value.filter(item => item.success).length,
    skipped: normalizedResultList.value.filter(item => item.skipped).length,
    failed: normalizedResultList.value.filter(item => !item.success && !item.skipped).length
  }
})

const displayResultList = computed(() => normalizedResultList.value.map(item => ({
  ...item,
  file_name: item.file_name || item.new_name || item.old_path?.split(/[\\/]/).pop() || item.new_path?.split(/[\\/]/).pop() || '-'
})))

const failedItems = computed(() => normalizedResultList.value.filter(item => !item.success && !item.skipped))
const canRefreshEmby = computed(() => Boolean(props.currentSource?.emby_library_id))
const canGenerateStrm = computed(() => {
  if (!props.currentSource || props.currentSource.source_type !== 'cloud115') {
    return false
  }
  return normalizedResultList.value.some(item => item.success)
})

const embyRefreshing = ref(false)
const strmGenerating = ref(false)
const getNestedData = (response) => response?.data?.data || response?.data || {}

const resultColumns = [
  { title: '文件名', key: 'file_name', minWidth: 220 },
  { title: '结果', key: 'message', minWidth: 220 },
  { title: '目标路径', key: 'new_path', minWidth: 280, ellipsis: { tooltip: true } },
  {
    title: '状态',
    key: 'status',
    width: 120,
    align: 'center',
    render: (row) => {
      if (row.skipped) {
        return h(NTag, { type: 'warning', size: 'small' }, { default: () => '已跳过' })
      }
      if (row.success) {
        return h(NTag, { type: 'success', size: 'small' }, { default: () => '成功' })
      }
      return h(NTag, { type: 'error', size: 'small' }, { default: () => '失败' })
    }
  }
]

const handleRetryFailed = () => {
  emit('retry-failed', failedItems.value)
}

const handleRefreshEmby = async () => {
  const libraryId = props.currentSource?.emby_library_id
  if (!libraryId) {
    message.warning('当前媒体源未绑定 Emby 媒体库')
    return
  }

  embyRefreshing.value = true
  try {
    const response = await refreshEmbyLibrary(libraryId)
    const payload = getNestedData(response)
    if (payload?.success === false) {
      message.error(payload.message || '刷新 Emby 媒体库失败')
      return
    }
    message.success('Emby 媒体库刷新已触发')
  } catch (error) {
    console.error('[OrganizeResultDialog] 刷新 Emby 媒体库失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '刷新 Emby 媒体库失败'
    message.error(errorMsg)
  } finally {
    embyRefreshing.value = false
  }
}

const handleGenerateStrm = async () => {
  if (!props.currentSource) return

  const successItems = normalizedResultList.value.filter(item => item.success && item.new_path)
  if (successItems.length === 0) {
    message.warning('没有成功整理的文件可生成 STRM')
    return
  }

  const targetPaths = [...new Set(
    successItems
      .map(item => {
        const currentPath = item.new_path || ''
        const parts = currentPath.split('/').filter(Boolean)
        if (parts.length >= 2) {
          return `/${parts.slice(0, 2).join('/')}`
        }
        return currentPath
      })
      .filter(Boolean)
  )]

  strmGenerating.value = true
  try {
    const response = await generateStrmFromOrganize({
      source_id: props.currentSource.id,
      target_paths: targetPaths,
      strm_config_id: 0
    })
    const data = response.data.data || response.data || {}
    message.success(`STRM 生成任务已创建，配置 ID: ${data.strm_config_id || '自动匹配'}`)
    visible.value = false
  } catch (error) {
    console.error('[OrganizeResultDialog] 生成 STRM 失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '生成 STRM 失败'
    message.error(errorMsg)
  } finally {
    strmGenerating.value = false
  }
}
</script>
