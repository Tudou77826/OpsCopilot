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
    globals: true,
    environment: 'jsdom',
    setupFiles: './src/setupTests.ts',
  },
} as any)
