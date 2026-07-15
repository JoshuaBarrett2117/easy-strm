<template>
  <div class="flex h-screen overflow-hidden bg-slate-100 dark:bg-ink-950">
    <!-- 侧边栏 -->
    <aside
      class="fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-slate-200 bg-white transition-transform duration-200 dark:border-white/5 dark:bg-ink-900 lg:static lg:translate-x-0"
      :class="mobileMenuOpen ? 'translate-x-0' : '-translate-x-full'"
    >
      <!-- 品牌 -->
      <div class="flex items-center gap-3 px-5 py-5">
        <div class="flex h-10 w-10 items-center justify-center rounded-xl bg-gradient-to-br from-cyan-400 to-cyan-700 text-sm font-extrabold tracking-wider text-white shadow-lg shadow-cyan-500/20">
          ES
        </div>
        <div>
          <div class="text-base font-bold text-slate-800 dark:text-white">Easy Stream</div>
          <div class="text-[11px] uppercase tracking-widest text-slate-400 dark:text-slate-500">Media Platform</div>
        </div>
      </div>

      <!-- 菜单 -->
      <nav class="flex-1 overflow-y-auto px-3 pb-4">
        <div v-for="section in menuSections" :key="section.title" class="mb-2">
          <div class="px-3 py-2 text-[11px] font-bold uppercase tracking-widest text-slate-400 dark:text-slate-600">
            {{ section.title }}
          </div>
          <button
            v-for="item in section.items"
            :key="item.path"
            type="button"
            class="group mb-0.5 flex w-full items-center gap-3 rounded-lg px-3 py-2.5 text-left text-sm transition-colors"
            :class="route.path === item.path
              ? 'bg-cyan-600/10 font-semibold text-cyan-700 dark:bg-cyan-400/10 dark:text-cyan-300'
              : 'text-slate-600 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-white/5 dark:hover:text-slate-200'"
            @click="navigate(item.path)"
          >
            <n-icon size="18" :component="item.icon" />
            <span>{{ item.label }}</span>
          </button>
        </div>
      </nav>
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
      <header class="flex items-center justify-between gap-4 border-b border-slate-200 bg-white/80 px-4 py-3 backdrop-blur dark:border-white/5 dark:bg-ink-900/80 lg:px-6">
        <div class="flex min-w-0 items-center gap-3">
          <button
            type="button"
            class="flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-white/5 lg:hidden"
            @click="mobileMenuOpen = !mobileMenuOpen"
          >
            <n-icon size="20" :component="MenuOutline" />
          </button>
          <div class="min-w-0">
            <h1 class="truncate text-lg font-bold text-slate-800 dark:text-white">{{ currentTitle }}</h1>
            <p class="hidden truncate text-xs text-slate-400 dark:text-slate-500 md:block">{{ currentDescription }}</p>
          </div>
        </div>

        <div class="flex items-center gap-2">
          <n-tooltip trigger="hover">
            <template #trigger>
              <button
                type="button"
                class="flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-slate-100 dark:text-slate-400 dark:hover:bg-white/5"
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
                class="flex h-9 w-9 items-center justify-center rounded-lg text-slate-500 transition-colors hover:bg-red-50 hover:text-red-500 dark:text-slate-400 dark:hover:bg-red-500/10 dark:hover:text-red-400"
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
      <main class="flex-1 overflow-y-auto p-4 lg:p-6">
        <router-view v-slot="{ Component }">
          <transition name="page-fade" mode="out-in">
            <component :is="Component" />
          </transition>
        </router-view>
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
  FilmOutline,
  SyncOutline,
  AlertCircleOutline,
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
      { path: '/dashboard/media-library', label: '资产台账', icon: FilmOutline },
      { path: '/dashboard/sync-tasks', label: '同步入库', icon: SyncOutline },
      { path: '/dashboard/pending-media', label: '待处理', icon: AlertCircleOutline },
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
