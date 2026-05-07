<template>
  <el-dialog
    v-model="visible"
    title="手动识别修正"
    width="560px"
    append-to-body
  >
    <section class="dialog-intro">
      <h3>修正整理预览中的识别结果</h3>
      <p>这里的修改会直接回写到整理预览中，用于下一次预览或正式执行整理。</p>
    </section>

    <el-form :model="form" label-width="110px">
      <el-form-item label="原文件名">
        <el-input v-model="form.file_name" disabled />
      </el-form-item>

      <el-form-item label="媒体类型">
        <el-select v-model="form.media_type" style="width: 100%">
          <el-option label="电影" value="movie" />
          <el-option label="剧集" value="tv" />
        </el-select>
      </el-form-item>

      <el-form-item label="标题">
        <el-input v-model="form.title" placeholder="请输入标题" />
      </el-form-item>

      <el-form-item label="年份">
        <el-input-number v-model="form.year" :min="0" :max="9999" style="width: 100%" />
      </el-form-item>

      <el-form-item v-if="form.media_type === 'tv'" label="季数">
        <el-input-number v-model="form.season" :min="0" :max="999" style="width: 100%" />
      </el-form-item>

      <el-form-item v-if="form.media_type === 'tv'" label="集数">
        <el-input-number v-model="form.episode" :min="0" :max="9999" style="width: 100%" />
      </el-form-item>

      <el-form-item label="TMDB ID">
        <el-input-number v-model="form.tmdb_id" :min="0" :max="999999999" style="width: 100%" />
      </el-form-item>
    </el-form>

    <template #footer>
      <span class="dialog-footer">
        <el-button @click="handleSearchTmdb">从 TMDB 选择</el-button>
        <el-button v-if="form.override_key" @click="handleClearOverride">清除修改</el-button>
        <el-button @click="visible = false">取消</el-button>
        <el-button type="primary" @click="handleApply">应用到预览</el-button>
      </span>
    </template>
  </el-dialog>
</template>

<script setup>
import { ElMessage } from 'element-plus'

const props = defineProps({
  form: {
    type: Object,
    required: true
  }
})

const emit = defineEmits(['apply', 'clear', 'search-tmdb'])

const visible = defineModel('visible', { type: Boolean, default: false })

const handleApply = () => {
  if (!props.form.title?.trim()) {
    ElMessage.warning('请输入识别标题')
    return
  }
  emit('apply', { ...props.form })
}

const handleClearOverride = () => {
  emit('clear', props.form.override_key)
}

const handleSearchTmdb = () => {
  emit('search-tmdb', { ...props.form })
}
</script>

<style scoped>
.dialog-intro {
  padding: 18px;
  margin-bottom: 16px;
  border-radius: 18px;
  background: linear-gradient(160deg, rgba(31, 111, 120, 0.12), rgba(242, 166, 90, 0.12));
  border: 1px solid rgba(31, 111, 120, 0.12);
}

.dialog-intro h3 {
  margin: 0;
  font-size: 18px;
  color: #17313a;
}

.dialog-intro p {
  margin: 10px 0 0;
  color: #6c6259;
  line-height: 1.7;
}

.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}

:global(.dark) .dialog-intro {
  background: rgba(16, 26, 37, 0.88);
  border-color: rgba(139, 163, 185, 0.12);
}

:global(.dark) .dialog-intro h3 {
  color: #e8edf4;
}

:global(.dark) .dialog-intro p {
  color: #9faebb;
}
</style>
