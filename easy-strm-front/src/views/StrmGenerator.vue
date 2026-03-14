<template>
  <div class="strm-generator-container">
    <el-card shadow="hover">
      <template #header>
        <div class="card-header">
          <span>生成STRM文件</span>
        </div>
      </template>
      
      <el-form ref="formRef" :model="form" :rules="rules" label-width="120px">
        <el-form-item label="115账号" prop="cloud115Id">
          <el-select v-model="form.cloud115Id" placeholder="请选择115账号">
            <el-option
              v-for="account in cloud115List"
              :key="account.id"
              :label="account.name"
              :value="account.id"
            />
          </el-select>
        </el-form-item>
        
        <el-form-item label="网盘媒体库目录" prop="netDiskPath">
          <el-input v-model="form.netDiskPath" placeholder="请输入115网盘媒体库目录路径" />
        </el-form-item>
        
        <el-form-item label="本地媒体库目录" prop="localPath">
          <el-input v-model="form.localPath" placeholder="请输入本地媒体库目录路径" />
        </el-form-item>
        
        <el-form-item>
          <el-button type="primary" @click="handleGenerate" :loading="loading">生成STRM文件</el-button>
          <el-button @click="resetForm">重置</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { request } from '../utils/api'

// 115账号列表
const cloud115List = ref([])

// 表单数据
const formRef = ref(null)
const loading = ref(false)
const form = ref({
  cloud115Id: '',
  netDiskPath: '',
  localPath: ''
})

// 表单验证规则
const rules = {
  cloud115Id: [{ required: true, message: '请选择115账号', trigger: 'blur' }],
  netDiskPath: [{ required: true, message: '请输入网盘媒体库目录', trigger: 'blur' }],
  localPath: [{ required: true, message: '请输入本地媒体库目录', trigger: 'blur' }]
}

// 获取115账号列表
const fetchCloud115List = async () => {
  try {
    const response = await request('/cloud115')
    cloud115List.value = response.data.data || []
  } catch (error) {
    ElMessage.error('获取115账号列表失败')
  }
}

// 生成STRM文件
const handleGenerate = () => {
  formRef.value.validate((valid) => {
    if (valid) {
      loading.value = true
      
      request('/strm/generate', {
        method: 'POST',
        data: form.value
      }).then(() => {
        ElMessage.success('STRM文件生成成功')
        resetForm()
      }).catch(() => {
        ElMessage.error('STRM文件生成失败')
      }).finally(() => {
        loading.value = false
      })
    }
  })
}

// 重置表单
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

// 初始化
onMounted(() => {
  fetchCloud115List()
})
</script>

<style scoped>
.strm-generator-container {
  padding: 20px;
  background-color: #f5f5f5;
  min-height: 100vh;
}

.card-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.form-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  margin-top: 20px;
}
</style>