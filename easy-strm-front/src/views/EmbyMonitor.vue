<template>
  <div class="space-y-5">
    <PageCard title="Emby 观影监控" subtitle="实时会话、播放历史与媒体入库的统一观察入口">
      <template #action>
        <n-select v-model:value="serverId" class="w-56" :options="serverOptions" placeholder="选择 Emby 实例" />
        <n-button :loading="loading" @click="reloadActive">刷新</n-button>
      </template>

      <n-alert v-if="meta.degraded_reason" type="warning" :closable="false" class="mb-4">
        Playback Reporting 暂不可用，已使用 easy-strm 自采历史。{{ meta.degraded_reason }}
      </n-alert>
      <div class="mb-4 flex flex-wrap items-center gap-2 text-xs text-slate-500">
        <n-tag size="small" :type="meta.source === 'playback_reporting' ? 'success' : 'info'">{{ sourceText }}</n-tag>
        <span>生成时间 {{ formatDateTime(meta.generated_at) }}</span>
        <span v-if="meta.history_start_at">· 统计始于 {{ formatDateTime(meta.history_start_at) }}</span>
      </div>

      <n-tabs v-model:value="activeTab" type="segment" animated @update:value="loadActive">
        <n-tab-pane name="overview" tab="概览"><OverviewPanel :data="overview" :loading="loading" :image-urls="imageURLs" /></n-tab-pane>
        <n-tab-pane name="users" tab="用户排行"><RankingPanel dimension="users" :server-id="serverId" :active="activeTab === 'users'" @meta="setMeta" /></n-tab-pane>
        <n-tab-pane name="media" tab="媒体排行"><RankingPanel dimension="media" :server-id="serverId" :active="activeTab === 'media'" @meta="setMeta" /></n-tab-pane>
        <n-tab-pane name="clients" tab="客户端排行"><RankingPanel dimension="clients" :server-id="serverId" :active="activeTab === 'clients'" @meta="setMeta" /></n-tab-pane>
        <n-tab-pane name="heatmap" tab="活跃热力图"><HeatmapPanel :server-id="serverId" :active="activeTab === 'heatmap'" @meta="setMeta" /></n-tab-pane>
        <n-tab-pane name="recent" tab="最近入库"><RecentItemsPanel :server-id="serverId" :active="activeTab === 'recent'" @meta="setMeta" /></n-tab-pane>
      </n-tabs>
    </PageCard>
  </div>
</template>

