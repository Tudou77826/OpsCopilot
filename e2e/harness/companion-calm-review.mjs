// Real-time check of the ordinary product cadence. No clock or wait overrides.
import {chromium,expect} from '@playwright/test';
import {mkdir,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const out=resolve(process.argv[2]??'e2e/.runtime/companion-calm');await mkdir(out,{recursive:true});
const browser=await chromium.launch({channel:'msedge',headless:true}),page=await browser.newPage({viewport:{width:1041,height:833}}),errors=[];
page.on('pageerror',e=>errors.push(e.message));
try{
 await page.goto('http://127.0.0.1:34115/');const before=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 await page.getByTestId('nav-icon-companion').click();
 const actor=page.locator('.oc-companion-roaming'),sprite=actor.locator('img');await expect(sprite).toHaveAttribute('data-ready','true');
 await expect(sprite).toHaveAttribute('data-action','idle');
 await page.evaluate(()=>{window.calmActions=[];const actor=document.querySelector('.oc-companion-roaming');new MutationObserver(()=>window.calmActions.push(actor.dataset.ambient)).observe(actor,{attributes:true,attributeFilter:['data-ambient']});});
 const start=Date.now();await page.waitForTimeout(44000);
 await expect(sprite).toHaveAttribute('data-action','idle');
 const naturalIdleMs=Date.now()-start,actions=await page.evaluate(()=>window.calmActions);expect(actions).toEqual([]);
 await actor.click();await expect(sprite).toHaveAttribute('data-action','greet');await expect(sprite).toHaveAttribute('data-action','idle',{timeout:5000});
 expect((await page.evaluate(()=>window.go.main.App.CompanionSnapshot())).pet).toEqual(before.pet);expect(errors).toEqual([]);
 await writeFile(resolve(out,'result.json'),JSON.stringify({character:before.pet?.speciesId,naturalIdleMs,observedWindowMs:44000,ambientActionsBeforeClick:actions,clickResponded:true,storedPetPreserved:true,errors},null,2));console.log('44 seconds of natural idle and explicit greeting passed');
}finally{await browser.close();}
