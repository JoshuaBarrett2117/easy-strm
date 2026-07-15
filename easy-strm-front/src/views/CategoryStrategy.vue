<template>
  <div class="space-y-4">
    <!-- 页头 -->
    <div class="rounded-2xl border border-slate-200 bg-white p-5 shadow-sm dark:border-white/5 dark:bg-ink-900 lg:p-6">
      <div class="flex flex-wrap items-end justify-between gap-4">
        <div>
          <div class="flex items-center gap-1.5 text-xs font-bold uppercase tracking-[0.16em] text-cyan-600 dark:text-cyan-400">
            <n-icon :component="PricetagsOutline" />
            自动归类策略
          </div>
          <h2 class="mt-1.5 text-2xl font-extrabold text-slate-800 dark:text-white">分类策略</h2>
          <p class="mt-1 text-xs text-slate-400 dark:text-slate-500">
            按内容类型、语种、国家/地区和年份匹配媒体，未命中内置规则时归入未分类。
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-2">
          <n-button :loading="loading" @click="fetchCategories">
            <template #icon>
              <n-icon :component="RefreshOutline" />
            </template>
            刷新
          </n-button>
          <n-button type="primary" @click="handleAdd">
            <template #icon>
              <n-icon :component="AddOutline" />
            </template>
            新增分类
          </n-button>
        </div>
      </div>
    </div>

    <!-- 主体:策略列表 + 策略编辑 -->
    <div class="grid grid-cols-1 items-start gap-4 lg:grid-cols-[340px_minmax(0,1fr)]">
      <!-- 侧栏:媒体类型切换 + 策略列表 -->
      <PageCard>
        <n-tabs v-model:value="activeMediaType" type="segment" @update:value="handleTabChange">
          <n-tab-pane name="movie">
            <template #tab>
              <span class="inline-flex items-center gap-1.5">
                <n-icon :component="FilmOutline" />电影 (MOVIE)
              </span>
            </template>
          </n-tab-pane>
          <n-tab-pane name="tv">
            <template #tab>
              <span class="inline-flex items-center gap-1.5">
                <n-icon :component="TvOutline" />电视剧 (TV)
              </span>
            </template>
          </n-tab-pane>
        </n-tabs>

        <div class="my-3 grid grid-cols-2 gap-3">
          <div class="rounded-xl bg-slate-50 p-3.5 dark:bg-ink-800">
            <span class="block text-2xl font-extrabold tabular-nums text-cyan-600 dark:text-cyan-400">{{ currentCategories.length }}</span>
            <small class="text-xs text-slate-400 dark:text-slate-500">当前策略</small>
          </div>
          <div class="rounded-xl bg-slate-50 p-3.5 dark:bg-ink-800">
            <span class="block text-2xl font-extrabold tabular-nums text-emerald-600 dark:text-emerald-400">{{ enabledCount }}</span>
            <small class="text-xs text-slate-400 dark:text-slate-500">启用中</small>
          </div>
        </div>

        <div class="flex max-h-[420px] flex-col gap-2 overflow-y-auto lg:max-h-[calc(100vh-380px)] lg:min-h-[360px]">
          <button
            v-for="category in currentCategories"
            :key="category.id"
            type="button"
            class="flex w-full items-center gap-3 rounded-xl border p-3 text-left transition-colors"
            :class="selectedCategory?.id === category.id
              ? 'border-cyan-500/60 bg-cyan-500/10'
              : 'border-slate-200 bg-white hover:border-cyan-500/40 dark:border-white/10 dark:bg-ink-800'"
            @click="selectCategory(category)"
          >
            <span class="flex h-9 w-9 shrink-0 items-center justify-center rounded-xl bg-cyan-500/10 text-cyan-600 dark:text-cyan-400">
              <n-icon :component="PricetagOutline" />
            </span>
            <span class="grid min-w-0 flex-1 gap-0.5">
              <strong class="truncate text-sm font-bold text-slate-800 dark:text-white">{{ category.name }}</strong>
              <small class="truncate text-xs text-slate-400 dark:text-slate-500">{{ getRuleSummary(category) }}</small>
            </span>
            <n-tag v-if="getRules(category).default" type="info" size="small">兜底</n-tag>
            <n-tag v-else-if="category.enabled" type="success" size="small">启用</n-tag>
            <n-tag v-else type="error" size="small">停用</n-tag>
          </button>
          <EmptyState v-if="!loading && currentCategories.length === 0" title="暂无分类策略" />
        </div>
      </PageCard>

      <!-- 策略编辑 -->
      <PageCard class="lg:min-h-[620px]">
        <n-spin :show="loading">
          <div v-if="selectedCategory" class="space-y-5">
            <div class="flex flex-wrap items-start justify-between gap-4 border-b border-slate-100 pb-4 dark:border-white/5">
              <div>
                <span class="text-xs font-medium text-slate-400 dark:text-slate-500">策略详情</span>
                <h3 class="mt-1 text-lg font-bold text-slate-800 dark:text-white">
                  {{ form.id ? '编辑分类策略' : '新增分类策略' }}
                </h3>
              </div>
              <div class="flex flex-wrap items-center gap-3">
                <div class="flex items-center gap-2">
                  <n-switch v-model:value="form.enabled" />
                  <span class="text-xs text-slate-500 dark:text-slate-400">{{ form.enabled ? '启用' : '停用' }}</span>
                </div>
                <n-button v-if="form.id" type="error" ghost @click="handleDelete">
                  <template #icon>
                    <n-icon :component="TrashOutline" />
                  </template>
                  删除
                </n-button>
              </div>
            </div>

            <n-form ref="formRef" :model="form" :rules="rules" label-placement="top">
              <div class="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
                <n-form-item label="分类名称（目录名）" path="name">
                  <n-input v-model:value="form.name" placeholder="例如：国漫" />
                </n-form-item>
                <n-form-item label="目标目录" path="target_path">
                  <n-input v-model:value="form.target_path" placeholder="/电视剧/国漫" />
                </n-form-item>
              </div>

              <div class="rounded-2xl border border-slate-200 bg-slate-50 p-4 dark:border-white/5 dark:bg-ink-800 lg:p-5">
                <div class="mb-4 flex items-center gap-2 text-sm font-bold text-slate-600 dark:text-slate-300">
                  <n-icon :component="GridOutline" />
                  匹配条件
                </div>
                <div class="grid grid-cols-1 gap-x-4 sm:grid-cols-2">
                  <n-form-item label="内容类型（Genre）">
                    <n-select
                      v-model:value="form.match_rules.genre_ids"
                      multiple
                      filterable
                      clearable
                      placeholder="内容类型（Genre）"
                      :options="genreOptions"
                    />
                  </n-form-item>
                  <n-form-item label="国家/地区（Country）">
                    <n-select
                      v-model:value="form.match_rules.countries"
                      multiple
                      filterable
                      clearable
                      placeholder="国家/地区（Country）"
                      :options="countryOptions"
                    />
                  </n-form-item>
                  <n-form-item label="语种（Language）">
                    <n-select
                      v-model:value="form.match_rules.languages"
                      multiple
                      filterable
                      clearable
                      placeholder="语种（Language）"
                      :options="languageOptions"
                    />
                  </n-form-item>
                  <n-form-item label="年份（Year）">
                    <n-select
                      v-model:value="form.match_rules.years"
                      multiple
                      filterable
                      tag
                      clearable
                      placeholder="输入年份后回车"
                      :options="yearSelectOptions"
                    />
                  </n-form-item>
                </div>
                <n-form-item label="标题关键字（可选）">
                  <n-select
                    v-model:value="form.match_rules.keywords"
                    multiple
                    filterable
                    tag
                    clearable
                    placeholder="输入关键字后回车"
                    :options="keywordSelectOptions"
                  />
                </n-form-item>
                <n-checkbox v-model:checked="form.match_rules.default">作为未命中兜底分类</n-checkbox>
              </div>
            </n-form>

            <div class="flex flex-wrap items-center justify-end gap-2 border-t border-slate-100 pt-4 dark:border-white/5">
              <n-button @click="resetSelection">取消</n-button>
              <n-button type="primary" :loading="saving" @click="handleSave">
                <template #icon>
                  <n-icon :component="CheckmarkDoneOutline" />
                </template>
                保存
              </n-button>
            </div>
          </div>
          <EmptyState v-else title="请选择或新增一个分类策略" />
        </n-spin>
      </PageCard>
    </div>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import {
  NButton,
  NCheckbox,
  NForm,
  NFormItem,
  NIcon,
  NInput,
  NSelect,
  NSpin,
  NSwitch,
  NTabPane,
  NTabs,
  NTag,
  useMessage
} from 'naive-ui'
import {
  AddOutline,
  CheckmarkDoneOutline,
  FilmOutline,
  GridOutline,
  PricetagOutline,
  PricetagsOutline,
  RefreshOutline,
  TrashOutline,
  TvOutline
} from '@vicons/ionicons5'
import PageCard from '../components/common/PageCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import { showConfirmDialog } from '../utils/ui/messageBox'
import {
  createMediaCategory,
  deleteMediaCategory,
  getMediaCategories,
  updateMediaCategory
} from '../utils/api/media'

