<template>
  <PageCard title="Emby 定时任务" subtitle="查看 Emby 已配置的任务与触发规则；立即触发记录可在任务中心查看">
    <template #action><n-button :loading="loading" @click="loadTasks">刷新</n-button></template>
    <n-alert v-if="error" type="error" class="mb-3">{{ error }}</n-alert>
    <p class="mb-3 text-sm text-slate-400">执行状态每 5 秒刷新。任务中心记录请求是否提交成功，实际执行结果以此处 Emby 返回结果为准。</p>
    <EmptyState v-if="!loading && !tasks.length && !error" title="暂无定时任务" />
    <div class="space-y-3">
      <div v-for="task in tasks" :key="task.Id" class="rounded-xl border border-white/10 p-4">
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div><strong>{{ task.Name }}</strong><span class="ml-2 text-sm text-slate-400">{{ task.Category }}</span></div>
          <div class="flex items-center gap-2">
            <n-tag :type="task.State === 'Running' ? 'info' : 'default'">{{ stateLabel(task.State) }}</n-tag>
            <n-button size="small" @click="editTriggers(task)">编辑触发规则</n-button>
            <n-button size="small" type="primary" :loading="pending.has(task.Id)" :disabled="task.State !== 'Idle' || pending.has(task.Id)" @click="runTask(task)">立即触发</n-button>
          </div>
        </div>
        <p class="mt-2 text-sm text-slate-400">{{ task.Description }}</p>
        <n-progress v-if="task.State === 'Running' && task.CurrentProgressPercentage != null" class="mt-3" type="line" :percentage="Math.min(100, Math.max(0, Math.round(task.CurrentProgressPercentage)))" />
        <p class="mt-3 text-sm">触发规则：{{ (task.Triggers || []).map(triggerLabel).join('；') || '未配置自动触发' }}</p>
        <p class="mt-2 text-sm">最近结果：{{ resultLabel(task.LastExecutionResult?.Status) }} · {{ formatTime(task.LastExecutionResult?.EndTimeUtc) }}</p>
        <p v-if="task.LastExecutionResult?.ErrorMessage" class="mt-2 text-sm text-red-400">{{ task.LastExecutionResult.ErrorMessage }}</p>
      </div>
    </div>
  </PageCard>
  <n-modal :show="!!editing" preset="card" title="编辑触发规则" style="width: min(640px, 94vw)" :mask-closable="!saving" :closable="!saving" @update:show="value => { if (!value) closeEditor() }">
    <p class="mb-3">{{ editing?.Name }} · 时间以 Emby 服务器时区为准。删除全部规则后仅保留手动触发，当前执行不受影响。</p>
    <div v-for="(rule, index) in draft" :key="index" class="rounded-xl border border-white/10 p-3 mb-3">
      <div class="flex flex-wrap gap-3 items-center">
        <n-select aria-label="触发类型" style="width: 150px" :value="rule.Type" :options="triggerTypes" :disabled="saving" @update:value="value => changeType(rule, value)" />
        <n-select v-if="rule.Type === 'WeeklyTrigger'" aria-label="星期" style="width: 120px" v-model:value="rule.DayOfWeek" :options="weekdays" :disabled="saving" />
        <n-time-picker v-if="['DailyTrigger', 'WeeklyTrigger'].includes(rule.Type)" aria-label="触发时间" format="HH:mm:ss" :formatted-value="ticksTime(rule.TimeOfDayTicks)" :disabled="saving" :clearable="false" @update:formatted-value="value => setRuleTime(rule, value)" />
        <template v-if="rule.Type === 'IntervalTrigger'">
          <n-input-number aria-label="间隔分钟" style="width: 160px" :value="rule.IntervalTicks / 600000000" :min="1" :max="15011998" :disabled="saving" @update:value="value => rule.IntervalTicks = value == null ? null : Math.round(value * 600000000)" />
          <span>分钟</span>
        </template>
        <n-button type="error" secondary :disabled="saving" @click="draft.splice(index, 1)">删除规则</n-button>
      </div>
    </div>
    <n-alert v-if="!draft.length" type="warning" class="mb-3">保存后此任务将不再自动触发。</n-alert>
    <n-button :disabled="saving" @click="draft.push({ Type: 'DailyTrigger', TimeOfDayTicks: 108000000000 })">添加规则</n-button>
    <template #footer>
      <div class="flex justify-end gap-3">
        <n-button :disabled="saving" @click="closeEditor">取消</n-button>
        <n-button type="primary" :loading="saving" @click="saveTriggers">保存规则</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { onBeforeUnmount, ref, watch } from 'vue'
import { NAlert, NButton, NProgress, NTag, NModal, NSelect, NTimePicker, NInputNumber, useMessage } from 'naive-ui'
import PageCard from './common/PageCard.vue'
import EmptyState from './common/EmptyState.vue'
import { getEmbyScheduledTasks, startEmbyScheduledTask, updateEmbyScheduledTaskTriggers } from '../utils/api/emby'

