<template>
  <section class="surface-card group rounded-[1.25rem] p-4 transition-[transform,box-shadow,border-color] duration-200 hover:-translate-y-0.5 hover:border-indigo-300/40 hover:shadow-[var(--shadow-card-hover)] lg:p-5">
    <div v-if="title || $slots.action" class="mb-5 flex flex-wrap items-start justify-between gap-3">
      <div>
        <div class="flex items-center gap-2.5">
          <span class="h-5 w-1 rounded-full bg-gradient-to-b from-indigo-500 to-cyan-400"></span>
          <h2 class="text-[15px] font-extrabold tracking-tight text-slate-800 dark:text-white">{{ title }}</h2>
        </div>
        <p v-if="subtitle" class="mt-1 pl-3.5 text-[11px] font-semibold uppercase tracking-[0.12em] text-slate-400 dark:text-slate-500">{{ subtitle }}</p>
      </div>
      <div v-if="$slots.action" class="flex flex-wrap items-center gap-2">
        <slot name="action" />
      </div>
    </div>
    <slot />
  </section>
</template>

<script setup>
defineProps({
  title: { type: String, default: '' },
  subtitle: { type: String, default: '' }
})
</script>
