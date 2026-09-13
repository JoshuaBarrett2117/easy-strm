<template>
  <section class="library-page">
    <n-space justify="space-between" align="center"
      ><div>
        <h2>分享资源库</h2>
        <p>已识别作品 · {{ total }} 部</p>
      </div>
      <n-space
        ><n-button @click="openExport">导出 STRM</n-button
        ><n-button :loading="enriching" @click="enrich">补全历史元数据</n-button></n-space
      ></n-space
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
              { label: '升序', value: 'asc' },
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
    <n-modal
      v-model:show="showSources"
      preset="card"
      :title="undefined"
      class="media-detail-modal"
      style="width: min(1120px, 94vw)"
    >
      <template v-if="selected?.media_type === 'tv'">
        <div class="tv-detail-shell">
          <header
            class="tv-hero"
            :style="selected && poster(selected) ? { '--tv-backdrop': `url(${poster(selected)})` } : undefined"
          >
            <button class="tv-close" type="button" aria-label="关闭" @click="showSources = false">×</button>
            <div class="tv-hero-content">
              <div class="tv-kicker">
                电视剧<span v-if="selected?.tmdb_id"> · TMDB {{ selected.tmdb_id }}</span>
              </div>
              <h2>{{ tvDetail?.title || selected?.title || '未知剧集' }}</h2>
              <p class="tv-meta">
                {{ selected?.year || '年份未知' }} · {{ tvDetail?.seasons?.length || 0 }} 季 · {{ tvEpisodeTotal }} 集 ·
                {{ selected?.rating == null ? '暂无评分' : Number(selected.rating).toFixed(1) + ' 分' }}
              </p>
              <p v-if="tvDetail?.overview" class="tv-description">{{ tvDetail.overview }}</p>
            </div>
            <div class="tv-stat-list">
              <span
                ><b>{{ tvDetail?.seasons?.length || 0 }}</b
                >季</span
              ><span
                ><b>{{ tvEpisodeTotal }}</b
                >集</span
              ><span
                ><b>{{ tvMatchedEpisodeTotal }}</b
                >已关联</span
              ><span
                ><b>{{ tvCoverage }}%</b>覆盖率</span
              >
            </div>
          </header>
          <n-spin :show="tvLoading">
            <n-alert v-if="tvError" type="error"
              >{{ tvError }} <n-button text @click="loadTVDetail">重试</n-button></n-alert
            >
            <template v-if="tvDetail">
              <n-alert v-if="tvDetail.warning" type="warning" class="tv-warning">{{ tvDetail.warning }}</n-alert>
              <div class="tv-detail-body">
                <aside class="season-sidebar">
                  <div class="season-sidebar-title">季列表</div>
                  <button
                    v-for="season in tvDetail.seasons"
                    :key="season.season_number"
                    type="button"
                    class="season-nav-item"
                    :class="{ active: activeSeason === season.season_number }"
                    @click="selectSeason(season.season_number)"
                  >
                    <strong>{{ season.name || seasonLabel(season.season_number) }}</strong>
                    <small
                      >{{ season.matched_episode_count }} / {{ season.episode_count }} 集 ·
                      {{ season.file_count }} 文件</small
                    >
                    <i
                      ><em
                        :style="{
                          width: `${season.episode_count ? Math.min(100, (season.matched_episode_count / season.episode_count) * 100) : 0}%`,
                        }"
                    /></i>
                  </button>
                </aside>
                <section class="season-content">
                  <n-alert v-if="activeSeason != null && tvSeasonErrors[activeSeason]" type="error"
                    >{{ tvSeasonErrors[activeSeason] }}
                    <n-button text @click="loadTVSeason(activeSeason, true)">重试</n-button></n-alert
                  >
                  <n-spin :show="activeSeason != null && Boolean(tvSeasonLoading[activeSeason])">
                    <template v-if="activeSeason != null && tvSeasons[activeSeason]">
                      <div class="season-content-header">
                        <div>
                          <h3>{{ tvSeasons[activeSeason].name || seasonLabel(activeSeason) }}</h3>
                          <p>
                            {{ tvSeasons[activeSeason].air_date || '播出日期未知' }} · 已关联
                            {{ seasonStat(activeSeason).matched_episode_count }} 集 /
                            {{ seasonStat(activeSeason).file_count }} 个文件
                          </p>
                        </div>
                        <div class="season-filters">
                          <span class="selected">全部 {{ tvSeasons[activeSeason].episodes.length }}</span
                          ><span>有资源 {{ seasonMatched(activeSeason) }}</span
                          ><span
                            >缺集
                            {{
                              Math.max(0, tvSeasons[activeSeason].episodes.length - seasonMatched(activeSeason))
                            }}</span
                          >
                        </div>
                      </div>
                      <n-alert v-if="tvSeasons[activeSeason].warning" type="warning" class="tv-warning">{{
                        tvSeasons[activeSeason].warning
                      }}</n-alert>
                      <article
                        v-for="episode in tvSeasons[activeSeason].episodes"
                        :key="episode.episode_number"
                        class="episode-row"
                      >
                        <header class="episode-heading">
                          <div class="episode-visual">
                            <img
                              v-if="episodeImage(episode) && !failed[episodeImage(episode)]"
                              :src="episodeImage(episode)"
                              :alt="episode.name || `第 ${episode.episode_number} 集`"
                              loading="lazy"
                              @error="failed[episodeImage(episode)] = true"
                            />
                            <span v-else>暂无剧照</span>
                            <b>E{{ padEpisode(episode.episode_number) }}</b>
                          </div>
                          <div class="episode-main">
                            <strong>{{ episode.name || '未命名' }}</strong>
                            <p v-if="episode.overview" class="episode-overview">{{ episode.overview }}</p>
                          </div>
                          <small
                            >{{ episode.air_date || '播出日期未知'
                            }}<template v-if="episode.runtime"> · {{ episode.runtime }} 分钟</template
                            ><n-tag :type="episode.files.length ? 'success' : 'default'">{{
                              episode.files.length ? `${episode.files.length} 个关联文件` : '暂无关联文件'
                            }}</n-tag></small
                          >
                        </header>
                        <n-empty v-if="!episode.files.length" size="small" description="暂无关联分享文件" />
                        <div v-else class="episode-file-grid">
                          <div v-for="file in episode.files" :key="file.id" class="episode-file">
                            <div>
                              <strong>🔗 {{ file.name || '未命名分享' }}</strong
                              ><n-tag :type="fileStatus(file).type">{{ fileStatus(file).label }}</n-tag>
                            </div>
                            <p>
                              {{ file.file_name }}<span v-if="file.file_size"> · {{ formatSize(file.file_size) }}</span>
                            </p>
                            <div class="episode-file-actions">
                              <span>{{ file.password ? `提取码 ${file.password}` : '无提取码' }}</span
                              ><n-button
                                text
                                size="small"
                                @click="
                                  router.push({ path: '/dashboard/share-records', query: { share_id: file.share_id } })
                                "
                                >进入分享管理 →</n-button
                              >
                            </div>
                          </div>
                        </div>
                      </article>
                      <n-empty v-if="!tvSeasons[activeSeason].episodes.length" description="该季暂无分集信息" />
                    </template>
                  </n-spin>
                </section>
              </div>
            </template>
          </n-spin>
        </div>
      </template>
      <template v-else>
        <div class="movie-detail-shell">
          <header
            class="tv-hero movie-hero"
            :style="selected && poster(selected) ? { '--tv-backdrop': `url(${poster(selected)})` } : undefined"
          >
            <button class="tv-close" type="button" aria-label="关闭" @click="showSources = false">×</button>
            <div class="tv-hero-content">
              <div class="tv-kicker">
                电影<span v-if="selected?.tmdb_id"> · TMDB {{ selected.tmdb_id }}</span>
              </div>
              <h2>{{ selected?.title || '未知电影' }}</h2>
              <p class="tv-meta">
                {{ selected?.year || '年份未知' }} ·
                {{ selected?.rating == null ? '暂无评分' : Number(selected.rating).toFixed(1) + ' 分' }}
              </p>
            </div>
            <div class="tv-stat-list">
              <span
                ><b>{{ selected?.source_count || 0 }}</b
                >分享来源</span
              >
              <span
                ><b>{{ selected?.file_count || sourceTotal || 0 }}</b
                >文件</span
              >
              <span
                ><b>{{ selected?.available ? '可用' : '失效' }}</b
                >资源状态</span
              >
            </div>
          </header>
          <section class="movie-source-content">
            <div class="movie-source-header">
              <div>
                <h3>分享文件</h3>
                <p>共 {{ sourceTotal }} 个可以关联的分享文件</p>
              </div>
              <n-tag :type="selected?.available ? 'success' : 'error'">{{
                selected?.available ? '存在有效分享' : '全部分享已失效'
              }}</n-tag>
            </div>
            <n-spin :show="sourceLoading">
              <n-alert v-if="sourceError" type="error"
                >{{ sourceError }} <n-button text @click="loadSources">重试</n-button></n-alert
              >
              <div class="movie-source-grid">
                <article v-for="row in sources" :key="row.id" class="movie-source-card">
                  <div class="episode-file-top">
                    <strong>🔗 {{ row.name || '未命名分享' }}</strong
                    ><n-tag :type="fileStatus(row).type">{{ fileStatus(row).label }}</n-tag>
                  </div>
                  <p class="movie-file-name">
                    {{ row.file_name }}<span v-if="row.file_size"> · {{ formatSize(row.file_size) }}</span>
                  </p>
                  <a :href="safeURL(row.url)" target="_blank" rel="noreferrer">{{ row.url }}</a>
                  <div class="episode-file-actions">
                    <span>{{ row.password ? `提取码 ${row.password}` : '无提取码' }}</span
                    ><n-button
                      text
                      size="small"
                      @click="router.push({ path: '/dashboard/share-records', query: { share_id: row.share_id } })"
                      >进入分享管理 →</n-button
                    >
                  </div>
                </article>
              </div>
              <n-empty v-if="!sourceLoading && !sources.length && !sourceError" description="暂无来源" />
            </n-spin>
            <n-pagination
              v-model:page="sourcePage"
              :page-size="20"
              :item-count="sourceTotal"
              :disabled="sourceLoading"
            />
          </section>
        </div>
      </template>
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
  useMessage,
} from 'naive-ui'
import {
  getShareLibrary,
  getLibraryOptions,
  getLibrarySources,
  getLibraryTVDetail,
  getLibraryTVSeason,
  enrichLibrary,
} from '../utils/api/share-library'
const router = useRouter(),
  message = useMessage()