const props = defineProps({ serverId: Number, active: Boolean })
const emit = defineEmits(['submitted'])
const message = useMessage()
const tasks = ref([])
const loading = ref(false)
const error = ref('')
const pending = ref(new Set())
const editing = ref(null)
const draft = ref([])
const saving = ref(false)
let editorVersion = 0
const triggerTypes = [
  { label: '每天', value: 'DailyTrigger' }, { label: '每周', value: 'WeeklyTrigger' },
  { label: '固定间隔', value: 'IntervalTrigger' }, { label: 'Emby 启动时', value: 'StartupTrigger' }
]
const weekdays = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday'].map((value, i) => ({ value, label: ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][i] }))
const ticksTime = ticks => {
  const seconds = Math.floor(Number(ticks || 0) / 10000000)
  return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60].map(n => String(n).padStart(2, '0')).join(':')
}
const setRuleTime = (rule, value) => {
  if (!value) return
  const [h, m, s = 0] = value.split(':').map(Number)
  rule.TimeOfDayTicks = (h * 3600 + m * 60 + s) * 10000000
}
const changeType = (rule, type) => {
  rule.Type = type
  delete rule.TimeOfDayTicks
  delete rule.DayOfWeek
  delete rule.IntervalTicks
  if (type === 'DailyTrigger' || type === 'WeeklyTrigger') rule.TimeOfDayTicks = 108000000000
  if (type === 'WeeklyTrigger') rule.DayOfWeek = 'Monday'
  if (type === 'IntervalTrigger') rule.IntervalTicks = 36000000000
}
const closeEditor = () => { editorVersion++; editing.value = null; draft.value = []; saving.value = false }
const editTriggers = task => {
  editorVersion++
  editing.value = { Id: task.Id, Name: task.Name, serverId: props.serverId }
  draft.value = JSON.parse(JSON.stringify(task.Triggers || []))
}
const saveTriggers = async () => {
  if (saving.value || !editing.value) return
  const target = editing.value
  const version = editorVersion
  saving.value = true
  try {
    await updateEmbyScheduledTaskTriggers(target.serverId, target.Id, draft.value)
    if (version !== editorVersion) return
    message.success('触发规则已保存')
    closeEditor()
    await loadTasks()
  } catch { /* API 封装统一展示操作错误，保留草稿供重试。 */ }
  finally { if (version === editorVersion) saving.value = false }
}
let timer
let generation = 0
const stateLabel = state => ({ Idle: '空闲', Running: '运行中', Cancelling: '取消中' }[state] || state)
const resultLabel = status => ({ Completed: '成功', Failed: '失败', Cancelled: '已取消', Aborted: '已中止' }[status] || status || '尚未执行')
const formatTime = value => value ? new Date(value).toLocaleString() : '—'
const triggerLabel = trigger => {
  const hour = Number(trigger.TimeOfDayTicks || 0) / 36000000000
  const time = `${String(Math.floor(hour)).padStart(2, '0')}:${String(Math.round((hour % 1) * 60)).padStart(2, '0')}`
  const day = { Sunday: '周日', Monday: '周一', Tuesday: '周二', Wednesday: '周三', Thursday: '周四', Friday: '周五', Saturday: '周六' }[trigger.DayOfWeek] || trigger.DayOfWeek || ''
  return { IntervalTrigger: `每 ${Number(trigger.IntervalTicks || 0) / 36000000000} 小时`, DailyTrigger: `每天 ${time}`, WeeklyTrigger: `${day} ${time}`, StartupTrigger: 'Emby 启动时' }[trigger.Type] || trigger.Type || '未知规则'
}
const loadTasks = async () => {
  clearTimeout(timer)
  if (!props.active || !props.serverId) return
  const version = ++generation
  loading.value = true
  try {
    const response = await getEmbyScheduledTasks(props.serverId)
    if (version !== generation) return
    const data = response?.data?.data || response?.data || {}
    tasks.value = Array.isArray(data) ? data : data.data || []
    error.value = ''
  } catch (err) {
    if (version === generation) error.value = err.response?.data?.error || err.response?.data?.message || err.message || '读取定时任务失败'
  } finally {
    if (version === generation) {
      loading.value = false
      timer = setTimeout(loadTasks, 5000)
    }
  }
}
const runTask = async task => {
  const serverId = props.serverId
  if (pending.value.has(task.Id)) return
  pending.value.add(task.Id)
  try {
    await startEmbyScheduledTask(serverId, task.Id)
    if (serverId !== props.serverId) return
    message.success('触发请求已提交')
    emit('submitted')
    await loadTasks()
  } catch { /* API 封装统一展示操作错误。 */ }
  finally { if (serverId === props.serverId) pending.value.delete(task.Id) }
}
watch(() => [props.serverId, props.active], () => {
  closeEditor()
  generation++
  clearTimeout(timer)
  loading.value = false
  tasks.value = []
  error.value = ''
  pending.value = new Set()
  loadTasks()
}, { immediate: true })
onBeforeUnmount(() => { closeEditor(); generation++; clearTimeout(timer) })
</script>
