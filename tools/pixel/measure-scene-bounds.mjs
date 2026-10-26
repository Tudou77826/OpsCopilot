import { readFileSync, writeFileSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';
import { decodePng } from './png.mjs';

// Ignore nearly invisible export noise; retain every authored pose and a soft
// four-pixel margin. These are presentation bounds, not changes to the artwork.
export function measureSceneBounds(images) {
  let left=Infinity,top=Infinity,right=0,bottom=0,width=0,height=0;
  for (const image of images) {
    if (width && (image.width!==width || image.height!==height)) throw new Error('Scene frames must share a canvas');
    width=image.width;height=image.height;
    for(let y=0;y<height;y++) for(let x=0;x<width;x++) {
      if(image.data[(y*width+x)*4+3]<16) continue;
      left=Math.min(left,x);top=Math.min(top,y);right=Math.max(right,x+1);bottom=Math.max(bottom,y+1);
    }
  }
  if(!Number.isFinite(left)) throw new Error('Scene artwork is empty');
  return {left:Math.max(0,left-4),top:Math.max(0,top-4),right:Math.min(width,right+4),bottom:Math.min(height,bottom+4)};
}

// Regenerate after replacing a content pack's images:
// node tools/pixel/measure-scene-bounds.mjs path/to/manifest.json [...]
if(process.argv[1] && import.meta.url===pathToFileURL(resolve(process.argv[1])).href) {
  if(process.argv.length<3) throw new Error('Provide at least one content manifest');
  for(const file of process.argv.slice(2)) {
    const manifest=JSON.parse(readFileSync(file,'utf8'));
    if(manifest.contractVersion!==2) throw new Error('Scene bounds require a detailed content pack');
    const images=Object.values(manifest.assets).filter(asset=>asset.role==='character').map(asset=>decodePng(readFileSync(resolve(dirname(file),asset.path))));
    manifest.sceneBounds=measureSceneBounds(images);
    writeFileSync(file,JSON.stringify(manifest,null,2)+'\n');
  }
}
