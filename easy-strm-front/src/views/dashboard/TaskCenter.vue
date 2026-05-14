<template>
  <div class="task-center">
    <section class="page-hero">
      <div>
        <div class="page-kicker">Task Center</div>
        <h2>统一任务队列</h2>
        <p>保留现有任务取消、恢复、详情与失败文件定位能力，并用正式页面承载。</p>
      </div>
      <div class="page-actions">
        <el-button :icon="Refresh" @click="loadTasks" :loading="taskLoading">刷新</el-button>
        <el-switch v-model="autoRefresh" active-text="自动刷新" />
      </div>
    </section>

    <section class="task-summary-grid">
      <div class="summary-card">
        <span>总任务</span>
        <strong>{{ taskList.length }}</strong>
      </div>
      <div class="summary-card">
        <span>执行中</span>
        <strong>{{ taskCounts.running }}</strong>
      </div>
      <div class="summary-card">
        <span>失败</span>
        <strong>{{ taskCounts.failed }}</strong>
      </div>
      <div class="summary-card">
        <span>已完成</span>
        <strong>{{ taskCounts.completed }}</strong>
      </div>
    </section>

    <section class="task-list-panel">
      <div v-if="taskList.length > 0" class="task-items">
        <TaskCard
          v-for="task in taskList"
          :key="task.task_id"
          :task="task"
          @cancel="handleCancelTask"
          @resume="handleResumeTask"
          @detail="handleTaskDetail"
        />
      </div>
      <el-empty v-else description="暂无任务记录" />
    </section>

    <el-drawer
      v-model="taskDetailVisible"
      title="任务详情"
      size="42%"
      :destroy-on-close="true"
      append-to-body
    >
      <div class="task-detail-drawer" v-loading="taskDetailLoading">
        <el-alert
          v-if="taskDetailError"
          type="error"
          :title="taskDetailError"
          show-icon
          :closable="false"
          class="task-detail-error"
        />

        <template v-if="taskDetailData">
          <el-descriptions :column="1" border class="task-detail-summary">
            <el-descriptions-item label="任务ID">{{ taskDetailData.task_id || '-' }}</el-descriptions-item>
            <el-descriptions-item label="任务名称">{{ taskDetailData.task_name || '-' }}</el-descriptions-item>
            <el-descriptions-item label="任务类型">{{ taskDetailData.task_type || '-' }}</el-descriptions-item>
            <el-descriptions-item label="状态">{{ taskDetailData.status || '-' }}</el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ taskDetailData.create_time || '-' }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ taskDetailData.update_time || '-' }}</el-descriptions-item>
            <el-descriptions-item label="进度">{{ taskDetailData.progress ?? 0 }}%</el-descriptions-item>
            <el-descriptions-item label="文件统计">
              成功 {{ taskDetailData.success_files || 0 }} / 总数 {{ taskDetailData.total_files || 0 }} / 失败 {{ taskDetailData.failed_files || 0 }}
            </el-descriptions-item>
            <el-descriptions-item label="错误信息">{{ taskDetailData.error_message || '-' }}</el-descriptions-item>
          </el-descriptions>

          <div class="task-detail-section">
            <div class="task-detail-section-title">执行步骤</div>
            <el-timeline v-if="taskDetailSteps.length > 0" class="task-detail-steps">
              <el-timeline-item
                v-for="step in taskDetailSteps"
                :key="step.step_key"
                :type="taskStepTimelineType(step.status)"
                :timestamp="step.finished_at || step.started_at || step.updated_at || ''"
              >
                <div class="task-step-title">{{ step.step_name || step.step_key }}</div>
                <div class="task-step-meta">
                  <el-tag size="small" :type="taskStepTagType(step.status)">{{ taskStepStatusLabel(step.status) }}</el-tag>
                  <span v-if="step.output_summary">{{ step.output_summary }}</span>
                  <span v-else-if="step.input_summary">{{ step.input_summary }}</span>
                </div>
                <div v-if="step.error_message" class="task-step-error">{{ step.error_message }}</div>
              </el-timeline-item>
            </el-timeline>
            <el-empty v-else description="暂无步骤记录" />
          </div>

          <div class="task-detail-section">
            <div class="task-detail-section-title">任务元数据</div>
            <div v-if="taskDetailMetadataRows.length > 0" class="task-detail-meta-grid">
              <div v-for="item in taskDetailMetadataRows" :key="item.key" class="task-detail-meta-item">
                <div class="task-detail-meta-label">{{ item.label }}</div>
                <div class="task-detail-meta-value">{{ item.value }}</div>
              </div>
            </div>
            <el-empty v-else description="暂无任务元数据" />
          </div>

          <div v-if="taskDetailFailedItems.length > 0" class="task-detail-section">
            <div class="task-detail-section-title">失败文件</div>
            <div v-if="taskDetailFailureGroups.length > 0" class="task-detail-failure-summary">
              <div v-for="group in taskDetailFailureGroups" :key="group.key" class="task-detail-failure-group">
                <div class="task-detail-failure-group-header">
                  <div class="task-detail-failure-group-title">{{ group.label }}</div>
                  <el-tag :type="group.tagType" size="small">{{ group.items.length }}</el-tag>
                </div>
                <div class="task-detail-failure-group-reason">{{ group.reason || '未提供失败原因' }}</div>
              </div>
            </div>
            <div class="task-detail-failed-list">
              <div
                v-for="item in taskDetailFailedItems"
                :key="`${item.file_id || item.file_name}-${item.reason}`"
                class="task-detail-failed-item"
              >
                <div class="task-detail-failed-main">
                  <span class="task-detail-failed-name">{{ item.file_name }}</span>
                  <span v-if="item.file_id" class="task-detail-failed-id">{{ item.file_id }}</span>
                  <el-tag v-if="item.category" :type="taskDetailFailureTagType(item.category)" size="small">
                    {{ taskDetailFailureTagLabel(item.category) }}
                  </el-tag>
                </div>
                <div class="task-detail-failed-reason">{{ item.reason || '未提供失败原因' }}</div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </el-drawer>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import TaskCard from '../../components/TaskCard.vue'
