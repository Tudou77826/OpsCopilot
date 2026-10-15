import {readFileSync,writeFileSync,mkdirSync,copyFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng} from './png.mjs';
import {sampleRgba} from './rgba.mjs';
import {atlasRegions} from './atlas.mjs';
import {extractFrame,visibleBounds} from './sheet.mjs';
import {plantedRegistration,translateRgba} from './register-motion.mjs';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';
const root=resolve('art/companion/roster-v5'),retainedRoot=resolve('art/companion/roster-v4'),species=JSON.parse(readFileSync('pkg/companion/species.json','utf8')),report=[];
const actions=['idle','greet','happy','sleep','look','stretch','walk'];
function scaleImage(image,scale,cx,baseline) {
 const canvas={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
 for(let y=0;y<256;y++)for(let x=0;x<256;x++)canvas.data.set(sampleRgba(image,(x-128)/scale+cx,(y-240)/scale+baseline),(y*256+x)*4);
 return canvas;
}
function clip(frames,fps,loop,staticFrame=0,holds={}) {
 const sequence=frames.map((_,frame)=>({frame,durationMs:holds[frame]??Math.round((frame+1)*1000/fps)-Math.round(frame*1000/fps)}));
 return {frames,fps,anchor:{x:128,y:240},sequence,loop,staticFrame};
}
for(const item of species) {
 const folder=resolve(root,'staging',item.id);mkdirSync(folder,{recursive:true});
 const old=resolve('frontend-shell/src/companion/packs',item.id+'-v5');
 const original=decodePng(readFileSync(resolve(old,item.id+'-idle-0.png'))),oldBounds=visibleBounds(original);
 const referenceHeight=oldBounds.bottom-oldBounds.top+1, sources={};let fit=1;
 for(const action of actions) {
  const sourceRoot=['idle','look','sleep'].includes(action)?retainedRoot:root;
  const sheet=decodePng(readFileSync(resolve(sourceRoot,item.id,action+'-source.png')));
  const columns=action==='walk'?8:4,rows=action==='walk'?4:action==='idle'||action==='sleep'?2:3;
  const regions=atlasRegions(sheet,columns,rows),cells=regions.map((region,index)=>extractFrame(sheet,index%columns,Math.floor(index/columns),columns,region));
  const bounds=cells.map(visibleBounds), first=bounds[0],scale=referenceHeight/(first.bottom-first.top+1);
  const airborneEnd=item.id==='ember'?8:7;
  const grounded=action==='happy'?[0,1,2,3,...Array.from({length:11-airborneEnd},(_,i)=>i+airborneEnd+1)]:[];
  const groundFraction=grounded.length?grounded.reduce((sum,index)=>sum+bounds[index].bottom/cells[index].height,0)/grounded.length:0;
  const baselines=bounds.map((b,index)=>action==='happy'&&index>=4&&index<=airborneEnd?Math.max(b.bottom,groundFraction*cells[index].height):b.bottom);
  for(let index=0;index<cells.length;index++) {
   const cell=cells[index],b=bounds[index];
   if(cell.bounds.left<2||cell.bounds.top<2||cell.bounds.right>=cell.width-2||cell.bounds.bottom>=cell.height-2)throw new Error(item.id+'/'+action+'/'+index+': source body touches crop boundary');
   fit=Math.min(fit,224/((b.right-b.left+1)*scale),224/((baselines[index]-b.top+1)*scale));
  }
  sources[action]={cells,bounds,scale,baselines,airborneEnd};
 }
 if(fit<.8)throw new Error(item.id+': pose exceeds common body scale');
 const reference=scaleImage(original,fit,128,240),assets={},built={},clips={};
 for(const action of actions) {
  const source=sources[action],frames=[];
  for(let index=0;index<source.cells.length;index++) {
   const b=source.bounds[index],canvas=scaleImage(source.cells[index],source.scale*fit,(b.left+b.right)/2,source.baselines[index]);
   const offset=plantedRegistration(reference,canvas);
   // Happy airborne frames retain vertical motion; walk keeps authored stride poses.
   const canvasBounds=visibleBounds(canvas);
   // Registration must never cut an appendage. Preserve the full pose inside the export margin.
   const dx=Math.max(8-canvasBounds.left,Math.min(247-canvasBounds.right,offset.dx));
   const dy=action==='happy'&&index>=4&&index<=source.airborneEnd?0:offset.dy;
   const image=action==='walk'?canvas:translateRgba(canvas,{dx,dy});
   frames.push(image);
  }
  if(action!=='sleep') {frames[0]=reference;if(action!=='walk')frames[frames.length-1]=reference;}
  if(action==='walk') {
   frames[4]=frames[3];frames[7]=frames[8];frames[24]=frames[8];frames[28]=frames[27];frames[31]=reference;
  }
  const ids=[];
  for(let index=0;index<frames.length;index++) {
   const id=item.id+'-'+action+'-'+index,path=id+'.png';
   writeFileSync(resolve(folder,path),encodePng(frames[index],{adaptive:true}));
   assets[id]={path,role:'character',width:256,height:256};ids.push(id);
  }
  built[action]=ids;
  if(action==='idle')clips[action]=clip(ids,12,true,0,{0:3400,1:100,2:90,3:70,4:110,5:70,6:90,7:270});
  else if(action==='sleep')clips[action]=clip(ids,2,true,0,Object.fromEntries(ids.map((_,i)=>[i,750])));
  else if(action!=='walk')clips[action]=clip(ids,12,false,action==='happy'?6:action==='look'?4:6,action==='look'?{3:1300,8:1300}:action==='stretch'?{6:350}:{6:220});
 }
 const walk=built.walk;
 // Select adjacent head turns by direction rather than rapidly alternating source directions.
 const lookOrder={ember:[0,1,4,5,1,0,2,3,7,8,10,11],astra:[0,5,1,6,1,5,0,7,8,9,10,11],sage:[0,1,2,4,2,6,5,7,8,9,6,11]}[item.id];
 // A directed glance pauses at each side. Slow the turning poses as well as the holds.
 clips.look.sequence=(lookOrder??clips.look.frames.map((_,i)=>i)).map((frame,index)=>({frame,durationMs:index===3||index===8?1300:160}));
 const phases={turnOut:clip(walk.slice(0,4),8,false),walkStart:clip(walk.slice(4,8),12,false),walkStop:clip(walk.slice(24,28),12,false),turnIn:clip(walk.slice(28,32),8,false),stand:clip([walk[27]],2,true)};
 const baseClips=Object.fromEntries(['idle','greet','happy','sleep'].map(action=>[action,clips[action]]));
 const strides={lumen:56,ember:60,moss:48,bolt:56,astra:60,tide:48,sage:52};
 const alternateSettle=17;
 const manifest={contractVersion:2,id:item.id+'-v7',speciesId:item.id,style:'pixel',frameSize:256,colorMode:'rgba',paletteHues:{mint:null,amber:38,lavender:272},
  stageScale:{child:160/216,growing:188/216,adult:1},assets,stages:{child:baseClips,growing:baseClips,adult:baseClips},
  ambient:{look:clips.look,stretch:clips.stretch,walk:clip(walk.slice(8,24),12,true)},
  locomotion:{stride:strides[item.id],contacts:[0,8],phases,stopByContact:{8:clip([walk[16],walk[alternateSettle],walk[26],walk[27]],12,false)}}};
 const result=validateCompanionPack(manifest,folder);
 writeFileSync(resolve(folder,'manifest.json'),JSON.stringify(manifest,null,2)+'\n');
 const entries=Object.entries(assets);
 writeFileSync(resolve(folder,'catalog.ts'),entries.map(([id,asset],i)=>'import asset'+i+" from './"+asset.path+"';").join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>"'"+id+"':asset"+i+',').join('\n')+'\n};\n');
 report.push({species:item.id,fit,...result});console.log(item.id,result);
}
for(const item of species) {
 const folder=resolve(root,'staging',item.id),output=resolve('frontend-shell/src/companion/packs',item.id+'-v7');mkdirSync(output,{recursive:true});
 const manifest=JSON.parse(readFileSync(resolve(folder,'manifest.json'),'utf8'));
 for(const file of [...Object.values(manifest.assets).map(asset=>asset.path),'manifest.json','catalog.ts'])copyFileSync(resolve(folder,file),resolve(output,file));
}
writeFileSync(resolve(root,'build-report.json'),JSON.stringify(report,null,2)+'\n');
