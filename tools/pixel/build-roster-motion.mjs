// Eight authored poses per action. One native frame set serves all three growth stages.
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng} from './png.mjs';
import {sampleRgba} from './rgba.mjs';
import {extractFrame,visibleBounds} from './sheet.mjs';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';
const source=resolve(process.argv.slice(2).find(arg=>!arg.startsWith('--'))??'art/companion/roster-v2');
const replace=process.argv.includes('--replace');
const catalog=JSON.parse(readFileSync('pkg/companion/species.json','utf8'));
const timing={idle:[3400,100,90,70,110,70,90,270],greet:[150,120,110,140,130,130,150,240],happy:[180,140,130,140,180,140,150,220],sleep:[800,750,650,650,800,750,650,650]};
for(const species of catalog){
 const sheet=decodePng(readFileSync(resolve(source,species.id+'-source.png')));
 const cells=Array.from({length:32},(_,index)=>extractFrame(sheet,index%8,Math.floor(index/8),8));
 const extent=cell=>Math.max(cell.bounds.right-cell.bounds.left+1,cell.bounds.bottom-cell.bounds.top+1);
 // Fix the body's center from neutral poses. A raised arm must not recenter the torso.
 const center=cells.slice(0,8).reduce((sum,cell)=>sum+(cell.bounds.left+cell.bounds.right)/2,0)/8;
 const baselines=Array.from({length:4},(_,row)=>Math.max(...cells.slice(row*8,row*8+8).map(cell=>cell.bounds.bottom)));
 const bounds=cells.map(visibleBounds);
 const horizontal=Math.max(...bounds.map(bound=>Math.max(center-bound.left,bound.right-center)));
 const above=Math.max(...bounds.map((bound,index)=>baselines[Math.floor(index/8)]-bound.top));
 const below=Math.max(1,...bounds.map((bound,index)=>bound.bottom-baselines[Math.floor(index/8)]));
 const scale=Math.min(216/Math.max(...cells.slice(0,8).map(extent)),116/horizontal,228/above,8/below);
 const output=resolve('frontend-shell/src/companion/packs',species.id+'-v4');mkdirSync(output,{recursive:true});
 const assets={},clips={};
 for(const [row,action] of ['idle','greet','happy','sleep'].entries()){
  const baseline=baselines[row];
  const frames=[];
  for(let frame=0;frame<8;frame++){
   const cell=cells[row*8+frame],canvas={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
   for(let y=0;y<256;y++) for(let x=0;x<256;x++)canvas.data.set(sampleRgba(cell,(x-128)/scale+center,(y-240)/scale+baseline),(y*256+x)*4);
   const id=`${species.id}-${action}-${frame}`,file=id+'.png',binary=encodePng(canvas,{adaptive:true}),path=resolve(output,file);
   const differs=existsSync(path)&&!readFileSync(path).equals(binary);
   if(differs&&!replace)throw new Error('Refusing different existing asset '+path);
   if(!existsSync(path)||differs)writeFileSync(path,binary);
   assets[id]={path:file,role:'character',width:256,height:256};frames.push(id);
  }
  const consistentWave=species.id==='lumen'||species.id==='tide' ? [0,1,2,3,4,3,2,1,7] : species.id==='moss' ? [0,1,2,3,5,3,2,1,7] : undefined;
  const sequence=action==='greet'&&consistentWave ? consistentWave.map(frame=>({frame,durationMs:frame===0||frame===7?200:120})) : timing[action].map((durationMs,frame)=>({frame,durationMs}));
  clips[action]={frames,fps:action==='idle'||action==='sleep'?2:8,anchor:{x:128,y:240},sequence,loop:action==='idle'||action==='sleep'};
 }
 const manifest={contractVersion:2,id:species.id+'-v4',speciesId:species.id,style:'pixel',frameSize:256,colorMode:'rgba',paletteHues:{mint:null,amber:38,lavender:272},stageScale:{child:160/216,growing:188/216,adult:1},assets,stages:{child:clips,growing:clips,adult:clips}};
 const result=validateCompanionPack(manifest,output);writeFileSync(resolve(output,'manifest.json'),JSON.stringify(manifest,null,2));
 const entries=Object.entries(assets);
 writeFileSync(resolve(output,'catalog.ts'),entries.map(([id],i)=>`import asset${i} from './${id}.png';`).join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>`'${id}':asset${i},`).join('\n')+'\n};\n');
 console.log(species.id+': '+result.assets+' native motion frames, '+result.bytes+' bytes');
}