import { cancelTask, getTaskDetail, getUnifiedTaskList, resumeTask } from '../../utils/api/task'

const route = useRoute()

const taskLoading = ref(false)
const taskList = ref([])
const autoRefresh = ref(false)
const taskDetailVisible = ref(false)
const taskDetailLoading = ref(false)
const taskDetailData = ref(null)
const taskDetailError = ref('')
const activeTaskId = ref('')
let timer = null
let taskDetailTimer = null

const taskCounts = computed(() => {
  return taskList.value.reduce((acc, item) => {
    const key = item.status || 'other'
    acc[key] = (acc[key] || 0) + 1
    return acc
  }, { running: 0, failed: 0, completed: 0 })
})

const taskDetailMetadataRows = computed(() => {
  const metadata = taskDetailData.value?.metadata
  if (!metadata || typeof metadata !== 'object' || Array.isArray(metadata)) {
    return []
  }

  const labelMap = {
    source_id: '来源ID',
    source_name: '来源媒体源',
    source_type: '来源类型',
    source_path: '来源路径',
    organize_target_path: '整理目标',
    media_type: '媒体类型',
    conflict_policy: '冲突策略',
    operation_mode: '整理方式',
    watch_interval: '监控间隔',
    trigger_mode: '触发方式',
    detected_files: '检测文件数',
    success_files: '成功数',
    failed_files: '失败数',
    skipped_files: '跳过数',
    result_summary: '结果概览',
    failure_category: '失败分类',
    failure_reason: '失败原因',
    failed_item_count: '失败文件数'
  }

  return Object.entries(metadata)
    .filter(([key, value]) => key !== 'failed_items' && value !== null && value !== undefined && value !== '')
    .map(([key, value]) => {
      let formatted = value
      if (key === 'trigger_mode') {
        formatted = value === 'polling' ? '115 轮询监控' : value === 'fsnotify' ? '本地实时监控' : value
      } else if (key === 'media_type') {
        formatted = ({ all: '全部', movie: '电影', tv: '剧集' })[value] || value
      } else if (key === 'conflict_policy') {
        formatted = ({ skip: '跳过', overwrite: '覆盖', suffix: '追加序号' })[value] || value
      } else if (key === 'operation_mode') {
        formatted = ({ move: '移动', copy: '复制', hardlink: '硬链接', symlink: '软链接' })[value] || value
      } else if (key === 'watch_interval' && Number.isFinite(Number(value))) {
        formatted = `${value} 秒`
      } else if (Array.isArray(value)) {
        formatted = `${value.length} 项`
      } else if (typeof value === 'object') {
        formatted = JSON.stringify(value)
      }

      return {
        key,
        label: labelMap[key] || key,
        value: String(formatted)
      }
    })
})

