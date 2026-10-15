import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {validateCompanionPack} from './check-companion-assets.mjs';
import {decodePng} from '../pixel/png.mjs';
import {visibleBounds} from '../pixel/sheet.mjs';
import {plantedRegistration} from '../pixel/register-motion.mjs';
const species=JSON.parse(readFileSync('pkg/companion/species.json','utf8'));
test('every species has planted idle feet and three complete authored ambient clips',()=>{
 for(const item of species){
  const root=resolve('frontend-shell/src/companion/packs',item.id+'-v5'),pack=JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8'));
  assert.equal(validateCompanionPack(pack,root).assets,56);
  const feet=pack.stages.child.idle.frames.map(id=>visibleBounds(decodePng(readFileSync(resolve(root,pack.assets[id].path)))).bottom);
  assert.equal(new Set(feet).size,1,`${item.id}: idle baseline drift`);
  const frames=pack.stages.child.idle.frames.map(id=>decodePng(readFileSync(resolve(root,pack.assets[id].path))));
  for(const image of frames) assert.equal(plantedRegistration(frames[0],image).dx,0,`${item.id}: idle torso drift`);
  for(const action of ['look','stretch','walk']) assert.equal(pack.ambient[action].frames.length,8);
 }
});
test('rejects missing ambient frames, wrong anchors, and looping finite gestures',()=>{
 const root=resolve('frontend-shell/src/companion/packs/lumen-v5'),original=JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8'));
 for(const change of [pack=>delete pack.ambient.walk,pack=>pack.ambient.look.anchor.x++,pack=>pack.ambient.stretch.loop=true]){
  const pack=structuredClone(original);change(pack);assert.throws(()=>validateCompanionPack(pack,root),/complete|Ambient/);
 }
});
