<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <PageCard title="缓存管理" subtitle="Cache Center">
      <template #action>
        <n-button :loading="loading" @click="loadOverview">
          <template #icon>
            <n-icon :component="RefreshOutline" />
          </template>
          刷新概览
        </n-button>
        <n-button type="error" secondary :loading="clearingScope === 'all'" @click="handleClear('all', '全部缓存')">
          一键清理
        </n-button>
      </template>
      <p class="text-sm text-slate-400 dark:text-slate-500">
        集中查看识别、刮削和 Redis 热点缓存，并支持按类型手动清理。
      </p>
    </PageCard>

    <!-- 统计卡 -->
    <section class="grid grid-cols-2 gap-3 lg:grid-cols-4 lg:gap-4">
      <StatCard
        label="Redis 状态"
        :value="overview.redis_connected ? '已连接' : '未连接'"
        :icon="PulseOutline"
        :tone="overview.redis_connected ? 'green' : 'red'"
      />
      <StatCard label="Redis Key 总数" :value="formatCount(overview.redis_total_keys)" :icon="KeyOutline" tone="cyan" />
      <StatCard label="Redis 内存" :value="overview.redis_memory || '-'" :icon="HardwareChipOutline" tone="violet" />
      <StatCard label="已纳管缓存" :value="overview.groups.length" :icon="LayersOutline" tone="amber" />
    </section>

    <!-- 缓存分组 -->
    <PageCard>
      <div class="overflow-x-auto">
        <n-data-table
          :columns="cacheColumns"
          :data="overview.groups"
          :loading="loading"
          :row-key="(row) => row.key"
          :scroll-x="800"
        />
      </div>
    </PageCard>
  </div>
</template>

<script setup>
import { h, reactive, ref } from 'vue'
import { NButton, NDataTable, NIcon, NTag, useMessage } from 'naive-ui'
import {
  RefreshOutline,
  PulseOutline,
  KeyOutline,
  HardwareChipOutline,
  LayersOutline
} from '@vicons/ionicons5'
import PageCard from '../../components/common/PageCard.vue'
import StatCard from '../../components/common/StatCard.vue'
import { getCacheOverview, clearCacheGroup } from '../../utils/api/cache'
import { showConfirmDialog } from '../../utils/ui/messageBox'

const message = useMessage()

const loading = ref(false)
const clearingScope = ref('')
const overview = reactive({
  redis_connected: false,
  redis_total_keys: 0,
  redis_memory: '',
  groups: []
})

const cacheColumns = [
  { title: '缓存名称', key: 'name', minWidth: 160 },
  { title: '说明', key: 'description', minWidth: 260 },
  {
    title: '存储位置',
    key: 'storage',
    width: 120,
    render: (row) => h(
      NTag,
      { type: storageTagType(row.storage), size: 'small' },
      { default: () => storageLabel(row.storage) }
    )
  },
  {
    title: '条目数',
    key: 'count',
    width: 120,
    render: (row) => h('strong', { class: 'tabular-nums' }, formatCount(row.count))
  },
  {
    title: '操作',
    key: 'actions',
    width: 150,
    render: (row) => h(
      NButton,
      {
        type: 'error',
        text: true,
        size: 'small',
        disabled: row.count <= 0,
        loading: clearingScope.value === row.key,
        onClick: () => handleClear(row.key, row.name)
      },
      { default: () => '清理' }
    )
  }
]

const formatCount = (value) => {
  const parsed = Number(value || 0)
  return Number.isFinite(parsed) ? parsed.toLocaleString('zh-CN') : '0'
}

const storageLabel = (storage) => {
  return {
    redis: 'Redis',
    postgres: 'PostgreSQL',
    mixed: '混合'
  }[storage] || storage
}

const storageTagType = (storage) => {
  return {
    redis: 'error',
    postgres: 'success',
    mixed: 'warning'
  }[storage] || 'info'
}

const loadOverview = async () => {
  loading.value = true
  try {
    const response = await getCacheOverview()
    const payload = response?.data?.data || {}
    overview.redis_connected = Boolean(payload.redis_connected)
    overview.redis_total_keys = Number(payload.redis_total_keys || 0)
    overview.redis_memory = payload.redis_memory || ''
    overview.groups = Array.isArray(payload.groups) ? payload.groups : []
  } catch (error) {
    message.error('加载缓存概览失败')
  } finally {
    loading.value = false
  }
}

const handleClear = async (scope, label) => {
  try {
    await showConfirmDialog(`确认清理${label}吗？此操作会直接删除对应缓存内容。`, '清理缓存', {
      confirmButtonText: '立即清理',
      cancelButtonText: '取消',
      type: 'warning'
    })
  } catch (error) {
    return
  }

  clearingScope.value = scope
  try {
    const response = await clearCacheGroup(scope)
    const deletedCount = response?.data?.data?.deleted_count ?? 0
    message.success(`缓存清理完成，共删除 ${formatCount(deletedCount)} 项`)
    await loadOverview()
  } catch (error) {
    message.error(error?.response?.data?.error || '清理缓存失败')
  } finally {
    clearingScope.value = ''
  }
}

loadOverview()
</script>