const taskDetailSteps = computed(() => {
  const steps = taskDetailData.value?.steps
  return Array.isArray(steps) ? steps : []
})

const taskStepStatusLabel = (status) => {
  const map = {
    pending: '等待中',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    cancelled: '已取消',
    skipped: '已跳过'
  }
  return map[status] || status || '-'
}

const taskStepTagType = (status) => {
  if (status === 'completed') return 'success'
  if (status === 'running') return 'primary'
  if (status === 'failed') return 'danger'
  if (status === 'skipped') return 'info'
  return 'warning'
}

const taskStepTimelineType = (status) => {
  if (status === 'completed') return 'success'
  if (status === 'running') return 'primary'
  if (status === 'failed') return 'danger'
  return 'info'
}

const taskDetailFailedItems = computed(() => {
  const items = taskDetailData.value?.metadata?.failed_items
  if (!Array.isArray(items)) {
    return []
  }

  return items
    .map(item => ({
      file_id: item?.file_id || item?.fileID || '',
      file_name: item?.file_name || item?.fileName || item?.file_id || '',
      category: item?.category || '',
      reason: item?.reason || ''
    }))
    .filter(item => item.file_id || item.file_name)
})

const taskDetailFailureGroups = computed(() => {
  const items = taskDetailFailedItems.value
  if (items.length === 0) {
    return []
  }

  const groupOrder = ['identify_failed', 'organize_failed', 'cloud115_auth_failed', 'cloud115_failed', 'scan_failed', 'target_path', 'partial_failed', 'conflict_skipped', 'panic', 'other']
  const groupMap = new Map()

  const getGroupKey = (category, reason) => {
    const normalizedCategory = String(category || '').trim()
    if (normalizedCategory) return normalizedCategory
    const text = String(reason || '').toLowerCase()
    if (text.includes('identify') || text.includes('tmdb') || text.includes('recogniz')) return 'identify_failed'
    if (text.includes('organize') || text.includes('move') || text.includes('copy')) return 'organize_failed'
    if (text.includes('cookie') || text.includes('auth') || text.includes('unauthorized')) return 'cloud115_auth_failed'
    if (text.includes('cloud115') || text.includes('115')) return 'cloud115_failed'
    if (text.includes('scan') || text.includes('list')) return 'scan_failed'
    if (text.includes('target') && text.includes('path')) return 'target_path'
    if (text.includes('partial') || text.includes('部分')) return 'partial_failed'
    if (text.includes('conflict') || text.includes('冲突')) return 'conflict_skipped'
    if (text.includes('panic') || text.includes('exception')) return 'panic'
    return 'other'
  }

  const getGroupMeta = (key) => {
    const meta = {
      identify_failed: { label: '识别失败', tagType: 'warning' },
      organize_failed: { label: '整理失败', tagType: 'danger' },
      cloud115_auth_failed: { label: '115 账号失效', tagType: 'danger' },
      cloud115_failed: { label: '115 操作失败', tagType: 'danger' },
      scan_failed: { label: '扫描失败', tagType: 'warning' },
      target_path: { label: '目标路径异常', tagType: 'warning' },
      partial_failed: { label: '部分失败', tagType: 'warning' },
      conflict_skipped: { label: '冲突跳过', tagType: 'warning' },
      panic: { label: '异常中断', tagType: 'danger' },
      other: { label: '其他失败', tagType: 'info' }
    }
    return meta[key] || meta.other
  }

  for (const item of items) {
    const key = getGroupKey(item.category, item.reason)
    if (!groupMap.has(key)) {
      const meta = getGroupMeta(key)
      groupMap.set(key, {
        key,
        label: meta.label,
        tagType: meta.tagType,
        reason: item.reason || '',
        items: []
      })
    }
    groupMap.get(key).items.push(item)
  }

  return groupOrder.filter(key => groupMap.has(key)).map(key => groupMap.get(key))
})

