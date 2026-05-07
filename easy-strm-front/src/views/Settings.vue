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
        <el-divider content-position="left">Alist 配置</el-divider>

        <el-form-item label="Alist 服务地址">
          <el-input
            v-model="form.alist_url"
            placeholder="例如：http://192.168.1.100:5244"
            clearable
          />
          <div class="form-tip">
            用于 Alist 秒传场景，格式示例：`http://your-alist-server:port`。
          </div>
        </el-form-item>

        <el-form-item label="Alist 访问令牌">
          <el-input
            v-model="form.alist_token"
            type="password"
            placeholder="请输入 Alist 访问令牌"
            show-password
            clearable
          />
          <div class="form-tip">可在 Alist 设置页面获取 Token。</div>
        </el-form-item>

        <el-alert
          v-if="alistHelpVisible"
          type="info"
          :closable="false"
          show-icon
          class="alist-help"
        >
          <template #title>Alist 秒传配置说明</template>
          <ul class="alist-help-list">
            <li>确认 Alist 服务可被当前部署环境访问。</li>
            <li>确认 Alist 已开启直链相关能力。</li>
            <li>115 账号中选择 `alist` 秒传方式时会用到这里的配置。</li>
          </ul>
        </el-alert>

        <el-divider content-position="left">TMDB 配置</el-divider>

        <el-form-item label="TMDB API Key">
          <el-input
            v-model="tmdbForm.api_key"
            type="password"
            :placeholder="
              tmdbMaskedKey
                ? `当前已配置：${tmdbMaskedKey}，留空则保持不变`
                : '请输入 TMDB API Key'
            "
            show-password
            clearable
          />
          <div class="form-tip">
            <template v-if="hasTmdbKey">
              <el-tag type="success" size="small">已配置</el-tag>
              输入新的 API Key 将覆盖已有值。
            </template>
            <template v-else>
              可前往
              <el-link
                type="primary"
                href="https://www.themoviedb.org/settings/api"
                target="_blank"
                >TMDB API 设置页</el-link
              >
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
          <el-button
            type="primary"
            @click="handleTmdbSubmit"
            :loading="tmdbLoading"
          >
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
          <el-input-number
            v-model="form.log_save_day_limit"
            :min="1"
            :max="365"
            :step="1"
          />
          <div class="form-tip">日志文件保留天数，范围 1-365 天。</div>
        </el-form-item>

        <el-divider content-position="left">网络代理配置</el-divider>

        <el-form-item label="代理服务器地址">
          <el-input
            v-model="form.proxy_url"
            placeholder="例如：http://127.0.0.1:7890"
            clearable
          />
          <div class="form-tip">
            支持 `http://host:port` 或
            `socks5://host:port`，留空表示不启用代理。
          </div>
        </el-form-item>

        <el-form-item label="代理站点列表">
          <el-input
            v-model="form.proxy_domains"
            type="textarea"
            :rows="3"
            placeholder="支持逗号或换行，例如：tg,github"
          />
          <div class="form-tip">
            按域名匹配，可填 `tg`、`github` 或完整域名（如
            `api.telegram.org`）。
          </div>
        </el-form-item>

        <el-divider content-position="left">Emby 配置</el-divider>

        <el-form-item label="启用 Emby 集成">
          <el-switch
            v-model="embyForm.enabled"
            active-text="启用"
            inactive-text="关闭"
          />
          <div class="form-tip">启用后，整理完成时可自动刷新 Emby 媒体库。</div>
        </el-form-item>

        <el-form-item label="Emby 服务地址">
          <el-input
            v-model="embyForm.emby_url"
            placeholder="例如：http://emby:8096"
            clearable
            :disabled="!embyForm.enabled"
          />
          <div class="form-tip">Emby 服务器的完整访问地址，包含端口号。</div>
        </el-form-item>

        <el-form-item label="Emby API Key">
          <el-input
            v-model="embyForm.emby_api_key"
            type="password"
            placeholder="请输入 Emby API Key"
            show-password
            clearable
            :disabled="!embyForm.enabled"
          />
          <div class="form-tip">
            可在 Emby 后台「设置 → 高级 → API 密钥」中创建。
          </div>
        </el-form-item>

        <el-form-item v-if="embyForm.enabled && embyForm.emby_url">
          <el-button
            type="success"
            @click="testEmbyConnection"
            :loading="embyTesting"
          >
            <el-icon><Connection /></el-icon>
            测试连接
          </el-button>
          <el-tag
            v-if="embyConnectionStatus !== null"
            :type="embyConnectionStatus ? 'success' : 'danger'"
            style="margin-left: 12px"
          >
            {{ embyConnectionStatus ? "连接成功" : "连接失败" }}
          </el-tag>
          <span
            v-if="embyServerInfo"
            style="margin-left: 8px; color: #909399; font-size: 12px"
          >
            {{ embyServerInfo.ServerName }} (v{{ embyServerInfo.Version }})
          </span>
        </el-form-item>

        <el-form-item class="form-actions">
          <el-button
            type="primary"
            @click="handleEmbySubmit"
            :loading="embyLoading"
          >
            <el-icon><Check /></el-icon>
            保存 Emby 配置
          </el-button>
          <el-button @click="handleEmbyReset">
            <el-icon><RefreshRight /></el-icon>
            重置
          </el-button>
        </el-form-item>

        <el-divider content-position="left">刮削配置</el-divider>

        <el-form-item label="整理后同步刮削">
          <el-switch
            v-model="form.scrape_enabled_on_organize"
            active-text="开启"
            inactive-text="关闭"
          />
          <div class="form-tip">整理成功后自动对本地目标文件执行刮削。</div>
        </el-form-item>

        <el-form-item label="生成 NFO">
          <el-switch
            v-model="form.scrape_write_nfo"
            active-text="开启"
            inactive-text="关闭"
          />
          <div class="form-tip">关闭后只生成图片侧车，不写入 NFO 文件。</div>
        </el-form-item>

        <el-form-item label="下载海报">
          <el-switch
            v-model="form.scrape_write_poster"
            active-text="开启"
            inactive-text="关闭"
          />
        </el-form-item>

        <el-form-item label="下载背景图">
          <el-switch
            v-model="form.scrape_write_fanart"
            active-text="开启"
            inactive-text="关闭"
          />
        </el-form-item>

        <el-form-item label="下载剧照">
          <el-switch
            v-model="form.scrape_write_thumb"
            active-text="开启"
            inactive-text="关闭"
          />
          <div class="form-tip">剧照仅对剧集分集生效。</div>
        </el-form-item>

        <el-divider content-position="left">整理命名配置</el-divider>

        <el-form-item label="电影命名模板">
          <el-input
            v-model="form.movie_naming_template"
            placeholder="{{ title }}{% if year %} ({{ year }}){% endif %}/{{ title }}{% if year %} ({{ year }}){% endif %}{{ fileExt }}"
          />
          <div class="template-tags">
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('movie', '{{ title }}')"
              >&#123;&#123; title &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('movie', '{{ en_title }}')"
              >&#123;&#123; en_title &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('movie', '{{ year }}')"
              >&#123;&#123; year &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('movie', '{{ videoFormat }}')"
              >&#123;&#123; videoFormat &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('movie', '{{ fileExt }}')"
              >&#123;&#123; fileExt &#125;&#125;</el-tag
            >
          </div>
        </el-form-item>

        <el-form-item label="电视剧命名模板">
          <el-input
            v-model="form.tv_naming_template"
            placeholder="{{ title }}/Season {{ '%02d'|format(season|int) }}/{{ title }} - S{{ '%02d'|format(season|int) }}E{{ '%02d'|format(episode|int) }}{{ fileExt }}"
          />
          <div class="template-tags">
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ title }}')"
              >&#123;&#123; title &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ en_title }}')"
              >&#123;&#123; en_title &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ season }}')"
              >&#123;&#123; season &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ episode }}')"
              >&#123;&#123; episode &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ year }}')"
              >&#123;&#123; year &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ videoFormat }}')"
              >&#123;&#123; videoFormat &#125;&#125;</el-tag
            >
            <el-tag
              size="small"
              class="tag-item"
              @click="addTag('tv', '{{ fileExt }}')"
              >&#123;&#123; fileExt &#125;&#125;</el-tag
            >
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
import { ref, onMounted, computed } from "vue";
import {
  Setting,
  Check,
  RefreshRight,
  Connection,
} from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { getSettings, updateSettings } from "../utils/api";
import { getTmdbConfig, updateTmdbApiKey } from "../utils/api/media";
import { getEmbyStatus } from "../utils/api/emby";

