import React, {useEffect,useRef,useState} from 'react';
import {PixelSprite} from './PixelSprite';
import {packForSpecies} from '../../companion/assets';
import {clipDuration,companionClip,companionDisplayScale,type CompanionAction} from '../../companion/content';
import type {CompanionPalette,CompanionStage} from '../../companion/types';
import {roamingRange} from '../../companion/ambientMotion';
import {ambientAction,ambientDelay,planSteps,responseSequence,walkSequence,type MotionSegment} from '../../companion/motionSequence';

/** A small sequence player coordinates authored phases and compositor travel. No business writes. */
export function SceneCompanion({species,stage,palette,name,action,visible,size,onInteract,onComplete}:{
  species?:string;stage:CompanionStage;palette?:CompanionPalette;name:string;action:CompanionAction;visible:boolean;size:string;onInteract():void;onComplete():void;
}) {
  const button=useRef<HTMLButtonElement>(null),travel=useRef<Animation>(),timer=useRef<ReturnType<typeof setTimeout>>();
  const position=useRef(0),next=useRef(0),direction=useRef<-1|1>(1),queue=useRef<MotionSegment[]>([]),pending=useRef<'greet'|'happy'>(),lastFrame=useRef(0);
  const [current,setCurrent]=useState<MotionSegment>({action:'idle',facing:0}),[epoch,setEpoch]=useState<number|null>(null),[running,setRunning]=useState(false);
  const currentRef=useRef(current),completion=useRef(onComplete),active=useRef(running);
  currentRef.current=current;completion.current=onComplete;active.current=running;
  const pack=packForSpecies(species),scale=companionDisplayScale(pack,stage);
  const locomotion=pack.contractVersion===2?pack.locomotion:undefined;
  function transform(x:number){return 'translateX(calc(-50% + '+x+'px))';}
  function freezeTravel(destination?:number) {
    const element=button.current;
    if(travel.current&&element) {
      position.current=destination??new DOMMatrixReadOnly(getComputedStyle(element).transform).m41+element.offsetWidth/2;
      travel.current.cancel();travel.current=undefined;element.style.transform=transform(position.current);
    }
  }
  function select(segment:MotionSegment) {lastFrame.current=0;setEpoch(null);setCurrent(segment);currentRef.current=segment;}
  function play(sequence:MotionSegment[]) {if(!sequence.length)return;queue.current=sequence.slice(1);select(sequence[0]);}
  function respond(intent:'greet'|'happy') {
    pending.current=undefined;queue.current=[];
    const state=currentRef.current;
    if(state.action==='walk'||state.action==='walkStart') {
      const entry=companionClip(pack,stage,state.action).frames[lastFrame.current];
      play([{action:'walkStop',facing:state.facing,entryFrame:entry,stopContact:state.action==='walk'?lastFrame.current:undefined},...responseSequence(state.facing,intent)]);
    } else play(responseSequence(state.action==='turnIn'?0:state.facing,intent));
  }
  function complete() {
    const state=currentRef.current;
    if(state.action==='walk')freezeTravel(state.destination);
    if(pending.current) {respond(pending.current);return;}
    const following=queue.current.shift();
    if(following)select(following);
    else select({action:state.facing===0?'idle':'stand',facing:state.facing});
    if(state.action==='greet'||state.action==='happy')completion.current();
  }
  function frameChanged(frame:number,finished=false) {
    lastFrame.current=frame;
    if(!finished&&pending.current&&currentRef.current.action==='walk'&&locomotion?.contacts.includes(frame)) {
      freezeTravel();respond(pending.current);
    }
  }
  function ready() {
    if(!active.current||epoch!==null)return;
    const start=performance.now();setEpoch(start);
    const state=currentRef.current,element=button.current;
    if(state.action==='walk'&&state.destination!==undefined&&element&&typeof element.animate==='function') {
      const duration=clipDuration(companionClip(pack,stage,'walk'))*(state.repetitions??1);
      const motion=element.animate([{transform:transform(position.current)},{transform:transform(state.destination)}],{duration,easing:'linear',fill:'forwards'});
      motion.startTime=start;travel.current=motion;
      // Pose completion owns the next phase; the compositor never switches the image.
    }
  }
  useEffect(()=>{
    const element=button.current;if(!element)return;
    const preference=window.matchMedia('(prefers-reduced-motion: reduce)');let inView=true;
    const update=()=>setRunning(visible&&!document.hidden&&!preference.matches&&inView);
    const observer=typeof IntersectionObserver==='undefined'?undefined:new IntersectionObserver(entries=>{inView=entries[0]?.isIntersecting??false;update();});
    observer?.observe(element);update();preference.addEventListener('change',update);document.addEventListener('visibilitychange',update);
    return()=>{observer?.disconnect();preference.removeEventListener('change',update);document.removeEventListener('visibilitychange',update);};
  },[visible]);
  useEffect(()=>{
    const element=button.current,scene=element?.parentElement;if(!element||!scene)return;
    const fit=()=>{
      freezeTravel();const bounds=roamingRange(scene.clientWidth,element.offsetWidth,scale);
      position.current=Math.max(bounds.min,Math.min(bounds.max,position.current));
      element.style.left=String(bounds.center)+'px';element.style.transform=transform(position.current);
      queue.current=[];pending.current=undefined;select({action:currentRef.current.facing===0?'idle':'stand',facing:currentRef.current.facing});
    };
    position.current=0;next.current=0;direction.current=1;pending.current=undefined;currentRef.current={action:'idle',facing:0};fit();
    const observer=typeof ResizeObserver==='undefined'?undefined:new ResizeObserver(fit);observer?.observe(scene);
    return()=>{observer?.disconnect();freezeTravel();};
  },[species,stage,scale]);
  useEffect(()=>{
    if(!running){freezeTravel();queue.current=[];pending.current=undefined;select({action:currentRef.current.facing===0?'idle':'stand',facing:currentRef.current.facing});}
    else setEpoch(null);
    return()=>{clearTimeout(timer.current);freezeTravel();};
  },[running]);
  useEffect(()=>{
    if(action!=='greet'&&action!=='happy') {
      if(action==='idle'&&!running&&(currentRef.current.action==='greet'||currentRef.current.action==='happy'))select({action:'idle',facing:0});
      return;
    }
    if(!running) {select({action,facing:0});return;}
    const state=currentRef.current;
    if(['walk','turnOut','walkStart','walkStop','turnIn','greet','happy'].includes(state.action))pending.current=action;
    else {freezeTravel();respond(action);}
  },[action]);
  useEffect(()=>{
    if(!running||!locomotion||epoch===null||current.action!=='idle'&&current.action!=='stand')return;
    timer.current=setTimeout(()=>{
      const selected=ambientAction(next.current++);
      if(selected==='walk'&&button.current) {
        const bounds=roamingRange(button.current.parentElement!.clientWidth,button.current.offsetWidth,scale);
        const planned=planSteps(position.current,direction.current,bounds.min,bounds.max,locomotion.stride,button.current.offsetWidth,scale);
        if(planned.repetitions>0){play(walkSequence(currentRef.current.facing,direction.current,planned.repetitions,planned.destination));direction.current=direction.current===1?-1:1;}
        else play(responseSequence(currentRef.current.facing,'look'));
      } else play(responseSequence(currentRef.current.facing,selected==='walk'?'look':selected));
    },ambientDelay(Math.random()));
    return()=>clearTimeout(timer.current);
  },[running,epoch,current,locomotion,scale]);
  useEffect(()=>()=>{clearTimeout(timer.current);freezeTravel();},[]);
  return <button ref={button} type="button" className="oc-companion-touch oc-companion-roaming" style={{width:size,height:size}}
    data-ambient={current.action} data-facing={current.facing===0?'front':current.facing<0?'left':'right'} data-roaming={running&&current.action==='walk'}
    aria-label={'与'+name+'打招呼'} title="点击伙伴，打个招呼" onClick={onInteract}>
    <PixelSprite species={species} stage={stage} palette={palette} name={name} action={current.action} scale={3} animate={running}
      startedAt={epoch} repetitions={current.repetitions} entryFrame={current.entryFrame} stopContact={current.stopContact} mirror={current.facing<0}
      onReady={ready} onFrame={frameChanged} onComplete={complete}/>
  </button>;
}
