<template>
  <el-dialog
    v-model="visible"
    title="TMDB 候选结果"
    width="760px"
    append-to-body
  >
    <div class="candidates-container" v-loading="loading">
      <section class="candidates-overview">
        <div class="candidates-hint">
          <el-icon style="margin-right: 4px;"><InfoFilled /></el-icon>
          为您找到了以下匹配结果，请选择正确的条目；如果都不匹配，可以切换到手动搜索。
        </div>
        <div class="overview-chip">
          <span>候选数量</span>
          <strong>{{ candidatesList.length }}</strong>
        </div>
      </section>

      <div v-if="candidatesList.length > 0" class="candidates-grid">
        <div
          v-for="(item, index) in candidatesList"
          :key="item.tmdb_id || index"
          class="candidate-card"
          @click="handleSelect(item)"
        >
          <div class="candidate-rank">{{ index + 1 }}</div>

          <div class="candidate-poster">
            <img v-if="item.poster_path" :src="item.poster_path.trim()" alt="poster" />
            <div v-else class="no-poster-sm">无海报</div>
          </div>

          <div class="candidate-info">
            <div class="candidate-title">{{ item.title || item.original_title || '-' }}</div>

            <div class="candidate-meta">
              <span v-if="item.year" class="candidate-year">{{ item.year }}</span>
              <span v-if="item.media_type" class="candidate-type">
                <el-tag size="small" :type="item.media_type === 'tv' ? 'warning' : 'primary'">
                  {{ item.media_type === 'tv' ? '剧集' : '电影' }}
                </el-tag>
              </span>
              <span v-if="item.vote_average" class="candidate-rating">
                <el-icon style="color: #f7ba2a; margin-right: 2px;"><StarFilled /></el-icon>
                {{ item.vote_average.toFixed(1) }}
              </span>
            </div>

            <div v-if="item.overview" class="candidate-overview">{{ item.overview }}</div>
            <div
              v-if="item.original_title && item.original_title !== item.title"
              class="candidate-original-title"
            >
              {{ item.original_title }}
            </div>
          </div>
        </div>
      </div>

      <el-empty v-else-if="!loading" description="未找到候选结果" />
    </div>

    <template #footer>
      <span class="dialog-footer">
        <el-button @click="handleManualSearch">
          <el-icon><Search /></el-icon>
          手动搜索
        </el-button>
        <el-button @click="visible = false">取消</el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { InfoFilled, StarFilled, Search } from '@element-plus/icons-vue'

defineProps({
  candidatesList: {
    type: Array,
    default: () => []
  },
  mediaType: {
    type: String,
    default: 'movie'
  }
})

const emit = defineEmits(['select', 'manual-search'])

const visible = defineModel('visible', { type: Boolean, default: false })
const loading = defineModel('loading', { type: Boolean, default: false })

const handleSelect = (candidate) => {
  emit('select', candidate)
}

const handleManualSearch = () => {
  emit('manual-search')
}
</script>

<style scoped>
.candidates-container {
  min-height: 200px;
}

.candidates-overview {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-bottom: 16px;
}

.candidates-hint {
  display: flex;
  align-items: center;
  padding: 12px 16px;
  background: linear-gradient(135deg, rgba(31, 111, 120, 0.1), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
  border-radius: 14px;
  font-size: 13px;
  color: #24535f;
  line-height: 1.5;
  flex: 1;
}

.overview-chip {
  padding: 12px 16px;
  border-radius: 14px;
  background: rgba(244, 239, 231, 0.88);
  min-width: 110px;
  text-align: center;
}

.overview-chip span {
  display: block;
  font-size: 12px;
  color: #8a7b6d;
}

.overview-chip strong {
  display: block;
  margin-top: 6px;
  font-size: 24px;
  color: #17313a;
}

.candidates-grid {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.candidate-card {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 14px;
  border: 2px solid #e4e7ed;
  border-radius: 10px;
  cursor: pointer;
  transition: all 0.25s ease;
  background: #fff;
  position: relative;
}

.candidate-card:hover {
  border-color: #409eff;
  box-shadow: 0 4px 16px rgba(64, 158, 255, 0.18);
  transform: translateY(-1px);
}

.candidate-rank {
  position: absolute;
  top: -1px;
  left: -1px;
  width: 26px;
  height: 26px;
  border-radius: 10px 0 10px 0;
  background: #409eff;
  color: #fff;
  font-size: 13px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
}

.candidate-poster {
  width: 80px;
  flex-shrink: 0;
}

.candidate-poster img {
  width: 80px;
  height: 114px;
  object-fit: cover;
  border-radius: 6px;
  background: #f5f7fa;
}

.no-poster-sm {
  width: 80px;
  height: 114px;
  border-radius: 6px;
  background: #f5f7fa;
  color: #909399;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
}

.candidate-info {
  flex: 1;
  min-width: 0;
}

.candidate-title {
  font-size: 16px;
  font-weight: 700;
  color: #303133;
  margin-bottom: 8px;
}

.candidate-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 8px;
  color: #606266;
  font-size: 13px;
}

.candidate-overview {
  color: #606266;
  font-size: 13px;
  line-height: 1.6;
  margin-bottom: 6px;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  overflow: hidden;
  -webkit-line-clamp: 3;
}

.candidate-original-title {
  color: #909399;
  font-size: 12px;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:global(.dark) .candidates-hint,
:global(.dark) .overview-chip,
:global(.dark) .candidate-card {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .candidate-title,
:global(.dark) .overview-chip strong {
  color: #e8edf4;
}

:global(.dark) .candidate-meta,
:global(.dark) .candidate-overview,
:global(.dark) .candidate-original-title,
:global(.dark) .candidates-hint,
:global(.dark) .overview-chip span,
:global(.dark) .no-poster-sm {
  color: #9faebb;
}

@media (max-width: 768px) {
  .candidates-overview {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
