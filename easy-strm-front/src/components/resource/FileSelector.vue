<template>
  <div class="file-selector">
    <!-- 工具栏 -->
    <div v-if="state !== 'loading' && state !== 'empty'" class="toolbar">
      <div class="toolbar-left">
        <n-checkbox
          :checked="isAllSelected"
          :indeterminate="isIndeterminate"
          @update:checked="handleSelectAll"
        >
          全选
        </n-checkbox>
        <n-checkbox
          :checked="isAllDeselected"
          @update:checked="handleDeselectAll"
          style="margin-left:4px"
        >
          反选
        </n-checkbox>
      </div>
      <div class="toolbar-right">
        <n-select
          v-model:value="typeFilter"
          size="small"
          placeholder="类型筛选"
          clearable
          :options="typeFilterOptions"
          style="width:120px"
        />
        <n-input
          v-model:value="searchKeyword"
          size="small"
          placeholder="搜索文件名..."
          clearable
          style="width:180px"
        >
          <template #prefix>
            <n-icon :component="SearchOutline" size="14" />
          </template>
        </n-input>
      </div>
    </div>

    <!-- 内容区域 -->
    <div class="content-area">
      <!-- loading -->
      <div v-if="state === 'loading'" class="state-center">
        <n-spin size="medium" />
        <p class="state-hint">正在加载文件列表...</p>
      </div>

      <!-- empty -->
      <div v-else-if="state === 'empty'" class="state-center">
        <n-empty description="暂无文件" />
      </div>

      <!-- loaded / filtered -->
      <div v-else class="file-tree-wrapper" :class="{ 'virtual-scroll': displayFiles.length > 50 }">
        <div
          v-for="node in treeData"
          :key="node.key"
          class="tree-node"
        >
          <!-- 文件夹节点 -->
          <div
            v-if="node.isDir"
            class="folder-node"
            :style="{ paddingLeft: (node.level * 16) + 'px' }"
          >
            <div
              class="folder-header"
              @click="toggleFolder(node.key)"
            >
              <n-icon size="14" :component="node.expanded ? ChevronDownOutline : ChevronForwardOutline" />
              <n-icon size="16" :component="FolderOutline" color="#f59e0b" />
              <n-checkbox
                :checked="node.checked"
                :indeterminate="node.indeterminate"
                @click.stop
                @update:checked="(val) => toggleNodeCheck(node, val)"
              />
              <span class="folder-name">{{ node.name }}</span>
              <span class="folder-count">({{ node.children?.length || 0 }})</span>
            </div>
            <!-- 展开子文件 -->
            <template v-if="node.expanded">
              <div
                v-for="child in node.children"
                :key="child.key"
                class="file-node"
                :style="{ paddingLeft: ((node.level + 1) * 16) + 'px' }"
              >
                <n-icon size="14" :component="getFileIcon(child.type)" :color="getFileIconColor(child.type)" />
                <n-checkbox
                  :checked="child.checked"
                  @update:checked="(val) => toggleNodeCheck(child, val)"
                />
                <span class="file-name">{{ child.name }}</span>
                <span class="file-size">{{ formatSize(child.size) }}</span>
              </div>
            </template>
          </div>

          <!-- 顶层文件节点（非目录） -->
          <div
            v-else
            class="file-node top-level"
            :style="{ paddingLeft: (node.level * 16) + 'px' }"
          >
            <n-icon size="14" :component="getFileIcon(node.type)" :color="getFileIconColor(node.type)" />
            <n-checkbox
              :checked="node.checked"
              @update:checked="(val) => toggleNodeCheck(node, val)"
            />
            <span class="file-name">{{ node.name }}</span>
            <span class="file-size">{{ formatSize(node.size) }}</span>
          </div>
        </div>
      </div>
    </div>

    <!-- 底部统计栏 -->
    <div v-if="state !== 'loading' && state !== 'empty'" class="sticky-footer">
      <div class="footer-stats">
        <span>共 <strong>{{ totalCount }}</strong> 个</span>
        <span class="footer-sep">|</span>
        <span>已选 <strong class="selected">{{ selectedCount }}</strong> 个</span>
        <span class="footer-sep">|</span>
        <span>总大小 <strong>{{ formatSize(selectedSize) }}</strong></span>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch, onMounted } from 'vue'
