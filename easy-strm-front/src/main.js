import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import 'element-plus/theme-chalk/dark/css-vars.css'
import 'element-plus/es/components/message/style/css'
import App from './App.vue'
import './style.css'

const Login = () => import('./views/Login.vue')
const Dashboard = () => import('./views/Dashboard.vue')
const Cloud115 = () => import('./views/Cloud115.vue')
const StrmConfig = () => import('./views/StrmConfig.vue')
const Settings = () => import('./views/Settings.vue')
const MediaManager = () => import('./views/MediaManager.vue')
const CategoryStrategy = () => import('./views/CategoryStrategy.vue')
const MediaLibrary = () => import('./views/MediaLibrary.vue')
const PendingMedia = () => import('./views/PendingMedia.vue')
const SyncTasks = () => import('./views/SyncTasks.vue')
const DashboardHome = () => import('./views/dashboard/DashboardHome.vue')
const TaskCenter = () => import('./views/dashboard/TaskCenter.vue')
const SystemLogs = () => import('./views/dashboard/SystemLogs.vue')
const NetworkCenter = () => import('./views/dashboard/NetworkCenter.vue')
const CacheCenter = () => import('./views/dashboard/CacheCenter.vue')

const routes = [
  { path: '/', redirect: '/login' },
  { path: '/login', component: Login, meta: { title: '登录' } },
  {
    path: '/dashboard',
    component: Dashboard,
    redirect: '/dashboard/home',
    children: [
      {
        path: 'home',
        component: DashboardHome,
        meta: {
          title: '仪表盘',
          description: '围绕任务、STRM、媒体源和网络探针重组后的新首页。'
        }
      },
      {
        path: 'tasks',
        component: TaskCenter,
        meta: {
          title: '任务中心',
          description: '统一任务列表、任务详情、取消恢复与失败文件排查。'
        }
      },
      {
        path: 'system-logs',
        component: SystemLogs,
        meta: {
          title: '系统日志',
          description: '日志文件读取、自动刷新与日志保留策略维护。'
        }
      },
      {
        path: 'network',
        component: NetworkCenter,
        meta: {
          title: '网络测试',
          description: '关键外部站点连通性检测与代理路径观察。'
        }
      },
      {
        path: 'cache',
        component: CacheCenter,
        meta: {
          title: '缓存管理',
          description: '查看 Redis 与识别/刮削缓存占用，并按类型清理。'
        }
      },
      {
        path: 'media-manager',
        component: MediaManager,
        meta: {
          title: '文件管理',
          description: '媒体源浏览、识别、整理、刮削等核心业务工作台。'
        }
      },
      {
        path: 'media-library',
        component: MediaLibrary,
        meta: {
          title: '媒体库',
          description: '媒体条目、STRM 状态、元数据完整度与失效诊断。'
        }
      },
      {
        path: 'pending-media',
        component: PendingMedia,
        meta: {
          title: '待处理',
          description: '识别失败、人工修正、重新入库与批量处理入口。'
        }
      },
      {
        path: 'sync-tasks',
        component: SyncTasks,
        meta: {
          title: '同步任务',
          description: '媒体源全量同步、增量同步与同步索引查看。'
        }
      },
      {
        path: 'strm-config',
        component: StrmConfig,
        meta: {
          title: 'STRM 生成',
          description: 'STRM 配置、全量生成、定时任务与执行状态。'
        }
      },
      {
        path: 'cloud115',
        component: Cloud115,
        meta: {
          title: '115 云管理',
          description: '115 账号接入、状态管理与云端能力维护。'
        }
      },
      {
        path: 'category-strategy',
        component: CategoryStrategy,
        meta: {
          title: '整理规则',
          description: '分类策略、整理规则与媒体归档的策略配置。'
        }
      },
      {
        path: 'settings',
        component: Settings,
        meta: {
          title: '系统设置',
          description: 'TMDB、Emby、日志、代理与命名模板配置。'
        }
      }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  const isLoginPage = to.path === '/login'

  if (!token && !isLoginPage) {
    next('/login')
  } else if (token && isLoginPage) {
    next('/dashboard/home')
  } else {
    next()
  }
})

const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark') {
  document.documentElement.classList.add('dark')
}

const app = createApp(App)
app.use(router)
app.mount('#app')
