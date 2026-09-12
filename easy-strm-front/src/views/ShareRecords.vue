<template>
  <div class="share-records-page">
    <n-card title="分享管理">
      <n-space class="mb-4 share-toolbar">
        <n-button type="primary" @click="openCreate">新增分享</n-button>
        <n-button @click="openBatchImport">批量导入分享</n-button>
        <n-upload :show-file-list="false" accept=".json" @change="importFile"><n-button>导入记录</n-button></n-upload>
        <n-button @click="batch(false)">批量自动识别</n-button>
        <n-button @click="batch(true)">重新识别失败项</n-button>
        <n-button @click="batch(false, true)">继续识别待识别内容</n-button>
        <n-button @click="taskSettingsShow=true">识别任务设置</n-button>
        <n-button type="warning" :loading="clearingAll" :disabled="clearingAll || !pagination.itemCount" @click="clearAllMedia">批量清空识别内容</n-button>
      </n-space>
      <n-alert v-if="taskState" class="mb-4" :type="taskState.status === 'failed' ? 'error' : taskState.status === 'completed' ? 'success' : 'info'" :title="`批量识别：${taskPhase}`">
        <n-progress type="line" :percentage="taskState.progress || 0" indicator-placement="inside" />
        <div class="mt-2 text-xs">任务 {{ taskState.task_id }} · 已处理 {{ taskState.processed_files || 0 }}/{{ taskState.total_files || 0 }} · 成功 {{ taskState.success_files || 0 }} · 失败 {{ taskState.failed_files || 0 }}<span v-if="taskState.metadata?.cancelled_shares"> · 已跳过取消分享 {{ taskState.metadata.cancelled_shares }} 个</span><span v-if="taskState.metadata?.current_share"> · 分享：{{ taskState.metadata.current_share }}</span><span v-if="taskState.metadata?.current_file"> · 当前：{{ taskState.metadata.current_file }}</span></div>
        <n-space class="mt-2"><n-tag v-for="step in (taskState.metadata?.steps || [])" :key="step.name" size="small" :type="step.status === 'completed' ? 'success' : step.status === 'running' ? 'info' : 'default'">{{ step.name }}：{{ step.status === 'completed' ? '完成' : step.status === 'running' ? '进行中' : '等待' }}</n-tag></n-space>
      </n-alert>
      <n-data-table :row-key="row => row.id" :columns="columns" :data="rows" :loading="loading" :pagination="pagination" remote @update:page="page=$event;load()" />
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
import { useRoute } from 'vue-router'
import { dialog } from '../utils/ui/feedback'
import ShareImportDialog from '../components/ShareImportDialog.vue'
import ShareTaskSettingsDialog from '../components/ShareTaskSettingsDialog.vue'
import ShareMediaGallery from '../components/ShareMediaGallery.vue'
import { NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NImage, NInput, NModal, NSpace, NProgress, NSelect, NTag, NUpload, useMessage } from 'naive-ui'
import { getShareRecords, clearShareMedia, clearAllShareMedia, createShareRecord, updateShareRecord, deleteShareRecord, identifyShareMedia, identifyShareRecord, syncShareRecord, deleteShareMedia, batchIdentifyShareRecords, getShareIdentifyTask } from '../utils/api/media'
import { getUnifiedTaskList } from '../utils/api/task'
const route=useRoute()
const msg=useMessage(),rows=ref([]),loading=ref(false),show=ref(false),editing=ref(false),batchImportShow=ref(false),page=ref(1),taskState=ref(null),taskTimer=ref(null),form=ref({name:'',url:'',password:'',note:'',media:[]}),sources=[{label:'自动',value:'auto'},{label:'TMDB',value:'tmdb'},{label:'MetaTube',value:'metatube'}],pagination=ref({page:1,pageSize:20,itemCount:0}),taskPhase=computed(()=>taskState.value?.metadata?.phase||taskState.value?.status||'准备中')
const galleryRevision=ref(0)
const load=async()=>{loading.value=true;try{const r=await getShareRecords({page:page.value,page_size:20,share_id:route.query.share_id});const payload=r.data?.data||r.data||{};galleryRevision.value++;rows.value=Array.isArray(payload)?payload:(payload.data||[]);pagination.value={...pagination.value,page:page.value,itemCount:payload.total||rows.value.length}}finally{loading.value=false}}
const clearingIDs = ref(new Set())
const clearingAll = ref(false)
const taskSettingsShow = ref(false)
const clearAllMedia = () => {
  if (clearingAll.value) return
  dialog.warning({
    title: '批量清空识别内容',
    content: `确定清空全部 ${pagination.value.itemCount || 0} 个分享下的媒体记录吗？已识别、失败、待识别和脱敏记录都会删除。分享链接及配置会保留，网盘文件不受影响。`,
    positiveText: '确认全部清空',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (clearingAll.value) return false
      clearingAll.value = true
      try {
        const response = await clearAllShareMedia()
        msg.success(`已批量清空 ${response.data?.data?.deleted || 0} 条媒体记录`)
        try { await load() } catch { msg.error('已清空，但列表刷新失败，请刷新页面') }
      } catch { return false }
      finally { clearingAll.value = false }
    }
  })
}
const clearMedia = row => {
  if (clearingIDs.value.has(row.id)) return
  dialog.warning({
    title: '清空识别内容',
    content: `确定清空“${row.name}”下的全部 ${row.media_count || 0} 条媒体记录吗？包含已识别、失败、待识别和脱敏记录。分享链接及配置会保留，网盘文件不受影响；清空后可点击“识别”重新扫描。`,
    positiveText: '确认清空',
    negativeText: '取消',
    onPositiveClick: async () => {
      if (clearingIDs.value.has(row.id)) return false
      clearingIDs.value.add(row.id)
      try {
        const response = await clearShareMedia(row.id)
        msg.success(`已清空 ${response.data?.data?.deleted || 0} 条媒体记录，可重新识别`)
        try { await load() } catch { msg.error('已清空，但列表刷新失败，请刷新页面') }
      } catch { return false }
      finally { clearingIDs.value.delete(row.id) }
    }
  })
}
const restoreTask=async()=>{try{const r=await getUnifiedTaskList();const payload=r.data?.data||r.data;const tasks=Array.isArray(payload)?payload:(payload?.data||[]);const active=tasks.filter(t=>['share_identify','share_sync'].includes(t.task_type)&&['pending','running'].includes(t.status)).sort((a,b)=>String(b.update_time||'').localeCompare(String(a.update_time||'')))[0];if(active?.task_id)await watchTask(active.task_id)}catch(e){/* 任务恢复失败不阻塞分享列表 */}}
const openBatchImport = () => { batchImportShow.value = true }
const openCreate=()=>{editing.value=false;form.value={name:'',url:'',password:'',note:'',media:[]};show.value=true};const openEdit=r=>{editing.value=true;form.value={id:r.id,version:r.version,name:r.name,url:r.url,password:r.password,note:r.note,media_type:r.media_type||'auto'};show.value=true};const save=async()=>{if(editing.value)await updateShareRecord(form.value.id,form.value);else await createShareRecord(form.value);show.value=false;load()};const pollTask=async id=>{try{const r=await getShareIdentifyTask(id);taskState.value=r.data?.data||r.data;if(['completed','failed','cancelled'].includes(taskState.value?.status)){clearInterval(taskTimer.value);taskTimer.value=null;load()}}catch(e){clearInterval(taskTimer.value);taskTimer.value=null}};const watchTask=async id=>{taskState.value={task_id:id,status:'pending',progress:0,metadata:{phase:'准备中',steps:[]}};await pollTask(id);taskTimer.value=setInterval(()=>pollTask(id),1000)};const batch=async (retry,pendingOnly=false)=>{const r=await batchIdentifyShareRecords({retry_failed:retry,pending_only:pendingOnly});const id=r.data?.data?.task_id;if(id)await watchTask(id);else msg.error('批量识别任务创建失败')};const syncRecord=async r=>{const response=await syncShareRecord(r.id);const id=response.data?.data?.task_id;if(id)await watchTask(id);else msg.error('分享文件同步任务创建失败')};const identifyRecord=async (r,pendingOnly=false,failedOnly=false)=>{const response=await identifyShareRecord(r.id,pendingOnly,failedOnly);const id=response.data?.data?.task_id;if(id)await watchTask(id);else msg.error('分享识别任务创建失败')};const identify=async m=>{await identifyShareMedia(m.id,{file_name:m.file_name,metadata_source:m.metadata_source,version:m.version});load()};const importFile=async({file})=>{for(const r of JSON.parse(await file.file.text()))await createShareRecord(r);load()};const remove=async r=>{await deleteShareRecord(r.id);load()};onBeforeUnmount(()=>{if(taskTimer.value)clearInterval(taskTimer.value)})

