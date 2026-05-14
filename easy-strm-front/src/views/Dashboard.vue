<template>
  <div class="dashboard-shell" :class="{ 'dashboard-shell-dark': isDark }">
    <div class="shell-bg">
      <div class="shell-orb shell-orb-a"></div>
      <div class="shell-orb shell-orb-b"></div>
      <div class="shell-grid"></div>
    </div>

    <aside class="shell-sidebar" :class="{ 'shell-sidebar-open': mobileMenuOpen }">
      <div class="sidebar-brand">
        <div class="brand-mark">ES</div>
        <div class="brand-copy">
          <div class="brand-title">Easy Stream</div>
          <div class="brand-subtitle">媒体中台工作区</div>
        </div>
      </div>

      <div class="sidebar-scroll">
        <section v-for="section in menuSections" :key="section.title" class="sidebar-section">
          <div class="section-title">{{ section.title }}</div>
          <button
            v-for="item in section.items"
            :key="item.path"
            type="button"
            class="sidebar-link"
            :class="{ 'sidebar-link-active': route.path === item.path }"
            @click="navigate(item.path)"
          >
            <el-icon class="sidebar-link-icon"><component :is="item.icon" /></el-icon>
            <div class="sidebar-link-copy">
              <span class="sidebar-link-title">{{ item.label }}</span>
              <span class="sidebar-link-desc">{{ item.desc }}</span>
            </div>
          </button>
        </section>
      </div>
    </aside>

    <div v-if="mobileMenuOpen" class="shell-mask" @click="mobileMenuOpen = false"></div>

    <main class="shell-main">
      <header class="shell-header">
        <div class="header-left">
          <button type="button" class="mobile-menu-btn" @click="mobileMenuOpen = !mobileMenuOpen">
            <el-icon><Menu /></el-icon>
          </button>
          <div class="header-copy">
            <div class="header-title">{{ currentTitle }}</div>
            <div class="header-subtitle">{{ currentDescription }}</div>
          </div>
        </div>

        <div class="header-actions">
          <router-link to="/dashboard/media-library" class="header-chip">
            <el-icon><Film /></el-icon>
            <span>资产台账</span>
          </router-link>
          <router-link to="/dashboard/tasks" class="header-chip">
            <el-icon><List /></el-icon>
            <span>任务中心</span>
          </router-link>
          <button type="button" class="header-chip" @click="toggleTheme">
            <el-icon><component :is="isDark ? Sunny : Moon" /></el-icon>
            <span>{{ isDark ? '浅色' : '深色' }}</span>
          </button>
          <button type="button" class="header-logout" @click="handleLogout">
            <el-icon><SwitchButton /></el-icon>
            <span>退出</span>
          </button>
        </div>
      </header>

      <section class="shell-content">
        <router-view />
      </section>
    </main>
  </div>
</template>

<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  Brush,
  Connection,
  DataLine,
  Document,
  Files,
  Film,
  FolderOpened,
  HomeFilled,
  List,
  Menu,
  Moon,
  Setting,
  Sunny,
  SwitchButton,
  Tickets,
  Tools,
  Refresh
} from '@element-plus/icons-vue'
import { showConfirmDialog } from '../utils/ui/messageBox'
import { logout } from '../utils/api/auth'

const route = useRoute()
const router = useRouter()

const isDark = ref(document.documentElement.classList.contains('dark'))
const mobileMenuOpen = ref(false)
const isMobile = ref(window.innerWidth <= 960)

const menuSections = [
  {
    title: '资源整理',
    items: [
      { path: '/dashboard/home', label: '首页', desc: '资源整理总览与快捷入口', icon: HomeFilled },
      { path: '/dashboard/media-library', label: '资产台账', desc: '资源状态、STRM 与任务追踪', icon: Film },
      { path: '/dashboard/sync-tasks', label: '同步入库', desc: '同步索引与入库流水线', icon: Refresh },
      { path: '/dashboard/pending-media', label: '待处理', desc: '识别失败与人工修正', icon: Tickets },
      { path: '/dashboard/tasks', label: '任务中心', desc: '同步、入库、刷新任务详情', icon: List }
    ]
  },
  {
    title: '支撑配置',
    items: [
      { path: '/dashboard/media-manager', label: '文件工作台', desc: '手动浏览、识别、整理', icon: FolderOpened },
      { path: '/dashboard/strm-config', label: 'STRM 配置', desc: '流水线输出规则与定时任务', icon: DataLine },
      { path: '/dashboard/cloud115', label: '115 云管理', desc: '账号、配额与能力接入', icon: Files },
      { path: '/dashboard/category-strategy', label: '整理规则', desc: '分类策略与归档规则', icon: Tools },
      { path: '/dashboard/settings', label: '系统设置', desc: 'TMDB、Emby 与系统参数', icon: Setting }
    ]
  },
  {
    title: '运维观察',
    items: [
      { path: '/dashboard/system-logs', label: '系统日志', desc: '日志文件与保留策略', icon: Document },
      { path: '/dashboard/network', label: '网络测试', desc: '连通性探测与错误定位', icon: Connection },
      { path: '/dashboard/cache', label: '缓存管理', desc: '缓存命中面与手动清理', icon: Brush }
    ]
  }
]

