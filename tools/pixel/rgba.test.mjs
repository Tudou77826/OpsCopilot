import {test} from 'node:test';
import assert from 'node:assert/strict';
import {sampleRgba} from './rgba.mjs';
test('preserves native full-color pixels and fractional transparency',()=>{
 const source={width:1,height:1,data:new Uint8ClampedArray([117,93,201,137])};
 assert.deepEqual(sampleRgba(source,0,0),[117,93,201,137]);
});
test('resizes soft edges without blending transparent black into the visible color',()=>{
 const source={width:2,height:1,data:new Uint8ClampedArray([200,120,40,255,0,0,0,0])};
 assert.deepEqual(sampleRgba(source,.5,0),[200,120,40,127.5]);
 assert.deepEqual(sampleRgba(source,4,0),[0,0,0,0]);
});
