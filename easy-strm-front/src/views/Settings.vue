<template>
  <div class="space-y-4">
    <PageCard title="系统配置" subtitle="管理 TMDB、Emby、代理与整理刮削等全局设置">
      <n-tabs v-model:value="activeTab" type="line" animated>
        <!-- 基础设置：日志 / 代理 -->
        <n-tab-pane name="basic" tab="基础设置">
          <n-form :model="form" label-placement="top" class="max-w-2xl">
            <h3 class="mb-3 text-sm font-bold text-slate-800 dark:text-white">日志配置</h3>

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

        <!-- Telegram 通知 -->
        <n-tab-pane name="notification" tab="通知">
          <n-form :model="telegramForm" label-placement="top" class="max-w-2xl">
            <div class="mb-5 rounded-xl border border-slate-200 bg-slate-50 p-4 dark:border-white/5 dark:bg-white/5">
              <div class="flex flex-wrap items-center gap-2">
                <span class="text-sm font-bold text-slate-700 dark:text-slate-200">机器人状态</span>
                <n-tag :type="telegramStatus.running ? 'success' : 'default'" size="small" round>
                  {{ telegramStatus.running ? '运行中' : '未运行' }}
                </n-tag>
                <span v-if="telegramStatus.bot_username" class="text-sm text-slate-500 dark:text-slate-400">
                  @{{ telegramStatus.bot_username }}
                </span>
              </div>
              <n-alert
                v-if="telegramStatus.last_error"
                type="error"
                :show-icon="true"
                :closable="false"
                class="mt-3"
              >
                {{ telegramStatus.last_error }}
              </n-alert>
            </div>

            <n-form-item label="启用 Telegram 机器人">
              <n-switch v-model:value="telegramForm.enabled">
                <template #checked>启用</template>
                <template #unchecked>关闭</template>
              </n-switch>
            </n-form-item>

            <n-form-item label="Bot Token">
              <div class="w-full">
                <n-input
                  v-model:value="telegramForm.bot_token"
                  type="password"
                  show-password-on="click"
                  clearable
                  :placeholder="telegramForm.has_bot_token ? '已配置，留空则保持不变' : '请输入 BotFather 提供的 Bot Token'"
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  Token 不会通过配置读取接口返回；保存时留空会保留原值。
                </p>
              </div>
            </n-form-item>

            <n-form-item label="管理员私聊 Chat ID">
              <div class="w-full">
                <n-input
                  v-model:value="telegramForm.chat_id"
                  placeholder="例如：123456789"
                  clearable
                />
                <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
                  仅该私聊可以接收通知和执行机器人操作，不支持群组或多用户。
                </p>
              </div>
            </n-form-item>

            <h3 class="mb-3 mt-6 text-sm font-bold text-slate-800 dark:text-white">通知事件</h3>
            <div class="grid grid-cols-1 gap-3 sm:grid-cols-2">
              <div class="rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5">
                <span class="text-sm text-slate-700 dark:text-slate-200">任务成功</span>
                <n-switch v-model:value="telegramForm.notify_task_completed" class="float-right" />
              </div>
              <div class="rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5">
                <span class="text-sm text-slate-700 dark:text-slate-200">任务失败</span>
                <n-switch v-model:value="telegramForm.notify_task_failed" class="float-right" />
              </div>
              <div class="rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5">
                <span class="text-sm text-slate-700 dark:text-slate-200">任务取消</span>
                <n-switch v-model:value="telegramForm.notify_task_cancelled" class="float-right" />
              </div>
              <div class="rounded-xl bg-slate-50 px-4 py-3 dark:bg-white/5">
                <span class="text-sm text-slate-700 dark:text-slate-200">115 账号状态</span>
                <n-switch v-model:value="telegramForm.notify_account_status" class="float-right" />
              </div>
            </div>

            <div class="mt-6 flex flex-wrap gap-2">
              <n-button type="primary" :loading="telegramLoading" @click="handleTelegramSubmit">
                <template #icon><n-icon :component="CheckmarkOutline" /></template>
                保存并应用
              </n-button>
              <n-button type="success" :loading="telegramTesting" @click="handleTelegramTest">
                <template #icon><n-icon :component="PaperPlaneOutline" /></template>
                发送测试卡片
              </n-button>
              <n-button :loading="telegramStatusLoading" @click="fetchTelegramStatus">
                <template #icon><n-icon :component="RefreshOutline" /></template>
                刷新状态
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
import { ref, onMounted } from 'vue'
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
import { CheckmarkOutline, RefreshOutline, LinkOutline, PaperPlaneOutline } from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import { getSettings, updateSettings } from '../utils/api/setting'
import { getTmdbConfig, updateTmdbApiKey } from '../utils/api/media'
import { getEmbyStatus } from '../utils/api/emby'
import {
  getTelegramConfig,
  updateTelegramConfig,
  getTelegramStatus,
  testTelegram
} from '../utils/api/notification'

