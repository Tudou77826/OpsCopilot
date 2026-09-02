import { defineConfig } from 'vitest/config';

export default defineConfig({
  test: {
    // jsdom：共享 UI 的 hook/组件测试（renderHook/@testing-library/react）需要 DOM；
    // 原 core 协议测试在 jsdom 下同样可运行（WebSocket 由测试 mock 全局覆盖）。
    // 阶段 4 起 .tsx 组件测试（FilesPanel 等）纳入质量门。
    environment: 'jsdom',
    css: { include: [/shell-theme\.css/] },
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: './src/setupTests.ts',
  },
});
