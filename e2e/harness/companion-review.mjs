// Visual checks against the real product page, shared by full acceptance and
// review of an already running local instance. No fake state or preview route.
import { chromium, expect } from '@playwright/test';
import { mkdir, writeFile } from 'node:fs/promises';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

export async function captureCompanionReview(page, evidence, appURL = 'http://127.0.0.1:34115/') {
  await mkdir(evidence, {recursive:true});
  const errors = [];
  const collect = error => errors.push(error.message);
  page.on('pageerror', collect);
  await page.goto(appURL);
  const entry = page.getByTestId('nav-icon-companion');
  await expect(entry).toBeVisible();
  await expect(page.locator('[title="AI 问答"]')).toHaveCount(0);
  expect(await entry.evaluate(element => element.nextElementSibling?.getAttribute('data-testid'))).toBe('nav-icon-quickcommands');
  await entry.click();
  await expect(page.getByRole('heading',{name:'伙伴助手',exact:true})).toBeVisible();
  const input = page.getByPlaceholder('输入问题…');
  const display = page.getByRole('region',{name:'伙伴展示',exact:true});
  await expect(display).toHaveAttribute('data-expanded','true');
  await expect(display.getByRole('progressbar',{name:'伙伴升级进度'})).toBeVisible();
  await expect(display.locator('.oc-assistant-companion-scene .oc-assistant-medals')).toHaveCount(1);
  await expect(display.locator('.oc-assistant-companion-fold svg')).toHaveCount(1);
  await expect(page.getByText('COMPANION ASSISTANT',{exact:true})).toHaveCount(0);
  await expect(page.getByText('和伙伴一起探索运维知识',{exact:true})).toHaveCount(0);
  const local = await page.evaluate(() => window.go.main.App.AchievementStatus());
  const definitions = await page.evaluate(() => window.go.main.App.AchievementCatalog());
  const earned = definitions.filter(item => local.summary.progress.find(record => record.kind === item.kind)?.medal);
  const weight = {bronze:0,silver:1,gold:2};
  const shown = local.selected.length
    ? local.selected.flatMap(kind => earned.filter(item => item.kind === kind)).slice(0,3)
    : [...earned].sort((a,b) => weight[local.summary.progress.find(record => record.kind === b.kind).medal.tier] - weight[local.summary.progress.find(record => record.kind === a.kind).medal.tier]).slice(0,3);
  await expect(display.locator('.oc-assistant-medal')).toHaveCount(shown.length);
  expect(shown.length).toBeLessThanOrEqual(3);
  expect(await display.locator('.oc-assistant-medal').evaluateAll(items => items.map(item=>item.dataset.kind))).toEqual(shown.map(item=>item.kind));
  const tiers = {bronze:'铜章',silver:'银章',gold:'金章'};
  for(const item of shown) {
    const record = local.summary.progress.find(record => record.kind === item.kind);
    const medal = display.locator(`[data-kind="${item.kind}"]`);
    const description = `${item.name} · ${tiers[record.medal.tier]} · ${record.count}${item.unit}`;
    await expect(medal).toHaveAttribute('data-earned','true');
    await expect(medal.getByRole('button',{name:description,exact:true})).toBeVisible();
    await expect(medal.getByRole('tooltip')).not.toBeVisible();
    await medal.getByRole('button').focus();
    await expect(medal.getByRole('tooltip')).toBeVisible();
    await input.focus();
  }
  const assistantSprite=display.locator('.oc-assistant-companion-scene .oc-pixel-sprite');
  await expect(assistantSprite).toHaveAttribute('data-motion','running');
  const samples=[];
  for(let index=0;index<10;index++) { samples.push(await assistantSprite.evaluate(image=>({pose:image.src,translate:getComputedStyle(image).translate,scale:getComputedStyle(image).scale}))); await page.waitForTimeout(100); }
  expect(new Set(samples.map(sample=>sample.pose)).size).toBeLessThanOrEqual(4);
  expect(new Set(samples.map(sample=>sample.translate+sample.scale)).size).toBeGreaterThan(3);
  await input.fill('收起伙伴后仍在的草稿');
  const expandedHeight = await display.evaluate(element => element.getBoundingClientRect().height);
  const initialChatHeight = await page.locator('.ai-chat-panel').evaluate(element => element.getBoundingClientRect().height);
  await page.getByRole('button',{name:'收起伙伴展示',exact:true}).click();
  await expect(display).toHaveAttribute('data-expanded','false');
  await expect(display.locator('.oc-assistant-medal')).toHaveCount(0);
  await expect(input).toHaveValue('收起伙伴后仍在的草稿');
  expect(await display.evaluate(element => element.getBoundingClientRect().height)).toBeLessThan(expandedHeight-150);
  expect(await page.locator('.ai-chat-panel').evaluate(element => element.getBoundingClientRect().height)).toBeGreaterThan(initialChatHeight+150);
  await page.screenshot({path:path.join(evidence,'companion-assistant-collapsed.png')});
  await page.getByRole('button',{name:'展开伙伴展示',exact:true}).click();
  await expect(display.locator('.oc-assistant-medal')).toHaveCount(shown.length);
  await input.fill('伙伴入口的真实问答验证');
  await page.locator('.ai-chat-panel').getByTitle('发送',{exact:true}).click();
  await expect(page.locator('.ai-assistant-message')).toContainText('Error:',{timeout:20000});
  await expect(input).toBeEnabled();
  await page.getByTitle('知识库',{exact:true}).click();
  await entry.click();
  await expect(page.locator('.ai-user-message')).toContainText('伙伴入口的真实问答验证');
  await input.fill('切换入口后保留的草稿');
  await entry.click();
  await entry.click();
  await expect(input).toHaveValue('切换入口后保留的草稿');
  for(const width of [1440,960,794]) {
    await page.setViewportSize({width,height:width === 794 ? 833 : 1000});
    await expect(input).toBeVisible();
    const layout = await display.evaluate(element => {
      const outer=element.getBoundingClientRect(), medals=element.querySelector('.oc-assistant-medals').getBoundingClientRect(), scene=element.querySelector('.oc-assistant-companion-scene').getBoundingClientRect(), title=element.querySelector('.oc-assistant-companion-title').getBoundingClientRect();
      return {width:element.clientWidth,scroll:element.scrollWidth,allMedalsInView:medals.bottom<=scene.bottom && medals.left>=scene.left && medals.right<=scene.right,medalsWidth:medals.width,sceneHeight:scene.height,titleHeight:title.height,totalHeight:outer.height,medalsRight:scene.right-medals.right,vertical:[...element.querySelectorAll('.oc-assistant-medal')].map(item=>({x:item.offsetLeft,y:item.offsetTop}))};
    });
    expect(layout.scroll).toBeLessThanOrEqual(layout.width+1);
    expect(layout.allMedalsInView).toBe(true);
    expect(layout.medalsWidth).toBeLessThanOrEqual(48);
    expect(layout.medalsRight).toBeLessThanOrEqual(16);
    expect(layout.sceneHeight).toBeGreaterThanOrEqual(260);
    for(let index=1;index<layout.vertical.length;index++) { expect(layout.vertical[index].x).toBe(layout.vertical[0].x); expect(layout.vertical[index].y).toBeGreaterThan(layout.vertical[index-1].y); }
    expect(layout.titleHeight).toBeLessThanOrEqual(44);
    expect(layout.totalHeight).toBeGreaterThanOrEqual(300);
    expect(layout.totalHeight).toBeLessThanOrEqual(385);
    await page.screenshot({path:path.join(evidence,`companion-assistant-${width}.png`)});
  }
  await page.setViewportSize({width:1440,height:1000});
  await page.getByRole('button',{name:'养成与奖章',exact:true}).click();
  await expect(page.getByRole('heading',{name:'伙伴与奖章',exact:true})).toBeVisible();
  const appearanceBefore=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
  await page.getByRole('button',{name:'修改名字',exact:true}).click();
  await expect(page.getByLabel('伙伴名称',{exact:true})).toHaveValue(appearanceBefore.pet.name);
  const scene = page.locator('.oc-companion-scene img');
  await expect.poll(() => scene.evaluate(image => image.complete && image.naturalWidth === 256)).toBe(true);
  const colors = await scene.evaluate(image => {
    const canvas = document.createElement('canvas'); canvas.width=256; canvas.height=256;
    const context = canvas.getContext('2d'); context.drawImage(image,0,0);
    const pixels=context.getImageData(0,0,256,256).data, values=new Set();let softAlpha=0,purple=0;
    for(let i=0;i<pixels.length;i+=4) if(pixels[i+3]) {values.add((pixels[i]<<16)|(pixels[i+1]<<8)|pixels[i+2]);if(pixels[i+3]<255) softAlpha++;if(pixels[i+2]>pixels[i+1]+30&&pixels[i]>pixels[i+1]+20) purple++;}
    return {distinct:values.size,softAlpha,purple,rendering:getComputedStyle(image).imageRendering};
  });
  expect(colors.distinct).toBeGreaterThan(500);expect(colors.softAlpha).toBeGreaterThan(100);expect(colors.rendering).toBe('auto');
  await page.locator('.oc-companion-actions').getByRole('button',{name:'取消',exact:true}).click();
  expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(appearanceBefore.pet);
  await page.emulateMedia({reducedMotion:'reduce'});
  await page.getByRole('button',{name:'打个招呼',exact:true}).click();
  await expect(scene).toHaveAttribute('data-action','greet');
  await expect(scene).toHaveAttribute('data-motion','reduced');
  expect(await scene.evaluate(image=>getComputedStyle(image).animationName)).toBe('none');
  // Let the asynchronously recolored action frames settle, then sample across
  // multiple frame periods so a running animation cannot pass by coincidence.
  await expect.poll(async () => {
    const samples=[];
    for(let index=0;index<5;index++) { samples.push(await scene.getAttribute('src')); await page.waitForTimeout(170); }
    return new Set(samples).size === 1 && await scene.getAttribute('data-palette') === appearanceBefore.pet.paletteId && await scene.getAttribute('data-action') === 'greet';
  },{timeout:3500}).toBe(true);
  await page.emulateMedia({reducedMotion:'no-preference'});
  const results=[];
  for(const [theme,label] of [['light','亮色'],['dark','暗色']]) {
    await page.getByText('外观',{exact:true}).click();
    await page.getByRole('radio',{name:label,exact:true}).click();
    await page.getByRole('button',{name:'保存更改',exact:true}).click();
    await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
    for(const width of [1440,960]) {
      await page.setViewportSize({width,height:1000});
      await page.getByRole('heading',{name:'伙伴与奖章',exact:true}).scrollIntoViewIfNeeded();
      await expect(scene).toBeVisible();
      const bounds = await page.locator('.oc-companion-home').evaluate(element => ({client:element.clientWidth,scroll:element.scrollWidth}));
      expect(bounds.scroll).toBeLessThanOrEqual(bounds.client+1);
      const broken = await page.locator('.oc-companion-page img').evaluateAll(images => images.filter(image=>!image.complete || !image.naturalWidth).length);
      expect(broken).toBe(0);
      await page.screenshot({path:path.join(evidence,`companion-${theme}-${width}.png`)});
      results.push({theme,width,overflow:false,brokenImages:broken});
    }
  }
  await page.setViewportSize({width:1440,height:1000});
  await page.getByText('外观',{exact:true}).click();
  await page.getByRole('radio',{name:'亮色',exact:true}).click();
  await page.getByRole('button',{name:'保存更改',exact:true}).click();
  await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
  await page.getByLabel('分类榜').selectOption('script-created');
  await expect(page.locator('.oc-achievements-table tbody tr')).toHaveCount(2);
  await page.locator('.oc-achievements-table').getByRole('button',{name:'同组织实测成员',exact:true}).click();
  await expect(page.getByRole('region',{name:'成员资料'})).toContainText('同伴琥珀');
  await page.getByRole('region',{name:'成员资料'}).scrollIntoViewIfNeeded();
  await page.screenshot({path:path.join(evidence,'companion-other-member.png')});
  await page.getByRole('heading',{name:'伙伴与奖章',exact:true}).scrollIntoViewIfNeeded();
  await page.getByRole('button',{name:'关闭',exact:true}).click();
  await expect(input).toHaveValue('切换入口后保留的草稿');
  await expect(page.locator('.ai-user-message')).toContainText('伙伴入口的真实问答验证');
  await page.getByRole('button',{name:'+ 新建对话',exact:true}).click();
  await expect(page.locator('.ai-user-message')).toHaveCount(0);
  await expect(input).toHaveValue('');
  await expect(page.locator('.oc-companion-assistant-header strong')).toHaveText('本地微光二号');
  await page.screenshot({path:path.join(evidence,'companion-assistant.png')});
  await page.setViewportSize({width:794,height:833});
  await page.screenshot({path:path.join(evidence,'companion-assistant-narrow.png')});
  const videoContext=await page.context().browser().newContext({viewport:{width:794,height:833},recordVideo:{dir:evidence,size:{width:794,height:833}}});
  const videoPage=await videoContext.newPage();
  const video=videoPage.video();
  await videoPage.goto(appURL);
  await videoPage.getByTestId('nav-icon-companion').click();
  await expect(videoPage.locator('.oc-assistant-companion-scene .oc-pixel-sprite')).toHaveAttribute('data-motion','running');
  await videoPage.waitForTimeout(5500);
  await videoContext.close();
  await video.saveAs(path.join(evidence,'companion-motion.webm'));
  expect(errors).toEqual([]);
  page.off('pageerror',collect);
  await writeFile(path.join(evidence,'visual-result.json'),JSON.stringify({testedAt:new Date().toISOString(),results,originalPalettePreserved:true,reducedMotion:true,sidebarEntry:true,quickCommandAdjacent:true,unifiedAssistant:true,largePartner:true,directMedals:true,compactShowcase:true,medalsOnBackground:true,rightVerticalMedals:true,expandedScene:true,continuousMotion:true,integratedIdentityAndXP:true,iconFold:true,promotionRemoved:true,keyboardMedalDetails:true,collapsibleDisplay:true,commentViewport794x833:true,realUnconfiguredAIResponse:true,conversationAndDraftPreserved:true,otherMemberProfile:true,browserErrors:errors},null,2));
}

if(process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const browser=await chromium.launch({headless:true,channel:process.env.E2E_BROWSER_CHANNEL || 'msedge'});
  try { const page=await browser.newPage({viewport:{width:1440,height:1000}}); await captureCompanionReview(page,path.resolve(process.argv[2])); }
  finally {await browser.close();}
}
