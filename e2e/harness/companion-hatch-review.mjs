// Actual Wails UI/API. Existing review data is read only; paid draws run in an
// isolated test save with explicitly seeded XP and a separately owned process.
import {chromium, expect} from '@playwright/test';
import assert from 'node:assert/strict';
import {spawn, execFileSync} from 'node:child_process';
import {mkdir, readFile, writeFile, copyFile} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
import net from 'node:net';

const root = fileURLToPath(new URL('../../', import.meta.url));
const out = path.resolve(process.argv[2] ?? 'e2e/.runtime/companion-hatch-review');
const fixture = path.join(root, 'e2e/.runtime', `hatch-${Date.now()}`);
const appDir = path.join(fixture, 'app');
const exe = path.join(appDir, 'opscopilot-hatch.exe');
const url = 'http://127.0.0.1:34125/';
await mkdir(out, {recursive:true}); await mkdir(appDir, {recursive:true});
await new Promise((resolve,reject) => {
  const server=net.createServer(); server.once('error',reject);
  server.listen(34125,'127.0.0.1',()=>server.close(resolve));
});
await copyFile(path.join(root,'e2e/.runtime/social-1791625578908/app/opscopilot-social.exe'),exe);
const compatibility=JSON.parse(execFileSync(exe,['--plugin-info'],{windowsHide:true,encoding:'utf8'}));
assert.equal(compatibility.apiRevision,2); assert.ok(compatibility.capabilities.includes('companion.hatch.v1'));
await writeFile(path.join(appDir,'config.json'),JSON.stringify({experimental:{companion_enabled:true},appearance:{theme:'light'},docs:{dir:path.join(appDir,'docs')},scripts:{dir:path.join(appDir,'scripts')},log:{dir:path.join(appDir,'logs')},patch_store:{enabled:false},session_share:{enabled:false}}));
await writeFile(path.join(appDir,'service-center.json'),JSON.stringify({baseUrl:'',choice:'disabled'}));
await writeFile(path.join(appDir,'quick_commands.json'),'[]');
let child;
const logs=[];
function stop() {
  if(child && child.exitCode===null) execFileSync('taskkill',['/PID',String(child.pid),'/T','/F'],{windowsHide:true,stdio:'ignore'});
  child=undefined;
}
async function start() {
  child=spawn(exe,[],{cwd:fixture,windowsHide:true,stdio:['ignore','pipe','pipe'],env:{...process.env,devserver:'127.0.0.1:34125',assetdir:path.join(root,'frontend/dist')}});
  child.stdout.on('data',value=>logs.push(String(value))); child.stderr.on('data',value=>logs.push(String(value)));
  for(let i=0;i<100;i++) {
    if(child.exitCode!==null) throw new Error('Isolated Wails exited: '+logs.join('').slice(-2000));
    try {if((await fetch(url)).ok)return;}catch{}
    await new Promise(resolve=>setTimeout(resolve,200));
  }
  throw new Error('Isolated Wails did not start');
}
const browser=await chromium.launch({channel:'msedge',headless:true});
const page=await browser.newPage({viewport:{width:1113,height:833}});
const errors=[]; page.on('pageerror',error=>errors.push(error.message));
const go=(method,...args)=>page.evaluate(({method,args})=>window.go.main.App[method](...args),{method,args});
async function settings() {
  await page.getByRole('button',{name:'设置',exact:true}).click();
  await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
  await expect(page.locator('.oc-companion-home')).toBeVisible();
}
try {
  // Verify the real review instance without spending or renaming anything.
  await page.goto('http://127.0.0.1:34115/'); await page.waitForFunction(()=>window.go?.main?.App);
  const existing=await go('CompanionSnapshot');
  await settings();
  await expect(page.locator('.oc-companion-overview')).toContainText('Lv.'+existing.level);
  if (existing.pet) {
    const reroll=page.getByRole('button',{name:'重新开蛋',exact:true});
    await expect(reroll).toBeEnabled();
    expect(existing.hatchCost).toBe(50);
  } else {await expect(page.getByRole('button',{name:'免费开蛋',exact:true})).toBeVisible();}
  await expect(page.locator('.oc-companion-scene img')).toHaveAttribute('data-ready','true');
  const portrait=await page.locator('.oc-companion-scene img').evaluate(image=>{
    const canvas=document.createElement('canvas');canvas.width=image.naturalWidth;canvas.height=image.naturalHeight;
    const ctx=canvas.getContext('2d');ctx.drawImage(image,0,0);
    const pixels=ctx.getImageData(0,0,canvas.width,canvas.height).data;let top=canvas.height,bottom=0;
    for(let y=0;y<canvas.height;y++)for(let x=0;x<canvas.width;x++)if(pixels[(y*canvas.width+x)*4+3]>32){top=Math.min(top,y);bottom=Math.max(bottom,y);}
    const rect=image.getBoundingClientRect(),scene=image.closest('.oc-companion-scene').getBoundingClientRect();
    return {visibleHeight:(bottom-top+1)*rect.height/canvas.height,artTop:rect.top+top*rect.height/canvas.height,artBottom:rect.top+(bottom+1)*rect.height/canvas.height,sceneTop:scene.top,sceneBottom:scene.bottom,homeHeight:image.closest('.oc-companion-home').getBoundingClientRect().height};
  });
  expect(portrait.visibleHeight).toBeGreaterThan(100);expect(portrait.artTop).toBeGreaterThanOrEqual(portrait.sceneTop);
  expect(portrait.artBottom).toBeLessThanOrEqual(portrait.sceneBottom);expect(portrait.homeHeight).toBeLessThan(280);
  await page.screenshot({path:path.join(out,'existing-settings.png')});
  await page.getByRole('button',{name:'重新开蛋',exact:true}).click();
  const ritual=page.getByRole('dialog',{name:'确认重新开蛋'});
  await expect(ritual).toBeVisible();
  if(!existing.canHatch)await expect(ritual.getByRole('button',{name:'确认消耗 50 点并开蛋'})).toBeDisabled();
  await page.screenshot({path:path.join(out,'existing-ritual.png')});
  await page.keyboard.press('Escape');await expect(ritual).toHaveCount(0);
  await expect(page.getByRole('heading',{name:'伙伴与奖章',exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'重新开蛋',exact:true})).toBeFocused();
  expect(await go('CompanionSnapshot')).toEqual(existing);

  await start(); await page.goto(url); await page.waitForFunction(()=>window.go?.main?.App);
  await expect.poll(()=>go('CompanionSnapshot'),{timeout:20000}).toMatchObject({pet:null,hatchCost:0,canHatch:true});
  await settings();
  await expect(page.getByRole('region',{name:'我的伙伴'}).getByRole('img',{name:'伙伴蛋',exact:true})).toBeVisible();
  await expect(page.getByRole('radiogroup',{name:'伙伴形象'})).toHaveCount(0);
  await page.screenshot({path:path.join(out,'first-egg.png')});
  await page.getByRole('button',{name:'免费开蛋',exact:true}).click();
  await expect(page.getByRole('img',{name:'正在打开的伙伴蛋'})).toBeVisible();
  await page.screenshot({path:path.join(out,'opening.png')});
  await expect(page.getByRole('button',{name:'修改名字',exact:true})).toBeVisible();
  const firstDraw=await go('CompanionSnapshot'); expect(firstDraw.pet.xp).toBe(0); expect(firstDraw.level).toBe(1);
  const naming=page.getByRole('dialog',{name:'初次遇见伙伴'});
  const nameInput=naming.getByRole('textbox',{name:'给伙伴起个名字'});
  await expect(nameInput).toBeFocused();
  await expect(nameInput).toHaveValue(firstDraw.pet.name);
  await expect(naming.getByRole('button',{name:'就叫这个名字'})).toBeEnabled();
  await nameInput.fill('初遇小伙伴');
  await page.screenshot({path:path.join(out,'first-naming.png')});
  await page.keyboard.press('Enter');
  await expect(naming).toHaveCount(0);
  const first=await go('CompanionSnapshot');expect(first.pet.name).toBe('初遇小伙伴');
  expect(first.pet.id).toBe(firstDraw.pet.id);expect(first.revision).toBe(firstDraw.revision+1);
  await page.reload();await page.waitForFunction(()=>window.go?.main?.App);await settings();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.locator('.oc-companion-overview')).toContainText('初遇小伙伴');
  await expect(page.locator('.oc-companion-scene img')).toHaveAttribute('data-species',first.pet.speciesId);
  await page.screenshot({path:path.join(out,'first-result.png')});
  const bindings=await page.evaluate(()=>Object.keys(window.go.main.App));
  for(const removed of ['CompanionAdopt','CompanionUpdate','CompanionAppearance']) assert.ok(!bindings.includes(removed));
  await assert.rejects(go('CompanionHatch',first.revision));
  expect((await go('CompanionSnapshot')).pet).toEqual(first.pet);

  // Seed only the stopped, isolated test save. The actual review save stays unchanged.
  stop();
  const savePath=path.join(appDir,'companion.json');
  const seeded=JSON.parse(await readFile(savePath,'utf8')); seeded.pet.xp=73;
  await writeFile(savePath,JSON.stringify(seeded));
  await start(); await page.goto(url); await page.waitForFunction(()=>window.go?.main?.App);
  await expect.poll(()=>go('CompanionSnapshot')).toMatchObject({level:8,canHatch:true});
  await settings();
  await page.getByRole('button',{name:'重新开蛋',exact:true}).click();
  await expect(page.getByRole('dialog',{name:'确认重新开蛋'})).toContainText('73 → 23');
  await page.screenshot({path:path.join(out,'confirmation-light.png')});
  await page.getByRole('button',{name:'保留当前伙伴'}).focus();await page.keyboard.press('Tab');
  await expect(page.getByRole('button',{name:'关闭开蛋仪式'})).toBeFocused();
  await page.getByRole('button',{name:'保留当前伙伴'}).click();
  expect((await go('CompanionSnapshot')).pet.xp).toBe(73);
  await page.getByRole('button',{name:'重新开蛋',exact:true}).click();
  await page.getByRole('button',{name:'确认消耗 50 点并开蛋'}).click();
  await expect(page.getByRole('dialog').getByRole('heading',{name:'新的相遇，即将到来'})).toBeVisible();
  await expect(page.locator('.oc-hatch-visual')).toHaveAttribute('data-phase','cracking');
  const motion=await page.locator('.oc-hatch-visual').evaluate(visual=>{
    const animations=visual.getAnimations({subtree:true});
    const swing=animations.find(animation=>animation.animationName==='oc-egg-open');
    if(!swing)throw new Error('Missing finite shell animation');
    const effect=swing.effect,timing=effect.getTiming(),frames=effect.getKeyframes();
    const original=swing.currentTime;
    swing.pause();
    const samples=[0,396,704,1276,1738,1914].map(time=>{
      swing.currentTime=time;
      return {time,transform:getComputedStyle(effect.target).transform};
    });
    swing.currentTime=original;swing.play();
    return {duration:timing.duration,easing:getComputedStyle(effect.target).animationTimingFunction,segmentEasings:[...new Set(frames.map(frame=>frame.easing))],iterations:timing.iterations,offsets:frames.map(frame=>frame.computedOffset),samples,finiteEffects:animations.every(animation=>animation.effect.getTiming().iterations===1)};
  });
  expect(motion.duration).toBe(2200);expect(motion.iterations).toBe(1);expect(motion.finiteEffects).toBe(true);
  expect(motion.easing).toContain('cubic-bezier');
  expect(motion.offsets).toContain(.83);expect(new Set(motion.samples.map(sample=>sample.transform)).size).toBeGreaterThan(3);
  await writeFile(path.join(out,'motion.json'),JSON.stringify(motion,null,2));
  await page.screenshot({path:path.join(out,'ritual-opening.png')});
  await expect(page.getByRole('dialog').getByRole('heading',{name:/遇见了/})).toBeVisible();
  await expect(page.getByRole('dialog').locator('img')).toHaveAttribute('data-ready','true');
  await expect.poll(()=>page.getByRole('dialog').locator('.oc-hatch-visual > .oc-pixel-sprite').evaluate(el=>getComputedStyle(el).opacity)).toBe('1');
  await page.setViewportSize({width:850,height:833});
  const paidNameInput=page.getByRole('dialog').getByRole('textbox',{name:'给伙伴起个名字'});
  await expect(paidNameInput).toBeFocused();
  const nameIcons=page.locator('.oc-hatch-name-actions button');
  await expect(nameIcons).toHaveCount(1);
  for(const icon of await nameIcons.all()) {
    expect(await icon.textContent()).toBe('');
    const bounds=await icon.boundingBox();expect(bounds.width).toBe(32);expect(bounds.height).toBe(32);
    expect(await icon.getAttribute('title')).toBeTruthy();
  }
  const paidDraw=await go('CompanionSnapshot');expect(paidDraw.pet.name).toBe(first.pet.name);
  await expect(paidNameInput).toHaveValue(paidDraw.pet.name);
  expect(paidDraw.pet.xp).toBe(23);expect(paidDraw.revision).toBe(first.revision+1);
  await paidNameInput.fill('再次相遇的伙伴');
  await page.screenshot({path:path.join(out,'ritual-result.png')});
  await page.emulateMedia({reducedMotion:'reduce'});
  expect(await page.locator('.oc-hatch-visual').evaluate(visual=>[...visual.querySelectorAll('span, i, img')].every(element=>getComputedStyle(element).animationName==='none'))).toBe(true);
  await page.emulateMedia({reducedMotion:'no-preference'});
  await page.getByRole('button',{name:'就叫这个名字'}).click();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByRole('button',{name:'修改名字',exact:true})).toBeVisible();
  const paid=await go('CompanionSnapshot');
  expect(paid.pet.xp).toBe(23); expect(paid.level).toBe(3); expect(paid.levelXP).toBe(3);
  expect(paid.pet.speciesId).not.toBe(first.pet.speciesId);
  for(const key of ['id','adoptedAt']) expect(paid.pet[key]).toBe(first.pet[key]);
  expect(paid.pet.name).toBe('再次相遇的伙伴');
  expect(paid.revision).toBe(first.revision+2);
  await assert.rejects(go('CompanionHatch',first.revision));
  expect((await go('CompanionSnapshot')).pet.xp).toBe(23);
  await expect(page.locator('.oc-companion-overview')).toContainText('Lv.3');
  await page.screenshot({path:path.join(out,'paid-result.png')});
  const persisted=JSON.parse(await readFile(savePath,'utf8'));
  expect(persisted.seen).toEqual(seeded.seen); expect(persisted.daily).toEqual(seeded.daily);
  await page.getByRole('button',{name:'关闭',exact:true}).click();
  await page.getByRole('button',{name:'切换到暗色',exact:true}).click();
  await settings(); await page.screenshot({path:path.join(out,'paid-result-dark.png')});
  await page.getByRole('button',{name:'重新开蛋',exact:true}).click();
  await page.screenshot({path:path.join(out,'confirmation-dark.png')});
  await page.keyboard.press('Escape');
  stop(); await start(); await page.goto(url); await page.waitForFunction(()=>window.go?.main?.App);
  await expect.poll(()=>go('CompanionSnapshot')).toMatchObject({revision:paid.revision,pet:paid.pet});
  expect(errors).toEqual([]);
  await writeFile(path.join(out,'result.json'),JSON.stringify({pluginApiRevision:compatibility.apiRevision,firstSpecies:first.pet.speciesId,paidSpecies:paid.pet.speciesId,cost:50,xpBefore:73,xpAfter:23,levelBefore:8,levelAfter:3,levelXP:3,portrait,modalFocusContained:true,escapePreservesSettings:true,dedicatedHatchEntry:true,stagedReveal:true,firstFree:true,firstNamingPersisted:true,paidNamingPersisted:true,namingDoesNotChargeAgain:true,confirmationCancellable:true,oldFreeChoiceBindingsRemoved:true,staleAndInsufficientRejected:true,historyPreserved:true,restartPersisted:true,existingReviewSaveUnchanged:true,errors},null,2));
  console.log('PASS free hatch, paid hatch, cancel, insufficient/stale rejection, persistence and existing-save preservation');
} finally {await browser.close(); stop(); await writeFile(path.join(fixture,'app.log'),logs.join(''));}
