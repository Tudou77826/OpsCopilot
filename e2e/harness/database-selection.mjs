// Selection rendering regression against the running local product. No SQL is executed.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const evidence = fileURLToPath(new URL('../.runtime/database-workspace/evidence/', import.meta.url));
await mkdir(evidence, { recursive: true });
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1100, height: 833 } });
const sessions = [];
const checks = [];
try {
  await page.goto('http://127.0.0.1:34115/');
  await page.waitForFunction(() => window.go?.main?.App?.DatabaseConnect);
  await page.exposeFunction('recordSelectionSession', id => sessions.push(id));
  await page.evaluate(() => {
    const original = window.go.main.App.DatabaseConnect;
    window.go.main.App.DatabaseConnect = async (...args) => {
      const session = await original(...args);
      await window.recordSelectionSession(session.id);
      return session;
    };
  });
  await page.getByRole('button', { name: '数据库', exact: true }).click();
  await page.locator('.oc-db-connection').filter({ hasText: '本地 MySQL · 体验库' }).dblclick();
  const editor = page.getByRole('textbox', { name: 'SQL 编辑器', exact: true });
  await expect(editor).toBeVisible();
  await editor.fill('SELECT\n  1\n  AS\n  connection_still_active;');
  for (const theme of ['dark', 'light']) {
    // DOM-only theme override avoids changing the user's saved appearance preference.
    await page.evaluate(theme => { document.documentElement.dataset.theme = theme; }, theme);
    for (const direction of ['forward', 'backward']) {
      await editor.press(direction === 'forward' ? 'Control+Home' : 'Control+End');
      await editor.press(direction === 'forward' ? 'Control+Shift+End' : 'Control+Shift+Home');
      const selection = page.locator('.cm-selectionBackground');
      await expect(selection.first()).toBeVisible();
      await expect.poll(() => editor.evaluate(() => window.getSelection()?.toString())).toBe('SELECT\n  1\n  AS\n  connection_still_active;');
      const activeColor = await page.locator('.cm-activeLine').evaluate(line => getComputedStyle(line).backgroundColor);
      expect(activeColor).toBe('rgba(0, 0, 0, 0)');
      const colors = await selection.evaluateAll(markers => markers.map(marker => getComputedStyle(marker).backgroundColor));
      expect(new Set(colors).size).toBe(1);
      expect(colors[0]).not.toBe('rgb(215, 212, 240)');
      await page.screenshot({ path: `${evidence}/selection-${theme}-${direction}.png` });
      checks.push({ theme, direction, activeColor, selectionColor: colors[0] });
    }
    await editor.press('ArrowRight');
    const cursorLineColor = await page.locator('.cm-activeLine').evaluate(line => getComputedStyle(line).backgroundColor);
    expect(cursorLineColor).not.toBe('rgba(0, 0, 0, 0)');
    await editor.press('Control+End');
    await editor.press('Control+Shift+ArrowLeft');
    await expect.poll(() => page.locator('.cm-activeLine').evaluate(line => getComputedStyle(line).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  }
  await page.getByRole('button', { name: '新建 SQL 查询', exact: true }).focus();
  await expect(page.locator('.cm-selectionBackground').first()).toBeVisible();
  await expect.poll(() => page.locator('.cm-activeLine').evaluate(line => getComputedStyle(line).backgroundColor)).toBe('rgba(0, 0, 0, 0)');
  await writeFile(`${evidence}/selection-result.json`, JSON.stringify(checks, null, 2));
  console.log(JSON.stringify({ selectionCases: checks.length, partialSelection: true, blurredSelection: true, cursorLineRestored: true }));
} finally {
  for (const id of sessions) await page.evaluate(id => window.go.main.App.DatabaseDisconnect(id), id).catch(() => {});
  await browser.close();
}
