import { expect, it } from 'vitest';
import { animationFrame, animationFrameIndex, animationStep, visibleFrameHeightRatio, companionClip, companionDisplayScale, clipDuration, playbackStep } from './content';
import { packForSpecies } from './assets';
import { roamingRange } from './ambientMotion';
import { planSteps } from './motionSequence';
it('renders every species at 70% while keeping travel stride and floor limits consistent',()=>{
 for(const species of ['lumen','ember','moss','bolt','astra','tide','sage'])for(const stage of ['child','growing','adult'] as const){
  const pack=packForSpecies(species);if(pack.contractVersion!==2)throw new Error('Detailed pack required');
  const scale=companionDisplayScale(pack,stage);
  expect(scale).toBeCloseTo(.7*pack.stageScale![stage]);
  const range=roamingRange(420,320,scale),travel=planSteps(0,1,range.min,range.max,pack.locomotion!.stride,320,scale);
  expect(travel.repetitions).toBeGreaterThan(0);
  expect(travel.destination).toBeLessThanOrEqual(range.max);
  expect(travel.destination/travel.repetitions).toBeCloseTo(pack.locomotion!.stride*320/256*scale);
 }
});
it('finishes a whole number of walk cycles on the first grounded contact',()=>{
 const clip={frames:['contact','passing','other-contact','passing-back'],fps:16,anchor:{x:128,y:240},loop:true,sequence:[{frame:0,durationMs:63},{frame:1,durationMs:62},{frame:2,durationMs:63},{frame:3,durationMs:62}]};
 expect(clipDuration(clip)).toBe(250);
 expect(playbackStep(clip,749,3)).toEqual({frame:3,waitMs:1});
 expect(playbackStep(clip,750,3)).toEqual({frame:0});
 expect(playbackStep(clip,2000,3)).toEqual({frame:0});
});
it('resolves authored ambient clips without changing base stage actions',()=>{
 const pack=packForSpecies('moss');
 expect(companionClip(pack,'child','greet')).toBe(pack.stages.child.greet);
 if(pack.contractVersion===2){
  expect(companionClip(pack,'adult','walk')).toBe(pack.ambient!.walk);
  expect(companionClip({...pack,ambient:undefined},'growing','look')).toBe(pack.stages.growing.idle);
  expect(companionClip(pack,'child','walkStop',8)).toBe(pack.locomotion!.stopByContact!['8']);
  expect(companionClip(pack,'child','walkStop',0)).toBe(pack.locomotion!.phases.walkStop);
 }
});

it('selects stable discrete frames and freezes at the first frame for reduced motion', () => {
  const clip = { frames: ['a', 'b', 'c'], fps: 4, anchor: { x: 32, y: 56 } };
  expect(animationFrame(clip, 0)).toBe('a');
  expect(animationFrame(clip, 249)).toBe('a');
  expect(animationFrame(clip, 250)).toBe('b');
  expect(animationFrame(clip, 750)).toBe('a');
  expect(animationFrame(clip, 500, true)).toBe('a');
});
it('sleeps until a pose boundary and finishes a finite interaction on its final pose',()=>{
 const clip={frames:['neutral','wave','settled'],fps:8,anchor:{x:128,y:224},sequence:[{frame:0,durationMs:200},{frame:1,durationMs:100},{frame:2,durationMs:150}],loop:false};
 expect(animationStep(clip,0)).toEqual({frame:0,waitMs:200});expect(animationStep(clip,250)).toEqual({frame:1,waitMs:50});
 expect(animationStep(clip,450)).toEqual({frame:2});expect(animationStep(clip,5000)).toEqual({frame:2});
 expect(animationStep({...clip,loop:true},450)).toEqual({frame:0,waitMs:200});
});
it('fits scaled artwork below the medal row at short and tall scene sizes',()=>{
 for(const species of ['lumen','ember','moss','bolt','astra','tide','sage']) {
  const pack=packForSpecies(species),stage='child' as const;
  const clip=pack.stages[stage].idle,native=pack.assets[clip.frames[0]];
  const ratio=visibleFrameHeightRatio(pack,stage);
  expect(ratio).toBeCloseTo((native.height-12)*companionDisplayScale(pack,stage)/native.height);
  for(const height of [120,180,260,340]) {
   const size=Math.min(560,(height-56)/ratio);
   expect(height-12-size*ratio).toBeGreaterThanOrEqual(43.99);
   expect(size).toBeLessThanOrEqual(560);
  }
 }
});
it('holds idle poses and traverses a timed wave instead of flashing every pose uniformly',()=>{
 const clip={frames:['open','half','closed','open-again'],fps:2,anchor:{x:128,y:224},sequence:[{frame:0,durationMs:3600},{frame:1,durationMs:100},{frame:2,durationMs:200},{frame:1,durationMs:100},{frame:3,durationMs:200}]};
 expect(animationFrameIndex(clip,3500)).toBe(0);expect(animationFrameIndex(clip,3650)).toBe(1);
 expect(animationFrameIndex(clip,3800)).toBe(2);expect(animationFrameIndex(clip,3950)).toBe(1);
 expect(animationFrameIndex(clip,4200)).toBe(0);expect(animationFrame(clip,3800,true)).toBe('open');
});
