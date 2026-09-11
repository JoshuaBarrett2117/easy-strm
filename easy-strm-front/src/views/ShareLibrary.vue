<template>
  <section class="library-page">
    <n-space justify="space-between" align="center"
      ><div>
        <h2>分享资源库</h2>
        <p>已识别作品 · {{ total }} 部</p>
      </div>
      <n-space><n-button @click="openExport">导出 STRM</n-button><n-button :loading="enriching" @click="enrich">补全历史元数据</n-button></n-space></n-space
    >
    <n-card class="filters">
      <n-grid :cols="'1 s:2 m:4'" responsive="screen" :x-gap="12" :y-gap="12">
        <n-gi><n-input v-model:value="form.keyword" placeholder="名称搜索" clearable @keyup.enter="apply" /></n-gi>
        <n-gi><n-input-number v-model:value="form.tmdb_id" placeholder="TMDB ID" :min="1" :precision="0" /></n-gi>
        <n-gi
          ><n-select v-model:value="form.media_type" :options="mediaTypes" placeholder="电影 / 电视剧" clearable
        /></n-gi>
        <n-gi
          ><n-select v-model:value="form.genres" multiple :options="genreOptions" placeholder="题材类型" clearable
        /></n-gi>
        <n-gi
          ><n-input-number v-model:value="form.year_min" placeholder="起始年份" :min="1" :max="9999" :precision="0"
        /></n-gi>
        <n-gi
          ><n-input-number v-model:value="form.year_max" placeholder="截止年份" :min="1" :max="9999" :precision="0"
        /></n-gi>
        <n-gi
          ><n-input-number v-model:value="form.rating_min" placeholder="最低评分" :min="0" :max="10" :step="0.5"
        /></n-gi>
        <n-gi
          ><n-input-number v-model:value="form.rating_max" placeholder="最高评分" :min="0" :max="10" :step="0.5"
        /></n-gi>
        <n-gi
          ><n-select
            v-model:value="form.countries"
            multiple
            :options="countryOptions"
            placeholder="国家或地区"
            clearable
        /></n-gi>
        <n-gi><n-select v-model:value="form.sort" :options="sortOptions" /></n-gi>
        <n-gi
          ><n-select
            v-model:value="form.direction"
            :options="[
              { label: '降序', value: 'desc' },
              { label: '升序', value: 'asc' }
            ]"
        /></n-gi>
        <n-gi><n-checkbox v-model:checked="form.available">仅有有效分享</n-checkbox></n-gi>
      </n-grid>
      <n-space style="margin-top: 16px"
        ><n-button type="primary" @click="apply">筛选</n-button><n-button @click="reset">重置</n-button></n-space
      >
      <small>国家或地区：电影按制作国家，电视剧按来源国家；有效分享指尚未标记取消。</small>
    </n-card>
    <n-alert v-if="error" type="error" style="margin-top: 16px"
      >{{ error }} <n-button text @click="load">重试</n-button></n-alert
    >
    <n-spin :show="loading"
      ><div class="poster-wall">
        <button v-for="item in items" :key="item.work_key" class="poster-card" @click="openSources(item)">
          <div class="poster-image">
            <img
              v-if="poster(item) && !failed[poster(item)]"
              :src="poster(item)"
              :alt="item.title"
              loading="lazy"
              @error="failed[poster(item)] = true"
            /><span v-else>暂无海报</span>
          </div>
          <div class="poster-info">
            <strong>{{ item.title || '未知名称' }}</strong>
            <p>
              {{ item.year || '年份未知' }} · {{ item.media_type === 'tv' ? '电视剧' : '电影' }} ·
              {{ item.rating == null ? '暂无评分' : Number(item.rating).toFixed(1) + '分' }}
            </p>
            <small>{{ item.source_count }} 个分享来源<span v-if="!item.available"> · 全部已失效</span></small>
          </div>
        </button>
      </div>
      <n-empty v-if="!loading && !items.length && !error" description="没有匹配的作品"
    /></n-spin>
    <n-pagination
      v-model:page="page"
      v-model:page-size="pageSize"
      :page-sizes="[24, 48, 96]"
      show-size-picker
      :item-count="total"
      :disabled="loading"
    />
    <n-modal v-model:show="showSources" preset="card" :title="selected?.title" style="width: min(850px, 94vw)">
      <n-spin :show="sourceLoading"
        ><n-alert v-if="sourceError" type="error"
          >{{ sourceError }} <n-button text @click="loadSources">重试</n-button></n-alert
        >
        <article v-for="row in sources" :key="row.id" class="source-row">
          <strong>{{ row.name || '未命名分享' }}</strong
          ><n-tag :type="row.share_cancelled ? 'error' : 'success'">{{
            row.share_cancelled ? '分享已取消' : '未标记失效'
          }}</n-tag>
          <p>{{ row.file_name }}</p>
          <a :href="safeURL(row.url)" target="_blank" rel="noreferrer">{{ row.url }}</a>
          <p v-if="row.password">提取码：{{ row.password }}</p>
          <n-button
            size="small"
            @click="router.push({ path: '/dashboard/share-records', query: { share_id: row.share_id } })"
            >进入分享管理</n-button
          >
        </article>
        <n-empty v-if="!sourceLoading && !sources.length && !sourceError" description="暂无来源" /> </n-spin
      ><n-pagination v-model:page="sourcePage" :page-size="20" :item-count="sourceTotal" :disabled="sourceLoading" />
    </n-modal>
    <ShareStrmDialog v-model:show="showExport" :filters="exportFilters" :count="total" />
  </section>
