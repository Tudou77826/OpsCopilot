import {chromium,expect} from '@playwright/test';
import {mkdir,writeFile} from 'node:fs/promises';
import {readFileSync} from 'node:fs';
import path from 'node:path';
const evidence=path.resolve(process.argv[2]??'e2e/.runtime/ambient-review');await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true}),context=await browser.newContext({viewport:{width:1041,height:833},recordVideo:{dir:evidence,size:{width:1041,height:833}}});
const page=await context.newPage(),video=page.video(),errors=[];page.on('pageerror',error=>errors.push(error.message));
// Shorten only the long ambient wait in this isolated review page, never pose playback or travel.
await page.addInitScript(()=>{const original=window.setTimeout.bind(window);window.ambientWaits=[];window.setTimeout=(callback,delay,...args)=>{
 if(delay>=45000&&delay<=75000){window.ambientWaits.push(delay);return original(callback,6500,...args);}return original(callback,delay,...args);
};});
try {
 await page.goto('http://127.0.0.1:34115/');const before=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 const pack=JSON.parse(readFileSync(path.resolve('frontend-shell/src/companion/packs',before.pet.speciesId+'-v7/manifest.json'),'utf8'));
 await page.getByTestId('nav-icon-companion').click();
 const button=page.locator('.oc-companion-roaming'),sprite=button.locator('img'),measure=()=>button.evaluate(element=>element.getBoundingClientRect().x);
 await expect(sprite).toHaveAttribute('data-motion','running');
 await page.evaluate(()=>{window.motionPhases=[];let previous='';const observer=new MutationObserver(()=>{const sprite=document.querySelector('.oc-companion-roaming img');const action=sprite?.dataset.action;if(action&&action!==previous){previous=action;window.motionPhases.push({action,facing:sprite.closest('button').dataset.facing,time:performance.now(),x:sprite.closest('button').getBoundingClientRect().x});}});observer.observe(document.querySelector('.oc-companion-roaming'),{subtree:true,attributes:true,attributeFilter:['data-action']});});
 const origin=await measure();await page.waitForTimeout(4400);expect(Math.abs(await measure()-origin)).toBeLessThan(1);
 for(const action of ['look','look','stretch','look']) {
  await expect(sprite).toHaveAttribute('data-action',action,{timeout:15000});
  const held=action==='look'?pack.ambient.look.sequence.find(pose=>pose.durationMs>=1200).frame:6;
  await expect.poll(()=>sprite.getAttribute('data-frame'),{intervals:[30]}).toBe(String(held));
  await page.getByRole('region',{name:'伙伴展示',exact:true}).screenshot({path:path.join(evidence,action+'.png')});
  await expect(sprite).toHaveAttribute('data-action','idle',{timeout:6000});console.log(action+' completed');
 }
 await expect(sprite).toHaveAttribute('data-action','walk',{timeout:15000});await expect(sprite).toHaveAttribute('data-motion','running');
 const rightStart=await measure();await page.waitForTimeout(800);const rightMiddle=await measure();expect(rightMiddle-rightStart).toBeGreaterThan(8);
 await page.getByRole('region',{name:'伙伴展示',exact:true}).screenshot({path:path.join(evidence,'walk-right.png')});
 const startPose=await sprite.getAttribute('src');await page.waitForTimeout(180);expect(await sprite.getAttribute('src')).not.toBe(startPose);
 await expect(sprite).toHaveAttribute('data-action','turnIn',{timeout:10000});
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:3000});
 await expect(button).toHaveAttribute('data-facing','front');const settled=await measure();
 await page.waitForTimeout(500);expect(Math.abs(await measure()-settled)).toBeLessThan(1);
 console.log('right walk returned to front idle');
 await expect(sprite).toHaveAttribute('data-action','walk',{timeout:75000});await expect(sprite).toHaveAttribute('data-motion','running');
 const leftStart=await measure();await page.waitForTimeout(800);const leftMiddle=await measure();expect(leftStart-leftMiddle).toBeGreaterThan(8);
 await page.getByRole('region',{name:'伙伴展示',exact:true}).screenshot({path:path.join(evidence,'walk-left.png')});
 const interactionStart=await measure();await button.click({force:true});await expect(sprite).toHaveAttribute('data-action','greet',{timeout:6000});
 expect(Math.abs(await measure()-interactionStart)).toBeLessThan(60);const responsePosition=await measure();
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:4500});expect(Math.abs(await measure()-responsePosition)).toBeLessThan(1);
 console.log('left walk grounded interruption and response passed');
 await page.emulateMedia({reducedMotion:'reduce'});await expect(sprite).toHaveAttribute('data-motion','reduced');
 const paused=await measure();await page.waitForTimeout(700);expect(Math.abs(await measure()-paused)).toBeLessThan(1);
 await page.emulateMedia({reducedMotion:'no-preference'});
 await page.setViewportSize({width:794,height:833});
 const input=page.getByPlaceholder('输入问题…');await input.fill('走动与收起测试草稿');
 await page.getByRole('button',{name:'收起伙伴展示',exact:true}).click();await expect(button).toHaveCount(0);await expect(input).toHaveValue('走动与收起测试草稿');
 await page.getByRole('button',{name:'展开伙伴展示',exact:true}).click();await expect(sprite).toHaveAttribute('data-motion','running');
 await page.getByRole('region',{name:'伙伴展示',exact:true}).screenshot({path:path.join(evidence,'narrow.png')});
 expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(before.pet);expect(errors).toEqual([]);
 const phases=await page.evaluate(()=>window.motionPhases);
 const actions=phases.map(phase=>phase.action);
 for(const required of ['turnOut','walkStart','walk','walkStop','turnIn','greet'])expect(actions).toContain(required);
 const ambientWaits=await page.evaluate(()=>window.ambientWaits);expect(ambientWaits.every(wait=>wait>=45000&&wait<=75000)).toBe(true);
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({assetsVersion:'v7',waitOnlyAccelerated:true,ambientWaits,stableIdle:true,look:true,stretch:true,rightMovement:rightMiddle-rightStart,leftMovement:leftStart-leftMiddle,frontIdleRestored:true,interactionPositionPreserved:true,reducedMotion:true,foldPreservesDraft:true,storedPetPreserved:true,phases,errors},null,2));
 console.log('ambient review passed');
} finally {await context.close();await video.saveAs(path.join(evidence,'companion-ambient.webm'));await browser.close();}
