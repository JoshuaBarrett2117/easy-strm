<template>
  <div class="space-y-4">
    <PageCard title="系统配置" subtitle="管理 Alist、TMDB、Emby、代理与整理刮削等全局设置">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <!-- 基础设置:Alist / 日志 / 代理 -->
        <n-tab-pane name="basic" tab="基础设置">
          <n-form :model="form" label-placement="top" class="max-w-2xl">
            <h3 class="mb-3 text-sm font-bold text-slate-800 dark:text-white">Alist 配置</h3>

            <n-form-item label="Alist 服务地址">
              <div class="w-full">
                <n-input
                  v-model:value="form.alist_url"
                  placeholder="例如:http://192.168.1.100:5244"
                  clearable
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  用于 Alist 秒传场景,格式示例:`http://your-alist-server:port`。
                </p>
              </div>
            </n-form-item>

            <n-form-item label="Alist 访问令牌">
              <div class="w-full">
                <n-input
                  v-model:value="form.alist_token"
                  type="password"
                  show-password-on="click"
                  placeholder="请输入 Alist 访问令牌"
                  clearable
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">可在 Alist 设置页面获取 Token。</p>
              </div>
            </n-form-item>

            <n-alert v-if="alistHelpVisible" type="info" title="Alist 秒传配置说明" class="mb-4">
              <ul class="list-disc space-y-1 pl-5 text-xs">
                <li>确认 Alist 服务可被当前部署环境访问。</li>
                <li>确认 Alist 已开启直链相关能力。</li>
                <li>115 账号中选择 `alist` 秒传方式时会用到这里的配置。</li>
              </ul>
            </n-alert>

            <h3 class="mb-3 mt-6 text-sm font-bold text-slate-800 dark:text-white">日志配置</h3>

            <n-form-item label="日志保留天数">
              <div class="w-full">
                <n-input-number
                  v-model:value="form.log_save_day_limit"
                  :min="1"
                  :max="365"
                  :step="1"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">日志文件保留天数,范围 1-365 天。</p>
              </div>
            </n-form-item>

            <h3 class="mb-3 mt-6 text-sm font-bold text-slate-800 dark:text-white">网络代理配置</h3>

            <n-form-item label="代理服务器地址">
              <div class="w-full">
                <n-input
                  v-model:value="form.proxy_url"
                  placeholder="例如:http://127.0.0.1:7890"
                  clearable
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  支持 `http://host:port` 或 `socks5://host:port`,留空表示不启用代理。
                </p>
              </div>
            </n-form-item>

            <n-form-item label="代理站点列表">
              <div class="w-full">
                <n-input
                  v-model:value="form.proxy_domains"
                  type="textarea"
                  :rows="3"
                  placeholder="支持逗号或换行,例如:tg,github"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  按域名匹配,可填 `tg`、`github` 或完整域名(如 `api.telegram.org`)。
                </p>
              </div>
            </n-form-item>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="loading" @click="handleSubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存配置
              </n-button>
              <n-button @click="handleReset">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                重置
              </n-button>
            </div>
          </n-form>
        </n-tab-pane>

        <!-- TMDB -->
        <n-tab-pane name="tmdb" tab="TMDB">
          <n-form :model="tmdbForm" label-placement="top" class="max-w-2xl">
            <n-form-item label="TMDB API Key">
              <div class="w-full">
                <n-input
                  v-model:value="tmdbForm.api_key"
                  type="password"
                  show-password-on="click"
                  :placeholder="
                    tmdbMaskedKey
                      ? `当前已配置:${tmdbMaskedKey},留空则保持不变`
                      : '请输入 TMDB API Key'
                  "
                  clearable
                />
                <p class="mt-1 flex flex-wrap items-center gap-1 text-xs text-slate-400 dark:text-slate-500">
                  <template v-if="hasTmdbKey">
                    <n-tag type="success" size="small">已配置</n-tag>
                    输入新的 API Key 将覆盖已有值。
                  </template>
                  <template v-else>
                    可前往
                    <a
                      href="https://www.themoviedb.org/settings/api"
                      target="_blank"
                      rel="noopener noreferrer"
                      class="text-cyan-600 hover:underline dark:text-cyan-400"
                    >TMDB API 设置页</a>
                    创建并获取 API Key。
                  </template>
                </p>
              </div>
            </n-form-item>

            <n-form-item label="语言">
              <div class="w-full">
                <n-select
                  v-model:value="tmdbForm.language"
                  :options="tmdbLanguageOptions"
                  placeholder="请选择语言"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">用于 TMDB 检索结果的默认语言。</p>
              </div>
            </n-form-item>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="tmdbLoading" @click="handleTmdbSubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存 TMDB 配置
              </n-button>
              <n-button @click="handleTmdbReset">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                重置
              </n-button>
            </div>
          </n-form>
        </n-tab-pane>

        <!-- Emby -->
        <n-tab-pane name="emby" tab="Emby">
          <n-form :model="embyForm" label-placement="top" class="max-w-2xl">
            <n-form-item label="启用 Emby 集成">
              <div class="w-full">
                <n-switch v-model:value="embyForm.enabled">
                  <template #checked>启用</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">启用后,整理完成时可自动刷新 Emby 媒体库。</p>
              </div>
            </n-form-item>

            <n-form-item label="Emby 服务地址">
              <div class="w-full">
                <n-input
                  v-model:value="embyForm.emby_url"
                  placeholder="例如:http://emby:8096"
                  clearable
                  :disabled="!embyForm.enabled"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">Emby 服务器的完整访问地址,包含端口号。</p>
              </div>
            </n-form-item>

            <n-form-item label="Emby API Key">
              <div class="w-full">
                <n-input
                  v-model:value="embyForm.emby_api_key"
                  type="password"
                  show-password-on="click"
                  placeholder="请输入 Emby API Key"
                  clearable
                  :disabled="!embyForm.enabled"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  可在 Emby 后台「设置 → 高级 → API 密钥」中创建。
                </p>
              </div>
            </n-form-item>

            <div
              v-if="embyForm.enabled && embyForm.emby_url"
              class="mb-2 flex flex-wrap items-center gap-3"
            >
              <n-button type="success" :loading="embyTesting" @click="testEmbyConnection">
                <template #icon><n-icon :component="LinkOutline" /></template>
                测试连接
              </n-button>
              <n-tag
                v-if="embyConnectionStatus !== null"
                :type="embyConnectionStatus ? 'success' : 'error'"
                size="small"
              >
                {{ embyConnectionStatus ? '连接成功' : '连接失败' }}
              </n-tag>
              <span v-if="embyServerInfo" class="text-xs text-slate-400 dark:text-slate-500">
                {{ embyServerInfo.ServerName }} (v{{ embyServerInfo.Version }})
              </span>
            </div>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="embyLoading" @click="handleEmbySubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存 Emby 配置
              </n-button>
              <n-button @click="handleEmbyReset">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                重置
              </n-button>
            </div>
          </n-form>
        </n-tab-pane>

        <!-- 刮削 -->
        <n-tab-pane name="scrape" tab="刮削">
          <n-form :model="form" label-placement="top" class="max-w-2xl">
            <n-form-item label="整理后同步刮削">
              <div class="w-full">
                <n-switch v-model:value="form.scrape_enabled_on_organize">
                  <template #checked>开启</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">整理成功后自动对本地目标文件执行刮削。</p>
              </div>
            </n-form-item>

            <n-form-item label="生成 NFO">
              <div class="w-full">
                <n-switch v-model:value="form.scrape_write_nfo">
                  <template #checked>开启</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">关闭后只生成图片侧车,不写入 NFO 文件。</p>
              </div>
            </n-form-item>

            <n-form-item label="下载海报">
              <n-switch v-model:value="form.scrape_write_poster">
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </n-switch>
            </n-form-item>

            <n-form-item label="下载背景图">
              <n-switch v-model:value="form.scrape_write_fanart">
                <template #checked>开启</template>
                <template #unchecked>关闭</template>
              </n-switch>
            </n-form-item>

            <n-form-item label="下载剧照">
              <div class="w-full">
                <n-switch v-model:value="form.scrape_write_thumb">
                  <template #checked>开启</template>
                  <template #unchecked>关闭</template>
                </n-switch>
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">剧照仅对剧集分集生效。</p>
              </div>
            </n-form-item>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="loading" @click="handleSubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存配置
              </n-button>
              <n-button @click="handleReset">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                重置
              </n-button>
            </div>
          </n-form>
        </n-tab-pane>

        <!-- 命名模板 -->
        <n-tab-pane name="naming" tab="命名模板">
          <n-form :model="form" label-placement="top" class="max-w-2xl">
            <n-form-item label="电影命名模板">
              <div class="w-full">
                <n-input
                  v-model:value="form.movie_naming_template"
                  placeholder="{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{{ fileExt }}"
                />
                <div class="mt-2 flex flex-wrap gap-2">
                  <n-tag
                    v-for="tag in movieTemplateTags"
                    :key="tag"
                    size="small"
                    class="cursor-pointer transition-transform hover:-translate-y-0.5"
                    @click="addTag('movie', tag)"
                  >
                    {{ tag }}
                  </n-tag>
                </div>
              </div>
            </n-form-item>

            <n-form-item label="电视剧命名模板">
              <div class="w-full">
                <n-input
                  v-model:value="form.tv_naming_template"
                  placeholder="{{ title }}/Season {{ '%02d'|format(season|int) }}/{{ title }} - S{{ '%02d'|format(season|int) }}E{{ '%02d'|format(episode|int) }}{{ fileExt }}"
                />
                <div class="mt-2 flex flex-wrap gap-2">
                  <n-tag
                    v-for="tag in tvTemplateTags"
                    :key="tag"
                    size="small"
                    class="cursor-pointer transition-transform hover:-translate-y-0.5"
                    @click="addTag('tv', tag)"
                  >
                    {{ tag }}
                  </n-tag>
                </div>
              </div>
            </n-form-item>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="loading" @click="handleSubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存配置
              </n-button>
              <n-button @click="handleReset">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                重置
              </n-button>
            </div>
          </n-form>
        </n-tab-pane>
      </n-tabs>
    </PageCard>
  </div>
