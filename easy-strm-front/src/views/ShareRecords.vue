<template>
  <div class="share-records-page">
    <n-card title="分享管理">
      <n-space class="mb-4 share-toolbar">
        <n-button @click="router.push('/dashboard/share-review')">手动核对中心</n-button>
        <n-button type="primary" @click="openCreate">新增分享</n-button>
        <n-button type="primary" secondary @click="openBatchImport">批量导入分享</n-button>
        <n-upload :show-file-list="false" accept=".json" @change="importFile"><n-button type="info">导入记录</n-button></n-upload>
        <n-button type="success" :disabled="!selectedShareIDs.length || selectedShareIDs.some(shareBusy)" @click="batch(false)">批量自动识别</n-button>
        <n-button type="success" secondary :disabled="!selectedShareIDs.length || selectedShareIDs.some(shareBusy)" @click="batch(true)">重新识别失败项</n-button>
        <n-button type="success" tertiary :disabled="!selectedShareIDs.length || selectedShareIDs.some(shareBusy)" @click="batch(false, true)">继续识别待识别内容</n-button>
        <n-button type="info" :disabled="!selectedShareIDs.length || selectedShareIDs.some(shareBusy)" @click="syncSelected()">批量同步分享文件</n-button>
        <n-button type="primary" secondary @click="processSelected">一键处理全部</n-button>
        <n-button type="warning" @click="taskSettingsShow=true">识别任务设置</n-button>
        <n-button type="error" :loading="clearingAll" :disabled="clearingAll || !selectedShareIDs.length || selectedShareIDs.some(shareBusy)" @click="clearSelectedMedia">批量清空识别内容</n-button>
      </n-space>
      <n-alert v-for="taskState in taskStates" :key="taskState.task_id" class="mb-4" :type="taskState.status === 'failed' ? 'error' : taskState.status === 'completed' ? 'success' : 'info'" :title="`分享任务：${taskState.metadata?.phase || taskState.status || '准备中'}`">
        <n-progress type="line" :percentage="taskState.progress || 0" indicator-placement="inside" />
        <div class="mt-2 text-xs">任务 {{ taskState.task_id }} · 已处理 {{ taskState.processed_files || 0 }}/{{ taskState.total_files || 0 }} · 成功 {{ taskState.success_files || 0 }} · 失败 {{ taskState.failed_files || 0 }}<span v-if="taskState.metadata?.cancelled_shares"> · 已跳过取消分享 {{ taskState.metadata.cancelled_shares }} 个</span><span v-if="taskState.metadata?.current_share"> · 分享：{{ taskState.metadata.current_share }}</span><span v-if="taskState.metadata?.current_file"> · 当前：{{ taskState.metadata.current_file }}</span></div>
        <n-space class="mt-2"><n-tag v-for="step in (taskState.metadata?.steps || [])" :key="step.name" size="small" :type="step.status === 'completed' ? 'success' : step.status === 'running' ? 'info' : 'default'">{{ step.name }}：{{ step.status === 'completed' ? '完成' : step.status === 'running' ? '进行中' : '等待' }}</n-tag></n-space>
        <n-space v-if="operationTypes.includes(taskState.task_type)" class="mt-2">
          <span>{{ taskState.status === 'pending' ? '等待目标资源，其他分享可继续操作' : taskState.status === 'running' ? '正在执行清理' : taskState.status === 'completed' ? '清理完成' : taskState.error_message || taskState.status }}</span>
          <span v-if="taskState.status === 'pending' && taskState.metadata?.blocking_task_ids?.length">等待任务：{{ taskState.metadata.blocking_task_ids.join('、') }}</span>
          <n-button v-if="taskState.status === 'pending' && taskState.metadata?.cancellable !== false" size="small" :loading="cancellingIDs.has(taskState.task_id)" @click="cancelOperation(taskState)">取消等待</n-button>
          <n-button size="small" @click="router.push('/dashboard/tasks')">任务中心</n-button>
        </n-space>
      </n-alert>
      <n-data-table :row-key="row => row.id" :columns="columns" :data="rows" :loading="loading" :pagination="pagination" :checked-row-keys="selectedShareIDs" remote @update:checked-row-keys="selectedShareIDs=$event" @update:page="page=$event;load()" @update:page-size="changePageSize" />
    </n-card>
    <ShareImportDialog v-model:show="batchImportShow" @imported="load" />
    <ShareTaskSettingsDialog v-model:show="taskSettingsShow" />
    <n-modal v-model:show="show"><n-card :title="editing ? '编辑分享' : '新增分享'" style="width:720px">
      <n-form>
        <template v-if="!editing"><n-alert type="info" class="mb-4">新分享默认按单条媒体自动判断电影或电视剧。</n-alert><n-form-item label="分享内容"><n-input v-model:value="form.url" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="粘贴115分享链接或完整分享文案，将自动识别链接和访问码" /></n-form-item></template>
        <template v-else>
          <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item><n-form-item label="分享链接"><n-input v-model:value="form.url" /></n-form-item><n-form-item label="密码"><n-input v-model:value="form.password" /></n-form-item><n-form-item label="备注"><n-input v-model:value="form.note" /></n-form-item>
          <n-form-item label="媒体类型"><n-select v-model:value="form.media_type" :options="[{label:'自动（混合）',value:'auto'},{label:'电影',value:'movie'},{label:'电视剧',value:'tv'}]" /></n-form-item>
        </template>
      </n-form><n-button type="primary" @click="save">保存</n-button>
    </n-card></n-modal>
  </div>
