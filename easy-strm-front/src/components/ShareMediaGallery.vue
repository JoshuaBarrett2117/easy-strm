<template>
  <section class="share-gallery">
    <div class="gallery-heading"><strong>已识别媒体 <span>{{ media.length }}</span></strong><span>封面与媒体信息</span></div>
    <div class="gallery-grid">
      <article v-for="item in media" :key="item.id" class="media-card">
        <div class="poster">
          <img v-if="poster(item) && !failedImages[poster(item)]" :src="poster(item)" :alt="title(item)" loading="lazy" @error="failedImages[poster(item)] = true" />
          <div v-else class="poster-empty">暂无海报</div>
          <span class="media-type">{{ item.result?.media_type === 'tv' ? '电视剧' : '电影' }}</span>
        </div>
        <div class="card-body">
          <h3 :title="title(item)">{{ title(item) }}</h3>
          <div class="original-title" :title="item.result?.original_title">{{ item.result?.original_title || '—' }}</div>
          <div class="media-facts"><span>{{ item.result?.year || '年份未知' }}</span><span>{{ source(item) }}</span></div>
          <div class="directory" :title="item.file_name">{{ item.file_name }}</div>
          <div class="card-actions">
            <n-button size="tiny" secondary type="primary" @click="$emit('identify', item)">重新识别</n-button>
            <n-button size="tiny" secondary @click="openManual(item)">手动识别</n-button>
            <n-button size="tiny" quaternary type="error" @click="$emit('remove', item)">删除</n-button>
          </div>
        </div>
      </article>
    </div>
    <n-modal v-model:show="manualShow" preset="card" title="手动识别" style="width: min(720px, 94vw)" :mask-closable="!saving" :closable="!saving">
      <p class="manual-path">{{ target?.file_name }}</p>
      <n-space vertical>
        <n-input v-model:value="keyword" placeholder="输入正确的电影或剧集名称" @keyup.enter="search" />
        <n-space>
          <n-select v-model:value="mediaType" :options="[{label:'电视剧',value:'tv'},{label:'电影',value:'movie'}]" style="width:120px" />
          <n-select v-model:value="metadataSource" :disabled="mediaType === 'tv'" :options="[{label:'TMDB',value:'tmdb'},{label:'MetaTube',value:'metatube'}]" style="width:130px" />
          <n-input-number v-model:value="year" placeholder="年份（可选）" :min="1800" :max="2200" style="width:150px" />
          <n-button type="primary" :loading="searching" :disabled="saving" @click="search">搜索</n-button>
        </n-space>
        <div v-for="candidate in candidates" :key="`${candidate.media_type}-${candidate.tmdb_id}-${candidate.metadata_id}`" class="manual-result">
          <img v-if="candidate.poster_path" :src="candidate.poster_path" alt="候选海报" />
          <div><strong>{{ candidate.title }}</strong><p>{{ candidate.original_title }} · {{ candidate.year }} · {{ candidate.media_type === 'tv' ? '电视剧' : '电影' }}</p></div>
          <n-button :disabled="saving" @click="saveManual(candidate)">选择并保存</n-button>
        </div>
        <p v-if="searched && !searching && !candidates.length">没有匹配结果，请调整名称、年份或媒体类型。</p>
      </n-space>
    </n-modal>
  </section>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { NButton, NModal, NInput, NInputNumber, NSelect, NSpace, useMessage } from 'naive-ui'
import { searchTmdb, manualIdentifyShareMedia } from '../utils/api/media'

