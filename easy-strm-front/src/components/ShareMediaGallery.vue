<template>
  <section class="share-gallery">
    <div class="gallery-heading"><strong>已识别媒体 <span>{{ total }}</span></strong><n-button size="small" @click="openFiles">查看全部文件</n-button></div>
    <p v-if="loading">正在加载…</p>
    <n-button v-if="loadError" @click="loadPage">加载失败，点击重试</n-button>
    <div class="gallery-grid">
      <article v-for="item in visibleMedia" :key="item.media_id || item.id" class="media-card">
        <div class="poster">
          <img v-if="poster(item) && !failedImages[poster(item)]" :src="poster(item)" :alt="title(item)" loading="lazy" @error="failedImages[poster(item)] = true" />
          <div v-else class="poster-empty">暂无海报</div>
          <span class="media-type">{{ item.result?.media_type === 'tv' ? '电视剧' : '电影' }}</span>
        </div>
        <div class="card-body">
          <h3 :title="title(item)">{{ title(item) }}</h3>
          <div class="original-title" :title="item.result?.original_title">{{ item.result?.original_title || '—' }}</div>
          <div class="media-facts"><span>{{ item.result?.year || '年份未知' }}</span><span>{{ source(item) }}</span></div>
          <div class="directory">关联 {{ item.file_count || 1 }} 个有效文件</div>
          <div class="card-actions">
            <n-button size="tiny" secondary type="primary" @click="$emit('identify', item)">重新识别</n-button>
            <n-button size="tiny" secondary @click="openManual(item)">手动识别</n-button>
          </div>
        </div>
      </article>
    </div>
    <n-space align="center" style="margin-top: 16px">
      <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" :disabled="loading" />
      <span>每页条数</span>
      <n-input-number v-model:value="pageSizeInput" aria-label="每页条数" :min="1" :max="100" :precision="0" style="width: 120px" @keyup.enter="changePageSize" />
      <n-button :disabled="loading" @click="changePageSize">应用</n-button>
    </n-space>
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
        <n-space v-if="mediaType === 'tv'">
          <n-input-number v-model:value="season" placeholder="季" :min="0" :precision="0" style="width:120px" />
          <n-input v-model:value="episodeText" placeholder="集号，多个用逗号分隔，如 1,2" style="width:280px" />
        </n-space>
        <div v-for="candidate in candidates" :key="`${candidate.media_type}-${candidate.tmdb_id}-${candidate.metadata_id}`" class="manual-result">
          <img v-if="candidate.poster_path" :src="candidate.poster_path" alt="候选海报" />
          <div><strong>{{ candidate.title }}</strong><p>{{ candidate.original_title }} · {{ candidate.year }} · {{ candidate.media_type === 'tv' ? '电视剧' : '电影' }}</p></div>
          <n-button :disabled="saving" @click="saveManual(candidate)">选择并保存</n-button>
        </div>
        <p v-if="searched && !searching && !candidates.length">没有匹配结果，请调整名称、年份或媒体类型。</p>
      </n-space>
    </n-modal>
    <n-modal v-model:show="filesShow" preset="card" title="分享文件" style="width: min(1000px, 96vw)">
      <n-data-table :columns="fileColumns" :data="files" :loading="filesLoading" :row-key="row => row.id" />
      <n-pagination v-model:page="filePage" :page-size="20" :item-count="fileTotal" style="margin-top:16px" />
    </n-modal>
  </section>
</template>

<script setup>
import { h, reactive, ref, watch, onBeforeUnmount } from 'vue'
import { NPagination, NButton, NModal, NInput, NInputNumber, NSelect, NSpace, NDataTable, NTag, useMessage } from 'naive-ui'
import { getShareMedia, getShareFiles, searchTmdb, manualIdentifyShareMedia } from '../utils/api/media'

