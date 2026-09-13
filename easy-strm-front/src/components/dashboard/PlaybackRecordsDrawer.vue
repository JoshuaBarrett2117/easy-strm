<template>
  <n-drawer v-model:show="visible" placement="right" width="min(460px, 100vw)" :show-mask="false" :trap-focus="false" :block-scroll="false">
    <n-drawer-content title="STRM 播放记录" closable>
      <div class="flex h-full flex-col">
        <div class="mb-3 text-xs text-slate-400">最近 1000 次播放会话 · 5 分钟内重复解析自动合并 · 每 5 秒刷新</div>
        <div class="mb-3 flex items-center gap-3"><n-button size="small" :loading="loading" @click="refresh">刷新</n-button><span class="text-xs text-slate-400">共 {{ total }} 条</span></div>
        <p v-if="error" role="alert" class="mb-3 text-xs text-red-500">{{ error }}</p>
        <div class="flex-1 space-y-3 overflow-y-auto pr-1">
          <div v-if="records.length === 0 && !loading && !error" class="rounded-xl border border-dashed border-slate-200 p-8 text-center text-sm text-slate-400 dark:border-white/10">暂无播放记录</div>
          <article v-for="record in records" :key="record.id" class="rounded-2xl border border-slate-200/80 bg-white p-3 shadow-sm dark:border-white/10 dark:bg-white/[.04]">
            <div class="flex gap-3">
              <img v-if="record.poster" :src="record.poster" class="h-24 w-16 rounded-lg object-cover" alt="海报">
              <div v-else class="flex h-24 w-16 items-center justify-center rounded-lg bg-slate-100 text-xs text-slate-400 dark:bg-white/10">无海报</div>
              <div class="min-w-0 flex-1">
                <h3 class="break-words text-sm font-bold text-slate-800 dark:text-white" :title="record.name">{{ record.name }}</h3>
                <a :href="record.url" target="_blank" class="mt-1 block truncate text-xs text-indigo-500" :title="record.url">{{ record.url }}</a>
                <p class="mt-3 text-[11px] text-slate-400">{{ new Date(record.time).toLocaleString() }} · {{ record.method }}</p>
                <p class="text-[11px] text-slate-400">{{ record.ip }} · {{ record.location }}</p>
              </div>
            </div>
          </article>
          <n-button v-if="records.length < total" block :loading="loading" @click="loadMore">加载更早记录</n-button>
        </div>
      </div>
    </n-drawer-content>
  </n-drawer>
</template>
<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NDrawer, NDrawerContent } from 'naive-ui'
import { getPlaybackRecords } from '../../utils/api/strm'
const props = defineProps({ show: Boolean })
const emit = defineEmits(['update:show'])
const visible = computed({ get: () => props.show, set: value => emit('update:show', value) })
const records = ref([])
const total = ref(0)
const loading = ref(false)
const error = ref('')
let timer
let request

// 关闭抽屉立即取消请求和轮询，避免旧响应覆盖重开后的状态。
const stop = () => {
  clearTimeout(timer)
  request?.abort()
  request = null
  loading.value = false
}
const load = async (more = false) => {
  if (!props.show || loading.value) return
  clearTimeout(timer)
  const current = new AbortController()
  request = current
  loading.value = true
  try {
    const offset = more ? records.value.length : 0
    const { data: response } = await getPlaybackRecords({ offset, limit: 20 }, current.signal)
    if (request !== current) return
    const result = response.data
    // 按 ID 合并新记录，自动刷新时保留用户正在查看的历史卡片。
    const merged = new Map(records.value.map(record => [record.id, record]))
    for (const record of result.data) merged.set(record.id, record)
    records.value = [...merged.values()].sort((a, b) => new Date(b.time) - new Date(a.time)).slice(0, 1000)
    total.value = result.total
    error.value = ''
  } catch (err) {
    if (!current.signal.aborted) error.value = err.response?.data?.error || '播放记录加载失败，请重试'
  } finally {
    if (request === current) {
      loading.value = false
      request = null
      if (props.show) timer = setTimeout(() => load(), 5000)
    }
  }
}
const refresh = () => load()
const loadMore = () => load(true)
watch(() => props.show, show => {
  stop()
  if (show) { records.value = []; total.value = 0; error.value = ''; load() }
}, { immediate: true })
onBeforeUnmount(stop)
</script>
