import React from 'react';
import { render, screen, fireEvent, waitFor, cleanup, act } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { CompanionEntry, CompanionAssistantHeader } from './CompanionEntry';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';

const presentationKey = 'opscopilot.companion.presentation.v1';
beforeEach(() => {
 localStorage.clear();
 // Existing scene/medal cases start from the user's saved expanded preference.
 localStorage.setItem(presentationKey, JSON.stringify({expanded:true,height:180}));
 vi.stubGlobal('matchMedia', () => ({matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()}));
});
afterEach(() => { cleanup(); vi.unstubAllGlobals(); vi.restoreAllMocks(); localStorage.clear(); });
const snapshot: CompanionSnapshot = {schemaVersion:1,ruleVersion:1,revision:1,level:3,levelXP:0,levelXPRequired:10,todayXP:0,dailyXPMax:12,hatchCost:50,canHatch:false,pet:{id:'pet',speciesId:'lumen',name:'灯灯',paletteId:'amber',xp:20,adoptedAt:'2026-10-10T00:00:00Z'}};
function host(): CompanionHost { return {snapshot:vi.fn(async()=>snapshot),hatch:vi.fn(),rename:vi.fn()}; }
function social(): SocialHost {
 return {
   status:vi.fn(async()=>({summary:{ruleVersion:1 as const,progress:[{kind:'script-created' as const,count:3,medal:{kind:'script-created' as const,count:3,tier:'bronze' as const}},{kind:'active-day' as const,count:2,nextThreshold:7}]},selected:[],pending:0,repairs:0,profilePending:false})),
   catalog:vi.fn(async()=>[{kind:'script-created' as const,name:'脚本匠人',unit:'个',group:'creation' as const,thresholds:[1,10,30] as [number,number,number]},{kind:'active-day' as const,name:'持续实践者',unit:'天',group:'consistency' as const,thresholds:[7,30,90] as [number,number,number]}]),
   connect:vi.fn(),sync:vi.fn(),participation:vi.fn(),showcase:vi.fn(),rankings:vi.fn(),profile:vi.fn(),
 };
}
it('shows the partner, real medal tiers and growth together, and stops medal reads when collapsed', async () => {
 const runtime=host(), ledger=social(), manage=vi.fn();
 render(<CompanionAssistantHeader host={runtime} socialHost={ledger} enabled visible onManage={manage}/>);
 await screen.findByRole('button',{name:'脚本匠人 · 铜章 · 3个'});
 expect(screen.queryByText('持续实践者')).toBeNull();
 expect(screen.getByRole('button',{name:'脚本匠人 · 铜章 · 3个'})).toHaveAttribute('aria-describedby',screen.getByText('脚本匠人 · 铜章 · 3个').closest('[role="tooltip"]')!.id);
 expect(screen.getByRole('progressbar',{name:'伙伴升级进度'})).toHaveAttribute('max','10');
 expect(document.querySelector('.oc-assistant-companion-scene .oc-assistant-medals')).not.toBeNull();
 expect(screen.getByRole('progressbar',{name:'伙伴升级进度'}).closest('.oc-assistant-companion-identity')).not.toBeNull();
 expect(screen.getByRole('button',{name:'收起伙伴展示'})).toHaveTextContent('灯灯');
 expect(screen.getByRole('button',{name:'收起伙伴展示'}).querySelector('svg')).not.toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'养成与奖章'})); expect(manage).toHaveBeenCalledOnce();
 expect(ledger.sync).not.toHaveBeenCalled(); expect(ledger.participation).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'收起伙伴展示'}));
 expect(screen.getByRole('button',{name:'展开伙伴展示'})).toHaveAttribute('aria-expanded','false');
 expect(document.querySelector('.oc-assistant-companion-scene')).toBeNull();
 const calls=vi.mocked(ledger.status).mock.calls.length;
 document.dispatchEvent(new Event('visibilitychange'));
 expect(ledger.status).toHaveBeenCalledTimes(calls);
 fireEvent.click(screen.getByRole('button',{name:'展开伙伴展示'}));
 await screen.findByRole('button',{name:'脚本匠人 · 铜章 · 3个'});
 expect(ledger.status).toHaveBeenCalledTimes(calls+1);
});
it('does not read medals while disabled or hidden and distinguishes load errors from zero progress', async () => {
 const runtime=host(), ledger=social();
 const view=render(<CompanionAssistantHeader host={runtime} socialHost={ledger} enabled={false} visible onManage={vi.fn()}/>);
 expect(ledger.status).not.toHaveBeenCalled(); expect(ledger.catalog).not.toHaveBeenCalled();
 expect(screen.queryByText('勿打扰')).toBeNull();
 expect(screen.queryByLabelText('伙伴展示')).toBeNull();
 expect(screen.queryByRole('button',{name:'收起伙伴展示'})).toBeNull();
 expect(document.querySelector('.oc-companion-roaming')).toBeNull();
 expect(runtime.snapshot).not.toHaveBeenCalled();
 view.rerender(<CompanionAssistantHeader host={runtime} socialHost={ledger} enabled visible={false} onManage={vi.fn()}/>);
 expect(ledger.status).not.toHaveBeenCalled();
 vi.mocked(ledger.status).mockRejectedValue(new Error('read failed'));
 view.rerender(<CompanionAssistantHeader host={runtime} socialHost={ledger} enabled visible onManage={vi.fn()}/>);
 await screen.findByText('奖章暂未读取');
 expect(screen.queryByText('尚未获得奖章')).toBeNull();
 expect(screen.queryByText('0 / 7天')).toBeNull();
});
it('keeps the AI entry available without reading disabled pet data and stops reads after disabling', async () => {
 const runtime=host(), open=vi.fn();
 const view=render(<CompanionEntry host={runtime} enabled={false} selected={false} onOpen={open}/>);
 fireEvent.click(screen.getByRole('button',{name:'AI 助手'})); expect(open).toHaveBeenCalledOnce();
 expect(runtime.snapshot).not.toHaveBeenCalled();
 view.rerender(<CompanionEntry host={runtime} enabled selected onOpen={open}/>);
 await waitFor(()=>expect(screen.getByRole('img')).toHaveAttribute('data-palette','amber'));
 expect(screen.getByRole('img')).toHaveAttribute('data-level','3');
 view.rerender(<CompanionEntry host={runtime} enabled={false} selected onOpen={open}/>);
 const calls=vi.mocked(runtime.snapshot).mock.calls.length;
 document.dispatchEvent(new Event('visibilitychange'));
 expect(runtime.snapshot).toHaveBeenCalledTimes(calls);
 expect(screen.getByRole('button',{name:'AI 助手'})).toBeVisible();
 expect(screen.queryByRole('img')).toBeNull();
 expect(screen.getByRole('button',{name:'AI 助手'})).toHaveAttribute('title','AI 助手');
});
it('reads header identity only while visible and exposes the correct management action', async () => {
 const runtime=host(), manage=vi.fn();
 const view=render(<CompanionAssistantHeader host={runtime} enabled visible={false} onManage={manage}/>);
 expect(runtime.snapshot).not.toHaveBeenCalled();
 view.rerender(<CompanionAssistantHeader host={runtime} enabled visible onManage={manage}/>);
 await screen.findByText('灯灯'); fireEvent.click(screen.getByRole('button',{name:'养成与奖章'}));
 expect(manage).toHaveBeenCalledOnce();
 view.rerender(<CompanionAssistantHeader host={runtime} enabled={false} visible onManage={manage}/>);
 expect(screen.queryByText('勿打扰')).toBeNull();
 expect(screen.queryByLabelText('伙伴展示')).toBeNull();
 expect(screen.queryByText('灯灯')).toBeNull();
 const calls=vi.mocked(runtime.snapshot).mock.calls.length;
 document.dispatchEvent(new Event('visibilitychange'));
 expect(runtime.snapshot).toHaveBeenCalledTimes(calls);
 expect(screen.queryByRole('button',{name:'伙伴设置'})).toBeNull();
 expect(screen.queryByRole('button',{name:'开启养成'})).toBeNull();
 expect(manage).toHaveBeenCalledTimes(1);
});

