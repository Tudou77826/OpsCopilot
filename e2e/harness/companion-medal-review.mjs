// Inspect real stored medals and responsive presentation without writing preferences.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
const evidence=path.resolve(process.argv[2]??'e2e/.runtime/medal-review');
await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true});
const page=await browser.newPage({viewport:{width:1041,height:833}}), errors=[];
page.on('pageerror',error=>errors.push(error.message));
try {
  await page.goto('http://127.0.0.1:34115/');
  const original=await page.evaluate(async()=>({pet:(await window.go.main.App.CompanionSnapshot()).pet,selected:(await window.go.main.App.AchievementStatus()).selected}));
  await page.getByTestId('nav-icon-companion').click();
  const medals=page.locator('.oc-assistant-medal img');
  await expect.poll(()=>medals.count()).toBeGreaterThan(0);
  await expect.poll(()=>medals.evaluateAll(images=>images.every(image=>image.complete&&image.naturalWidth===128))).toBe(true);
  const measurements=[];
  for(const width of [1041,794]) {
    await page.setViewportSize({width,height:833});
    const layout=await medals.evaluateAll(images=>images.map(image=>{
      const rect=image.getBoundingClientRect(),scene=image.closest('.oc-assistant-companion-scene').getBoundingClientRect();
      return {width:rect.width,x:rect.x,y:rect.y,right:scene.right-rect.right,bottom:scene.bottom-rect.bottom,rendering:getComputedStyle(image).imageRendering,native:image.naturalWidth};
    }));
    expect(new Set(layout.map(item=>item.x)).size).toBe(1);
    for(const item of layout) { expect(item.width).toBe(48); expect(item.native).toBe(128); expect(item.rendering).toBe('auto'); expect(item.right).toBeGreaterThanOrEqual(8); expect(item.bottom).toBeGreaterThan(0); }
    measurements.push({width,medals:layout});
    await page.getByPlaceholder('输入问题…').focus();
    await page.screenshot({path:path.join(evidence,`assistant-${width}.png`)});
    if(width===1041) await page.getByRole('region',{name:'伙伴展示',exact:true}).screenshot({path:path.join(evidence,'medal-showcase.png')});
  }
  const button=page.locator('.oc-assistant-medal > button').first(),tooltip=page.locator('.oc-assistant-medal-caption').first();
  await button.focus(); await expect(tooltip).toBeVisible();
  await page.screenshot({path:path.join(evidence,'medal-detail.png')});
  await page.emulateMedia({reducedMotion:'reduce'});
  await expect.poll(()=>medals.first().evaluate(image=>getComputedStyle(image).transform)).toBe('none');
  await page.getByRole('button',{name:'养成与奖章',exact:true}).click();
  await expect(page.locator('.oc-achievements-medal')).toHaveCount(8);
  await expect.poll(()=>page.locator('.oc-achievements-medal img').evaluateAll(images=>images.every(image=>image.complete&&image.naturalWidth===128))).toBe(true);
  for(const width of [1351,960]) {
    await page.setViewportSize({width,height:833});
    const layout=await page.locator('.oc-companion-page').evaluate(element=>({height:element.getBoundingClientRect().height,overflow:element.scrollWidth>element.clientWidth+1}));
    expect(layout.height).toBeLessThan(700); expect(layout.overflow).toBe(false);
    measurements.push({width,settings:layout});
    await page.screenshot({path:path.join(evidence,`settings-${width}.png`)});
  }
  const after=await page.evaluate(async()=>({pet:(await window.go.main.App.CompanionSnapshot()).pet,selected:(await window.go.main.App.AchievementStatus()).selected}));
  expect(after).toEqual(original); expect(errors).toEqual([]);
  await writeFile(path.join(evidence,'result.json'),JSON.stringify({measurements,keyboardDetails:true,reducedMotion:true,preservedPreferences:true,errors},null,2));
  console.log(JSON.stringify({measurements,preservedPreferences:true,errors}));
} finally { await browser.close(); }