const formRef = ref(null);
const loading = ref(false);
const tmdbLoading = ref(false);
const initialForm = ref({});

const form = ref({
  alist_url: "",
  alist_token: "",
  log_save_day_limit: 1,
  proxy_url: "",
  proxy_domains: "",
  scrape_enabled_on_organize: true,
  scrape_write_nfo: true,
  scrape_write_poster: true,
  scrape_write_fanart: true,
  scrape_write_thumb: true,
  movie_naming_template: "",
  tv_naming_template: "",
});

const tmdbForm = ref({
  api_key: "",
  language: "zh-CN",
});

const initialTmdbForm = ref({
  api_key: "",
  language: "zh-CN",
});

const hasTmdbKey = ref(false);
const tmdbMaskedKey = ref("");

// --- Emby 配置 ---
const embyForm = ref({
  enabled: false,
  emby_url: "",
  emby_api_key: "",
});
const initialEmbyForm = ref({
  enabled: false,
  emby_url: "",
  emby_api_key: "",
});
const embyLoading = ref(false);
const embyTesting = ref(false);
const embyConnectionStatus = ref(null);
const embyServerInfo = ref(null);

const alistHelpVisible = computed(() => {
  return form.value.alist_url || form.value.alist_token;
});

const fetchSettings = async () => {
  try {
    const response = await getSettings({ skipGlobalErrorMessage: true });
    const data = response.data.data || {};
    form.value = {
      alist_url: data.alist_url || "",
      alist_token: data.alist_token || "",
      log_save_day_limit: parseInt(data.log_save_day_limit, 10) || 1,
      proxy_url: data.proxy_url || "",
      proxy_domains: data.proxy_domains || "",
      scrape_enabled_on_organize: !(
        data.scrape_enabled_on_organize === "false" ||
        data.scrape_enabled_on_organize === "0"
      ),
      scrape_write_nfo: !(
        data.scrape_write_nfo === "false" || data.scrape_write_nfo === "0"
      ),
      scrape_write_poster: !(
        data.scrape_write_poster === "false" || data.scrape_write_poster === "0"
      ),
      scrape_write_fanart: !(
        data.scrape_write_fanart === "false" || data.scrape_write_fanart === "0"
      ),
      scrape_write_thumb: !(
        data.scrape_write_thumb === "false" || data.scrape_write_thumb === "0"
      ),
      movie_naming_template: data.movie_naming_template || "",
      tv_naming_template: data.tv_naming_template || "",
    };
    initialForm.value = { ...form.value };

    // 加载 Emby 配置
    embyForm.value = {
      enabled: data.emby_enabled === "true" || data.emby_enabled === "1",
      emby_url: data.emby_url || "",
      emby_api_key: data.emby_api_key || "",
    };
    initialEmbyForm.value = { ...embyForm.value };
  } catch (error) {
    console.error("获取系统配置失败:", error);
    if (error.response?.status === 404) {
      form.value = {
        alist_url: "",
        alist_token: "",
        log_save_day_limit: 1,
        proxy_url: "",
        proxy_domains: "",
        scrape_enabled_on_organize: true,
        scrape_write_nfo: true,
        scrape_write_poster: true,
        scrape_write_fanart: true,
        scrape_write_thumb: true,
        movie_naming_template: "",
        tv_naming_template: "",
      };
      initialForm.value = { ...form.value };
    }
  }
};

