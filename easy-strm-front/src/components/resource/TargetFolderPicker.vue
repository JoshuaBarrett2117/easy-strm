<template>
  <div class="target-folder-picker">
    <n-tree-select
      :value="currentPath || null"
      :options="treeData"
      :loading="loading"
      :disabled="disabled || cloud115Id <= 0"
      :placeholder="cloud115Id > 0 ? placeholder : '请先选择 115 账号'"
      :on-load="loadChildren"
      key-field="key"
      label-field="label"
      children-field="children"
      clearable
      filterable
      @update:value="handleSelect"
    />
    <n-alert v-if="loadError" type="error" :closable="false" class="load-error">
      {{ loadError }}
    </n-alert>
    <n-alert v-if="spaceWarning" type="warning" :closable="false" class="space-warning">
      目标账号剩余空间可能不足，请确认后继续
    </n-alert>
  </div>
</template>

<script setup>
import { computed, onMounted, ref, watch } from 'vue'
import { NAlert, NTreeSelect } from 'naive-ui'
import { get115Files } from '../../utils/api/cloud115'
import { build115DirectoryNodes, extract115FileItems } from './targetFolderTree'

const props = defineProps({
  cloud115Id: { type: Number, default: 0 },
  defaultPath: { type: String, default: '' },
  selectedSize: { type: Number, default: 0 },
  availableSpace: { type: Number, default: 0 },
  placeholder: { type: String, default: '请选择 115 目录' },
  disabled: { type: Boolean, default: false }
})

const emit = defineEmits(['select', 'update:path'])
const currentPath = ref(props.defaultPath || '')
const treeData = ref([])
const loading = ref(false)
const loadError = ref('')

const spaceWarning = computed(() => Boolean(
  props.availableSpace && props.selectedSize && props.selectedSize > props.availableSpace
))

/** 请求指定账号目录，并只保留目录节点。 */
const requestDirectoryNodes = async (directoryId, parentPath) => {
  const response = await get115Files({
    cloud115_id: props.cloud115Id,
    cid: directoryId || '0',
    show_dir: 1,
    offset: 0,
    limit: 500
  })
  return build115DirectoryNodes(extract115FileItems(response), parentPath)
}

/** 加载账号根目录，配置值始终使用绝对路径。 */
const loadRootDirectory = async () => {
  if (props.cloud115Id <= 0) {
    treeData.value = []
    return
  }
  loading.value = true
  loadError.value = ''
  try {
    treeData.value = [{
      key: '/',
      label: '/（根目录）',
      path: '/',
      directoryId: '0',
      isLeaf: false,
      children: await requestDirectoryNodes('0', '/')
    }]
  } catch (error) {
    treeData.value = []
    loadError.value = error?.response?.data?.error || error?.message || '目录树加载失败'
  } finally {
    loading.value = false
  }
}

/** 懒加载子目录。 */
const loadChildren = async (node) => {
  if (Array.isArray(node.children)) return
  loadError.value = ''
  try {
    node.children = await requestDirectoryNodes(node.directoryId, node.path || node.key)
    treeData.value = [...treeData.value]
  } catch (error) {
    loadError.value = error?.response?.data?.error || error?.message || `目录“${node.label}”加载失败`
    return false
  }
}

const handleSelect = (path) => {
  currentPath.value = path || ''
  emit('select', currentPath.value)
  emit('update:path', currentPath.value)
}

watch(() => props.defaultPath, (path) => {
  currentPath.value = path || ''
})

watch(() => props.cloud115Id, () => {
  currentPath.value = props.defaultPath || ''
  loadRootDirectory()
})

onMounted(loadRootDirectory)
</script>

<style scoped>
.target-folder-picker { width: 100%; }
.load-error, .space-warning { margin-top: 8px; }
</style>
