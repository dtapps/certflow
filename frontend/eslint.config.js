import { existsSync, readFileSync } from 'node:fs'
import js from '@eslint/js'
import tseslint from 'typescript-eslint'
import pluginVue from 'eslint-plugin-vue'
import vueTsEslintConfig from '@vue/eslint-config-typescript'

// unplugin-auto-import 生成的全局变量（ref/computed 等），首次 vite dev/build 后产生
let autoImportGlobals = {}
const autoImportEslintrc = './.eslintrc-auto-import.json'
if (existsSync(autoImportEslintrc)) {
  autoImportGlobals = JSON.parse(readFileSync(autoImportEslintrc, 'utf-8')).globals ?? {}
}

export default [
  {
    // 忽略自动生成/产物文件，避免无意义 lint 报错与格式化改动
    ignores: [
      'node_modules',
      'dist',
      'bindings',
      'src/auto-imports.d.ts',
      'src/components.d.ts',
    ],
  },
  js.configs.recommended,
  ...tseslint.configs.recommended,
  ...pluginVue.configs['flat/essential'],
  ...vueTsEslintConfig(),
  {
    languageOptions: {
      globals: autoImportGlobals,
    },
    rules: {
      'vue/multi-word-component-names': 'off',
      '@typescript-eslint/no-explicit-any': 'warn',
      '@typescript-eslint/no-unused-vars': ['warn', { argsIgnorePattern: '^_' }],
      '@typescript-eslint/ban-ts-comment': 'off',
    },
  },
]