it('respects selected medal order, hides revoked selections and never saves an automatic preview', async () => {
 const ledger=social();
 const data=await ledger.status();
 data.summary.progress[1]={kind:'active-day',count:90,medal:{kind:'active-day',count:90,tier:'gold'}};
 data.selected=['active-day','script-created'];
 vi.mocked(ledger.status).mockResolvedValue(data);
 const view=render(<CompanionAssistantHeader host={host()} socialHost={ledger} enabled visible onManage={vi.fn()}/>);
 await screen.findByRole('button',{name:'持续实践者 · 金章 · 90天'});
 expect([...document.querySelectorAll('.oc-assistant-medal')].map(item=>item.getAttribute('data-kind'))).toEqual(['active-day','script-created']);
 // A selected medal can disappear when its underlying contributions are revoked.
 data.summary.progress=data.summary.progress.filter(item=>item.kind !== 'active-day');
 data.selected=['active-day'];
 view.unmount();
 render(<CompanionAssistantHeader host={host()} socialHost={ledger} enabled visible onManage={vi.fn()}/>);
 await screen.findByText('暂未选出可展示的奖章');
 expect(document.querySelectorAll('.oc-assistant-medal')).toHaveLength(0);
 expect(ledger.showcase).not.toHaveBeenCalled();
 expect(ledger.sync).not.toHaveBeenCalled();
});
it('previews at most three earned medals by tier when no showcase is selected', async () => {
 const ledger=social();
 const kinds=['knowledge-archive','script-created','quick-command-created','case-completed'] as const;
 vi.mocked(ledger.catalog).mockResolvedValue(kinds.map(kind=>({kind,name:kind,unit:'次',group:'creation',thresholds:[1,10,30]})));
 const data=await ledger.status();
 data.summary.progress=kinds.map((kind,index)=>({kind,count:index+1,medal:{kind,count:index+1,tier:index===3?'gold':'bronze'}}));
 vi.mocked(ledger.status).mockResolvedValue(data);
 render(<CompanionAssistantHeader host={host()} socialHost={ledger} enabled visible onManage={vi.fn()}/>);
 await screen.findByRole('button',{name:'case-completed · 金章 · 4次'});
 expect([...document.querySelectorAll('.oc-assistant-medal')].map(item=>item.getAttribute('data-kind'))).toEqual(['case-completed','knowledge-archive','script-created']);
 expect(ledger.showcase).not.toHaveBeenCalled();
});
it('shows an honest empty display before any medal is earned', async () => {
 const ledger=social();
 const data=await ledger.status();
 data.summary.progress=data.summary.progress.map(({kind,count})=>({kind,count}));
 vi.mocked(ledger.status).mockResolvedValue(data);
 render(<CompanionAssistantHeader host={host()} socialHost={ledger} enabled visible onManage={vi.fn()}/>);
 await screen.findByText('尚未获得奖章');
 expect(document.querySelectorAll('.oc-assistant-medal')).toHaveLength(0);
 expect(screen.getByRole('button',{name:'养成与奖章'})).toBeVisible();
});