import {
  NCheckbox,
  NSelect,
  NInput,
  NIcon,
  NSpin,
  NEmpty
} from 'naive-ui'
import {
  SearchOutline,
  FolderOutline,
  ChevronDownOutline,
  ChevronForwardOutline,
  FilmOutline,
  MusicalNotesOutline,
  ImageOutline,
  DocumentOutline,
  FolderOpenOutline
} from '@vicons/ionicons5'

const props = defineProps({
  files: {
    type: Array,
    default: () => []
  }
})

const emit = defineEmits(['selection-change'])

// ===== 状态 =====
const state = ref('loading') // loading | loaded | filtered | empty
const typeFilter = ref(null)
const searchKeyword = ref('')
const treeData = ref([])
const expandedFolders = ref(new Set())
const checkedFiles = ref(new Set())

// 类型筛选选项
const typeFilterOptions = [
  { label: '视频', value: 'video' },
  { label: '音频', value: 'audio' },
  { label: '图片', value: 'image' },
  { label: '文档', value: 'document' },
  { label: '文件夹', value: 'folder' },
  { label: '其他', value: 'other' }
]

// 搜索防抖
let searchTimer = null

// ===== 计算属性 =====

/** 过滤后的展示文件 */
const filteredFiles = computed(() => {
  let result = [...props.files]
  if (typeFilter.value) {
    result = result.filter(f => f.type === typeFilter.value || (typeFilter.value === 'folder' && f.is_dir))
  }
  if (searchKeyword.value) {
    const kw = searchKeyword.value.toLowerCase()
    result = result.filter(f => f.name.toLowerCase().includes(kw))
  }
  return result
})

/** 扁平化展示文件列表（用于虚拟滚动判断） */
const displayFiles = computed(() => {
  const result = []
  const collectFlat = (files, prefix = '') => {
    for (const f of files) {
      const fullPath = prefix ? `${prefix}/${f.name}` : f.name
      result.push(f)
      if (f.is_dir && f.children && f.children.length > 0) {
        collectFlat(f.children, fullPath)
      }
    }
  }
  collectFlat(filteredFiles.value)
  return result
})

/** 构建树形数据 */
const buildTreeData = (files, level = 0, parentKey = '') => {
  return files.map((f, i) => {
    const key = parentKey ? `${parentKey}/${f.name}` : f.name
    const node = {
      key,
      name: f.name,
      size: f.size || 0,
      type: f.type || 'other',
      isDir: f.is_dir || false,
      level,
      checked: checkedFiles.value.has(key),
      expanded: expandedFolders.value.has(key),
      indeterminate: false,
      children: [],
      pickCode: f.pick_code || '',
      fid: f.fid || '',
      sha1: f.sha1 || ''
    }
    if (f.is_dir && f.children) {
      node.children = buildTreeData(f.children, level + 1, key)
      // 计算文件夹的选中状态
      updateFolderCheckState(node)
    }
    return node
  })
}

/** 更新文件夹节点的选中状态 */
const updateFolderCheckState = (node) => {
  if (!node.children || node.children.length === 0) return
  const totalChildren = node.children.length
  let checkedCount = 0
  for (const child of node.children) {
    if (child.isDir) {
      updateFolderCheckState(child)
    }
    if (child.checked) checkedCount++
  }
  if (checkedCount === 0) {
    node.checked = false
    node.indeterminate = false
  } else if (checkedCount === totalChildren) {
    node.checked = true
    node.indeterminate = false
  } else {
    node.checked = false
    node.indeterminate = true
  }
}

