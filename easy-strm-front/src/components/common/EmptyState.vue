<template>
  <div class="flex flex-col items-center justify-center rounded-2xl border border-dashed border-slate-200/80 bg-slate-50/60 px-5 py-12 text-center dark:border-white/10 dark:bg-white/[0.025]">
    <div class="mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-500/15 to-cyan-500/10 text-indigo-400 ring-1 ring-inset ring-indigo-400/10 dark:text-indigo-300">
      <n-icon size="26" :component="icon || FileTrayOutline" />
    </div>
    <p class="text-sm font-bold text-slate-600 dark:text-slate-300">{{ title }}</p>
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
