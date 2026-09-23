<template>
  <div class="space-y-4">
    <PageCard title="Emby 管理" subtitle="多实例、用户、媒体库、封面与神医助手统一工作台">
      <template #action>
        <n-button type="primary" @click="openServerDialog()">新增实例</n-button>
      </template>
      <div class="flex flex-wrap items-center gap-3">
        <n-select
          v-model:value="selectedServerId"
          class="min-w-64"
          :options="serverOptions"
          placeholder="请选择 Emby 实例"
        />
        <n-tag v-if="activeServer" :type="activeServer.enabled ? 'success' : 'warning'">
          {{ activeServer.enabled ? '已启用' : '已停用' }}
        </n-tag>
        <n-button v-if="activeServer" :loading="testing" @click="testConnection">测试连接</n-button>
        <n-button v-if="activeServer" @click="openServerDialog(activeServer)">编辑</n-button>
        <n-button v-if="activeServer" type="error" secondary @click="removeServer">删除</n-button>
      </div>
      <n-alert v-if="connectionText" class="mt-3" :type="connectionOK ? 'success' : 'error'" :closable="false">
        {{ connectionText }}
      </n-alert>
    </PageCard>

    <EmptyState v-if="!activeServer" title="请先新增或选择一个 Emby 实例" />

    <n-tabs v-else v-model:value="activeTab" type="line" animated>
      <n-tab-pane name="users" tab="用户管理">
        <PageCard title="Emby 用户" subtitle="管理用户、密码、常用权限和媒体库访问范围">
          <template #action>
            <n-button type="primary" @click="openUserDialog()">新增用户</n-button>
            <n-button :loading="usersLoading" @click="loadUsers">刷新</n-button>
          </template>
          <div v-if="users.length" class="grid gap-3 lg:grid-cols-2">
            <div v-for="user in users" :key="user.Id" class="rounded-2xl border border-slate-200 p-4 dark:border-white/10">
              <div class="flex items-start justify-between gap-3">
                <div class="flex min-w-0 items-center gap-3">
                  <div class="flex size-12 shrink-0 items-center justify-center overflow-hidden rounded-full bg-cyan-100 font-bold text-cyan-700 dark:bg-cyan-500/20 dark:text-cyan-300">
                    <img v-if="userAvatarURLs[user.Id]" :src="userAvatarURLs[user.Id]" :alt="`${user.Name} 头像`" class="size-full object-cover" />
                    <span v-else>{{ user.Name?.slice(0, 1)?.toUpperCase() || '?' }}</span>
                  </div>
                  <div class="min-w-0">
                  <div class="truncate font-bold text-slate-800 dark:text-white">{{ user.Name }}</div>
                  <div class="mt-1 flex flex-wrap gap-2">
                    <n-tag size="small" :type="user.Policy?.IsDisabled ? 'warning' : 'success'">{{ user.Policy?.IsDisabled ? '已停用' : '正常' }}</n-tag>
                    <n-tag v-if="user.Policy?.IsAdministrator" size="small" type="error">管理员</n-tag>
                    <n-tag size="small" type="info">{{ user.Policy?.EnableAllFolders ? '全部媒体库' : `${user.Policy?.EnabledFolders?.length || 0} 个媒体库` }}</n-tag>
                  </div>
                  </div>
                </div>
                <div class="flex flex-wrap justify-end gap-2">
                  <n-button size="small" @click="openUserDialog(user)">编辑权限</n-button>
                  <n-button size="small" @click="openPasswordDialog(user)">密码</n-button>
                  <n-button size="small" type="error" secondary @click="removeUser(user)">删除</n-button>
                </div>
              </div>
            </div>
          </div>
          <EmptyState v-else title="暂无 Emby 用户" />
        </PageCard>
      </n-tab-pane>

      <n-tab-pane name="libraries" tab="媒体库管理">
        <PageCard title="媒体源联动" subtitle="整理完成后按所选 Emby 实例和媒体库创建刷新任务">
          <div class="grid gap-3 md:grid-cols-[1fr_1fr_auto]">
            <n-select v-model:value="bindingForm.sourceId" :options="mediaSourceOptions" placeholder="选择 easy-strm 媒体源" />
            <n-select v-model:value="bindingForm.libraryId" :options="libraryOptions" placeholder="选择 Emby 媒体库" />
            <n-button type="primary" @click="saveMediaSourceBinding">保存绑定</n-button>
          </div>
        </PageCard>
        <PageCard title="Emby 媒体库" subtitle="配置媒体库、刷新任务以及主封面">
          <template #action>
            <n-button type="primary" @click="openLibraryDialog()">新增媒体库</n-button>
            <n-button :loading="librariesLoading" @click="refreshAllLibraries">刷新全部</n-button>
            <n-button :loading="librariesLoading" @click="loadLibraries">重新读取</n-button>
          </template>
          <div v-if="libraries.length" class="grid gap-4 lg:grid-cols-2">
            <article v-for="library in libraries" :key="libraryId(library)" class="group overflow-hidden rounded-2xl border border-slate-200 bg-white shadow-sm transition hover:-translate-y-0.5 hover:shadow-lg dark:border-white/10 dark:bg-slate-900/60">
              <div class="grid min-h-44 grid-cols-[9rem_1fr] sm:grid-cols-[11rem_1fr]">
                <div class="relative overflow-hidden bg-gradient-to-br from-indigo-500 via-violet-500 to-cyan-500">
                  <img v-if="libraryCoverURLs[libraryId(library)]" :src="libraryCoverURLs[libraryId(library)]" :alt="`${library.Name} 封面`" class="absolute inset-0 size-full object-cover transition duration-300 group-hover:scale-105" />
                  <div v-else class="absolute inset-0 flex items-center justify-center p-4 text-center text-lg font-bold text-white/90">
                    {{ library.Name?.slice(0, 6) || 'Emby' }}
                  </div>
                  <div class="absolute inset-x-0 bottom-0 h-14 bg-gradient-to-t from-black/35 to-transparent" />
                </div>
                <div class="flex min-w-0 flex-col p-4">
                  <div class="flex items-start justify-between gap-3">
                    <div class="min-w-0">
                      <h3 class="truncate text-base font-bold text-slate-800 dark:text-white">{{ library.Name }}</h3>
                      <n-tag class="mt-2" size="small" :type="libraryTypeMeta(library.CollectionType).type" round>
                        {{ libraryTypeMeta(library.CollectionType).label }}
                      </n-tag>
                    </div>
                    <div class="shrink-0 text-right">
                      <div class="text-xl font-extrabold tabular-nums text-slate-800 dark:text-white">{{ formatMediaCount(library.MediaFileCount) }}</div>
                      <div class="text-xs text-slate-400">媒体文件</div>
                    </div>
                  </div>
                  <div v-if="library.RefreshProgress > 0" class="mt-3 text-xs text-cyan-600 dark:text-cyan-300">
                    正在刷新 · {{ Math.round(library.RefreshProgress) }}%
                  </div>
                  <div class="mt-auto flex flex-wrap justify-end gap-2 pt-4">
                    <n-button size="small" @click="runLibraryRefresh(library)">刷新</n-button>
                    <n-button size="small" @click="openCoverDialog(library)">封面</n-button>
                    <n-button size="small" @click="openLibraryDialog(library)">编辑</n-button>
                    <n-button size="small" type="error" secondary @click="removeLibrary(library)">删除</n-button>
                  </div>
                </div>
              </div>
            </article>
          </div>
          <EmptyState v-else title="暂无媒体库" />
        </PageCard>
      </n-tab-pane>

      <n-tab-pane name="scheduled" tab="定时任务管理">
        <EmbyScheduledTasks :server-id="selectedServerId" :active="activeTab === 'scheduled'" @submitted="loadRecentTasks" />
      </n-tab-pane>

      <n-tab-pane name="plugin" tab="神医助手">
        <PageCard title="神医助手（StrmAssistant）" subtitle="检测插件并触发常用计划任务">
          <template #action><n-button :loading="pluginLoading" @click="loadPluginStatus">重新检测</n-button></template>
          <n-alert :type="pluginStatus.installed ? 'success' : 'warning'" :closable="false">
            {{ pluginStatus.message || '正在检测神医助手状态' }}
            <template v-if="pluginStatus.version">（版本 {{ pluginStatus.version }}）</template>
          </n-alert>
          <p class="mt-3 text-sm text-slate-500">
            easy-strm 不负责安装、升级或卸载插件。未安装时以下功能不会生效。
            <a class="text-cyan-600 hover:underline" href="https://github.com/sjtuross/StrmAssistant/wiki" target="_blank" rel="noopener noreferrer">查看安装说明</a>
          </p>
          <n-alert class="mt-3" type="info" :closable="false">
            “扫描 STRM 并生成视频封面”会在确认后自动启用所选媒体库的 Image Capture、合并神医助手 Library Scope，再扫描媒体库并运行 Extract MediaInfo。
          </n-alert>
          <div class="mt-4 grid gap-3 md:grid-cols-2 xl:grid-cols-4">
            <div v-for="item in pluginActions" :key="item.value" class="rounded-2xl border border-slate-200 p-4 dark:border-white/10">
              <div class="font-bold text-slate-800 dark:text-white">{{ item.label }}</div>
              <div class="mt-1 text-xs text-slate-500">{{ item.description }}</div>
              <div class="mt-1 text-xs text-slate-500">{{ capability(item.value)?.task_name || '未检测到对应计划任务' }}</div>
              <n-select v-model:value="pluginLibrary[item.value]" class="my-3" :options="libraryOptions" clearable :placeholder="item.requiresLibrary ? '请选择媒体库（必选）' : '选择媒体库（可选）'" />
              <n-button type="primary" block :disabled="!pluginStatus.installed || !capability(item.value)?.available || (item.requiresLibrary && !pluginLibrary[item.value])" @click="runPlugin(item.value)">触发任务</n-button>
              <div v-if="capability(item.value)?.requirement" class="mt-2 text-xs leading-relaxed text-amber-600 dark:text-amber-300">{{ capability(item.value).requirement }}</div>
            </div>
          </div>
        </PageCard>
      </n-tab-pane>

      <n-tab-pane name="tasks" tab="最近任务">
        <PageCard title="当前实例最近任务" subtitle="完整进度和结论可在任务中心查看">
          <template #action><n-button @click="goTaskCenter">前往任务中心</n-button></template>
          <div v-if="recentTasks.length" class="space-y-2">
            <div v-for="task in recentTasks" :key="task.task_id" class="flex flex-wrap items-center justify-between gap-3 rounded-xl border border-slate-200 px-4 py-3 dark:border-white/10">
              <div>
                <div class="font-medium text-slate-800 dark:text-white">{{ task.task_name }}</div>
                <div class="text-xs text-slate-500">{{ task.metadata?.current_step || task.metadata?.conclusion || task.task_type }}</div>
              </div>
              <div class="flex items-center gap-2"><n-tag :type="taskTagType(task.status)">{{ taskStatusText(task.status) }}</n-tag><n-button size="small" @click="goTask(task.task_id)">详情</n-button></div>
            </div>
          </div>
          <EmptyState v-else title="当前实例暂无任务" />
        </PageCard>
      </n-tab-pane>
    </n-tabs>

    <n-modal v-model:show="serverDialog" preset="card" class="max-w-xl" :title="serverForm.id ? '编辑 Emby 实例' : '新增 Emby 实例'">
      <n-form label-placement="top">
        <n-form-item label="实例名称"><n-input v-model:value="serverForm.name" /></n-form-item>
        <n-form-item label="服务地址"><n-input v-model:value="serverForm.base_url" placeholder="http://emby:8096" /></n-form-item>
        <n-form-item label="API Key"><SecretConfigInput v-model:value="serverForm.api_key" secret-key="emby_server_api_key" :server-id="serverForm.id || 0" :has-saved="!!serverForm.api_key_mask" :reset-key="serverDialog" :placeholder="serverForm.api_key_mask ? `已配置 ${serverForm.api_key_mask}，留空保持不变` : '请输入 API Key'" /></n-form-item>
        <div class="flex gap-6"><n-checkbox v-model:checked="serverForm.enabled">启用</n-checkbox><n-checkbox v-model:checked="serverForm.is_default">默认实例</n-checkbox></div>
      </n-form>
      <template #footer><div class="flex justify-end gap-2"><n-button @click="serverDialog=false">取消</n-button><n-button type="primary" :loading="saving" @click="saveServer">保存</n-button></div></template>
    </n-modal>

    <n-modal v-model:show="userDialog" preset="card" class="max-w-2xl" :title="userForm.id ? '编辑 Emby 用户' : '新增 Emby 用户'">
      <n-form label-placement="top">
        <n-form-item label="用户名"><n-input v-model:value="userForm.name" /></n-form-item>
        <n-form-item v-if="!userForm.id" label="初始密码"><n-input v-model:value="userForm.password" type="password" show-password-on="click" /></n-form-item>
        <n-form-item label="用户头像（JPG、PNG 或 WebP）">
          <div class="flex flex-wrap items-center gap-3">
            <div class="flex size-16 items-center justify-center overflow-hidden rounded-full bg-slate-100 text-lg font-bold text-slate-500 dark:bg-white/10">
              <img v-if="userAvatarPreviewURL || userAvatarURLs[userForm.id]" :src="userAvatarPreviewURL || userAvatarURLs[userForm.id]" alt="用户头像预览" class="size-full object-cover" />
              <span v-else>{{ userForm.name?.slice(0, 1)?.toUpperCase() || '?' }}</span>
            </div>
            <input type="file" accept="image/jpeg,image/png,image/webp" @change="selectUserAvatar" />
          </div>
        </n-form-item>
        <template v-if="userForm.id">
          <div class="grid grid-cols-2 gap-3 md:grid-cols-3">
            <n-checkbox v-model:checked="userForm.policy.IsAdministrator">管理员</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.IsDisabled">停用用户</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableRemoteAccess">远程访问</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableMediaPlayback">媒体播放</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableVideoPlaybackTranscoding">视频转码</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableAudioPlaybackTranscoding">音频转码</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableContentDownloading">媒体下载</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableSubtitleDownloading">字幕下载</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableSubtitleManagement">字幕管理</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableRemoteControlOfOtherUsers">设备控制</n-checkbox>
            <n-checkbox v-model:checked="userForm.policy.EnableAllFolders">全部媒体库</n-checkbox>
          </div>
          <n-form-item v-if="!userForm.policy.EnableAllFolders" class="mt-4" label="允许访问的媒体库"><n-select v-model:value="userForm.policy.EnabledFolders" multiple :options="userLibraryOptions" /></n-form-item>
        </template>
      </n-form>
      <template #footer><div class="flex justify-end gap-2"><n-button @click="userDialog=false">取消</n-button><n-button type="primary" :loading="saving" @click="saveUser">保存</n-button></div></template>
    </n-modal>

    <n-modal v-model:show="passwordDialog" preset="card" class="max-w-md" title="修改 Emby 用户密码">
      <n-form-item label="新密码"><n-input v-model:value="passwordForm.password" type="password" show-password-on="click" /></n-form-item>
      <n-checkbox v-model:checked="passwordForm.reset">清空密码</n-checkbox>
      <template #footer><div class="flex justify-end gap-2"><n-button @click="passwordDialog=false">取消</n-button><n-button type="primary" :loading="saving" @click="savePassword">保存</n-button></div></template>
    </n-modal>

    <n-modal v-model:show="libraryDialog" preset="card" class="max-w-2xl" :title="libraryForm.id ? '编辑媒体库' : '新增媒体库'">
      <n-form label-placement="top">
        <div class="grid gap-3 md:grid-cols-2">
          <n-form-item label="名称"><n-input v-model:value="libraryForm.name" /></n-form-item>
          <n-form-item label="内容类型"><n-select v-model:value="libraryForm.collection_type" :disabled="!!libraryForm.id" :options="collectionOptions" /></n-form-item>
        </div>
        <n-form-item label="媒体目录（每行一个）"><n-input v-model:value="libraryForm.pathsText" type="textarea" :rows="4" /></n-form-item>
        <div class="grid gap-3 md:grid-cols-2"><n-form-item label="元数据语言"><n-input v-model:value="libraryForm.metadata_language" placeholder="zh-CN" /></n-form-item><n-form-item label="国家/地区"><n-input v-model:value="libraryForm.metadata_country" placeholder="CN" /></n-form-item></div>
        <n-checkbox v-model:checked="libraryForm.enable_realtime_monitor">启用实时监控</n-checkbox>
      </n-form>
      <template #footer><div class="flex justify-end gap-2"><n-button @click="libraryDialog=false">取消</n-button><n-button type="primary" :loading="saving" @click="saveLibrary">保存</n-button></div></template>
    </n-modal>

    <n-modal v-model:show="coverDialog" preset="card" class="max-w-2xl" title="媒体库封面">
      <div class="space-y-4">
        <div><label class="mb-2 block text-sm font-medium">手动上传（选择后先预览，不会立即覆盖）</label><input type="file" accept="image/jpeg,image/png,image/webp" @change="selectManualCover" /></div>
        <div class="grid gap-3 md:grid-cols-2"><n-button :loading="coverLoading" @click="generateCover('collage')">生成海报拼图</n-button><n-button :loading="coverLoading" @click="generateCover('ai')">AI 生成封面</n-button></div>
        <n-input v-model:value="coverDescription" type="textarea" placeholder="AI 封面补充描述（可选）" />
        <div v-if="coverPreviewURL" class="space-y-3"><img :src="coverPreviewURL" alt="封面预览" class="mx-auto max-h-96 rounded-xl object-contain" /><n-button type="primary" block :loading="coverLoading" @click="applyCover">确认应用到 Emby</n-button></div>
        <n-collapse><n-collapse-item title="AI 图片接口配置" name="ai"><div class="grid gap-3 md:grid-cols-2"><n-select v-model:value="aiForm.provider" :options="aiProviderOptions" /><n-input v-model:value="aiForm.model" placeholder="模型" /><n-input v-model:value="aiForm.base_url" placeholder="Base URL" /><SecretConfigInput v-model:value="aiForm.api_key" secret-key="emby_cover_ai_api_key" :has-saved="!!aiForm.api_key_mask" :reset-key="`${coverDialog}:${aiSecretReset}`" :placeholder="aiForm.api_key_mask ? `已配置 ${aiForm.api_key_mask}` : 'API Key'" /></div><n-button class="mt-3" @click="saveAIConfig">保存 AI 配置</n-button></n-collapse-item></n-collapse>
      </div>
    </n-modal>
  </div>
