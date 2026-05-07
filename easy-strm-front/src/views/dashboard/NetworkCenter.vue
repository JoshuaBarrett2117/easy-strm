<template>
  <div class="network-center">
    <section class="network-hero">
      <div>
        <div class="page-kicker">Network Probe</div>
        <h2>网络连通性测试</h2>
        <p>直接复用现有 `/api/network/test` 能力，对 Telegram、GitHub、TMDB 等关键站点进行逐一探测。</p>
      </div>
      <el-button :icon="Refresh" @click="loadNetworkResults" :loading="networkLoading">重新探测</el-button>
    </section>

    <section class="network-panel">
      <el-table :data="networkResults" v-loading="networkLoading" stripe border>
        <el-table-column prop="name" label="站点" min-width="130" />
        <el-table-column prop="url" label="地址" min-width="240" />
        <el-table-column label="连通状态" width="110">
          <template #default="{ row }">
            <el-tag :type="getNetworkStatusTag(row)">{{ getNetworkStatusText(row) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="代理路径" width="120">
          <template #default="{ row }">
            <el-tag v-if="row.via_proxy !== null && row.via_proxy !== undefined" :type="row.via_proxy ? 'warning' : 'info'">
              {{ row.via_proxy ? '代理' : '直连' }}
            </el-tag>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column prop="status_code" label="HTTP" width="90" />
        <el-table-column prop="duration_ms" label="耗时(ms)" width="110" />
        <el-table-column label="错误信息" min-width="220">
          <template #default="{ row }">
            {{ row.error || row.status_message || '-' }}
          </template>
        </el-table-column>
      </el-table>
    </section>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import { getNetworkProbeSites, testNetworkConnectivity } from '../../utils/api/setting'

const networkLoading = ref(false)
const networkResults = ref([])
let runId = 0

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
    ElMessage.error('加载网络测试结果失败')
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
  if (row.ok === false) return 'danger'
  return 'info'
}

loadNetworkResults()
</script>

<style scoped>
.network-center {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.network-hero,
.network-panel {
  padding: 24px;
  border-radius: 24px;
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

:global(.dark) .network-hero,
:global(.dark) .network-panel {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

.network-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}

.page-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.network-hero h2 {
  margin: 10px 0 6px;
  font-size: 30px;
}

.network-hero p {
  color: #6f6457;
}

@media (max-width: 860px) {
  .network-hero {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