const columns=[{type:'expand',expandable:r=>(r.file_count||0)>0,renderExpand:r=>h(ShareMediaGallery,{shareId:r.id,revision:galleryRevision.value,onIdentify:identify,onSaved:load,onRemove:async m=>{await deleteShareMedia(r.id,m.id);await load()}})},{title:'名称',key:'name',render:r=>h(NSpace,{align:'center'},()=>[h('span',r.name),r.share_cancelled?h(NTag,{type:'error',size:'small'},()=> '分享已取消'):null])},{title:'链接',key:'url',ellipsis:true},{title:'文件与媒体',render:r=>h('div',{},[h('div',{},`文件 ${r.file_count||0} · 媒体 ${r.media_count||0}`),h('div',{class:'text-xs text-slate-500'},'已识别 '+(r.identified_count||0)+' · 失败 '+(r.failed_count||0)+' · 待识别 '+(r.pending_count||0)+' · 失效 '+(r.unavailable_count||0))])},{title:'操作',render:r=>h('div',{class:'flex flex-wrap gap-2'},[h(NButton,{size:'small',type:'warning',loading:clearingIDs.value.has(r.id),disabled:clearingIDs.value.has(r.id),onClick:()=>clearMedia(r)},{default:()=>'清空文件记录'}),h(NButton,{size:'small',onClick:()=>openEdit(r)},{default:()=>'编辑'}),h(NButton,{size:'small',onClick:()=>remove(r)},{default:()=>'删除'}),h(NButton,{size:'small',type:'info',disabled:r.share_cancelled,onClick:()=>syncRecord(r)},{default:()=>'同步分享文件'}),h(NButton,{size:'small',type:'primary',disabled:r.share_cancelled||!r.file_count,onClick:()=>identifyRecord(r)},{default:()=>'识别媒体'}),h(NButton,{size:'small',type:'primary',secondary:true,disabled:r.share_cancelled||!r.pending_count,onClick:()=>identifyRecord(r,true)},{default:()=>'识别待处理'}),h(NButton,{size:'small',type:'warning',secondary:true,disabled:r.share_cancelled||!r.failed_count,onClick:()=>identifyRecord(r,false,true)},{default:()=>'重试失败'})])}]
watch(()=>route.query.share_id,()=>{page.value=1;load()})
onMounted(async()=>{await load();await restoreTask()})
</script>