</template>
<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import ShareStrmDialog from '../components/ShareStrmDialog.vue'
import {
  NAlert,
  NButton,
  NCard,
  NCheckbox,
  NEmpty,
  NGi,
  NGrid,
  NInput,
  NInputNumber,
  NModal,
  NPagination,
  NSelect,
  NSpace,
  NSpin,
  NTag,
  useMessage
} from 'naive-ui'
import { getShareLibrary, getLibraryOptions, getLibrarySources, enrichLibrary } from '../utils/api/share-library'
const router = useRouter(),
  message = useMessage()
const showExport = ref(false), exportFilters = ref({})
const openExport = () => { exportFilters.value = { ...active }; showExport.value = true }
const defaults = () => ({
  keyword: '',
  tmdb_id: null,
  media_type: null,
  year_min: null,
  year_max: null,
  rating_min: null,
  rating_max: null,
  genres: [],
  countries: [],
  available: false,
  sort: 'created',
  direction: 'desc'
})
const form = reactive(defaults()),
  items = ref([]),
  total = ref(0),
  page = ref(1),
  pageSize = ref(24),
  loading = ref(false),
  error = ref(''),
  failed = reactive({}),
  enriching = ref(false)
const options = ref({ genres: [], countries: [] }),
  mediaTypes = [
    { label: '电影', value: 'movie' },
    { label: '电视剧', value: 'tv' }
  ],
  sortOptions = [
    { label: '最近收录', value: 'created' },
    { label: '年份', value: 'year' },
    { label: '评分', value: 'rating' },
    { label: '名称', value: 'title' }
  ]
const genreNames = {
  28: '动作',
  12: '冒险',
  16: '动画',
  35: '喜剧',
  80: '犯罪',
  99: '纪录',
  18: '剧情',
  10751: '家庭',
  14: '奇幻',
  36: '历史',
  27: '恐怖',
  10402: '音乐',
  9648: '悬疑',
  10749: '爱情',
  878: '科幻',
  10770: '电视电影',
  53: '惊悚',
  10752: '战争',
  37: '西部',
  10759: '动作冒险',
  10762: '儿童',
  10763: '新闻',
  10764: '真人秀',
  10765: '科幻奇幻',
  10766: '肥皂剧',
  10767: '脱口秀',
  10768: '战争政治'
}
const regions = new Intl.DisplayNames(['zh-CN'], { type: 'region' })
const genreOptions = computed(() =>
  options.value.genres.map((value) => ({ value, label: genreNames[value] || String(value) }))
)
const countryOptions = computed(() =>
  options.value.countries.map((value) => ({ value, label: regions.of(value) || value }))
)
let revision = 0,
  sourceRevision = 0,
  disposed = false,
  active = {}
