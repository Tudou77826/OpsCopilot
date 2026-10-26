import { readFileSync, readdirSync, existsSync } from 'node:fs';
import { dirname, resolve, sep, isAbsolute } from 'node:path';
import { fileURLToPath } from 'node:url';
import { decodePng } from '../pixel/png.mjs';
import { checkMedalAssets } from './check-medal-assets.mjs';
import { measureSceneBounds } from '../pixel/measure-scene-bounds.mjs';

const stages = ['child', 'growing', 'adult'];
const actions = ['idle', 'happy', 'greet', 'sleep'];
const idPattern = /^[a-z0-9][a-z0-9-]{0,63}$/;
const sizes = { character: [64, 64], icon: [32, 32], background: [320, 180] };
const fail = message => { throw new Error(message); };
const onlyKeys = (value, allowed, label) => {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).some(key => !allowed.includes(key))) fail(`Unexpected fields in ${label}`);
};

export function validateCompanionPack(manifest, root, load = path => readFileSync(path)) {
  const detailed=manifest?.contractVersion===2;
  onlyKeys(manifest, detailed ? ['contractVersion','id','speciesId','style','frameSize','colorMode','paletteHues','stageScale','sceneBounds','ambient','locomotion','assets','stages'] : ['contractVersion','id','speciesId','style','palette','palettes','assets','stages'], 'pack');
  if (![1,2].includes(manifest.contractVersion) || manifest.style !== 'pixel' || !idPattern.test(manifest.id) || !idPattern.test(manifest.speciesId)) fail('Invalid companion pack identity or version');
  if(detailed) {
    if(manifest.frameSize!==256||manifest.colorMode!=='rgba') fail('Detailed frames require a 256px RGBA canvas');
    if(!manifest.paletteHues||Object.keys(manifest.paletteHues).sort().join()!=='amber,lavender,mint'||manifest.paletteHues.mint!==null||!['amber','lavender'].every(id=>Number.isFinite(manifest.paletteHues[id])&&manifest.paletteHues[id]>=0&&manifest.paletteHues[id]<360)) fail('Invalid full-color palette variants');
    if(manifest.stageScale && (Object.keys(manifest.stageScale).sort().join()!==[...stages].sort().join()||!stages.every(stage=>Number.isFinite(manifest.stageScale[stage])&&manifest.stageScale[stage]>=.5&&manifest.stageScale[stage]<=1)||!(manifest.stageScale.child<manifest.stageScale.growing&&manifest.stageScale.growing<manifest.stageScale.adult))) fail('Invalid shared-art stage scale');
  } else {
    if (!Array.isArray(manifest.palette) || manifest.palette.length < 2 || manifest.palette.length > 32 || new Set(manifest.palette).size !== manifest.palette.length || manifest.palette.some(color => !/^#[0-9a-f]{6}$/.test(color))) fail('Palette must contain 2-32 unique lowercase RGB colors');
    if (!manifest.palettes || Object.keys(manifest.palettes).sort().join() !== ['amber', 'lavender', 'mint'].join()) fail('Three named palette variants required');
    for (const [id, colors] of Object.entries(manifest.palettes)) {
      if (!Array.isArray(colors) || colors.length !== manifest.palette.length || new Set(colors).size !== colors.length || colors.some(color => !/^#[0-9a-f]{6}$/.test(color))) fail(`Invalid indexed palette ${id}`);
    }
  }
  if (!manifest.assets || typeof manifest.assets !== 'object' || Array.isArray(manifest.assets) || Object.keys(manifest.assets).length === 0) fail('Pack must contain assets');
  const allowedColors = new Set((manifest.palette ?? []).map(color => Number.parseInt(color.slice(1), 16)));
  const actual = new Map();
  const paths = new Set();
  let bytes = 0;
  for (const [id, asset] of Object.entries(manifest.assets)) {
    onlyKeys(asset, ['path','role','width','height'], id);
    if (!idPattern.test(id) || !asset || !sizes[asset.role]) fail(`Invalid asset ${id}`);
    if (typeof asset.path !== 'string' || isAbsolute(asset.path) || asset.path.includes('\\') || asset.path.includes(':') || !asset.path.endsWith('.png') || asset.path.split('/').some(part => !part || part === '.' || part === '..')) fail(`Unsafe asset path ${id}`);
    const path = resolve(root, asset.path);
    if (!path.startsWith(resolve(root) + sep) || paths.has(path)) fail(`Duplicate or escaped asset ${id}`);
    paths.add(path);
    const data = load(path); bytes += data.length;
    if(detailed&&asset.role!=='character') fail('Detailed roster packs contain only characters');
    if (data.length > (detailed ? 180000 : asset.role === 'background' ? 150000 : 20000)) fail(`Asset budget exceeded: ${id}`);
    const [width, height] = detailed ? [256,256] : sizes[asset.role];
    // Check the fixed canvas before decompression of candidate input.
    if (data.length < 33 || data.toString('hex', 0, 8) !== '89504e470d0a1a0a' || data.toString('ascii', 12, 16) !== 'IHDR' || data.readUInt32BE(16) !== width || data.readUInt32BE(20) !== height) fail(`Invalid PNG canvas: ${id}`);
    const image = decodePng(data);
    if (image.width !== width || image.height !== height || asset.width !== width || asset.height !== height) fail(`Invalid dimensions: ${id}`);
    let opaque = 0, transparent = 0;
    for (let i = 0; i < image.data.length; i += 4) {
      const alpha = image.data[i + 3];
      if (!detailed && alpha !== 0 && alpha !== 255) fail(`Non-pixel alpha: ${id}`);
      if (alpha === 0) { transparent++; continue; }
      opaque++;
      if(detailed&&manifest.stageScale&&alpha>=128) {
        const x=(i/4)%width,y=Math.floor(i/4/width);
        if(x<8||x>=width-8||y<8||y>=height-8) fail(`Motion silhouette touches the canvas margin: ${id}`);
      }
      const rgb = (image.data[i] << 16) | (image.data[i + 1] << 8) | image.data[i + 2];
      if (!detailed && !allowedColors.has(rgb)) fail(`Color outside shared palette: ${id}`);
    }
    if (opaque === 0 || (asset.role !== 'background' && transparent === 0) || (asset.role === 'background' && transparent !== 0)) fail(`Invalid transparency: ${id}`);
    actual.set(id, { ...asset, image });
  }
  if (bytes > (detailed ? 8000000 : 2000000)) fail('Pack exceeds asset budget');
  if(detailed && manifest.sceneBounds!==undefined) {
    onlyKeys(manifest.sceneBounds,['left','top','right','bottom'],'scene bounds');
    const measured=measureSceneBounds([...actual.values()].filter(asset=>asset.role==='character').map(asset=>asset.image));
    if(Object.entries(measured).some(([key,value])=>manifest.sceneBounds[key]!==value)) fail('Scene bounds do not match the artwork; regenerate them');
  }
  if (!manifest.stages || Object.keys(manifest.stages).sort().join() !== [...stages].sort().join()) fail('Exactly three stages required');
  const groups=stages.map(stage=>({stage,clips:manifest.stages[stage],expected:actions}));
  if(manifest.ambient!==undefined) groups.push({stage:'ambient',clips:manifest.ambient,expected:['look','stretch','walk']});
  if(manifest.locomotion!==undefined) {
    onlyKeys(manifest.locomotion,['stride','contacts','phases','stopByContact'],'locomotion');
    if(!detailed||!manifest.ambient||!Number.isFinite(manifest.locomotion.stride)||manifest.locomotion.stride<=0||manifest.locomotion.stride>128||!Array.isArray(manifest.locomotion.contacts)||!manifest.locomotion.contacts.length||new Set(manifest.locomotion.contacts).size!==manifest.locomotion.contacts.length||manifest.locomotion.contacts.some(frame=>!Number.isInteger(frame)||frame<0||frame>=manifest.ambient.walk.frames.length))fail('Invalid locomotion stride or contacts');
    groups.push({stage:'locomotion',clips:manifest.locomotion.phases,expected:['stand','turnOut','walkStart','walkStop','turnIn']});
    const alternates=manifest.locomotion.contacts.filter(frame=>frame!==0).map(String);
    if(alternates.length)groups.push({stage:'stops',clips:manifest.locomotion.stopByContact,expected:alternates});
  }
  for (const {stage,clips,expected} of groups) {
    if (!clips || Object.keys(clips).sort().join() !== [...expected].sort().join()) fail(`${stage}: complete actions required`);
    let anchor;
    for (const action of expected) {
      const clip = clips[action];
      onlyKeys(clip, detailed ? ['frames','fps','anchor','sequence','loop','staticFrame'] : ['frames','fps','anchor'], `${stage}/${action}`);
      if (!clip || !Array.isArray(clip.frames) || clip.frames.length < (stage==='locomotion'&&action==='stand'?1:2) || clip.frames.length > (detailed?24:4) || new Set(clip.frames).size !== clip.frames.length || !Number.isInteger(clip.fps) || clip.fps < 2 || clip.fps > (detailed?24:8) || (clip.loop!==undefined&&typeof clip.loop!=='boolean') || (clip.staticFrame!==undefined&&(!Number.isInteger(clip.staticFrame)||clip.staticFrame<0||clip.staticFrame>=clip.frames.length))) fail(`Invalid clip ${stage}/${action}`);
      const canvasSize=detailed?256:64;
      if (!clip.anchor || !Number.isInteger(clip.anchor.x) || !Number.isInteger(clip.anchor.y) || clip.anchor.x < 0 || clip.anchor.x >= canvasSize || clip.anchor.y < 0 || clip.anchor.y >= canvasSize) fail(`Invalid anchor ${stage}/${action}`);
      onlyKeys(clip.anchor, ['x','y'], 'anchor');
      const value = `${clip.anchor.x},${clip.anchor.y}`;
      if(stage==='ambient' && ((!manifest.locomotion&&clip.frames.length!==8) || clip.loop!==(action==='walk') || value!==`${manifest.stages.child.idle.anchor.x},${manifest.stages.child.idle.anchor.y}`)) fail('Ambient poses require complete frames, matching foot anchors and explicit playback');
      if(stage==='locomotion'&&(clip.loop!==(action==='stand')||value!==`${manifest.stages.child.idle.anchor.x},${manifest.stages.child.idle.anchor.y}`))fail('Locomotion phases require matching anchors and explicit playback');
      if(stage==='stops'&&(clip.loop!==false||value!==`${manifest.stages.child.idle.anchor.x},${manifest.stages.child.idle.anchor.y}`))fail('Contact stops require matching anchors and finite playback');
      if (anchor !== undefined && anchor !== value) fail(`${stage}: foot anchors must agree across actions`);
      anchor = value;
      if(detailed) {
        if(!Array.isArray(clip.sequence)||!clip.sequence.length||clip.sequence.length>32) fail(`Invalid pose sequence ${stage}/${action}`);
        for(const pose of clip.sequence) {
          onlyKeys(pose,['frame','durationMs'],'pose');
          if(!Number.isInteger(pose.frame)||pose.frame<0||pose.frame>=clip.frames.length||!Number.isInteger(pose.durationMs)||pose.durationMs<50||pose.durationMs>6000) fail(`Invalid pose timing ${stage}/${action}`);
        }
        if(clip.sequence.reduce((sum,pose)=>sum+pose.durationMs,0)>30000) fail('Animation cycle is too long');
      }
      for (const frame of clip.frames) if (actual.get(frame)?.role !== 'character') fail(`Missing character frame ${frame}`);
    }
  }
  if(manifest.locomotion) {
    const phases=manifest.locomotion.phases,walk=manifest.ambient.walk,idle=manifest.stages.child.idle;
    const same=(a,b)=>Buffer.from(actual.get(a).image.data).equals(Buffer.from(actual.get(b).image.data));
    const first=clip=>clip.frames[0],last=clip=>clip.frames.at(-1);
    for(const [a,b] of [[first(idle),first(phases.turnOut)],[last(phases.turnOut),first(phases.walkStart)],[last(phases.walkStart),first(walk)],[first(walk),first(phases.walkStop)],[last(phases.walkStop),first(phases.stand)],[first(phases.stand),first(phases.turnIn)],[last(phases.turnIn),first(idle)]])if(!same(a,b))fail('Motion endpoints do not share the same pose');
    for(const [contact,stop]of Object.entries(manifest.locomotion.stopByContact??{}))if(!same(first(stop),walk.frames[Number(contact)])||!same(last(stop),first(phases.stand)))fail('Contact stop endpoints do not match gait and stand');
  }
  return { assets: actual.size, bytes };
}

export function checkCompanionAssets() {
  checkMedalAssets();
  const root = resolve(dirname(fileURLToPath(import.meta.url)), '../../frontend-shell/src/companion/packs');
  const packs = existsSync(root) ? readdirSync(root, { withFileTypes: true }).filter(entry => entry.isDirectory()) : [];
  if (!packs.some(pack => pack.name === 'lumen-v1')) fail('Required approved lumen-v1 pack is missing');
  const species = JSON.parse(readFileSync(resolve(root, '../../../../pkg/companion/species.json'), 'utf8'));
  for (const item of species) if (!packs.some(pack => pack.name === `${item.id}-v${item.id==='lumen'?8:7}`)) fail(`Required species pack is missing: ${item.id}`);
  for (const pack of packs) {
    const folder = resolve(root, pack.name);
    const manifest = JSON.parse(readFileSync(resolve(folder, 'manifest.json'), 'utf8'));
    if(pack.name.endsWith('-v5')&&!manifest.ambient) fail(`Required ambient clips missing: ${pack.name}`);
    if((pack.name.endsWith('-v6')||pack.name.endsWith('-v7')||pack.name.endsWith('-v8'))&&!manifest.locomotion) fail(`Required locomotion phases missing: ${pack.name}`);
    const result = validateCompanionPack(manifest, folder);
    if (pack.name === 'lumen-v1') {
      for (const kind of ['knowledge-archive','script-created','quick-command-created','case-completed','knowledge-read','script-used','quick-command-used','active-day']) {
        for (const tier of ['locked','bronze','silver','gold']) if (manifest.assets[`medal-${kind}-${tier}`]?.role !== 'icon') fail(`Missing release medal ${kind}/${tier}`);
      }
      if (manifest.assets['profile-background']?.role !== 'background') fail('Missing release profile background');
    }
    console.log(`${pack.name}: ${result.assets} assets, ${result.bytes} bytes`);
  }
}
if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) checkCompanionAssets();