</template>

<script setup>
import SecretConfigInput from '../components/common/SecretConfigInput.vue'
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import EmbyScheduledTasks from '../components/EmbyScheduledTasks.vue'
import { NAlert, NButton, NCheckbox, NCollapse, NCollapseItem, NForm, NFormItem, NInput, NModal, NSelect, NTabPane, NTabs, NTag } from 'naive-ui'
import PageCard from '../components/common/PageCard.vue'
import EmptyState from '../components/common/EmptyState.vue'
import { message } from '../utils/ui/feedback'
import { showConfirmDialog } from '../utils/ui/messageBox'
import { getTaskDetail, getUnifiedTaskList } from '../utils/api/task'
import {
  applyEmbyLibraryCover, createEmbyLibrary, createEmbyServer, createEmbyUser, deleteEmbyLibrary,
  bindEmbyMediaSource,
  deleteEmbyServer, deleteEmbyUser, generateEmbyLibraryCover, getEmbyCoverAIConfig, getEmbyCoverPreview,
  getEmbyLibraryCover, getEmbyServers, getEmbyUserAvatar, getEmbyUsers, getEmbyUserLibraries, getManagedEmbyLibraries, getStrmAssistantStatus, refreshAllManagedEmbyLibraries,
  refreshManagedEmbyLibrary, runStrmAssistantTask, setEmbyUserPassword, testEmbyServer, updateEmbyCoverAIConfig,
  updateEmbyLibrary, updateEmbyServer, updateEmbyUser, uploadEmbyLibraryCover, uploadEmbyUserAvatar
} from '../utils/api/emby'
import { getMediaSources } from '../utils/api/media'

