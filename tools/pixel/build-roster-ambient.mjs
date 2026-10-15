// Keep interactions, register stationary idle frames, add authored ambient poses.
import {readFileSync,writeFileSync,mkdirSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng} from './png.mjs';
import {sampleRgba} from './rgba.mjs';
import {extractFrame,visibleBounds} from './sheet.mjs';
import {atlasRegions} from './atlas.mjs';
import {plantedRegistration,translateRgba} from './register-motion.mjs';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';
const species=JSON.parse(readFileSync('pkg/companion/species.json','utf8')), report=[];
for(const item of species) {
  const base=resolve('frontend-shell/src/companion/packs',item.id+'-v4'),output=resolve('frontend-shell/src/companion/packs',item.id+'-v5');
  mkdirSync(output,{recursive:true});
  const manifest=JSON.parse(readFileSync(resolve(base,'manifest.json'),'utf8'));manifest.id=item.id+'-v5';
  const rawReference=decodePng(readFileSync(resolve(base,manifest.assets[manifest.stages.child.idle.frames[0]].path))),rawBounds=visibleBounds(rawReference);
  const baseBounds=Object.values(manifest.assets).map(asset=>visibleBounds(decodePng(readFileSync(resolve(base,asset.path)))));
  const rootCorrection=Math.max(8-Math.min(...baseBounds.map(b=>b.left)),Math.min(247-Math.max(...baseBounds.map(b=>b.right)),128-Math.round((rawBounds.left+rawBounds.right)/2)));
  const reference=translateRgba(rawReference,{dx:rootCorrection,dy:0}),neutral=visibleBounds(reference),corrections=[];
  for(const [id,asset] of Object.entries(manifest.assets)) {
    const binary=readFileSync(resolve(base,asset.path));
    if(id.includes('-idle-')) {
      const image=decodePng(binary),offset=plantedRegistration(reference,image);corrections.push({id,...offset});
      writeFileSync(resolve(output,asset.path),encodePng(translateRgba(image,offset),{adaptive:true}));
    } else writeFileSync(resolve(output,asset.path),encodePng(translateRgba(decodePng(binary),{dx:rootCorrection,dy:0}),{adaptive:true}));
  }
  const sheet=decodePng(readFileSync(resolve('art/companion/roster-v3',item.id+'-source.png')));
  if(sheet.width/8!==sheet.height/4) throw new Error('Ambient sheet must be 8 by 4 square cells');
  const regions=atlasRegions(sheet);
  const cells=regions.map((region,index)=>extractFrame(sheet,index%8,Math.floor(index/8),8,region));
  const first=visibleBounds(cells[0]),referenceHeight=neutral.bottom-neutral.top+1;
  // A common scale connects the source's neutral pose to the existing character.
  const b=cells.map(visibleBounds);
  const scale=Math.min(referenceHeight/(first.bottom-first.top+1),216/Math.max(...b.map(bound=>bound.bottom-bound.top+1)),216/Math.max(...b.map(bound=>bound.right-bound.left+1)));
  manifest.ambient={};
  for(const [row,action] of ['blink-reference','look','stretch','walk'].entries()) {
    if(row===0) continue;
    const frames=[];
    for(let frame=0;frame<8;frame++) {
      const cell=cells[row*8+frame],bound=b[row*8+frame];
      // Neutral root center for stationary actions; each walk body centered in place.
      const center=action==='walk'?(bound.left+bound.right)/2:(first.left+first.right)/2;
      const baseline=bound.bottom,canvas={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
      for(let y=0;y<256;y++) for(let x=0;x<256;x++) canvas.data.set(sampleRgba(cell,(x-128)/scale+center,(y-neutral.bottom)/scale+baseline),(y*256+x)*4);
      const registered=action==='walk'?canvas:translateRgba(canvas,plantedRegistration(reference,canvas));
      const id=`${item.id}-${action}-${frame}`,path=id+'.png';
      writeFileSync(resolve(output,path),encodePng(registered,{adaptive:true}));
      manifest.assets[id]={path,role:'character',width:256,height:256};frames.push(id);
    }
    const durations=action==='look'?[160,180,220,420,200,220,420,200]:action==='stretch'?[180,200,220,240,450,240,220,200]:Array(8).fill(140);
    manifest.ambient[action]={frames,fps:7,anchor:{x:128,y:240},sequence:durations.map((durationMs,frame)=>({frame,durationMs})),loop:action==='walk'};
  }
  const result=validateCompanionPack(manifest,output);
  writeFileSync(resolve(output,'manifest.json'),JSON.stringify(manifest,null,2)+'\n');
  const entries=Object.entries(manifest.assets);
  writeFileSync(resolve(output,'catalog.ts'),entries.map(([id,asset],i)=>`import asset${i} from './${asset.path}';`).join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>`'${id}':asset${i},`).join('\n')+'\n};\n');
  report.push({species:item.id,corrections,...result});console.log(item.id,result);
}
writeFileSync('art/companion/roster-v3/registration.json',JSON.stringify(report,null,2)+'\n');
