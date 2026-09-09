<template>
  <section class="schedule-page">
    <n-space justify="space-between"
      ><div>
        <h2>定时任务管理</h2>
        <p>统一管理业务与系统维护任务</p>
      </div>
      <n-button type="primary" @click="edit()">新建任务</n-button></n-space
    >
    <n-space style="margin-bottom: 20px"
      ><n-input v-model:value="keyword" placeholder="任务名称" clearable @keyup.enter="search" /><n-select
        v-model:value="handler"
        :options="handlerOptions"
        placeholder="处理器"
        clearable
        style="width: 180px"
      /><n-select
        v-model:value="status"
        :options="statusOptions"
        placeholder="状态"
        clearable
        style="width: 130px"
      /><n-button @click="search">查询</n-button><n-button @click="load">刷新</n-button></n-space
    >
    <n-alert v-if="error" type="error">{{ error }}</n-alert>
    <n-data-table :columns="columns" :data="rows" :loading="loading" :scroll-x="1300" :row-key="(row) => row.id" />
    <n-pagination v-model:page="page" :page-size="20" :item-count="total" style="margin-top: 20px" />
    <n-modal
      v-model:show="showEdit"
      preset="card"
      :title="form.id ? '编辑任务' : '新建任务'"
      style="width: min(600px, 94vw)"
      :mask-closable="!saving"
      :closable="!saving"
    >
      <n-form label-placement="top"
        ><n-form-item label="任务名称"><n-input v-model:value="form.task_name" /></n-form-item>
        <n-form-item label="处理器"
          ><n-select
            v-model:value="form.handler"
            :options="handlerOptions"
            :disabled="form.builtin"
            @update:value="resetParams"
        /></n-form-item>
        <n-form-item v-for="parameter in currentHandler?.parameters || []" :key="parameter.key" :label="parameter.label"
          ><n-input-number v-model:value="form.params[parameter.key]" :min="1" :precision="0"
        /></n-form-item>
        <n-form-item label="Cron表达式（五段或六段）"
          ><n-input v-model:value="form.cron_expr" placeholder="0 0 3 * * *"
        /></n-form-item>
        <n-form-item label="时区"
          ><n-input v-model:value="form.timezone" placeholder="Local 或 Asia/Shanghai"
        /></n-form-item>
        <p>Local 使用服务所在时区。停用仅停止后续调度，当前执行可在任务中心取消。</p>
        <n-form-item label="状态"
          ><n-select v-model:value="form.status" :options="statusOptions"
        /></n-form-item> </n-form
      ><n-button type="primary" :loading="saving" @click="save">保存</n-button>
    </n-modal>
    <n-modal
      v-model:show="showRuns"
      preset="card"
      :title="'执行记录 · ' + (selected?.task_name || '')"
      style="width: min(1000px, 94vw)"
    >
      <n-space style="margin-bottom: 12px"
        ><n-button @click="loadRuns">刷新记录</n-button><span>运行任务的进度与取消请进入任务中心。</span></n-space
      >
      <n-alert v-if="runsError" type="error">{{ runsError }}</n-alert>
      <n-data-table :columns="runColumns" :data="runs" :loading="runsLoading" :scroll-x="850" />
      <n-pagination v-model:page="runPage" :page-size="20" :item-count="runTotal" style="margin-top: 16px" />
    </n-modal>
  </section>
</template>
<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  NAlert,
  NButton,
  NDataTable,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NModal,
  NPagination,
  NSelect,
  NSpace,
  NTag,
  useDialog,
  useMessage
} from 'naive-ui'
import {
  getCronTasks,
  getCronHandlers,
  createCronTask,
  updateCronTask,
  deleteCronTask,
  runCronTask,
  getCronRuns
} from '../utils/api/cron'
const message = useMessage(),
  dialog = useDialog(),
  rows = ref([]),
  handlers = ref([]),
  page = ref(1),
  total = ref(0),
  loading = ref(false),
  error = ref(''),
  keyword = ref(''),
  handler = ref(null),
  status = ref(null)
const statusOptions = [
  { label: '启用', value: 'enabled' },
  { label: '停用', value: 'disabled' }
]
const handlerOptions = computed(() => handlers.value.map((h) => ({ label: h.name, value: h.key })))
let revision = 0,
  runRevision = 0,
  disposed = false,
  query = {}
const errorText = (e) => e.response?.data?.error || e.response?.data?.message || e.message
const load = async () => {
  const v = ++revision
  loading.value = true
  error.value = ''
  try {
    const r = await getCronTasks({ ...query, page: page.value, page_size: 20 })
    if (disposed || v !== revision) return
    rows.value = r.data.data.data
    total.value = r.data.data.total
    if (page.value > Math.max(1, Math.ceil(total.value / 20))) page.value = 1
  } catch (e) {
    if (v === revision) error.value = errorText(e)
  } finally {
    if (v === revision) loading.value = false
  }
}
const search = () => {
  query = { keyword: keyword.value, handler: handler.value, status: status.value }
  if (page.value !== 1) page.value = 1
  else load()
}
watch(page, load)
const showEdit = ref(false),
  saving = ref(false),
  form = ref({ params: {} })
