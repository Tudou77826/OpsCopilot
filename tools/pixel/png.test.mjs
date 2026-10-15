import { test } from 'node:test';
import assert from 'node:assert/strict';
import { deflateSync } from 'node:zlib';
import { decodePng, encodePng, cropImage } from './png.mjs';

test('adaptive PNG compression preserves every RGBA byte while reducing a detailed gradient',()=>{
 const image={width:128,height:128,data:new Uint8ClampedArray(128*128*4)};
 for(let y=0;y<128;y++) for(let x=0;x<128;x++) image.data.set([x*2,y*2,(x+y)%256,(x*3+y*2)%256],(y*128+x)*4);
 const compressed=encodePng(image,{adaptive:true});assert.deepEqual(decodePng(compressed).data,image.data);
 assert.ok(compressed.length<encodePng(image).length*.7);
});

function chunk(type,bytes) {
  const name=Buffer.from(type),data=Buffer.concat([name,bytes]);let crc=0xffffffff;
  for(const byte of data){crc^=byte;for(let i=0;i<8;i++)crc=(crc>>>1)^((crc&1)?0xedb88320:0);}
  const out=Buffer.alloc(bytes.length+12);out.writeUInt32BE(bytes.length);data.copy(out,4);out.writeUInt32BE((crc^0xffffffff)>>>0,out.length-4);return out;
}
test('RGB expands to opaque RGBA and roundtrips without pixel changes', () => {
  const header=Buffer.alloc(13);header.writeUInt32BE(2);header.writeUInt32BE(1,4);header[8]=8;header[9]=2;
  const source=Buffer.concat([Buffer.from('89504e470d0a1a0a','hex'),chunk('IHDR',header),chunk('IDAT',deflateSync(Buffer.from([0,10,20,30,40,50,60]))),chunk('IEND',Buffer.alloc(0))]);
  const image=decodePng(source);
  assert.deepEqual([...image.data],[10,20,30,255,40,50,60,255]);
  assert.deepEqual(decodePng(encodePng(image)),image);
});
test('RGBA preserves transparency and cropping copies the selected pixels', () => {
  const image={width:2,height:2,data:new Uint8ClampedArray([10,20,30,255,0,0,0,0,40,50,60,128,70,80,90,255])};
  assert.deepEqual(decodePng(encodePng(image)),image);
  const crop=cropImage(image,1,0,1,2);
  assert.deepEqual([...crop.data],[0,0,0,0,70,80,90,255]);
  crop.data[0]=99;assert.equal(image.data[4],0);
});
test('rejects truncated input and corrupt chunks rather than manufacturing pixels', () => {
  const image={width:1,height:1,data:new Uint8ClampedArray([0,0,0,255])};
  const binary=encodePng(image);
  assert.throws(()=>decodePng(binary.subarray(0,binary.length-12)));
  const corrupt=Buffer.from(binary);corrupt[16]^=1;
  assert.throws(()=>decodePng(corrupt));
});
