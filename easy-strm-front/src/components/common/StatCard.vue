<template>
  <article class="surface-card group rounded-[1.25rem] p-4 transition-[transform,box-shadow,border-color] duration-200 hover:-translate-y-1 hover:border-indigo-300/40 hover:shadow-[var(--shadow-card-hover)] lg:p-5">
    <span class="absolute inset-x-5 top-0 h-px bg-gradient-to-r from-transparent via-indigo-400/70 to-transparent opacity-0 transition-opacity group-hover:opacity-100"></span>
    <div class="flex items-start justify-between">
      <div class="min-w-0">
        <p class="text-[11px] font-bold uppercase tracking-[0.1em] text-slate-400 dark:text-slate-500">{{ label }}</p>
        <p class="mt-2 truncate text-3xl font-black tracking-tight tabular-nums text-slate-800 dark:text-white">
          {{ value }}
        </p>
        <p v-if="hint" class="mt-1.5 truncate text-xs text-slate-400 dark:text-slate-500">{{ hint }}</p>
      </div>
      <div
        v-if="icon"
        class="flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl ring-1 ring-inset ring-current/10 transition-transform duration-200 group-hover:scale-105"
        :class="toneClass"
      >
        <n-icon size="20" :component="icon" />
      </div>
    </div>
  </article>
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
