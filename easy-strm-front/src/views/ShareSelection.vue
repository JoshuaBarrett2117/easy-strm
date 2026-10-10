<template>
  <section class="selection-page">
    <n-space justify="space-between" align="center"><div><h2>STRM 来源选择</h2><p>逐条选择电影或季集的播放来源；改选在下一次导出生效，文件路径不变。</p></div><n-button :loading="refreshing" @click="refresh">刷新应用库候选</n-button></n-space>
    <n-alert type="info">首次默认使用最先持久发现的来源。手选不可用时保留旧文件并报错，不会自动换源。增量缺少可信基线时，请到定时任务显式执行“分享库 STRM 全量对账”。</n-alert>
    <n-card class="selection-search"><n-space><n-input v-model:value="keyword" placeholder="搜索标题或媒体键" clearable @keyup.enter="search" /><n-button @click="search">搜索</n-button></n-space></n-card>
    <n-alert v-if="listError" type="error">{{ listError }} <n-button text @click="() => load()">重试</n-button></n-alert>
    <n-data-table :columns="columns" :data="items" :loading="loading" :row-key="row => row.media_item_key" :scroll-x="850"><template #empty>暂无可选来源，请先扫描识别分享文件，再刷新候选。</template></n-data-table>
    <n-pagination v-model:page="page" :page-size="pageSize" :item-count="total" @update:page="() => load()" />
    <n-modal :show="!!activeKey" preset="card" title="条目来源详情" style="width: min(1000px, 95vw)" @update:show="close">
      <n-spin :show="detailLoading">
        <n-alert v-if="detailError" type="error">{{ detailError }} <n-button text @click="open(activeKey)">刷新详情</n-button></n-alert>
        <template v-if="detail">
          <h3>{{ detail.selection.title }} · {{ episodeLabel(detail.selection) }}</h3>
          <p>当前来源 #{{ detail.selection.selected_candidate_id }} · {{ detail.selection.selection_mode === 'manual' ? '手动粘性选择' : '自动持久选择' }} · 版本 {{ detail.selection.selection_revision }}</p>
          <p>输出相对路径：{{ detail.selection.stable_relative_path || '首次成功导出后固定' }}</p>
          <n-alert v-if="!detail.selection.valid" type="warning">选定来源失效，原 STRM 文件保持不变。请明确选择有效候选。</n-alert>
          <n-alert v-if="detail.selection.selection_revision > detail.selection.exported_revision" type="info">待下次导出应用（已导出版本 {{ detail.selection.exported_revision }}）</n-alert>
          <n-data-table :columns="candidateColumns" :data="detail.candidates" :row-key="row => row.candidate_id" :scroll-x="800" />
        </template>
      </n-spin>
    </n-modal>
  </section>
</template>

<script setup>
import { computed, h, onMounted, onBeforeUnmount, ref } from 'vue'
import { NAlert, NButton, NCard, NDataTable, NInput, NModal, NPagination, NSpace, NSpin, useDialog, useMessage } from 'naive-ui'
import { listShareSelections, getShareSelectionDetail, changeShareSelection, refreshShareSelections } from '../utils/api/share-selection'
import { createActionTrace } from '../utils/ui/action-trace'

