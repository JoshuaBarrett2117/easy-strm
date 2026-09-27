<template>
  <div class="space-y-4">
    <n-card title="手动核对中心">
      <n-alert type="info" class="mb-4">集中处理自动识别失败和待识别的分享媒体。选择 TMDB 候选并保存后，媒体会从核对队列移除。</n-alert>
      <n-space align="center" wrap>
        <n-select v-model:value="status" :options="statusOptions" style="width: 180px" @update:value="reload" />
        <n-select v-model:value="shareID" filterable remote clearable :options="shareOptions" placeholder="筛选分享来源" style="width:220px" @search="loadShares" @update:value="reload" />
        <n-input v-model:value="keyword" clearable placeholder="搜索分享名称、文件路径或错误信息" style="width: min(360px, 80vw)" @keyup.enter="reload" />
        <n-button type="primary" :loading="loading" @click="reload">搜索</n-button>
        <n-button :loading="loading" @click="load">刷新</n-button>
        <span class="text-xs text-slate-500">共 {{ total }} 条待核对媒体</span>
      </n-space>
    </n-card>

    <n-card>
      <n-data-table :scroll-x="1000" :columns="columns" :data="items" :loading="loading" :pagination="pagination" remote :row-key="row => row.id" @update:page="page=$event;load()" @update:page-size="changePageSize" />
    </n-card>

    <n-modal :content-style="{maxHeight:'75vh',overflowY:'auto'}" v-model:show="reviewShow" preset="card" title="手动核对媒体" style="width: min(900px, 96vw)" :mask-closable="!saving" :closable="!saving">
      <n-space vertical v-if="target">
        <n-descriptions bordered :column="1" size="small">
          <n-descriptions-item label="分享">{{ target.share_name }}</n-descriptions-item>
          <n-descriptions-item label="文件路径"><span class="break-all">{{ target.file_name }}</span></n-descriptions-item>
          <n-descriptions-item label="当前状态">{{ statusText(target.status) }}</n-descriptions-item>
          <n-descriptions-item label="识别说明">{{ target.error || target.result?.message || '暂无' }}</n-descriptions-item>
          <n-descriptions-item label="规则解析">{{ target.parsed_title || '标题未知' }} · {{ target.parsed_year || '年份未知' }} · {{ target.media_type || '类型未知' }}</n-descriptions-item>
        </n-descriptions>
        <n-form label-placement="top" :disabled="saving">
          <n-form-item label="片名"><n-input v-model:value="form.keyword" placeholder="输入电影或剧集名称" @keyup.enter="search" /></n-form-item>
          <n-space>
            <n-form-item label="媒体类型"><n-select v-model:value="form.type" :options="typeOptions" style="width:130px" /></n-form-item>
            <n-form-item label="年份"><n-input-number v-model:value="form.year" :min="1800" :max="2200" placeholder="可选" /></n-form-item>
            <n-form-item label="数据源"><n-select v-model:value="form.source" :disabled="form.type === 'tv'" :options="sourceOptions" style="width:130px" /></n-form-item>
          </n-space>
          <n-space v-if="form.type === 'tv'">
            <n-form-item label="季号"><n-input-number v-model:value="form.season" :min="0" :precision="0" /></n-form-item>
            <n-form-item label="集号"><n-input v-model:value="form.episodes" placeholder="例如 1,2" style="width:220px" /></n-form-item>
          </n-space>
        </n-form>
        <n-space><n-button type="primary" :loading="searching" :disabled="saving" @click="search">搜索 TMDB</n-button><n-button :disabled="saving" @click="reviewShow=false">取消</n-button></n-space>
        <n-empty v-if="searched && !searching && !candidates.length" description="没有匹配候选，请调整片名、年份或类型" />
        <n-list v-if="candidates.length" bordered>
          <n-list-item v-for="candidate in candidates" :key="`${candidate.media_type}-${candidate.tmdb_id}-${candidate.metadata_id}`">
            <n-thing :title="candidate.title" :description="`${candidate.original_title || '无原名'} · ${candidate.year || '年份未知'} · ${candidate.media_type === 'tv' ? '电视剧' : '电影'}`">
              <template #avatar><n-image v-if="candidate.poster_path" :src="candidate.poster_path" width="48" height="72" object-fit="cover" /></template>
              <template #action><n-button type="primary" :loading="saving" @click="selected=candidate">选择候选</n-button></template>
            </n-thing>
          </n-list-item>
        </n-list>
        <n-alert v-if="selected" type="info">确认将当前文件关联到 {{ selected.title }} · {{ selected.year }} · {{ selected.media_type }} · ID {{ selected.tmdb_id || selected.metadata_id }}<span v-if="form.type==='tv'"> · 第 {{ form.season }} 季 · 集号 {{ form.episodes }}</span></n-alert>
        <n-button v-if="selected" type="primary" :loading="saving" @click="save(selected)">保存核对结果</n-button>
      </n-space>
    </n-modal>
  </div>
