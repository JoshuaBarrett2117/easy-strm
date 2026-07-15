<template>
  <div class="relative flex min-h-screen items-center justify-center overflow-hidden bg-ink-950">
    <!-- 背景装饰 -->
    <div class="pointer-events-none absolute inset-0">
      <div class="absolute -left-32 -top-32 h-96 w-96 rounded-full bg-cyan-500/10 blur-3xl"></div>
      <div class="absolute -bottom-40 -right-32 h-[28rem] w-[28rem] rounded-full bg-indigo-500/10 blur-3xl"></div>
      <div
        class="absolute inset-0 opacity-[0.35]"
        style="background-image: linear-gradient(rgba(255,255,255,0.03) 1px, transparent 1px), linear-gradient(90deg, rgba(255,255,255,0.03) 1px, transparent 1px); background-size: 40px 40px;"
      ></div>
    </div>

    <!-- 登录卡片 -->
    <div class="relative z-10 w-full max-w-md px-4">
      <div class="rounded-2xl border border-white/10 bg-ink-900/90 p-8 shadow-2xl shadow-black/40 backdrop-blur-xl">
        <div class="mb-8 text-center">
          <div class="mx-auto mb-4 flex h-16 w-16 items-center justify-center rounded-2xl bg-gradient-to-br from-cyan-400 to-cyan-700 shadow-lg shadow-cyan-500/25">
            <n-icon size="32" color="#fff" :component="PlayCircleOutline" />
          </div>
          <h1 class="text-2xl font-bold text-white">Easy Stream</h1>
          <p class="mt-1 text-sm text-slate-400">影视资源管理平台</p>
        </div>

        <n-form ref="formRef" :model="form" :rules="rules" :show-label="false" @keyup.enter="handleLogin">
          <n-form-item path="name">
            <n-input
              v-model:value="form.name"
              size="large"
              placeholder="用户名"
              :input-props="{ autocomplete: 'username' }"
            >
              <template #prefix>
                <n-icon :component="PersonOutline" />
              </template>
            </n-input>
          </n-form-item>
          <n-form-item path="password">
            <n-input
              v-model:value="form.password"
              type="password"
              size="large"
              show-password-on="click"
              placeholder="密码"
              :input-props="{ autocomplete: 'current-password' }"
            >
              <template #prefix>
                <n-icon :component="LockClosedOutline" />
              </template>
            </n-input>
          </n-form-item>
          <n-button
            type="primary"
            size="large"
            block
            :loading="loading"
            class="mt-2"
            @click="handleLogin"
          >
            登录
          </n-button>
        </n-form>

        <p class="mt-6 text-center text-xs text-slate-500">安全登录 · 数据加密</p>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref } from 'vue'
import { NForm, NFormItem, NInput, NButton, NIcon, useMessage } from 'naive-ui'
import { PersonOutline, LockClosedOutline, PlayCircleOutline } from '@vicons/ionicons5'
import md5 from 'crypto-js/md5'
import { login } from '../utils/api/auth'

const message = useMessage()
const formRef = ref(null)
const loading = ref(false)
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
