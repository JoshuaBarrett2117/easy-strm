import { createApp } from 'vue'
import { createRouter, createWebHistory } from 'vue-router'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import App from './App.vue'
import Login from './views/Login.vue'
import Dashboard from './views/Dashboard.vue'
import UserInfo from './views/UserInfo.vue'
import Cloud115 from './views/Cloud115.vue'
import StrmConfig from './views/StrmConfig.vue'
import './style.css'

const routes = [
  { path: '/', redirect: '/login' },
  { path: '/login', component: Login },
  { path: '/dashboard', component: Dashboard, children: [
    { path: 'user-info', component: UserInfo },
    { path: 'cloud115', component: Cloud115 },
    { path: 'strm-config', component: StrmConfig }
  ] }
]

const router = createRouter({
  history: createWebHistory(),
  routes
})

const app = createApp(App)
app.use(router)
app.use(ElementPlus)
app.mount('#app')