it('shows backend level/progress independently of the fixed artwork, including max level',async()=>{
 const runtime=host();vi.mocked(runtime.snapshot).mockResolvedValue({...snapshot,level:99,levelXP:9,levelXPRequired:10,pet:{...snapshot.pet!,xp:989}});
 render(<CompanionAssistantHeader host={runtime} enabled visible onManage={vi.fn()}/>);
 await screen.findByText('Lv.99');
 const progress=screen.getByRole('progressbar',{name:'伙伴升级进度'});
 expect(progress).toHaveAttribute('value','9');expect(progress).toHaveAttribute('max','10');
 expect(document.querySelector('.oc-companion-roaming img')).toHaveAttribute('data-stage','child');
 vi.mocked(runtime.snapshot).mockResolvedValue({...snapshot,revision:2,level:100,levelXP:0,levelXPRequired:0,pet:{...snapshot.pet!,xp:990}});
 document.dispatchEvent(new Event('visibilitychange'));await screen.findByText('Lv.100');
 expect(progress).toHaveAttribute('value','1');expect(progress).toHaveAttribute('max','1');
 expect(progress.title).toContain('已满级');
 expect(screen.queryByText(/幼年|成熟/)).toBeNull();
});
it('shows an unhatched egg rather than an owned species and links to the acquisition flow',async()=>{
 const runtime=host(),manage=vi.fn();
 vi.mocked(runtime.snapshot).mockResolvedValue({...snapshot,pet:null,level:0,hatchCost:0,canHatch:true});
 render(<CompanionAssistantHeader host={runtime} enabled visible onManage={manage}/>);
 await screen.findByText('尚未开蛋');
 expect(screen.getAllByRole('img',{name:'伙伴蛋'})).toHaveLength(2);
 expect(document.querySelector('.oc-companion-roaming')).toBeNull();
 fireEvent.click(screen.getByRole('button',{name:'去开蛋'}));
 expect(manage).toHaveBeenCalledOnce();
});

