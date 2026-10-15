import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {validateCompanionPack} from './check-companion-assets.mjs';
import {decodePng,encodePng} from '../pixel/png.mjs';
import {visibleBounds} from '../pixel/sheet.mjs';
const species=JSON.parse(readFileSync('pkg/companion/species.json','utf8'));
test('all seven packs have grounded idle frames, calm glances and a matched leisurely gait',()=>{
 for(const item of species){
  const root=resolve('frontend-shell/src/companion/packs',item.id+'-v'+(item.id==='lumen'?8:7)),pack=JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8'));
  assert.equal(validateCompanionPack(pack,root).assets,item.id==='lumen'?88:96);
  assert.equal(pack.ambient.walk.frames.length,16);assert.equal(pack.ambient.walk.fps,12);
  const cycle=pack.ambient.walk.sequence.reduce((sum,pose)=>sum+pose.durationMs,0);
  assert.equal(cycle,1333);assert.ok(pack.locomotion.stride>=48&&pack.locomotion.stride<=60);
  assert.ok(pack.locomotion.stride/cycle*1000>=36,'travel speed must match the leisurely cadence');
  assert.equal(pack.ambient.look.sequence.filter(pose=>pose.durationMs>=1200).length,2);
  assert.ok(pack.ambient.look.sequence.every(pose=>pose.durationMs>=160));
  assert.deepEqual(pack.locomotion.contacts,item.id==='lumen'?[0]:[0,8]);
  assert.equal(pack.locomotion.phases.stand.frames.length,1);
  const feet=pack.stages.child.idle.frames.map(id=>visibleBounds(decodePng(readFileSync(resolve(root,pack.assets[id].path)))).bottom);
  assert.equal(new Set(feet).size,1,item.id+': idle feet drift');
 }
});
test('rejects a changed transition endpoint, an invalid contact and an unavailable static frame',()=>{
 const root=resolve('frontend-shell/src/companion/packs/moss-v7'),original=JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8'));
 const altered=structuredClone(original);altered.locomotion.contacts=[16];assert.throws(()=>validateCompanionPack(altered,root),/contacts/);
 const badStatic=structuredClone(original);badStatic.stages.child.greet.staticFrame=12;assert.throws(()=>validateCompanionPack(badStatic,root),/Invalid clip/);
 const missingStop=structuredClone(original);delete missingStop.locomotion.stopByContact;assert.throws(()=>validateCompanionPack(missingStop,root),/complete actions/);
 const wrongStop=structuredClone(original);wrongStop.locomotion.stopByContact['8'].frames[0]=original.ambient.walk.frames[0];assert.throws(()=>validateCompanionPack(wrongStop,root),/Contact stop endpoints/);
 const path=resolve(root,original.assets[original.locomotion.phases.turnOut.frames[0]].path);
 const image=decodePng(readFileSync(path));image.data[(128*256+128)*4]=1;
 assert.throws(()=>validateCompanionPack(original,root,file=>file===path?encodePng(image):readFileSync(file)),/endpoints/);
});
