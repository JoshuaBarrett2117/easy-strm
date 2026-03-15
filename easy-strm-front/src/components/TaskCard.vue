<template>
  <el-card class="task-card" shadow="hover" :class="['task-type-' + task.task_type]">
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
        <span class="task-time">{{ task.create_time }}</span>
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
import { computed } from 'vue'
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
    required: true
  }
})

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
  return taskTypeNames[props.task.task_type] || '未知任务'
})

const taskDisplayName = computed(() => {
  if (props.task.task_name) {
    return props.task.task_name
  }
  if (props.task.task_type) {
    return taskTypeNames[props.task.task_type] || '未知任务'
  }
  return '任务'
})

const taskTypeTagType = computed(() => {
  const types = {
    'strm_generate': 'primary',
    'incremental_sync': 'success',
    'log_clean': 'warning',
    'sync_files': 'info'
  }
  return types[props.task.task_type] || 'info'
})

const taskStatusType = computed(() => {
  const types = {
    'pending': 'info',
    'running': 'warning',
    'completed': 'success',
    'failed': 'danger',
    'scheduled': ''
  }
  return types[props.task.status] || 'info'
})

const taskStatusText = computed(() => {
  const texts = {
    'pending': '待执行',
    'running': '执行中',
    'completed': '已完成',
    'failed': '失败',
    'scheduled': '已调度'
  }
  return texts[props.task.status] || '未知'
})

const taskStatusIcon = computed(() => {
  if (props.task.status === 'running') return Loading
  if (props.task.status === 'completed') return CircleCheck
  if (props.task.status === 'failed') return CircleClose
  if (props.task.status === 'scheduled') return Clock
  return Timer
})

const showProgress = computed(() => {
  const taskType = props.task.task_type || 'strm_generate'
  return ['running', 'completed', 'failed'].includes(props.task.status) &&
    taskType === 'strm_generate'
})

const showFileStats = computed(() => {
  const taskType = props.task.task_type || 'strm_generate'
  return taskType === 'strm_generate' &&
    (props.task.total_files > 0 || props.task.success_files > 0 || props.task.failed_files > 0)
})

const progressStatus = computed(() => {
  if (props.task.status === 'completed') return 'success'
  if (props.task.status === 'failed') return 'exception'
  return ''
})

const pendingCount = computed(() => {
  const total = props.task.total_files || 0
  const success = props.task.success_files || 0
  const failed = props.task.failed_files || 0
  return Math.max(0, total - success - failed)
})

const progressFormat = (percentage) => {
  if (props.task.status === 'completed') return '完成'
  if (props.task.status === 'running' && props.task.total_files === 0) return '准备中...'
  return `${percentage}%`
}
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

.task-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.task-title {
  display: flex;
  align-items: center;
  gap: 8px;
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
  font-size: 14px;
}

.status-tag {
  margin-left: 8px;
}

.task-time {
  font-size: 12px;
  color: #909399;
}

.task-body {
  padding-top: 10px;
}

.task-type-info {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 12px;
}

.config-name {
  font-size: 13px;
  color: #606266;
}

.task-progress {
  margin-bottom: 15px;
}

.task-stats {
  display: grid;
  grid-template-columns: repeat(4, 1fr);
  gap: 10px;
  padding: 12px;
  background-color: #f5f7fa;
  border-radius: 6px;
}

.stat-item {
  text-align: center;
}

.stat-value {
  font-size: 18px;
  font-weight: 600;
  color: #303133;
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
  margin-top: 4px;
}

.scheduled-info {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  background-color: #fdf6ec;
  border-radius: 4px;
  color: #e6a23c;
  font-size: 13px;
  margin-bottom: 10px;
}

.task-error {
  margin-top: 10px;
}
</style>