it('starts collapsed in a single identity toolbar and remembers expansion without mounting the scene', async () => {
 localStorage.removeItem(presentationKey);
 const ledger=social(), runtime=host();
 const view=render(<CompanionAssistantHeader host={runtime} socialHost={ledger} enabled visible onManage={vi.fn()} actions={<button>关闭</button>}/>);
 await screen.findByText('灯灯');
 expect(screen.getByRole('button',{name:'展开伙伴展示'})).toHaveAttribute('aria-expanded','false');
 expect(document.querySelector('.oc-assistant-companion-scene')).toBeNull();
 expect(ledger.status).not.toHaveBeenCalled();
 expect(screen.queryByText('伙伴助手')).toBeNull();
 expect(screen.queryByText('AI 助手')).toBeNull();
 expect(screen.getByRole('button',{name:'关闭'}).closest('header')).toBe(screen.getByText('灯灯').closest('header'));
 fireEvent.click(screen.getByRole('button',{name:'展开伙伴展示'}));
 await screen.findByRole('separator',{name:'调整伙伴场景高度'});
 expect(ledger.status).toHaveBeenCalledOnce();
 view.unmount();
 render(<CompanionAssistantHeader host={runtime} enabled visible onManage={vi.fn()}/>);
 await screen.findByRole('button',{name:'收起伙伴展示'});
});

it('keeps only conversation actions when companionship is disabled', () => {
 const runtime=host();
 render(<CompanionAssistantHeader host={runtime} enabled={false} visible onManage={vi.fn()} actions={<button>新建对话</button>}/>);
 expect(screen.getByRole('button',{name:'新建对话'})).toBeVisible();
 expect(screen.queryByRole('img')).toBeNull();
 expect(screen.queryByLabelText('伙伴展示')).toBeNull();
 expect(runtime.snapshot).not.toHaveBeenCalled();
});

