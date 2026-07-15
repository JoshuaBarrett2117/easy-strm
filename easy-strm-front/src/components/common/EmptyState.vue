<template>
  <div class="flex flex-col items-center justify-center py-12 text-center">
    <div class="mb-3 flex h-14 w-14 items-center justify-center rounded-2xl bg-slate-100 text-slate-300 dark:bg-white/5 dark:text-slate-600">
      <n-icon size="26" :component="icon || FileTrayOutline" />
    </div>
    <p class="text-sm font-medium text-slate-500 dark:text-slate-400">{{ title }}</p>
    <p v-if="description" class="mt-1 max-w-sm text-xs text-slate-400 dark:text-slate-500">{{ description }}</p>
    <div v-if="$slots.default" class="mt-4">
      <slot />
    </div>
  </div>
</template>

<script setup>
import { NIcon } from 'naive-ui'
import { FileTrayOutline } from '@vicons/ionicons5'

defineProps({
  title: { type: String, default: '暂无数据' },
  description: { type: String, default: '' },
  icon: { type: Object, default: null }
})
</script>
