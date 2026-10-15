import {test} from 'node:test';
import assert from 'node:assert/strict';
import {plantedRegistration,translateRgba} from './register-motion.mjs';
test('removes translated planted-body drift while preserving authored pixels',()=>{
 const reference={width:64,height:64,data:new Uint8ClampedArray(64*64*4)};
 for(let y=14;y<52;y++)for(let x=20;x<44;x++)reference.data.set([80+x,140+y,40,255],(y*64+x)*4);
 const shifted=translateRgba(reference,{dx:7,dy:-4});
 const offset=plantedRegistration(reference,shifted);
 assert.deepEqual(offset,{dx:-7,dy:4});assert.deepEqual(translateRgba(shifted,offset).data,reference.data);
});
test('a moving appendage does not recenter the planted lower torso',()=>{
 const reference={width:64,height:64,data:new Uint8ClampedArray(64*64*4)};
 for(let y=10;y<54;y++)for(let x=22;x<42;x++)reference.data.set([100,150,200,255],(y*64+x)*4);
 const raised={...reference,data:new Uint8ClampedArray(reference.data)};
 for(let y=10;y<20;y++)for(let x=8;x<22;x++)raised.data.set([100,150,200,255],(y*64+x)*4);
 assert.deepEqual(plantedRegistration(reference,raised),{dx:0,dy:0});
});
