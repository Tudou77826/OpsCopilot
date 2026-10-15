import { test } from 'node:test';
import assert from 'node:assert/strict';
import { normalizePixels } from './normalize.mjs';

test('keeps transparent pixels and maps visible pixels to the shared palette', () => {
  const source={width:2,height:2,data:new Uint8ClampedArray([250,250,250,255,5,5,5,255,123,123,123,0,240,240,240,200])};
  const out=normalizePixels(source,2,2,['#ffffff','#000000']);
  assert.deepEqual([...out.data],[255,255,255,255,0,0,0,255,0,0,0,0,255,255,255,255]);
  assert.equal(source.data[0],250);
});
test('rejects implicit cropping and invalid palettes', () => {
  const source={width:2,height:2,data:new Uint8ClampedArray(16)};
  assert.throws(()=>normalizePixels(source,64,32,['#ffffff','#000000']));
  assert.throws(()=>normalizePixels(source,64,64,['#FFFFFF','#000000']));
});
test('keeps mostly transparent source cells clear and discards low-alpha edges before sampling', () => {
  const source={width:2,height:2,data:new Uint8ClampedArray([255,255,255,255,255,255,255,127,0,0,0,0,0,0,0,0])};
  const out=normalizePixels(source,1,1,['#ffffff','#000000']);
  assert.deepEqual([...out.data],[0,0,0,0]);
});
