<template>
  <div class="relative min-h-screen overflow-hidden bg-[#080b14] text-white">
    <div class="pointer-events-none absolute inset-0">
      <div class="absolute -left-40 -top-48 h-[34rem] w-[34rem] rounded-full bg-indigo-600/20 blur-[110px]"></div>
      <div class="absolute -bottom-48 right-[-10rem] h-[38rem] w-[38rem] rounded-full bg-cyan-500/10 blur-[120px]"></div>
      <div class="absolute inset-0 opacity-40" style="background-image: linear-gradient(rgba(255,255,255,0.025) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.025) 1px, transparent 1px); background-size: 44px 44px;"></div>
    </div>

    <div class="relative z-10 mx-auto grid min-h-screen w-full max-w-[1480px] lg:grid-cols-[1.15fr_0.85fr]">
      <section class="hidden flex-col justify-between px-10 py-10 lg:flex xl:px-16 xl:py-14">
        <div class="flex items-center gap-3">
          <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-400 via-indigo-500 to-cyan-500 shadow-lg shadow-indigo-950/50 ring-1 ring-white/20">
            <n-icon size="23" :component="PlayCircleOutline" />
          </div>
          <div>
            <div class="text-[15px] font-extrabold tracking-tight">Easy Stream</div>
            <div class="text-[10px] font-bold uppercase tracking-[0.2em] text-slate-500">Media Workspace</div>
          </div>
        </div>

        <div class="max-w-2xl pb-8">
          <div class="mb-5 inline-flex items-center gap-2 rounded-full border border-indigo-400/20 bg-indigo-400/10 px-3 py-1.5 text-xs font-bold text-indigo-200">
            <span class="h-1.5 w-1.5 rounded-full bg-cyan-300"></span>
            让媒体整理回归清晰与秩序
          </div>
          <h1 class="max-w-xl text-5xl font-black leading-[1.1] tracking-[-0.04em] xl:text-6xl">
            一站式管理你的
            <span class="bg-gradient-to-r from-indigo-300 via-violet-200 to-cyan-300 bg-clip-text text-transparent">媒体资产</span>
          </h1>
          <p class="mt-6 max-w-xl text-base leading-8 text-slate-400">
            从媒体源接入、识别整理到 STRM 生成与任务追踪，让复杂的资源工作流变得直观、可见、可掌控。
          </p>

          <div class="mt-10 grid max-w-2xl grid-cols-3 gap-3">
            <div v-for="feature in features" :key="feature.title" class="rounded-2xl border border-white/[0.07] bg-white/[0.035] p-4 backdrop-blur-sm">
              <div class="mb-3 flex h-9 w-9 items-center justify-center rounded-xl bg-white/[0.06] text-indigo-200">
                <n-icon size="18" :component="feature.icon" />
              </div>
              <div class="text-sm font-bold text-slate-200">{{ feature.title }}</div>
              <div class="mt-1 text-xs leading-5 text-slate-500">{{ feature.description }}</div>
            </div>
          </div>
        </div>

        <div class="text-xs text-slate-600">Easy Stream · 轻量媒体整理工作台</div>
      </section>

      <section class="flex min-h-screen items-center justify-center px-4 py-8 sm:px-8 lg:px-10">
        <div class="w-full max-w-[29rem]">
          <div class="mb-8 flex items-center gap-3 lg:hidden">
            <div class="flex h-11 w-11 items-center justify-center rounded-2xl bg-gradient-to-br from-indigo-400 via-indigo-500 to-cyan-500 shadow-lg shadow-indigo-950/50 ring-1 ring-white/20">
              <n-icon size="23" :component="PlayCircleOutline" />
            </div>
            <div>
              <div class="font-extrabold">Easy Stream</div>
              <div class="text-[10px] font-bold uppercase tracking-[0.2em] text-slate-500">Media Workspace</div>
            </div>
          </div>

          <div class="rounded-[1.75rem] border border-white/[0.09] bg-white/[0.055] p-6 shadow-2xl shadow-black/30 backdrop-blur-2xl sm:p-9">
            <div class="mb-8">
              <div class="text-xs font-extrabold uppercase tracking-[0.18em] text-indigo-300">Welcome back</div>
              <h2 class="mt-2 text-3xl font-black tracking-tight text-white">登录工作台</h2>
              <p class="mt-2 text-sm leading-6 text-slate-400">输入账号信息，继续管理媒体资源与整理任务。</p>
            </div>

            <n-form ref="formRef" :model="form" :rules="rules" :show-label="false" @keyup.enter="handleLogin">
              <n-form-item path="name">
                <n-input v-model:value="form.name" size="large" placeholder="用户名" :input-props="{ autocomplete: 'username' }">
                  <template #prefix><n-icon :component="PersonOutline" /></template>
                </n-input>
              </n-form-item>
              <n-form-item path="password">
                <n-input v-model:value="form.password" type="password" size="large" show-password-on="click" placeholder="密码" :input-props="{ autocomplete: 'current-password' }">
                  <template #prefix><n-icon :component="LockClosedOutline" /></template>
                </n-input>
              </n-form-item>
              <n-button type="primary" size="large" block :loading="loading" class="mt-2 !h-12 !rounded-xl" @click="handleLogin">
                进入工作台
              </n-button>
            </n-form>

            <div class="mt-7 flex items-center gap-3 text-[11px] text-slate-600">
              <span class="h-px flex-1 bg-white/[0.07]"></span>
              <span>MEDIA OPERATIONS CONSOLE</span>
              <span class="h-px flex-1 bg-white/[0.07]"></span>
            </div>
          </div>
        </div>
      </section>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { NForm, NFormItem, NInput, NButton, NIcon, useMessage } from 'naive-ui'
import { PersonOutline, LockClosedOutline, PlayCircleOutline, FilmOutline, CloudOutline, FlashOutline } from '@vicons/ionicons5'
import md5 from 'crypto-js/md5'
import { login } from '../utils/api/auth'

const message = useMessage()
const formRef = ref(null)
const loading = ref(false)
const features = [
  { title: '统一资产', description: '媒体来源与台账集中管理', icon: FilmOutline },
  { title: '云端协同', description: '本地与 115 资源无缝衔接', icon: CloudOutline },
  { title: '自动流程', description: '识别、整理和 STRM 串联', icon: FlashOutline }
]
const form = ref({
  name: '',
  password: ''
})

const rules = {
  name: [{ required: true, message: '请输入用户名', trigger: 'blur' }],
  // 仅做非空校验，避免把后端真实默认密码误判为无效
  password: [{ required: true, message: '请输入密码', trigger: 'blur' }]
}

const handleLogin = () => {
  formRef.value?.validate((errors) => {
    if (errors) return
    loading.value = true

    const md5Password = md5(form.value.password).toString()

    login({
      name: form.value.name.trim(),
      password: md5Password
    }, {
      skipGlobalErrorMessage: true
    }).catch((error) => {
      const errorMsg = error.response?.data?.error || error.message || '登录失败，请检查用户名和密码'
      message.error(errorMsg)
    }).finally(() => {
      loading.value = false
    })
  })
}
</script>