const message = useMessage()

const loading = ref(false)
const saving = ref(false)
const categories = ref([])
const activeMediaType = ref('movie')
const selectedCategory = ref(null)
const formRef = ref(null)

const createEmptyForm = (mediaType = activeMediaType.value) => ({
  id: null,
  name: '',
  media_type: mediaType,
  target_path: mediaType === 'movie' ? '/电影/未分类' : '/电视剧/未分类',
  enabled: true,
  match_rules: {
    genre_ids: [],
    countries: [],
    languages: [],
    years: [],
    keywords: [],
    default: false
  }
})

const form = ref(createEmptyForm())

const genreOptions = [
  { label: '动画 (Animation)', value: 16 },
  { label: '纪录 (Documentary)', value: 99 },
  { label: '儿童 (Kids)', value: 10762 },
  { label: '真人秀 (Reality)', value: 10764 },
  { label: '访谈 (Talk)', value: 10767 },
  { label: '动作冒险 (Action & Adventure)', value: 10759 },
  { label: '剧情 (Drama)', value: 18 },
  { label: '喜剧 (Comedy)', value: 35 },
  { label: '犯罪 (Crime)', value: 80 },
  { label: '科幻奇幻 (Sci-Fi & Fantasy)', value: 10765 }
]

const countryOptions = [
  { label: '中国大陆 (CN)', value: 'CN' },
  { label: '中国台湾 (TW)', value: 'TW' },
  { label: '中国香港 (HK)', value: 'HK' },
  { label: '日本 (JP)', value: 'JP' },
  { label: '韩国 (KR)', value: 'KR' },
  { label: '朝鲜 (KP)', value: 'KP' },
  { label: '泰国 (TH)', value: 'TH' },
  { label: '印度 (IN)', value: 'IN' },
  { label: '新加坡 (SG)', value: 'SG' },
  { label: '美国 (US)', value: 'US' },
  { label: '英国 (GB)', value: 'GB' },
  { label: '英国 (UK)', value: 'UK' },
  { label: '法国 (FR)', value: 'FR' },
  { label: '德国 (DE)', value: 'DE' },
  { label: '西班牙 (ES)', value: 'ES' },
  { label: '意大利 (IT)', value: 'IT' },
  { label: '荷兰 (NL)', value: 'NL' },
  { label: '葡萄牙 (PT)', value: 'PT' },
  { label: '俄罗斯 (RU)', value: 'RU' }
]

