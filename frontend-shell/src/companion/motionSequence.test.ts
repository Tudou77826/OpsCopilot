import {expect,it} from 'vitest';
import {ambientAction,ambientDelay,planSteps,responseSequence,walkSequence} from './motionSequence';
import {roamingRange} from './ambientMotion';
import {rosterPacks} from './rosterAssets';
import {companionDisplayScale} from './content';

it('fits only whole native strides at the displayed scale and never crosses a boundary',()=>{
 for(const scale of [.74,.87,1])for(const size of [200,256,320])for(const direction of [-1,1] as const){
  const plan=planSteps(10,direction,-75,80,24,size,scale);
  expect(plan.destination).toBeGreaterThanOrEqual(-75);expect(plan.destination).toBeLessThanOrEqual(80);
  expect(Math.abs(plan.destination-10)).toBeCloseTo(plan.repetitions*24*size/256*scale);
 }
 expect(planSteps(0,1,-1,1,24,256,1)).toEqual({repetitions:0,destination:0});
});
it('can take a complete stride from the center of the actual 420px child scene for every species',()=>{
 for(const {pack} of Object.values(rosterPacks)){
  if(pack.contractVersion!==2||!pack.locomotion)throw new Error('missing locomotion');
  const scale=companionDisplayScale(pack,'child'),range=roamingRange(420,320,scale);
  expect(planSteps(0,1,range.min,range.max,pack.locomotion.stride,320,scale).repetitions).toBeGreaterThan(0);
 }
});
it('settles and returns to front idle after walking, preserving the destination',()=>{
 expect(walkSequence(0,1,2,48).map(s=>s.action)).toEqual(['turnOut','walkStart','walk','walkStop','turnIn','idle']);
 expect(walkSequence(1,1,2,48).map(s=>s.action)).toEqual(['walkStart','walk','walkStop','turnIn','idle']);
 const reverse=walkSequence(1,-1,2,-48);
 expect(reverse.map(s=>s.action)).toEqual(['turnIn','turnOut','walkStart','walk','walkStop','turnIn','idle']);
 expect(reverse[0].facing).toBe(1);expect(reverse.at(-2)?.facing).toBe(-1);expect(reverse.at(-1)?.facing).toBe(0);
 expect(walkSequence(0,1,0,0)).toEqual([]);
});
it('leaves long idle gaps and limits stretching and walking to once per six ambient gestures',()=>{
 expect(ambientDelay(0)).toBe(45000);expect(ambientDelay(1)).toBe(75000);
 const actions=Array.from({length:12},(_,i)=>ambientAction(i));
 expect(actions.filter(a=>a==='walk')).toHaveLength(2);expect(actions.filter(a=>a==='stretch')).toHaveLength(2);
 expect(actions.filter(a=>a==='look')).toHaveLength(8);
});
it('turns a side stand toward the user before responding',()=>{
 expect(responseSequence(-1,'greet')).toEqual([{action:'turnIn',facing:-1},{action:'greet',facing:0},{action:'idle',facing:0}]);
 expect(responseSequence(0,'stretch').map(s=>s.action)).toEqual(['stretch','idle']);
});