const router = useRouter()
const servers = ref([]); const selectedServerId = ref(null); const activeTab = ref('users')
const users = ref([]); const libraries = ref([]); const recentTasks = ref([]); const pluginStatus = ref({})
const userAvatarURLs = ref({}); const userAvatarFile = ref(null); const userAvatarPreviewURL = ref('')
const libraryCoverURLs = ref({})
const mediaSources = ref([])
const testing = ref(false); const saving = ref(false); const usersLoading = ref(false); const librariesLoading = ref(false); const pluginLoading = ref(false)
const connectionText = ref(''); const connectionOK = ref(false)
const serverDialog = ref(false); const userDialog = ref(false); const passwordDialog = ref(false); const libraryDialog = ref(false); const coverDialog = ref(false)
const coverLoading = ref(false); const coverTaskId = ref(''); const coverPreviewURL = ref(''); const coverDescription = ref(''); const activeCoverLibrary = ref(null)
const manualCoverFile = ref(null)
let coverPollTimer = null

const serverForm = reactive({ id: null, name: '', base_url: '', api_key: '', api_key_mask: '', enabled: true, is_default: false })
const emptyPolicy = () => ({ IsAdministrator: false, IsDisabled: false, EnableRemoteAccess: true, EnableMediaPlayback: true, EnableVideoPlaybackTranscoding: true, EnableAudioPlaybackTranscoding: true, EnableContentDownloading: false, EnableSubtitleDownloading: true, EnableSubtitleManagement: false, EnableRemoteControlOfOtherUsers: false, EnableAllFolders: true, EnabledFolders: [] })
const userForm = reactive({ id: '', name: '', password: '', policy: emptyPolicy() })
const userLibraries = ref([])
// 用户权限使用专用接口的 Guid，不能复用刷新和封面的 ItemId。
const userLibraryOptions = computed(() => userLibraries.value.map(item => ({ label: item.name, value: item.id })))
let userDialogRequest = 0
const passwordForm = reactive({ id: '', password: '', reset: false })
const libraryForm = reactive({ id: '', original_name: '', name: '', collection_type: 'movies', pathsText: '', metadata_language: 'zh-CN', metadata_country: 'CN', enable_realtime_monitor: true })
const pluginLibrary = reactive({ strm_scan_capture: null, media_info: null, subtitle_scan: null, metadata_refresh: null })
const aiSecretReset = ref(0)
const aiForm = reactive({ provider: 'openai', base_url: 'https://api.openai.com/v1', api_key: '', api_key_mask: '', model: 'gpt-image-1' })
const bindingForm = reactive({ sourceId: null, libraryId: null })