const taskDetailFailureTagLabel = (category) => {
  const map = {
    identify_failed: '识别失败',
    organize_failed: '整理失败',
    cloud115_auth_failed: '115 账号失效',
    cloud115_failed: '115 操作失败',
    scan_failed: '扫描失败',
    target_path: '目标路径异常',
    panic: '异常中断',
    partial_failed: '部分失败',
    conflict_skipped: '冲突跳过',
    other: '其他失败'
  }
  return map[String(category || '').trim()] || '其他失败'
}

const taskDetailFailureTagType = (category) => {
  const key = String(category || '').trim()
  if (key === 'identify_failed' || key === 'scan_failed' || key === 'target_path') return 'warning'
  if (key === 'organize_failed' || key === 'cloud115_auth_failed' || key === 'cloud115_failed' || key === 'panic') return 'danger'
  if (key === 'partial_failed') return 'warning'
  return 'info'
}

const stopAutoRefresh = () => {
  if (timer) {
    clearInterval(timer)
    timer = null
  }
}

const stopTaskDetailRefresh = () => {
  if (taskDetailTimer) {
    clearInterval(taskDetailTimer)
    taskDetailTimer = null
  }
}

const startAutoRefresh = () => {
  stopAutoRefresh()
  timer = setInterval(() => {
    loadTasks()
  }, 3000)
}

const startTaskDetailRefresh = () => {
  stopTaskDetailRefresh()
  taskDetailTimer = setInterval(() => {
    if (!taskDetailVisible.value || !activeTaskId.value) return
    loadTaskDetail(activeTaskId.value, false)
  }, 2000)
}

const syncTaskInList = (taskDetail) => {
  if (!taskDetail?.task_id) return
  const index = taskList.value.findIndex(item => item.task_id === taskDetail.task_id)
  if (index === -1) return
  taskList.value[index] = {
    ...taskList.value[index],
    ...taskDetail
  }
}

const loadTasks = async () => {
  taskLoading.value = true
  try {
    const response = await getUnifiedTaskList()
    const payload = response.data?.data?.data || response.data?.data || response.data || []
    taskList.value = Array.isArray(payload) ? payload : []
  } catch (error) {
    ElMessage.error('加载任务列表失败')
  } finally {
    taskLoading.value = false
  }
}

const loadTaskDetail = async (taskId, showLoading = true) => {
  activeTaskId.value = taskId
  if (showLoading) {
    taskDetailLoading.value = true
  }
  try {
    const response = await getTaskDetail(taskId)
    const payload = response.data.data || response.data || null
    taskDetailData.value = payload
    syncTaskInList(payload)
    const status = payload?.status
    if (taskDetailVisible.value && (status === 'pending' || status === 'running')) {
      startTaskDetailRefresh()
    } else {
      stopTaskDetailRefresh()
    }
  } catch (error) {
    taskDetailError.value = error.response?.data?.error || error.message || '加载任务详情失败'
    stopTaskDetailRefresh()
  } finally {
    if (showLoading) {
      taskDetailLoading.value = false
    }
  }
}