const message = useMessage(), dialog = useDialog()
const keyword = ref(''), page = ref(1), pageSize = 20, total = ref(0), items = ref([])
const loading = ref(false), refreshing = ref(false), listError = ref('')
const activeKey = ref(''), detail = ref(null), detailLoading = ref(false), detailError = ref(''), changingID = ref(null)
let listRevision = 0, detailRevision = 0
const errorText = error => error.response?.data?.error || error.response?.data?.message || error.message || '请求失败'
const episodeLabel = row => row.season_number === 0 && row.episode_number === 0 ? '电影' : `S${String(row.season_number).padStart(2, '0')}E${String(row.episode_number).padStart(2, '0')}`
const columns = [
  { title: '作品', key: 'title', minWidth: 200 },
  { title: '季集', key: 'episode_number', render: episodeLabel },
  { title: '选定来源', key: 'selected_candidate_id', render: row => `#${row.selected_candidate_id} · ${row.selection_mode === 'manual' ? '手动' : '自动'}` },
  { title: '状态', key: 'valid', render: row => !row.valid ? '来源失效 · 保留旧文件' : row.selection_revision > row.exported_revision ? '待导出' : '已应用' },
  { title: '操作', key: 'actions', fixed: 'right', width: 120, render: row => h(NButton, { onClick: () => open(row.media_item_key) }, { default: () => '查看候选' }) }
]
const candidateColumns = computed(() => {
  const changing = changingID.value
  const selected = detail.value?.selection.selected_candidate_id
  return [
  { title: '分享来源', key: 'share_name', minWidth: 150 },
  { title: '文件', key: 'file_name', minWidth: 200, ellipsis: { tooltip: true } },
  { title: '大小', key: 'file_size', render: row => `${(row.file_size / 1024 ** 3).toFixed(2)} GB` },
  { title: '发现顺序', key: 'first_seen_seq' },
  { title: '状态', key: 'available', render: row => row.revoked ? '识别映射已撤销' : row.available ? '可用' : '不可用' },
  { title: '操作', key: 'actions', fixed: 'right', width: 150, render: row => h(NButton, { disabled: !row.available || row.revoked || changing !== null, loading: changing === row.candidate_id, onClick: () => confirmChange(row) }, { default: () => row.candidate_id === selected ? '选定 · 设为手动' : '选择此来源' }) }
  ]
})
async function load(traceId = createActionTrace()) {
  const revision = ++listRevision
  loading.value = true; listError.value = ''
  try { const response = await listShareSelections({ page: page.value, page_size: pageSize, keyword: keyword.value }, traceId); if (revision === listRevision) { items.value = response.data.data.data; total.value = response.data.data.total } }
  catch (error) { if (revision === listRevision) listError.value = errorText(error) }
  finally { if (revision === listRevision) loading.value = false }
}
function search() { page.value = 1; load() }
async function refresh() {
  const traceId = createActionTrace()
  refreshing.value = true
  try { await refreshShareSelections(traceId); message.success('应用库候选已刷新，不访问分享网络'); await load(traceId) }
  catch (error) { message.error(errorText(error)) }
  finally { refreshing.value = false }
}
async function open(key, traceId = createActionTrace()) {
  activeKey.value = key; detail.value = null; detailError.value = ''; detailLoading.value = true
  const revision = ++detailRevision
  try { const response = await getShareSelectionDetail(key, traceId); if (revision === detailRevision) detail.value = response.data.data }
  catch (error) { if (revision === detailRevision) detailError.value = errorText(error) }
  finally { if (revision === detailRevision) detailLoading.value = false }
}
function close() { detailRevision++; activeKey.value = ''; detail.value = null; detailLoading.value = false; detailError.value = '' }
function confirmChange(row) {
  const selection = detail.value.selection
  dialog.warning({ title: '确认改选来源', content: `将 ${selection.title} ${episodeLabel(selection)} 改为 ${row.share_name} 的 ${row.file_name}。只在下一次导出替换内容，路径保持不变。`, positiveText: '确认改选', negativeText: '取消', onPositiveClick: async () => {
    const traceId = createActionTrace()
    changingID.value = row.candidate_id
    try { await changeShareSelection({ media_item_key: selection.media_item_key, candidate_id: row.candidate_id, expected_revision: selection.selection_revision }, traceId); message.success('改选已保存，待下一次导出生效'); if (activeKey.value === selection.media_item_key) await open(activeKey.value, traceId); await load(traceId) }
    catch (error) { message.error(errorText(error)); if (activeKey.value === selection.media_item_key) await open(activeKey.value, traceId) }
    finally { changingID.value = null }
  } })
}
onMounted(() => load())
onBeforeUnmount(() => { listRevision++; close() })
</script>

<style scoped>
.selection-page { display: grid; gap: 16px; min-width: 0; width: 100%; }
.selection-page > * { min-width: 0; max-width: 100%; }
.selection-page h2, .selection-page p { margin: 0 0 8px; }
.selection-search { margin-top: 0; }
@media (max-width: 640px) { .selection-page { gap: 12px; } }
</style>
