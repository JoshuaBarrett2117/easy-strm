<template>
  <div class="space-y-4">
    <PageCard title="文件名识别测试" subtitle="Filename Recognition Lab">
      <p class="text-sm leading-6 text-slate-500 dark:text-slate-400">
        输入一个媒体文件名，先使用真实整理链路的规则提取标题、类型、季集等信息，再按需查询 TMDB 候选。规则测试不写入识别缓存。
      </p>

      <div class="mt-5 grid gap-4 lg:grid-cols-[minmax(0,1fr)_auto] lg:items-end">
        <n-form-item label="媒体文件名" :show-feedback="false">
          <n-input
            v-model:value="filename"
            type="textarea"
            :autosize="{ minRows: 2, maxRows: 4 }"
            placeholder="例如：妖精的尾巴 百年任务 - S01E05 - 艰难的决断.mp4"
            @keydown.ctrl.enter.prevent="runFullRecognition"
          />
        </n-form-item>
        <div class="flex flex-wrap gap-2">
          <n-button :loading="parseLoading" @click="runLocalParse">仅解析规则</n-button>
          <n-button type="primary" :loading="identifyLoading" @click="runFullRecognition">
            解析并查询 TMDB
          </n-button>
        </div>
      </div>

      <div class="mt-4 flex flex-wrap items-center gap-2">
        <span class="text-xs font-semibold text-slate-400">常用样例</span>
        <n-button
          v-for="example in examples"
          :key="example"
          size="tiny"
          quaternary
          @click="selectExample(example)"
        >
          {{ example }}
        </n-button>
      </div>
    </PageCard>

    <div v-if="parsedResult" class="grid gap-4 xl:grid-cols-[minmax(0,0.9fr)_minmax(0,1.1fr)]">
      <PageCard title="规则解析结果" subtitle="Local Parse">
        <div class="grid grid-cols-2 gap-3 sm:grid-cols-3">
          <ResultField label="媒体标题" :value="parsedResult.title || '-'" class="col-span-2 sm:col-span-3" />
          <ResultField label="媒体类型" :value="mediaTypeText(parsedResult.media_type)" />
          <ResultField label="年份" :value="parsedResult.year || '-'" />
          <ResultField label="画质" :value="parsedResult.quality || '-'" />
          <ResultField v-if="parsedResult.media_type === 'tv'" label="季" :value="numberText(parsedResult.season)" highlight />
          <ResultField v-if="parsedResult.media_type === 'tv'" label="集" :value="numberText(parsedResult.episode)" highlight />
          <ResultField label="来源" :value="parsedResult.source || '-'" />
          <ResultField label="编码" :value="parsedResult.codec || '-'" />
          <ResultField label="命中规则" :value="parsedResult.matched_rule_name || '未命中规则，按电影标题清理'" class="col-span-2" />
        </div>
        <div v-if="parsedResult.search_titles?.length" class="mt-4 rounded-xl bg-slate-50 p-3 dark:bg-slate-900/50">
          <div class="text-xs font-bold text-slate-400">TMDB 搜索标题</div>
          <div class="mt-2 flex flex-wrap gap-2">
            <n-tag v-for="title in parsedResult.search_titles" :key="title" size="small">{{ title }}</n-tag>
          </div>
        </div>
      </PageCard>

      <PageCard title="TMDB 候选" subtitle="Online Match">
        <n-alert v-if="identifyMessage" :type="tmdbCandidates.length ? 'success' : 'warning'" :show-icon="true" class="mb-4">
          {{ identifyMessage }}
        </n-alert>
        <div v-if="tmdbCandidates.length" class="space-y-3">
          <article
            v-for="candidate in tmdbCandidates"
            :key="`${candidate.media_type}-${candidate.tmdb_id}`"
            class="flex gap-3 rounded-xl border border-slate-200/80 p-3 dark:border-slate-700/80"
          >
            <img
              v-if="candidate.poster_path"
              :src="posterURL(candidate.poster_path)"
              :alt="candidate.title"
              class="h-24 w-16 shrink-0 rounded-lg object-cover"
            />
            <div class="min-w-0 flex-1">
              <div class="flex flex-wrap items-center gap-2">
                <h3 class="truncate font-bold text-slate-800 dark:text-white">{{ candidate.title }}</h3>
                <n-tag size="small" type="info">TMDB {{ candidate.tmdb_id }}</n-tag>
                <n-tag size="small">{{ mediaTypeText(candidate.media_type || parsedResult.media_type) }}</n-tag>
              </div>
              <p class="mt-1 text-xs text-slate-400">
                {{ candidate.original_title || '无原始标题' }} · {{ candidate.year || '年份未知' }} · 评分 {{ candidate.vote_average || '-' }}
              </p>
              <p class="mt-2 line-clamp-2 text-sm leading-5 text-slate-500 dark:text-slate-400">
                {{ candidate.overview || '暂无简介' }}
              </p>
            </div>
          </article>
        </div>
        <div v-else class="flex min-h-40 items-center justify-center rounded-xl border border-dashed border-slate-200 text-sm text-slate-400 dark:border-slate-700">
          {{ identifyAttempted ? '没有可展示的 TMDB 候选' : '点击“解析并查询 TMDB”获取在线媒体信息' }}
        </div>
      </PageCard>
    </div>

    <PageCard title="文件名识别规则" subtitle="Shared Organize Rules">
      <template #action>
        <n-button size="small" @click="addRule">新增规则</n-button>
        <n-button size="small" :loading="resetLoading" @click="resetRules">恢复常用模板</n-button>
        <n-button size="small" type="primary" :loading="saveLoading" @click="saveRules">保存并启用</n-button>
      </template>

      <n-alert type="info" :show-icon="true" class="mb-4">
        正则匹配的是标准化文件名：{{ inputTransform || '去扩展名并统一分隔符' }}。所有规则必须包含
        <code>(?P&lt;title&gt;...)</code>；剧集规则还必须包含 <code>(?P&lt;episode&gt;...)</code>，季数可用
        <code>(?P&lt;season&gt;...)</code> 或“默认季”。优先级数字越小越先匹配。
      </n-alert>

      <div v-if="rulesLoading" class="py-16 text-center text-sm text-slate-400">正在加载规则...</div>
      <div v-else class="space-y-3">
        <article
          v-for="(rule, index) in rules"
          :key="rule.localKey"
          class="rounded-2xl border p-4 transition-colors"
          :class="rule.enabled ? 'border-indigo-200 bg-indigo-50/30 dark:border-indigo-900/70 dark:bg-indigo-950/10' : 'border-slate-200 bg-slate-50/50 dark:border-slate-700 dark:bg-slate-900/30'"
        >
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex min-w-0 items-center gap-3">
              <n-switch v-model:value="rule.enabled" />
              <div class="min-w-0">
                <div class="truncate font-bold text-slate-800 dark:text-white">{{ rule.name || '未命名规则' }}</div>
                <div class="mt-0.5 text-xs text-slate-400">{{ rule.id || '尚未填写 ID' }} · 优先级 {{ rule.priority }}</div>
              </div>
            </div>
            <div class="flex gap-1">
              <n-button size="tiny" quaternary :disabled="index === 0" @click="moveRule(index, -1)">上移</n-button>
              <n-button size="tiny" quaternary :disabled="index === rules.length - 1" @click="moveRule(index, 1)">下移</n-button>
              <n-button size="tiny" quaternary type="error" @click="removeRule(index)">删除</n-button>
            </div>
          </div>

          <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <n-form-item label="规则名称" :show-feedback="false">
              <n-input v-model:value="rule.name" placeholder="SxxExx 标准剧集" />
            </n-form-item>
            <n-form-item label="规则 ID" :show-feedback="false">
              <n-input v-model:value="rule.id" placeholder="tv_sxe" />
            </n-form-item>
            <n-form-item label="媒体类型" :show-feedback="false">
              <n-select v-model:value="rule.media_type" :options="mediaTypeOptions" />
            </n-form-item>
            <n-form-item label="默认季" :show-feedback="false">
              <n-input-number v-model:value="rule.default_season" :min="0" :max="99" class="w-full" />
            </n-form-item>
          </div>
          <n-form-item label="Go 正则表达式" :show-feedback="false" class="mt-3">
            <n-input v-model:value="rule.pattern" type="textarea" :autosize="{ minRows: 2, maxRows: 5 }" class="font-mono" />
          </n-form-item>
          <div class="mt-3 grid gap-3 lg:grid-cols-2">
            <n-form-item label="匹配示例" :show-feedback="false">
              <n-input v-model:value="rule.example" placeholder="用于保存时校验规则是否可用" />
            </n-form-item>
            <n-form-item label="规则说明" :show-feedback="false">
              <n-input v-model:value="rule.description" placeholder="说明适用的文件名格式与限制" />
            </n-form-item>
          </div>
        </article>
      </div>
    </PageCard>
  </div>
