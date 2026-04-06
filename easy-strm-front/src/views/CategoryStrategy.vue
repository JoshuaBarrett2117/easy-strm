<template>
  <div class="category-page">
    <section class="category-hero">
      <div>
        <div class="eyebrow">
          <el-icon><CollectionTag /></el-icon>
          自动归类策略
        </div>
        <h2>分类策略</h2>
        <p>按内容类型、语种、国家/地区和年份匹配媒体，未命中内置规则时归入未分类。</p>
      </div>
      <div class="hero-actions">
        <el-button :icon="Refresh" @click="fetchCategories" :loading="loading">刷新</el-button>
        <el-button type="primary" :icon="Plus" @click="handleAdd">新增分类</el-button>
      </div>
    </section>

    <section class="strategy-shell">
      <aside class="strategy-sidebar">
        <el-tabs v-model="activeMediaType" stretch @tab-change="handleTabChange">
          <el-tab-pane label="电影 (MOVIE)" name="movie">
            <template #label>
              <span class="tab-label"><el-icon><Film /></el-icon>电影 (MOVIE)</span>
            </template>
          </el-tab-pane>
          <el-tab-pane label="电视剧 (TV)" name="tv">
            <template #label>
              <span class="tab-label"><el-icon><Monitor /></el-icon>电视剧 (TV)</span>
            </template>
          </el-tab-pane>
        </el-tabs>

        <div class="summary-grid">
          <div class="summary-item">
            <span>{{ currentCategories.length }}</span>
            <small>当前策略</small>
          </div>
          <div class="summary-item">
            <span>{{ enabledCount }}</span>
            <small>启用中</small>
          </div>
        </div>

        <el-scrollbar class="strategy-list">
          <button
            v-for="category in currentCategories"
            :key="category.id"
            class="strategy-item"
            :class="{ active: selectedCategory?.id === category.id }"
            @click="selectCategory(category)"
          >
            <span class="strategy-icon"><el-icon><PriceTag /></el-icon></span>
            <span class="strategy-copy">
              <strong>{{ category.name }}</strong>
              <small>{{ getRuleSummary(category) }}</small>
            </span>
            <el-tag v-if="getRules(category).default" type="info" size="small">兜底</el-tag>
            <el-tag v-else-if="category.enabled" type="success" size="small">启用</el-tag>
            <el-tag v-else type="danger" size="small">停用</el-tag>
          </button>
          <el-empty v-if="!loading && currentCategories.length === 0" description="暂无分类策略" />
        </el-scrollbar>
      </aside>

      <main class="strategy-editor" v-loading="loading">
        <div v-if="selectedCategory" class="editor-inner">
          <div class="editor-header">
            <div>
              <span class="editor-kicker">策略详情</span>
              <h3>{{ form.id ? '编辑分类策略' : '新增分类策略' }}</h3>
            </div>
            <div class="editor-actions">
              <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
              <el-button v-if="form.id" type="danger" plain :icon="Delete" @click="handleDelete">删除</el-button>
            </div>
          </div>

          <el-form ref="formRef" :model="form" :rules="rules" label-position="top" class="category-form">
            <div class="form-row">
              <el-form-item label="分类名称（目录名）" prop="name">
                <el-input v-model="form.name" placeholder="例如：国漫" />
              </el-form-item>
              <el-form-item label="目标目录" prop="target_path">
                <el-input v-model="form.target_path" placeholder="/电视剧/国漫" />
              </el-form-item>
            </div>

            <div class="rule-panel">
              <div class="rule-panel-title">
                <el-icon><Grid /></el-icon>
                匹配条件
              </div>
              <div class="form-row">
                <el-form-item label="内容类型（Genre）">
                  <el-select v-model="form.match_rules.genre_ids" multiple filterable clearable placeholder="内容类型（Genre）">
                    <el-option
                      v-for="genre in genreOptions"
                      :key="genre.value"
                      :label="genre.label"
                      :value="genre.value"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="国家/地区（Country）">
                  <el-select v-model="form.match_rules.countries" multiple filterable clearable placeholder="国家/地区（Country）">
                    <el-option
                      v-for="country in countryOptions"
                      :key="country.value"
                      :label="country.label"
                      :value="country.value"
                    />
                  </el-select>
                </el-form-item>
              </div>
              <div class="form-row">
                <el-form-item label="语种（Language）">
                  <el-select v-model="form.match_rules.languages" multiple filterable clearable placeholder="语种（Language）">
                    <el-option
                      v-for="language in languageOptions"
                      :key="language.value"
                      :label="language.label"
                      :value="language.value"
                    />
                  </el-select>
                </el-form-item>
                <el-form-item label="年份（Year）">
                  <el-select v-model="form.match_rules.years" multiple filterable allow-create default-first-option clearable placeholder="输入年份后回车">
                    <el-option
                      v-for="year in yearOptions"
                      :key="year"
                      :label="String(year)"
                      :value="year"
                    />
                  </el-select>
                </el-form-item>
              </div>
              <el-form-item label="标题关键字（可选）">
                <el-select v-model="form.match_rules.keywords" multiple filterable allow-create default-first-option clearable placeholder="输入关键字后回车">
                  <el-option
                    v-for="keyword in keywordOptions"
                    :key="keyword"
                    :label="keyword"
                    :value="keyword"
                  />
                </el-select>
              </el-form-item>
              <el-checkbox v-model="form.match_rules.default">作为未命中兜底分类</el-checkbox>
            </div>
          </el-form>

          <div class="editor-footer">
            <el-button @click="resetSelection">取消</el-button>
            <el-button type="primary" :icon="Finished" :loading="saving" @click="handleSave">保存</el-button>
          </div>
        </div>
        <el-empty v-else description="请选择或新增一个分类策略" />
      </main>
    </section>
  </div>
