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

      <el-form ref="formRef" :model="form" label-width="140px" class="settings-form">
        <el-divider content-position="left">Alist 配置</el-divider>

        <el-form-item label="Alist 服务地址">
          <el-input v-model="form.alist_url" placeholder="例如：http://192.168.1.100:5244" clearable />
          <div class="form-tip">用于 Alist 秒传场景，格式示例：`http://your-alist-server:port`。</div>
        </el-form-item>

        <el-form-item label="Alist 访问令牌">
          <el-input v-model="form.alist_token" type="password" placeholder="请输入 Alist 访问令牌" show-password clearable />
          <div class="form-tip">可在 Alist 设置页面获取 Token。</div>
        </el-form-item>

        <el-alert v-if="alistHelpVisible" type="info" :closable="false" show-icon class="alist-help">
          <template #title>Alist 秒传配置说明</template>
          <ul class="alist-help-list">
            <li>确认 Alist 服务可被当前部署环境访问。</li>
            <li>确认 Alist 已开启直链相关能力。</li>
            <li>115 账号中选择 `alist` 秒传方式时会用到这里的配置。</li>
          </ul>
        </el-alert>

        <el-divider content-position="left">TMDB 配置</el-divider>

        <el-form-item label="TMDB API Key">
          <el-input v-model="tmdbForm.api_key" type="password" placeholder="请输入 TMDB API Key" show-password clearable />
          <div class="form-tip">
            <template v-if="hasTmdbKey">
              <el-tag type="success" size="small">已配置</el-tag>
              输入新的 API Key 将覆盖已有值。
            </template>
            <template v-else>
              可前往
              <el-link type="primary" href="https://www.themoviedb.org/settings/api" target="_blank">TMDB API 设置页</el-link>
              创建并获取 API Key。
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
          <div class="form-tip">用于 TMDB 检索结果的默认语言。</div>
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

        <el-divider content-position="left">其它配置</el-divider>

        <el-form-item label="日志保留天数">
          <el-input-number v-model="form.log_save_day_limit" :min="1" :max="365" :step="1" />
          <div class="form-tip">日志文件保留天数，范围 1-365 天。</div>
        </el-form-item>

        <el-divider content-position="left">网络代理配置</el-divider>

        <el-form-item label="代理服务器地址">
          <el-input v-model="form.proxy_url" placeholder="例如：http://127.0.0.1:7890" clearable />
          <div class="form-tip">支持 `http://host:port` 或 `socks5://host:port`，留空表示不启用代理。</div>
        </el-form-item>

        <el-form-item label="代理站点列表">
          <el-input
            v-model="form.proxy_domains"
            type="textarea"
            :rows="3"
            placeholder="支持逗号或换行，例如：tg,github"
          />
          <div class="form-tip">按域名匹配，可填 `tg`、`github` 或完整域名（如 `api.telegram.org`）。</div>
        </el-form-item>

        <el-divider content-position="left">整理命名配置</el-divider>

        <el-form-item label="电影命名模板">
          <el-input
            v-model="form.movie_naming_template"
            placeholder="{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{{ fileExt }}"
          />
          <div class="template-tags">
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ title }}')">&#123;&#123; title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ en_title }}')">&#123;&#123; en_title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ year }}')">&#123;&#123; year &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ videoFormat }}')">&#123;&#123; videoFormat &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('movie', '{{ fileExt }}')">&#123;&#123; fileExt &#125;&#125;</el-tag>
          </div>
        </el-form-item>

        <el-form-item label="电视剧命名模板">
          <el-input
            v-model="form.tv_naming_template"
            placeholder="{{ title }}/Season {{ '%02d'|format(season|int) }}/{{ title }} - S{{ '%02d'|format(season|int) }}E{{ '%02d'|format(episode|int) }}{{ fileExt }}"
          />
          <div class="template-tags">
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ title }}')">&#123;&#123; title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ en_title }}')">&#123;&#123; en_title &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ season }}')">&#123;&#123; season &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ episode }}')">&#123;&#123; episode &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ year }}')">&#123;&#123; year &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ videoFormat }}')">&#123;&#123; videoFormat &#125;&#125;</el-tag>
            <el-tag size="small" class="tag-item" @click="addTag('tv', '{{ fileExt }}')">&#123;&#123; fileExt &#125;&#125;</el-tag>
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
import { getSettings, updateSettings } from '../utils/api'
import { getTmdbConfig, updateTmdbApiKey } from '../utils/api/media'

const formRef = ref(null)
const loading = ref(false)
const tmdbLoading = ref(false)
const initialForm = ref({})

const form = ref({
  alist_url: '',
  alist_token: '',
  log_save_day_limit: 1,
  proxy_url: '',
  proxy_domains: '',
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
      log_save_day_limit: parseInt(data.log_save_day_limit, 10) || 1,
      proxy_url: data.proxy_url || '',
      proxy_domains: data.proxy_domains || '',
      movie_naming_template: data.movie_naming_template || '',
      tv_naming_template: data.tv_naming_template || ''
    }
    initialForm.value = { ...form.value }
  } catch (error) {
    console.error('获取系统配置失败:', error)
    if (error.response?.status === 404) {
      form.value = {
        alist_url: '',
        alist_token: '',
        log_save_day_limit: 1,
        proxy_url: '',
        proxy_domains: '',
        movie_naming_template: '',
        tv_naming_template: ''
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
      proxy_url: form.value.proxy_url,
      proxy_domains: form.value.proxy_domains,
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
  max-width: 900px;
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
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);
}

.form-actions {
  margin-top: 30px;
}

.form-actions :deep(.el-form-item__content) {
  justify-content: flex-start;
  gap: 10px;
}
</style>