</template>

<script setup>
import { h, onMounted, ref } from 'vue'
import {
  NAlert,
  NButton,
  NFormItem,
  NInput,
  NInputNumber,
  NSelect,
  NSwitch,
  NTag,
  useMessage
} from 'naive-ui'
import PageCard from '../components/common/PageCard.vue'
import {
  autoIdentifyFile,
  getFilenameRecognitionRules,
  parseMediaFilename,
  resetFilenameRecognitionRules,
  updateFilenameRecognitionRules
} from '../utils/api/media'
import { showConfirmDialog } from '../utils/ui/messageBox'

const ResultField = (props) => h('div', {
  class: props.class || ''
}, [
  h('div', { class: 'text-xs font-bold text-slate-400' }, props.label),
  h('div', {
    class: props.highlight
      ? 'mt-1 text-xl font-black text-indigo-600 dark:text-indigo-300'
      : 'mt-1 break-all text-sm font-bold text-slate-700 dark:text-slate-200'
  }, String(props.value))
])
ResultField.props = ['label', 'value', 'highlight', 'class']

const message = useMessage()
const filename = ref('妖精的尾巴 百年任务 - S01E05 - 艰难的决断.mp4')
const examples = [
  '妖精的尾巴 百年任务 - S01E05 - 艰难的决断.mp4',
  'Breaking.Bad.S02E07.1080p.mkv',
  '庆余年 第2季 第05集.mp4',
  'Inception.2010.1080p.BluRay.mkv'
]
const mediaTypeOptions = [
  { label: '剧集', value: 'tv' },
  { label: '电影', value: 'movie' }
]