const handleTaskDetail = async (taskId) => {
  taskDetailVisible.value = true
  taskDetailError.value = ''
  taskDetailData.value = null
  await loadTaskDetail(taskId, true)
}

const handleCancelTask = async (taskId) => {
  try {
    await cancelTask(taskId)
    ElMessage.success('任务已取消')
    await loadTasks()
  } catch (error) {
    ElMessage.error('取消任务失败')
  }
}

const handleResumeTask = async (taskId) => {
  try {
    await resumeTask(taskId)
    ElMessage.success('任务已重新执行')
    await loadTasks()
  } catch (error) {
    ElMessage.error('恢复任务失败')
  }
}

const openRouteTask = async () => {
  const taskId = route.query.task_id
  if (taskId) {
    await handleTaskDetail(String(taskId))
  }
}

watch(autoRefresh, (value) => {
  if (value) startAutoRefresh()
  else stopAutoRefresh()
})

watch(taskDetailVisible, (value) => {
  if (!value) {
    stopTaskDetailRefresh()
    activeTaskId.value = ''
    taskDetailError.value = ''
    taskDetailData.value = null
  }
})

watch(() => route.query.task_id, async (taskId) => {
  if (taskId) {
    await handleTaskDetail(String(taskId))
  }
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  stopTaskDetailRefresh()
})

onMounted(async () => {
  await loadTasks()
  await openRouteTask()
})
</script>

<style scoped>
.task-center {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.page-hero,
.task-summary-grid,
.task-list-panel {
  background: rgba(255, 252, 247, 0.84);
  border: 1px solid rgba(120, 101, 72, 0.12);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
  border-radius: 24px;
}

:global(.dark) .page-hero,
:global(.dark) .task-summary-grid,
:global(.dark) .task-list-panel {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

.page-hero {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 18px;
  padding: 24px;
}

.page-kicker {
  color: #1f6f78;
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.16em;
  text-transform: uppercase;
}

.page-hero h2 {
  margin: 10px 0 6px;
  font-size: 32px;
}

.page-hero p {
  color: #6f6457;
}

.page-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.task-summary-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 1px;
  padding: 1px;
}

.summary-card {
  padding: 18px 22px;
}

.summary-card span {
  display: block;
  color: #6f6457;
  font-size: 13px;
}

.summary-card strong {
  display: block;
  margin-top: 10px;
  font-size: 34px;
}

.task-list-panel {
  padding: 20px;
}

.task-items {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.task-detail-drawer {
  padding-right: 8px;
}

.task-detail-error,
.task-detail-summary,
.task-detail-section {
  margin-bottom: 16px;
}

.task-detail-section-title {
  margin-bottom: 12px;
  font-size: 14px;
  font-weight: 600;
}

.task-detail-meta-grid,
.task-detail-failure-summary {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 10px;
}

.task-detail-meta-item,
.task-detail-failure-group,
.task-detail-failed-item {
  padding: 12px 14px;
  border-radius: 12px;
  border: 1px solid #ebeef5;
  background: #f7f8fa;
}

.task-detail-meta-label {
  margin-bottom: 4px;
  color: #909399;
  font-size: 12px;
}

.task-detail-meta-value,
.task-detail-failure-group-reason,
.task-detail-failed-reason {
  line-height: 1.6;
  word-break: break-word;
}

.task-detail-failed-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.task-detail-failed-main,
.task-detail-failure-group-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 8px;
  flex-wrap: wrap;
}

.task-detail-failed-name {
  font-weight: 700;
}

.task-detail-failed-id {
  color: #909399;
  font-size: 12px;
}

.task-detail-steps {
  padding-left: 4px;
}

.task-step-title {
  margin-bottom: 8px;
  font-weight: 700;
}

.task-step-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: #606266;
}

.task-step-error {
  margin-top: 8px;
  color: #c45656;
  line-height: 1.6;
}

@media (max-width: 860px) {
  .page-hero {
    flex-direction: column;
    align-items: stretch;
  }

  .task-summary-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}
</style>
