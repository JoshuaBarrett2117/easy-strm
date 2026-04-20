<template>
  <div class="user-info-container">
    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><User /></el-icon>
            <span>用户信息</span>
          </div>
          <el-button type="primary" size="small" @click="fetchUserInfo" :loading="loading">
            <el-icon><RefreshRight /></el-icon>
            <span>刷新</span>
          </el-button>
        </div>
      </template>
      <div v-if="loading && !userInfo" class="loading-wrapper">
        <el-icon class="loading-icon is-loading"><Loading /></el-icon>
        <span>加载中...</span>
      </div>
      <div v-else-if="userInfo" class="user-info-content">
        <div class="user-avatar">
          <div class="avatar">
            <svg class="avatar-svg" viewBox="0 0 64 64" xmlns="http://www.w3.org/2000/svg">
              <defs>
                <linearGradient id="avatarBgGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" style="stop-color:#667eea"/>
                  <stop offset="100%" style="stop-color:#764ba2"/>
                </linearGradient>
                <linearGradient id="avatarPlayGrad" x1="0%" y1="0%" x2="100%" y2="100%">
                  <stop offset="0%" style="stop-color:#38ef7d"/>
                  <stop offset="100%" style="stop-color:#11998e"/>
                </linearGradient>
              </defs>
              <circle cx="32" cy="32" r="30" fill="url(#avatarBgGrad)"/>
              <circle cx="32" cy="32" r="26" fill="none" stroke="rgba(255,255,255,0.2)" stroke-width="2"/>
              <polygon points="26,20 26,44 46,32" fill="url(#avatarPlayGrad)" stroke="white" stroke-width="1.5" stroke-linejoin="round"/>
              <circle cx="50" cy="14" r="8" fill="#ffd93d" opacity="0.9"/>
              <path d="M50 10 L51 13 L54 13.5 L52 15.5 L52.5 18.5 L50 17 L47.5 18.5 L48 15.5 L46 13.5 L49 13 Z" fill="white"/>
            </svg>
          </div>
          <h3 class="user-name">{{ userInfo.name || '用户' }}</h3>
        </div>
        <el-descriptions :column="1" border class="user-descriptions">
          <el-descriptions-item label-align="right" label-class-name="desc-label">
            <template #label>
              <div class="label-content">
                <el-icon><Key /></el-icon>
                <span>用户ID</span>
              </div>
            </template>
            <el-tag type="info">{{ userInfo.id || '未知' }}</el-tag>
          </el-descriptions-item>
          <el-descriptions-item label-align="right">
            <template #label>
              <div class="label-content">
                <el-icon><User /></el-icon>
                <span>用户名</span>
              </div>
            </template>
            {{ userInfo.name || '未知' }}
          </el-descriptions-item>
          <el-descriptions-item label-align="right">
            <template #label>
              <div class="label-content">
                <el-icon><Calendar /></el-icon>
                <span>创建时间</span>
              </div>
            </template>
            {{ userInfo.create_time || '未知' }}
          </el-descriptions-item>
          <el-descriptions-item label-align="right">
            <template #label>
              <div class="label-content">
                <el-icon><Timer /></el-icon>
                <span>更新时间</span>
              </div>
            </template>
            {{ userInfo.update_time || '未知' }}
          </el-descriptions-item>
        </el-descriptions>
      </div>
      <el-empty v-else description="无法获取用户信息" />
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { getUserInfo } from '../utils/api/auth'
import { ElMessage } from 'element-plus'
import { User, Key, Calendar, Timer, RefreshRight, Loading } from '@element-plus/icons-vue'

const userInfo = ref(null)
const loading = ref(false)
let isMounted = true

const fetchUserInfo = async () => {
  loading.value = true
  try {
    const response = await getUserInfo({ skipGlobalErrorMessage: true })
    if (isMounted) {
      userInfo.value = response.data.data || {}
    }
  } catch (error) {
    if (isMounted) {
      ElMessage.error('获取用户信息失败')
    }
  } finally {
    if (isMounted) {
      loading.value = false
    }
  }
}

onMounted(() => {
  isMounted = true
  fetchUserInfo()
})

onUnmounted(() => {
  isMounted = false
})
</script>

<style scoped>
.user-info-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
}

.main-card {
  max-width: 700px;
  margin: 0 auto;
  border-radius: 12px;
  overflow: hidden;
}

.main-card :deep(.el-card__header) {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  padding: 16px 20px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-size: 18px;
  font-weight: 600;
}

.header-icon {
  font-size: 22px;
}

.loading-wrapper {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 60px 20px;
  gap: 16px;
  color: #909399;
}

.loading-icon {
  font-size: 32px;
  color: #409eff;
}

.user-info-content {
  padding: 10px 0;
}

.user-avatar {
  display: flex;
  flex-direction: column;
  align-items: center;
  margin-bottom: 30px;
}

.avatar {
  width: 80px;
  height: 80px;
  margin-bottom: 12px;
}

.avatar-svg {
  width: 80px;
  height: 80px;
}

.user-name {
  margin: 0;
  font-size: 20px;
  font-weight: 600;
  color: #303133;
}

.user-descriptions {
  border-radius: 8px;
  overflow: hidden;
}

.user-descriptions :deep(.el-descriptions__label) {
  width: 140px;
  background-color: #f8f9fa;
  font-weight: 500;
}

.user-descriptions :deep(.el-descriptions__cell) {
  padding: 14px 16px;
}

.label-content {
  display: flex;
  align-items: center;
  gap: 8px;
}

.label-content .el-icon {
  color: #667eea;
}
</style>

