<template>
  <section class="notification-task-scope mb-8 max-w-2xl" aria-label="任务通知范围">
    <h2 class="mb-3 text-base font-bold text-slate-800 dark:text-white">任务通知范围</h2>
    <p class="mb-4 text-sm text-slate-500 dark:text-slate-400">仅勾选的任务会推送通知；未勾选的任务静默执行</p>
    <n-alert v-if="loadError" type="error" class="mb-3">{{ loadError }}</n-alert>
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <n-button size="small" :disabled="!ready || saving" @click="selectAll">全选</n-button>
      <n-button size="small" :disabled="!ready || saving" @click="selected = []">全不选</n-button>
      <n-button size="small" :disabled="!ready || saving" @click="invertSelection">反选</n-button>
      <n-button size="small" :loading="loading" :disabled="saving" @click="loadScope">刷新任务</n-button>
      <span class="text-xs text-slate-500">已选 {{ selectedTaskCount }} / {{ tasks.length }}</span>
    </div>
    <n-spin :show="loading">
      <n-checkbox-group v-model:value="selected" :disabled="!ready || saving">
        <div class="notification-task-list">
          <n-checkbox v-for="(task, index) in tasks" :key="`${task.task_key}:${index}`" :value="task.task_key" :disabled="!task.task_key">
            <span class="notification-task-label">
              <span class="font-medium">{{ task.task_name }}</span>
              <span class="text-xs text-slate-500">{{ task.handler || '无处理器' }}</span>
              <span class="text-xs text-slate-500">{{ statusLabel(task.status) }}</span>
            </span>
          </n-checkbox>
          <p v-if="ready && !tasks.length" class="text-sm text-slate-500">暂无调度任务</p>
        </div>
      </n-checkbox-group>
    </n-spin>
    <n-button class="mt-4" type="primary" :loading="saving" :disabled="!ready || loading" @click="saveScope">保存任务范围</n-button>
  </section>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { NAlert, NButton, NCheckbox, NCheckboxGroup, NSpin, useMessage } from 'naive-ui'
import { getNotificationTaskScope, updateNotificationTaskScope } from '../../utils/api/notification'

const message = useMessage()
const tasks = ref([])
const selected = ref([])
const loading = ref(false)
const saving = ref(false)
const ready = ref(false)
const loadError = ref('')

const selectedTaskCount = computed(() => {
  const keys = new Set(selected.value)
  return tasks.value.filter(task => keys.has(task.task_key)).length
})

const statusLabel = status => ({ enabled: '已启用', disabled: '已停用' }[status] || status || '未知状态')

const selectAll = () => {
  selected.value = [...new Set(tasks.value.map(task => task.task_key).filter(Boolean))]
}

const invertSelection = () => {
  const previous = new Set(selected.value)
  selected.value = [...new Set(tasks.value.map(task => task.task_key).filter(key => key && !previous.has(key)))]
}

const loadScope = async () => {
  if (loading.value || saving.value) return
  loading.value = true
  ready.value = false
  loadError.value = ''
  try {
    const response = await getNotificationTaskScope({ skipGlobalErrorMessage: true })
    tasks.value = response.data.data.tasks
    selected.value = response.data.data.selected
    ready.value = true
  } catch (error) {
    loadError.value = error.response?.data?.error || '获取任务通知范围失败'
    message.error(loadError.value)
  } finally {
    loading.value = false
  }
}

const saveScope = async () => {
  if (!ready.value || loading.value || saving.value) return
  saving.value = true
  try {
    const response = await updateNotificationTaskScope({ selected: selected.value }, { skipGlobalErrorMessage: true })
    selected.value = response.data.data.selected
    message.success('任务通知范围已保存')
  } catch (error) {
    message.error(error.response?.data?.error || '保存任务通知范围失败')
  } finally {
    saving.value = false
  }
}

onMounted(loadScope)
</script>

<style scoped>
.notification-task-scope {
  min-width: 0;
}

.notification-task-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.notification-task-label {
  display: flex;
  flex-wrap: wrap;
  gap: 4px 12px;
  overflow-wrap: anywhere;
  word-break: break-word;
}

.notification-task-list :deep(.n-checkbox__label) {
  min-width: 0;
}
</style>