/** 全选状态 */
const isAllSelected = computed(() => {
  const leaves = getLeafNodes(treeData.value)
  return leaves.length > 0 && leaves.every(n => n.checked)
})

const isIndeterminate = computed(() => {
  const leaves = getLeafNodes(treeData.value)
  const someChecked = leaves.some(n => n.checked)
  return someChecked && !isAllSelected.value
})

const isAllDeselected = computed(() => {
  return selectedCount.value > 0
})

/** 获取所有叶子节点 */
const getLeafNodes = (nodes) => {
  const leaves = []
  for (const node of nodes) {
    if (node.isDir && node.children) {
      leaves.push(...getLeafNodes(node.children))
    } else if (!node.isDir) {
      leaves.push(node)
    }
  }
  return leaves
}

/** 统计 */
const totalCount = computed(() => {
  return getLeafNodes(treeData.value).length
})

const selectedCount = computed(() => {
  return checkedFiles.value.size
})

const selectedSize = computed(() => {
  let total = 0
  for (const node of getLeafNodes(treeData.value)) {
    if (node.checked) total += node.size
  }
  return total
})

// ===== 方法 =====

const formatSize = (bytes) => {
  if (!bytes || bytes === 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let idx = 0
  let size = bytes
  while (size >= 1024 && idx < units.length - 1) {
    size /= 1024
    idx++
  }
  return `${size.toFixed(idx > 0 ? 1 : 0)} ${units[idx]}`
}

const getFileIcon = (type) => {
  switch (type) {
    case 'video': return FilmOutline
    case 'audio': return MusicalNotesOutline
    case 'image': return ImageOutline
    case 'folder': return FolderOutline
    case 'document': return DocumentOutline
    default: return DocumentOutline
  }
}

const getFileIconColor = (type) => {
  switch (type) {
    case 'video': return '#8b5cf6'
    case 'audio': return '#ec4899'
    case 'image': return '#06b6d4'
    case 'folder': return '#f59e0b'
    case 'document': return '#6b7280'
    default: return '#6b7280'
  }
}

/** 展开/折叠文件夹 */
const toggleFolder = (key) => {
  if (expandedFolders.value.has(key)) {
    expandedFolders.value.delete(key)
  } else {
    expandedFolders.value.add(key)
  }
  // 触发响应式更新
  expandedFolders.value = new Set(expandedFolders.value)
}

/** 切换节点选中 */
const toggleNodeCheck = (node, checked) => {
  if (node.isDir && node.children) {
    // 递归设置所有子节点
    const setChildren = (n, val) => {
      if (n.isDir && n.children) {
        for (const child of n.children) setChildren(child, val)
      } else if (!n.isDir) {
        if (val) {
          checkedFiles.value.add(n.key)
        } else {
          checkedFiles.value.delete(n.key)
        }
        n.checked = val
      }
    }
    setChildren(node, checked)
  } else if (!node.isDir) {
    if (checked) {
      checkedFiles.value.add(node.key)
    } else {
      checkedFiles.value.delete(node.key)
    }
    node.checked = checked
  }
  checkedFiles.value = new Set(checkedFiles.value)
  syncSelection()
}

/** 全选 */
const handleSelectAll = (val) => {
  const leaves = getLeafNodes(treeData.value)
  for (const leaf of leaves) {
    if (val) {
      checkedFiles.value.add(leaf.key)
      leaf.checked = true
    } else {
      checkedFiles.value.delete(leaf.key)
      leaf.checked = false
    }
  }
  checkedFiles.value = new Set(checkedFiles.value)
  // 更新文件夹状态
  for (const node of treeData.value) {
    if (node.isDir) updateFolderCheckState(node)
  }
  syncSelection()
}

/** 全不选 */
const handleDeselectAll = () => {
  const leaves = getLeafNodes(treeData.value)
  for (const leaf of leaves) {
    checkedFiles.value.delete(leaf.key)
    leaf.checked = false
  }
  checkedFiles.value = new Set(checkedFiles.value)
  for (const node of treeData.value) {
    if (node.isDir) updateFolderCheckState(node)
  }
  syncSelection()
}

/** 同步选中的文件信息给父组件 */
const syncSelection = () => {
  const leaves = getLeafNodes(treeData.value)
  const selected = leaves
    .filter(n => n.checked)
    .map(n => ({
      fid: n.fid,
      pick_code: n.pickCode,
      name: n.name,
      size: n.size
    }))
  emit('selection-change', selected)
}

// ===== 监听 =====

watch(() => props.files, (newFiles) => {
  if (!newFiles || newFiles.length === 0) {
    state.value = 'empty'
    treeData.value = []
    return
  }
  state.value = 'loaded'
  treeData.value = buildTreeData(newFiles)
}, { immediate: true, deep: true })

// 搜索防抖
watch(searchKeyword, () => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    if (props.files && props.files.length > 0) {
      treeData.value = buildTreeData(filteredFiles.value)
    }
    searchTimer = null
  }, 300)
})

