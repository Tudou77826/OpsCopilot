// AI-authored four-row clips, normalized with one shared scale and foot anchor.
import {readFileSync,writeFileSync,mkdirSync,existsSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng} from './png.mjs';
import {normalizePixels} from './normalize.mjs';
import {validateCompanionPack} from '../checks/check-companion-assets.mjs';
const source=resolve(process.argv[2] ?? 'art/companion/roster-v1');
const catalog=JSON.parse(readFileSync('pkg/companion/species.json','utf8'));
const crop=(image,x,y,width,height)=>{const data=new Uint8ClampedArray(width*height*4);for(let row=0;row<height;row++)data.set(image.data.subarray(((y+row)*image.width+x)*4,((y+row)*image.width+x+width)*4),row*width*4);return {width,height,data};};
function bounds(image){
 const visited=new Uint8Array(image.width*image.height);let largest=[];
 for(let i=0;i<visited.length;i++)if(!visited[i]&&image.data[i*4+3]>=128){const component=[i];visited[i]=1;for(let cursor=0;cursor<component.length;cursor++){const p=component[cursor],x=p%image.width,y=Math.floor(p/image.width);for(let dy=-1;dy<=1;dy++)for(let dx=-1;dx<=1;dx++){const nx=x+dx,ny=y+dy,n=ny*image.width+nx;if(nx>=0&&ny>=0&&nx<image.width&&ny<image.height&&!visited[n]&&image.data[n*4+3]>=128){visited[n]=1;component.push(n);}}}if(component.length>largest.length)largest=component;}
 if(!largest.length)throw new Error('Empty AI frame');
 const body=new Set(largest);const clean={...image,data:new Uint8ClampedArray(image.data)};
 let x0=image.width,y0=image.height,x1=0,y1=0;
 for(let i=0;i<visited.length;i++){if(!body.has(i)){clean.data.fill(0,i*4,i*4+4);continue;}const x=i%image.width,y=Math.floor(i/image.width);x0=Math.min(x0,x);y0=Math.min(y0,y);x1=Math.max(x1,x);y1=Math.max(y1,y);}
 return crop(clean,x0,y0,x1-x0+1,y1-y0+1);
}
const hex=rgb=>'#'+rgb.map(value=>value.toString(16).padStart(2,'0')).join('');
function paletteOf(cells){const bins=new Map();for(const cell of cells)for(let i=0;i<cell.data.length;i+=4)if(cell.data[i+3]>=128){const rgb=Array.from(cell.data.subarray(i,i+3)).map(v=>Math.min(255,Math.round(v/24)*24));const key=hex(rgb);bins.set(key,(bins.get(key)??0)+1);}return [...bins].sort((a,b)=>b[1]-a[1]).slice(0,32).map(([key])=>key);}
function tinted(palette,hue){const used=new Set();return palette.map(color=>{const rgb=color.slice(1).match(/../g).map(v=>parseInt(v,16));const max=Math.max(...rgb),min=Math.min(...rgb);if(max-min<30||max<65||min>205)return color;const h=hue/60,c=max-min,x=c*(1-Math.abs(h%2-1));const parts=h<1?[c,x,0]:h<2?[x,c,0]:h<3?[0,c,x]:h<4?[0,x,c]:h<5?[x,0,c]:[c,0,x];return hex(parts.map(v=>Math.round(v+min)));}).map(color=>{while(used.has(color)){let v=parseInt(color.slice(1),16);color='#'+((v+1)&0xffffff).toString(16).padStart(6,'0');}used.add(color);return color;});}
for(const species of catalog){
 const sheet=decodePng(readFileSync(resolve(source,species.id+'-source.png')));
 if(sheet.width<256||sheet.height<256)throw new Error('Expected large 4x4 sheet');
 const cells=Array.from({length:16},(_,index)=>{const column=index%4,row=Math.floor(index/4),x=Math.floor(column*sheet.width/4),y=Math.floor(row*sheet.height/4);return bounds(crop(sheet,x,y,Math.floor((column+1)*sheet.width/4)-x,Math.floor((row+1)*sheet.height/4)-y));});
 const palette=paletteOf(cells),side=Math.max(...cells.map(cell=>Math.max(cell.width,cell.height)));
 const output=resolve('frontend-shell/src/companion/packs',species.id+'-v2');mkdirSync(output,{recursive:true});
 const assets={},stages={};
 for(const [stageIndex,stage] of ['child','growing','adult'].entries()){
  const size=[40,47,54][stageIndex];stages[stage]={};
  for(const [row,action] of ['idle','greet','happy','sleep'].entries()){
   const frames=[];
   for(let frame=0;frame<4;frame++){
    const cell=cells[row*4+frame],square={width:side,height:side,data:new Uint8ClampedArray(side*side*4)};
    const dx=Math.floor((side-cell.width)/2),dy=side-cell.height;
    for(let y=0;y<cell.height;y++)square.data.set(cell.data.subarray(y*cell.width*4,(y+1)*cell.width*4),((dy+y)*side+dx)*4);
    const pixels=normalizePixels(square,size,size,palette),canvas={width:64,height:64,data:new Uint8ClampedArray(64*64*4)};
    for(let y=0;y<size;y++)canvas.data.set(pixels.data.subarray(y*size*4,(y+1)*size*4),((56-size+y)*64+Math.floor((64-size)/2))*4);
    const id=`${species.id}-${stage}-${action}-${frame}`,file=id+'.png',encoded=encodePng(canvas),path=resolve(output,file);
    if(existsSync(path)&&!readFileSync(path).equals(encoded))throw new Error('Refusing different existing asset '+path);
    if(!existsSync(path))writeFileSync(path,encoded);
    assets[id]={path:file,role:'character',width:64,height:64};frames.push(id);
   }
   stages[stage][action]={frames,fps:action==='idle'||action==='sleep'?2:5,anchor:{x:32,y:56}};
  }
 }
 const manifest={contractVersion:1,id:species.id+'-v2',speciesId:species.id,style:'pixel',palette,palettes:{mint:palette,amber:tinted(palette,38),lavender:tinted(palette,272)},assets,stages};
 validateCompanionPack(manifest,output);writeFileSync(resolve(output,'manifest.json'),JSON.stringify(manifest,null,2));
 const entries=Object.entries(assets);
 writeFileSync(resolve(output,'catalog.ts'),entries.map(([id],i)=>`import asset${i} from './${id}.png';`).join('\n')+'\nexport const images:Record<string,string>={\n'+entries.map(([id],i)=>`'${id}':asset${i},`).join('\n')+'\n};\n');
 console.log(species.id+': 48 frames validated');
}
