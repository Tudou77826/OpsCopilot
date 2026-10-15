// Extract full-color frames from the original AI sheets; preserve shading and soft alpha.
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng} from './png.mjs';
import {sampleRgba} from './rgba.mjs';
import {extractFrame} from './sheet.mjs';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';
const source=resolve(process.argv[2] ?? 'art/companion/roster-v1');
const catalog=JSON.parse(readFileSync('pkg/companion/species.json','utf8'));
const timing={
 idle:[[0,3600],[1,100],[2,200],[1,100],[3,200]],
 greet:[[0,180],[1,130],[2,220],[1,150],[2,200],[3,240],[0,250]],
 happy:[[0,160],[1,180],[2,160],[3,200],[1,160],[0,280]],
 sleep:[[0,1100],[1,1200],[2,1300],[3,1200]],
};
for(const species of catalog) {
 const sheet=decodePng(readFileSync(resolve(source,species.id+'-source.png')));
 if(sheet.width<256||sheet.height<256) throw new Error('Expected a large four-by-four sheet');
 const cells=Array.from({length:16},(_,index)=>extractFrame(sheet,index%4,Math.floor(index/4)));
 const side=Math.max(...cells.map(cell=>Math.max(cell.bounds.right-cell.bounds.left+1,cell.bounds.bottom-cell.bounds.top+1)));
 const output=resolve('frontend-shell/src/companion/packs',species.id+'-v3');mkdirSync(output,{recursive:true});
 const assets={},stages={};
 for(const [stageIndex,stage] of ['child','growing','adult'].entries()) {
  const scale=[160,188,216][stageIndex]/side;stages[stage]={};
  for(const [row,action] of ['idle','greet','happy','sleep'].entries()) {
   // A shared row baseline preserves the authored jump instead of pinning every pose to the floor.
   const baseline=Math.max(...cells.slice(row*4,row*4+4).map(cell=>cell.bounds.bottom));
   const frames=[];
   for(let frame=0;frame<4;frame++) {
    const cell=cells[row*4+frame],center=(cell.bounds.left+cell.bounds.right)/2;
    const canvas={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
    for(let y=0;y<256;y++) for(let x=0;x<256;x++) canvas.data.set(sampleRgba(cell,(x-128)/scale+center,(y-224)/scale+baseline),(y*256+x)*4);
    const id=`${species.id}-${stage}-${action}-${frame}`,file=id+'.png',encoded=encodePng(canvas),path=resolve(output,file);
    if(existsSync(path)&&!readFileSync(path).equals(encoded)) throw new Error('Refusing different existing asset '+path);
    if(!existsSync(path)) writeFileSync(path,encoded);
    assets[id]={path:file,role:'character',width:256,height:256};frames.push(id);
   }
   stages[stage][action]={frames,fps:action==='idle'||action==='sleep'?2:5,anchor:{x:128,y:224},sequence:timing[action].map(([frame,durationMs])=>({frame,durationMs}))};
  }
 }
 const manifest={contractVersion:2,id:species.id+'-v3',speciesId:species.id,style:'pixel',frameSize:256,colorMode:'rgba',paletteHues:{mint:null,amber:38,lavender:272},assets,stages};
 const result=validateCompanionPack(manifest,output);
 writeFileSync(resolve(output,'manifest.json'),JSON.stringify(manifest,null,2));
 const entries=Object.entries(assets);
 writeFileSync(resolve(output,'catalog.ts'),entries.map(([id],i)=>`import asset${i} from './${id}.png';`).join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>`'${id}':asset${i},`).join('\n')+'\n};\n');
 console.log(species.id+': '+result.assets+' detailed frames, '+result.bytes+' bytes');
}
