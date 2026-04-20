import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import './style.css'

const Login = () => import('./views/Login.vue')
const Dashboard = () => import('./views/Dashboard.vue')
const UserInfo = () => import('./views/UserInfo.vue')
const Cloud115 = () => import('./views/Cloud115.vue')
const StrmConfig = () => import('./views/StrmConfig.vue')
const Settings = () => import('./views/Settings.vue')
const MediaManager = () => import('./views/MediaManager.vue')
const CategoryStrategy = () => import('./views/CategoryStrategy.vue')

const routes = [
  { path: '/', redirect: '/login' },
  { path: '/login', component: Login },
  {
    path: '/dashboard',
    component: Dashboard,
    redirect: '/dashboard/user-info',
    children: [
      { path: 'user-info', component: UserInfo },
      { path: 'cloud115', component: Cloud115 },
      { path: 'strm-config', component: StrmConfig },
      { path: 'settings', component: Settings },
      { path: 'media-manager', component: MediaManager },
      { path: 'category-strategy', component: CategoryStrategy }
    ]
  }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

// 路由守卫：检查登录状态
router.beforeEach((to, from, next) => {
  const token = localStorage.getItem('token')
  const isLoginPage = to.path === '/login'
  
  if (!token && !isLoginPage) {
    // 未登录且不是登录页，跳转到登录页
    next('/login')
  } else if (token && isLoginPage) {
    // 已登录且是登录页，跳转到首页
    next('/dashboard/user-info')
  } else {
    next()
  }
})

// 初始化主题：在应用挂载前读取 localStorage 中的主题偏好并应用
const savedTheme = localStorage.getItem('theme')
if (savedTheme === 'dark') {
  document.documentElement.classList.add('dark')
}

const app = createApp(App)
app.use(router)
app.mount('#app')
