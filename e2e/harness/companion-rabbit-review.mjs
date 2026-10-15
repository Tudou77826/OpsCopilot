// Actual product integration check. Only local view state and unsaved input change.
import {chromium,expect} from '@playwright/test';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const out=resolve(process.argv[2]??'e2e/.runtime/companion-rabbit');await mkdir(out,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true}),context=await browser.newContext({viewport:{width:1041,height:833},recordVideo:{dir:out}}),page=await context.newPage(),errors=[];
// Accelerate ambient waiting only in this isolated test page, never in the product bundle.
await page.addInitScript(()=>{const original=window.setTimeout.bind(window);window.setTimeout=(cb,delay,...args)=>original(cb,delay>=45000&&delay<=75000?1500:delay,...args);});
page.on('pageerror',e=>errors.push(e.message));
const cdp=await context.newCDPSession(page);await cdp.send('Performance.enable');
const metrics=async()=>Object.fromEntries((await cdp.send('Performance.getMetrics')).metrics.map(v=>[v.name,v.value]));
try{
 await page.goto('http://127.0.0.1:34115/');const before=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 await page.getByTestId('nav-icon-companion').click();
 const region=page.getByRole('region',{name:'伙伴展示',exact:true}),actor=region.locator('.oc-companion-roaming'),sprite=actor.locator('img');
 await expect(sprite).toHaveAttribute('data-ready','true');
 const size=await sprite.evaluate(im=>({canvas:im.offsetWidth,transform:im.style.transform,src:im.src,species:im.dataset.species,stage:im.dataset.stage}));
 expect(Number(size.transform.match(/scale\(([^)]+)\)/)[1])).toBeCloseTo(.7*({child:160/216,growing:188/216,adult:1}[size.stage]),5);
 const themes={};
 for(const theme of ['light','dark']){
  await page.evaluate(theme=>document.documentElement.dataset.theme=theme,theme);
  themes[theme]=await region.locator('.oc-assistant-companion-scene').evaluate(el=>getComputedStyle(el).backgroundImage);
  expect(themes[theme]).toContain(theme+'-');
  await region.screenshot({path:resolve(out,theme+'.png')});
 }
 await page.evaluate(()=>document.documentElement.dataset.theme='light');
 await expect(sprite).toHaveAttribute('data-action','walk',{timeout:30000});
 const start=await metrics(),x=await actor.evaluate(el=>new DOMMatrix(getComputedStyle(el).transform).m41);
 await page.waitForTimeout(1000);
 const end=await metrics(),x2=await actor.evaluate(el=>new DOMMatrix(getComputedStyle(el).transform).m41);
 expect(Math.abs(x2-x)).toBeGreaterThan(15);expect(Math.abs(x2-x)).toBeLessThan(45);
 await region.screenshot({path:resolve(out,'walk.png')});
 // Stop only at the sole grounded contact, then return to front before reacting.
 await actor.click();await expect(sprite).toHaveAttribute('data-action','greet',{timeout:6000});
 await expect(actor).toHaveAttribute('data-facing','front');
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:5000});
 const settled=await actor.evaluate(el=>new DOMMatrix(getComputedStyle(el).transform).m41);
 await expect(sprite).toHaveAttribute('data-action','stretch',{timeout:25000});
 await expect.poll(()=>sprite.getAttribute('data-frame'),{intervals:[30]}).toBe('6');
 await region.screenshot({path:resolve(out,'stretch.png')});
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:5000});
 await expect(sprite).toHaveAttribute('data-action','walk',{timeout:15000});
 await expect(actor).toHaveAttribute('data-facing','left');
 await expect(sprite).toHaveAttribute('data-action','idle',{timeout:15000});await expect(actor).toHaveAttribute('data-facing','front');
 const latencies=[];for(let i=0;i<20;i++){const start=performance.now();await page.getByPlaceholder('输入问题…').fill('未保存预览输入 '+i);latencies.push(performance.now()-start);}
 await page.getByRole('button',{name:'收起伙伴展示',exact:true}).click();
 let frameChanges=0;await page.exposeFunction('rabbitFrameChange',()=>frameChanges++);
 await page.evaluate(()=>{const o=new MutationObserver(()=>window.rabbitFrameChange());o.observe(document.querySelector('.oc-companion-assistant-header'),{subtree:true,attributes:true,attributeFilter:['src']});});
 await page.waitForTimeout(3000);expect(frameChanges).toBe(0);const collapsedPoseChanges=frameChanges;await expect(page.getByPlaceholder('输入问题…')).toHaveValue('未保存预览输入 19');
 await page.getByRole('button',{name:'展开伙伴展示',exact:true}).click();
 await page.emulateMedia({reducedMotion:'reduce'});await expect(sprite).toHaveAttribute('data-motion','reduced');await page.waitForTimeout(3500);await expect(sprite).toHaveAttribute('data-action','idle');
 await page.setViewportSize({width:794,height:833});await region.screenshot({path:resolve(out,'narrow.png')});
 await page.getByPlaceholder('输入问题…').fill('');
 expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(before.pet);expect(errors).toEqual([]);
 latencies.sort((a,b)=>a-b);
 const report={packs:'lumen-v8; other species v7',ambientWaitOnlyAcceleratedInTestPage:true,size,themes,movementInOneSecond:Math.abs(x2-x),walkScriptMs:(end.ScriptDuration-start.ScriptDuration)*1000,walkTaskMs:(end.TaskDuration-start.TaskDuration)*1000,walkLayouts:end.LayoutCount-start.LayoutCount,settledPosition:settled,inputP95Ms:latencies[18],collapsedPoseChanges,storedPetPreserved:true,errors};
 await writeFile(resolve(out,'result.json'),JSON.stringify(report,null,2));console.log(JSON.stringify(report));
}finally{await context.close();await browser.close();}
