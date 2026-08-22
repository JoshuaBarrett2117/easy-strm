<template>
  <section class="flex min-h-[34rem] min-w-0 flex-1 flex-col overflow-hidden rounded-2xl border border-slate-200/80 bg-white shadow-sm dark:border-white/[0.07] dark:bg-white/[0.025]">
    <header class="border-b border-slate-200/70 p-3 dark:border-white/[0.07]">
      <div class="mb-3 flex items-center justify-between gap-2">
        <div>
          <div class="text-sm font-extrabold text-slate-800 dark:text-slate-100">{{ title }}</div>
          <div class="mt-0.5 text-[11px] text-slate-400">{{ entries.length }} 项 · 已选 {{ checkedKeys.length }} 项</div>
        </div>
        <n-button quaternary circle size="small" :loading="loading" title="刷新" @click="loadFiles">
          <template #icon><n-icon :component="RefreshOutline" /></template>
        </n-button>
      </div>
      <n-select v-model:value="selectedLocationKey" :options="locationOptions" placeholder="选择本地媒体源或115账号" @update:value="handleLocationChange" />
      <div class="mt-3 flex min-h-8 items-center gap-1 overflow-x-auto rounded-lg bg-slate-50 px-2 py-1 dark:bg-black/15">
        <n-button v-for="(crumb, index) in history" :key="`${crumb.path}-${index}`" text size="tiny" :type="index === history.length - 1 ? 'primary' : 'default'" @click="navigateHistory(index)">
          {{ crumb.name }}
        </n-button>
        <span v-if="history.length === 0" class="text-xs text-slate-400">请选择位置</span>
      </div>
    </header>

    <div class="flex flex-wrap gap-1.5 border-b border-slate-200/70 px-3 py-2 dark:border-white/[0.07]">
      <n-button size="small" :disabled="!selectedItems.length" @click="emitClipboard('copy')"><template #icon><n-icon :component="CopyOutline" /></template>复制</n-button>
      <n-button size="small" :disabled="!selectedItems.length" @click="emitClipboard('move')"><template #icon><n-icon :component="CutOutline" /></template>剪切</n-button>
      <n-button size="small" type="primary" secondary :loading="pasting" :disabled="pasting || clipboardCount === 0 || !currentLocation" @click="emitPaste"><template #icon><n-icon :component="ClipboardOutline" /></template>粘贴<span v-if="clipboardCount">（{{ clipboardCount }}）</span></n-button>
      <n-button size="small" type="error" secondary :disabled="!selectedItems.length" @click="emitDelete"><template #icon><n-icon :component="TrashOutline" /></template>删除</n-button>
    </div>

    <div class="relative min-h-0 flex-1 p-2">
      <n-spin :show="loading" class="h-full">
        <n-data-table v-if="entries.length" v-model:checked-row-keys="checkedKeys" :columns="columns" :data="entries" :row-key="row => row.id" :max-height="560" size="small" striped />
        <n-empty v-else-if="!loading" class="py-20" description="当前目录为空" />
      </n-spin>
    </div>
  </section>
</template>

<script setup>
import { computed, h, ref, watch } from 'vue'
import { NButton, NDataTable, NEmpty, NIcon, NSelect, NSpin, NTag } from 'naive-ui'
import { ClipboardOutline, CopyOutline, CutOutline, RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import { browseFileManager } from '../../utils/api/fileManager'
import { message } from '../../utils/ui/feedback'

const props = defineProps({
  title: { type: String, required: true },
  locations: { type: Array, default: () => [] },
  preferredIndex: { type: Number, default: 0 },
  clipboardCount: { type: Number, default: 0 },
  pasting: { type: Boolean, default: false }
})
const emit = defineEmits(['copy', 'cut', 'paste', 'delete'])
const loading = ref(false)
const selectedLocationKey = ref(null)
const entries = ref([])
const checkedKeys = ref([])
const currentPath = ref('')
const history = ref([])

const locationOptions = computed(() => props.locations.map(location => ({
  label: `${location.type === 'local' ? '本地' : '115'} · ${location.name}`,
  value: `${location.type}:${location.id}`
})))
const currentLocation = computed(() => props.locations.find(location => `${location.type}:${location.id}` === selectedLocationKey.value) || null)
const selectedItems = computed(() => {
  const keys = new Set(checkedKeys.value)
  return entries.value.filter(entry => keys.has(entry.id))
})

const formatSize = (size) => {
  if (!size) return '-'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let value = Number(size)
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) { value /= 1024; unit += 1 }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`
}

const columns = [
  { type: 'selection' },
  { title: '名称', key: 'name', ellipsis: { tooltip: true }, render: row => h(NButton, { text: true, type: row.is_directory ? 'primary' : 'default', class: 'max-w-full justify-start', onClick: () => row.is_directory && openDirectory(row) }, { default: () => `${row.is_directory ? '📁' : '📄'} ${row.name}` }) },
  { title: '类型', key: 'type', width: 72, render: row => h(NTag, { size: 'small', bordered: false, type: row.is_directory ? 'info' : 'default' }, { default: () => row.is_directory ? '目录' : '文件' }) },
  { title: '大小', key: 'size', width: 90, render: row => row.is_directory ? '-' : formatSize(row.size) }
]

const resetPath = () => {
  currentPath.value = currentLocation.value?.type === 'cloud115' ? '0' : ''
  history.value = currentLocation.value ? [{ name: currentLocation.value.name, path: currentPath.value }] : []
  checkedKeys.value = []
}
const handleLocationChange = () => { resetPath(); loadFiles() }
const loadFiles = async () => {
  if (!currentLocation.value) { entries.value = []; return }
  loading.value = true
  try {
    const response = await browseFileManager({ location_type: currentLocation.value.type, location_id: currentLocation.value.id, path: currentPath.value })
    entries.value = response.data?.data?.entries || []
    checkedKeys.value = []
  } catch (error) {
    entries.value = []
    message.error(error?.response?.data?.error || '读取目录失败')
  } finally { loading.value = false }
}
const openDirectory = (entry) => { currentPath.value = entry.path; history.value.push({ name: entry.name, path: entry.path }); loadFiles() }
const navigateHistory = (index) => { currentPath.value = history.value[index].path; history.value = history.value.slice(0, index + 1); loadFiles() }
const payload = () => ({ location: currentLocation.value, path: currentPath.value, items: selectedItems.value })
const emitClipboard = operation => emit(operation === 'copy' ? 'copy' : 'cut', payload())
const emitPaste = () => emit('paste', { location: currentLocation.value, path: currentPath.value })
const emitDelete = () => emit('delete', payload())

watch(() => props.locations, locations => {
  if (!locations.length || selectedLocationKey.value) return
  const location = locations[Math.min(props.preferredIndex, locations.length - 1)]
  selectedLocationKey.value = `${location.type}:${location.id}`
  resetPath()
  loadFiles()
}, { immediate: true })
defineExpose({ refresh: loadFiles })
</script>
