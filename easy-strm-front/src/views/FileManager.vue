<template>
  <div class="space-y-4">
    <section class="overflow-hidden rounded-2xl border border-indigo-100 bg-gradient-to-r from-indigo-50 via-white to-cyan-50 p-4 dark:border-indigo-500/15 dark:from-indigo-500/10 dark:via-white/[0.025] dark:to-cyan-500/10">
      <div class="flex flex-col justify-between gap-3 sm:flex-row sm:items-center">
        <div>
          <div class="flex items-center gap-2">
            <n-icon size="22" color="#6366f1" :component="SwapHorizontalOutline" />
            <h2 class="text-base font-extrabold text-slate-800 dark:text-white">双栏文件管理器</h2>
          </div>
          <p class="mt-1 text-xs leading-5 text-slate-500 dark:text-slate-400">左右两栏可独立选择本地媒体源或任意 115 账号。复制或剪切后，在另一栏进入目标目录并粘贴。</p>
        </div>
        <div v-if="clipboard" class="flex items-center gap-2 rounded-xl border border-indigo-200/70 bg-white/80 px-3 py-2 text-xs dark:border-indigo-400/20 dark:bg-black/15">
          <n-tag :type="clipboard.operation === 'move' ? 'warning' : 'info'" size="small">{{ clipboard.operation === 'move' ? '剪切' : '复制' }}</n-tag>
          <span class="font-semibold text-slate-600 dark:text-slate-300">{{ clipboard.items.length }} 项来自 {{ clipboard.source.name }}</span>
          <n-button text type="error" size="tiny" @click="clipboard = null">清空</n-button>
        </div>
      </div>
    </section>

    <n-alert v-if="loadingLocations" type="info" :show-icon="false">正在加载本地媒体源与115账号…</n-alert>
    <n-alert v-else-if="!locations.length" type="warning">暂无可用位置，请先添加本地媒体源或115账号。</n-alert>

    <div v-else class="flex flex-col gap-4 xl:flex-row">
      <FileManagerPane ref="leftPane" title="左侧位置" :locations="locations" :preferred-index="0" :clipboard-count="clipboard?.items?.length || 0" :pasting="transferPending" @copy="setClipboard('copy', $event)" @cut="setClipboard('move', $event)" @paste="pasteTo($event)" @delete="deleteEntries($event, 'left')" />
      <FileManagerPane ref="rightPane" title="右侧位置" :locations="locations" :preferred-index="1" :clipboard-count="clipboard?.items?.length || 0" :pasting="transferPending" @copy="setClipboard('copy', $event)" @cut="setClipboard('move', $event)" @paste="pasteTo($event)" @delete="deleteEntries($event, 'right')" />
    </div>
  </div>
</template>

<script setup>
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NIcon, NTag } from 'naive-ui'
import { SwapHorizontalOutline } from '@vicons/ionicons5'
import FileManagerPane from '../components/file-manager/FileManagerPane.vue'
import { createFileManagerTransfer, deleteFileManagerEntries, getFileManagerLocations } from '../utils/api/fileManager'
import { getTaskDetail } from '../utils/api/task'
import { message } from '../utils/ui/feedback'
import { showConfirmDialog } from '../utils/ui/messageBox'

const locations = ref([])
const loadingLocations = ref(false)
const clipboard = ref(null)
const transferPending = ref(false)
const leftPane = ref(null)
const rightPane = ref(null)

const loadLocations = async () => {
  loadingLocations.value = true
  try {
    const response = await getFileManagerLocations()
    locations.value = response.data?.data?.data || []
  } catch (error) {
    message.error(error?.response?.data?.error || '加载文件位置失败')
  } finally { loadingLocations.value = false }
}

const setClipboard = (operation, payload) => {
  clipboard.value = { operation, source: payload.location, items: payload.items }
  message.success(`已${operation === 'move' ? '剪切' : '复制'} ${payload.items.length} 项，请选择目标目录后粘贴`)
}

const waitForTransferTask = async taskId => {
  for (let attempt = 0; attempt < 300; attempt += 1) {
    const response = await getTaskDetail(taskId)
    const task = response.data?.data || {}
    if (task.status === 'completed') return task
    if (task.status === 'failed') throw new Error(task.error_message || '文件传输任务执行失败')
    if (task.status === 'cancelled') throw new Error('文件传输任务已取消')
    await new Promise(resolve => setTimeout(resolve, 1000))
  }
  throw new Error('文件传输任务等待超时，请前往任务中心查看')
}

const refreshBothPanes = () => {
  leftPane.value?.refresh?.()
  rightPane.value?.refresh?.()
}

const pasteTo = async target => {
  if (!clipboard.value || transferPending.value) return
  transferPending.value = true
  try {
    const response = await createFileManagerTransfer({
      operation: clipboard.value.operation,
      source: { type: clipboard.value.source.type, id: clipboard.value.source.id },
      target: { type: target.location.type, id: target.location.id },
      target_path: target.path,
      items: clipboard.value.items.map(item => ({ id: item.id, name: item.name, path: item.path, is_directory: item.is_directory, pick_code: item.pick_code || '' }))
    })
    const task = response.data?.data || {}
    if (!task.task_id) throw new Error('文件传输任务未返回任务ID')
    message.info(`文件传输任务已创建：${task.task_id}`)
    await waitForTransferTask(task.task_id)
    if (clipboard.value.operation === 'move') clipboard.value = null
    refreshBothPanes()
    message.success('文件传输完成')
  } catch (error) {
    message.error(error?.response?.data?.error || error?.message || '文件传输任务失败')
  } finally {
    transferPending.value = false
  }
}

const deleteEntries = async (payload, pane) => {
  try {
    await showConfirmDialog(`确定永久删除选中的 ${payload.items.length} 项吗？目录会连同其内容一起删除。`, '删除文件', { confirmButtonText: '删除', cancelButtonText: '取消', type: 'warning' })
    await deleteFileManagerEntries({
      location: { type: payload.location.type, id: payload.location.id },
      items: payload.items.map(item => ({ id: item.id, name: item.name, path: item.path, is_directory: item.is_directory, pick_code: item.pick_code || '' }))
    })
    message.success(`已删除 ${payload.items.length} 项`)
    refreshPane(pane)
  } catch (error) {
    if (error === 'cancel' || error === 'close') return
    message.error(error?.response?.data?.error || '删除失败')
  }
}

const refreshPane = pane => (pane === 'left' ? leftPane.value : rightPane.value)?.refresh?.()
onMounted(loadLocations)
</script>
