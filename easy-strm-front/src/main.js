import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import App from './App.vue'
import './style.css'

const Login = () => import('./views/Login.vue')
const Dashboard = () => import('./views/Dashboard.vue')
const Cloud115 = () => import('./views/Cloud115.vue')
const StrmConfig = () => import('./views/StrmConfig.vue')
const Settings = () => import('./views/Settings.vue')
const MediaManager = () => import('./views/MediaManager.vue')
const CategoryStrategy = () => import('./views/CategoryStrategy.vue')
const DashboardHome = () => import('./views/dashboard/DashboardHome.vue')
const TaskCenter = () => import('./views/dashboard/TaskCenter.vue')
const SystemLogs = () => import('./views/dashboard/SystemLogs.vue')
const NetworkCenter = () => import('./views/dashboard/NetworkCenter.vue')
const CacheCenter = () => import('./views/dashboard/CacheCenter.vue')
const ResourceAggregation = () => import('./views/ResourceAggregation.vue')
const FilenameRecognition = () => import('./views/FilenameRecognition.vue')
const FileManager = () => import('./views/FileManager.vue')
const EmbyManagement = () => import('./views/EmbyManagement.vue')
const EmbyMonitor = () => import('./views/EmbyMonitor.vue')

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
          description: '媒体整理工作台首页，聚合媒体源、STRM、任务与运行状态。'
        }
      },
      {
        path: 'tasks',
        component: TaskCenter,
        meta: {
          title: '任务中心',
          description: '同步、入库、STRM、刷新等资源整理任务的统一追踪入口。'
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
          title: '文件工作台',
          description: '保留手动浏览、识别、整理、刮削等细粒度文件操作。'
        }
      },
      {
        path: 'strm-config',
        component: StrmConfig,
        meta: {
          title: 'STRM 配置',
          description: '维护 STRM 输出规则、定时任务与生成配置。'
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
        path: 'file-manager',
        component: FileManager,
        meta: {
          title: '文件管理',
          description: '在本地媒体源与任意115账号之间复制、剪切、粘贴和删除文件。'
        }
      },
      {
        path: 'filename-recognition',
        component: FilenameRecognition,
        meta: {
          title: '识别测试',
          description: '模拟媒体文件名解析与 TMDB 匹配，并维护整理链路使用的识别规则。'
        }
      },
      {
        path: 'emby-management',
        component: EmbyManagement,
        meta: {
          adminOnly: true,
          title: 'Emby 管理',
          description: '管理多个 Emby 实例、用户、媒体库、封面和神医助手任务。'
        }
      },
      {
        path: 'emby-monitor',
        component: EmbyMonitor,
        meta: {
          adminOnly: true,
          title: 'Emby 监控',
          description: '查看 Emby 实时播放、历史排行、活跃热力图和最近入库。'
        }
      },
      {
        path: 'settings',
        component: Settings,
        meta: {
          title: '系统设置',
          description: 'TMDB、Emby、日志、代理与命名模板配置。'
        }
      },
      {
        path: 'resources/transfer',
        component: ResourceAggregation,
        meta: {
          title: '资源聚合',
          description: '115分享链接一键解析与批量转存、115云下载（ed2k/磁力等离线下载）。'
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
  } else if (to.meta.adminOnly && localStorage.getItem('user_name') !== 'admin') {
    next('/dashboard/home')
  } else {
    next()
  }
})

const app = createApp(App)
app.use(router)
app.mount('#app')