<script setup>
import { computed, defineComponent, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { NAlert, NAvatar, NButton, NEmpty, NProgress, NSelect, NSpin, NTabPane, NTabs, NTag } from 'naive-ui'
import VChart from 'vue-echarts'
import { use } from 'echarts/core'
import { CanvasRenderer } from 'echarts/renderers'
import { HeatmapChart, LineChart } from 'echarts/charts'
import { GridComponent, LegendComponent, TooltipComponent, VisualMapComponent } from 'echarts/components'
import PageCard from '../components/common/PageCard.vue'
import { getEmbyServers, getEmbyMonitorOverview, getEmbyMonitorRankings, getEmbyMonitorHeatmap, getEmbyMonitorRecentItems, getEmbyMonitorItemImage, getEmbyUserAvatar } from '../utils/api/emby'

use([CanvasRenderer, HeatmapChart, LineChart, GridComponent, LegendComponent, TooltipComponent, VisualMapComponent])

const payload = response => response?.data?.data || response?.data || {}
const servers = ref([])
const serverId = ref(null)
const activeTab = ref('overview')
const loading = ref(false)
const overview = ref({ summary: {}, trend: [], sessions: [] })
const meta = ref({})
const imageURLs = ref({})
let overviewTimer = null

const serverOptions = computed(() => servers.value.map(item => ({ label: item.name, value: item.id })))
const sourceText = computed(() => meta.value.source === 'playback_reporting' ? 'Playback Reporting 历史' : 'easy-strm 自采历史')
const setMeta = value => { meta.value = value || {} }
const formatDateTime = value => value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '--'
const formatDuration = seconds => { const value = Math.max(0, Number(seconds || 0)); const h = Math.floor(value / 3600); const m = Math.floor((value % 3600) / 60); return h ? `${h}小时${m}分` : `${m}分` }
const revokeImages = () => { Object.values(imageURLs.value).forEach(URL.revokeObjectURL); imageURLs.value = {} }
const loadImage = async (key, request) => { try { const response = await request(); imageURLs.value = { ...imageURLs.value, [key]: URL.createObjectURL(response.data) } } catch {} }
const loadOverviewImages = data => { revokeImages(); data.sessions?.forEach(item => { if (item.item_id) loadImage(`item:${item.item_id}`, () => getEmbyMonitorItemImage(serverId.value, item.item_id)); if (item.user_id) loadImage(`user:${item.user_id}`, () => getEmbyUserAvatar(serverId.value, item.user_id)) }) }
const loadOverview = async () => { if (!serverId.value) return; loading.value = true; try { const data = payload(await getEmbyMonitorOverview(serverId.value)); overview.value = data; setMeta(data.meta); loadOverviewImages(data) } finally { loading.value = false } }
const clearOverviewTimer = () => { clearInterval(overviewTimer); overviewTimer = null }
const startOverviewTimer = () => { clearOverviewTimer(); if (activeTab.value === 'overview' && serverId.value) overviewTimer = setInterval(loadOverview, 15000) }
const loadActive = () => { if (activeTab.value === 'overview') loadOverview(); startOverviewTimer() }
const reloadActive = () => { if (activeTab.value === 'overview') loadOverview(); else window.dispatchEvent(new CustomEvent('emby-monitor-refresh', { detail: activeTab.value })) }

const statLabels = [['today_seconds', '今日观影'], ['week_seconds', '本周观影'], ['month_seconds', '本月观影'], ['total_seconds', '历史总时长'], ['today_active_users', '今日活跃'], ['week_active_users', '本周活跃'], ['month_active_users', '本月活跃']]
const OverviewPanel = defineComponent({
  props: { data: Object, loading: Boolean, imageUrls: Object },
  setup(props) {
    const trendOption = computed(() => ({ tooltip: { trigger: 'axis' }, grid: { left: 42, right: 18, top: 28, bottom: 42 }, xAxis: { type: 'category', data: (props.data?.trend || []).map(p => new Date(p.time).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit' })) }, yAxis: { type: 'value', minInterval: 1 }, series: [{ type: 'line', smooth: true, showSymbol: false, areaStyle: { opacity: .12 }, lineStyle: { width: 2, color: '#22c7b8' }, data: (props.data?.trend || []).map(p => p.active_users) }] }))
    return () => h(NSpin, { show: props.loading }, { default: () => h('div', { class: 'space-y-5' }, [
      h('div', { class: 'grid gap-3 sm:grid-cols-2 xl:grid-cols-4' }, statLabels.map(([key, label]) => h('div', { class: 'rounded-2xl border border-slate-200 p-4 dark:border-white/10' }, [h('div', { class: 'text-xs text-slate-500' }, label), h('div', { class: 'mt-2 text-xl font-extrabold' }, key.includes('seconds') ? formatDuration(props.data?.summary?.[key]) : String(props.data?.summary?.[key] || 0))]))),
      h('div', { class: 'rounded-2xl border border-slate-200 p-3 dark:border-white/10' }, [h('div', { class: 'mb-2 flex justify-between text-sm font-bold' }, [h('span', '近 7 天每小时活跃用户'), h('span', `峰值 ${props.data?.summary?.peak_users || 0} 人`)]), h(VChart, { class: 'h-72', option: trendOption.value, autoresize: true })]),
      h('div', {}, [h('h3', { class: 'mb-3 font-extrabold' }, '正在观看'), props.data?.sessions?.length ? h('div', { class: 'grid gap-3 md:grid-cols-2 xl:grid-cols-3' }, props.data.sessions.map(item => h('div', { class: 'rounded-2xl border border-slate-200 p-4 dark:border-white/10' }, [h('div', { class: 'flex gap-3' }, [h(NAvatar, { round: true, size: 48, src: props.imageUrls?.[`user:${item.user_id}`] }, { default: () => (item.user_name || '?').slice(0, 1) }), h('div', { class: 'min-w-0 flex-1' }, [h('div', { class: 'font-bold' }, item.user_name || '未知用户'), h('div', { class: 'truncate text-sm text-slate-600 dark:text-slate-300' }, item.series_name ? `${item.series_name} · ${item.item_name}` : item.item_name), h('div', { class: 'mt-1 text-xs text-slate-500' }, `${item.client_name || '未知客户端'} · ${item.device_name || '未知设备'} · ${item.playback_method || '未知方式'}`)])]), h(NProgress, { class: 'mt-3', percentage: Math.min(100, Math.round(item.progress || 0)), status: item.paused ? 'warning' : 'success', 'show-indicator': true })])) ) : h(NEmpty, { description: '当前没有播放中的会话' })])
    ]) })
  }
})

const periodOptions = [{ label: '日榜', value: 'day' }, { label: '昨日榜', value: 'yesterday' }, { label: '周榜', value: 'week' }, { label: '月榜', value: 'month' }, { label: '总榜', value: 'total' }]
const RankingPanel = defineComponent({
  props: { dimension: String, serverId: Number, active: Boolean }, emits: ['meta'],
  setup(props, { emit }) {
    const period = ref('day'); const mediaType = ref('movie'); const data = ref([]); const busy = ref(false); const urls = ref({})
    const revoke = () => { Object.values(urls.value).forEach(URL.revokeObjectURL); urls.value = {} }
    const load = async () => {
      if (!props.active || !props.serverId) return
      busy.value = true; revoke()
      try {
        const result = payload(await getEmbyMonitorRankings(props.serverId, props.dimension, { period: period.value, media_type: mediaType.value }))
        data.value = result.data || []; emit('meta', result.meta)
        if (props.dimension !== 'clients') data.value.forEach(async item => {
          if (!item.id) return
          try {
            const response = props.dimension === 'users' ? await getEmbyUserAvatar(props.serverId, item.id) : await getEmbyMonitorItemImage(props.serverId, item.id)
            urls.value = { ...urls.value, [item.id]: URL.createObjectURL(response.data) }
          } catch {}
        })
      } finally { busy.value = false }
    }
    watch(() => [props.serverId, props.active, period.value, mediaType.value], load, { immediate: true })
    onMounted(() => window.addEventListener('emby-monitor-refresh', load))
    onBeforeUnmount(() => { revoke(); window.removeEventListener('emby-monitor-refresh', load) })
    return () => h('div', { class: 'space-y-4' }, [
      h('div', { class: 'flex flex-wrap gap-2' }, [
        h(NSelect, { value: period.value, options: periodOptions, class: 'w-36', 'onUpdate:value': value => period.value = value }),
        props.dimension === 'media' ? h(NSelect, { value: mediaType.value, options: [{ label: '电影', value: 'movie' }, { label: '剧集', value: 'series' }], class: 'w-32', 'onUpdate:value': value => mediaType.value = value }) : null
      ]),
      h(NSpin, { show: busy.value }, { default: () => data.value.length ? h('div', { class: 'space-y-2' }, data.value.map(item => h('div', { class: 'rounded-2xl border border-slate-200 p-4 dark:border-white/10' }, [
        h('div', { class: 'flex items-start justify-between gap-3' }, [
          h('div', { class: 'flex min-w-0 items-center gap-3' }, [h(NAvatar, { round: props.dimension !== 'media', size: 48, src: urls.value[item.id] }, { default: () => (item.name || '?').slice(0, 1) }), h('div', [h('div', { class: 'font-bold' }, `${item.rank}. ${item.name || '未知'}`), h('div', { class: 'text-xs text-slate-500' }, `${item.subtitle || (props.dimension === 'clients' ? '客户端' : '用户')} · 播放 ${item.play_count} 次`)])]),
          h('strong', formatDuration(item.watched_seconds))
        ]),
        h(NProgress, { class: 'mt-2', percentage: Math.round(item.percentage || 0), 'show-indicator': false, color: '#22a6f2' })
      ]))) : h(NEmpty, { description: '当前范围没有播放记录' }) })
    ])
  }
})

const HeatmapPanel = defineComponent({
  props: { serverId: Number, active: Boolean }, emits: ['meta'],
  setup(props, { emit }) { const range = ref('7d'); const userId = ref(''); const users = ref([]); const busy = ref(false); const load = async () => { if (!props.active || !props.serverId) return; busy.value = true; try { const result = payload(await getEmbyMonitorHeatmap(props.serverId, { range: range.value, user_id: userId.value || undefined })); users.value = result.users || []; emit('meta', result.meta) } finally { busy.value = false } }; const userOptions = computed(() => [{ label: '全部用户', value: '' }, ...users.value.map(u => ({ label: u.user_name, value: u.user_id }))]); const optionFor = user => { const dates = [...new Set((user.cells || []).map(c => c.date))].sort(); return { tooltip: { position: 'top', formatter: p => `${dates[p.value[1]]} ${p.value[0]}:00<br/>${formatDuration(p.value[2])}` }, grid: { left: 56, right: 20, top: 18, bottom: 42, containLabel: true }, xAxis: { type: 'category', data: Array.from({ length: 24 }, (_, i) => i), splitArea: { show: true }, axisLabel: { interval: 0 } }, yAxis: { type: 'category', data: dates, splitArea: { show: true } }, visualMap: { min: 0, max: Math.max(60, ...user.cells.map(c => c.seconds)), show: false, inRange: { color: ['#eef2f7', '#5eead4', '#0891b2'] } }, series: [{ type: 'heatmap', data: user.cells.map(c => [c.hour, dates.indexOf(c.date), c.seconds]), itemStyle: { borderColor: '#fff', borderWidth: 1 } }] } }; watch(() => [props.serverId, props.active, range.value, userId.value], load, { immediate: true }); onMounted(() => window.addEventListener('emby-monitor-refresh', load)); onBeforeUnmount(() => window.removeEventListener('emby-monitor-refresh', load)); return () => h('div', { class: 'space-y-4' }, [h('div', { class: 'flex flex-wrap gap-2' }, [h(NSelect, { value: range.value, options: [{ label: '今日', value: 'today' }, { label: '昨日', value: 'yesterday' }, { label: '近 7 天', value: '7d' }], class: 'w-36', 'onUpdate:value': v => range.value = v }), h(NSelect, { value: userId.value, options: userOptions.value, class: 'w-48', 'onUpdate:value': v => userId.value = v })]), h(NSpin, { show: busy.value }, { default: () => users.value.length ? h('div', { class: 'grid gap-3 xl:grid-cols-2' }, users.value.map(user => h('div', { class: 'rounded-2xl border border-slate-200 p-3 dark:border-white/10' }, [h('div', { class: 'flex justify-between font-bold' }, [h('span', user.user_name), h('span', formatDuration(user.total_seconds))]), h(VChart, { style: { height: '260px', width: '100%' }, option: optionFor(user), autoresize: true })]))) : h(NEmpty, { description: '当前范围没有热力数据' }) })]) }
})

const RecentItemsPanel = defineComponent({
  props: { serverId: Number, active: Boolean }, emits: ['meta'],
  setup(props, { emit }) { const basis = ref('emby'); const range = ref('today'); const mediaType = ref('movie'); const data = ref([]); const busy = ref(false); const urls = ref({}); const revoke = () => { Object.values(urls.value).forEach(URL.revokeObjectURL); urls.value = {} }; const load = async () => { if (!props.active || !props.serverId) return; busy.value = true; revoke(); try { const result = payload(await getEmbyMonitorRecentItems(props.serverId, { time_basis: basis.value, range: range.value, media_type: mediaType.value })); data.value = result.data || []; emit('meta', result.meta); data.value.forEach(async item => { try { const response = await getEmbyMonitorItemImage(props.serverId, item.item_id); urls.value = { ...urls.value, [item.item_id]: URL.createObjectURL(response.data) } } catch {} }) } finally { busy.value = false } }; watch(() => [props.serverId, props.active, basis.value, range.value, mediaType.value], load, { immediate: true }); onMounted(() => window.addEventListener('emby-monitor-refresh', load)); onBeforeUnmount(() => { revoke(); window.removeEventListener('emby-monitor-refresh', load) }); return () => h('div', { class: 'space-y-4' }, [h('div', { class: 'flex flex-wrap gap-2' }, [h(NSelect, { value: basis.value, options: [{ label: '按 Emby 时间', value: 'emby' }, { label: '按首次发现', value: 'first_seen' }], class: 'w-40', 'onUpdate:value': v => basis.value = v }), h(NSelect, { value: range.value, options: [{ label: '今天', value: 'today' }, { label: '昨天', value: 'yesterday' }, { label: '近 7 天', value: '7d' }], class: 'w-32', 'onUpdate:value': v => range.value = v }), h(NSelect, { value: mediaType.value, options: [{ label: '电影', value: 'movie' }, { label: '剧集', value: 'series' }], class: 'w-28', 'onUpdate:value': v => mediaType.value = v })]), h(NSpin, { show: busy.value }, { default: () => data.value.length ? h('div', { class: 'grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-5 2xl:grid-cols-7' }, data.value.map(item => h('div', { class: 'overflow-hidden rounded-2xl border border-slate-200 bg-slate-50 dark:border-white/10 dark:bg-white/[.03]' }, [urls.value[item.item_id] ? h('img', { src: urls.value[item.item_id], class: 'aspect-[2/3] w-full object-cover', alt: item.name }) : h('div', { class: 'flex aspect-[2/3] items-center justify-center text-slate-400' }, '暂无海报'), h('div', { class: 'p-3' }, [h('div', { class: 'line-clamp-2 font-bold' }, item.name), h('div', { class: 'mt-1 text-xs text-slate-500' }, item.year || '年份未知'), h('div', { class: 'mt-1 text-[11px] text-slate-400' }, formatDateTime(basis.value === 'emby' ? item.date_created : item.first_seen_at))])])) ) : h(NEmpty, { description: '当前范围没有入库媒体' }) })]) }
})

onMounted(async () => { const data = payload(await getEmbyServers()); servers.value = data.data || []; serverId.value = servers.value.find(item => item.is_default)?.id || servers.value[0]?.id || null; if (serverId.value) await loadOverview(); startOverviewTimer() })
watch(serverId, () => { revokeImages(); loadActive() })
onBeforeUnmount(() => { clearOverviewTimer(); revokeImages() })
</script>
