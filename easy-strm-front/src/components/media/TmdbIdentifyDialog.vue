<template>
  <el-dialog
    v-model="visible"
    title="TMDB 手动搜索"
    :width="isMobile ? '100%' : '960px'"
    append-to-body
  >
    <div class="tmdb-container">
      <div class="tmdb-search">
        <el-input
          v-model="searchKeyword"
          placeholder="输入电影或剧集名称搜索"
          @keyup.enter="handleSearch"
        >
          <template #prefix>
            <el-icon><Search /></el-icon>
          </template>
        </el-input>

        <el-select v-model="searchType" placeholder="类型" class="search-type-select">
          <el-option label="电影" value="movie" />
          <el-option label="剧集" value="tv" />
        </el-select>

        <el-button type="primary" @click="handleSearch" class="search-btn">搜索</el-button>
      </div>

      <div class="tmdb-results" v-loading="loading">
        <div v-if="results.length > 0" class="result-grid">
          <div
            v-for="item in results"
            :key="item.tmdb_id || item.id"
            class="result-item"
            @click="handleSelect(item)"
          >
            <img
              v-if="item.poster_path"
              :src="item.poster_path.trim()"
              alt="poster"
              class="poster-img"
            />
            <div v-else class="no-poster">无海报</div>

            <div class="result-info">
              <div class="result-title">{{ item.title || item.name }}</div>
              <div class="result-year">
                {{ item.release_date || item.first_air_date || item.year || '-' }}
              </div>
              <div v-if="item.overview" class="result-overview">{{ item.overview }}</div>
              <div v-if="item.vote_average" class="result-rating">
                <el-rate
                  :model-value="item.vote_average / 2"
                  disabled
                  show-score
                  :score-template="item.vote_average.toFixed(1)"
                />
              </div>
            </div>
          </div>
        </div>

        <el-empty v-else description="暂无搜索结果" />
      </div>
    </div>

    <template #footer>
      <el-button @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>
</template>

<script setup>
import { ref, watch, onMounted, onUnmounted } from 'vue'
import { Search } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { searchTmdb } from '../../utils/api/media'

const props = defineProps({
  selectMode: {
    type: String,
    default: 'cache'
  },
  currentFile: {
    type: Object,
    default: null
  }
})

const emit = defineEmits(['select'])

const visible = defineModel('visible', { type: Boolean, default: false })
const isMobile = ref(window.innerWidth < 768)
let resizeTimer = null

const searchKeyword = ref('')
const searchType = ref('movie')
const results = ref([])
const loading = ref(false)

const handleResize = () => {
  clearTimeout(resizeTimer)
  resizeTimer = setTimeout(() => {
    isMobile.value = window.innerWidth < 768
  }, 150)
}

watch(visible, (val) => {
  if (val && props.currentFile) {
    const filename = props.currentFile.name || props.currentFile.file_name || ''
    searchKeyword.value = filename.replace(/\.[^/.]+$/, '')
    results.value = []
  }
})

onMounted(() => {
  window.addEventListener('resize', handleResize)
})

onUnmounted(() => {
  window.removeEventListener('resize', handleResize)
  if (resizeTimer) {
    clearTimeout(resizeTimer)
  }
})

const handleSearch = async () => {
  if (!searchKeyword.value.trim()) {
    ElMessage.warning('请输入搜索关键词')
    return
  }

  loading.value = true
  try {
    const response = await searchTmdb({
      keyword: searchKeyword.value,
      type: searchType.value
    })
    results.value = response.data.data?.data || []
  } catch (error) {
    console.error('[TmdbIdentifyDialog] TMDB 搜索失败:', error)
    const errorMsg = error.response?.data?.error || error.message || 'TMDB 搜索失败'
    ElMessage.error(errorMsg)
  } finally {
    loading.value = false
  }
}

const handleSelect = (item) => {
  emit('select', {
    item,
    mode: props.selectMode,
    searchType: searchType.value
  })
}

const setKeyword = (keyword, type) => {
  searchKeyword.value = keyword || ''
  searchType.value = type || 'movie'
  results.value = []
}

defineExpose({ setKeyword })
</script>

<style scoped>
.tmdb-container {
  min-height: 400px;
}

.tmdb-search {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-bottom: 20px;
}

.search-type-select {
  width: 140px;
  flex-shrink: 0;
}

.search-btn {
  flex-shrink: 0;
}

.tmdb-results {
  max-height: 500px;
  overflow-y: auto;
}

.result-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 20px;
}

.result-item {
  cursor: pointer;
  border: 1px solid #e4e7ed;
  border-radius: 8px;
  overflow: hidden;
  transition: all 0.3s;
  background: #fff;
}

.result-item:hover {
  border-color: #409eff;
  box-shadow: 0 4px 16px rgba(64, 158, 255, 0.2);
  transform: translateY(-2px);
}

.poster-img {
  width: 100%;
  height: 280px;
  object-fit: cover;
}

.no-poster {
  width: 100%;
  height: 280px;
  display: flex;
  align-items: center;
  justify-content: center;
  background-color: #f5f7fa;
  color: #909399;
  font-size: 14px;
}

.result-info {
  padding: 12px;
}

.result-title {
  font-size: 14px;
  font-weight: 600;
  color: #303133;
}

.result-year {
  margin-top: 6px;
  color: #909399;
  font-size: 12px;
}

.result-overview {
  margin-top: 8px;
  color: #606266;
  font-size: 12px;
  line-height: 1.6;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  -webkit-line-clamp: 4;
}

.result-rating {
  margin-top: 10px;
}

@media (max-width: 768px) {
  .tmdb-search {
    flex-direction: column;
    align-items: stretch;
  }

  .search-type-select {
    width: 100%;
  }

  .result-grid {
    grid-template-columns: 1fr;
  }
}
</style>