const activeServer = computed(() => servers.value.find(item => item.id === selectedServerId.value) || null)
const serverOptions = computed(() => servers.value.map(item => ({ label: `${item.name}${item.is_default ? '（默认）' : ''}`, value: item.id })))
// Emby 不同版本对虚拟文件夹标识字段存在差异；选项值始终使用 ID，展示文本始终优先使用媒体库名称。
const libraryId = library => String(library?.ItemId || library?.Id || library?.id || '')
const libraryOptions = computed(() => libraries.value
  .map(item => {
    const value = libraryId(item)
    const label = item.Name || item.name || item.LibraryOptions?.Name || item.LibraryOptions?.name || value
    return value ? { label, value } : null
  })
  .filter(Boolean))
const libraryTypeMeta = type => ({
  movies: { label: '电影', type: 'info' },
  tvshows: { label: '电视剧', type: 'success' },
  music: { label: '音乐', type: 'warning' },
  musicvideos: { label: '音乐视频', type: 'warning' },
  books: { label: '图书', type: 'default' },
  photos: { label: '照片', type: 'error' },
  mixed: { label: '混合内容', type: 'default' }
})[String(type || 'mixed').toLowerCase()] || { label: type || '混合内容', type: 'default' }
const formatMediaCount = count => Number.isInteger(count) && count >= 0 ? count.toLocaleString('zh-CN') : '—'
const mediaSourceOptions = computed(() => mediaSources.value.map(item => ({ label: item.name, value: item.id })))
const collectionOptions = [{ label: '电影', value: 'movies' }, { label: '电视剧', value: 'tvshows' }, { label: '音乐', value: 'music' }, { label: '混合内容', value: 'mixed' }]
const aiProviderOptions = [{ label: 'OpenAI 官方', value: 'openai' }, { label: 'OpenAI 兼容接口', value: 'compatible' }]
const pluginActions = [
  { label: '扫描 STRM 并生成视频封面', value: 'strm_scan_capture', requiresLibrary: true, description: '扫描媒体库中的 STRM，再为缺少主图的视频触发截图。' },
  { label: '媒体信息提取', value: 'media_info', description: '运行神医助手 Extract MediaInfo 计划任务。' },
  { label: '外挂字幕扫描', value: 'subtitle_scan', description: '扫描媒体文件旁的外挂字幕。' },
  { label: '元数据刷新', value: 'metadata_refresh', description: '运行插件提供的元数据刷新任务。' }
]
const payload = response => response?.data?.data || response?.data || {}
const listPayload = response => { const data = payload(response); return Array.isArray(data) ? data : (data.data || []) }

