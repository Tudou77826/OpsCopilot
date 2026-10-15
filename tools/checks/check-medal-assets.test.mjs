import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { validateMedalAssets } from './check-medal-assets.mjs';
import { decodePng, encodePng } from '../pixel/png.mjs';
const root = resolve('frontend-shell/src/companion/medals/v2');
const original = JSON.parse(readFileSync(resolve(root,'manifest.json'),'utf8'));
test('release atlas is complete, unique, transparent and within budget', () => {
  assert.equal(validateMedalAssets(original,root).assets,32);
});
test('rejects missing tiers and unsafe file paths', () => {
  const missing=structuredClone(original); delete missing.assets['medal-active-day-gold'];
  assert.throws(()=>validateMedalAssets(missing,root),/complete/);
  const unsafe=structuredClone(original); unsafe.assets['medal-active-day-gold']='../outside.png';
  assert.throws(()=>validateMedalAssets(unsafe,root),/Unsafe/);
});
test('rejects duplicated art and clipped visible borders',()=>{
  const first=readFileSync(resolve(root,Object.values(original.assets)[0]));
  assert.throws(()=>validateMedalAssets(original,root,()=>first),/Duplicate/);
  const image=decodePng(first); image.data.set([255,255,255,255],0);
  assert.throws(()=>validateMedalAssets(original,root,()=>encodePng(image,{adaptive:true})),/Clipped/);
});