</template>

<script setup>
import { h, onMounted, onBeforeUnmount, watch, reactive, ref } from 'vue'
import { useRoute } from 'vue-router'
import { NAlert, NButton, NCard, NDataTable, NDescriptions, NDescriptionsItem, NEmpty, NForm, NFormItem, NImage, NInput, NInputNumber, NList, NListItem, NModal, NSpace, NSelect, NThing, useMessage } from 'naive-ui'
import { getShareRecords, getShareReviewItems, manualIdentifyShareMedia, searchTmdb } from '../utils/api/media'

const route = useRoute(), message = useMessage()
const shareID=ref(Number(route.query.share_id)||null), shareOptions=ref([]), selected=ref(null)
let disposed=false
let loadVersion=0, searchVersion=0, shareVersion=0
const loadShares=async (keyword='')=>{ const version=++shareVersion; try { const r=await getShareRecords({keyword,page:1,page_size:100}); if(version===shareVersion) shareOptions.value=(r.data?.data?.data||[]).map(s=>({label:s.name,value:s.id})) } catch { message.error('分享来源加载失败') } }
const items = ref([]), total = ref(0), loading = ref(false), page = ref(1), status = ref(route.query.status || 'failed,pending'), keyword = ref('')
const pagination = reactive({ page: 1, pageSize: 20, pageSizes: [10, 20, 50, 100], showSizePicker: true, itemCount: 0 })
const reviewShow = ref(false), target = ref(null), searching = ref(false), saving = ref(false), searched = ref(false), candidates = ref([])
const form = reactive({ keyword: '', type: 'tv', source: 'tmdb', year: null, season: 1, episodes: '' })
const statusOptions = [{ label: '全部待核对', value: 'failed,pending' }, { label: '识别失败', value: 'failed' }, { label: '待识别', value: 'pending' }]
const typeOptions = [{ label: '电视剧', value: 'tv' }, { label: '电影', value: 'movie' }]
const sourceOptions = [{ label: 'TMDB', value: 'tmdb' }, { label: 'MetaTube', value: 'metatube' }]
const statusText = value => value === 'failed' ? '识别失败' : value === 'pending' ? '待识别' : value
const load = async () => {
  const version=++loadVersion
  loading.value = true
  pagination.page=page.value
  try {
    const response = await getShareReviewItems({ page: page.value, page_size: pagination.pageSize, status: status.value, keyword: keyword.value, share_id: shareID.value || undefined })
    if(version!==loadVersion)return
    const payload = response.data?.data || {}
    items.value = payload.data || []; total.value = payload.total || 0; pagination.itemCount = total.value
    if(page.value>1 && !items.value.length){ page.value=Math.max(1,Math.ceil(total.value/pagination.pageSize)); return load() }
  } catch { message.error('加载核对队列失败') } finally { if(version===loadVersion) loading.value = false }
}
const reload = () => { page.value = 1; load() }
const changePageSize = size => { pagination.pageSize = size; page.value = 1; load() }
const openReview = item => {
  searchVersion++; searching.value=false; selected.value=null; target.value = item; candidates.value = []; searched.value = false; reviewShow.value = true
  form.keyword = item.result?.title || item.result?.original_title || item.parsed_title || item.file_name.split(/[\\/]/).pop().replace(/\.[^.]+$/, '')
  form.type = ['movie','tv'].includes(item.result?.media_type) ? item.result.media_type : (item.media_type==='tv'?'tv':'movie'); form.source = item.metadata_source === 'metatube' ? 'metatube' : 'tmdb'; form.year = item.result?.year || item.parsed_year || null
  form.season = item.episodes?.[0]?.season_number ?? item.result?.season_number ?? 1; form.episodes = (item.episodes || []).map(e => e.episode_number).join(',') || String(item.result?.episode_number || '')
}
const search = async () => {
  if (!form.keyword.trim() || searching.value) return
  const version=++searchVersion
  searching.value = true; candidates.value = []; selected.value=null
  try { const response = await searchTmdb({ keyword: form.keyword.trim(), year: form.year || undefined, type: form.type, metadata_source: form.type === 'tv' ? 'tmdb' : form.source }); if(version!==searchVersion)return; candidates.value = response.data?.data?.data || []; searched.value = true }
  catch { message.error('TMDB 搜索失败') } finally { if(version===searchVersion) searching.value = false }
}
const save = async candidate => {
  if(saving.value)return
  const tokens=form.episodes.trim().split(/[,，\s]+/)
  if(form.type==='tv' && tokens.some(v=>!/^\d+$/.test(v)||Number(v)<=0)){ message.error('集号必须为正整数，多个用逗号分隔'); return }
  const episodes = form.type === 'tv' ? [...new Set(form.episodes.split(/[,，\s]+/).map(Number).filter(v => Number.isInteger(v) && v > 0))] : []
  if (form.type === 'tv' && (!Number.isInteger(form.season) || form.season < 0 || !episodes.length)) { message.error('电视剧必须填写有效的季号和集号'); return }
  saving.value = true
  try { await manualIdentifyShareMedia(target.value.id, { ...target.value, result: candidate, episodes: episodes.map(v => ({ season_number: form.season, episode_number: v })) }); message.success('手动核对已保存'); reviewShow.value = false; await load() }
  catch (error) { message.error(error.response?.data?.error || error.response?.data?.message || '保存核对结果失败') } finally { saving.value = false }
}
const columns = [
  { title: '分享', key: 'share_name', ellipsis: { tooltip: true } },
  { title: '文件路径', key: 'file_name', ellipsis: { tooltip: true } },
  { title: '状态', key: 'status', render: row => h('span', { class: row.status === 'failed' ? 'text-red-500' : 'text-amber-500' }, statusText(row.status)) },
  { title: '识别说明', render: row => row.error || row.result?.message || '待识别', ellipsis: { tooltip: true } },
  { title: '季集', render: row => (row.episodes || []).map(e => `S${String(e.season_number).padStart(2, '0')}E${String(e.episode_number).padStart(2, '0')}`).join('、') || '—' },
  { title: '操作', render: row => h(NButton, { type: 'primary', size: 'small', onClick: () => openReview(row) }, { default: () => '核对' }) }
]
watch(()=>[form.keyword,form.type,form.source,form.year],()=>{searchVersion++; searching.value=false; candidates.value=[]; selected.value=null; searched.value=false})
watch(reviewShow,()=>{searchVersion++; searching.value=false})
onBeforeUnmount(()=>{disposed=true;loadVersion++;searchVersion++;shareVersion++})
onMounted(async()=>{ await Promise.all([load(),loadShares()]); if(!disposed && route.query.media_id){ try {const r=await getShareReviewItems({media_id:route.query.media_id,page:1,page_size:1});const item=r.data?.data?.data?.[0];if(disposed)return;if(item)openReview(item);else message.info('该媒体已处理或不在待核对队列中')}catch{message.error('目标媒体加载失败')} } })
</script>