const languageOptions = [
  { label: '中文 (zh)', value: 'zh' },
  { label: '中文 (cn)', value: 'cn' },
  { label: '英语 (en)', value: 'en' },
  { label: '日语 (ja)', value: 'ja' },
  { label: '韩语 (ko)', value: 'ko' },
  { label: '法语 (fr)', value: 'fr' },
  { label: '德语 (de)', value: 'de' },
  { label: '西班牙语 (es)', value: 'es' },
  { label: '意大利语 (it)', value: 'it' },
  { label: '俄语 (ru)', value: 'ru' },
  { label: '葡萄牙语 (pt)', value: 'pt' },
  { label: '泰语 (th)', value: 'th' },
  { label: '印地语 (hi)', value: 'hi' }
]

const keywordOptions = ['动漫', 'Anime', '韩剧', '日剧', 'Mandarin', 'Cantonese']
const keywordSelectOptions = keywordOptions.map(keyword => ({ label: keyword, value: keyword }))

const currentYear = new Date().getFullYear()
const yearOptions = Array.from({ length: 30 }, (_, index) => currentYear - index)
const yearSelectOptions = yearOptions.map(year => ({ label: String(year), value: year }))

const rules = {
  name: [{ required: true, message: '请输入分类名称', trigger: 'blur' }],
  target_path: [{ required: true, message: '请输入目标目录', trigger: 'blur' }]
}

const currentCategories = computed(() => {
  return categories.value.filter(item => item.media_type === activeMediaType.value)
})

const enabledCount = computed(() => currentCategories.value.filter(item => item.enabled).length)

