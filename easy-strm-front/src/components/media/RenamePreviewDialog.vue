<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="重命名预览"
    class="w-[96vw] max-w-[760px]"
  >
    <n-spin :show="loading">
      <div class="max-h-[500px] overflow-y-auto sm:max-h-none">
        <section class="mb-4 grid grid-cols-1 gap-3 sm:grid-cols-2">
          <article class="rounded-2xl bg-slate-100 p-4 dark:border dark:border-white/10 dark:bg-white/5">
            <span class="block text-xs text-slate-400 dark:text-slate-500">待处理项目</span>
            <strong class="mt-2 block text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ previewList.length }}</strong>
          </article>
          <article class="rounded-2xl bg-slate-100 p-4 dark:border dark:border-white/10 dark:bg-white/5">
            <span class="block text-xs text-slate-400 dark:text-slate-500">当前动作</span>
            <strong class="mt-2 block text-2xl font-extrabold text-slate-800 dark:text-white">批量重命名</strong>
          </article>
        </section>
        <div class="overflow-x-auto">
          <n-data-table
            :columns="previewColumns"
            :data="previewList"
            :striped="true"
            :scroll-x="650"
          />
        </div>
      </div>
    </n-spin>

    <template #action>
      <div class="flex flex-wrap justify-end gap-2">
        <n-button @click="visible = false">取消</n-button>
        <n-button type="primary" :loading="loading" @click="handleExecute">执行重命名</n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { h } from 'vue'
import { NModal, NSpin, NDataTable, NButton, NIcon, useMessage } from 'naive-ui'
import { ArrowForwardOutline } from '@vicons/ionicons5'
import { batchExecuteRename } from '../../utils/api/media'

const props = defineProps({
  previewList: {
    type: Array,
    default: () => []
  },
  sourceId: {
    type: Number,
    default: null
  }
})

const emit = defineEmits(['execute-success'])

const visible = defineModel('visible', { type: Boolean, default: false })
const loading = defineModel('loading', { type: Boolean, default: false })
const message = useMessage()

const previewColumns = [
  { title: '原文件名', key: 'original_name', minWidth: 300 },
  {
    title: '',
    key: 'arrow',
    width: 50,
    align: 'center',
    render: () => h(NIcon, { component: ArrowForwardOutline, class: 'text-slate-400' })
  },
  { title: '新文件名', key: 'new_name', minWidth: 300 }
]

const handleExecute = async () => {
  loading.value = true
  try {
    const items = props.previewList.map(item => ({
      source_id: props.sourceId,
      file_id: item.file_id,
      new_name: item.new_name
    }))

    const response = await batchExecuteRename({ items })
    const data = response.data.data || response.data || {}
    const successCount = data.success || 0
    const failedCount = data.failed || 0
    if (failedCount > 0 && successCount === 0) {
      message.error(`批量重命名失败：共 ${failedCount} 项未执行成功`)
      return
    }
    if (failedCount > 0) {
      message.warning(`批量重命名部分完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
    } else {
      message.success(`批量重命名完成：成功 ${successCount} 项，失败 ${failedCount} 项`)
    }
    visible.value = false
    emit('execute-success')
  } catch (error) {
    console.error('[RenamePreviewDialog] 批量重命名失败:', error)
    const errorMsg = error.response?.data?.error || error.message || '批量重命名失败'
    message.error(errorMsg)
  } finally {
    loading.value = false
  }
}
</script>