watch(typeFilter, () => {
  if (props.files && props.files.length > 0) {
    treeData.value = buildTreeData(filteredFiles.value)
    const totalLeaves = getLeafNodes(treeData.value)
    if (totalLeaves.length === 0) {
      state.value = 'empty'
    } else {
      state.value = 'filtered'
    }
  }
})

onMounted(() => {
  if (!props.files || props.files.length === 0) {
    state.value = 'empty'
  } else {
    state.value = 'loaded'
    treeData.value = buildTreeData(props.files)
  }
})
</script>

<style scoped>
.file-selector {
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 12px;
  overflow: hidden;
  background: var(--card-bg, #fff);
}

.toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  border-bottom: 1px solid var(--border-color, #e5e7eb);
  background: var(--bg-subtle, #f9fafb);
  flex-wrap: wrap;
  gap: 8px;
}

.toolbar-left,
.toolbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

.content-area {
  min-height: 120px;
  max-height: 400px;
  overflow-y: auto;
}

.file-tree-wrapper.virtual-scroll {
  max-height: 380px;
}

.state-center {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 40px 20px;
  gap: 12px;
}

.state-hint {
  margin: 0;
  font-size: 13px;
  color: var(--text-secondary, #6b7280);
}

.folder-node {
  border-bottom: 1px solid var(--border-subtle, #f3f4f6);
}

.folder-header {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  cursor: pointer;
  transition: background 0.15s;
}

.folder-header:hover {
  background: var(--bg-hover, #f3f4f6);
}

.folder-name {
  font-size: 13px;
  font-weight: 600;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.folder-count {
  font-size: 11px;
  color: var(--text-tertiary, #9ca3af);
}

.file-node {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 5px 12px;
  border-bottom: 1px solid var(--border-subtle, #f9fafb);
  transition: background 0.1s;
}

.file-node:hover {
  background: var(--bg-hover, #f9fafb);
}

.file-node.top-level {
  padding-left: 12px;
}

.file-name {
  font-size: 13px;
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-size {
  font-size: 11px;
  color: var(--text-tertiary, #9ca3af);
  white-space: nowrap;
}

/* 底部统计栏 */
.sticky-footer {
  position: sticky;
  bottom: 0;
  border-top: 1px solid var(--border-color, #e5e7eb);
  padding: 10px 14px;
  background: var(--bg-subtle, #f9fafb);
}

.footer-stats {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  color: var(--text-secondary, #6b7280);
}

.footer-stats strong {
  font-weight: 700;
  color: var(--text-primary, #374151);
}

.footer-stats .selected {
  color: #2563eb;
}

.footer-sep {
  color: var(--border-color, #d1d5db);
}
</style>
