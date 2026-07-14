/// <reference types="vitest" />
import {defineConfig} from 'vite'
import react from '@vitejs/plugin-react'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [react()],
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
