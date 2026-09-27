import { defineConfig } from '@playwright/test';

// E2E 配置：被测对象是 harness/launch.mjs 拉起的完整应用
// （frontend/dist 构建产物 + dev 版 Go exe 自托管 web 界面 + 双 fakessh 端点）。
//
// 浏览器用系统安装的 Chrome（channel），免 Playwright Chromium 下载；
// CI 或无 Chrome 环境可设 E2E_BROWSER_CHANNEL=chromium 换回内置浏览器。
export default defineConfig({
  testDir: './golden',
  timeout: 120_000,
  expect: { timeout: 10_000 },
  // 黄金用例是有状态的长流程（建连接、改落盘），且共享一个应用实例，串行执行
  workers: 1,
  fullyParallel: false,
  retries: 0,
  reporter: [['list']],
  use: {
    channel: process.env.E2E_BROWSER_CHANNEL || 'chrome',
    baseURL: 'http://127.0.0.1:34315',
    viewport: { width: 1280, height: 720 },
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
    actionTimeout: 15_000,
  },
  webServer: {
    command: 'node harness/launch.mjs',
    url: 'http://127.0.0.1:34315/',
    reuseExistingServer: false,
    // 首次运行含 go build（冷缓存可能较久）；E2E_SKIP_BUILD=1 可跳过构建加速
    timeout: 240_000,
    stdout: 'pipe',
    stderr: 'pipe',
  },
});
