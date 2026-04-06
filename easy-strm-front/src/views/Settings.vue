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

        <el-divider content-position="left">TMDB配置</el-divider>

        <el-form-item label="TMDB API Key">
          <el-input
            v-model="tmdbForm.api_key"
            type="password"
            placeholder="请输入 TMDB API Key"
            show-password
            clearable
          />
          <div class="form-tip">
            <template v-if="hasTmdbKey">
              <el-tag type="success" size="small">已配置</el-tag>
              输入新的 API Key 将覆盖原有配置
            </template>
            <template v-else>
              获取方式：访问 
              <el-link type="primary" href="https://www.themoviedb.org/settings/api" target="_blank">
                TMDB API 设置
              </el-link>
              创建应用获取 API Key
            </template>
          </div>
        </el-form-item>

        <el-form-item label="语言">
          <el-select v-model="tmdbForm.language" placeholder="请选择语言">
            <el-option label="简体中文" value="zh-CN" />
            <el-option label="繁体中文" value="zh-TW" />
            <el-option label="英语" value="en" />
            <el-option label="日语" value="ja" />
            <el-option label="韩语" value="ko" />
          </el-select>
          <div class="form-tip">
            设置 TMDB 搜索结果的默认语言
          </div>
        </el-form-item>

        <el-form-item class="form-actions">
          <el-button type="primary" @click="handleTmdbSubmit" :loading="tmdbLoading">
            <el-icon><Check /></el-icon>
            保存 TMDB 配置
          </el-button>
          <el-button @click="handleTmdbReset">
            <el-icon><RefreshRight /></el-icon>
            重置
          </el-button>
        </el-form-item>

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

        <el-divider content-position="left">整理更名配置</el-divider>

        <el-form-item label="电影命名模版">
          <el-input v-model="form.movie_naming_template" placeholder="{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if en_title and en_title != title %} - {{ en_title }}{% endif %}{{ fileExt }}" />
          <div class="template-tags">
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ title }}')">&#123;&#123; title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ en_title }}')">&#123;&#123; en_title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ year }}')">&#123;&#123; year &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ videoFormat }}')">&#123;&#123; videoFormat &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ fileExt }}')">&#123;&#123; fileExt &#125;&#125;</el-tag>
          </div>
          <div class="form-tip">
            Jinja2: &#123;&#123; title &#125;&#125;{% if year %} (&#123;&#123; year &#125;&#125;){% endif %}/&#123;&#123; title &#125;&#125;{% if en_title and en_title != title %} - &#123;&#123; en_title &#125;&#125;{% endif %}{% if year %} (&#123;&#123; year &#125;&#125;){% endif %}{% if videoFormat %} [&#123;&#123; videoFormat &#125;&#125;]{% endif %}&#123;&#123; fileExt &#125;&#125;
          </div>
        </el-form-item>

        <el-form-item label="电视剧命名模版">
          <el-input v-model="form.tv_naming_template" placeholder="{{ title }}/Season {{ &quot;%02d&quot;|format(season|int) }}/{{ title }} - S{{ &quot;%02d&quot;|format(season|int) }}E{{ &quot;%02d&quot;|format(episode|int) }}{{ fileExt }}" />
          <div class="template-tags">
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ title }}')">&#123;&#123; title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ en_title }}')">&#123;&#123; en_title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ season }}')">&#123;&#123; season &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ episode }}')">&#123;&#123; episode &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ year }}')">&#123;&#123; year &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ videoFormat }}')">&#123;&#123; videoFormat &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ fileExt }}')">&#123;&#123; fileExt &#125;&#125;</el-tag>
          </div>
          <div class="form-tip">
            Jinja2: &#123;&#123; title &#125;&#125;{% if year %} (&#123;&#123; year &#125;&#125;){% endif %}/Season &#123;&#123; "%02d"|format(season|int) &#125;&#125;/&#123;&#123; title &#125;&#125;{% if en_title and en_title != title %} - &#123;&#123; en_title &#125;&#125;{% endif %} - S&#123;&#123; "%02d"|format(season|int) &#125;&#125;E&#123;&#123; "%02d"|format(episode|int) &#125;&#125;{% if videoFormat %} [&#123;&#123; videoFormat &#125;&#125;]{% endif %}&#123;&#123; fileExt &#125;&#125;
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
import { getTmdbConfig, updateTmdbApiKey } from '../utils/api/media.js'

const formRef = ref(null)
const loading = ref(false)
const tmdbLoading = ref(false)
const initialForm = ref({})

const form = ref({
  alist_url: '',
  alist_token: '',
  log_save_day_limit: 1,
  movie_naming_template: '',
  tv_naming_template: ''
})

const tmdbForm = ref({
  api_key: '',
  language: 'zh-CN'
})

const initialTmdbForm = ref({
  api_key: '',
  language: 'zh-CN'
})

const hasTmdbKey = ref(false)

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
      log_save_day_limit: parseInt(data.log_save_day_limit) || 1,
      movie_naming_template: data.movie_naming_template || '',
      tv_naming_template: data.tv_naming_template || ''
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

const fetchTmdbConfig = async () => {
  try {
    const response = await getTmdbConfig()
    const data = response.data.data || {}
    hasTmdbKey.value = data.has_key || false
    tmdbForm.value = {
      api_key: data.api_key || '',
      language: data.language || 'zh-CN'
    }
    initialTmdbForm.value = { ...tmdbForm.value }
  } catch (error) {
    console.error('获取 TMDB 配置失败:', error)
  }
}

const handleSubmit = async () => {
  loading.value = true
  try {
    const settings = {
      alist_url: form.value.alist_url,
      alist_token: form.value.alist_token,
      log_save_day_limit: String(form.value.log_save_day_limit),
      movie_naming_template: form.value.movie_naming_template,
      tv_naming_template: form.value.tv_naming_template
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

const addTag = (type, tag) => {
  if (type === 'movie') {
    form.value.movie_naming_template += tag
  } else {
    form.value.tv_naming_template += tag
  }
}

const handleReset = () => {
  form.value = { ...initialForm.value }
}

const handleTmdbSubmit = async () => {
  if (!tmdbForm.value.api_key) {
    ElMessage.warning('请输入 TMDB API Key')
    return
  }
  tmdbLoading.value = true
  try {
    await updateTmdbApiKey({
      api_key: tmdbForm.value.api_key,
      language: tmdbForm.value.language
    })
    ElMessage.success('TMDB 配置保存成功')
    hasTmdbKey.value = true
    initialTmdbForm.value = { ...tmdbForm.value }
  } catch (error) {
    console.error('保存 TMDB 配置失败:', error)
    ElMessage.error('保存 TMDB 配置失败')
  } finally {
    tmdbLoading.value = false
  }
}

const handleTmdbReset = () => {
  tmdbForm.value = { ...initialTmdbForm.value }
}

onMounted(() => {
  fetchSettings()
  fetchTmdbConfig()
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

.template-tags {
  margin-top: 8px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tag-item {
  cursor: pointer;
  transition: all 0.3s;
}

.tag-item:hover {
  transform: translateY(-2px);
  box-shadow: 0 2px 8px rgba(0,0,0,0.1);
}

.form-actions {
  margin-top: 30px;
}

.form-actions :deep(.el-form-item__content) {
  justify-content: flex-start;
  gap: 10px;
}
</style>
