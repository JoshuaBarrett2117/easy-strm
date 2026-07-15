<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <PageCard title="网络连通性测试" subtitle="Network Probe">
      <template #action>
        <n-button :loading="networkLoading" @click="loadNetworkResults">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          重新探测
        </n-button>
      </template>
      <p class="text-sm text-slate-400 dark:text-slate-500">
        直接复用现有 `/api/network/test` 能力，对 Telegram、GitHub、TMDB 等关键站点进行逐一探测。
      </p>
    </PageCard>

    <!-- 探测结果 -->
    <PageCard>
      <div class="overflow-x-auto">
        <n-data-table
          :columns="networkColumns"
          :data="networkResults"
          :loading="networkLoading"
          :row-key="(row) => `${row.name}-${row.url}`"
          :scroll-x="900"
        />
      </div>
    </PageCard>
  </div>
</template>

<script setup>
import { h, ref } from 'vue'
import { NButton, NDataTable, NIcon, NTag, useMessage } from 'naive-ui'
import { RefreshOutline } from '@vicons/ionicons5'
import PageCard from '../../components/common/PageCard.vue'
import { getNetworkProbeSites, testNetworkConnectivity } from '../../utils/api/setting'

const message = useMessage()

const networkLoading = ref(false)
const networkResults = ref([])
let runId = 0

const networkColumns = [
  { title: '站点', key: 'name', minWidth: 130 },
  { title: '地址', key: 'url', minWidth: 240, ellipsis: { tooltip: true } },
  {
    title: '连通状态',
    key: 'status',
    width: 110,
    render: (row) => h(
      NTag,
      { type: getNetworkStatusTag(row), size: 'small' },
      { default: () => getNetworkStatusText(row) }
    )
  },
  {
    title: '代理路径',
    key: 'via_proxy',
    width: 120,
    render: (row) => {
      if (row.via_proxy === null || row.via_proxy === undefined) return '-'
      return h(
        NTag,
        { type: row.via_proxy ? 'warning' : 'info', size: 'small' },
        { default: () => (row.via_proxy ? '代理' : '直连') }
      )
    }
  },
  { title: 'HTTP', key: 'status_code', width: 90, render: (row) => row.status_code ?? '-' },
  { title: '耗时(ms)', key: 'duration_ms', width: 110, render: (row) => row.duration_ms ?? '-' },
  {
    title: '错误信息',
    key: 'error',
    minWidth: 220,
    render: (row) => row.error || row.status_message || '-'
  }
]

const normalizeSites = (payload) => {
  if (Array.isArray(payload)) return payload
  if (Array.isArray(payload?.data)) return payload.data
  return []
}

const normalizeResult = (payload) => {
  if (Array.isArray(payload)) return payload[0] || null
  if (Array.isArray(payload?.data)) return payload.data[0] || null
  if (payload?.data && typeof payload.data === 'object') return payload.data
  if (payload && typeof payload === 'object') return payload
  return null
}

const loadNetworkResults = async () => {
  const currentRun = ++runId
  networkLoading.value = true
  try {
    const siteResponse = await getNetworkProbeSites({ skipGlobalErrorMessage: true })
    const sites = normalizeSites(siteResponse.data.data)
    networkResults.value = sites.map(site => ({
      ...site,
      ok: null,
      status_code: null,
      duration_ms: null,
      via_proxy: null,
      error: '',
      status_message: '待测试'
    }))

    for (const site of sites) {
      if (currentRun !== runId) return
      const currentItem = networkResults.value.find(item => item.name === site.name && item.url === site.url)
      if (currentItem) currentItem.status_message = '测试中...'

      try {
        const response = await testNetworkConnectivity({
          name: site.name,
          url: site.url,
          skipGlobalErrorMessage: true
        })
        const result = normalizeResult(response.data.data)
        if (currentItem && result) {
          Object.assign(currentItem, result, { status_message: result.ok ? '成功' : '失败' })
        }
      } catch (error) {
        if (currentItem) {
          currentItem.ok = false
          currentItem.error = error.response?.data?.error || error.message || '测试失败'
          currentItem.status_message = '失败'
        }
      }
    }
  } catch (error) {
    networkResults.value = []
    message.error('加载网络测试结果失败')
  } finally {
    if (currentRun === runId) {
      networkLoading.value = false
    }
  }
}

const getNetworkStatusText = (row) => {
  if (row.ok === true) return '成功'
  if (row.ok === false) return '失败'
  return row.status_message || '待测试'
}

const getNetworkStatusTag = (row) => {
  if (row.ok === true) return 'success'
  if (row.ok === false) return 'error'
  return 'info'
}

loadNetworkResults()
</script>
