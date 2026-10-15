import {test} from 'node:test';
import assert from 'node:assert/strict';
import {atlasRegions} from './atlas.mjs';
test('finds irregular transparent gutters without cutting authored feet',()=>{
 const sheet={width:256,height:128,data:new Uint8ClampedArray(256*128*4)};
 const bands=[[5,27],[36,58],[69,86],[98,124]];
 for(const [top,bottom] of bands)for(let column=0;column<8;column++)for(let y=top;y<bottom;y++)for(let x=column*32+7;x<column*32+25;x++)sheet.data.set([120,180,200,255],(y*256+x)*4);
 const regions=atlasRegions(sheet);
 assert.equal(regions.length,32);
 for(let row=0;row<4;row++)for(let column=0;column<8;column++){
  const region=regions[row*8+column];assert.ok(region.top<=bands[row][0]);assert.ok(region.bottom>=bands[row][1]);assert.ok(region.left<=column*32+7);assert.ok(region.right>=column*32+25);
 }
});
