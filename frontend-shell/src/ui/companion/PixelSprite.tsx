import React, { useEffect, useMemo, useRef, useState } from 'react';
import { assetUrl, packForSpecies, paletteAsset } from '../../companion/assets';
import { companionClip, companionDisplayScale, playbackStep, type CompanionAction } from '../../companion/content';
import type { CompanionPalette, CompanionStage, MedalKind, Medal } from '../../companion/types';
import './companion.css';
import { medalImages } from '../../companion/medals/v2/catalog';

export function PixelSprite({ species = 'lumen', stage = 'child', palette = 'mint', action = 'idle', scale = 3, name = '微光', animate = true, onComplete, onReady, onFrame, startedAt, repetitions, entryFrame, stopContact, mirror = false }: {
  species?: string; stage?: CompanionStage; palette?: CompanionPalette; action?: CompanionAction; scale?: 1 | 2 | 3; name?: string; animate?: boolean;
  onComplete?: () => void; onReady?: () => void; onFrame?: (frame: number, finished?: boolean) => void;
  /** null prepares the clip; a shared epoch starts pose playback and spatial motion. */
  startedAt?: number | null; repetitions?: number; entryFrame?: string; stopContact?: number; mirror?: boolean;
}) {
  const pack = packForSpecies(species), base = companionClip(pack, stage, action, stopContact);
  const clip = useMemo(() => entryFrame ? {...base,frames:[entryFrame,...base.frames.slice(1)]} : base,[base,entryFrame]);
  const key = species + ':' + action + ':' + clip.frames.join(',') + ':' + palette;
  const [images,setImages] = useState<{key:string;urls:Record<string,string>}>(), [failed,setFailed] = useState(false);
  const [frame,setFrame] = useState(0);
  const [motion,setMotion] = useState<'running'|'paused'|'reduced'>(()=>window.matchMedia('(prefers-reduced-motion: reduce)').matches?'reduced':animate&&!document.hidden?'running':'paused');
  const fallback = clip.staticFrame ?? 0;
  const required = motion==='running' ? clip.frames : [clip.frames[fallback]];
  const requiredKey = required.join(',');
  const imageRef=useRef<HTMLImageElement>(null),callbacks=useRef({onComplete,onReady,onFrame}),lastSource=useRef<string>();
  useEffect(()=>{callbacks.current={onComplete,onReady,onFrame};},[onComplete,onReady,onFrame]);
  useEffect(()=>{
    if(images?.key===key&&required.every(id=>images.urls[id])) return;
    let live=true;setFailed(false);
    void Promise.all(required.map(async id=>[id,images?.key===key&&images.urls[id]?images.urls[id]:await paletteAsset(id,palette)] as const)).then(entries=>{
      if(live)setImages({key,urls:{...(images?.key===key?images.urls:{}),...Object.fromEntries(entries)}});
    }).catch(()=>{if(live)setFailed(true);});
    return()=>{live=false;};
  },[key,requiredKey,palette,images]);
  useEffect(()=>{
    const preference=window.matchMedia('(prefers-reduced-motion: reduce)');let shown=true;
    const update=()=>setMotion(preference.matches?'reduced':animate&&shown&&!document.hidden?'running':'paused');
    const observer=typeof IntersectionObserver==='undefined'?undefined:new IntersectionObserver(entries=>{shown=entries[0]?.isIntersecting??false;update();});
    if(imageRef.current)observer?.observe(imageRef.current);
    update();preference.addEventListener('change',update);document.addEventListener('visibilitychange',update);
    return()=>{observer?.disconnect();preference.removeEventListener('change',update);document.removeEventListener('visibilitychange',update);};
  },[animate]);
  const ready=images?.key===key&&required.every(id=>images.urls[id]);
  const preparing=startedAt===null;
  useEffect(()=>{if(ready&&motion==='running'&&(startedAt===undefined||preparing))callbacks.current.onReady?.();},[ready,motion,key,preparing]);
  useEffect(()=>{
    if(!ready||motion!=='running'||startedAt===null){setFrame(0);return;}
    const start=startedAt??performance.now();let timer:ReturnType<typeof setTimeout>;
    const advance=()=>{
      const step=playbackStep(clip,performance.now()-start,repetitions);
      setFrame(step.frame);callbacks.current.onFrame?.(step.frame,step.waitMs===undefined);
      if(step.waitMs===undefined)callbacks.current.onComplete?.();
      else if(clip.frames.length>1)timer=setTimeout(advance,step.waitMs);
    };
    advance();return()=>clearTimeout(timer);
  },[ready,motion,clip,key,startedAt,repetitions]);
  const shownFrame=motion==='running'?Math.min(frame,clip.frames.length-1):fallback;
  const id=clip.frames[shownFrame],native=pack.assets[clip.frames[0]];
  const source=ready?images!.urls[id]:lastSource.current??assetUrl(clip.frames[0]);
  if(ready)lastSource.current=source;
  const stageScale=companionDisplayScale(pack,stage);
  const frontEndpoint=action==='turnOut'&&shownFrame===0||action==='turnIn'&&shownFrame===clip.frames.length-1;
  const flipped=mirror&&!frontEndpoint;
  return failed?<span role="status">伙伴图像暂不可用</span>:<img ref={imageRef} className="oc-pixel-sprite" src={source} width={64*scale} height={64*scale}
    style={{transformOrigin:String(clip.anchor.x/native.width*100)+'% '+String(clip.anchor.y/native.height*100)+'%',transform:'scale('+stageScale+')'+(flipped?' scaleX(-1)':'')}}
    alt={name} data-species={species} data-stage={stage} data-palette={palette} data-action={action} data-frame={shownFrame}
    data-motion={ready?motion:'paused'} data-ready={ready} onError={()=>setFailed(true)}/>;
}
export function PixelMedal({ kind, tier = 'locked', small = false }: { kind: MedalKind; tier?: Medal['tier'] | 'locked'; small?: boolean }) {
  return <img className="oc-pixel-medal" data-tier={tier} src={medalImages['medal-'+kind+'-'+tier]} width={small?32:64} height={small?32:64} alt="" aria-hidden="true" decoding="async"/>;
}