const currentTitle = computed(() => route.meta?.title || '控制台')
const currentDescription = computed(() => route.meta?.description || '已接入后端核心能力的新后台壳层')

const handleResize = () => {
  isMobile.value = window.innerWidth <= 960
  if (!isMobile.value) {
    mobileMenuOpen.value = false
  }
}

const navigate = (path) => {
  mobileMenuOpen.value = false
  if (route.path !== path) {
    router.push(path)
  }
}

const toggleTheme = () => {
  isDark.value = !isDark.value
  document.documentElement.classList.toggle('dark', isDark.value)
  localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
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

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

onBeforeUnmount(() => {
  window.removeEventListener('resize', handleResize)
})
</script>

<style scoped>
.dashboard-shell {
  --shell-bg: #f5f1e8;
  --shell-panel: rgba(255, 252, 247, 0.92);
  --shell-panel-strong: rgba(255, 250, 242, 0.98);
  --shell-line: rgba(120, 101, 72, 0.14);
  --shell-text: #1f2933;
  --shell-muted: #6f6457;
  --shell-accent: #1f6f78;
  --shell-accent-soft: #f2a65a;
  --shell-shadow: 0 24px 60px rgba(58, 42, 24, 0.12);
  position: relative;
  min-height: 100vh;
  background:
    radial-gradient(circle at top left, rgba(242, 166, 90, 0.18), transparent 28%),
    radial-gradient(circle at right 10%, rgba(31, 111, 120, 0.12), transparent 24%),
    linear-gradient(180deg, #f7f3eb 0%, #f1ecdf 100%);
  color: var(--shell-text);
}

.dashboard-shell-dark {
  --shell-bg: #111826;
  --shell-panel: rgba(18, 25, 38, 0.92);
  --shell-panel-strong: rgba(13, 20, 31, 0.98);
  --shell-line: rgba(139, 163, 185, 0.14);
  --shell-text: #ebf2fa;
  --shell-muted: #8fa1b5;
  --shell-accent: #77c3d4;
  --shell-accent-soft: #f0b469;
  --shell-shadow: 0 24px 60px rgba(0, 0, 0, 0.28);
  background:
    radial-gradient(circle at top left, rgba(119, 195, 212, 0.14), transparent 22%),
    radial-gradient(circle at bottom right, rgba(240, 180, 105, 0.12), transparent 18%),
    linear-gradient(180deg, #0c111a 0%, #121a26 100%);
}

.shell-bg,
.shell-grid,
.shell-orb {
  position: absolute;
  inset: 0;
  pointer-events: none;
}

.shell-grid {
  background-image:
    linear-gradient(rgba(255, 255, 255, 0.04) 1px, transparent 1px),
    linear-gradient(90deg, rgba(255, 255, 255, 0.04) 1px, transparent 1px);
  background-size: 32px 32px;
  opacity: 0.2;
}

.shell-orb {
  filter: blur(28px);
  opacity: 0.65;
}

.shell-orb-a {
  inset: 20px auto auto 18%;
  width: 240px;
  height: 240px;
  background: rgba(242, 166, 90, 0.22);
}

.shell-orb-b {
  inset: auto 12% 8% auto;
  width: 320px;
  height: 320px;
  background: rgba(31, 111, 120, 0.18);
}

.shell-sidebar {
  position: fixed;
  inset: 18px auto 18px 18px;
  z-index: 30;
  width: 290px;
  display: flex;
  flex-direction: column;
  background: var(--shell-panel);
  border: 1px solid var(--shell-line);
  border-radius: 28px;
  box-shadow: var(--shell-shadow);
  backdrop-filter: blur(18px);
}

.sidebar-brand {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 24px 22px 20px;
  border-bottom: 1px solid var(--shell-line);
}

.brand-mark {
  width: 52px;
  height: 52px;
  display: grid;
  place-items: center;
  border-radius: 18px;
  background: linear-gradient(135deg, var(--shell-accent), var(--shell-accent-soft));
  color: #fffdf8;
  font-size: 18px;
  font-weight: 700;
  letter-spacing: 0.08em;
}

.brand-title {
  font-size: 20px;
  font-weight: 700;
}

.brand-subtitle {
  margin-top: 4px;
  color: var(--shell-muted);
  font-size: 12px;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.sidebar-scroll {
  flex: 1;
  overflow: auto;
  padding: 12px 14px 18px;
}

.sidebar-section + .sidebar-section {
  margin-top: 10px;
}

.section-title {
  padding: 12px 12px 8px;
  color: var(--shell-muted);
  font-size: 12px;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.sidebar-link {
  width: 100%;
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 13px 12px;
  margin-bottom: 6px;
  border: 0;
  border-radius: 18px;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  transition: transform 0.2s ease, background-color 0.2s ease, box-shadow 0.2s ease;
}

.sidebar-link:hover {
  transform: translateX(2px);
  background: rgba(31, 111, 120, 0.08);
}

.sidebar-link-active {
  background: linear-gradient(135deg, rgba(31, 111, 120, 0.18), rgba(242, 166, 90, 0.16));
  box-shadow: inset 0 0 0 1px rgba(31, 111, 120, 0.12);
}

.sidebar-link-icon {
  width: 38px;
  height: 38px;
  display: grid;
  place-items: center;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.46);
  font-size: 18px;
}

.dashboard-shell-dark .sidebar-link-icon {
  background: rgba(255, 255, 255, 0.06);
}

.sidebar-link-copy {
  display: flex;
  flex-direction: column;
  gap: 3px;
}

.sidebar-link-title {
  font-size: 14px;
  font-weight: 700;
}

.sidebar-link-desc {
  color: var(--shell-muted);
  font-size: 12px;
}

.shell-main {
  position: relative;
  z-index: 10;
  margin-left: 326px;
  min-height: 100vh;
  padding: 18px 18px 18px 0;
}

.shell-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 18px 22px;
  margin-bottom: 18px;
  background: var(--shell-panel);
  border: 1px solid var(--shell-line);
  border-radius: 24px;
  box-shadow: var(--shell-shadow);
  backdrop-filter: blur(18px);
}

.header-left,
.header-actions {
  display: flex;
  align-items: center;
  gap: 14px;
}

.header-title {
  font-size: 28px;
  font-weight: 800;
  letter-spacing: -0.03em;
}

.header-subtitle {
  margin-top: 4px;
  color: var(--shell-muted);
  font-size: 13px;
}

.mobile-menu-btn,
.header-chip,
.header-logout {
  border: 0;
  cursor: pointer;
}

.mobile-menu-btn {
  display: none;
  width: 42px;
  height: 42px;
  border-radius: 14px;
  background: rgba(31, 111, 120, 0.1);
  color: var(--shell-text);
  font-size: 18px;
}

.header-chip,
.header-logout {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.5);
  color: var(--shell-text);
  text-decoration: none;
  transition: transform 0.2s ease, background-color 0.2s ease;
}

.dashboard-shell-dark .header-chip,
.dashboard-shell-dark .header-logout {
  background: rgba(255, 255, 255, 0.06);
}

.header-chip:hover,
.header-logout:hover {
  transform: translateY(-1px);
}

.header-logout {
  background: linear-gradient(135deg, rgba(220, 94, 74, 0.18), rgba(242, 166, 90, 0.16));
}

.shell-content {
  min-height: calc(100vh - 132px);
}

.shell-mask {
  position: fixed;
  inset: 0;
  z-index: 20;
  background: rgba(7, 12, 18, 0.45);
  backdrop-filter: blur(4px);
}

@media (max-width: 960px) {
  .shell-sidebar {
    transform: translateX(-110%);
    transition: transform 0.25s ease;
  }

  .shell-sidebar-open {
    transform: translateX(0);
  }

  .shell-main {
    margin-left: 0;
    padding-left: 18px;
  }

  .mobile-menu-btn {
    display: inline-grid;
    place-items: center;
  }

  .header-chip span {
    display: none;
  }
}

@media (max-width: 720px) {
  .shell-header {
    flex-direction: column;
    align-items: stretch;
  }

  .header-left,
  .header-actions {
    justify-content: space-between;
  }

  .header-title {
    font-size: 24px;
  }
}
</style>