</template>
<script setup>
import { computed, h, ref, onBeforeUnmount, onMounted, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { dialog } from '../utils/ui/feedback'
import ShareImportDialog from '../components/ShareImportDialog.vue'
import ShareTaskSettingsDialog from '../components/ShareTaskSettingsDialog.vue'
import ShareMediaGallery from '../components/ShareMediaGallery.vue'
import { NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NImage, NInput, NModal, NSpace, NProgress, NSelect, NTag, NUpload, useMessage } from 'naive-ui'
import { getShareRecords, clearShareMedia, clearSelectedShareMedia, createShareRecord, updateShareRecord, deleteShareRecord, identifyShareMedia, identifyShareRecord, syncShareRecord, deleteShareMedia, batchIdentifyShareRecords, batchSyncShareRecords, getShareIdentifyTask } from '../utils/api/media'
import { createShareTaskWatcher } from '../utils/ui/shareTaskWatcher'
import { cancelTask, getUnifiedTaskList } from '../utils/api/task'
const route=useRoute(), router=useRouter()
const msg=useMessage(),rows=ref([]),loading=ref(false),show=ref(false),editing=ref(false),batchImportShow=ref(false),page=ref(1),taskStates=ref({}),selectedShareIDs=ref([]),form=ref({name:'',url:'',password:'',note:'',media:[]}),sources=[{label:'自动',value:'auto'},{label:'TMDB',value:'tmdb'},{label:'MetaTube',value:'metatube'}],pagination=ref({page:1,pageSize:20,itemCount:0,pageSizes:[10,20,50,100],showSizePicker:true})
const galleryRevision=ref(0)
const load=async()=>{loading.value=true;try{const r=await getShareRecords({page:page.value,page_size:pagination.value.pageSize,share_id:route.query.share_id});const payload=r.data?.data||r.data||{};galleryRevision.value++;rows.value=Array.isArray(payload)?payload:(payload.data||[]);pagination.value={...pagination.value,page:page.value,itemCount:payload.total||rows.value.length}}finally{loading.value=false}}
const changePageSize=size=>{pagination.value={...pagination.value,pageSize:size};page.value=1;load()}
const operationTypes = ['share_delete', 'share_clear', 'share_media_delete']
const activeOperations = computed(() => Object.values(taskStates.value).filter(task => operationTypes.includes(task.task_type) && ['pending', 'running'].includes(task.status)))
const busyShareIDs = computed(() => new Set(activeOperations.value.filter(task => task.task_type !== 'share_media_delete').flatMap(task => task.metadata?.record_ids || []).map(Number)))
const fileSubmittingIDs = ref(new Set())
const busyFileIDs = computed(() => [...fileSubmittingIDs.value, ...activeOperations.value.filter(task => task.task_type === 'share_media_delete').map(task => Number(task.metadata?.file_id))])
const shareBusy = id => busyShareIDs.value.has(Number(id))
const cancellingIDs = ref(new Set())
const cancelOperation = async task => {
  cancellingIDs.value.add(task.task_id)
  try { await cancelTask(task.task_id); msg.success('已取消等待中的操作') } catch {} finally { cancellingIDs.value.delete(task.task_id) }
}
const submitOperation = (response, kind, ids, fileID = 0) => {
  const id = response.data?.data?.task_id
  if (!id) throw new Error('操作任务创建失败')
  taskStates.value = { ...taskStates.value, [id]: { task_id: id, task_type: kind, status: 'pending', metadata: { record_ids: ids, file_id: fileID, phase: '等待目标资源', cancellable: true } } }
  msg.info('已提交，等待目标资源空闲后执行')
  void watchTask(id).catch(() => msg.error('任务状态读取失败，请在任务中心查看'))
  return id
}
const deletingIDs = ref(new Set())
const remove = row => {
  if (deletingIDs.value.has(row.id) || shareBusy(row.id)) return
  dialog.warning({
    title: '删除分享',
    content: `确定删除“${row.name}”吗？将同时删除已同步的分享文件记录、关联的 STRM 数据库记录和已生成的 STRM 文件，此操作不可恢复。网盘原文件不受影响。若目标正在处理，将等待当前操作结束后自动删除；服务重启后会继续执行。`,
    positiveText: '确认删除',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (deletingIDs.value.has(row.id)) return false
      deletingIDs.value.add(row.id)
      try {
        const response = await deleteShareRecord(row.id)
        submitOperation(response, 'share_delete', [row.id])
      } catch { return false }
      finally { deletingIDs.value.delete(row.id) }
    }
  })
}
const clearingIDs = ref(new Set())
const clearingAll = ref(false)
const taskSettingsShow = ref(false)
const clearSelectedMedia = () => {
  const ids = [...selectedShareIDs.value]
  dialog.warning({
    title: '批量清空识别内容',
    content: `确定清空选中的 ${ids.length} 个分享下的全部文件候选及识别关联吗？分享链接及配置会保留，网盘文件不受影响。若目标正在处理，将等待结束后自动清空；服务重启后会继续执行。`,
    positiveText: '确认清空', negativeText: '取消',
    onPositiveClick: async () => {
      clearingAll.value = true
      try {
        const response = await clearSelectedShareMedia(ids)
        submitOperation(response, 'share_clear', ids)
      } catch { return false } finally { clearingAll.value = false }
    }
  })
}
const clearMedia = row => {
  if (clearingIDs.value.has(row.id) || shareBusy(row.id)) return
  dialog.warning({
    title: '清空识别内容',
    content: `确定清空“${row.name}”下的全部 ${row.file_count || 0} 条文件候选及识别关联吗？包含已识别、失败、待识别和脱敏记录。分享链接及配置会保留，网盘文件不受影响；清空后可点击“同步分享文件”重新扫描。若目标正在处理，将等待结束后自动清空；服务重启后会继续执行。`,
    positiveText: '确认清空',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (clearingIDs.value.has(row.id)) return false
      clearingIDs.value.add(row.id)
      try {
        const response = await clearShareMedia(row.id)
        submitOperation(response, 'share_clear', [row.id])
      } catch { return false }
      finally { clearingIDs.value.delete(row.id) }
    }
  })
}
const removeFile = (share, file) => {
  if (shareBusy(share.id) || busyFileIDs.value.includes(file.id)) return
  dialog.warning({
    title: '删除文件候选',
    content: `确定删除“${file.file_name}”的候选和识别关联吗？网盘原文件不受影响。若该文件正在处理，将等待当前操作结束后自动删除；服务重启后会继续执行。`,
    positiveText: '确认删除', negativeText: '取消',
    onPositiveClick: async () => {
      fileSubmittingIDs.value.add(file.id)
      try { const response = await deleteShareMedia(share.id, file.id); submitOperation(response, 'share_media_delete', [share.id], file.id) }
      catch { return false } finally { fileSubmittingIDs.value.delete(file.id) }
    }
  })
}
const taskWatcher = createShareTaskWatcher(
  async id => { const r = await getShareIdentifyTask(id); return r.data?.data || r.data },
  task => { taskStates.value = { ...taskStates.value, [task.task_id]: task } },
  task => {
    if (operationTypes.includes(task.task_type)) {
      if (task.status === 'completed') {
        msg.success(task.task_type === 'share_clear' ? `已清空 ${task.metadata?.result?.deleted || 0} 条文件记录` : '删除已完成')
        selectedShareIDs.value = selectedShareIDs.value.filter(id => !(task.metadata?.record_ids || []).includes(id))
      } else if (task.status === 'failed') msg.error(task.error_message || '清理失败，请在任务中心查看详情')
    }
    void load().catch(() => msg.error('任务已结束，但列表刷新失败'))
  },
  1000,
  () => msg.warning('任务状态暂不可用，正在自动重试')
)
const watchTask = id => taskWatcher.watch(id)
const restoreTask = async () => {
  try {
    const r = await getUnifiedTaskList()
    const payload = r.data?.data || r.data
    const tasks = Array.isArray(payload) ? payload : (payload?.data || [])
    for (const task of tasks.filter(t => ['share_identify', 'share_sync', ...operationTypes].includes(t.task_type) && ['pending', 'running'].includes(t.status))) {
      taskStates.value = { ...taskStates.value, [task.task_id]: task }
      void watchTask(task.task_id).catch(() => msg.error('分享任务进度获取失败，请刷新重试'))
    }
  } catch { /* 任务恢复失败不阻塞分享列表 */ }
}
const openBatchImport = () => { batchImportShow.value = true }
const openCreate=()=>{editing.value=false;form.value={name:'',url:'',password:'',note:'',media:[]};show.value=true};const openEdit=r=>{editing.value=true;form.value={id:r.id,version:r.version,name:r.name,url:r.url,password:r.password,note:r.note,media_type:r.media_type||'auto'};show.value=true};const save=async()=>{if(editing.value)await updateShareRecord(form.value.id,form.value);else await createShareRecord(form.value);show.value=false;load()};const batch=async (retry,pendingOnly=false,shareIDs=selectedShareIDs.value)=>{const r=await batchIdentifyShareRecords({share_ids:shareIDs,retry_failed:retry,pending_only:pendingOnly});const id=r.data?.data?.task_id;if(id)await watchTask(id);else msg.error('批量识别任务创建失败')};const filterUnsyncedShareIDs=shareIDs=>{const selected=new Set((shareIDs||[]).map(Number));return [...selected].filter(id=>{const row=rows.value.find(item=>Number(item.id)===id);return !row||Number(row.file_count||0)===0})};const syncSelected=async shareIDs=>{const candidateIDs=filterUnsyncedShareIDs(shareIDs||selectedShareIDs.value);if(!candidateIDs.length){msg.warning('所选分享均已同步，无需重复同步');return null}const response=await batchSyncShareRecords(candidateIDs);const id=response.data?.data?.task_id;if(id)return watchTask(id);msg.error('批量同步分享文件任务创建失败')};const loadAllShareIDs=async()=>{const ids=[];let pageNumber=1;let total=0;do{const response=await getShareRecords({page:pageNumber,page_size:200});const payload=response.data?.data||response.data||{};const records=Array.isArray(payload)?payload:(payload.data||[]);ids.push(...records.filter(record=>Number(record.file_count||0)===0).map(record=>record.id).filter(id=>Number.isInteger(id)));total=Number(payload.total||records.length);pageNumber++;if(!records.length)break}while(pageNumber===1||ids.length<total);return [...new Set(ids)]};const processSelected=async()=>{const shareIDs=await loadAllShareIDs();if(!shareIDs.length){msg.warning('没有需要同步的分享');return}const syncTask=await syncSelected(shareIDs);if(!syncTask)return;const syncedIDs=syncTask.metadata?.synced_share_ids||[];if(syncedIDs.length)await batch(false,false,syncedIDs);else msg.warning('没有同步成功的分享，未发起识别任务')};const syncRecord=async r=>{const response=await syncShareRecord(r.id);const id=response.data?.data?.task_id;if(id)await watchTask(id);else msg.error('分享文件同步任务创建失败')};const identifyRecord=async (r,pendingOnly=false,failedOnly=false)=>{const response=await identifyShareRecord(r.id,pendingOnly,failedOnly);const id=response.data?.data?.task_id;if(id)await watchTask(id);else msg.error('分享识别任务创建失败')};const identify=async m=>{await identifyShareMedia(m.id,{file_name:m.file_name,metadata_source:m.metadata_source,version:m.version});load()};const importFile=async({file})=>{for(const r of JSON.parse(await file.file.text()))await createShareRecord(r);load()};onBeforeUnmount(()=>taskWatcher.dispose())