defineProps({ media: { type: Array, default: () => [] } })
const emit = defineEmits(['identify', 'remove', 'saved'])
const message = useMessage()
const manualShow = ref(false), target = ref(null), keyword = ref(''), year = ref(null), mediaType = ref('tv'), metadataSource = ref('tmdb')
const candidates = ref([]), searching = ref(false), saving = ref(false), searched = ref(false)
let searchVersion = 0
const openManual = item => {
  searchVersion++
  searching.value = false
  target.value = item
  keyword.value = (item.file_name || '').split(/[\\/]/).pop().replace(/\s*[（(]\d{4}[）)].*$/, '')
  year.value = null
  mediaType.value = item.result?.media_type || 'tv'
  candidates.value = []
  searched.value = false
  manualShow.value = true
}
const search = async () => {
  if (!keyword.value.trim() || searching.value || saving.value) return
  const version = ++searchVersion
  searching.value = true
  candidates.value = []
  try {
    const response = await searchTmdb({keyword:keyword.value.trim(), year:year.value || undefined, type:mediaType.value, metadata_source:mediaType.value === 'tv' ? 'tmdb' : metadataSource.value})
    if (version === searchVersion) { candidates.value = response.data?.data?.data || []; searched.value = true }
  } catch (error) { message.error(error.response?.data?.message || '搜索失败，请重试') }
  finally { if (version === searchVersion) searching.value = false }
}
const saveManual = async candidate => {
  saving.value = true
  try {
    await manualIdentifyShareMedia(target.value.id, {...target.value, result:candidate})
    message.success('手动识别已保存')
    manualShow.value = false
    emit('saved')
  } catch (error) { message.error(error.response?.data?.message || '保存失败，请刷新后重试') }
  finally { saving.value = false }
}
const failedImages = reactive({})
const title = item => item.result?.title || item.result?.original_title || item.file_name
const poster = item => item.result?.poster_path || item.result?.candidates?.[0]?.poster_path
const source = item => {
  const value = item.result?.metadata_source || item.metadata_source
  return value === 'tmdb' ? 'TMDB' : value === 'metatube' ? 'MetaTube' : '自动识别'
}
</script>

<style scoped>
.share-gallery { padding: 20px; background: var(--n-color, #f8fafc); color: var(--n-text-color, #263248); }
.gallery-heading { display: flex; justify-content: space-between; align-items: center; gap: 12px; margin-bottom: 16px; font-size: 12px; }
.gallery-heading strong { font-size: 14px; }
.gallery-heading strong span { margin-left: 8px; color: #7265dc; }
.gallery-heading > span { opacity: .55; }
.gallery-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(170px, 1fr)); gap: 18px; max-height: 720px; overflow-y: auto; padding: 2px 6px 8px 2px; align-items: start; }
.media-card { min-width: 0; overflow: hidden; border: 1px solid rgba(128, 138, 158, .18); border-radius: 12px; background: var(--n-color, #fff); box-shadow: 0 3px 10px rgba(25, 40, 70, .04); }
.poster { position: relative; aspect-ratio: 2 / 3; background: rgba(128, 138, 158, .12); overflow: hidden; }
.poster img { width: 100%; height: 100%; object-fit: cover; display: block; }
.poster-empty { height: 100%; display: grid; place-items: center; opacity: .5; font-size: 13px; }
.media-type { position: absolute; bottom: 10px; left: 10px; padding: 3px 8px; border-radius: 6px; background: rgba(15, 23, 42, .75); color: white; font-size: 11px; }
.card-body { padding: 12px; }
.card-body h3 { margin: 0 0 5px; font-size: 14px; line-height: 20px; height: 40px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
.original-title, .directory { overflow: hidden; white-space: nowrap; text-overflow: ellipsis; font-size: 11px; line-height: 18px; opacity: .55; }
.media-facts { display: flex; justify-content: space-between; margin: 8px 0; font-size: 12px; opacity: .75; }
.card-actions { display: flex; flex-wrap: wrap; justify-content: space-between; align-items: center; gap: 6px; padding-top: 10px; margin-top: 10px; border-top: 1px solid rgba(128, 138, 158, .15); }
.manual-path { overflow-wrap: anywhere; opacity: .65; }
.manual-result { display: flex; align-items: center; gap: 12px; padding: 12px 0; border-bottom: 1px solid #ddd; }
.manual-result img { width: 48px; height: 72px; object-fit: cover; }
.manual-result > div { flex: 1; min-width: 0; }
.manual-result p { font-size: 12px; opacity: .65; }
@media (max-width: 600px) {
  .share-gallery { padding: 12px; }
  .gallery-grid { grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; max-height: 65vh; }
}
</style>