const parsedResult = ref(null)
const tmdbCandidates = ref([])
const identifyMessage = ref('')
const identifyAttempted = ref(false)
const parseLoading = ref(false)
const identifyLoading = ref(false)
const rulesLoading = ref(false)
const saveLoading = ref(false)
const resetLoading = ref(false)
const rules = ref([])
const inputTransform = ref('')
let localKeySeed = 0

const responseData = (response) => response?.data?.data ?? response?.data ?? null
const withLocalKey = (rule) => ({ ...rule, localKey: `rule-${++localKeySeed}` })
const mediaTypeText = (type) => type === 'tv' ? '剧集' : type === 'movie' ? '电影' : '未知'
const numberText = (value) => Number.isInteger(value) ? value : '-'
const posterURL = (path) => `https://image.tmdb.org/t/p/w185${path}`

const requireFilename = () => {
  filename.value = filename.value.trim()
  if (!filename.value) {
    message.warning('请输入要测试的媒体文件名')
    return false
  }
  return true
}

const runLocalParse = async () => {
  if (!requireFilename()) return null
  parseLoading.value = true
  try {
    const response = await parseMediaFilename({ filename: filename.value })
    parsedResult.value = responseData(response)
    tmdbCandidates.value = []
    identifyMessage.value = ''
    identifyAttempted.value = false
    return parsedResult.value
  } catch (error) {
    message.error(error.response?.data?.error || error.message || '文件名解析失败')
    return null
  } finally {
    parseLoading.value = false
  }
}

