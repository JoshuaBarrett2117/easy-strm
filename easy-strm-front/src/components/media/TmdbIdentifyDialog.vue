<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="TMDB 手动搜索"
    class="w-[96vw] max-w-[960px]"
  >
    <div class="min-h-[400px]">
      <section class="mb-4 flex flex-col gap-3 sm:flex-row">
        <div class="flex-1 rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
          <h3 class="text-lg font-bold text-slate-800 dark:text-white">手动搜索 TMDB</h3>
          <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
            当自动识别不够准确时，可以在这里切换电影或剧集并人工确认目标条目。
          </p>
        </div>
        <div class="min-w-[180px] rounded-2xl bg-slate-100 p-4 dark:bg-white/5 lg:p-5">
          <span class="block text-xs text-slate-400 dark:text-slate-500">当前关键词</span>
          <strong class="mt-2 block break-all text-sm font-bold text-slate-800 dark:text-white">{{ searchKeyword || '待输入' }}</strong>
        </div>
      </section>

      <div class="mb-5 flex flex-col gap-3 sm:flex-row sm:items-center">
        <n-input
          v-model:value="searchKeyword"
          placeholder="输入电影或剧集名称搜索"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <n-icon :component="SearchOutline" />
          </template>
        </n-input>

        <n-select
          v-model:value="searchType"
          placeholder="类型"
          :options="searchTypeOptions"
          class="w-full shrink-0 sm:w-[140px]"
        />

        <n-button type="primary" class="shrink-0" @click="handleSearch">搜索</n-button>
      </div>

      <n-spin :show="loading">
        <div class="max-h-[500px] overflow-y-auto">
          <div v-if="results.length > 0" class="grid grid-cols-1 gap-5 sm:grid-cols-[repeat(auto-fill,minmax(200px,1fr))]">
            <div
              v-for="item in results"
              :key="item.tmdb_id || item.id"
              data-testid="tmdb-result-item"
              class="cursor-pointer overflow-hidden rounded-xl border border-slate-200 bg-white transition hover:-translate-y-0.5 hover:border-cyan-400 hover:shadow-lg hover:shadow-cyan-500/10 dark:border-white/10 dark:bg-ink-800"
              @click="handleSelect(item)"
            >
              <img
                v-if="item.poster_path"
                :src="item.poster_path.trim()"
                alt="poster"
                class="h-[280px] w-full object-cover"
              />
              <div v-else class="flex h-[280px] w-full items-center justify-center bg-slate-100 text-sm text-slate-400 dark:bg-white/5 dark:text-slate-500">
                无海报
              </div>

              <div class="p-3">
                <div class="text-sm font-semibold text-slate-800 dark:text-white">{{ item.title || item.name }}</div>
                <div class="mt-1.5 text-xs text-slate-400 dark:text-slate-500">
                  {{ item.release_date || item.first_air_date || item.year || '-' }}
                </div>
                <div v-if="item.overview" class="mt-2 line-clamp-4 text-xs leading-relaxed text-slate-500 dark:text-slate-400">
                  {{ item.overview }}
                </div>
                <div v-if="item.vote_average" class="mt-2.5 flex items-center gap-1.5">
                  <n-rate :value="item.vote_average / 2" readonly allow-half size="small" />
                  <span class="text-xs font-medium text-amber-500">{{ item.vote_average.toFixed(1) }}</span>
                </div>
              </div>
            </div>
          </div>

          <EmptyState v-else title="暂无搜索结果" />
        </div>
      </n-spin>
    </div>

    <template #action>
      <div class="flex justify-end">
        <n-button @click="visible = false">关闭</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, watch } from 'vue'
import { NModal, NInput, NSelect, NButton, NSpin, NRate, NIcon, useMessage } from 'naive-ui'
import { SearchOutline } from '@vicons/ionicons5'
import EmptyState from '../common/EmptyState.vue'
import { searchTmdb } from '../../utils/api/media'

const props = defineProps({
  selectMode: {
    type: String,
    default: 'cache'
  },
  currentFile: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['select'])

const visible = defineModel('visible', { type: Boolean, default: false })
const message = useMessage()

const searchKeyword = ref('')
const searchType = ref('movie')
const results = ref([])
const loading = ref(false)

const searchTypeOptions = [
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' }
]

watch(visible, (val) => {
  if (val && props.currentFile) {
    const filename = props.currentFile.name || props.currentFile.file_name || ''
    searchKeyword.value = filename.replace(/\.[^/.]+$/, '')
    results.value = []
  }
})

const handleSearch = async () => {
  if (!searchKeyword.value.trim()) {
    message.warning('请输入搜索关键词')
    return
  }

  loading.value = true
  try {
    const response = await searchTmdb({
      keyword: searchKeyword.value,
      type: searchType.value
    })
    results.value = response.data.data?.data || []
  } catch (error) {
    console.error('[TmdbIdentifyDialog] TMDB 搜索失败:', error)
    const errorMsg = error.response?.data?.error || error.message || 'TMDB 搜索失败'
    message.error(errorMsg)
  } finally {
    loading.value = false
  }
}

const handleSelect = (item) => {
  emit('select', {
    item,
    mode: props.selectMode,
    searchType: searchType.value
  })
}

const setKeyword = (keyword, type) => {
  searchKeyword.value = keyword || ''
  searchType.value = type || 'movie'
  results.value = []
}

defineExpose({ setKeyword })
</script>
