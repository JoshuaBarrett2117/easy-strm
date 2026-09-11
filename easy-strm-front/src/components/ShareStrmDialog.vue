<template>
  <n-modal :show="show" preset="card" title="导出资料库 STRM" style="width: min(640px, 94vw)" @update:show="close">
    <n-alert type="info" style="margin-bottom: 16px">
      导出当前已应用筛选条件下的 {{ count }} 部作品，仅使用本地资料库已有电影和单集记录，不重新扫描分享。播放时才定位并转存对应文件。
    </n-alert>
    <n-spin :show="loading">
      <n-alert v-if="loadError" type="error">{{ loadError }} <n-button text @click="load">重试</n-button></n-alert>
      <n-form label-placement="top">
        <n-form-item label="服务器导出目录"><n-input v-model:value="form.output_path" placeholder="例如 /media/share-library（Docker 内路径）" /></n-form-item>
        <n-form-item label="播放访问地址"><n-input v-model:value="form.base_url" placeholder="例如 http://192.168.1.10:8080" /></n-form-item>
        <n-form-item label="播放转存到115账号"><n-select v-model:value="form.cloud115_id" :options="accounts" placeholder="请先在115账号管理中配置账号" /></n-form-item>
        <n-form-item label="115转存目录"><n-input v-model:value="form.transfer_path" placeholder="例如 /STRM播放" /></n-form-item>
      </n-form>
      <p class="export-note">播放地址须能从媒体服务器访问。目录按整理规则中的分类策略生成，电视剧按 Season / SxxExx 分层；无法识别的集会记录为失败。同作品同集使用首个有效来源，已有同名 STRM 更新内容。修改账号或转存目录后，已有链接在下次播放时使用新配置。</p>
    </n-spin>
    <n-alert v-if="taskID" type="success" style="margin-bottom: 16px">导出任务已创建：{{ taskID }}，可在任务中心查看生成数量、失败详情或取消。</n-alert>
    <TaskCard v-if="exportTask" :task="exportTask" class="mb-4" @detail="openTask" @cancel="cancelExport" />
    <n-alert v-if="progressError" type="error" style="margin-bottom: 16px">{{ progressError }} <n-button text @click="refreshProgress">重试进度查询</n-button></n-alert>
    <n-space justify="end">
      <n-button @click="close(false)">关闭</n-button>
      <n-button :loading="saving" :disabled="loading || !!loadError || exporting" @click="save">保存配置</n-button>
      <n-button type="primary" :loading="exporting" :disabled="loading || !!loadError || saving || count === 0" @click="start">保存并导出</n-button>
    </n-space>
  </n-modal>
</template>
<script setup>
import { reactive, ref, watch, onBeforeUnmount } from 'vue'
import { useRouter } from 'vue-router'
import TaskCard from './TaskCard.vue'
import { getTaskDetail, cancelTask } from '../utils/api/task'
import { NModal, NAlert, NButton, NForm, NFormItem, NInput, NSelect, NSpace, NSpin, useMessage } from 'naive-ui'
import { getCloud115List } from '../utils/api/cloud115'
import { getLibraryStrmSettings, saveLibraryStrmSettings, exportLibraryStrm } from '../utils/api/share-library'
const props = defineProps({ show: Boolean, filters: { type: Object, default: () => ({}) }, count: { type: Number, default: 0 } })
const emit = defineEmits(['update:show'])
const message = useMessage()
const router = useRouter()
const exportTask = ref(null), progressError = ref('')
let progressTimer, progressRevision = 0
function stopProgress() { clearTimeout(progressTimer); progressRevision++ }
async function refreshProgress() {
  stopProgress()
  if (!props.show || !taskID.value) return
  const current = progressRevision, id = taskID.value
  try {
    const response = await getTaskDetail(id)
    if (current !== progressRevision) return
    exportTask.value = response.data.data
    progressError.value = ''
    if (['pending', 'running'].includes(exportTask.value?.status)) progressTimer = setTimeout(refreshProgress, 1000)
  } catch (e) { if (current === progressRevision) progressError.value = errorText(e) }
}
function openTask() { close(false); router.push({ path: '/dashboard/tasks', query: { task_id: taskID.value } }) }
async function cancelExport() {
  try { await cancelTask(taskID.value); await refreshProgress() }
  catch (e) { message.error(errorText(e)) }
}
const form = reactive({ output_path: '', base_url: '', cloud115_id: null, transfer_path: '/STRM播放' })
const accounts = ref([]), loading = ref(false), saving = ref(false), exporting = ref(false), loadError = ref(''), taskID = ref('')
let revision = 0
const errorText = (e) => e.response?.data?.error || e.response?.data?.message || e.message
const close = (value) => emit('update:show', value)
async function load() {
  const current = ++revision
  loading.value = true
  loadError.value = ''
  try {
    const [settings, list] = await Promise.all([getLibraryStrmSettings(), getCloud115List()])
    if (current !== revision) return
    const value = settings.data.data
    Object.assign(form, { ...value, cloud115_id: value.cloud115_id || null, transfer_path: value.transfer_path || '/STRM播放' })
    const accountData = list.data.data
    accounts.value = (Array.isArray(accountData) ? accountData : accountData?.data || []).map((account) => ({ label: account.name, value: account.id }))
  } catch (e) {
    if (current === revision) loadError.value = errorText(e)
  } finally {
    if (current === revision) loading.value = false
  }
}
async function save() {
  saving.value = true
  try { await saveLibraryStrmSettings({ ...form }); message.success('STRM配置已保存') }
  catch (e) { message.error(errorText(e)) }
  finally { saving.value = false }
}
async function start() {
  exporting.value = true
  const filters = { ...props.filters }
  try {
    await saveLibraryStrmSettings({ ...form })
    const result = await exportLibraryStrm(filters)
    taskID.value = result.data.data.task_id
    exportTask.value = { task_id: taskID.value, task_name: '分享资料库STRM导出', task_type: 'strm_generate', status: 'pending', progress: 0 }
    refreshProgress()
    message.success('导出任务已创建，可在任务中心查看')
  } catch (e) { message.error(errorText(e)) }
  finally { exporting.value = false }
}
watch(() => props.show, (show) => { if (show) { load(); refreshProgress() } else { revision++; loading.value = false; stopProgress() } })
onBeforeUnmount(() => { revision++; stopProgress() })
</script>
<style scoped>
.export-note { color: var(--text-color-3, #888); font-size: 13px; line-height: 1.7; }
</style>
