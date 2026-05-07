<template>
  <div class="strm-generator-container">
    <section class="hero-panel">
      <div class="hero-copy">
        <el-tag type="success" effect="dark" round>独立生成入口</el-tag>
        <h1>直接发起 STRM 生成</h1>
        <p>保留原有 STRM 生成接口，用于在不经过配置中心的情况下临时指定 115 账号、网盘目录和本地目录发起生成。</p>
      </div>
      <div class="hero-metrics">
        <article class="metric-card">
          <span>可用 115 账号</span>
          <strong>{{ cloud115List.length }}</strong>
        </article>
        <article class="metric-card">
          <span>当前状态</span>
          <strong>{{ loading ? '生成中' : '待执行' }}</strong>
        </article>
      </div>
    </section>

    <el-card shadow="hover" class="main-card">
      <template #header>
        <div class="card-header">
          <div class="header-title">生成参数</div>
          <el-button @click="resetForm">重置</el-button>
        </div>
      </template>

      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px" class="generator-form">
        <el-form-item label="115账号" prop="cloud115Id">
          <el-select v-model="form.cloud115Id" placeholder="请选择115账号" style="width: 100%">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="网盘媒体库目录" prop="netDiskPath">
          <el-input v-model="form.netDiskPath" placeholder="请输入 115 网盘媒体库目录路径" />
        </el-form-item>

        <el-form-item label="本地媒体库目录" prop="localPath">
          <el-input v-model="form.localPath" placeholder="请输入本地媒体库目录路径" />
        </el-form-item>

        <div class="form-actions">
          <el-button type="primary" @click="handleGenerate" :loading="loading">生成 STRM 文件</el-button>
          <el-button @click="resetForm">清空参数</el-button>
        </div>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { request } from '../utils/api'

const cloud115List = ref([])
const formRef = ref(null)
const loading = ref(false)
const form = ref({
  cloud115Id: '',
  netDiskPath: '',
  localPath: ''
})

const rules = {
  cloud115Id: [{ required: true, message: '请选择115账号', trigger: 'blur' }],
  netDiskPath: [{ required: true, message: '请输入网盘媒体库目录', trigger: 'blur' }],
  localPath: [{ required: true, message: '请输入本地媒体库目录', trigger: 'blur' }]
}

const fetchCloud115List = async () => {
  try {
    const response = await request('/cloud115')
    const apiData = response.data.data
    cloud115List.value = Array.isArray(apiData) ? apiData : (apiData?.data || [])
  } catch (_error) {
    ElMessage.error('获取115账号列表失败')
  }
}

const handleGenerate = () => {
  formRef.value.validate((valid) => {
    if (!valid) return

    loading.value = true
    request('/strm/generate', {
      method: 'POST',
      data: form.value
    }).then(() => {
      ElMessage.success('STRM 文件生成成功')
      resetForm()
    }).catch(() => {
      ElMessage.error('STRM 文件生成失败')
    }).finally(() => {
      loading.value = false
    })
  })
}

const resetForm = () => {
  form.value = {
    cloud115Id: '',
    netDiskPath: '',
    localPath: ''
  }
  if (formRef.value) {
    formRef.value.resetFields()
  }
}

onMounted(() => {
  fetchCloud115List()
})
</script>

<style scoped>
.strm-generator-container {
  padding: 20px;
  min-height: calc(100vh - 100px);
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.hero-panel {
  display: grid;
  grid-template-columns: minmax(0, 2fr) minmax(260px, 1fr);
  gap: 20px;
  padding: 28px;
  border-radius: 28px;
  background:
    radial-gradient(circle at top left, rgba(70, 167, 137, 0.16), transparent 30%),
    radial-gradient(circle at bottom right, rgba(244, 176, 88, 0.18), transparent 28%),
    linear-gradient(135deg, #17313a 0%, #214852 48%, #2a6d73 100%);
  color: #f5f7f2;
}

.hero-copy h1 {
  margin: 16px 0 10px;
  font-size: 32px;
  line-height: 1.2;
}

.hero-copy p {
  margin: 0;
  color: rgba(245, 247, 242, 0.82);
  line-height: 1.75;
}

.hero-metrics {
  display: grid;
  gap: 14px;
}

.metric-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.1);
  border: 1px solid rgba(255, 255, 255, 0.12);
  backdrop-filter: blur(12px);
}

.metric-card span {
  display: block;
  font-size: 12px;
  color: rgba(245, 247, 242, 0.72);
}

.metric-card strong {
  display: block;
  margin-top: 10px;
  font-size: 28px;
}

.main-card {
  border-radius: 24px;
  overflow: hidden;
  border: 1px solid rgba(120, 101, 72, 0.12);
  background: rgba(255, 252, 247, 0.84);
  box-shadow: 0 24px 60px rgba(58, 42, 24, 0.08);
}

.main-card :deep(.el-card__header) {
  background:
    radial-gradient(circle at top right, rgba(242, 166, 90, 0.28), transparent 32%),
    linear-gradient(135deg, #1f6f78 0%, #24535f 55%, #17313a 100%);
  padding: 20px 24px;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.header-title {
  color: white;
  font-size: 22px;
  font-weight: 700;
}

.generator-form {
  padding-top: 8px;
}

.form-actions {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 24px;
}

:global(.dark) .main-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}

@media (max-width: 768px) {
  .strm-generator-container {
    padding: 12px;
  }

  .hero-panel {
    grid-template-columns: 1fr;
    padding: 20px;
  }

  .hero-copy h1 {
    font-size: 26px;
  }

  .form-actions,
  .card-header {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
