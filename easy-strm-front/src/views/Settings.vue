<template>
  <div class="settings-container">
    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">
            <el-icon class="header-icon"><Setting /></el-icon>
            <span>系统配置</span>
          </div>
        </div>
      </template>

      <el-form
        ref="formRef"
        :model="form"
        label-width="140px"
        class="settings-form"
      >
        <el-divider content-position="left">Alist配置</el-divider>

        <el-form-item label="Alist服务器地址">
          <el-input
            v-model="form.alist_url"
            placeholder="例如：http://192.168.1.100:5244"
            clearable
          />
          <div class="form-tip">
            Alist服务器的地址，格式：http://your-alist-server:port
          </div>
        </el-form-item>

        <el-form-item label="Alist访问令牌">
          <el-input
            v-model="form.alist_token"
            type="password"
            placeholder="请输入Alist访问令牌"
            show-password
            clearable
          />
          <div class="form-tip">
            获取方式：登录Alist网页端 → 设置 → 左侧菜单"其他" → 查看"令牌"
          </div>
        </el-form-item>

        <el-alert
          v-if="alistHelpVisible"
          type="info"
          :closable="false"
          show-icon
          class="alist-help"
        >
          <template #title>
            Alist秒传配置说明
          </template>
          <ul class="alist-help-list">
            <li>确保Alist服务器可以正常访问</li>
            <li>确保Alist的"直链强度"设置为"弱"（允许获取直链）</li>
            <li>在115云管理页面配置秒传方式为alist时使用此配置</li>
          </ul>
        </el-alert>

        <el-divider content-position="left">其他配置</el-divider>

        <el-form-item label="日志保留天数">
          <el-input-number
            v-model="form.log_save_day_limit"
            :min="1"
            :max="365"
            :step="1"
          />
          <div class="form-tip">
            设置日志文件保留的天数，范围：1-365天
          </div>
        </el-form-item>

        <el-form-item class="form-actions">
          <el-button type="primary" @click="handleSubmit" :loading="loading">
            <el-icon><Check /></el-icon>
            保存配置
          </el-button>
          <el-button @click="handleReset">
            <el-icon><RefreshRight /></el-icon>
            重置
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import { Setting, Check, RefreshRight } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getSettings, updateSettings } from '../utils/api.js'

const formRef = ref(null)
const loading = ref(false)
const initialForm = ref({})

const form = ref({
  alist_url: '',
  alist_token: '',
  log_save_day_limit: 1
})

const alistHelpVisible = computed(() => {
  return form.value.alist_url || form.value.alist_token
})

const fetchSettings = async () => {
  try {
    const response = await getSettings()
    const data = response.data.data || {}
    form.value = {
      alist_url: data.alist_url || '',
      alist_token: data.alist_token || '',
      log_save_day_limit: parseInt(data.log_save_day_limit) || 1
    }
    initialForm.value = { ...form.value }
  } catch (error) {
    console.error('获取系统配置失败:', error)
    // 如果是404（配置不存在），使用默认值
    if (error.response?.status === 404) {
      form.value = {
        alist_url: '',
        alist_token: '',
        log_save_day_limit: 1
      }
      initialForm.value = { ...form.value }
    }
  }
}

const handleSubmit = async () => {
  loading.value = true
  try {
    const settings = {
      alist_url: form.value.alist_url,
      alist_token: form.value.alist_token,
      log_save_day_limit: String(form.value.log_save_day_limit)
    }
    await updateSettings(settings)
    ElMessage.success('配置保存成功')
    initialForm.value = { ...form.value }
  } catch (error) {
    console.error('保存配置失败:', error)
    ElMessage.error('保存配置失败')
  } finally {
    loading.value = false
  }
}

const handleReset = () => {
  form.value = { ...initialForm.value }
}

onMounted(() => {
  fetchSettings()
})
</script>

<style scoped>
.settings-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
}

.main-card {
  border-radius: 12px;
  overflow: hidden;
  max-width: 800px;
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

.settings-form {
  padding: 20px 0;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}

.alist-help {
  margin: 20px 40px;
}

.alist-help-list {
  margin: 10px 0 0 0;
  padding-left: 20px;
  color: #606266;
  font-size: 13px;
}

.alist-help-list li {
  margin: 5px 0;
}

.form-actions {
  margin-top: 30px;
}

.form-actions :deep(.el-form-item__content) {
  justify-content: flex-start;
  gap: 10px;
}
</style>