</template>

<script setup>
import { ref, onMounted, computed } from 'vue'
import {
  NTabs,
  NTabPane,
  NForm,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NButton,
  NTag,
  NAlert,
  NIcon,
  useMessage
} from 'naive-ui'
import { CheckmarkOutline, RefreshOutline, LinkOutline } from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import { getSettings, updateSettings } from '../utils/api/setting'
import { getTmdbConfig, updateTmdbApiKey } from '../utils/api/media'
import { getEmbyStatus } from '../utils/api/emby'

const message = useMessage()

const activeTab = ref('basic')
const loading = ref(false)
const tmdbLoading = ref(false)
const initialForm = ref({})

const form = ref({
  alist_url: '',
  alist_token: '',
  log_save_day_limit: 1,
  proxy_url: '',
  proxy_domains: '',
  scrape_enabled_on_organize: true,
  scrape_write_nfo: true,
  scrape_write_poster: true,
  scrape_write_fanart: true,
  scrape_write_thumb: true,
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
const tmdbMaskedKey = ref('')

const tmdbLanguageOptions = [
  { label: '简体中文', value: 'zh-CN' },
  { label: '繁体中文', value: 'zh-TW' },
  { label: '英语', value: 'en' },
  { label: '日语', value: 'ja' },
  { label: '韩语', value: 'ko' }
]

const movieTemplateTags = ['{{ title }}', '{{ en_title }}', '{{ year }}', '{{ videoFormat }}', '{{ fileExt }}']
const tvTemplateTags = ['{{ title }}', '{{ en_title }}', '{{ season }}', '{{ episode }}', '{{ year }}', '{{ videoFormat }}', '{{ fileExt }}']

// --- Emby 配置 ---
const embyForm = ref({
  enabled: false,
  emby_url: '',
  emby_api_key: ''
})
const initialEmbyForm = ref({
  enabled: false,
  emby_url: '',
  emby_api_key: ''
})
const embyLoading = ref(false)
const embyTesting = ref(false)
const embyConnectionStatus = ref(null)
const embyServerInfo = ref(null)

const alistHelpVisible = computed(() => {
  return form.value.alist_url || form.value.alist_token
})

const fetchSettings = async () => {
  try {
    const response = await getSettings({ skipGlobalErrorMessage: true })
    const data = response.data.data || {}
    form.value = {
      alist_url: data.alist_url || '',
      alist_token: data.alist_token || '',
      log_save_day_limit: parseInt(data.log_save_day_limit, 10) || 1,
      proxy_url: data.proxy_url || '',
      proxy_domains: data.proxy_domains || '',
      scrape_enabled_on_organize: !(
        data.scrape_enabled_on_organize === 'false' ||
        data.scrape_enabled_on_organize === '0'
      ),
      scrape_write_nfo: !(
        data.scrape_write_nfo === 'false' || data.scrape_write_nfo === '0'
      ),
      scrape_write_poster: !(
        data.scrape_write_poster === 'false' || data.scrape_write_poster === '0'
      ),
      scrape_write_fanart: !(
        data.scrape_write_fanart === 'false' || data.scrape_write_fanart === '0'
      ),
      scrape_write_thumb: !(
        data.scrape_write_thumb === 'false' || data.scrape_write_thumb === '0'
      ),
      movie_naming_template: data.movie_naming_template || '',
      tv_naming_template: data.tv_naming_template || ''
    }
    initialForm.value = { ...form.value }

    // 加载 Emby 配置
    embyForm.value = {
      enabled: data.emby_enabled === 'true' || data.emby_enabled === '1',
      emby_url: data.emby_url || '',
      emby_api_key: data.emby_api_key || ''
    }
    initialEmbyForm.value = { ...embyForm.value }
  } catch (error) {
    console.error('获取系统配置失败:', error)
    if (error.response?.status === 404) {
      form.value = {
        alist_url: '',
        alist_token: '',
        log_save_day_limit: 1,
        proxy_url: '',
        proxy_domains: '',
        scrape_enabled_on_organize: true,
        scrape_write_nfo: true,
        scrape_write_poster: true,
        scrape_write_fanart: true,
        scrape_write_thumb: true,
        movie_naming_template: '',
        tv_naming_template: ''
      }
      initialForm.value = { ...form.value }
    }
  }
}

const fetchTmdbConfig = async () => {
  try {
    const response = await getTmdbConfig({ skipGlobalErrorMessage: true })
    const data = response.data.data || {}
    hasTmdbKey.value = data.has_key || false
    tmdbMaskedKey.value = data.api_key || ''
    tmdbForm.value = {
      api_key: '',
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
      scrape_enabled_on_organize: form.value.scrape_enabled_on_organize
        ? 'true'
        : 'false',
      scrape_write_nfo: form.value.scrape_write_nfo ? 'true' : 'false',
      scrape_write_poster: form.value.scrape_write_poster ? 'true' : 'false',
      scrape_write_fanart: form.value.scrape_write_fanart ? 'true' : 'false',
      scrape_write_thumb: form.value.scrape_write_thumb ? 'true' : 'false',
      movie_naming_template: form.value.movie_naming_template,
      tv_naming_template: form.value.tv_naming_template,
      emby_enabled: embyForm.value.enabled ? 'true' : 'false',
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key
    }
    await updateSettings(settings, { skipGlobalErrorMessage: true })
    message.success('配置保存成功')
    initialForm.value = { ...form.value }
  } catch (error) {
    console.error('保存配置失败:', error)
    message.error('保存配置失败')
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
  const apiKey = tmdbForm.value.api_key.trim()
  if (!apiKey && !hasTmdbKey.value) {
    message.warning('请输入 TMDB API Key')
    return
  }
  tmdbLoading.value = true
  try {
    await updateTmdbApiKey(
      {
        api_key: apiKey,
        language: tmdbForm.value.language
      },
      {
        skipGlobalErrorMessage: true
      }
    )
    message.success('TMDB 配置保存成功')
    await fetchTmdbConfig()
  } catch (error) {
    console.error('保存 TMDB 配置失败:', error)
    message.error('保存 TMDB 配置失败')
  } finally {
    tmdbLoading.value = false
  }
}

const handleTmdbReset = () => {
  tmdbForm.value = { ...initialTmdbForm.value }
}

// --- Emby 方法 ---
const testEmbyConnection = async () => {
  embyTesting.value = true
  embyConnectionStatus.value = null
  embyServerInfo.value = null
  try {
    // 先保存 Emby 配置,再测试连接
    const settings = {
      emby_enabled: embyForm.value.enabled ? 'true' : 'false',
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key
    }
    await updateSettings(settings, { skipGlobalErrorMessage: true })

    const response = await getEmbyStatus({ skipGlobalErrorMessage: true })
    const data = response.data.data || {}
    embyConnectionStatus.value = data.connected || false
    embyServerInfo.value = data.info || null
    if (data.connected) {
      message.success('Emby 连接成功')
    } else {
      message.error(`Emby 连接失败: ${data.error || '未知错误'}`)
    }
  } catch (error) {
    embyConnectionStatus.value = false
    message.error('Emby 连接测试失败')
  } finally {
    embyTesting.value = false
  }
}

const handleEmbySubmit = async () => {
  embyLoading.value = true
  try {
    const settings = {
      emby_enabled: embyForm.value.enabled ? 'true' : 'false',
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key
    }
    await updateSettings(settings, { skipGlobalErrorMessage: true })
    message.success('Emby 配置保存成功')
    initialEmbyForm.value = { ...embyForm.value }
  } catch (error) {
    console.error('保存 Emby 配置失败:', error)
    message.error('保存 Emby 配置失败')
  } finally {
    embyLoading.value = false
  }
}

const handleEmbyReset = () => {
  embyForm.value = { ...initialEmbyForm.value }
  embyConnectionStatus.value = null
  embyServerInfo.value = null
}

onMounted(() => {
  fetchSettings()
  fetchTmdbConfig()
})
</script>
