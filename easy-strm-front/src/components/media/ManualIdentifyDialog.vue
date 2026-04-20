<template>
  <el-dialog
    v-model="visible"
    title="手动识别修正"
    width="560px"
    append-to-body
  >
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
.dialog-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
</style>
