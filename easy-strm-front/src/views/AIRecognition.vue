<template>
  <div class="ai-settings">
    <n-card title="AI 辅助识别" :bordered="false">
      <n-alert type="info" class="mb-4">用于分享媒体识别。AI只提供片名、年份和类型建议，仍由媒体数据库核验。原始路径不会修改；启用后所选场景的路径会发送到配置的端点。</n-alert>
      <n-spin :show="loading">
        <n-form label-placement="top">
          <n-form-item label="启用 AI 辅助"><n-switch v-model:value="form.enabled" /></n-form-item>
          <n-form-item label="接口地址">
            <n-input v-model:value="form.base_url" placeholder="https://api.openai.com/v1" @update:value="models=[]" />
          </n-form-item>
          <n-form-item label="API Key">
            <n-input v-model:value="form.api_key" type="password" show-password-on="click"
              :placeholder="form.has_api_key ? '已保存，留空保留（更换地址后请重新填写）' : '填写端点密钥，无需认证的本地服务可留空'" />
          </n-form-item>
          <n-checkbox v-model:checked="form.clear_api_key">清除已保存密钥</n-checkbox>
          <n-form-item label="模型" class="mt-4">
            <n-space vertical style="width:100%">
              <n-select v-model:value="form.model" :options="models.map(value=>({label:value,value}))" filterable tag placeholder="获取后选择，或输入模型 ID" />
              <n-button :loading="fetchingModels" @click="fetchModels">获取模型列表</n-button>
              <span class="hint">使用表单中的连接信息获取模型。列表可能包含非聊天模型，可用下方测试确认兼容性。</span>
            </n-space>
          </n-form-item>
          <n-form-item label="请求超时（秒）"><n-input-number v-model:value="form.timeout_seconds" :min="1" :max="120" /></n-form-item>
          <n-form-item label="调用场景">
            <n-checkbox-group v-model:value="form.scenes">
              <n-space vertical>
                <n-checkbox value="complex_title">常规识别失败且标题复杂：原盘、DIY、字幕等信息较多</n-checkbox>
                <n-checkbox value="uncertain_type">常规识别失败且类型不确定：电影和剧集查询均未确认</n-checkbox>
                <n-checkbox value="no_match">无可靠匹配：常规检索无法确认媒体</n-checkbox>
              </n-space>
            </n-checkbox-group>
          </n-form-item>
          <p class="hint">每条媒体最多调用一次AI；关闭或未勾选的场景不会调用。AI故障时保留常规识别能力，未确认的结果不会作为成功保存。</p>
          <n-form-item label="提示词">
            <n-input v-model:value="form.prompt" type="textarea" :autosize="{minRows:5,maxRows:12}" placeholder="留空使用默认影视解析提示词" />
          </n-form-item>
          <n-space><n-button type="primary" :loading="saving" @click="save">保存配置</n-button></n-space>
        </n-form>
      </n-spin>
    </n-card>
    <n-card title="连接与提示词测试" class="mt-4">
      <n-form-item label="测试文件名或路径">
        <n-input v-model:value="testFilename" type="textarea" :autosize="{minRows:2,maxRows:5}" />
      </n-form-item>
      <n-button :loading="testing" @click="test">测试 AI 识别</n-button>
      <p class="hint">测试使用当前未保存的表单，发送一次模型请求；不会创建媒体记录。</p>
      <n-alert v-if="testError" type="error" class="mt-4">{{ testError }}</n-alert>
      <n-descriptions v-if="testResult" bordered :column="1" class="mt-4">
        <n-descriptions-item label="片名">{{ testResult.title || '未知' }}</n-descriptions-item>
        <n-descriptions-item label="原名">{{ testResult.original_title || '未知' }}</n-descriptions-item>
        <n-descriptions-item label="年份">{{ testResult.year || '未知' }}</n-descriptions-item>
        <n-descriptions-item label="类型">{{ ({movie:'电影',tv:'电视剧',unknown:'待确认'})[testResult.media_type] }}</n-descriptions-item>
      </n-descriptions>
    </n-card>
  </div>
</template>
<script setup>
import { onMounted, reactive, ref } from 'vue'
import { NAlert,NButton,NCard,NCheckbox,NCheckboxGroup,NDescriptions,NDescriptionsItem,NForm,NFormItem,NInput,NInputNumber,NSelect,NSpace,NSpin,NSwitch,useMessage } from 'naive-ui'
import { getAIConfig,saveAIConfig,getAIModels,testAIRecognition } from '../utils/api/ai'
const message=useMessage()
const form=reactive({enabled:false,base_url:'https://api.openai.com/v1',api_key:'',has_api_key:false,clear_api_key:false,model:'',timeout_seconds:30,scenes:['no_match'],prompt:''})
const loading=ref(false),saving=ref(false),fetchingModels=ref(false),testing=ref(false)
const models=ref([]),testResult=ref(null),testError=ref('')
const testFilename=ref('七龙珠[国粤日语 153集全]Dragonball.1986/Dragonball.1986.D11.Blu-ray.iso')
const load=async()=>{
  loading.value=true
  try{Object.assign(form,(await getAIConfig()).data.data);if(form.model || form.has_api_key) await fetchModels()}catch{}finally{loading.value=false}
}
const save=async()=>{
  if(saving.value)return
  saving.value=true
  try{Object.assign(form,(await saveAIConfig({...form})).data.data);form.clear_api_key=false;message.success('AI配置已保存')}catch{}finally{saving.value=false}
}
const fetchModels=async()=>{
  if(fetchingModels.value)return
  fetchingModels.value=true
  try{models.value=(await getAIModels({...form})).data.data || [];message.success(`获取到 ${models.value.length} 个模型`)}catch{}finally{fetchingModels.value=false}
}
const test=async()=>{
  if(testing.value)return
  testing.value=true;testResult.value=null;testError.value=''
  try{testResult.value=(await testAIRecognition({...form},testFilename.value)).data.data}catch(error){testError.value=error?.response?.data?.error || 'AI测试失败'}finally{testing.value=false}
}
onMounted(load)
</script>
<style scoped>
.ai-settings{max-width:960px;margin:0 auto}
.hint{font-size:12px;opacity:.65;line-height:1.7}
</style>

