<template>
  <div class="rounded-2xl border border-slate-200 bg-white p-4 shadow-sm transition-shadow hover:shadow-md dark:border-white/5 dark:bg-ink-900 lg:p-5">
    <div class="flex items-start justify-between">
      <div class="min-w-0">
        <p class="text-xs font-medium text-slate-400 dark:text-slate-500">{{ label }}</p>
        <p class="mt-1 truncate text-2xl font-extrabold tabular-nums text-slate-800 dark:text-white">
          {{ value }}
        </p>
        <p v-if="hint" class="mt-1 truncate text-xs text-slate-400 dark:text-slate-500">{{ hint }}</p>
      </div>
      <div
        v-if="icon"
        class="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl"
        :class="toneClass"
      >
        <n-icon size="20" :component="icon" />
      </div>
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'
import { NIcon } from 'naive-ui'

const props = defineProps({
  label: { type: String, required: true },
  value: { type: [String, Number], default: '-' },
  hint: { type: String, default: '' },
  icon: { type: Object, default: null },
  tone: { type: String, default: 'cyan' } // cyan | green | amber | red | violet | slate
})

const TONES = {
  cyan: 'bg-cyan-500/10 text-cyan-600 dark:text-cyan-400',
  green: 'bg-emerald-500/10 text-emerald-600 dark:text-emerald-400',
  amber: 'bg-amber-500/10 text-amber-600 dark:text-amber-400',
  red: 'bg-red-500/10 text-red-600 dark:text-red-400',
  violet: 'bg-violet-500/10 text-violet-600 dark:text-violet-400',
  slate: 'bg-slate-500/10 text-slate-600 dark:text-slate-400'
}

const toneClass = computed(() => TONES[props.tone] || TONES.cyan)
</script>
