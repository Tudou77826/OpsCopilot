import { chromium, expect } from '@playwright/test';
import { mkdir } from 'node:fs/promises';
import path from 'node:path';

const evidence = path.resolve(process.argv[2] ?? 'e2e/.runtime/companion-settings-visibility');
await mkdir(evidence, { recursive: true });
const browser = await chromium.launch({ headless: true, channel: 'msedge' });
const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
try {
  await page.goto('http://127.0.0.1:34115/');
  await page.waitForFunction(() => !!window.go?.main?.App?.CompanionSnapshot);
  await page.getByRole('button', { name: '设置', exact: true }).click();
  await page.getByRole('navigation').getByText('伙伴与奖章', { exact: true }).click();
  const title = page.getByRole('heading', { name: '伙伴与奖章', exact: true });
  await expect(title).toBeVisible();
  await expect(page.getByRole('region', { name: '我的伙伴' })).toBeVisible();
  await expect(page.getByRole('heading', { name: /^功能奖章/ })).toBeVisible();
  await expect(page.getByText('伙伴人设')).toHaveCount(0);
  await expect(page.getByLabel('人设提示词')).toHaveCount(0);
  await expect(page.getByRole('button', { name: '保存更改' })).toHaveCount(0);

  for (const theme of ['light', 'dark']) {
    await page.evaluate(value => { document.documentElement.dataset.theme = value; }, theme);
    await page.screenshot({ path: path.join(evidence, `companion-settings-${theme}.png`) });
  }
  console.log('Companion settings page passed in both themes');
} finally {
  await browser.close();
}
