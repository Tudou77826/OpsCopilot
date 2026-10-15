import { readFileSync, writeFileSync } from 'node:fs';
import { resolve, dirname } from 'node:path';
import { decodePng, encodePng } from './png.mjs';
const file = resolve(process.argv[2]), manifest = JSON.parse(readFileSync(file, 'utf8'));
const canvas = { width: 1536, height: 960, data: new Uint8ClampedArray(1536 * 960 * 4) };
for (let i = 0; i < canvas.data.length; i += 4) canvas.data.set([255,241,213,255], i);
function paint(id, left, top, scale) {
  const image = decodePng(readFileSync(resolve(dirname(file), manifest.assets[id].path)));
  for (let y = 0; y < image.height * scale; y++) for (let x = 0; x < image.width * scale; x++) {
    const p = (Math.floor(y / scale) * image.width + Math.floor(x / scale)) * 4;
    if (image.data[p + 3]) canvas.data.set(image.data.subarray(p, p + 4), ((top + y) * canvas.width + left + x) * 4);
  }
}
for (const [row,stage] of ['child','growing','adult'].entries()) for (const [column,action] of ['idle','happy','greet','sleep'].entries()) for (let frame=0;frame<2;frame++) paint(`${stage}-${action}-${frame}`,(column*2+frame)*192,row*192,3);
paint('profile-background',0,600,2);
const kinds=['knowledge-archive','script-created','quick-command-created','case-completed','knowledge-read','script-used','quick-command-used','active-day'];
for (const [row,tier] of ['locked','bronze','silver','gold'].entries()) for (const [column,kind] of kinds.entries()) paint(`medal-${kind}-${tier}`,720+column*96,600+row*88,2);
writeFileSync(resolve(process.argv[3]), encodePng(canvas));
