<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="TMDB 候选结果"
    class="w-[96vw] max-w-[760px]"
  >
    <n-spin :show="loading">
      <div class="min-h-[200px]">
        <section class="mb-4 flex flex-col gap-3 sm:flex-row sm:items-center">
          <div class="flex flex-1 items-center gap-2 rounded-xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 px-4 py-3 text-[13px] leading-relaxed text-slate-600 dark:text-slate-300">
            <n-icon :component="InformationCircleOutline" size="18" class="shrink-0 text-cyan-500" />
            <span>为您找到了以下匹配结果，请选择正确的条目；如果都不匹配，可以切换到手动搜索。</span>
          </div>
          <div class="min-w-[110px] rounded-xl bg-slate-100 px-4 py-3 text-center dark:bg-white/5">
            <span class="block text-xs text-slate-400 dark:text-slate-500">候选数量</span>
            <strong class="mt-1.5 block text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ candidatesList.length }}</strong>
          </div>
        </section>

        <div v-if="candidatesList.length > 0" class="flex flex-col gap-3">
          <div
            v-for="(item, index) in candidatesList"
            :key="item.tmdb_id || index"
            class="relative flex cursor-pointer items-start gap-3.5 rounded-xl border-2 border-slate-200 bg-white p-3.5 transition hover:-translate-y-px hover:border-cyan-400 hover:shadow-lg hover:shadow-cyan-500/10 dark:border-white/10 dark:bg-ink-800"
            @click="handleSelect(item)"
          >
            <div class="absolute -left-px -top-px flex h-[26px] w-[26px] items-center justify-center rounded-tl-xl rounded-br-xl bg-cyan-500 text-[13px] font-bold text-white">
              {{ index + 1 }}
            </div>

            <div class="w-20 shrink-0">
              <img
                v-if="item.poster_path"
                :src="item.poster_path.trim()"
                alt="poster"
                class="h-[114px] w-20 rounded-md bg-slate-100 object-cover dark:bg-white/5"
              />
              <div v-else class="flex h-[114px] w-20 items-center justify-center rounded-md bg-slate-100 text-xs text-slate-400 dark:bg-white/5 dark:text-slate-500">
                无海报
              </div>
            </div>

            <div class="min-w-0 flex-1">
              <div class="mb-2 text-base font-bold text-slate-800 dark:text-white">{{ item.title || item.original_title || '-' }}</div>

              <div class="mb-2 flex flex-wrap items-center gap-2 text-[13px] text-slate-500 dark:text-slate-400">
                <span v-if="item.year">{{ item.year }}</span>
                <n-tag v-if="item.media_type" size="small" :type="item.media_type === 'tv' ? 'warning' : 'primary'">
                  {{ item.media_type === 'tv' ? '剧集' : '电影' }}
                </n-tag>
                <span v-if="item.vote_average" class="flex items-center gap-0.5">
                  <n-icon :component="Star" class="text-amber-400" />
                  {{ item.vote_average.toFixed(1) }}
                </span>
              </div>

              <div v-if="item.overview" class="mb-1.5 line-clamp-3 text-[13px] leading-relaxed text-slate-500 dark:text-slate-400">
                {{ item.overview }}
              </div>
              <div
                v-if="item.original_title && item.original_title !== item.title"
                class="text-xs text-slate-400 dark:text-slate-500"
              >
                {{ item.original_title }}
              </div>
            </div>
          </div>
        </div>

        <EmptyState v-else-if="!loading" title="未找到候选结果" />
      </div>
    </n-spin>

    <template #action>
      <div class="flex flex-wrap justify-end gap-2">
        <n-button @click="handleManualSearch">
          <template #icon>
            <n-icon :component="SearchOutline" />
          </template>
          手动搜索
        </n-button>
        <n-button @click="visible = false">取消</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { NModal, NSpin, NButton, NTag, NIcon } from 'naive-ui'
import { InformationCircleOutline, Star, SearchOutline } from '@vicons/ionicons5'
import EmptyState from '../common/EmptyState.vue'

defineProps({
  candidatesList: {
    type: Array,
    default: () => []
  },
  mediaType: {
    type: String,
    default: 'movie'
  }
})

const emit = defineEmits(['select', 'manual-search'])

const visible = defineModel('visible', { type: Boolean, default: false })
const loading = defineModel('loading', { type: Boolean, default: false })

const handleSelect = (candidate) => {
  emit('select', candidate)
}

const handleManualSearch = () => {
  emit('manual-search')
}
</script>