const fetchTmdbConfig = async () => {
  try {
    const response = await getTmdbConfig({ skipGlobalErrorMessage: true });
    const data = response.data.data || {};
    hasTmdbKey.value = data.has_key || false;
    tmdbMaskedKey.value = data.api_key || "";
    tmdbForm.value = {
      api_key: "",
      language: data.language || "zh-CN",
    };
    initialTmdbForm.value = { ...tmdbForm.value };
  } catch (error) {
    console.error("获取 TMDB 配置失败:", error);
  }
};

const handleSubmit = async () => {
  loading.value = true;
  try {
    const settings = {
      alist_url: form.value.alist_url,
      alist_token: form.value.alist_token,
      log_save_day_limit: String(form.value.log_save_day_limit),
      proxy_url: form.value.proxy_url,
      proxy_domains: form.value.proxy_domains,
      scrape_enabled_on_organize: form.value.scrape_enabled_on_organize
        ? "true"
        : "false",
      scrape_write_nfo: form.value.scrape_write_nfo ? "true" : "false",
      scrape_write_poster: form.value.scrape_write_poster ? "true" : "false",
      scrape_write_fanart: form.value.scrape_write_fanart ? "true" : "false",
      scrape_write_thumb: form.value.scrape_write_thumb ? "true" : "false",
      movie_naming_template: form.value.movie_naming_template,
      tv_naming_template: form.value.tv_naming_template,
      emby_enabled: embyForm.value.enabled ? "true" : "false",
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key,
    };
    await updateSettings(settings, { skipGlobalErrorMessage: true });
    ElMessage.success("配置保存成功");
    initialForm.value = { ...form.value };
  } catch (error) {
    console.error("保存配置失败:", error);
    ElMessage.error("保存配置失败");
  } finally {
    loading.value = false;
  }
};

