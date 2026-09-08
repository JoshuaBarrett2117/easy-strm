<template>
  <div class="share-records-page">
    <n-card title="分享管理">
      <n-space class="mb-4">
        <n-button type="primary" @click="openCreate">新增分享</n-button>
        <n-button @click="batch(false)">批量自动识别</n-button>
        <n-button @click="batch(true)">重新识别失败项</n-button>
        <n-button @click="openBatchImport">批量导入分享</n-button><n-upload :show-file-list="false" accept=".json" @change="importFile"><n-button>导入记录</n-button></n-upload>
      </n-space>
      <n-alert v-if="taskState" class="mb-4" :type="taskState.status === 'failed' ? 'error' : taskState.status === 'completed' ? 'success' : 'info'" :title="`批量识别：${taskPhase}`">
        <n-progress type="line" :percentage="taskState.progress || 0" indicator-placement="inside" />
        <div class="mt-2 text-xs">任务 {{ taskState.task_id }} · 已处理 {{ taskState.processed_files || 0 }}/{{ taskState.total_files || 0 }} · 成功 {{ taskState.success_files || 0 }} · 失败 {{ taskState.failed_files || 0 }}<span v-if="taskState.metadata?.current_share"> · 分享：{{ taskState.metadata.current_share }}</span><span v-if="taskState.metadata?.current_file"> · 当前：{{ taskState.metadata.current_file }}</span></div>
        <n-space class="mt-2"><n-tag v-for="step in (taskState.metadata?.steps || [])" :key="step.name" size="small" :type="step.status === 'completed' ? 'success' : step.status === 'running' ? 'info' : 'default'">{{ step.name }}：{{ step.status === 'completed' ? '完成' : step.status === 'running' ? '进行中' : '等待' }}</n-tag></n-space>
      </n-alert>
      <n-data-table :row-key="row => row.id" :columns="columns" :data="rows" :loading="loading" :pagination="pagination" remote @update:page="page=$event;load()" />
    </n-card>
    <ShareImportDialog v-model:show="batchImportShow" @imported="load" />
    <n-modal v-model:show="show"><n-card :title="editing ? '编辑分享' : '新增分享'" style="width:720px">
      <n-form>
        <template v-if="!editing"><n-form-item label="分享内容"><n-input v-model:value="form.url" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="粘贴115分享链接或完整分享文案，将自动识别链接和访问码" /></n-form-item></template>
        <template v-else>
          <n-form-item label="名称"><n-input v-model:value="form.name" /></n-form-item><n-form-item label="分享链接"><n-input v-model:value="form.url" /></n-form-item><n-form-item label="密码"><n-input v-model:value="form.password" /></n-form-item><n-form-item label="备注"><n-input v-model:value="form.note" /></n-form-item>
          <n-form-item label="媒体类型"><n-select v-model:value="form.media_type" :options="[{label:'电影',value:'movie'},{label:'电视剧',value:'tv'}]" /></n-form-item>
        </template>
      </n-form><n-button type="primary" @click="save">保存</n-button>
    </n-card></n-modal>
  </div>
