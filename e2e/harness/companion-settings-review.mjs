// Review the real settings page without changing stored preferences or records.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';

const evidence = path.resolve(process.argv[2] ?? 'e2e/.runtime/settings-density');
const baseline = process.argv.includes('--baseline');
await mkdir(evidence, { recursive: true });
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1351, height: 833 } });
const errors = [];
page.on('pageerror', error => errors.push(error.message));
try {
  await page.goto('http://127.0.0.1:34115/');
  await page.getByTestId('nav-icon-companion').click();
  await page.getByRole('button', { name: '养成与奖章', exact: true }).click();
  await expect(page.getByRole('heading', { name: '伙伴与奖章', exact: true })).toBeVisible();
  await expect(page.locator('.oc-achievements-medal')).toHaveCount(8);
  await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(1);
  const measurements = [];
  for (const width of [1351, 960]) {
    await page.setViewportSize({ width, height: 833 });
    const metrics = await page.locator('.oc-companion-page').evaluate(element => {
      let scroller = element.parentElement;
      while (scroller && !['auto', 'scroll'].includes(getComputedStyle(scroller).overflowY)) scroller = scroller.parentElement;
      const grid = element.querySelector('.oc-achievements-grid');
      return {
        pageHeight: Math.ceil(element.getBoundingClientRect().height),
        availableHeight: scroller?.clientHeight,
        scrollHeight: scroller?.scrollHeight,
        horizontalOverflow: element.scrollWidth > element.clientWidth + 1,
        medalColumns: getComputedStyle(grid).gridTemplateColumns.split(' ').length,
        sections: [...element.querySelectorAll('.oc-companion-home, .oc-achievements-card')].map(section => ({ class: section.className, height: Math.ceil(section.getBoundingClientRect().height) })),
      };
    });
    if (!baseline) {
      expect(metrics.horizontalOverflow).toBe(false);
      expect(metrics.pageHeight).toBeLessThan(700);
      if (width === 1351) {
        expect(metrics.medalColumns).toBe(4);
        expect(metrics.scrollHeight).toBeLessThanOrEqual(metrics.availableHeight + 1);
      }
    }
    measurements.push({ width, ...metrics });
    await page.screenshot({ path: path.join(evidence, `settings-${width}.png`) });
  }
  if (!baseline) {
    await page.setViewportSize({ width: 1351, height: 833 });
    await page.getByRole('button', { name: '修改名字', exact: true }).click();
    await expect(page.getByLabel('伙伴名称', { exact: true })).toHaveValue('本地微光二号');
    await page.getByRole('button', { name: '取消', exact: true }).click();
    await page.getByText('开蛋与成长规则', { exact: true }).click();
    await expect(page.getByText(/每日总上限 12/)).toBeVisible();
    await page.getByText('开蛋与成长规则', { exact: true }).click();
    await page.getByText('计数说明', { exact: true }).click();
    await expect(page.getByText('含有效结论的独立归档记录', { exact: true })).toBeVisible();
    await page.getByText('计数说明', { exact: true }).click();
    await page.getByLabel('分类榜').selectOption('script-created');
    await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(2);
    await page.locator('.oc-achievements-table').getByRole('button', { name: '同组织实测成员', exact: true }).click();
    await expect(page.getByRole('region', { name: '成员资料', exact: true })).toContainText('同伴琥珀');
    await page.getByRole('region', { name: '成员资料', exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidence, 'settings-profile.png') });
    await page.getByRole('button', { name: '收起资料', exact: true }).click();
    await page.getByLabel('分类榜').selectOption('knowledge-archive');
    await page.getByRole('heading', { name: '伙伴与奖章', exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: path.join(evidence, 'settings-1351.png') });
  }
  expect(errors).toEqual([]);
  await writeFile(path.join(evidence, 'result.json'), JSON.stringify({ baseline, measurements, errors }, null, 2));
  console.log(JSON.stringify(measurements));
} finally { await browser.close(); }
