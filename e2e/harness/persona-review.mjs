// Exercise persona settings through the real Wails product and preserve the prior preference.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
const evidence=path.resolve(process.argv[2] ?? 'e2e/.runtime/persona-review');
await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({headless:true,channel:'msedge'});
const page=await browser.newPage({viewport:{width:1351,height:833}});
const errors=[];page.on('pageerror',error=>errors.push(error.message));
let original;
try {
  await page.goto('http://127.0.0.1:34115/');
  original=await page.evaluate(()=>window.go.main.App.GetAssistantSettings());
  const configBefore=await page.evaluate(()=>window.go.main.App.GetSettings());
  await page.getByRole('button',{name:'设置',exact:true}).click();
  await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
  await expect(page.getByRole('navigation').getByText('伙伴人设',{exact:true})).toHaveCount(0);
  const input=page.getByLabel('人设提示词',{exact:true});
  await expect(input).toBeHidden();
  await page.screenshot({path:path.join(evidence,'persona-collapsed.png')});
  await page.locator('.oc-persona-disclosure > summary').click();
  await expect(input).toHaveValue(original.personaPrompt || original.defaultPrompt);
  const custom='你叫小圆，是耐心、可靠的运维伙伴。先用一句话给出结论，再简短解释原因。不要使用固定口头禅。';
  await input.fill(custom);
  expect((await page.evaluate(()=>window.go.main.App.GetAssistantSettings())).personaPrompt).toBe(original.personaPrompt);
  await page.getByRole('button',{name:'取消',exact:true}).click();
  await expect(page.getByText('有未保存的更改',{exact:true})).toBeVisible();
  await page.getByRole('button',{name:'继续编辑',exact:true}).click();
  await page.locator('.oc-persona-disclosure > summary').click();
  await expect(input).toBeHidden();
  await page.keyboard.press('Control+s');
  await expect(page.getByText('人设已保存，下次问答生效',{exact:true})).toBeVisible();
  expect((await page.evaluate(()=>window.go.main.App.GetAssistantSettings())).personaPrompt).toBe(custom);
  expect(await page.evaluate(()=>window.go.main.App.GetSettings())).toEqual(configBefore);
  await page.locator('.oc-persona-disclosure > summary').click();
  await page.screenshot({path:path.join(evidence,'persona-settings.png')});
  await page.reload();
  await page.getByRole('button',{name:'设置',exact:true}).click();
  await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
  await expect(input).toBeHidden();
  await page.locator('.oc-persona-disclosure > summary').click();
  await expect(input).toHaveValue(custom);
  await page.getByRole('button',{name:'恢复默认',exact:true}).click();
  await expect(input).toHaveValue(original.defaultPrompt);
  expect((await page.evaluate(()=>window.go.main.App.GetAssistantSettings())).personaPrompt).toBe(custom);
  await page.getByRole('button',{name:'保存更改',exact:true}).click();
  await expect(page.getByText('人设已保存，下次问答生效',{exact:true})).toBeVisible();
  expect((await page.evaluate(()=>window.go.main.App.GetAssistantSettings())).personaPrompt).toBe('');
  for(const width of [1351,960]) {
    await page.setViewportSize({width,height:833});
    const bounds=await page.getByRole('region',{name:'伙伴人设设置'}).evaluate(element=>({client:element.clientWidth,scroll:element.scrollWidth,bottom:element.getBoundingClientRect().bottom}));
    expect(bounds.scroll).toBeLessThanOrEqual(bounds.client+1);expect(bounds.bottom).toBeLessThan(750);
    await page.screenshot({path:path.join(evidence,`persona-default-${width}.png`)});
  }
  expect(errors).toEqual([]);
  await writeFile(path.join(evidence,'result.json'),JSON.stringify({saved:true,mergedNavigation:true,defaultCollapsed:true,collapsedDraftRetained:true,keyboardSave:true,reloadPersistence:true,defaultRestoration:true,noAutoSave:true,unsavedCloseProtection:true,unrelatedConfigPreserved:true,responsive:true,pageErrors:errors},null,2));
  console.log('Persona product settings review passed');
} finally {
  if(original) {
    await page.evaluate(async previous=>{const current=await window.go.main.App.GetAssistantSettings();await window.go.main.App.SaveAssistantSettings(previous.personaPrompt,current.revision);},original).catch(error=>{console.error('Preference restore failed:',error.message);process.exitCode=1;});
  }
  await browser.close();
}