const columns=[{type:'selection'},{type:'expand',expandable:r=>(r.file_count||0)>0,renderExpand:r=>h(ShareMediaGallery,{shareId:r.id,revision:galleryRevision.value,onIdentify:identify,onSaved:load,blocked:shareBusy(r.id),busyFileIDs:busyFileIDs.value,onRemove:m=>removeFile(r,m)})},{title:'名称',key:'name',render:r=>h(NSpace,{align:'center'},()=>[h('span',r.name),r.share_cancelled?h(NTag,{type:'error',size:'small'},()=> '分享已失效'):null])},{title:'链接',key:'url',ellipsis:true},{title:'文件与媒体',render:r=>h('div',{},[h('div',{},`文件 ${r.file_count||0} · 媒体 ${r.media_count||0}`),h('div',{class:'text-xs text-slate-500'},'已识别 '+(r.identified_count||0)+' · 失败 '+(r.failed_count||0)+' · 待识别 '+(r.pending_count||0)+' · 失效 '+(r.unavailable_count||0))])},{title:'操作',render:r=>h('div',{class:'flex flex-wrap gap-2'},[h(NButton,{size:'small',onClick:()=>router.push({path:'/dashboard/share-review',query:{share_id:r.id,status:'failed'}})},{default:()=>'核对失败项'}),h(NButton,{size:'small',type:'warning',loading:clearingIDs.value.has(r.id),disabled:clearingIDs.value.has(r.id)||shareBusy(r.id),onClick:()=>clearMedia(r)},{default:()=>'清空文件记录'}),h(NButton,{size:'small',disabled:shareBusy(r.id),onClick:()=>openEdit(r)},{default:()=>'编辑'}),h(NButton,{size:'small',loading:deletingIDs.value.has(r.id),disabled:deletingIDs.value.has(r.id)||shareBusy(r.id),onClick:()=>remove(r)},{default:()=>'删除'}),h(NButton,{size:'small',type:'info',disabled:shareBusy(r.id)||r.share_cancelled,onClick:()=>syncRecord(r)},{default:()=>'同步分享文件'}),h(NButton,{size:'small',type:'primary',disabled:shareBusy(r.id)||r.share_cancelled||!r.file_count,onClick:()=>identifyRecord(r)},{default:()=>'识别媒体'}),h(NButton,{size:'small',type:'primary',secondary:true,disabled:shareBusy(r.id)||r.share_cancelled||!r.pending_count,onClick:()=>identifyRecord(r,true)},{default:()=>'识别待处理'}),h(NButton,{size:'small',type:'warning',secondary:true,disabled:shareBusy(r.id)||r.share_cancelled||!r.failed_count,onClick:()=>identifyRecord(r,false,true)},{default:()=>'重试失败'})])}]
watch(()=>route.query.share_id,()=>{page.value=1;load()})
onMounted(async()=>{await load();await restoreTask()})
</script>
