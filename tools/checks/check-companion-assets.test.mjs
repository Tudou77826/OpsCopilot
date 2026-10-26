import { test } from 'node:test';
import assert from 'node:assert/strict';
import { encodePng } from '../pixel/png.mjs';
import { validateCompanionPack } from './check-companion-assets.mjs';

function fixture() {
  const image = { width: 64, height: 64, data: new Uint8ClampedArray(64 * 64 * 4) };
  image.data.set([255,255,255,255], (56 * 64 + 32) * 4);
  const binary = encodePng(image);
  const assets = {}, stages = {};
  for (const stage of ['child','growing','adult']) {
    stages[stage] = {};
    for (const action of ['idle','happy','greet','sleep']) {
      const frames = [0,1].map(frame => `${stage}-${action}-${frame}`);
      for (const id of frames) assets[id] = { path: `assets/${id}.png`, role: 'character', width:64, height:64 };
      stages[stage][action] = { frames, fps:4, anchor:{x:32,y:56} };
    }
  }
  return { manifest:{ contractVersion:1, id:'lumen', speciesId:'lumen', style:'pixel', palette:['#ffffff','#000000'], palettes:{mint:['#ffffff','#000000'],amber:['#ffff00','#000000'],lavender:['#ff00ff','#000000']}, assets, stages }, load:()=>binary };
}
test('accepts a complete three-stage four-action pixel pack', () => {
  const {manifest,load} = fixture();
  assert.equal(validateCompanionPack(manifest, process.cwd(), load).assets, 24);
});
test('rejects missing animations, misaligned anchors, unsafe paths and wrong dimensions', () => {
  for (const change of [
    m => { delete m.stages.adult.sleep; },
    m => { m.stages.child.happy.anchor.x = 31; },
    m => { m.assets['child-idle-0'].path = '../outside.png'; },
    m => { m.assets['child-idle-0'].width = 128; },
    m => { m.palette = ['#000000','#010101']; },
    m => { m.palettes.amber = ['#ffffff']; },
    m => { m.price = 35; },
  ]) {
    const {manifest,load} = fixture();change(manifest);
    assert.throws(() => validateCompanionPack(manifest,process.cwd(),load));
  }
});
test('rejects fractional alpha and empty character frames', () => {
  for (const alpha of [0,128]) {
    const {manifest} = fixture();const image={width:64,height:64,data:new Uint8ClampedArray(64*64*4)};
    image.data.set([255,255,255,alpha]);
    assert.throws(()=>validateCompanionPack(manifest,process.cwd(),()=>encodePng(image)));
  }
});

function detailedFixture() {
 const {manifest}=fixture();
 manifest.contractVersion=2;manifest.frameSize=256;manifest.colorMode='rgba';manifest.paletteHues={mint:null,amber:38,lavender:272};
 delete manifest.palette;delete manifest.palettes;
 for(const asset of Object.values(manifest.assets)){asset.width=256;asset.height=256;}
 for(const clips of Object.values(manifest.stages)) for(const clip of Object.values(clips)){clip.anchor={x:128,y:224};clip.sequence=[{frame:0,durationMs:3600},{frame:1,durationMs:100}];}
 const image={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
 image.data.set([117,93,201,137],(224*256+128)*4);
 return {manifest,load:()=>encodePng(image)};
}
test('accepts detailed full-color soft-alpha frames without forcing an indexed palette',()=>{
 const {manifest,load}=detailedFixture();assert.equal(validateCompanionPack(manifest,process.cwd(),load).assets,24);
});
test('checks measured scene bounds against the actual animation frames',()=>{
 const {manifest,load}=detailedFixture();
 manifest.sceneBounds={left:124,top:220,right:133,bottom:229};
 assert.equal(validateCompanionPack(manifest,process.cwd(),load).assets,24);
 for(const bounds of [ {...manifest.sceneBounds,top:221}, {...manifest.sceneBounds,bottom:256}, {...manifest.sceneBounds,extra:1} ]) {
  assert.throws(()=>validateCompanionPack({...manifest,sceneBounds:bounds},process.cwd(),load),/Scene bounds|scene bounds/);
 }
});
test('rejects invalid detailed canvas, palette hue and pose timing',()=>{
 for(const change of [m=>{m.frameSize=64;},m=>{m.paletteHues.amber=360;},m=>{m.stages.child.idle.sequence[0].frame=2;},m=>{m.stages.child.idle.sequence[0].durationMs=0;},m=>{m.stageScale={child:1,growing:.5,adult:.8};},m=>{m.stages.child.idle.loop='yes';}]){
  const {manifest,load}=detailedFixture();change(manifest);assert.throws(()=>validateCompanionPack(manifest,process.cwd(),load));
 }
});
test('rejects an opaque motion silhouette clipped against the canvas edge',()=>{
 const {manifest}=detailedFixture();manifest.stageScale={child:.74,growing:.87,adult:1};
 const image={width:256,height:256,data:new Uint8ClampedArray(256*256*4)};
 image.data.set([117,93,201,255],128*4);
 assert.throws(()=>validateCompanionPack(manifest,process.cwd(),()=>encodePng(image)),/canvas margin/);
});
