// Run against an isolated desktop preview. No production connection or model
// credentials are read or logged; the runtime check sends no model prompt.
import assert from 'node:assert/strict';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { chromium, expect } from '@playwright/test';

const executable = process.argv.find(arg => arg.startsWith('--icode='))?.slice('--icode='.length);
const url = process.argv.find(arg => arg.startsWith('--url='))?.slice('--url='.length) || 'http://127.0.0.1:34185/';
assert.ok(executable && path.isAbsolute(executable), 'Provide --icode=<absolute path>');
const directory = path.resolve('e2e/.runtime', `runtime-settings-${Date.now()}`);
await mkdir(directory, { recursive: true });
const browser = await chromium.launch({ channel: 'msedge', headless: true });
const page = await browser.newPage({ viewport: { width: 1180, height: 900 } });
const errors = [];
page.on('pageerror', error => errors.push(error.message));
const checks = {};
async function openSettings() {
    await page.getByTitle('设置', { exact: true }).click();
    await page.locator('nav span').filter({ hasText: /^AI 接入$/ }).click();
    await expect(page.getByLabel('程序路径', { exact: true })).toBeVisible();
}
try {
    await page.goto(url);
    await page.waitForFunction(() => !!window.go?.main?.App?.CheckAssistantRuntime);
    await openSettings();
    const panel = page.locator('.oc-runtime-settings');
    const input = page.getByLabel('程序路径', { exact: true });
    await input.fill('C:/not-installed-opscopilot-test/icode.exe');
    await panel.getByRole('button', { name: '检测环境', exact: true }).click();
    await expect(panel.getByText('运行环境未就绪', { exact: true })).toBeVisible();
    await expect(panel.getByText('找不到指定的 iCode 程序，请检查运行组件路径', { exact: true })).toBeVisible();
    checks.missingPathHasActionableFailure = true;
    await page.screenshot({ path: path.join(directory, 'missing-program.png') });
    await input.fill(executable);
    await panel.getByRole('button', { name: '检测环境', exact: true }).click();
    await expect(input).toBeDisabled();
    await expect(panel.getByText('运行环境可用', { exact: true })).toBeVisible({ timeout: 55000 });
    await expect(panel.locator('.oc-runtime-checks .is-passed')).toHaveCount(6);
    assert.equal((await page.evaluate(() => window.go.main.App.GetSettings())).assistant_runtime.executable, '');
    checks.realRuntimeStartupAndAllSixChecks = true;
    checks.checkDoesNotSaveDraft = true;
    await page.screenshot({ path: path.join(directory, 'runtime-ready.png') });
    await page.getByRole('button', { name: '保存更改', exact: true }).click();
    await expect.poll(async () => (await page.evaluate(() => window.go.main.App.GetSettings())).assistant_runtime.executable).toBe(executable);
    checks.settingsSaveRuntimePath = true;
    await page.reload();
    await page.waitForFunction(() => !!window.go?.main?.App?.CheckAssistantRuntime);
    await page.getByTitle('设置', { exact: true }).click();
    await page.getByPlaceholder('搜索设置...').fill('icode');
    await page.locator('nav span').filter({ hasText: /^AI 接入$/ }).click();
    await expect(page.getByLabel('程序路径', { exact: true })).toHaveValue(executable);
    checks.reloadPreservesPathAndSearchFindsSettings = true;
    await page.getByPlaceholder('搜索设置...').fill('');
    await page.setViewportSize({ width: 850, height: 760 });
    const widths = await page.locator('.oc-runtime-path').evaluate(element => ({ client: element.clientWidth, scroll: element.scrollWidth }));
    assert.ok(widths.scroll <= widths.client + 1);
    checks.narrowLayoutHasNoPathOverflow = true;
    await page.screenshot({ path: path.join(directory, 'runtime-narrow.png') });
    await page.setViewportSize({ width: 1180, height: 900 });
    await page.getByRole('button', { name: '关闭系统设置', exact: true }).click();
    assert.deepEqual(errors, []);
    await writeFile(path.join(directory, 'result.json'), JSON.stringify({ checks, errors, directory }, null, 2));
    console.log('RUNTIME_SETTINGS_PASSED', JSON.stringify(checks));
    console.log('EVIDENCE', directory);
} catch (error) {
    await page.screenshot({ path: path.join(directory, 'failed.png') });
    await writeFile(path.join(directory, 'failure.json'), JSON.stringify({ message: String(error), errors }, null, 2));
    throw error;
} finally { await browser.close(); }