const addTag = (type, tag) => {
  if (type === "movie") {
    form.value.movie_naming_template += tag;
  } else {
    form.value.tv_naming_template += tag;
  }
};

const handleReset = () => {
  form.value = { ...initialForm.value };
};

const handleTmdbSubmit = async () => {
  const apiKey = tmdbForm.value.api_key.trim();
  if (!apiKey && !hasTmdbKey.value) {
    ElMessage.warning("请输入 TMDB API Key");
    return;
  }
  tmdbLoading.value = true;
  try {
    await updateTmdbApiKey(
      {
        api_key: apiKey,
        language: tmdbForm.value.language,
      },
      {
        skipGlobalErrorMessage: true,
      },
    );
    ElMessage.success("TMDB 配置保存成功");
    await fetchTmdbConfig();
  } catch (error) {
    console.error("保存 TMDB 配置失败:", error);
    ElMessage.error("保存 TMDB 配置失败");
  } finally {
    tmdbLoading.value = false;
  }
};

const handleTmdbReset = () => {
  tmdbForm.value = { ...initialTmdbForm.value };
};

// --- Emby 方法 ---
const testEmbyConnection = async () => {
  embyTesting.value = true;
  embyConnectionStatus.value = null;
  embyServerInfo.value = null;
  try {
    // 先保存 Emby 配置，再测试连接
    const settings = {
      emby_enabled: embyForm.value.enabled ? "true" : "false",
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key,
    };
    await updateSettings(settings, { skipGlobalErrorMessage: true });

    const response = await getEmbyStatus({ skipGlobalErrorMessage: true });
    const data = response.data.data || {};
    embyConnectionStatus.value = data.connected || false;
    embyServerInfo.value = data.info || null;
    if (data.connected) {
      ElMessage.success("Emby 连接成功");
    } else {
      ElMessage.error(`Emby 连接失败: ${data.error || "未知错误"}`);
    }
  } catch (error) {
    embyConnectionStatus.value = false;
    ElMessage.error("Emby 连接测试失败");
  } finally {
    embyTesting.value = false;
  }
};

const handleEmbySubmit = async () => {
  embyLoading.value = true;
  try {
    const settings = {
      emby_enabled: embyForm.value.enabled ? "true" : "false",
      emby_url: embyForm.value.emby_url,
      emby_api_key: embyForm.value.emby_api_key,
    };
    await updateSettings(settings, { skipGlobalErrorMessage: true });
    ElMessage.success("Emby 配置保存成功");
    initialEmbyForm.value = { ...embyForm.value };
  } catch (error) {
    console.error("保存 Emby 配置失败:", error);
    ElMessage.error("保存 Emby 配置失败");
  } finally {
    embyLoading.value = false;
  }
};

const handleEmbyReset = () => {
  embyForm.value = { ...initialEmbyForm.value };
  embyConnectionStatus.value = null;
  embyServerInfo.value = null;
};

onMounted(() => {
  fetchSettings();
  fetchTmdbConfig();
});
</script>

<style scoped>
.settings-container {
  padding: 8px 0 0;
  min-height: calc(100vh - 100px);
}

.main-card {
  border-radius: 24px;
  overflow: hidden;
  max-width: 980px;
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
  display: flex;
  align-items: center;
  gap: 10px;
  color: white;
  font-size: 22px;
  font-weight: 700;
}

.header-icon {
  font-size: 24px;
}

.settings-form {
  padding: 12px 6px 8px;
}

.form-tip {
  font-size: 12px;
  color: #909399;
  margin-top: 4px;
  line-height: 1.4;
}

.alist-help {
  margin: 20px 8px;
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

:global(.dark) .main-card {
  background: rgba(14, 21, 32, 0.86);
  border-color: rgba(139, 163, 185, 0.12);
  box-shadow: 0 24px 60px rgba(0, 0, 0, 0.24);
}
</style>
