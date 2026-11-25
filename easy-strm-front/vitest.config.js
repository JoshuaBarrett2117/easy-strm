import { defineConfig } from 'vitest/config'
import vue from '@vitejs/plugin-vue'

// 组件/工具单元测试（vitest + @vue/test-utils）；tests/unit/*.test.mjs 仍由 node --test 运行。
export default defineConfig({
  plugins: [vue()],
  test: {
    environment: 'jsdom',
    include: ['tests/vitest/**/*.spec.js'],
    css: false
  }
})