const showExport = ref(false),
  exportFilters = ref({})
const openExport = () => {
  exportFilters.value = { ...active }
  showExport.value = true
}
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
  direction: 'desc',
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
    { label: '电视剧', value: 'tv' },
  ],
  sortOptions = [
    { label: '最近收录', value: 'created' },
    { label: '年份', value: 'year' },
    { label: '评分', value: 'rating' },
    { label: '名称', value: 'title' },
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
  10768: '战争政治',
}
const regions = new Intl.DisplayNames(['zh-CN'], { type: 'region' })
const genreOptions = computed(() =>
  options.value.genres.map((value) => ({
    value,
    label: genreNames[value] || String(value),
  })),
)
const countryOptions = computed(() =>
  options.value.countries.map((value) => ({
    value,
    label: regions.of(value) || value,
  })),
)
let revision = 0,
  sourceRevision = 0,
  disposed = false,
  active = {}
const params = () =>
  Object.fromEntries(
    Object.entries(form)
      .filter(([, v]) => v !== null && v !== '')
      .map(([k, v]) => [k, Array.isArray(v) ? v.join(',') : v]),
  )
const load = async () => {
  const v = ++revision
  loading.value = true
  error.value = ''
  try {
    const r = await getShareLibrary({
      ...active,
      page: page.value,
      page_size: pageSize.value,
    })
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
const tmdbImage = (path, size = 'w300') => (path?.startsWith('/') ? `https://image.tmdb.org/t/p/${size}${path}` : path)
const episodeImage = (episode) => tmdbImage(episode?.still_path, 'w300')
const safeURL = (url) => (/^https?:\/\//i.test(url) ? url : undefined)
const seasonLabel = (season) => (season === 0 ? '特别篇' : `第 ${season} 季`)
const padEpisode = (episode) => String(episode).padStart(2, '0')
const formatSize = (size) => {
  const value = Number(size)
  if (!Number.isFinite(value) || value <= 0) return ''
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  const index = Math.min(Math.floor(Math.log(value) / Math.log(1024)), units.length - 1)
  return `${(value / 1024 ** index).toFixed(index > 1 ? 2 : 0)} ${units[index]}`
}
const fileStatus = (file) => {
  if (file.share_cancelled) return { type: 'error', label: '分享已取消' }
  if (!file.available) return { type: 'warning', label: '文件已失效' }
  return { type: 'success', label: '可用' }
}
const showSources = ref(false),
  selected = ref(null),
  sources = ref([]),
  sourcePage = ref(1),
  sourceTotal = ref(0),
  sourceLoading = ref(false),
  sourceError = ref('')
const tvDetail = ref(null),
  tvLoading = ref(false),
  tvError = ref(''),
  activeSeason = ref(null),
  tvSeasons = reactive({}),
  tvSeasonLoading = reactive({}),
  tvSeasonErrors = reactive({})
const tvEpisodeTotal = computed(() =>
  (tvDetail.value?.seasons || []).reduce((sum, season) => sum + Number(season.episode_count || 0), 0),
)
const tvMatchedEpisodeTotal = computed(() =>
  (tvDetail.value?.seasons || []).reduce((sum, season) => sum + Number(season.matched_episode_count || 0), 0),
)
const tvCoverage = computed(() =>
  tvEpisodeTotal.value ? Math.round((tvMatchedEpisodeTotal.value / tvEpisodeTotal.value) * 100) : 0,
)
const seasonStat = (season) => tvDetail.value?.seasons?.find((item) => item.season_number === season) || {}
const seasonMatched = (season) => (tvSeasons[season]?.episodes || []).filter((episode) => episode.files.length).length
let tvRevision = 0
const loadSources = async () => {
  if (!showSources.value || !selected.value || selected.value.media_type === 'tv') return
  const v = ++sourceRevision
  sourceLoading.value = true
  sourceError.value = ''
  try {
    const r = await getLibrarySources({
      work_key: selected.value.work_key,
      page: sourcePage.value,
      page_size: 20,
    })
    if (disposed || v !== sourceRevision) return
    sources.value = r.data.data.data
    sourceTotal.value = r.data.data.total
  } catch (e) {
    if (v === sourceRevision) sourceError.value = e.message
  } finally {
    if (v === sourceRevision) sourceLoading.value = false
  }
}
const loadTVDetail = async () => {
  if (!showSources.value || selected.value?.media_type !== 'tv') return
  const v = ++tvRevision
  tvLoading.value = true
  tvError.value = ''
  try {
    const r = await getLibraryTVDetail({ work_key: selected.value.work_key })
    if (disposed || v !== tvRevision) return
    tvDetail.value = r.data.data
    const seasons = tvDetail.value?.seasons || []
    if (seasons.length) {
      const preferred = [...seasons].reverse().find((season) => season.matched_episode_count > 0) || seasons[0]
      selectSeason(preferred.season_number)
    }
  } catch (e) {
    if (v === tvRevision) tvError.value = e.response?.data?.error || e.response?.data?.message || e.message
  } finally {
    if (v === tvRevision) tvLoading.value = false
  }
}
const loadTVSeason = async (season, force = false) => {
  if (
    !showSources.value ||
    selected.value?.media_type !== 'tv' ||
    ((tvSeasonLoading[season] || tvSeasons[season]) && !force)
  )
    return
  const v = tvRevision
  tvSeasonLoading[season] = true
  tvSeasonErrors[season] = ''
  try {
    const r = await getLibraryTVSeason({
      work_key: selected.value.work_key,
      season_number: season,
    })
    if (disposed || v !== tvRevision) return
    tvSeasons[season] = r.data.data
  } catch (e) {
    if (v === tvRevision) tvSeasonErrors[season] = e.response?.data?.error || e.response?.data?.message || e.message
  } finally {
    if (v === tvRevision) tvSeasonLoading[season] = false
  }
}
const selectSeason = (season) => {
  activeSeason.value = Number(season)
  loadTVSeason(activeSeason.value)
}
const openSources = (item) => {
  tvRevision++
  selected.value = item
  sources.value = []
  sourceTotal.value = 0
  tvDetail.value = null
  tvError.value = ''
  tvLoading.value = false
  activeSeason.value = null
  for (const key of Object.keys(tvSeasons)) delete tvSeasons[key]
  for (const key of Object.keys(tvSeasonLoading)) delete tvSeasonLoading[key]
  for (const key of Object.keys(tvSeasonErrors)) delete tvSeasonErrors[key]
  showSources.value = true
  if (item.media_type === 'tv') loadTVDetail()
  else if (sourcePage.value !== 1) sourcePage.value = 1
  else loadSources()
}
watch(sourcePage, loadSources)
watch(showSources, (v) => {
  if (!v) {
    sourceRevision++
    tvRevision++
  }
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
  tvRevision++
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
.tv-overview,
.episode-overview {
  color: var(--text-color-2);
  line-height: 1.65;
}
.tv-warning {
  margin-bottom: 12px;
}
.season-heading,
.episode-heading {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 12px;
  min-width: 0;
}
.season-heading small,
.episode-heading small {
  flex: none;
  color: var(--text-color-3);
}
.episode-row {
  padding: 14px 0;
  border-bottom: 1px solid #8883;
}
.episode-file {
  margin-top: 10px;
  padding: 12px;
  border: 1px solid #8883;
  border-radius: 8px;
  background: #8881;
  overflow-wrap: anywhere;
}
.episode-file p {
  margin: 6px 0;
}
.episode-file .n-tag {
  margin-left: 10px;
}
.media-detail-modal :deep(.n-card__content) {
  padding: 0;
}
.media-detail-modal :deep(.n-card-header) {
  display: none;
}
.tv-detail-shell {
  overflow: hidden;
  border-radius: 18px;
  background: var(--surface-muted, #f8faff);
}
.tv-hero {
  position: relative;
  min-height: 190px;
  padding: 26px 30px 68px;
  overflow: hidden;
  color: #fff;
  background:
    linear-gradient(90deg, #11192bf2, #28334dcc), var(--tv-backdrop, linear-gradient(120deg, #49546f, #7b75ac));
  background-size: cover;
  background-position: center 25%;
}
.tv-hero::after {
  position: absolute;
  inset: 0;
  content: '';
  pointer-events: none;
  background: radial-gradient(circle at 84% 35%, #67e8f940, transparent 25rem);
}
.tv-hero-content {
  position: relative;
  z-index: 1;
  max-width: 72%;
}
.tv-kicker {
  color: #bdc6ff;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 1.4px;
}
.tv-hero h2 {
  margin: 6px 0 3px;
  color: #fff;
  font-size: 27px;
}
.tv-meta,
.tv-description {
  margin: 0;
  color: #d8dfeb;
  font-size: 13px;
}
.tv-description {
  max-width: 760px;
  margin-top: 12px;
  line-height: 1.65;
}
.tv-close {
  position: absolute;
  z-index: 2;
  top: 18px;
  right: 22px;
  width: 36px;
  height: 36px;
  border: 0;
  border-radius: 50%;
  color: #fff;
  background: #ffffff20;
  font-size: 22px;
  cursor: pointer;
}
.tv-stat-list {
  position: absolute;
  right: 26px;
  bottom: 22px;
  z-index: 1;
  display: flex;
  gap: 8px;
}
.tv-stat-list span {
  padding: 7px 10px;
  border: 1px solid #ffffff26;
  border-radius: 11px;
  color: #e6eaf2;
  background: #ffffff20;
  font-size: 12px;
}
.tv-stat-list b {
  margin-right: 3px;
  color: #fff;
  font-size: 18px;
}
.tv-detail-shell > .n-spin-container > .n-alert {
  margin: 14px 18px 0;
}
.tv-detail-body {
  display: grid;
  grid-template-columns: 235px minmax(0, 1fr);
  min-height: 560px;
  max-height: calc(90vh - 190px);
}
.season-sidebar {
  padding: 18px 14px;
  overflow: auto;
  border-right: 1px solid #e3e8f1;
  background: #fff;
}
.season-sidebar-title {
  padding: 0 10px 8px;
  color: #98a2b3;
  font-size: 11px;
  font-weight: 800;
  letter-spacing: 1.2px;
}
.season-nav-item {
  display: block;
  width: 100%;
  margin: 4px 0;
  padding: 12px;
  text-align: left;
  border: 1px solid transparent;
  border-radius: 13px;
  color: inherit;
  background: transparent;
  cursor: pointer;
}
.season-nav-item:hover {
  background: var(--surface-muted, #f5f7fb);
}
.season-nav-item.active {
  border-color: #d8ddff;
  color: var(--brand, #4848c8);
  background: linear-gradient(135deg, #ececff, #edfbff);
}
.season-nav-item strong,
.season-nav-item small {
  display: block;
}
.season-nav-item small {
  margin-top: 2px;
  color: #8993a5;
}
.season-nav-item i {
  display: block;
  height: 5px;
  margin-top: 8px;
  overflow: hidden;
  border-radius: 3px;
  background: #edf0f5;
}
.season-nav-item em {
  display: block;
  height: 100%;
  border-radius: inherit;
  background: linear-gradient(90deg, var(--brand, #5b5ce2), var(--cyan, #06b6d4));
}
.season-content {
  padding: 18px 22px;
  overflow: auto;
}
.season-content-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 14px;
}
.season-content-header h3 {
  margin: 0;
  font-size: 18px;
}
.season-content-header p {
  margin: 3px 0 0;
  color: #7b8496;
  font-size: 12px;
}
.season-filters {
  display: flex;
  gap: 7px;
}
.season-filters span {
  padding: 6px 10px;
  border: 1px solid #e0e5ee;
  border-radius: 9px;
  color: #667085;
  background: #fff;
  font-size: 12px;
}
.season-filters .selected {
  border-color: #d5d5ff;
  color: var(--brand, #4d4ed0);
  background: #ececff;
}
.episode-row {
  margin-top: 10px;
  padding: 15px;
  border: 1px solid #e1e6ef;
  border-radius: 14px;
  background: #fff;
  box-shadow: 0 5px 16px #3341550b;
}
.episode-heading {
  display: grid;
  grid-template-columns: 132px minmax(0, 1fr) auto;
  align-items: start;
  gap: 12px;
}
.episode-visual {
  position: relative;
  width: 132px;
  aspect-ratio: 16 / 9;
  overflow: hidden;
  border-radius: 11px;
  color: #8993a5;
  background: #eef1f6;
  font-size: 11px;
  display: grid;
  place-items: center;
}
.episode-visual img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
.episode-visual b {
  position: absolute;
  bottom: 6px;
  left: 7px;
  padding: 3px 7px;
  border-radius: 7px;
  color: #fff;
  background: #11192bcc;
  font-size: 11px;
}
.episode-main {
  min-width: 0;
}
.episode-heading strong {
  display: block;
  color: #303b55;
  font-size: 15px;
}
.episode-heading small {
  display: flex;
  align-items: flex-end;
  flex-direction: column;
  gap: 5px;
  color: #8993a5;
  white-space: nowrap;
}
.episode-file-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 9px;
  margin-left: 144px;
}
.episode-file {
  margin-top: 10px;
  padding: 11px 12px;
  border: 1px solid #e5e9f1;
  border-radius: 11px;
  background: #fafbfe;
}
.episode-file > div:first-child {
  display: flex;
  justify-content: space-between;
  gap: 8px;
}
.episode-file a {
  display: block;
  overflow: hidden;
  color: var(--brand, #5b5ce2);
  text-overflow: ellipsis;
  white-space: nowrap;
}
.episode-file-actions {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  color: #8993a5;
  font-size: 11px;
}
.movie-detail-shell {
  overflow: hidden;
  border-radius: 18px;
  background: var(--surface-muted, #f8faff);
}
.movie-hero {
  min-height: 210px;
}
.movie-source-content {
  max-height: calc(90vh - 210px);
  padding: 20px 24px;
  overflow: auto;
}
.movie-source-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 15px;
}
.movie-source-header h3 {
  margin: 0;
  font-size: 18px;
}
.movie-source-header p {
  margin: 3px 0 0;
  color: #7b8496;
  font-size: 12px;
}
.movie-source-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 11px;
  margin-bottom: 18px;
}
.movie-source-card {
  min-width: 0;
  padding: 14px;
  border: 1px solid #e1e6ef;
  border-radius: 13px;
  background: #fff;
  box-shadow: 0 5px 16px #3341550b;
}
.episode-file-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
}
.episode-file-top strong {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.movie-file-name {
  overflow-wrap: anywhere;
  color: #667085;
  font-size: 12px;
}
.movie-source-card > a {
  display: block;
  margin-bottom: 9px;
  overflow: hidden;
  color: var(--brand, #5b5ce2);
  font-size: 12px;
  text-overflow: ellipsis;
  white-space: nowrap;
}
@media (max-width: 600px) {
  .library-page {
    padding: 12px;
  }
  .poster-wall {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }
  .season-heading,
  .episode-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 4px;
  }
  .episode-file {
    padding: 10px;
  }
  .tv-hero {
    padding: 22px 20px 92px;
  }
  .tv-hero-content {
    max-width: 100%;
  }
  .tv-hero h2 {
    font-size: 22px;
  }
  .tv-stat-list {
    right: 18px;
    left: 18px;
    flex-wrap: wrap;
  }
  .tv-stat-list span {
    flex: 1;
    text-align: center;
  }
  .season-heading,
  .episode-heading {
    align-items: flex-start;
    flex-direction: column;
    gap: 5px;
  }
  .tv-detail-body {
    display: block;
    max-height: calc(90vh - 255px);
    overflow: auto;
  }
  .season-sidebar {
    display: flex;
    gap: 8px;
    padding: 10px;
    overflow-x: auto;
    border-right: 0;
    border-bottom: 1px solid #e3e8f1;
  }
  .season-sidebar-title {
    display: none;
  }
  .season-nav-item {
    flex: 0 0 150px;
    margin: 0;
  }
  .season-content {
    padding: 12px 10px;
    overflow: visible;
  }
  .season-content-header {
    align-items: flex-start;
    flex-direction: column;
  }
  .episode-heading {
    grid-template-columns: 104px minmax(0, 1fr);
  }
  .episode-visual {
    width: 104px;
  }
  .episode-heading small {
    grid-column: 2;
    align-items: flex-start;
  }
  .episode-file-grid {
    grid-template-columns: 1fr;
    margin-left: 0;
  }
  .movie-source-grid {
    grid-template-columns: 1fr;
  }
  .movie-source-content {
    padding: 14px 10px;
  }
}
</style>
