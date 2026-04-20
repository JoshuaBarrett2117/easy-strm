<template>
  <div class="login-container">
    <div class="login-background">
      <div class="bg-shape bg-shape-1"></div>
      <div class="bg-shape bg-shape-2"></div>
      <div class="bg-shape bg-shape-3"></div>
    </div>
    <el-card class="login-card" shadow="always">
      <template #header>
        <div class="login-header">
          <div class="logo-container">
            <svg class="logo-svg" viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg">
              <defs>
                <linearGradient id="bgGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" style="stop-color:#667eea"/>
                  <stop offset="100%" style="stop-color:#764ba2"/>
                </linearGradient>
                <linearGradient id="playGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" style="stop-color:#38ef7d"/>
                  <stop offset="100%" style="stop-color:#11998e"/>
                </linearGradient>
              </defs>
              <circle cx="32" cy="32" r="30" fill="url(#bgGrad)"/>
              <circle cx="32" cy="32" r="26" fill="none" stroke="rgba(255,255,255,0.2)" stroke-width="2"/>
              <polygon points="26,20 26,44 46,32" fill="url(#playGrad)" stroke="white" stroke-width="1.5" stroke-linejoin="round"/>
              <circle cx="50" cy="14" r="8" fill="#ffd93d" opacity="0.9"/>
              <path d="M50 10 L51 13 L54 13.5 L52 15.5 L52.5 18.5 L50 17 L47.5 18.5 L48 15.5 L46 13.5 L49 13 Z" fill="white"/>
            </svg>
          </div>
          <h2>Easy Stream</h2>
          <p>115网盘STRM文件管理系统</p>
        </div>
      </template>
      
      <el-form ref="formRef" :model="form" :rules="rules" label-width="80px" class="login-form" @submit.prevent="handleLogin">
        <el-form-item label="用户名" prop="name">
          <el-input v-model="form.name" placeholder="请输入用户名" :prefix-icon="User" @keyup.enter="handleLogin" />
        </el-form-item>
        <el-form-item label="密码" prop="password">
          <el-input v-model="form.password" type="password" placeholder="请输入密码" :prefix-icon="Lock" show-password @keyup.enter="handleLogin" />
        </el-form-item>
        <el-form-item class="login-btn-container">
          <el-button type="primary" @click="handleLogin" :loading="loading" class="login-btn">
            <el-icon v-if="!loading"><Right /></el-icon>
            登录系统
          </el-button>
        </el-form-item>
      </el-form>
      
      <div class="login-footer">
        <p>安全登录 · 数据加密</p>
      </div>
    </el-card>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { ElMessage } from 'element-plus'
import { Lock, Right, User } from '@element-plus/icons-vue'
import { login } from '../utils/api/auth'
import md5 from 'crypto-js/md5'

const formRef = ref(null)
const loading = ref(false)
const form = ref({
  name: '',
  password: ''
})

const rules = {
  name: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  // 仅做非空校验，避免把后端真实默认密码误判为无效
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

/**
 * 处理登录请求
 */
const handleLogin = () => {
  formRef.value.validate((valid) => {
    if (valid) {
      loading.value = true
      
      const md5Password = md5(form.value.password).toString()
      
      login({
        name: form.value.name.trim(),
        password: md5Password
      }, {
        skipGlobalErrorMessage: true
      }).catch((error) => {
        const errorMsg = error.response?.data?.error || error.message || '登录失败，请检查用户名和密码'
        ElMessage.error(errorMsg)
      }).finally(() => {
        loading.value = false
      })
    }
  })
}
</script>

<style scoped>
.login-container {
  display: flex;
  justify-content: center;
  align-items: center;
  height: 100vh;
  background: linear-gradient(135deg, #1a1a2e 0%, #16213e 50%, #0f3460 100%);
  position: relative;
  overflow: hidden;
}

.login-background {
  position: absolute;
  width: 100%;
  height: 100%;
  overflow: hidden;
}

.bg-shape {
  position: absolute;
  border-radius: 50%;
  opacity: 0.1;
}

.bg-shape-1 {
  width: 400px;
  height: 400px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  top: -100px;
  left: -100px;
  animation: float 8s ease-in-out infinite;
}

.bg-shape-2 {
  width: 300px;
  height: 300px;
  background: linear-gradient(135deg, #11998e 0%, #38ef7d 100%);
  bottom: -50px;
  right: -50px;
  animation: float 6s ease-in-out infinite reverse;
}

.bg-shape-3 {
  width: 200px;
  height: 200px;
  background: linear-gradient(135deg, #fc466b 0%, #3f5efb 100%);
  top: 50%;
  left: 50%;
  transform: translate(-50%, -50%);
  animation: pulse 4s ease-in-out infinite;
}

@keyframes float {
  0%, 100% {
    transform: translateY(0) rotate(0deg);
  }
  50% {
    transform: translateY(-30px) rotate(10deg);
  }
}

@keyframes pulse {
  0%, 100% {
    transform: translate(-50%, -50%) scale(1);
    opacity: 0.1;
  }
  50% {
    transform: translate(-50%, -50%) scale(1.2);
    opacity: 0.15;
  }
}

.login-card {
  width: 420px;
  border-radius: 16px;
  z-index: 10;
  backdrop-filter: blur(10px);
  background: rgba(255, 255, 255, 0.95);
  border: none;
  box-shadow: 0 25px 50px -12px rgba(0, 0, 0, 0.25);
}

.login-card :deep(.el-card__header) {
  border-bottom: none;
  padding-bottom: 0;
}

.login-header {
  text-align: center;
  margin-bottom: 10px;
}

.logo-container {
  width: 70px;
  height: 70px;
  margin: 0 auto 15px;
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 10px 30px rgba(102, 126, 234, 0.4);
}

.logo-svg {
  width: 70px;
  height: 70px;
}

.login-header h2 {
  color: #1a1a2e;
  margin-bottom: 8px;
  font-size: 26px;
  font-weight: 700;
  letter-spacing: 1px;
}

.login-header p {
  color: #909399;
  font-size: 14px;
  font-weight: 400;
}

.login-form {
  margin-top: 20px;
  padding: 0 10px;
}

.login-form :deep(.el-form-item__label) {
  font-weight: 500;
  color: #606266;
}

.login-form :deep(.el-input__wrapper) {
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  transition: all 0.3s ease;
}

.login-form :deep(.el-input__wrapper:hover) {
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.12);
}

.login-form :deep(.el-input__wrapper.is-focus) {
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.3);
}

.login-btn-container {
  margin-top: 30px;
}

.login-btn {
  width: 100%;
  height: 46px;
  font-size: 16px;
  font-weight: 600;
  border-radius: 8px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  border: none;
  box-shadow: 0 4px 15px rgba(102, 126, 234, 0.4);
  transition: all 0.3s ease;
}

.login-btn:hover {
  transform: translateY(-2px);
  box-shadow: 0 8px 25px rgba(102, 126, 234, 0.5);
}

.login-btn:active {
  transform: translateY(0);
}

.login-footer {
  text-align: center;
  padding-top: 15px;
  border-top: 1px solid #ebeef5;
  margin-top: 20px;
}

.login-footer p {
  color: #c0c4cc;
  font-size: 12px;
  margin: 0;
}
</style>
