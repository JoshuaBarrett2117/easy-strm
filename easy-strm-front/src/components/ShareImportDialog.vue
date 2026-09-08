<template>
  <n-modal :show="show" preset="card" title="批量导入分享"
    style="width:1000px;max-width:calc(100vw - 32px);max-height:calc(100dvh - 32px)"
    :content-style="{ overflow: 'auto', minHeight: 0 }" :closable="!busy"
    :mask-closable="!busy" :close-on-esc="!busy" @update:show="$emit('update:show', $event)">
    <n-form-item label="分享内容">
      <n-input v-model:value="text" type="textarea" :disabled="busy"
        :autosize="{ minRows: 5, maxRows: 10 }" @update:value="clearPreview"
        placeholder="粘贴分享文案，支持名称前置/后置、Markdown链接、独立访问码及share.115.com链接" />
    </n-form-item>
    <n-button :loading="parsing" :disabled="busy || !text.trim()" @click="parse">解析预览</n-button>
    <div v-if="preview" class="mt-4">
      <n-alert type="info">
        解析 {{ entries.length }} 条分享，合并 {{ preview.duplicates }} 条重复，忽略 {{ preview.ignored.length }} 行。
        当前选中 {{ selected.length }} 条。去重范围为本次粘贴内容。
        有提示的记录默认不选中，请核对候选及原文后勾选。
      </n-alert>
      <details v-if="preview.ignored.length" class="mt-2">
        <summary>查看忽略的内容</summary>
        <div v-for="(line, index) in preview.ignored" :key="index" class="import-source">{{ line }}</div>
      </details>
      <div class="import-preview">
        <n-card v-for="entry in entries" :key="entry.share_code" size="small" class="mt-2" data-testid="import-entry">
          <n-checkbox v-model:checked="entry.selected" :disabled="busy || entry.imported">
            {{ entry.imported ? '已导入' : '导入此分享' }} · 原文第 {{ entry.lines.join('、') }} 行
          </n-checkbox>
          <n-alert v-if="entry.warnings.length" type="warning" class="mt-2">{{ entry.warnings.join('；') }}</n-alert>
          <n-alert v-if="entry.error" type="error" class="mt-2">{{ entry.error }}</n-alert>
          <n-form-item label="名称" class="mt-2">
            <n-input v-model:value="entry.name" :disabled="busy || entry.imported" placeholder="请填写名称" />
          </n-form-item>
          <n-select v-if="entry.names.length > 1" v-model:value="entry.name"
            :options="entry.names.map(value => ({ label: value, value }))" :disabled="busy || entry.imported" placeholder="选择名称候选" />
          <n-form-item label="链接">
            <n-input :value="entry.url" readonly />
          </n-form-item>
          <n-form-item label="访问码">
            <n-input v-model:value="entry.password" :disabled="busy || entry.imported" placeholder="无访问码可留空" />
          </n-form-item>
          <n-select v-if="entry.passwords.length > 1" v-model:value="entry.password"
            :options="entry.passwords.map(value => ({ label: value, value }))" :disabled="busy || entry.imported" placeholder="选择访问码候选" />
        </n-card>
      </div>
    </div>
    <template #footer>
      <n-space justify="end">
        <n-button :disabled="busy" @click="$emit('update:show', false)">取消</n-button>
        <n-button type="primary" :loading="importing" :disabled="busy || !selected.length" @click="submit">
          导入选中（{{ selected.length }}）
        </n-button>
      </n-space>
    </template>
  </n-modal>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { NAlert, NButton, NCard, NCheckbox, NFormItem, NInput, NModal, NSelect, NSpace, useMessage } from 'naive-ui'
import { createShareRecord, parseShareImport } from '../utils/api/media'

const props = defineProps({ show: Boolean })
const emit = defineEmits(['update:show', 'imported'])
const msg = useMessage()
const text = ref('')
const preview = ref(null)
const entries = ref([])
const parsing = ref(false)
const importing = ref(false)
const busy = computed(() => parsing.value || importing.value)
const selected = computed(() => entries.value.filter(entry => entry.selected && !entry.imported))

const clearPreview = () => { preview.value = null; entries.value = [] }
const parse = async () => {
  if (busy.value) return
  parsing.value = true
  clearPreview()
  try {
    const response = await parseShareImport(text.value)
    preview.value = response.data.data
    entries.value = preview.value.records.map(entry => ({
      ...entry, selected: !entry.warnings.length, imported: false, error: ''
    }))
  } catch {
    // API 封装统一展示解析错误；保持输入以便修改。
  } finally { parsing.value = false }
}
const submit = async () => {
  if (busy.value || !selected.value.length) return
  const records = [...selected.value]
  if (records.some(entry => !entry.name.trim())) { msg.error('请为选中的分享填写名称'); return }
  importing.value = true
  let imported = 0
  try {
    for (const entry of records) {
      entry.error = ''
      try {
        await createShareRecord({ name: entry.name.trim(), url: entry.url, password: entry.password.trim() })
        entry.imported = true
        entry.selected = false
        imported++
      } catch (error) {
        entry.error = error?.response?.data?.error || error?.message || '导入失败，请重试'
        msg.error(`已导入 ${imported} 条，其余选中项尚未导入，可直接重试`)
        return
      }
    }
    msg.success(`已导入 ${imported} 条分享`)
    if (entries.value.every(entry => entry.imported)) emit('update:show', false)
  } finally {
    importing.value = false
    if (imported) emit('imported')
  }
}
watch(() => props.show, show => {
  if (show) { text.value = ''; clearPreview() }
})
</script>

<style scoped>
.import-preview { max-height: 48vh; overflow: auto; margin-top: 12px; }
.import-source { overflow-wrap: anywhere; font-size: 12px; margin-top: 4px; }
</style>
