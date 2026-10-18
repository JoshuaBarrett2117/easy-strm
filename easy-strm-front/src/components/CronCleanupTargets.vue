<template>
  <section class="cleanup-targets">
    <n-alert type="warning" title="清空目录内容，请谨慎配置">
      全局仅有一个清空任务。将删除所有启用目标中的文件和子目录，保留目标目录本身；禁止根目录和同账号父子目录。删除后请到 115 回收站确认是否可恢复。
    </n-alert>
    <div v-for="(target, index) in modelValue" :key="index" class="cleanup-target">
      <n-form-item :label="`目标 ${index + 1} · 115账号`">
        <n-select v-model:value="target.cloud115_id" :options="accounts" :loading="loading" placeholder="选择账号" filterable />
      </n-form-item>
      <n-form-item label="完整目录路径">
        <n-input v-model:value="target.dir_path" placeholder="例如 /EmbyCache（支持手动输入）" />
      </n-form-item>
      <n-space justify="space-between">
        <n-checkbox v-model:checked="target.enabled">启用此目标</n-checkbox>
        <n-button size="small" type="error" @click="remove(index)">移除目标</n-button>
      </n-space>
    </div>
    <n-button :disabled="modelValue.length >= 100" @click="add">添加账号和目录</n-button>
    <p>共 {{ modelValue.length }} 个目标，启用 {{ modelValue.filter(target => target.enabled).length }} 个。按账号、目录依次执行；单个目标失败会继续其他目标，详情保存在执行记录中。</p>
  </section>
</template>
<script setup>
import { onMounted, ref } from 'vue'
import { NAlert, NButton, NCheckbox, NFormItem, NInput, NSelect, NSpace, useMessage } from 'naive-ui'
import { getCloud115List } from '../utils/api/cloud115'
const props = defineProps({ modelValue: { type: Array, default: () => [] } })
const emit = defineEmits(['update:modelValue'])
const accounts = ref([]), loading = ref(false), message = useMessage()
const add = () => emit('update:modelValue', [...props.modelValue, { cloud115_id: null, dir_path: '', enabled: true }])
const remove = index => emit('update:modelValue', props.modelValue.filter((_, i) => i !== index))
onMounted(async () => {
  loading.value = true
  try {
    const response = await getCloud115List()
    accounts.value = response.data.data.map(account => ({ label: account.name, value: account.id }))
  } catch (error) {
    message.error(error.response?.data?.error || '账号列表加载失败，请重新打开弹窗')
  } finally { loading.value = false }
})
</script>
<style scoped>
.cleanup-targets { display: grid; gap: 12px; }
.cleanup-target { border: 1px solid var(--border-color, #ddd); border-radius: 6px; padding: 12px; }
</style>