const params = () =>
  Object.fromEntries(
    Object.entries(form)
      .filter(([, v]) => v !== null && v !== '')
      .map(([k, v]) => [k, Array.isArray(v) ? v.join(',') : v])
  )
const load = async () => {
  const v = ++revision
  loading.value = true
  error.value = ''
  try {
    const r = await getShareLibrary({ ...active, page: page.value, page_size: pageSize.value })
    if (disposed || v !== revision) return
    items.value = r.data.data.data
    total.value = r.data.data.total
    if (page.value > Math.max(1, Math.ceil(total.value / pageSize.value))) page.value = 1
  } catch (e) {
    if (v === revision) error.value = e.response?.data?.error || e.response?.data?.message || e.message
  } finally {
    if (v === revision) loading.value = false
  }
}
const apply = () => {
  active = params()
  if (page.value !== 1) page.value = 1
  else load()
}
const reset = () => {
  Object.assign(form, defaults())
  apply()
}
watch(page, load)
watch(pageSize, () => {
  if (page.value !== 1) page.value = 1
  else load()
})
const poster = (item) =>
  item.poster_path?.startsWith('/') ? 'https://image.tmdb.org/t/p/w500' + item.poster_path : item.poster_path
const safeURL = (url) => (/^https?:\/\//i.test(url) ? url : undefined)
const showSources = ref(false),
  selected = ref(null),
  sources = ref([]),
  sourcePage = ref(1),
  sourceTotal = ref(0),
  sourceLoading = ref(false),
  sourceError = ref('')
const loadSources = async () => {
  if (!showSources.value || !selected.value) return
  const v = ++sourceRevision
  sourceLoading.value = true
  sourceError.value = ''
  try {
    const r = await getLibrarySources({ work_key: selected.value.work_key, page: sourcePage.value, page_size: 20 })
    if (disposed || v !== sourceRevision) return
    sources.value = r.data.data.data
    sourceTotal.value = r.data.data.total
  } catch (e) {
    if (v === sourceRevision) sourceError.value = e.message
  } finally {
    if (v === sourceRevision) sourceLoading.value = false
  }
}
const openSources = (item) => {
  selected.value = item
  sources.value = []
  sourceTotal.value = 0
  showSources.value = true
  if (sourcePage.value !== 1) sourcePage.value = 1
  else loadSources()
}
watch(sourcePage, loadSources)
watch(showSources, (v) => {
  if (!v) sourceRevision++
})
const enrich = async () => {
  enriching.value = true
  try {
    const r = await enrichLibrary()
    message.success('补全任务已创建：' + r.data.data.task_id + '，可在任务中心查看和取消')
  } catch (e) {
    message.error(e.response?.data?.error || e.response?.data?.message || e.message)
  } finally {
    enriching.value = false
  }
}
onMounted(async () => {
  apply()
  try {
    const r = await getLibraryOptions()
    if (!disposed) options.value = r.data.data
  } catch (e) {
    message.error('筛选选项加载失败：' + e.message)
  }
})
onBeforeUnmount(() => {
  disposed = true
  revision++
  sourceRevision++
})
</script>
<style scoped>
.library-page {
  padding: 24px;
}
.filters {
  margin-bottom: 20px;
}
.filters small {
  display: block;
  margin-top: 12px;
  color: var(--text-color-3, #888);
}
.poster-wall {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(170px, 1fr));
  gap: 20px;
  margin: 20px 0;
}
.poster-card {
  padding: 0;
  text-align: left;
  border: 1px solid #8883;
  border-radius: 12px;
  overflow: hidden;
  background: transparent;
  color: inherit;
  cursor: pointer;
}
.poster-card:hover {
  border-color: #18a058;
}
.poster-image {
  aspect-ratio: 2/3;
  background: #8881;
  display: flex;
  align-items: center;
  justify-content: center;
}
.poster-image img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.poster-info {
  padding: 12px;
}
.poster-info strong {
  display: block;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.poster-info p {
  font-size: 12px;
}
.source-row {
  padding: 16px 0;
  border-bottom: 1px solid #8883;
  overflow-wrap: anywhere;
}
.source-row .n-tag {
  margin-left: 12px;
}
@media (max-width: 600px) {
  .library-page {
    padding: 12px;
  }
  .poster-wall {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
}
</style>
