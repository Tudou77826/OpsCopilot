// Draft-only crop/export for visual approval. Does not write a runtime content pack.
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng,cropImage} from '../../../../tools/pixel/png.mjs';
import {visibleBounds} from '../../../../tools/pixel/sheet.mjs';
import {atlasRegions} from '../../../../tools/pixel/atlas.mjs';
import {sampleRgba} from '../../../../tools/pixel/rgba.mjs';
const root=resolve('art/companion/reviews/rabbit-v1'),manifest={status:'draft',fps:12,actions:{}};
function sample(image,scale,cx,baseline){
 const out={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
 for(let y=0;y<256;y++)for(let x=0;x<256;x++)out.data.set(sampleRgba(image,(x-128)/scale+cx,(y-240)/scale+baseline),(y*256+x)*4);
 return out;
}
const seated=decodePng(readFileSync(root+'/seated-source.png')),reference=visibleBounds(seated);
const referenceHeight=163;
writeFileSync(root+'/seated.png',encodePng(sample(seated,referenceHeight/(reference.bottom-reference.top+1),(reference.left+reference.right)/2,reference.bottom),{adaptive:true}));
for(const [action,rows] of [['walk',4],['stretch',3],['greet',2]]){
 const sheet=decodePng(readFileSync(root+'/'+action+'-source.png'));
 let empty=0;for(let p=3;p<sheet.data.length;p+=4)if(sheet.data[p]===0)empty++;
 if(!empty)throw new Error(action+': source is opaque, redraw before preview');
 const regions=atlasRegions(sheet,4,rows),frames=regions.map(r=>cropImage(sheet,r.left,r.top,r.right-r.left,r.bottom-r.top));
 const bounds=frames.map(visibleBounds),first=bounds[0],scale=referenceHeight/(first.bottom-first.top+1);
 // Contact poses register to the floor. The airborne group retains its height relative to push-off.
 const airborneGround=action==='walk'?bounds[4].bottom:undefined;
 const baselines=bounds.map((b,i)=>action==='walk'&&i>=4&&i<=7?Math.max(b.bottom,airborneGround):b.bottom);
 let fit=1;
 for(let n=0;n<frames.length;n++){
  const b=bounds[n],im=frames[n],cx=(b.left+b.right)/2,baseline=baselines[n];
  if(b.left<2||b.top<2||b.right>=im.width-2||b.bottom>=im.height-2)throw new Error(action+'/'+n+': body touches cell boundary');
  fit=Math.min(fit,112/(Math.max(cx-b.left,b.right-cx)*scale),224/((baseline-b.top+1)*scale));
 }
 mkdirSync(root+'/frames/'+action,{recursive:true});const files=[];
 for(let n=0;n<frames.length;n++){
  const im=frames[n],b=bounds[n],out=sample(im,scale*fit,(b.left+b.right)/2,baselines[n]),file='frames/'+action+'/'+String(n).padStart(2,'0')+'.png';
  writeFileSync(root+'/'+file,encodePng(out,{adaptive:true}));files.push(file);
 }
 // All frames use ONE scale; source pose geometry/colors stay intact.
 const durationsMs=files.map((_,i)=>action==='walk'?Math.round((i+1)*1000/12)-Math.round(i*1000/12):action==='stretch'?(i===6?500:i===0||i===11?250:120):[300,180,650,180,180,180,180,300][i]);
 manifest.actions[action]={frames:files,durationsMs,fit,scale:scale*fit};
}
writeFileSync(root+'/preview.json',JSON.stringify(manifest,null,2)+'\n');console.log('Draft frames exported; runtime unchanged');
