import React from 'react';
import { act, cleanup, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { PixelSprite } from './PixelSprite';
import { packForSpecies, paletteAsset } from '../../companion/assets';
import { companionClip } from '../../companion/content';

vi.mock('../../companion/assets', async () => {
 const actual = await vi.importActual<typeof import('../../companion/assets')>('../../companion/assets');
 return {...actual, paletteAsset:vi.fn(async (id:string) => actual.assetUrl(id))};
});
let media:{matches:boolean;addEventListener:ReturnType<typeof vi.fn>;removeEventListener:ReturnType<typeof vi.fn>};
beforeEach(() => {
 media={matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()};
 vi.stubGlobal('matchMedia', () => media);
 vi.mocked(paletteAsset).mockClear();
});
afterEach(() => { cleanup(); vi.useRealTimers(); vi.unstubAllGlobals(); });
it('prepares frames without stepping until the shared epoch starts, and can prepare the same clip again',async()=>{
 vi.useFakeTimers();const ready=vi.fn(),step=vi.fn();
 const view=render(<PixelSprite action="walk" startedAt={null} onReady={ready} onFrame={step}/>);await act(async()=>{});
 expect(ready).toHaveBeenCalledOnce();act(()=>vi.advanceTimersByTime(2000));expect(step).not.toHaveBeenCalled();
 view.rerender(<PixelSprite action="walk" startedAt={performance.now()} onReady={ready} onFrame={step}/>);
 const poses=companionClip(packForSpecies('lumen'),'child','walk').sequence!;
 act(()=>vi.advanceTimersByTime(poses[0].durationMs+poses[1].durationMs));expect(screen.getByRole('img')).toHaveAttribute('data-frame','2');
 view.rerender(<PixelSprite action="walk" startedAt={null} onReady={ready} onFrame={step}/>);
 expect(ready).toHaveBeenCalledTimes(2);expect(vi.getTimerCount()).toBe(0);
});
it('uses a meaningful static interaction pose and loads just that frame for reduced motion',async()=>{
 vi.useFakeTimers();media.matches=true;render(<PixelSprite action="greet"/>);await act(async()=>{});
 expect(screen.getByRole('img')).toHaveAttribute('data-frame',String(companionClip(packForSpecies('lumen'),'child','greet').staticFrame));expect(paletteAsset).toHaveBeenCalledTimes(1);
 expect(vi.getTimerCount()).toBe(0);
});
it('preloads a clip and plays a slow blink from real frames without loading on each tick', async () => {
 const view=render(<PixelSprite/>);
 const image=screen.getByRole('img');
 await waitFor(()=>expect(image).toHaveAttribute('data-motion','running'));
 const pose=image.getAttribute('src');
 view.rerender(<PixelSprite animate={false}/>);
 vi.useFakeTimers();
 view.rerender(<PixelSprite/>);
 act(()=>{vi.advanceTimersByTime(3800);});
 expect(image.getAttribute('src')).not.toBe(pose);
 expect(image).toHaveAttribute('data-frame','5');
 expect(paletteAsset).toHaveBeenCalledTimes(8);
 view.rerender(<PixelSprite animate={false}/>);
 expect(image).toHaveAttribute('data-motion','paused');
});
it('pauses offscreen and resumes without restarting asset loading', async () => {
 render(<PixelSprite/>);
 const image=screen.getByRole('img');
 await waitFor(()=>expect(image).toHaveAttribute('data-motion','running'));
 Object.defineProperty(document,'hidden',{configurable:true,value:true});
 act(()=>document.dispatchEvent(new Event('visibilitychange')));
 expect(image).toHaveAttribute('data-motion','paused');
 Object.defineProperty(document,'hidden',{configurable:true,value:false});
 act(()=>document.dispatchEvent(new Event('visibilitychange')));
 expect(image).toHaveAttribute('data-motion','running');
 expect(paletteAsset).toHaveBeenCalledTimes(8);
});
it('loads one pose for a static thumbnail, then reuses it when animation is enabled',async()=>{
 const view=render(<PixelSprite animate={false}/>);await waitFor(()=>expect(paletteAsset).toHaveBeenCalledTimes(1));
 await act(async()=>{});view.rerender(<PixelSprite/>);
 await waitFor(()=>expect(screen.getByRole('img')).toHaveAttribute('data-motion','running'));
 expect(paletteAsset).toHaveBeenCalledTimes(8);
});
it('does no repeated timer work during the long idle hold and cancels on pause',async()=>{
 const view=render(<PixelSprite animate={false}/>);await act(async()=>{});
 vi.useFakeTimers();view.rerender(<PixelSprite/>);await act(async()=>{});
 const schedule=vi.spyOn(globalThis,'setTimeout');
 act(()=>{vi.advanceTimersByTime(3000);});expect(schedule).not.toHaveBeenCalled();
 act(()=>{vi.advanceTimersByTime(400);});expect(schedule).toHaveBeenCalledTimes(1);
 view.rerender(<PixelSprite animate={false}/>);expect(vi.getTimerCount()).toBe(0);schedule.mockRestore();
});
it('finishes an interaction once on its neutral ending pose without looping or leaving a timer',async()=>{
 vi.useFakeTimers();const done=vi.fn();render(<PixelSprite action="greet" onComplete={done}/>);await act(async()=>{});
 act(()=>{vi.advanceTimersByTime(5000);});expect(done).toHaveBeenCalledOnce();expect(vi.getTimerCount()).toBe(0);
 expect(screen.getByRole('img')).toHaveAttribute('data-frame',String(companionClip(packForSpecies('lumen'),'child','greet').sequence!.at(-1)!.frame));
});
it('pauses below the viewport and releases the intersection observer on unmount',async()=>{
 let observed!:(entries:{isIntersecting:boolean}[])=>void;const disconnect=vi.fn();
 vi.stubGlobal('IntersectionObserver',class{constructor(callback:typeof observed){observed=callback;}observe(){}disconnect=disconnect;});
 const view=render(<PixelSprite/>);await waitFor(()=>expect(screen.getByRole('img')).toHaveAttribute('data-motion','running'));
 act(()=>observed([{isIntersecting:false}]));expect(screen.getByRole('img')).toHaveAttribute('data-motion','paused');
 act(()=>observed([{isIntersecting:true}]));expect(screen.getByRole('img')).toHaveAttribute('data-motion','running');
 view.unmount();expect(disconnect).toHaveBeenCalled();
});
it('honors live reduced-motion changes and releases both listeners on unmount', async () => {
 const view=render(<PixelSprite action="greet"/>);
 const image=screen.getByRole('img');
 await waitFor(()=>expect(image).toHaveAttribute('data-motion','running'));
 const update=media.addEventListener.mock.calls[0][1];
 media.matches=true; act(()=>update());
 expect(image).toHaveAttribute('data-motion','reduced');
 media.matches=false; act(()=>update());
 expect(image).toHaveAttribute('data-motion','running');
 view.unmount();
 expect(media.removeEventListener).toHaveBeenCalledWith('change',update);
});
it('retains the previous recolored pose while the next action is loading', async () => {
 const view=render(<PixelSprite palette="amber"/>);
 const image=screen.getByRole('img');
 await waitFor(()=>expect(image).toHaveAttribute('data-motion','running'));
 const previous=image.getAttribute('src');
 let finish!:(url:string)=>void;
 vi.mocked(paletteAsset).mockImplementationOnce(()=>new Promise(resolve=>{finish=resolve;}));
 view.rerender(<PixelSprite palette="amber" action="greet"/>);
 expect(image).toHaveAttribute('src',previous);
 expect(image).toHaveAttribute('data-motion','paused');
 await act(async()=>finish('data:image/png;base64,next-pose'));
 expect(image).toHaveAttribute('src','data:image/png;base64,next-pose');
 expect(image).toHaveAttribute('data-motion','running');
});