it('supports bounded keyboard resizing, viewport clamping, reset and height restoration', async () => {
 let parentHeight=800;
 const callbacks: (()=>void)[]=[];
 vi.spyOn(HTMLElement.prototype,'clientHeight','get').mockImplementation(()=>parentHeight);
 vi.stubGlobal('ResizeObserver', class {
  constructor(callback:()=>void) { callbacks.push(callback); }
  observe() {} disconnect() {}
 });
 const runtime=host();
 const view=render(<CompanionAssistantHeader host={runtime} enabled visible onManage={vi.fn()}/>);
 const separator=await screen.findByRole('separator',{name:'调整伙伴场景高度'});
 expect(separator).toHaveAttribute('aria-valuenow','180');
 fireEvent.keyDown(separator,{key:'ArrowDown',shiftKey:true});
 expect(separator).toHaveAttribute('aria-valuenow','220');
 fireEvent.keyDown(separator,{key:'End'});
 expect(separator).toHaveAttribute('aria-valuenow','320');
 parentHeight=600;
 act(()=>callbacks.forEach(callback=>callback()));
 expect(separator).toHaveAttribute('aria-valuenow','240');
 expect(JSON.parse(localStorage.getItem(presentationKey)!).height).toBe(320);
 parentHeight=800;
 act(()=>callbacks.forEach(callback=>callback()));
 expect(separator).toHaveAttribute('aria-valuenow','320');
 fireEvent.doubleClick(separator);
 expect(separator).toHaveAttribute('aria-valuenow','180');
 fireEvent.keyDown(separator,{key:'ArrowUp'});
 view.unmount();
 render(<CompanionAssistantHeader host={runtime} enabled visible onManage={vi.fn()}/>);
 expect(await screen.findByRole('separator')).toHaveAttribute('aria-valuenow','170');
 fireEvent.keyDown(screen.getByRole('separator'),{key:'Home'});
 expect(screen.getByRole('separator')).toHaveAttribute('aria-valuenow','120');
});

it('captures the drag, saves on release and cancels interrupted drags without changing the saved height', async () => {
 vi.stubGlobal('PointerEvent',class extends MouseEvent {
  pointerId:number; isPrimary:boolean;
  constructor(type:string,init:PointerEventInit) { super(type,init); this.pointerId=init.pointerId??1;this.isPrimary=init.isPrimary??true; }
 });
 render(<CompanionAssistantHeader host={host()} enabled visible onManage={vi.fn()}/>);
 const separator=await screen.findByRole('separator');
 separator.setPointerCapture=vi.fn();
 separator.hasPointerCapture=vi.fn(()=>true);
 separator.releasePointerCapture=vi.fn();
 fireEvent.pointerDown(separator,{pointerId:1,button:0,clientY:200});
 fireEvent.pointerMove(separator,{pointerId:1,clientY:250});
 expect(separator.setPointerCapture).toHaveBeenCalledWith(1);
 expect(separator).toHaveAttribute('aria-valuenow','230');
 expect(JSON.parse(localStorage.getItem(presentationKey)!).height).toBe(180);
 fireEvent.pointerUp(separator,{pointerId:1,clientY:250});
 expect(JSON.parse(localStorage.getItem(presentationKey)!).height).toBe(230);
 fireEvent.pointerDown(separator,{pointerId:2,button:0,clientY:250});
 fireEvent.pointerMove(separator,{pointerId:2,clientY:150});
 fireEvent.pointerCancel(separator,{pointerId:2});
 expect(separator).toHaveAttribute('aria-valuenow','230');
 expect(JSON.parse(localStorage.getItem(presentationKey)!).height).toBe(230);
 expect(separator.releasePointerCapture).toHaveBeenCalledWith(2);
});

it('handles corrupt or unavailable presentation storage without losing the conversation controls', async () => {
 localStorage.setItem(presentationKey,'not json');
 const view=render(<CompanionAssistantHeader host={host()} enabled visible onManage={vi.fn()}/>);
 await screen.findByRole('button',{name:'展开伙伴展示'});
 view.unmount();
 vi.spyOn(Storage.prototype,'getItem').mockImplementation(()=>{throw new Error('storage unavailable');});
 vi.spyOn(Storage.prototype,'setItem').mockImplementation(()=>{throw new Error('storage unavailable');});
 render(<CompanionAssistantHeader host={host()} enabled visible onManage={vi.fn()}/>);
 fireEvent.click(await screen.findByRole('button',{name:'展开伙伴展示'}));
 expect(await screen.findByRole('separator')).toHaveAttribute('aria-valuenow','180');
});