const message = useMessage()

const activeTab = ref('basic')
const loading = ref(false)
const tmdbLoading = ref(false)
const initialForm = ref({})

const form = ref({
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

// --- Telegram 配置 ---
const telegramForm = ref({
  enabled: false,
  bot_token: '',
  chat_id: '',
  has_bot_token: false,
  notify_task_completed: true,
  notify_task_failed: true,
  notify_task_cancelled: true,
  notify_account_status: true
})
const telegramStatus = ref({
  enabled: false,
  running: false,
  bot_username: '',
  chat_id: '',
  last_update_time: '',
  last_error: ''
})
const telegramLoading = ref(false)
const telegramTesting = ref(false)
const telegramStatusLoading = ref(false)

const fetchTelegramConfig = async () => {
  try {
    const response = await getTelegramConfig({ skipGlobalErrorMessage: true })
    const data = response.data.data || {}
    telegramForm.value = {
      enabled: data.enabled || false,
      bot_token: '',
      chat_id: data.chat_id || '',
      has_bot_token: data.has_bot_token || false,
      notify_task_completed: data.notify_task_completed !== false,
      notify_task_failed: data.notify_task_failed !== false,
      notify_task_cancelled: data.notify_task_cancelled !== false,
      notify_account_status: data.notify_account_status !== false
    }
  } catch (error) {
    console.error('获取 Telegram 配置失败:', error)
  }
}

const fetchTelegramStatus = async () => {
  telegramStatusLoading.value = true
  try {
    const response = await getTelegramStatus({ skipGlobalErrorMessage: true })
    telegramStatus.value = response.data.data || telegramStatus.value
  } catch (error) {
    console.error('获取 Telegram 状态失败:', error)
  } finally {
    telegramStatusLoading.value = false
  }
}

const fetchSettings = async () => {
  try {
    const response = await getSettings({ skipGlobalErrorMessage: true })
    const data = response.data.data || {}
    form.value = {
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

const handleTelegramSubmit = async () => {
  if (telegramForm.value.enabled) {
    if (!telegramForm.value.bot_token.trim() && !telegramForm.value.has_bot_token) {
      message.warning('请输入 Bot Token')
      return
    }
    if (!telegramForm.value.chat_id.trim()) {
      message.warning('请输入管理员私聊 Chat ID')
      return
    }
  }
  telegramLoading.value = true
  try {
    await updateTelegramConfig(
      {
        enabled: telegramForm.value.enabled,
        bot_token: telegramForm.value.bot_token.trim(),
        chat_id: telegramForm.value.chat_id.trim(),
        notify_task_completed: telegramForm.value.notify_task_completed,
        notify_task_failed: telegramForm.value.notify_task_failed,
        notify_task_cancelled: telegramForm.value.notify_task_cancelled,
        notify_account_status: telegramForm.value.notify_account_status
      },
      { skipGlobalErrorMessage: true }
    )
    message.success('Telegram 配置已保存并应用')
    await Promise.all([fetchTelegramConfig(), fetchTelegramStatus()])
  } catch (error) {
    message.error(error.response?.data?.error || '保存 Telegram 配置失败')
  } finally {
    telegramLoading.value = false
  }
}

const handleTelegramTest = async () => {
  telegramTesting.value = true
  try {
    await testTelegram({ skipGlobalErrorMessage: true })
    message.success('Telegram 测试卡片发送成功')
    await fetchTelegramStatus()
  } catch (error) {
    message.error(error.response?.data?.error || 'Telegram 测试失败')
  } finally {
    telegramTesting.value = false
  }
}

onMounted(() => {
  fetchSettings()
  fetchTmdbConfig()
  fetchTelegramConfig()
  fetchTelegramStatus()
})
</script>
