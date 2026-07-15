import { computed, ref } from 'vue'
import { setDiscreteTheme } from '../utils/ui/feedback'

/**
 * 全局主题状态(深浅双主题)
 * 单例共享:所有组件引用同一个 ref
 */

const isDark = ref(
  typeof document !== 'undefined'
    ? (localStorage.getItem('theme') ?? 'dark') === 'dark'
    : true
)

const applyTheme = () => {
  document.documentElement.classList.toggle('dark', isDark.value)
  setDiscreteTheme(isDark.value)
}

if (typeof document !== 'undefined') {
  applyTheme()
}

export const useTheme = () => {
  const toggleTheme = () => {
    isDark.value = !isDark.value
    localStorage.setItem('theme', isDark.value ? 'dark' : 'light')
    applyTheme()
  }

  return {
    isDark: computed(() => isDark.value),
    toggleTheme
  }
}
