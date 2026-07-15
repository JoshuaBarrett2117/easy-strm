import { createDiscreteApi, darkTheme } from 'naive-ui'
import { computed, ref } from 'vue'
import { themeOverridesDark, themeOverridesLight } from '../../theme'

/**
 * UI 反馈桥接层(基于 Naive UI 离散 API)
 * 供组件外部代码(如 axios 拦截器)调用 message / dialog / notification
 * 与主题状态联动,深浅色自动切换
 */

export const isDarkRef = ref(
  typeof document !== 'undefined' && document.documentElement.classList.contains('dark')
)

export const setDiscreteTheme = (isDark) => {
  isDarkRef.value = isDark
}

const configProviderProps = computed(() => ({
  theme: isDarkRef.value ? darkTheme : null,
  themeOverrides: isDarkRef.value ? themeOverridesDark : themeOverridesLight
}))

const { message, dialog, notification, loadingBar } = createDiscreteApi(
  ['message', 'dialog', 'notification', 'loadingBar'],
  { configProviderProps }
)

export { message, dialog, notification, loadingBar }