</template>
<script setup>
import { computed, h, ref, onBeforeUnmount, onMounted } from 'vue'
import ShareImportDialog from '../components/ShareImportDialog.vue'
import ShareMediaGallery from '../components/ShareMediaGallery.vue'
import { NAlert, NButton, NCard, NDataTable, NForm, NFormItem, NImage, NInput, NModal, NSpace, NProgress, NSelect, NTag, NUpload, useMessage } from 'naive-ui'
import { getShareRecords, createShareRecord, updateShareRecord, deleteShareRecord, identifyShareMedia, identifyShareRecord, deleteShareMedia, batchIdentifyShareRecords, getShareIdentifyTask } from '../utils/api/media'
const msg=useMessage(),rows=ref([]),loading=ref(false),show=ref(false),editing=ref(false),batchImportShow=ref(false),page=ref(1),taskState=ref(null),taskTimer=ref(null),form=ref({name:'',url:'',password:'',note:'',media:[]}),sources=[{label:'自动',value:'auto'},{label:'TMDB',value:'tmdb'},{label:'MetaTube',value:'metatube'}],pagination=ref({page:1,pageSize:20,itemCount:0}),taskPhase=computed(()=>taskState.value?.metadata?.phase||taskState.value?.status||'准备中')
const load=async()=>{loading.value=true;try{const r=await getShareRecords({page:page.value,page_size:20});const payload=r.data?.data||r.data||{};rows.value=Array.isArray(payload)?payload:(payload.data||[]);pagination.value={...pagination.value,page:page.value,itemCount:payload.total||rows.value.length}}finally{loading.value=false}}
const openBatchImport = () => { batchImportShow.value = true }
const openCreate=()=>{editing.value=false;form.value={name:'',url:'',password:'',note:'',media:[]};show.value=true};const openEdit=r=>{editing.value=true;form.value={id:r.id,version:r.version,name:r.name,url:r.url,password:r.password,note:r.note,media_type:r.media_type||'movie'};show.value=true};const save=async()=>{if(editing.value)await updateShareRecord(form.value.id,form.value);else await createShareRecord(form.value);show.value=false;load()};const pollTask=async id=>{try{const r=await getShareIdentifyTask(id);taskState.value=r.data?.data||r.data;if(['completed','failed','cancelled'].includes(taskState.value?.status)){clearInterval(taskTimer.value);taskTimer.value=null;load()}}catch(e){clearInterval(taskTimer.value);taskTimer.value=null}};const watchTask=async id=>{taskState.value={task_id:id,status:'pending',progress:0,metadata:{phase:'准备中',steps:[]}};await pollTask(id);taskTimer.value=setInterval(()=>pollTask(id),1000)};const batch=async retry=>{const r=await batchIdentifyShareRecords({retry_failed:retry});const id=r.data?.data?.task_id;if(id)await watchTask(id);else msg.error('批量识别任务创建失败')};const identifyRecord=async r=>{const response=await identifyShareRecord(r.id);const id=response.data?.data?.task_id;if(id)await watchTask(id);else msg.error('分享识别任务创建失败')};const identify=async m=>{await identifyShareMedia(m.id,{file_name:m.file_name,metadata_source:m.metadata_source,version:m.version});load()};const importFile=async({file})=>{for(const r of JSON.parse(await file.file.text()))await createShareRecord(r);load()};const remove=async r=>{await deleteShareRecord(r.id);load()};onBeforeUnmount(()=>{if(taskTimer.value)clearInterval(taskTimer.value)})
const identifiedMedia=r=>(r.media||[]).filter(m=>m.status==='identified'&&m.result?.success)
const columns=[{type:'expand',expandable:r=>identifiedMedia(r).length>0,renderExpand:r=>h(ShareMediaGallery,{media:identifiedMedia(r),onIdentify:identify,onSaved:load,onRemove:async m=>{await deleteShareMedia(r.id,m.id);await load()}})},{title:'名称',key:'name'},{title:'链接',key:'url',ellipsis:true},{title:'媒体数',render:r=>h('div',{},[h('div',{},'总数 '+(r.media?.length||0)),h('div',{class:'text-xs text-slate-500'},'已识别 '+identifiedMedia(r).length+' · 失败 '+(r.media||[]).filter(m=>m.status==='failed').length+' · 待识别 '+(r.media||[]).filter(m=>m.status!=='identified'&&m.status!=='failed'&&m.status!=='masked').length+' · 脱敏 '+(r.masked_count||0))])},{title:'操作',render:r=>h('div',{class:'flex gap-2'},[h(NButton,{size:'small',onClick:()=>openEdit(r)},{default:()=>'编辑'}),h(NButton,{size:'small',onClick:()=>remove(r)},{default:()=>'删除'}),h(NButton,{size:'small',type:'primary',onClick:()=>identifyRecord(r)},{default:()=>'识别'})])}]
onMounted(load)
</script>
