<template>
  <n-modal
    v-model:show="visible"
    preset="card"
    title="文件冲突预览"
    style="max-width:640px;width:90%"
    :mask-closable="false"
    @close="handleCancel"
  >
    <div class="conflict-modal-body">
      <!-- 冲突说明 -->
      <p class="conflict-desc">
        目标目录中已存在 {{ conflicts.length }} 个同名文件，请选择处理策略：
      </p>

      <!-- 冲突文件列表 -->
      <div class="conflict-list">
        <div
          v-for="(item, idx) in conflicts.slice(0, 10)"
          :key="idx"
          class="conflict-item"
        >
          <n-icon size="16" :component="DocumentOutline" color="#6b7280" />
          <div class="ci-info">
            <span class="ci-name">{{ item.name }}</span>
            <span class="ci-sizes" v-if="item.source_size && item.target_size">
              源文件 {{ formatSize(item.source_size) }} → 目标 {{ formatSize(item.target_size) }}
            </span>
          </div>
        </div>
        <div v-if="conflicts.length > 10" class="conflict-more">
          还有 {{ conflicts.length - 10 }} 个冲突文件...
        </div>
      </div>

      <!-- 策略选择 -->
      <div class="strategy-section">
        <span class="strategy-label">冲突处理策略：</span>
        <n-radio-group v-model:value="selectedStrategy" name="conflict-strategy">
          <n-space vertical :size="10">
            <n-radio value="skip">
              <div class="radio-content">
                <span class="radio-title">跳过</span>
                <span class="radio-desc">目标已存在则跳过，不覆盖</span>
              </div>
            </n-radio>
            <n-radio value="overwrite">
              <div class="radio-content">
                <span class="radio-title">覆盖</span>
                <span class="radio-desc">用源文件覆盖目标文件</span>
              </div>
            </n-radio>
            <n-radio value="rename">
              <div class="radio-content">
                <span class="radio-title">自动重命名</span>
                <span class="radio-desc">自动添加序号后缀（如 文件名(1).mkv）</span>
              </div>
            </n-radio>
          </n-space>
        </n-radio-group>
      </div>
    </div>

    <template #footer>
      <div class="modal-footer">
        <n-button @click="handleCancel">
          取消转存
        </n-button>
        <n-button type="primary" @click="handleConfirm">
          确认并开始转存
        </n-button>
      </div>
    </template>
  </n-modal>
</template>

<script setup>
import { ref, computed, watch } from 'vue'
import {
  NModal,
  NButton,
  NIcon,
  NRadioGroup,
  NRadio,
  NSpace
} from 'naive-ui'
import { DocumentOutline } from '@vicons/ionicons5'

const props = defineProps({
  visible: {
    type: Boolean,
    default: false
  },
  conflicts: {
    type: Array,
    default: () => []
  }
})

const emit = defineEmits(['update:visible', 'confirm', 'cancel'])

// ===== 状态 =====
const visible = ref(props.visible)
const selectedStrategy = ref('skip')

// 同步可见性
watch(() => props.visible, (val) => {
  visible.value = val
  if (val) {
    selectedStrategy.value = 'skip'
  }
})

watch(visible, (val) => {
  emit('update:visible', val)
})

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

const handleConfirm = () => {
  emit('confirm', selectedStrategy.value)
  visible.value = false
}

const handleCancel = () => {
  emit('cancel')
  visible.value = false
}
</script>

<style scoped>
.conflict-modal-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.conflict-desc {
  margin: 0;
  font-size: 14px;
  color: var(--text-secondary, #6b7280);
}

.conflict-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 200px;
  overflow-y: auto;
  padding: 10px 12px;
  border: 1px solid var(--border-color, #e5e7eb);
  border-radius: 8px;
  background: var(--bg-subtle, #f9fafb);
}

.conflict-item {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 6px 0;
  border-bottom: 1px solid var(--border-subtle, #f3f4f6);
}

.conflict-item:last-child {
  border-bottom: none;
}

.ci-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.ci-name {
  font-size: 13px;
  font-weight: 500;
}

.ci-sizes {
  font-size: 11px;
  color: var(--text-tertiary, #9ca3af);
}

.conflict-more {
  font-size: 12px;
  color: var(--text-tertiary, #9ca3af);
  text-align: center;
  padding: 4px 0;
}

.strategy-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.strategy-label {
  font-size: 14px;
  font-weight: 600;
}

.radio-content {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.radio-title {
  font-size: 13px;
  font-weight: 500;
}

.radio-desc {
  font-size: 11px;
  color: var(--text-tertiary, #9ca3af);
}

.modal-footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}
</style>
