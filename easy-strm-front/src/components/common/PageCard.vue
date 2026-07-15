<template>
  <div class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-5">
    <div v-if="title || $slots.action" class="mb-4 flex flex-wrap items-center justify-between gap-3">
      <div>
        <h2 class="text-base font-bold text-slate-800 dark:text-white">{{ title }}</h2>
        <p v-if="subtitle" class="mt-0.5 text-xs text-slate-400 dark:text-slate-500">{{ subtitle }}</p>
      </div>
      <div v-if="$slots.action" class="flex flex-wrap items-center gap-2">
        <slot name="action" />
      </div>
    </div>
    <slot />
  </div>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' }
})
</script>