const loadServers = async () => { const list = listPayload(await getEmbyServers()); servers.value = list; if (!selectedServerId.value || !list.some(x => x.id === selectedServerId.value)) selectedServerId.value = list.find(x => x.is_default)?.id || list[0]?.id || null }
const revokeUserAvatarURLs = () => { Object.values(userAvatarURLs.value).forEach(url => URL.revokeObjectURL(url)); userAvatarURLs.value = {} }
const loadUserAvatars = async list => {
  revokeUserAvatarURLs()
  const entries = await Promise.all(list.map(async user => {
    if (!user.PrimaryImageTag) return null
    try { const response = await getEmbyUserAvatar(selectedServerId.value, user.Id); return [user.Id, URL.createObjectURL(response.data)] } catch { return null }
  }))
  userAvatarURLs.value = Object.fromEntries(entries.filter(Boolean))
}
const loadUsers = async () => { if (!selectedServerId.value) return; usersLoading.value = true; try { const list = listPayload(await getEmbyUsers(selectedServerId.value)); users.value = list; await loadUserAvatars(list) } finally { usersLoading.value = false } }
const revokeLibraryCoverURLs = () => { Object.values(libraryCoverURLs.value).forEach(url => URL.revokeObjectURL(url)); libraryCoverURLs.value = {} }
const loadLibraryCovers = async (list, serverId) => {
  revokeLibraryCoverURLs()
  const entries = await Promise.all(list.map(async library => {
    const id = libraryId(library)
    if (!id) return null
    try {
      const response = await getEmbyLibraryCover(serverId, id)
      return [id, URL.createObjectURL(response.data)]
    } catch { return null }
  }))
  if (selectedServerId.value !== serverId) {
    entries.filter(Boolean).forEach(([, url]) => URL.revokeObjectURL(url))
    return
  }
  libraryCoverURLs.value = Object.fromEntries(entries.filter(Boolean))
}
const loadLibraries = async () => { if (!selectedServerId.value) return; const serverId = selectedServerId.value; librariesLoading.value = true; try { const list = listPayload(await getManagedEmbyLibraries(serverId)); libraries.value = list; await loadLibraryCovers(list, serverId) } finally { librariesLoading.value = false } }
const loadPluginStatus = async () => { if (!selectedServerId.value) return; pluginLoading.value = true; try { pluginStatus.value = payload(await getStrmAssistantStatus(selectedServerId.value)) } catch { pluginStatus.value = { installed: false, message: '神医助手状态检测失败' } } finally { pluginLoading.value = false } }
const loadRecentTasks = async () => { const tasks = listPayload(await getUnifiedTaskList()); recentTasks.value = tasks.filter(task => Number(task.metadata?.server_id) === Number(selectedServerId.value) && String(task.task_type).startsWith('emby_')).slice(0, 10) }
const loadMediaSources = async () => { mediaSources.value = listPayload(await getMediaSources()) }

