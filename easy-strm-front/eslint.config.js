// ESLint flat config：规则从宽，先保证 0 error；风格类规则交给后续逐步收紧。
import js from '@eslint/js'
import pluginVue from 'eslint-plugin-vue'
import globals from 'globals'

export default [
  {
    ignores: ['dist/**', 'node_modules/**', 'public/**', 'coverage/**']
  },
  js.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  {
    files: ['**/*.{js,mjs,vue}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: {
        ...globals.browser
      }
    },
    rules: {
      'no-unused-vars': ['warn', { args: 'none', caughtErrors: 'none', ignoreRestSiblings: true }],
      'no-empty': ['warn', { allowEmptyCatch: true }],
      'no-useless-escape': 'warn',
      'no-prototype-builtins': 'warn',
      'no-control-regex': 'warn',
      'no-async-promise-executor': 'warn',
      'no-case-declarations': 'warn',
      'no-useless-assignment': 'warn',
      'preserve-caught-error': 'off',
      'vue/multi-word-component-names': 'off',
      'vue/no-mutating-props': 'warn',
      'vue/no-unused-vars': 'warn',
      'vue/no-v-text-v-html-on-component': 'warn',
      'vue/valid-v-slot': ['error', { allowModifiers: true }]
    }
  },
  {
    // 构建配置、Node 端测试与 E2E 脚本运行在 Node 环境
    files: ['vite.config.js', 'eslint.config.js', 'vitest.config.js', 'tests/**/*.{js,mjs}'],
    languageOptions: {
      globals: {
        ...globals.node
      }
    }
  }
]