</template>

<script setup>
import { computed, onMounted, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  CollectionTag,
  Delete,
  Film,
  Finished,
  Grid,
  Monitor,
  Plus,
  PriceTag,
  Refresh
} from '@element-plus/icons-vue'
import {
  createMediaCategory,
  deleteMediaCategory,
  getMediaCategories,
  updateMediaCategory
} from '../utils/api/media'

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
const currentYear = new Date().getFullYear()
const yearOptions = Array.from({ length: 30 }, (_, index) => currentYear - index)

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
    ElMessage.error('获取分类策略失败')
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
      ElMessage.success('分类策略已更新')
    } else {
      await createMediaCategory(payload)
      ElMessage.success('分类策略已创建')
    }
    await fetchCategories()
  } catch (error) {
    console.error('[CategoryStrategy] 保存分类策略失败:', error)
    ElMessage.error('保存分类策略失败')
  } finally {
    saving.value = false
  }
}

const handleDelete = () => {
  if (!form.value.id) {
    resetSelection()
    return
  }

  ElMessageBox.confirm(`确定删除分类策略「${form.value.name}」吗？`, '删除确认', {
    confirmButtonText: '删除',
    cancelButtonText: '取消',
    type: 'warning'
  }).then(async () => {
    await deleteMediaCategory(form.value.id)
    ElMessage.success('分类策略已删除')
    selectedCategory.value = null
    await fetchCategories()
  }).catch(() => {})
}

onMounted(fetchCategories)
</script>

<style scoped>
.category-page {
  min-height: calc(100vh - 60px);
  padding: 24px;
  color: #1f2937;
}

.category-hero {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 24px;
  padding: 28px 30px;
  border-radius: 22px;
  background: linear-gradient(135deg, #ffffff 0%, #f4f1ff 55%, #ede7ff 100%);
  box-shadow: 0 14px 40px rgba(102, 126, 234, 0.12);
  margin-bottom: 20px;
}

.eyebrow,
.tab-label,
.rule-panel-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
}

.eyebrow {
  color: #6d5dfc;
  font-weight: 700;
  font-size: 13px;
  margin-bottom: 8px;
}

.category-hero h2 {
  font-size: 30px;
  line-height: 1.2;
  margin: 0 0 8px;
}

.category-hero p {
  margin: 0;
  color: #6b7280;
}

.hero-actions,
.editor-actions,
.editor-footer {
  display: flex;
  align-items: center;
  gap: 12px;
}

.strategy-shell {
  display: grid;
  grid-template-columns: 360px minmax(0, 1fr);
  gap: 20px;
}

.strategy-sidebar,
.strategy-editor {
  background: #fff;
  border-radius: 18px;
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.08);
}

.strategy-sidebar {
  padding: 18px;
}

.summary-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 12px;
  margin: 14px 0;
}

.summary-item {
  padding: 14px;
  border-radius: 14px;
  background: #f8f7ff;
}

.summary-item span {
  display: block;
  font-size: 24px;
  font-weight: 800;
  color: #6d5dfc;
}

.summary-item small,
.strategy-copy small,
.editor-kicker {
  color: #7c8494;
}

.strategy-list {
  height: calc(100vh - 330px);
  min-height: 360px;
}

.strategy-item {
  width: 100%;
  border: 0;
  background: transparent;
  border-radius: 14px;
  padding: 14px;
  display: flex;
  align-items: center;
  gap: 12px;
  cursor: pointer;
  text-align: left;
  transition: background 0.2s ease, transform 0.2s ease;
}

.strategy-item:hover,
.strategy-item.active {
  background: #f3f0ff;
  transform: translateY(-1px);
}

.strategy-icon {
  width: 34px;
  height: 34px;
  border-radius: 12px;
  display: grid;
  place-items: center;
  background: #eee9ff;
  color: #6d5dfc;
  flex: 0 0 auto;
}

.strategy-copy {
  display: grid;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.strategy-copy strong,
.strategy-copy small {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.strategy-editor {
  min-height: 620px;
  padding: 28px;
}

.editor-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 20px;
  padding-bottom: 20px;
  border-bottom: 1px solid #edf0f7;
  margin-bottom: 22px;
}

.editor-header h3 {
  margin: 4px 0 0;
  font-size: 22px;
}

.category-form :deep(.el-select) {
  width: 100%;
}

.form-row {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 18px;
}

.rule-panel {
  border-radius: 18px;
  background: #f8f9fc;
  padding: 20px;
  border: 1px solid #edf0f7;
}

.rule-panel-title {
  font-weight: 700;
  color: #4b5563;
  margin-bottom: 16px;
}

.editor-footer {
  justify-content: flex-end;
  padding-top: 22px;
}

@media (max-width: 1100px) {
  .strategy-shell {
    grid-template-columns: 1fr;
  }

  .strategy-list {
    height: auto;
    max-height: 420px;
  }
}

@media (max-width: 720px) {
  .category-page {
    padding: 14px;
  }

  .category-hero,
  .editor-header,
  .form-row {
    grid-template-columns: 1fr;
    flex-direction: column;
    align-items: stretch;
  }

  .category-hero {
    padding: 22px;
  }
}
</style>
