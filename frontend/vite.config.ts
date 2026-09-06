/// <reference types="vitest" />
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'
import {fileURLToPath, URL} from 'node:url'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: [
      {
        find: '@opscopilot/shell-terminal/ui/styles.css',
        replacement: fileURLToPath(new URL('../frontend-shell/src/ui/styles.css', import.meta.url)),
      },
      {
        find: '@opscopilot/shell-terminal/ui',
        replacement: fileURLToPath(new URL('../frontend-shell/src/ui/index.ts', import.meta.url)),
      },
      // 把 react/react-dom 钉到本目录副本：frontend-shell/node_modules 里存在
      // 另一份 react，vitest 下 flexlayout-react 等跨目录依赖会解析到它，
      // 产生双 React 实例导致 "Invalid hook call"。与下方 dedupe 语义一致，
      // 显式 alias 保证 vitest 与构建行为相同。
      {
        find: /^react$/,
        replacement: fileURLToPath(new URL('./node_modules/react', import.meta.url)),
      },
      {
        find: /^react-dom$/,
        replacement: fileURLToPath(new URL('./node_modules/react-dom', import.meta.url)),
      },
      {
        find: /^react-dom\/client$/,
        replacement: fileURLToPath(new URL('./node_modules/react-dom/client.js', import.meta.url)),
      },
    ],
    dedupe: ['react', 'react-dom', '@xterm/xterm', '@xterm/addon-fit', '@xterm/addon-search'],
  },
  // 固定 dev 端口 5174，避免与占用默认 5173 的其他项目（如 DTS-DEV）冲突，
  // 配合 wails.json frontend:dev:serverUrl 使用。
  server: {
    port: 5174,
    strictPort: true,
  },
  // @xterm/xterm 6 can stop parsing full-screen TUI control sequences after
  // esbuild minification in the packaged WebView2 runtime. The same production
  // build works reliably when its control flow is left intact (verified with vi).
  build: {
    minify: false,
  },
  test: {
    css: { include: [/App\.css/] },
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
    // flexlayout-react 会被 vitest 默认 externalize，其内部 import 'react'
    // 走 node 解析时命中 frontend-shell/node_modules 的另一份 react，
    // 导致双 React 实例。内联后经由上方 alias 归一到本目录副本。
    server: {
      deps: {
        inline: ['flexlayout-react'],
      },
    },
  },
} as any)
