<template>
  <n-input
    v-bind="$attrs"
    :value="displayValue"
    :disabled="disabled"
    :type="visible ? 'text' : 'password'"
    @update:value="updateValue"
  >
    <template #suffix>
      <n-button
        text
        :loading="loading"
        :disabled="disabled"
        :aria-label="visible ? '隐藏明文' : '查看明文'"
        :title="visible ? '隐藏明文' : '查看明文'"
        @click="toggleVisible"
      >
        <n-icon :component="visible ? EyeOffOutline : EyeOutline" />
      </n-button>
    </template>
  </n-input>
</template>

<script setup>
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { NButton, NIcon, NInput } from 'naive-ui'
import { EyeOutline, EyeOffOutline } from '@vicons/ionicons5'
import { revealConfigSecret } from '../../utils/api/configSecret'
import { message } from '../../utils/ui/feedback'

defineOptions({ inheritAttrs: false })
const props = defineProps({
  value: { type: String, default: '' },
  secretKey: { type: String, required: true },
  serverId: { type: Number, default: 0 },
  hasSaved: Boolean,
  disabled: Boolean,
  resetKey: { default: null }
})
const emit = defineEmits(['update:value'])
const visible = ref(false)
const loading = ref(false)
const savedValue = ref('')
let requestVersion = 0
// 查看旧值不写入表单，避免仅查看就把旧配置重新提交。
const displayValue = computed(() => props.value || (visible.value ? savedValue.value : ''))
const reset = () => {
  requestVersion++
  visible.value = false
  loading.value = false
  savedValue.value = ''
}
const updateValue = value => {
  requestVersion++
  loading.value = false
  savedValue.value = ''
  emit('update:value', value)
}
const toggleVisible = async () => {
  if (visible.value) { reset(); return }
  if (props.value || !props.hasSaved) { visible.value = true; return }
  const version = ++requestVersion
  loading.value = true
  try {
    const response = await revealConfigSecret(props.secretKey, props.serverId)
    if (version !== requestVersion) return
    savedValue.value = response.data.data.value || ''
    visible.value = true
    if (!savedValue.value) message.info('尚未保存此配置')
  } catch (error) {
    if (version === requestVersion) message.error(error?.response?.data?.error || '读取已保存配置失败')
  } finally {
    if (version === requestVersion) loading.value = false
  }
}
watch(() => [props.secretKey, props.serverId, props.resetKey, props.hasSaved], reset)
watch(() => props.value, value => { if (loading.value || !value) reset() })
onBeforeUnmount(reset)
</script>
