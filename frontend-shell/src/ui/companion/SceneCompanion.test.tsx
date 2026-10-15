import React from 'react';
import {act,cleanup,render,screen} from '@testing-library/react';
import {afterEach,beforeEach,expect,it,vi} from 'vitest';
import type {CompanionAction} from '../../companion/content';
import {SceneCompanion} from './SceneCompanion';
let sprite:{action:CompanionAction;startedAt?:number|null;stopContact?:number;onReady?():void;onComplete?():void;onFrame?(frame:number,finished?:boolean):void};
vi.mock('./PixelSprite',()=>({PixelSprite:(props:typeof sprite)=>{sprite=props;return <img alt="pet" data-action={props.action}/>;}}));
let animation:{cancel:ReturnType<typeof vi.fn>;startTime?:number};
const props={species:'moss',stage:'child' as const,name:'苔团',action:'idle' as const,visible:true,size:'250px',onInteract:vi.fn(),onComplete:vi.fn()};
beforeEach(()=>{
 vi.useFakeTimers();vi.spyOn(Math,'random').mockReturnValue(0);
 vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()}));
 vi.spyOn(HTMLElement.prototype,'clientWidth','get').mockReturnValue(600);
 vi.spyOn(HTMLElement.prototype,'offsetWidth','get').mockReturnValue(250);
 vi.stubGlobal('DOMMatrixReadOnly',class{m41=-100;});
 animation={cancel:vi.fn()};Object.defineProperty(HTMLElement.prototype,'animate',{configurable:true,value:vi.fn(()=>animation)});props.onComplete.mockClear();
});
afterEach(()=>{cleanup();vi.useRealTimers();vi.restoreAllMocks();vi.unstubAllGlobals();delete (HTMLElement.prototype as unknown as {animate?:unknown}).animate;});
function ready(){act(()=>sprite.onReady?.());}
function complete(){act(()=>sprite.onComplete?.());}
function startWalk(){
 for(const action of ['look','look','stretch','look']){
  ready();act(()=>vi.advanceTimersByTime(45000));expect(sprite.action).toBe(action);ready();complete();
 }
 ready();act(()=>vi.advanceTimersByTime(45000));expect(sprite.action).toBe('turnOut');ready();complete();
 expect(sprite.action).toBe('walkStart');ready();complete();expect(sprite.action).toBe('walk');
}
it('shares one epoch between stepping and travel, then turns back to idle at the destination',()=>{
 render(<SceneCompanion {...props}/>);startWalk();
 expect(HTMLElement.prototype.animate).not.toHaveBeenCalled();ready();
 expect(HTMLElement.prototype.animate).toHaveBeenCalledOnce();expect(animation.startTime).toBe(sprite.startedAt);
 complete();expect(sprite.action).toBe('walkStop');expect(animation.cancel).toHaveBeenCalled();
 ready();complete();expect(sprite.action).toBe('turnIn');ready();complete();expect(sprite.action).toBe('idle');expect(screen.getByRole('button')).toHaveAttribute('data-facing','front');
 const settled=screen.getByRole('button').style.transform;ready();expect(screen.getByRole('button').style.transform).toBe(settled);
});
it('keeps idle breathing and blinking uninterrupted for at least 45 seconds',()=>{
 render(<SceneCompanion {...props}/>);ready();act(()=>vi.advanceTimersByTime(44999));expect(sprite.action).toBe('idle');
 act(()=>vi.advanceTimersByTime(1));expect(sprite.action).toBe('look');
});
it('defers a click until grounded contact, then stops and turns before greeting at the same location',()=>{
 const view=render(<SceneCompanion {...props}/>);startWalk();ready();
 act(()=>sprite.onFrame?.(3));view.rerender(<SceneCompanion {...props} action="greet"/>);
 expect(sprite.action).toBe('walk');act(()=>sprite.onFrame?.(8));expect(sprite.action).toBe('walkStop');
 expect(sprite.stopContact).toBe(8);
 const stopped=screen.getByRole('button').style.transform;
 ready();complete();expect(sprite.action).toBe('turnIn');ready();complete();expect(sprite.action).toBe('greet');
 ready();complete();expect(sprite.action).toBe('idle');expect(props.onComplete).toHaveBeenCalledOnce();
 expect(screen.getByRole('button').style.transform).toBe(stopped);
});
it('cancels travel when hidden, preserving its position and direction without remaining timers',()=>{
 const view=render(<SceneCompanion {...props}/>);startWalk();ready();view.rerender(<SceneCompanion {...props} visible={false}/>);
 expect(sprite.action).toBe('stand');expect(screen.getByRole('button')).toHaveAttribute('data-facing','right');
 expect(animation.cancel).toHaveBeenCalled();expect(vi.getTimerCount()).toBe(0);view.unmount();expect(vi.getTimerCount()).toBe(0);
});
it('does not skip the stop phase when a queued interaction reaches the final gait contact',()=>{
 const view=render(<SceneCompanion {...props}/>);startWalk();ready();
 view.rerender(<SceneCompanion {...props} action="greet"/>);
 act(()=>{sprite.onFrame?.(0,true);sprite.onComplete?.();});
 expect(sprite.action).toBe('walkStop');ready();complete();expect(sprite.action).toBe('turnIn');
});
it('returns from a static reduced-motion greeting when the parent clears the intent',()=>{
 vi.stubGlobal('matchMedia',()=>({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()}));
 const view=render(<SceneCompanion {...props}/>);view.rerender(<SceneCompanion {...props} action="greet"/>);
 expect(sprite.action).toBe('greet');view.rerender(<SceneCompanion {...props}/>);expect(sprite.action).toBe('idle');expect(vi.getTimerCount()).toBe(0);
});
it('drops a pending interaction and restarts the calm schedule when changing species',()=>{
 const view=render(<SceneCompanion {...props}/>);startWalk();ready();
 view.rerender(<SceneCompanion {...props} action="greet"/>);expect(sprite.action).toBe('walk');
 view.rerender(<SceneCompanion {...props} species="lumen" action="idle"/>);expect(sprite.action).toBe('idle');
 ready();act(()=>vi.advanceTimersByTime(44999));expect(sprite.action).toBe('idle');
 act(()=>vi.advanceTimersByTime(1));expect(sprite.action).toBe('look');
 ready();complete();expect(sprite.action).toBe('idle');
});
