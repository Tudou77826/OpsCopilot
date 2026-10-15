// Actual numeric-level API, personal UI and authenticated public projection, preserving saved XP.
import {chromium,expect} from '@playwright/test';
import {mkdir,readFile,writeFile} from 'node:fs/promises';
import {resolve} from 'node:path';
const out=resolve(process.argv[2]??'e2e/.runtime/companion-levels');await mkdir(out,{recursive:true});
const before=JSON.parse(await readFile('e2e/.runtime/companion-level100-before.json','utf8'));
const browser=await chromium.launch({channel:'msedge',headless:true}),page=await browser.newPage({viewport:{width:860,height:833}}),errors=[];
page.on('pageerror',e=>errors.push(e.message));
try{
 await page.goto('http://127.0.0.1:34115/');await page.waitForFunction(()=>window.go?.main?.App);
 const snapshot=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());
 expect(snapshot.pet).toEqual(before.pet);expect(snapshot.revision).toBe(before.revision);
 expect(snapshot.level).toBe(Math.min(100,1+Math.floor(snapshot.pet.xp/10)));
 expect(snapshot.levelXP).toBe(snapshot.level===100?0:snapshot.pet.xp%10);
 expect(snapshot.levelXPRequired).toBe(snapshot.level===100?0:10);expect(snapshot.stage).toBeUndefined();
 await page.getByTestId('nav-icon-companion').click();const header=page.getByRole('region',{name:'伙伴展示',exact:true});
 await expect(header.locator('.oc-assistant-companion-identity small')).toHaveText('Lv.'+snapshot.level);
 const progress=header.getByRole('progressbar',{name:'伙伴升级进度'});await expect(progress).toHaveAttribute('value',String(snapshot.level===100?1:snapshot.levelXP));
 await expect(header.locator('.oc-companion-roaming img')).toHaveAttribute('data-stage','child');
 await header.screenshot({path:resolve(out,'assistant.png')});
 await page.getByRole('button',{name:'收起伙伴展示',exact:true}).click();await expect(header.getByText('Lv.'+snapshot.level,{exact:true})).toBeVisible();
 await page.getByRole('button',{name:'设置',exact:true}).click();await page.getByRole('navigation').getByText('伙伴与奖章',{exact:true}).click();
 await expect(page.locator('.oc-companion-overview')).toContainText('Lv.'+snapshot.level);
 await expect(page.locator('.oc-companion-page').getByRole('progressbar',{name:'伙伴升级进度'})).toHaveAttribute('max',String(snapshot.level===100?1:10));
 await expect(page.locator('.oc-companion-page')).not.toContainText(/幼年|成熟|进入了.*阶段/);
 await page.screenshot({path:resolve(out,'settings.png')});
 const initialStatus=await page.evaluate(()=>window.go.main.App.AchievementStatus());
 // Existing participation authorizes updating this member's derived companion projection.
 await page.evaluate(()=>window.go.main.App.SocialSync());
 const status=await page.evaluate(()=>window.go.main.App.AchievementStatus());
 expect(status.selected).toEqual(initialStatus.selected);
 let publicLevel;
 if(status.binding?.member?.userId){
  const profile=await page.evaluate(id=>window.go.main.App.SocialProfile(id),status.binding.member.userId);
  expect(profile.companion.level).toBe(snapshot.level);expect(profile.companion.stage).toBeUndefined();
  expect(Object.keys(profile.companion).sort()).toEqual(['level','name','paletteId','speciesId']);publicLevel=profile.companion.level;
 }
 const after=await page.evaluate(()=>window.go.main.App.CompanionSnapshot());expect(after.pet).toEqual(before.pet);expect(after.revision).toBe(before.revision);expect(errors).toEqual([]);
 await writeFile(resolve(out,'result.json'),JSON.stringify({level:snapshot.level,totalXP:snapshot.pet.xp,levelXP:snapshot.levelXP,levelXPRequired:snapshot.levelXPRequired,publicLevel,petAndXPUnchanged:true,petRevisionUnchanged:true,selectionUnchanged:true,appearanceFixed:'child art index; no gameplay stages',errors},null,2));console.log(JSON.stringify({level:snapshot.level,xp:snapshot.pet.xp,publicLevel,storedXPUnchanged:true}));
}finally{await browser.close();}