const reloadServerData = async () => { connectionText.value = ''; await Promise.all([loadUsers(), loadLibraries(), loadPluginStatus(), loadRecentTasks()]) }
watch(selectedServerId, () => {
  userDialogRequest++
  userDialog.value = false
  userLibraries.value = []
  reloadServerData()
})

const openServerDialog = server => { Object.assign(serverForm, server ? { ...server, api_key: '' } : { id: null, name: '', base_url: '', api_key: '', api_key_mask: '', enabled: true, is_default: servers.value.length === 0 }); serverDialog.value = true }
const saveServer = async () => { saving.value = true; try { if (serverForm.id) await updateEmbyServer(serverForm.id, serverForm); else await createEmbyServer(serverForm); serverDialog.value = false; await loadServers(); message.success('Emby 实例已保存') } finally { saving.value = false } }
const removeServer = async () => { try { await showConfirmDialog(`只删除 easy-strm 中的“${activeServer.value.name}”连接配置，不会删除 Emby 数据。`, '删除 Emby 实例'); await deleteEmbyServer(activeServer.value.id); await loadServers(); message.success('Emby 实例已删除') } catch {} }
const testConnection = async () => { testing.value = true; try { const data = payload(await testEmbyServer(selectedServerId.value)); connectionOK.value = !!data.connected; connectionText.value = data.connected ? `连接成功：${data.info?.ServerName || ''} v${data.info?.Version || ''}` : `连接失败：${data.error || '未知错误'}` } finally { testing.value = false } }

