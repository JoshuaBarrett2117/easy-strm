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
        <n-button @click="handleSearchTmdb">从 TMDB 选择</n-button>
        <n-button v-if="form.override_key" @click="handleClearOverride">清除修改</n-button>
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" @click="handleApply">应用到预览</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { NModal, NForm, NFormItem, NInput, NInputNumber, NSelect, NButton, useMessage } from 'naive-ui'

const props = defineProps({
  form: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['apply', 'clear', 'search-tmdb'])

const visible = defineModel('visible', { type: Boolean, default: false })
const message = useMessage()

const mediaTypeOptions = [
  { label: '电影', value: 'movie' },
  { label: '剧集', value: 'tv' }
]

const handleApply = () => {
  if (!props.form.title?.trim()) {
    message.warning('请输入识别标题')
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
</script>
