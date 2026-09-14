<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="手动识别修正"
    class="w-[92vw] max-w-[560px]"
  >
    <section class="mb-4 rounded-2xl border border-cyan-500/10 bg-gradient-to-br from-cyan-500/10 to-amber-400/10 p-4 lg:p-5">
      <h3 class="text-base font-bold text-slate-800 dark:text-white">修正整理预览中的识别结果</h3>
      <p class="mt-2 text-sm leading-relaxed text-slate-500 dark:text-slate-400">
        这里的修改会直接回写到整理预览中，用于下一次预览或正式执行整理。
      </p>
    </section>

    <n-form :model="form" label-placement="left" label-width="110">
      <n-form-item label="原文件名">
        <n-input v-model:value="form.file_name" disabled />
      </n-form-item>

      <n-form-item label="媒体类型">
        <n-select v-model:value="form.media_type" :options="mediaTypeOptions" />
      </n-form-item>

      <n-form-item label="标题">
        <n-input v-model:value="form.title" placeholder="请输入标题" />
      </n-form-item>

      <n-form-item label="原名">
        <n-input v-model:value="form.original_title" placeholder="AI可辅助预填，仍需选择数据源候选" />
      </n-form-item>

      <n-form-item label="年份">
        <n-input-number v-model:value="form.year" :min="0" :max="9999" class="w-full" />
      </n-form-item>

      <n-form-item v-if="form.media_type === 'tv'" label="季数">
        <n-input-number v-model:value="form.season" :min="0" :max="999" class="w-full" />
      </n-form-item>

      <n-form-item v-if="form.media_type === 'tv'" label="集数">
        <n-input-number v-model:value="form.episode" :min="0" :max="9999" class="w-full" />
      </n-form-item>

      <n-form-item label="TMDB ID">
        <n-input-number v-model:value="form.tmdb_id" :min="0" :max="999999999" class="w-full" />
      </n-form-item>
    </n-form>

    <template #action>
      <div class="flex flex-wrap justify-end gap-2">
        <n-button :loading="aiLoading" @click="handleAIParse">AI解析文件名</n-button>
        <n-button @click="handleSearchTmdb">从数据源选择</n-button>
        <n-button v-if="form.override_key" @click="handleClearOverride">清除修改</n-button>
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" @click="handleApply">应用到预览</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, watch } from 'vue'
import { NModal, NForm, NFormItem, NInput, NInputNumber, NSelect, NButton, useMessage } from 'naive-ui'
import { assistMediaFilename } from '../../utils/api/ai'

const props = defineProps({
  form: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['apply', 'clear', 'search-tmdb'])

const visible = defineModel('visible', { type: Boolean, default: false })
const message = useMessage()
const aiLoading = ref(false)
let requestGeneration = 0

watch([visible, () => props.form.file_name], () => {
  requestGeneration++
  aiLoading.value = false
}, { flush: 'sync' })

const mediaTypeOptions = [
  { label: '自动判断', value: 'unknown' },
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' }
]

const handleApply = () => {
  if (!props.form.title?.trim()) {
    message.warning('请输入识别标题')
    return
  }
  if (props.form.ai_suggested && !props.form.source_candidate_selected) {
    message.warning('AI建议仅用于预填，请先从数据源选择并核验候选')
    return
  }
  emit('apply', { ...props.form })
}

const handleClearOverride = () => {
  emit('clear', props.form.override_key)
}

const handleSearchTmdb = () => {
  emit('search-tmdb', { ...props.form })
}

const handleAIParse = async () => {
  const generation = ++requestGeneration
  const form = props.form
  const before = JSON.stringify(form)
  aiLoading.value = true
  try {
    const response = await assistMediaFilename(form.file_name)
    if (generation !== requestGeneration || !visible.value || props.form !== form) return
    if (JSON.stringify(form) !== before) {
      message.info('表单已修改，本次AI建议未覆盖你的输入')
      return
    }
    const hint = response?.data?.data ?? response?.data ?? {}
    props.form.title = hint.title || ''
    props.form.original_title = hint.original_title || ''
    props.form.year = hint.year || 0
    if (form.media_type !== 'movie' && form.media_type !== 'tv' && ['movie', 'tv'].includes(hint.media_type)) form.media_type = hint.media_type
    // 查询条件变化后清除旧身份，避免把AI建议与旧数据源ID混合提交。
    props.form.tmdb_id = 0
    props.form.metadata_source = ''
    props.form.metadata_id = ''
    props.form.metadata_provider = ''
    form.ai_suggested = true
    form.source_candidate_selected = false
    message.success('AI建议已预填，请继续从数据源选择候选')
  } catch (error) {
    if (generation !== requestGeneration || !visible.value) return
    message.error(error.response?.data?.error || error.message || 'AI解析文件名失败')
  } finally {
    if (generation === requestGeneration) aiLoading.value = false
  }
}
</script>