const props = defineProps({ shareId: { type: Number, required: true }, revision: Number })
const showDuplicates = ref(false), page = ref(1), total = ref(0)
const duplicateCount = ref(0), visibleMedia = ref([]), loading = ref(false), loadError = ref(false)
const filesShow = ref(false), filesLoading = ref(false), files = ref([]), filePage = ref(1), fileTotal = ref(0)
const pageSizeStorageKey = 'share:gallery:page_size'
const readPageSize = () => {
  try {
    const size = Number(localStorage.getItem(pageSizeStorageKey))
    return Number.isInteger(size) && size >= 1 && size <= 100 ? size : 10
  } catch { return 10 }
}
const pageSize = ref(readPageSize()), pageSizeInput = ref(pageSize.value)
let disposed = false
let requestVersion = 0
const loadPage = async () => {
  if (disposed) return
  const version = ++requestVersion
  loading.value = true
  loadError.value = false
  visibleMedia.value = []
  try {
    const response = await getShareMedia(props.shareId, {page: page.value, page_size: pageSize.value, show_duplicates: showDuplicates.value})
    if (version !== requestVersion) return
    const result = response.data.data
    total.value = result.total
    duplicateCount.value = result.duplicate_count
    const lastPage = Math.max(1, Math.ceil(result.total / pageSize.value))
    if (page.value > lastPage) { page.value = lastPage; return }
    visibleMedia.value = result.data
  } catch (error) {
    if (version === requestVersion) loadError.value = true
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
watch([() => props.shareId, showDuplicates], () => {
  if (page.value !== 1) page.value = 1
  else loadPage()
})
watch([page, () => props.revision], loadPage, {immediate: true})
const changePageSize = () => {
  const size = pageSizeInput.value
  if (!Number.isInteger(size) || size < 1 || size > 100) {
    message.error('每页条数请输入1至100的整数')
    return
  }
  try { localStorage.setItem(pageSizeStorageKey, String(size)) }
  catch { message.warning('浏览器缓存不可用，本次设置仅在当前页面生效') }
  if (size === pageSize.value) return
  pageSize.value = size
  if (page.value !== 1) page.value = 1
  else loadPage()
}
onBeforeUnmount(() => { disposed = true; requestVersion++; searchVersion++ })
const emit = defineEmits(['identify', 'saved'])
const message = useMessage()
const manualShow = ref(false), target = ref(null), keyword = ref(''), year = ref(null), mediaType = ref('tv'), metadataSource = ref('tmdb'), season = ref(1), episodeText = ref('')
const candidates = ref([]), searching = ref(false), saving = ref(false), searched = ref(false)
let searchVersion = 0
const openManual = item => {
  searchVersion++
  searching.value = false
  target.value = item
  keyword.value = (item.file_name || '').split(/[\\/]/).pop().replace(/\s*[（(]\d{4}[）)].*$/, '')
  year.value = null
  mediaType.value = item.result?.media_type || 'tv'
  season.value = item.episodes?.[0]?.season_number ?? item.result?.season_number ?? 1
  episodeText.value = (item.episodes || []).map(value => value.episode_number).join(',') || String(item.result?.episode_number || '')
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
    const episodeNumbers = mediaType.value === 'tv' ? [...new Set(episodeText.value.split(/[,，\s]+/).map(Number).filter(value => Number.isInteger(value) && value > 0))] : []
    if (mediaType.value === 'tv' && (!Number.isInteger(season.value) || season.value < 0 || !episodeNumbers.length)) {
      message.error('电视剧必须填写有效的季号和集号')
      return
    }
    await manualIdentifyShareMedia(target.value.id, {...target.value, result:candidate, episodes:episodeNumbers.map(value => ({season_number:season.value, episode_number:value}))})
    message.success('手动识别已保存')
    manualShow.value = false
    emit('saved')
  } catch (error) { message.error(error.response?.data?.message || '保存失败，请刷新后重试') }
  finally { saving.value = false }
}
const loadFiles = async () => {
  filesLoading.value = true
  try {
    const response = await getShareFiles(props.shareId, {page:filePage.value, page_size:20})
    const result = response.data?.data || {}
    files.value = result.data || []
    fileTotal.value = result.total || 0
  } finally { filesLoading.value = false }
}
const openFiles = async () => { filesShow.value = true; filePage.value = 1; await loadFiles() }
watch(filePage, () => { if (filesShow.value) loadFiles() })
const fileColumns = [
  {title:'路径', key:'file_name', ellipsis:{tooltip:true}},
  {title:'状态', key:'status', render:row => h(NTag, {size:'small', type:row.status==='identified'?'success':row.status==='failed'?'error':'default'}, {default:() => row.available===false?'失效':row.status})},
  {title:'季集', render:row => (row.episodes || []).map(value => `S${String(value.season_number).padStart(2,'0')}E${String(value.episode_number).padStart(2,'0')}`).join('、') || '—'},
  {title:'错误', key:'error', ellipsis:{tooltip:true}},
  {title:'操作', render:row => h(NSpace, {}, {default:() => [h(NButton,{size:'tiny',onClick:()=>emit('identify',row)},{default:()=>'自动识别'}),h(NButton,{size:'tiny',onClick:()=>openManual(row)},{default:()=>'手动识别'})]})}
]
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
