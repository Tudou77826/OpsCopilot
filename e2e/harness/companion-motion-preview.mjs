// Real idle, greeting and joy playback; writes no stored preferences or business records.
import {chromium,expect} from '@playwright/test';
import {mkdir,writeFile} from 'node:fs/promises';
import path from 'node:path';
const evidence=path.resolve(process.argv[2]??'e2e/.runtime/motion-preview');await mkdir(evidence,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true});
const context=await browser.newContext({viewport:{width:1041,height:833},recordVideo:{dir:evidence,size:{width:1041,height:833}}});
const page=await context.newPage(),video=page.video(),errors=[];page.on('pageerror',error=>errors.push(error.message));
const cdp=await context.newCDPSession(page);await cdp.send('Network.enable');let bytes=0;cdp.on('Network.loadingFinished',event=>{bytes+=event.encodedDataLength;});
try{
 await page.goto('http://127.0.0.1:34115/');const original=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 await page.getByTestId('nav-icon-companion').click();const sprite=page.locator('.oc-assistant-companion-scene img.oc-pixel-sprite');
 await expect(sprite).toHaveAttribute('data-species',original.pet.speciesId);await expect(sprite).toHaveAttribute('data-motion','running');
 await page.evaluate(()=>{window.motionPreview={loads:[],start:undefined,action:undefined};const sprite=document.querySelector('.oc-assistant-companion-scene img');const observer=new MutationObserver(()=>{const state=window.motionPreview;if(sprite.dataset.action!==state.action){state.action=sprite.dataset.action;if(['greet','happy'].includes(state.action))state.start=performance.now();}if(state.start!==undefined&&sprite.dataset.motion==='running'){state.loads.push({action:state.action,readyMs:performance.now()-state.start});state.start=undefined;}});observer.observe(sprite,{attributes:true,attributeFilter:['data-action','data-motion']});});
 await page.waitForTimeout(4400);
 for(const action of ['greet','happy']){
  await page.locator('.oc-assistant-companion-scene .oc-companion-touch').click();await expect(sprite).toHaveAttribute('data-action',action);
  await expect.poll(()=>sprite.getAttribute('data-frame'),{intervals:[50]}).toBe('4');
  await page.screenshot({path:path.join(evidence,`companion-${action}.png`)});
  await expect(sprite).toHaveAttribute('data-action','idle',{timeout:4000});
 }
 const beforeWarm=bytes;await page.locator('.oc-assistant-companion-scene .oc-companion-touch').click();await expect(sprite).toHaveAttribute('data-action','greet');
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:4000});const warmNetworkBytes=bytes-beforeWarm;expect(warmNetworkBytes).toBe(0);
 await page.getByRole('button',{name:'养成与奖章',exact:true}).click();
 const home=page.locator('.oc-companion-scene img');
 await page.setViewportSize({width:1041,height:400});
 await page.getByLabel('分类榜').scrollIntoViewIfNeeded();await expect(home).toHaveAttribute('data-motion','paused');
 await page.setViewportSize({width:1041,height:833});
 await page.getByRole('heading',{name:'伙伴与奖章',exact:true}).scrollIntoViewIfNeeded();await expect(home).toHaveAttribute('data-motion','running');
 expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(original.pet);
 expect(errors).toEqual([]);const loads=await page.evaluate(()=>window.motionPreview.loads);
 await writeFile(path.join(evidence,'result.json'),JSON.stringify({loads,warmNetworkBytes,finiteCompletion:true,offscreenPause:true,resume:true,preservedPet:true,pageErrors:errors},null,2));
 console.log(JSON.stringify({loads,finiteCompletion:true,offscreenPause:true,preservedPet:true}));
}finally{await context.close();await video.saveAs(path.join(evidence,'companion-motion.webm'));await browser.close();}
