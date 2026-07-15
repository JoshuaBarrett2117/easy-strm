<template>
  <n-config-provider
    :theme="isDark ? darkTheme : null"
    :theme-overrides="isDark ? themeOverridesDark : themeOverridesLight"
    :locale="zhCN"
    :date-locale="dateZhCN"
    class="h-full"
  >
    <n-dialog-provider>
      <n-message-provider>
        <router-view />
      </n-message-provider>
    </n-dialog-provider>
  </n-config-provider>
</template>

<script setup>
import { NConfigProvider, NDialogProvider, NMessageProvider, darkTheme, zhCN, dateZhCN } from 'naive-ui'
import { useTheme } from './composables/useTheme'
import { themeOverridesDark, themeOverridesLight } from './theme'

const { isDark } = useTheme()
</script>
