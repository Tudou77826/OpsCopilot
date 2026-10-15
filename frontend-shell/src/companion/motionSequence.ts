import type {CompanionAction} from './content';

export type Facing = -1 | 0 | 1;
export interface MotionSegment {action:CompanionAction;facing:Facing;repetitions?:number;destination?:number;entryFrame?:string;stopContact?:number;}

/** Fit complete authored strides inside the available floor. No speed/time clamping. */
export function planSteps(position:number,direction:-1|1,min:number,max:number,nativeStride:number,canvasSize:number,stageScale:number) {
  const stride=nativeStride*canvasSize/256*stageScale;
  const available=direction>0?max-position:position-min;
  const repetitions=Math.max(0,Math.floor((available+.001)/stride));
  return {repetitions,destination:position+direction*repetitions*stride};
}
export function walkSequence(facing:Facing,direction:-1|1,repetitions:number,destination:number):MotionSegment[] {
  if(repetitions<1)return [];
  const result:MotionSegment[]=[];
  if(facing!==0&&facing!==direction)result.push({action:'turnIn',facing});
  if(facing!==direction)result.push({action:'turnOut',facing:direction});
  result.push({action:'walkStart',facing:direction},{action:'walk',facing:direction,repetitions,destination},{action:'walkStop',facing:direction},{action:'turnIn',facing:direction},{action:'idle',facing:0});
  return result;
}

/** Idle blinking/breathing fills the gaps; larger autonomous gestures are rare. */
const ambientOrder=['look','look','stretch','look','walk','look'] as const;
export function ambientAction(index:number) {return ambientOrder[index%ambientOrder.length];}
export function ambientDelay(random:number) {return 45000+Math.max(0,Math.min(1,random))*30000;}
export function responseSequence(facing:Facing,action:'greet'|'happy'|'look'|'stretch'):MotionSegment[] {
  return [...(facing===0?[]:[{action:'turnIn' as const,facing}]),{action,facing:0},{action:'idle',facing:0}];
}
