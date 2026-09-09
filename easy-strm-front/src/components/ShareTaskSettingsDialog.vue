<template>
  <n-modal :show="show" preset="card" title="分享识别任务设置" style="width:520px;max-width:calc(100vw - 32px)" :closable="!saving" :mask-closable="!saving" @update:show="$emit('update:show',$event)">
    <n-spin :show="loading">
      <n-form-item label="任务总时限">
        <n-select v-model:value="mode" :options="[{label:'无限制',value:'unlimited'},{label:'自定义时限',value:'limited'}]" />
      </n-form-item>
      <n-form-item v-if="mode==='limited'" label="分钟（扫描与识别合计）">
        <n-input-number v-model:value="minutes" :min="1" :max="43200" />
      </n-form-item>
      <n-alert type="info">无限制时不会因任务运行时长终止，仍可在任务中心手动取消。外部接口保留各自请求超时。保存后仅对新发起的任务生效。</n-alert>
    </n-spin>
    <template #footer><n-button type="primary" :loading="saving" :disabled="loading || !loaded" @click="save">保存设置</n-button></template>
  </n-modal>
</template>
<script setup>
import { ref, watch } from 'vue'
import { NAlert,NButton,NFormItem,NInputNumber,NModal,NSelect,NSpin,useMessage } from 'naive-ui'
import { getShareTaskSettings,saveShareTaskSettings } from '../utils/api/media'
const props=defineProps({show:Boolean})
const emit=defineEmits(['update:show'])
const msg=useMessage(),mode=ref('unlimited'),minutes=ref(30),loading=ref(false),loaded=ref(false),saving=ref(false)
let version=0
watch(()=>props.show,async show=>{
  const request=++version
  if(!show)return
  loading.value=true;loaded.value=false
  try{
    const value=(await getShareTaskSettings()).data.data.timeout_minutes
    if(request!==version)return
    mode.value=value===0?'unlimited':'limited';minutes.value=value||30;loaded.value=true
  }catch{}finally{if(request===version)loading.value=false}
})
const save=async()=>{
  if(saving.value)return
  if(mode.value==='limited'&&(!Number.isInteger(minutes.value)||minutes.value<1)){msg.error('请输入正整数分钟');return}
  saving.value=true
  try{await saveShareTaskSettings({timeout_minutes:mode.value==='unlimited'?0:minutes.value});msg.success('已保存，下次发起任务时生效');emit('update:show',false)}catch{}finally{saving.value=false}
}
</script>