const currentHandler = computed(() => handlers.value.find((h) => h.key === form.value.handler))
const resetParams = () => {
  form.value.params = Object.fromEntries(
    (currentHandler.value?.parameters || []).map((p) => [p.key, p.default || null])
  )
}
const edit = (row) => {
  form.value = row
    ? JSON.parse(JSON.stringify(row))
    : {
        task_name: '',
        handler: handlers.value[0]?.key,
        params: {},
        cron_expr: '0 0 3 * * *',
        timezone: 'Local',
        status: 'enabled'
      }
  if (!row) resetParams()
  showEdit.value = true
}
const save = async () => {
  saving.value = true
  try {
    if (form.value.id) await updateCronTask(form.value.id, form.value)
    else await createCronTask(form.value)
    showEdit.value = false
    message.success('任务已保存，后续调度已更新')
    await load()
  } catch (e) {
    message.error(errorText(e))
  } finally {
    saving.value = false
  }
}
const busy = ref(new Set())
const action = async (row, fn) => {
  busy.value = new Set(busy.value).add(row.id)
  try {
    await fn()
    await load()
  } catch (e) {
    message.error(errorText(e))
  } finally {
    const n = new Set(busy.value)
    n.delete(row.id)
    busy.value = n
  }
}
const run = (row) =>
  action(row, async () => {
    const r = await runCronTask(row.id)
    message.success('触发记录：' + r.data.data.task_id + '，请查看执行记录或任务中心')
  })
const toggle = (row) =>
  action(row, () => updateCronTask(row.id, { status: row.status === 'enabled' ? 'disabled' : 'enabled' }))
const remove = (row) =>
  dialog.warning({
    title: '删除定时任务',
    content: '确定删除“' + row.task_name + '”？当前正在运行的任务不会因此停止。',
    positiveText: '删除',
    negativeText: '取消',
    onPositiveClick: () => action(row, () => deleteCronTask(row.id))
  })
const time = (value) => (value ? new Date(value).toLocaleString() : '—')
const stateNames = {
  enabled: '启用',
  disabled: '停用',
  pending: '待执行',
  running: '运行中',
  success: '成功',
  failed: '失败',
  cancelled: '已取消',
  skipped: '已跳过',
  interrupted: '已中断'
}
const button = (text, fn, row, disabled = false) =>
  h(NButton, { size: 'small', disabled: disabled || busy.value.has(row.id), onClick: fn }, () => text)
const columns = [
  { title: '名称', key: 'task_name', width: 190 },
  {
    title: '处理器',
    key: 'handler',
    width: 170,
    render: (r) => handlers.value.find((h) => h.key === r.handler)?.name || r.handler
  },
  { title: '周期 / 时区', key: 'cron_expr', width: 200, render: (r) => `${r.cron_expr} / ${r.timezone}` },
  {
    title: '状态',
    key: 'status',
    width: 80,
    render: (r) => h(NTag, { type: r.status === 'enabled' ? 'success' : 'default' }, () => stateNames[r.status])
  },
  { title: '最近结果', key: 'last_run_status', width: 110, render: (r) => stateNames[r.last_run_status] || '未执行' },
  { title: '下次执行', key: 'next_run_time', width: 185, render: (r) => time(r.next_run_time) },
  {
    title: '操作',
    key: 'actions',
    width: 380,
    render: (r) =>
      h(NSpace, {}, () => [
        button('编辑', () => edit(r), r),
        button(r.status === 'enabled' ? '停用' : '启用', () => toggle(r), r),
        button('立即执行', () => run(r), r),
        button('记录', () => openRuns(r), r),
        button('删除', () => remove(r), r, r.builtin)
      ])
  }
]
const showRuns = ref(false),
  selected = ref(null),
  runs = ref([]),
  runPage = ref(1),
  runTotal = ref(0),
  runsLoading = ref(false),
  runsError = ref('')
const loadRuns = async () => {
  if (!showRuns.value || !selected.value) return
  const v = ++runRevision
  runsLoading.value = true
  runsError.value = ''
  try {
    const r = await getCronRuns(selected.value.id, { page: runPage.value, page_size: 20 })
    if (disposed || v !== runRevision) return
    runs.value = r.data.data.data
    runTotal.value = r.data.data.total
  } catch (e) {
    if (v === runRevision) runsError.value = errorText(e)
  } finally {
    if (v === runRevision) runsLoading.value = false
  }
}
const openRuns = (row) => {
  selected.value = row
  runs.value = []
  runTotal.value = 0
  showRuns.value = true
  if (runPage.value !== 1) runPage.value = 1
  else loadRuns()
}
watch(runPage, loadRuns)
watch(showRuns, (v) => {
  if (!v) runRevision++
})
const runColumns = [
  { title: '执行ID', key: 'task_id', width: 260 },
  { title: '触发', key: 'trigger_type', width: 80, render: (r) => (r.trigger_type === 'scheduled' ? '定时' : '手动') },
  { title: '开始时间', key: 'started_at', width: 175, render: (r) => time(r.started_at) },
  { title: '结束时间', key: 'ended_at', width: 175, render: (r) => time(r.ended_at) },
  { title: '状态', key: 'status', width: 85, render: (r) => stateNames[r.status] || r.status },
  { title: '结果', key: 'message', width: 250 }
]
onMounted(async () => {
  load()
  try {
    const r = await getCronHandlers()
    if (!disposed) handlers.value = r.data.data
  } catch (e) {
    message.error(errorText(e))
  }
})
onBeforeUnmount(() => {
  disposed = true
  revision++
  runRevision++
})
</script>
<style scoped>
.schedule-page {
  padding: 24px;
}
.schedule-page p {
  color: var(--text-color-3, #888);
}
@media (max-width: 600px) {
  .schedule-page {
    padding: 12px;
  }
}
</style>
