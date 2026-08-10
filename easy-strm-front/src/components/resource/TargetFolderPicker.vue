<template>
  <div class="target-folder-picker">
    <!-- collapsed: 显示当前路径 + 展开按钮 -->
    <div v-if="state === 'collapsed'" class="collapsed-row">
      <div class="path-display">
        <n-icon size="16" :component="FolderOpenOutline" color="#f59e0b" />
        <span class="path-label">目标目录：</span>
        <span class="path-value">{{ currentPath || '/ (根目录)' }}</span>
      </div>
      <n-button size="small" secondary @click="expand">
        选择
      </n-button>
    </div>

    <!-- expanded / selecting: 目录树 -->
    <div v-else class="expanded-panel">
      <div class="panel-header">
        <span class="panel-title">选择目标目录</span>
        <n-button size="tiny" text @click="collapse">
          收起
        </n-button>
      </div>

      <div class="tree-container">
        <n-spin v-if="loading" size="small" />
        <n-tree
          v-else
          :data="treeData"
          :selected-keys="selectedKeys"
          :default-expanded-keys="defaultExpandedKeys"
          :node-props="nodeProps"
          :on-load="loadChildren"
          block-line
          selectable
          @update:selected-keys="handleSelect"
        />
      </div>

      <n-alert
        v-if="loadError"
        type="error"
        :closable="false"
        class="load-error"
      >
        {{ loadError }}
      </n-alert>

      <!-- 空间不足警告 -->
      <n-alert
        v-if="spaceWarning"
        type="warning"
        :closable="false"
        class="space-warning"
      >
        目标账号剩余空间可能不足，请确认后继续
      </n-alert>

      <div class="panel-footer">
        <n-button size="small" @click="collapse">
          取消
        </n-button>
        <n-button
          size="small"
          type="primary"
          :disabled="!selectedKeys.length || !!spaceWarning"
          @click="confirm"
        >
          确认选择
        </n-button>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import {
  NButton,
  NIcon,
  NTree,
  NSpin,
  NAlert
} from 'naive-ui'
import { FolderOpenOutline } from '@vicons/ionicons5'
import { get115Files } from '../../utils/api/cloud115'
import { build115DirectoryNodes, extract115FileItems } from './targetFolderTree'

const props = defineProps({
  cloud115Id: {
    type: Number,
    default: 0
  },
  defaultPath: {
    type: String,
    default: ''
  },
  selectedSize: {
    type: Number,
    default: 0
  },
  availableSpace: {
    type: Number,
    default: 0
  }
})

const emit = defineEmits(['select', 'update:path'])

// ===== 状态 =====
const state = ref('collapsed') // collapsed | expanded | selecting
const currentPath = ref(props.defaultPath || '')
const treeData = ref([])
const selectedKeys = ref([])
const defaultExpandedKeys = ref([])
const loading = ref(false)
const loadError = ref('')

// 空间不足警告
const spaceWarning = computed(() => {
  if (!props.availableSpace || !props.selectedSize) return false
  return props.selectedSize > props.availableSpace
})

/** 展开选择面板 */
const expand = async () => {
  state.value = 'expanded'
  if (props.cloud115Id > 0) {
    await loadRootDirectory()
  } else {
    treeData.value = []
    loadError.value = '请先选择可用的 115 账号'
  }
}

/** 收起选择面板 */
const collapse = () => {
  state.value = 'collapsed'
}

/** 确认选择 */
const confirm = () => {
  if (selectedKeys.value.length > 0) {
    const path = selectedKeys.value[0]
    currentPath.value = path
    emit('select', path)
    emit('update:path', path)
  }
  state.value = 'collapsed'
}

/** 加载根目录 */
const loadRootDirectory = async () => {
  loading.value = true
  loadError.value = ''
  try {
    const children = await requestDirectoryNodes('0', '/')
    treeData.value = [{
      key: '/',
      label: '/ (根目录)',
      path: '/',
      directoryId: '0',
      isLeaf: false,
      children
    }]
    defaultExpandedKeys.value = ['/']
    selectedKeys.value = [currentPath.value || '/']
  } catch (err) {
    treeData.value = [
      { key: '/', label: '/ (根目录)', isLeaf: false, children: [] }
    ]
    defaultExpandedKeys.value = ['/']
    loadError.value = err?.response?.data?.error || err?.message || '目录加载失败，请稍后重试'
  } finally {
    loading.value = false
  }
}

/** 请求指定 115 目录并转换为树节点。 */
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

/** 懒加载子目录，空数组会让 Naive UI 将节点识别为叶子。 */
const loadChildren = async (node) => {
  if (Array.isArray(node.children)) return
  loadError.value = ''
  try {
    node.children = await requestDirectoryNodes(node.directoryId, node.path || node.key)
    treeData.value = [...treeData.value]
  } catch (err) {
    loadError.value = err?.response?.data?.error || err?.message || `目录“${node.label}”加载失败`
    return false
  }
}

/** 处理选择 */
const handleSelect = (keys) => {
  selectedKeys.value = keys
  state.value = 'selecting'
}

/** 节点属性 */
const nodeProps = ({ option }) => {
  return {
    onClick: () => {
      handleSelect([option.key])
    }
  }
}

// 初始化
watch(() => props.defaultPath, (val) => {
  currentPath.value = val || ''
})

watch(() => props.cloud115Id, async () => {
  treeData.value = []
  selectedKeys.value = []
  loadError.value = ''
  if (state.value !== 'collapsed' && props.cloud115Id > 0) {
    await loadRootDirectory()
  }
})
</script>

<style scoped>
.target-folder-picker {
  margin-bottom: 16px;
}

.collapsed-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 10px;
  background: var(--bg-subtle, #f9fafb);
}

.path-display {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.path-label {
  font-size: 13px;
  color: var(--text-secondary, #6b7280);
}

.path-value {
  font-size: 13px;
  font-weight: 600;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.expanded-panel {
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  overflow: hidden;
  background: var(--card-bg, #fff);
}

.panel-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color, #e5e7eb);
  background: var(--bg-subtle, #f9fafb);
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
}

.tree-container {
  padding: 8px 12px;
  max-height: 300px;
  overflow-y: auto;
}

.space-warning {
  margin: 8px 12px 0;
}

.load-error {
  margin: 8px 12px 0;
}

.panel-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 10px 14px;
  border-top: 1px solid var(--border-color, #e5e7eb);
  background: var(--bg-subtle, #f9fafb);
}
</style>
