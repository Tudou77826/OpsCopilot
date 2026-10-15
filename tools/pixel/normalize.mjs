/** Normalize one AI source frame into a candidate pixel asset. Inputs are
 * immutable; existing outputs are refused. Publication requires a full pack
 * validation and visual review. See docs/companion-art-pipeline.md.
 */
import { readFileSync, writeFileSync, mkdirSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { decodePng, encodePng } from './png.mjs';
import { resampleMode } from './resample.mjs';

export function normalizePixels(source,width,height,palette) {
  if (!Number.isInteger(width) || !Number.isInteger(height) || width<1 || height<1 || width>1024 || height>1024) throw new Error('Invalid output dimensions');
  if (!Array.isArray(palette) || palette.length<2 || palette.length>32 || new Set(palette).size!==palette.length || palette.some(color=>!/^#[0-9a-f]{6}$/.test(color))) throw new Error('Palette requires 2-32 unique lowercase RGB colors');
  if (source.width*height!==source.height*width) throw new Error('Source aspect ratio must match the approved frame; crop deliberately before normalization');
  const pixels = new Uint8ClampedArray(source.data);
  for (let i = 0; i < pixels.length; i += 4) {
    if (pixels[i + 3] < 128) pixels.fill(0, i, i + 4);
    else pixels[i + 3] = 255;
  }
  const out=resampleMode({...source,data:pixels},width,height);
  const colors=palette.map(color=>{const value=Number.parseInt(color.slice(1),16);return [(value>>16)&255,(value>>8)&255,value&255];});
  for(let i=0;i<out.data.length;i+=4) {
    if(out.data[i+3]<128){out.data.fill(0,i,i+4);continue;}
    let best=colors[0],distance=Infinity;
    for(const color of colors){const d=color.reduce((sum,value,channel)=>sum+(value-out.data[i+channel])**2,0);if(d<distance){best=color;distance=d;}}
    out.data.set([...best,255],i);
  }
  return out;
}

if(process.argv[1] && resolve(process.argv[1])===fileURLToPath(import.meta.url)) {
  if(process.argv.length!==3) throw new Error('Usage: node tools/pixel/normalize.mjs <job.json>');
  const jobPath=resolve(process.argv[2]),job=JSON.parse(readFileSync(jobPath,'utf8'));
  const input=resolve(dirname(jobPath),job.input),output=resolve(dirname(jobPath),job.output);
  if(input===output) throw new Error('Source and candidate output must be different');
  const image=normalizePixels(decodePng(readFileSync(input)),job.width,job.height,job.palette);
  mkdirSync(dirname(output),{recursive:true});
  writeFileSync(output,encodePng(image),{flag:'wx'});
  console.log(`Candidate pixel asset: ${output}`);
}