const revokeUserAvatarPreview = () => { if (userAvatarPreviewURL.value) URL.revokeObjectURL(userAvatarPreviewURL.value); userAvatarPreviewURL.value = '' }
const openUserDialog = async user => {
  const requestId = ++userDialogRequest
  const serverId = selectedServerId.value
  if (!serverId) return
  revokeUserAvatarPreview()
  userAvatarFile.value = null
  // 先获取权限名称及 Guid，接口失败时不打开缺少选项的编辑弹窗。
  const folders = listPayload(await getEmbyUserLibraries(serverId))
  if (selectedServerId.value !== serverId || requestId !== userDialogRequest) return
  userLibraries.value = folders
  const selectedFolderIds = (user?.Policy?.EnabledFolders || []).map(folderId => {
    const id = String(folderId)
    return folders.find(folder => folder.id === id || folder.item_id === id)?.id || id
  })
  Object.assign(userForm, user ? { id: user.Id, name: user.Name, password: '', policy: { ...emptyPolicy(), ...(user.Policy || {}), EnabledFolders: selectedFolderIds } } : { id: '', name: '', password: '', policy: emptyPolicy() })
  userDialog.value = true
}
const selectUserAvatar = event => { const file = event.target.files?.[0]; if (!file) return; userAvatarFile.value = file; revokeUserAvatarPreview(); userAvatarPreviewURL.value = URL.createObjectURL(file); event.target.value = '' }
const saveUser = async () => {
  saving.value = true
  try {
    let userId = userForm.id
    // 尊重全库开关；切换为全部媒体库时清空隐藏的历史选择。
    const enabledFolders = [...new Set((userForm.policy.EnabledFolders || []).map(folderId => String(folderId).trim()).filter(Boolean))]
    const policy = { ...userForm.policy, EnabledFolders: userForm.policy.EnableAllFolders ? [] : enabledFolders }
    if (userId) await updateEmbyUser(selectedServerId.value, userId, { name: userForm.name, policy })
    else {
      const data = payload(await createEmbyUser(selectedServerId.value, { name: userForm.name, password: userForm.password }))
      userId = data.user?.Id || data.user?.id || ''
      // 新建用户也必须单独写入权限，创建接口只负责账号和密码。
      if (userId) await updateEmbyUser(selectedServerId.value, userId, { name: userForm.name, policy })
    }
    if (userAvatarFile.value && userId) await uploadEmbyUserAvatar(selectedServerId.value, userId, userAvatarFile.value)
    userDialog.value = false; userAvatarFile.value = null; revokeUserAvatarPreview()
    await Promise.all([loadUsers(), loadRecentTasks()]); message.success('用户操作已完成并写入任务中心')
  } finally { saving.value = false }
}
const openPasswordDialog = user => { Object.assign(passwordForm, { id: user.Id, password: '', reset: false }); passwordDialog.value = true }
const savePassword = async () => { saving.value = true; try { await setEmbyUserPassword(selectedServerId.value, passwordForm.id, { password: passwordForm.password, reset: passwordForm.reset }); passwordDialog.value = false; await loadRecentTasks(); message.success('密码操作已完成') } finally { saving.value = false } }
const removeUser = async user => { try { await showConfirmDialog(`确认删除 Emby 用户“${user.Name}”？`, '删除用户'); await deleteEmbyUser(selectedServerId.value, user.Id); await Promise.all([loadUsers(), loadRecentTasks()]); message.success('用户已删除') } catch {} }

const openLibraryDialog = library => { const locations = library?.Locations || (library?.Path ? [library.Path] : []); Object.assign(libraryForm, library ? { id: libraryId(library), original_name: library.Name, name: library.Name, collection_type: library.CollectionType || 'mixed', pathsText: locations.join('\n'), metadata_language: library.LibraryOptions?.PreferredMetadataLanguage ?? '', metadata_country: library.LibraryOptions?.MetadataCountryCode ?? '', enable_realtime_monitor: library.LibraryOptions?.EnableRealtimeMonitor ?? false } : { id: '', original_name: '', name: '', collection_type: 'movies', pathsText: '', metadata_language: 'zh-CN', metadata_country: 'CN', enable_realtime_monitor: true }); libraryDialog.value = true }
const libraryRequest = () => ({ original_name: libraryForm.original_name, name: libraryForm.name, collection_type: libraryForm.collection_type, paths: libraryForm.pathsText.split(/\r?\n/).map(Path => ({ Path: Path.trim() })).filter(item => item.Path), metadata_language: libraryForm.metadata_language, metadata_country: libraryForm.metadata_country, enable_realtime_monitor: libraryForm.enable_realtime_monitor })
const saveLibrary = async () => { saving.value = true; try { const data = libraryRequest(); if (libraryForm.id) await updateEmbyLibrary(selectedServerId.value, libraryForm.id, data); else await createEmbyLibrary(selectedServerId.value, data); libraryDialog.value = false; await Promise.all([loadLibraries(), loadRecentTasks()]); message.success('媒体库操作已完成') } finally { saving.value = false } }
const removeLibrary = async library => { try { await showConfirmDialog(`只从 Emby 移除媒体库“${library.Name}”，不会删除原始媒体文件。`, '删除媒体库'); await deleteEmbyLibrary(selectedServerId.value, library.ItemId, library.Name); await Promise.all([loadLibraries(), loadRecentTasks()]); message.success('媒体库配置已删除') } catch {} }
const saveMediaSourceBinding = async () => { if (!bindingForm.sourceId || !bindingForm.libraryId) { message.warning('请选择媒体源和 Emby 媒体库'); return } await bindEmbyMediaSource(selectedServerId.value, bindingForm.sourceId, bindingForm.libraryId); message.success('媒体源联动已保存') }
const runLibraryRefresh = async library => { const data = payload(await refreshManagedEmbyLibrary(selectedServerId.value, library.ItemId)); message.success('刷新任务已提交'); goTask(data.task_id) }
const refreshAllLibraries = async () => { librariesLoading.value = true; try { const data = payload(await refreshAllManagedEmbyLibraries(selectedServerId.value)); message.success('全部刷新任务已提交'); goTask(data.task_id) } finally { librariesLoading.value = false } }

