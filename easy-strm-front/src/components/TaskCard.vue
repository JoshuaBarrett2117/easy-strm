<template>
  <el-card v-if="task" class="task-card" shadow="hover" :class="['task-type-' + (task.task_type || 'unknown')]">
    <template #header>
      <div class="task-header">
        <div class="task-title">
          <el-icon class="task-icon" :class="{ 'spin': task.status === 'running' }">
            <component :is="taskStatusIcon" />
          </el-icon>
          <span class="task-name">{{ taskDisplayName }}</span>
          <el-tag :type="taskStatusType" size="small" class="status-tag">
            {{ taskStatusText }}
          </el-tag>
        </div>
        <span class="task-time">{{ task.create_time || '' }}</span>
      </div>
    </template>

    <div class="task-body">
      <div class="task-type-info">
        <el-tag :type="taskTypeTagType" size="default" effect="plain">
          {{ taskTypeName }}
        </el-tag>
        <span v-if="task.config_name" class="config-name">{{ task.config_name }}</span>
      </div>

      <el-progress
        v-if="showProgress"
        :percentage="task.progress || 0"
        :status="progressStatus"
        :stroke-width="14"
        class="task-progress"
        :format="progressFormat"
      />

      <div v-if="showFileStats" class="task-stats">
        <div class="stat-item">
          <div class="stat-value total">{{ task.total_files || 0 }}</div>
          <div class="stat-label">待生成文件</div>
        </div>
        <div class="stat-item">
          <div class="stat-value success">{{ task.success_files || 0 }}</div>
          <div class="stat-label">已生成</div>
        </div>
        <div class="stat-item">
          <div class="stat-value failed">{{ task.failed_files || 0 }}</div>
          <div class="stat-label">生成失败</div>
        </div>
        <div class="stat-item">
          <div class="stat-value pending">{{ pendingCount }}</div>
          <div class="stat-label">处理中</div>
        </div>
      </div>

      <div v-if="task.scheduled_time" class="scheduled-info">
        <el-icon><Clock /></el-icon>
        <span>计划执行时间: {{ task.scheduled_time }}</span>
      </div>

      <el-alert
        v-if="task.error_message"
        type="error"
        :title="task.error_message"
        show-icon
        :closable="false"
        class="task-error"
      />
    </div>
  </el-card>
</template>

<script setup>
import { computed, toRef } from 'vue'
import {
  Clock,
  Loading,
  CircleCheck,
  CircleClose,
  Timer,
  Document,
  Refresh
} from '@element-plus/icons-vue'

const props = defineProps({
  task: {
    type: Object,
    default: () => ({})
  }
})

const task = toRef(props, 'task')

const taskTypeNames = {
  'strm_generate': 'STRM文件生成',
  'incremental_sync': '增量同步',
  'log_clean': '日志清理',
  'sync_files': '文件同步'
}

const taskTypeIcons = {
  'strm_generate': Document,
  'incremental_sync': Timer,
  'log_clean': Refresh,
  'sync_files': Timer
}

const taskTypeName = computed(() => {
  const t = task.value
  if (!t) return '未知任务'
  return taskTypeNames[t.task_type] || '未知任务'
})

const taskDisplayName = computed(() => {
  const t = task.value
  if (!t) return '任务'
  if (t.task_name) {
    return t.task_name
  }
  if (t.task_type) {
    return taskTypeNames[t.task_type] || '未知任务'
  }
  return '任务'
})

const taskTypeTagType = computed(() => {
  const t = task.value
  if (!t) return 'info'
  const types = {
    'strm_generate': 'primary',
    'incremental_sync': 'success',
    'log_clean': 'warning',
    'sync_files': 'info'
  }
  return types[t.task_type] || 'info'
})

const taskStatusType = computed(() => {
  const t = task.value
  if (!t) return 'info'
  const types = {
    'pending': 'info',
    'running': 'warning',
    'completed': 'success',
    'failed': 'danger',
    'scheduled': ''
  }
  return types[t.status] || 'info'
})

const taskStatusText = computed(() => {
  const t = task.value
  if (!t) return '未知'
  const texts = {
    'pending': '待执行',
    'running': '执行中',
    'completed': '已完成',
    'failed': '失败',
    'scheduled': '已调度'
  }
  return texts[t.status] || '未知'
})

const taskStatusIcon = computed(() => {
  const t = task.value
  if (!t) return Timer
  if (t.status === 'running') return Loading
  if (t.status === 'completed') return CircleCheck
  if (t.status === 'failed') return CircleClose
  if (t.status === 'scheduled') return Clock
  return Timer
})

const showProgress = computed(() => {
  const t = task.value
  if (!t) return false
  const taskType = t.task_type || 'strm_generate'
  return ['running', 'completed', 'failed'].includes(t.status) &&
    taskType === 'strm_generate'
})

const showFileStats = computed(() => {
  const t = task.value
  if (!t) return false
  const taskType = t.task_type || 'strm_generate'
  return taskType === 'strm_generate' &&
    ((t.total_files > 0) || (t.success_files > 0) || (t.failed_files > 0))
})

const progressStatus = computed(() => {
  const t = task.value
  if (!t) return ''
  if (t.status === 'completed') return 'success'
  if (t.status === 'failed') return 'exception'
  return ''
})

const progressFormat = (percentage) => {
  const t = task.value
  if (!t) return ''
  if (t.status === 'completed') return '完成'
  if (t.status === 'running' && t.total_files === 0) return '准备中...'
  return `${percentage}%`
}

const pendingCount = computed(() => {
  const t = task.value
  if (!t) return 0
  const total = t.total_files || 0
  const success = t.success_files || 0
  const failed = t.failed_files || 0
  return Math.max(0, total - success - failed)
})
</script>

<style scoped>
.task-card {
  border-radius: 8px;
  margin-bottom: 12px;
  transition: all 0.3s ease;
}

.task-card:hover {
  transform: translateY(-2px);
}

.task-type-strm_generate {
  border-left: 4px solid #409eff;
}

.task-type-log_clean {
  border-left: 4px solid #e6a23c;
}

.task-type-sync_files {
  border-left: 4px solid #909399;
}

.task-type-incremental_sync {
  border-left: 4px solid #67c23a;
}

.task-type-unknown {
  border-left: 4px solid #909399;
}

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.task-title {
  display: flex;
  align-items: center;
  gap: 10px;
}

.task-icon {
  font-size: 18px;
}

.task-icon.spin {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.task-name {
  font-weight: 600;
  font-size: 15px;
  color: #303133;
}

.status-tag {
  margin-left: 8px;
}

.task-time {
  font-size: 12px;
  color: #909399;
}

.task-body {
  padding: 10px 0;
}

.task-type-info {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 15px;
}

.config-name {
  color: #606266;
  font-size: 14px;
}

.task-progress {
  margin-bottom: 15px;
}

.task-stats {
  display: flex;
  justify-content: space-around;
  padding: 15px 0;
  background: #f8f9fa;
  border-radius: 8px;
  margin-bottom: 15px;
}

.stat-item {
  text-align: center;
}

.stat-value {
  font-size: 20px;
  font-weight: 600;
  margin-bottom: 5px;
}

.stat-value.total {
  color: #409eff;
}

.stat-value.success {
  color: #67c23a;
}

.stat-value.failed {
  color: #f56c6c;
}

.stat-value.pending {
  color: #e6a23c;
}

.stat-label {
  font-size: 12px;
  color: #909399;
}

.scheduled-info {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #606266;
  font-size: 14px;
  margin-top: 10px;
}

.task-error {
  margin-top: 15px;
}
</style>
