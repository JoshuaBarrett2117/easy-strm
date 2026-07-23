<template>
  <div class="app-canvas flex h-screen overflow-hidden">
    <!-- 侧边栏 -->
    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-[17rem] flex-col overflow-hidden border-r border-slate-200/80 bg-white text-slate-800 shadow-2xl shadow-slate-950/10 transition-transform duration-200 dark:border-white/[0.07] dark:bg-[#0b0f1a] dark:text-white dark:shadow-slate-950/20 lg:static lg:translate-x-0 lg:shadow-none"
      :class="mobileMenuOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <!-- 品牌 -->
      <div class="relative flex items-center gap-3 px-5 pb-5 pt-6">
        <div class="absolute -left-16 -top-24 h-48 w-48 rounded-full bg-indigo-400/15 blur-3xl dark:bg-indigo-500/20"></div>
        <div class="relative flex h-11 w-11 items-center justify-center overflow-hidden rounded-2xl bg-gradient-to-br from-indigo-400 via-indigo-500 to-cyan-500 text-sm font-black tracking-wider text-white shadow-lg shadow-indigo-950/40 ring-1 ring-white/20">
          <span>ES</span>
          <span class="absolute -bottom-3 -right-3 h-8 w-8 rounded-full bg-white/20 blur-md"></span>
        </div>
        <div class="relative min-w-0">
          <div class="truncate text-[15px] font-extrabold tracking-tight text-slate-900 dark:text-white">Easy Stream</div>
          <div class="mt-0.5 text-[10px] font-bold uppercase tracking-[0.2em] text-slate-400 dark:text-slate-500">Media Workspace</div>
        </div>
      </div>

      <!-- 菜单 -->
      <nav class="relative flex-1 overflow-y-auto px-3 pb-4">
        <div v-for="section in menuSections" :key="section.title" class="mb-4">
          <div class="px-3 pb-2 pt-1 text-[10px] font-extrabold uppercase tracking-[0.18em] text-slate-400 dark:text-slate-600">
            {{ section.title }}
          </div>
          <button
            v-for="item in section.items"
            :key="item.path"
            type="button"
            class="group relative mb-1 flex w-full items-center gap-3 overflow-hidden rounded-xl px-3 py-2.5 text-left text-[13px] transition-all duration-150"
            :class="route.path === item.path
              ? 'bg-indigo-50 font-bold text-indigo-700 shadow-sm ring-1 ring-indigo-100 dark:bg-white/[0.09] dark:text-white dark:shadow-inner dark:shadow-white/[0.03] dark:ring-white/[0.06]'
              : 'text-slate-600 hover:bg-slate-100 hover:text-slate-900 dark:text-slate-400 dark:hover:bg-white/[0.045] dark:hover:text-slate-200'"
            @click="navigate(item.path)"
          >
            <span v-if="route.path === item.path" class="absolute inset-y-2 left-0 w-0.5 rounded-full bg-gradient-to-b from-indigo-400 to-cyan-300"></span>
            <span
              class="flex h-8 w-8 shrink-0 items-center justify-center rounded-lg transition-colors"
              :class="route.path === item.path
                ? 'bg-gradient-to-br from-indigo-100 to-cyan-50 text-indigo-600 dark:from-indigo-500/30 dark:to-cyan-500/20 dark:text-indigo-200'
                : 'bg-slate-100 text-slate-400 group-hover:text-slate-600 dark:bg-white/[0.035] dark:text-slate-500 dark:group-hover:text-slate-300'"
            >
              <n-icon size="17" :component="item.icon" />
            </span>
            <span>{{ item.label }}</span>
          </button>
        </div>
      </nav>

      <div class="relative m-3 rounded-2xl border border-slate-200/80 bg-slate-50/80 p-3.5 dark:border-white/[0.07] dark:bg-white/[0.035]">
        <div class="flex items-center gap-2.5">
          <span class="relative flex h-2.5 w-2.5">
            <span class="absolute inline-flex h-full w-full animate-ping rounded-full bg-emerald-400 opacity-30"></span>
            <span class="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400"></span>
          </span>
          <div>
            <div class="text-xs font-bold text-slate-700 dark:text-slate-200">工作区已连接</div>
            <div class="mt-0.5 text-[10px] text-slate-400 dark:text-slate-600">服务运行状态正常</div>
          </div>
        </div>
      </div>
    </aside>

    <!-- 移动端遮罩 -->
    <div
      v-if="mobileMenuOpen"
      class="fixed inset-0 z-30 bg-black/50 backdrop-blur-sm lg:hidden"
      @click="mobileMenuOpen = false"
    ></div>

    <!-- 主区域 -->
    <div class="flex min-w-0 flex-1 flex-col">
      <!-- 顶栏 -->
      <header class="relative z-20 flex min-h-[4.5rem] items-center justify-between gap-4 border-b border-slate-200/60 bg-white/65 px-4 backdrop-blur-xl dark:border-white/[0.06] dark:bg-[#0b0f1a]/60 lg:px-7">
        <div class="flex min-w-0 items-center gap-3">
          <button
            type="button"
            class="icon-button lg:hidden"
            @click="mobileMenuOpen = !mobileMenuOpen"
          >
            <n-icon size="20" :component="MenuOutline" />
          </button>
          <div class="min-w-0 py-2">
            <div class="mb-0.5 hidden items-center gap-1.5 text-[10px] font-bold uppercase tracking-[0.14em] text-slate-400 dark:text-slate-600 sm:flex">
              <span>Easy Stream</span>
              <span class="text-slate-300 dark:text-slate-700">/</span>
              <span>{{ currentSection }}</span>
            </div>
            <h1 class="truncate text-lg font-extrabold tracking-tight text-slate-800 dark:text-white">{{ currentTitle }}</h1>
            <p class="hidden max-w-3xl truncate text-xs text-slate-400 dark:text-slate-500 md:block">{{ currentDescription }}</p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <n-tooltip trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="icon-button"
                @click="toggleTheme"
              >
                <n-icon size="18" :component="isDark ? SunnyOutline : MoonOutline" />
              </button>
            </template>
            {{ isDark ? '切换浅色' : '切换深色' }}
          </n-tooltip>
          <n-tooltip trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="icon-button hover:!border-red-200 hover:!bg-red-50 hover:!text-red-500 dark:hover:!border-red-500/20 dark:hover:!bg-red-500/10 dark:hover:!text-red-400"
                @click="handleLogout"
              >
                <n-icon size="18" :component="LogOutOutline" />
              </button>
            </template>
            退出登录
          </n-tooltip>
        </div>
      </header>

      <!-- 内容 -->
      <main class="flex-1 overflow-y-auto px-3 py-4 sm:px-4 lg:px-7 lg:py-6">
        <div class="page-content">
          <router-view v-slot="{ Component }">
            <transition name="page-fade" mode="out-in">
              <component :is="Component" />
            </transition>
          </router-view>
        </div>
      </main>
    </div>
  </div>
</template>

<script setup>
import { computed, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { NIcon, NTooltip } from 'naive-ui'
import {
  HomeOutline,
  ListOutline,
  FolderOpenOutline,
  DocumentTextOutline,
  CloudOutline,
  OptionsOutline,
  SettingsOutline,
  ReaderOutline,
  WifiOutline,
  ServerOutline,
  MenuOutline,
  MoonOutline,
  SunnyOutline,
  LogOutOutline
} from '@vicons/ionicons5'
import { useTheme } from '../composables/useTheme'
import { showConfirmDialog } from '../utils/ui/messageBox'
import { logout } from '../utils/api/auth'

const route = useRoute()
const router = useRouter()
const { isDark, toggleTheme } = useTheme()

const mobileMenuOpen = ref(false)

const menuSections = [
  {
    title: '资源整理',
    items: [
      { path: '/dashboard/home', label: '仪表盘', icon: HomeOutline },
      { path: '/dashboard/tasks', label: '任务中心', icon: ListOutline }
    ]
  },
  {
    title: '支撑配置',
    items: [
      { path: '/dashboard/media-manager', label: '文件工作台', icon: FolderOpenOutline },
      { path: '/dashboard/strm-config', label: 'STRM 配置', icon: DocumentTextOutline },
      { path: '/dashboard/cloud115', label: '115 云管理', icon: CloudOutline },
      { path: '/dashboard/category-strategy', label: '整理规则', icon: OptionsOutline },
      { path: '/dashboard/settings', label: '系统设置', icon: SettingsOutline }
    ]
  },
  {
    title: '运维观察',
    items: [
      { path: '/dashboard/system-logs', label: '系统日志', icon: ReaderOutline },
      { path: '/dashboard/network', label: '网络测试', icon: WifiOutline },
      { path: '/dashboard/cache', label: '缓存管理', icon: ServerOutline }
    ]
  }
]

const currentTitle = computed(() => route.meta?.title || '控制台')
const currentDescription = computed(() => route.meta?.description || '')
const currentSection = computed(() => {
  return menuSections.find(section => section.items.some(item => item.path === route.path))?.title || '工作台'
})

watch(() => route.path, () => {
  mobileMenuOpen.value = false
})

const navigate = (path) => {
  mobileMenuOpen.value = false
  if (route.path !== path) {
    router.push(path)
  }
}

const handleLogout = () => {
  showConfirmDialog('确定要退出当前登录状态吗？', '退出登录', {
    confirmButtonText: '退出',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(() => {
    logout()
  }).catch(() => {})
}
</script>