const capability = action => pluginStatus.value?.capabilities?.[action]
const runPlugin = async action => {
  const item = pluginActions.find(option => option.value === action)
  if (item?.requiresLibrary && !pluginLibrary[action]) { message.warning('请选择要扫描的 Emby 媒体库'); return }
  let autoConfigure = false
  if (action === 'strm_scan_capture') {
    const libraryName = libraryOptions.value.find(option => option.value === pluginLibrary[action])?.label || pluginLibrary[action]
    try {
      await showConfirmDialog(`将自动执行以下操作：\n1. 在媒体库“${libraryName}”中启用 Image Capture；\n2. 确保神医助手 Library Scope 包含该媒体库；\n3. 回读确认配置后扫描 STRM 并生成视频封面。\n\n神医助手计划任务可能同时处理插件配置范围内的其他媒体库。`, '确认启用并触发')
      autoConfigure = true
    } catch { return }
  }
  const data = payload(await runStrmAssistantTask(selectedServerId.value, { action, library_id: pluginLibrary[action] || '', auto_configure: autoConfigure }))
  message.success(action === 'strm_scan_capture' ? '自动配置、STRM 扫描与截图任务已提交' : '神医助手任务已提交')
  goTask(data.task_id)
}

const revokePreview = () => { if (coverPreviewURL.value) URL.revokeObjectURL(coverPreviewURL.value); coverPreviewURL.value = '' }
const openCoverDialog = async library => { activeCoverLibrary.value = library; coverTaskId.value = ''; manualCoverFile.value = null; coverDescription.value = ''; revokePreview(); coverDialog.value = true; const config = payload(await getEmbyCoverAIConfig()); Object.assign(aiForm, { ...aiForm, ...config, api_key: '' }) }
const selectManualCover = event => { const file = event.target.files?.[0]; if (!file) return; manualCoverFile.value = file; coverTaskId.value = ''; revokePreview(); coverPreviewURL.value = URL.createObjectURL(file); event.target.value = '' }
const generateCover = async mode => { coverLoading.value = true; manualCoverFile.value = null; revokePreview(); try { const data = payload(await generateEmbyLibraryCover(selectedServerId.value, activeCoverLibrary.value.ItemId, { mode, library_name: activeCoverLibrary.value.Name, description: coverDescription.value })); coverTaskId.value = data.task_id; pollCoverTask() } catch { coverLoading.value = false } }
const pollCoverTask = async () => { clearTimeout(coverPollTimer); if (!coverTaskId.value) return; try { const task = payload(await getTaskDetail(coverTaskId.value)); if (task.metadata?.awaiting_confirmation) { const response = await getEmbyCoverPreview(coverTaskId.value); revokePreview(); coverPreviewURL.value = URL.createObjectURL(response.data); coverLoading.value = false; return } if (task.status === 'failed') { coverLoading.value = false; message.error(task.error_message || '封面生成失败'); return } } catch { coverLoading.value = false; return } coverPollTimer = setTimeout(pollCoverTask, 1500) }
const applyCover = async () => { coverLoading.value = true; try { if (manualCoverFile.value) await uploadEmbyLibraryCover(selectedServerId.value, activeCoverLibrary.value.ItemId, manualCoverFile.value); else await applyEmbyLibraryCover(selectedServerId.value, activeCoverLibrary.value.ItemId, coverTaskId.value); message.success('封面已应用到 Emby'); manualCoverFile.value = null; revokePreview(); coverDialog.value = false; await Promise.all([loadLibraries(), loadRecentTasks()]) } finally { coverLoading.value = false } }
const saveAIConfig = async () => { await updateEmbyCoverAIConfig(aiForm); const config = payload(await getEmbyCoverAIConfig()); Object.assign(aiForm, { ...config, api_key: '' }); aiSecretReset.value++; message.success('AI 图片接口配置已保存') }

const taskStatusText = status => ({ pending: '待执行/待确认', running: '执行中', success: '成功', partial_success: '部分成功', failed: '失败', cancelled: '已取消', unknown: '结果未知', completed: '已完成' })[status] || status
const taskTagType = status => ({ success: 'success', completed: 'success', partial_success: 'warning', failed: 'error', unknown: 'warning', running: 'info' })[status] || 'default'
const goTask = taskId => router.push({ path: '/dashboard/tasks', query: { task_id: taskId } })
const goTaskCenter = () => router.push({ path: '/dashboard/tasks', query: { emby_server_id: selectedServerId.value } })

onMounted(async () => { await Promise.all([loadServers(), loadMediaSources()]); if (selectedServerId.value) await reloadServerData() })
onBeforeUnmount(() => { clearTimeout(coverPollTimer); revokePreview(); revokeUserAvatarPreview(); revokeUserAvatarURLs(); revokeLibraryCoverURLs() })
</script>
