// Micro rabbit: independent authored actions, low quadruped anatomy, native RGBA.
import {readFileSync,writeFileSync} from 'node:fs';
import {resolve} from 'node:path';
import {decodePng,encodePng,cropImage} from './png.mjs';
import {sampleRgba} from './rgba.mjs';
import {atlasRegions} from './atlas.mjs';
import {visibleBounds} from './sheet.mjs';
import {plantedRegistration,translateRgba} from './register-motion.mjs';
import {publishCompanionPack} from './publish-pack.mjs';
const root=resolve('art/companion/reviews/rabbit-v1'),out=resolve('frontend-shell/src/companion/packs/lumen-v8');
const sources={},referenceHeight=163;let fit=1;
function sample(image,scale,cx,baseline){
 const result={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
 for(let y=0;y<256;y++)for(let x=0;x<256;x++)result.data.set(sampleRgba(image,(x-128)/scale+cx,(y-240)/scale+baseline),(y*256+x)*4);
 return result;
}
for(const [action,rows] of [['idle',2],['sleep',2],['greet',2],['look',3],['stretch',3],['walk',4],['turn',2]]){
 const sheet=decodePng(readFileSync(resolve(root,action+'-source.png')));
 if(!sheet.data.some((v,i)=>i%4===3&&v===0))throw new Error(action+': opaque source');
 const frames=atlasRegions(sheet,4,rows).map(r=>cropImage(sheet,r.left,r.top,r.right-r.left,r.bottom-r.top));
 const bounds=frames.map(visibleBounds),scale=referenceHeight/(bounds[0].bottom-bounds[0].top+1);
 const baselines=bounds.map((b,i)=>action==='walk'&&i>=4&&i<=7?Math.max(b.bottom,bounds[4].bottom):b.bottom);
 for(let i=0;i<frames.length;i++){
  const b=bounds[i],im=frames[i],cx=(b.left+b.right)/2;
  if(b.left<2||b.top<2||b.right>=im.width-2||b.bottom>=im.height-2)throw new Error(action+'/'+i+': crop clips silhouette');
  fit=Math.min(fit,112/(Math.max(cx-b.left,b.right-cx)*scale),224/((baselines[i]-b.top+1)*scale));
 }
 sources[action]={frames,bounds,baselines,scale};
}
if(fit<.8)throw new Error('Rabbit pose exceeds shared canvas; redraw');
const seated=decodePng(readFileSync(resolve(root,'seated-source.png'))),b=visibleBounds(seated);
const neutral=sample(seated,referenceHeight/(b.bottom-b.top+1)*fit,(b.left+b.right)/2,b.bottom),built={};
for(const [action,s]of Object.entries(sources)){
 built[action]=s.frames.map((im,i)=>{
  const b=s.bounds[i],canvas=sample(im,s.scale*fit,(b.left+b.right)/2,s.baselines[i]);
  // Align planted lower torsos; leave authored travel/rotation geometry intact.
  if(['walk','turn'].includes(action))return canvas;
  const correction=plantedRegistration(neutral,canvas),cb=visibleBounds(canvas);
  correction.dx=Math.max(8-cb.left,Math.min(247-cb.right,correction.dx));
  return translateRgba(canvas,correction);
 });
 if(action!=='walk'&&action!=='turn'){built[action][0]=neutral;built[action][built[action].length-1]=neutral;}
}
built.turn[0]=neutral;
const side=built.turn.at(-1),walk=built.walk;
const phases={turnOut:built.turn,walkStart:[side,built.turn[6],walk[15],walk[0]],walkStop:[walk[0],walk[15],built.turn[6],side],turnIn:[...built.turn].reverse()};
const assets={},ids={},binaries=new Map();
function emit(action,frames){
 ids[action]=frames.map((image,i)=>{const id='lumen-'+action.toLowerCase()+'-'+i,path=id+'.png';binaries.set(path,encodePng(image,{adaptive:true}));assets[id]={path,role:'character',width:256,height:256};return id;});
 return ids[action];
}
for(const action of ['idle','sleep','greet','look','stretch','walk'])emit(action,built[action]);
for(const [action,frames]of Object.entries(phases))emit(action,frames);
function clip(frames,fps,loop,staticFrame=0,durations){return {frames,fps,loop,staticFrame,anchor:{x:128,y:240},sequence:frames.map((_,frame)=>({frame,durationMs:durations?.[frame]??Math.round((frame+1)*1000/fps)-Math.round(frame*1000/fps)}))};}
const base={idle:clip(ids.idle,12,true,0,[3400,100,90,70,110,70,90,270]),sleep:clip(ids.sleep,2,true,4,[250,400,750,750,1500,750,400,250]),greet:clip(ids.greet,12,false,2,[300,180,650,180,180,180,180,300]),happy:clip(ids.greet,12,false,2,[250,200,800,200,200,200,200,250])};
const look=clip(ids.look,6,false,3);
// The authored sheet has an extra right glance in its middle row; skip it.
look.sequence=[0,1,2,3,2,0,8,9,9,10,11,0].map((frame,i)=>({frame,durationMs:i===3||i===8?1300:200}));
const manifest={contractVersion:2,id:'lumen-v8',speciesId:'lumen',style:'pixel',frameSize:256,colorMode:'rgba',paletteHues:{mint:null,amber:38,lavender:272},stageScale:{child:160/216,growing:188/216,adult:1},assets,stages:{child:base,growing:base,adult:base},ambient:{look,stretch:clip(ids.stretch,12,false,6,[250,160,160,160,160,180,600,180,160,160,160,250]),walk:clip(ids.walk,12,true)},locomotion:{stride:56,contacts:[0],phases:{stand:clip([ids.walkStop.at(-1)],2,true),turnOut:clip(ids.turnOut,10,false),walkStart:clip(ids.walkStart,8,false),walkStop:clip(ids.walkStop,8,false),turnIn:clip(ids.turnIn,10,false)}}};
const result=publishCompanionPack(manifest,binaries,out);
writeFileSync(resolve(root,'runtime-build-report.json'),JSON.stringify({id:manifest.id,fit,...result,contacts:manifest.locomotion.contacts,phaseEndpoints:'decoded pixels checked'},null,2)+'\n');
console.log(manifest.id,{fit,...result});