const parseRules = (rulesData) => {
  if (!rulesData) {
    return {}
  }
  if (typeof rulesData === 'string') {
    try {
      return JSON.parse(rulesData)
    } catch {
      return {}
    }
  }
  return rulesData
}

const normalizeRules = (rulesData) => {
  const parsed = parseRules(rulesData)
  return {
    genre_ids: parsed.genre_ids || [],
    countries: parsed.countries || [],
    languages: parsed.languages || [],
    years: parsed.years || [],
    keywords: parsed.keywords || [],
    default: Boolean(parsed.default)
  }
}

const getRules = (category) => normalizeRules(category.match_rules)

const getRuleSummary = (category) => {
  const rule = getRules(category)
  if (rule.default) {
    return '未命中其他策略时归入此目录'
  }
  const parts = []
  if (rule.genre_ids.length) parts.push(`${rule.genre_ids.length} 个类型`)
  if (rule.countries.length) parts.push(`${rule.countries.length} 个地区`)
  if (rule.languages.length) parts.push(`${rule.languages.length} 个语种`)
  if (rule.years.length) parts.push(`${rule.years.length} 个年份`)
  if (rule.keywords.length) parts.push(`${rule.keywords.length} 个关键字`)
  return parts.length ? parts.join(' · ') : '未设置匹配条件'
}

const toForm = (category) => ({
  id: category.id,
  name: category.name,
  media_type: category.media_type,
  target_path: category.target_path,
  enabled: category.enabled,
  match_rules: normalizeRules(category.match_rules)
})

const selectCategory = (category) => {
  selectedCategory.value = category
  form.value = toForm(category)
}

const resetSelection = () => {
  if (selectedCategory.value?.id) {
    form.value = toForm(selectedCategory.value)
  } else {
    selectedCategory.value = currentCategories.value[0] || null
    form.value = selectedCategory.value ? toForm(selectedCategory.value) : createEmptyForm()
  }
}

const handleTabChange = () => {
  const first = currentCategories.value[0] || null
  selectedCategory.value = first
  form.value = first ? toForm(first) : createEmptyForm(activeMediaType.value)
}

const handleAdd = () => {
  const draft = createEmptyForm(activeMediaType.value)
  selectedCategory.value = draft
  form.value = draft
}

const buildPayload = () => ({
  name: form.value.name.trim(),
  media_type: form.value.media_type,
  target_path: form.value.target_path.trim(),
  enabled: form.value.enabled,
  match_rules: {
    genre_ids: form.value.match_rules.genre_ids.map(Number).filter(Number.isFinite),
    countries: form.value.match_rules.countries.map(item => String(item).toUpperCase()),
    languages: form.value.match_rules.languages.map(item => String(item).toLowerCase()),
    years: form.value.match_rules.years.map(Number).filter(Number.isFinite),
    keywords: form.value.match_rules.keywords.map(item => String(item).trim()).filter(Boolean),
    default: Boolean(form.value.match_rules.default)
  }
})

const fetchCategories = async () => {
  loading.value = true
  try {
    const response = await getMediaCategories()
    const payload = response.data.data || {}
    categories.value = payload.data || []
    const current = selectedCategory.value?.id
      ? categories.value.find(item => item.id === selectedCategory.value.id)
      : currentCategories.value[0]
    selectedCategory.value = current || currentCategories.value[0] || null
    form.value = selectedCategory.value ? toForm(selectedCategory.value) : createEmptyForm(activeMediaType.value)
  } catch (error) {
    console.error('[CategoryStrategy] 获取分类策略失败:', error)
    message.error('获取分类策略失败')
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  await formRef.value?.validate()
  saving.value = true
  try {
    const payload = buildPayload()
    if (form.value.id) {
      await updateMediaCategory(form.value.id, payload)
      message.success('分类策略已更新')
    } else {
      await createMediaCategory(payload)
      message.success('分类策略已创建')
    }
    await fetchCategories()
  } catch (error) {
    console.error('[CategoryStrategy] 保存分类策略失败:', error)
    message.error('保存分类策略失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = () => {
  if (!form.value.id) {
    resetSelection()
    return
  }

  showConfirmDialog(`确定删除分类策略「${form.value.name}」吗？`, '删除确认', {
    confirmButtonText: '删除',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(async () => {
    await deleteMediaCategory(form.value.id)
    message.success('分类策略已删除')
    selectedCategory.value = null
    await fetchCategories()
  }).catch(() => {})
}

onMounted(fetchCategories)
</script>