const runFullRecognition = async () => {
  if (!requireFilename()) return
  identifyLoading.value = true
  identifyAttempted.value = true
  tmdbCandidates.value = []
  identifyMessage.value = ''
  try {
    const parseResponse = await parseMediaFilename({ filename: filename.value })
    parsedResult.value = responseData(parseResponse)
    const identifyResponse = await autoIdentifyFile({ filename: filename.value })
    const identifyResult = responseData(identifyResponse) || {}
    tmdbCandidates.value = identifyResult.candidates || []
    identifyMessage.value = identifyResult.success
      ? `识别成功，找到 ${tmdbCandidates.value.length} 个候选`
      : (identifyResult.message || '未找到匹配的媒体信息')
  } catch (error) {
    identifyMessage.value = error.response?.data?.error || error.message || 'TMDB 识别失败'
  } finally {
    identifyLoading.value = false
  }
}

const selectExample = (example) => {
  filename.value = example
  runLocalParse()
}

const loadRules = async () => {
  rulesLoading.value = true
  try {
    const response = await getFilenameRecognitionRules()
    const data = responseData(response) || {}
    rules.value = (data.rules || []).map(withLocalKey)
    inputTransform.value = data.input_transform || ''
  } catch (error) {
    message.error(error.response?.data?.error || error.message || '加载识别规则失败')
  } finally {
    rulesLoading.value = false
  }
}

const normalizeRulePriorities = () => {
  rules.value.forEach((rule, index) => { rule.priority = (index + 1) * 10 })
}

const moveRule = (index, offset) => {
  const target = index + offset
  if (target < 0 || target >= rules.value.length) return
  const next = [...rules.value]
  ;[next[index], next[target]] = [next[target], next[index]]
  rules.value = next
  normalizeRulePriorities()
}

const addRule = () => {
  rules.value.push(withLocalKey({
    id: `custom_rule_${rules.value.length + 1}`,
    name: '自定义规则',
    description: '',
    pattern: '^(?P<title>.+?)\\s+S(?P<season>\\d{1,2})E(?P<episode>\\d{1,3})$',
    media_type: 'tv',
    default_season: 1,
    example: 'Show S01E01.mkv',
    enabled: false,
    priority: (rules.value.length + 1) * 10
  }))
}

const removeRule = (index) => {
  rules.value.splice(index, 1)
  normalizeRulePriorities()
}

const saveRules = async () => {
  saveLoading.value = true
  try {
    normalizeRulePriorities()
    const payloadRules = rules.value.map(({ localKey, ...rule }) => rule)
    const response = await updateFilenameRecognitionRules({ rules: payloadRules })
    const data = responseData(response) || {}
    rules.value = (data.rules || []).map(withLocalKey)
    inputTransform.value = data.input_transform || inputTransform.value
    message.success('识别规则已保存并立即应用到整理链路')
    if (filename.value) await runLocalParse()
  } catch (error) {
    message.error(error.response?.data?.error || error.message || '保存识别规则失败')
  } finally {
    saveLoading.value = false
  }
}

const resetRules = async () => {
  try {
    await showConfirmDialog('恢复后将覆盖当前规则配置，是否继续？', '恢复常用模板')
  } catch {
    return
  }
  resetLoading.value = true
  try {
    const response = await resetFilenameRecognitionRules()
    const data = responseData(response) || {}
    rules.value = (data.rules || []).map(withLocalKey)
    inputTransform.value = data.input_transform || inputTransform.value
    message.success('已恢复并启用常用文件名规则模板')
    if (filename.value) await runLocalParse()
  } catch (error) {
    message.error(error.response?.data?.error || error.message || '恢复规则失败')
  } finally {
    resetLoading.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadRules(), runLocalParse()])
})
</script>
