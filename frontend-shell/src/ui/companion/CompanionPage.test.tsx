import React from 'react';
import { beforeEach, afterEach, beforeAll, afterAll, expect, it, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, renderHook, act } from '@testing-library/react';
import CompanionPage from './CompanionPage';
import { HATCH_OPEN_DURATION_MS } from './CompanionEgg';
import { useCompanion } from './useCompanion';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';
import type { SocialHost } from '../../companion/socialHost';

vi.mock('../../companion/assets', async () => {
  const actual = await vi.importActual<typeof import('../../companion/assets')>('../../companion/assets');
  return {...actual, paletteAsset: async (id: string) => actual.assetUrl(id)};
});
const originalDialogMethods = ['showModal','close'].map(name => Object.getOwnPropertyDescriptor(HTMLDialogElement.prototype,name));
beforeAll(()=>{
  Object.defineProperty(HTMLDialogElement.prototype,'showModal',{configurable:true,value:function(this:HTMLDialogElement){this.open=true;}});
  Object.defineProperty(HTMLDialogElement.prototype,'close',{configurable:true,value:function(this:HTMLDialogElement){this.open=false;}});
});
afterAll(()=>['showModal','close'].forEach((name,index)=>{
  const descriptor=originalDialogMethods[index];
  if(descriptor)Object.defineProperty(HTMLDialogElement.prototype,name,descriptor);else Reflect.deleteProperty(HTMLDialogElement.prototype,name);
}));
beforeEach(() => vi.stubGlobal('matchMedia', () => ({matches:true,addEventListener:vi.fn(),removeEventListener:vi.fn()})));
afterEach(() => vi.unstubAllGlobals());
const empty: CompanionSnapshot = {schemaVersion:1,ruleVersion:1,revision:0,level:0,levelXP:0,levelXPRequired:0,pet:null,todayXP:0,dailyXPMax:12,hatchCost:0,canHatch:true};
const owned: CompanionSnapshot = {...empty,revision:7,level:8,levelXP:3,levelXPRequired:10,hatchCost:50,canHatch:true,pet:{id:'pet',speciesId:'lumen',name:'原名',paletteId:'lavender',xp:73,adoptedAt:'2026-10-10T00:00:00Z'}};
function hosts(initial = empty) {
  let current = initial;
  const host: CompanionHost = {
    snapshot:vi.fn(async () => current),
    hatch:vi.fn(async () => current = {...owned,revision:current.revision+1,pet:{...owned.pet!,speciesId:'bolt',paletteId:'mint',xp:current.pet ? current.pet.xp-50 : 0}}),
    rename:vi.fn(async name => current = {...current,revision:current.revision+1,pet:{...current.pet!,name}}),
  };
  const social: SocialHost = {status:vi.fn(async () => ({summary:{ruleVersion:1 as const,progress:[]},pending:0,repairs:0,selected:[],profilePending:false})),catalog:vi.fn(async () => []),connect:vi.fn(),sync:vi.fn(),participation:vi.fn(),showcase:vi.fn(),rankings:vi.fn(),profile:vi.fn()};
  return {host,social};
}
it('waits for a deliberate touch and permits leaving the first ritual without drawing',async()=>{
  const {host,social}=hosts();render(<CompanionPage host={host} socialHost={social}/>);
  const entry=await screen.findByRole('button',{name:'开蛋'});entry.focus();fireEvent.click(entry);
  expect(screen.getByRole('button',{name:'开启伙伴蛋'})).toHaveFocus();
  expect(screen.getByRole('heading',{name:'一枚蛋，一次相遇'})).toBeVisible();
  expect(host.hatch).not.toHaveBeenCalled();
  fireEvent.keyDown(screen.getByRole('dialog'),{key:'Escape'});
  expect(host.hatch).not.toHaveBeenCalled();expect(entry).toHaveFocus();
});
it('opens the free first egg with no species/palette picker or automatic social opt-in', async () => {
  const {host,social} = hosts(); render(<CompanionPage host={host} socialHost={social}/>);
  await screen.findByRole('button',{name:'开蛋'});
  expect(screen.getByRole('img',{name:'伙伴蛋'})).toBeVisible();
  expect(screen.queryByRole('radiogroup')).toBeNull();
  fireEvent.click(screen.getByRole('button',{name:'开蛋'}));fireEvent.click(screen.getByRole('button',{name:'开启伙伴蛋'}));
  await screen.findByRole('button',{name:'修改名字'});
  const input=screen.getByRole('textbox',{name:'给伙伴起个名字'});
  expect(input).toHaveFocus();
  expect(input).toHaveValue('原名');
  expect(screen.getByRole('button',{name:'就叫这个名字'})).toBeEnabled();
  expect(screen.getByRole('button',{name:'就叫这个名字'}).textContent).toBe('');
  expect(document.querySelectorAll('.oc-hatch-name-actions button')).toHaveLength(1);
  fireEvent.change(input,{target:{value:' 小搭档 '}});
  fireEvent.submit(input.closest('form')!);
  await waitFor(()=>expect(host.rename).toHaveBeenCalledExactlyOnceWith('小搭档',1));
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.getByRole('heading',{name:'小搭档'})).toHaveFocus();
  expect(host.hatch).toHaveBeenCalledExactlyOnceWith(0);
  expect(document.querySelector('.oc-companion-scene img')).toHaveAttribute('data-species','bolt');
  expect(social.connect).not.toHaveBeenCalled(); expect(social.participation).not.toHaveBeenCalled();
});
it('can keep the default first name and never draws again after leaving the encounter',async()=>{
  const {host,social}=hosts();
  const view=render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'开蛋'}));fireEvent.click(screen.getByRole('button',{name:'开启伙伴蛋'}));
  const input=await screen.findByRole('textbox',{name:'给伙伴起个名字'});
  fireEvent.change(input,{target:{value:'未提交的新名字'}});
  fireEvent.click(screen.getByRole('button',{name:'关闭开蛋仪式'}));
  expect(screen.queryByRole('dialog')).toBeNull();expect(host.rename).not.toHaveBeenCalled();
  view.unmount();render(<CompanionPage host={host} socialHost={social}/>);
  await screen.findByRole('button',{name:'修改名字'});
  expect(screen.queryByRole('dialog')).toBeNull();expect(host.hatch).toHaveBeenCalledOnce();
});
it('confirms the visible prefilled name and rejects a blank or whitespace-only name',async()=>{
  const {host,social}=hosts();render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'开蛋'}));fireEvent.click(screen.getByRole('button',{name:'开启伙伴蛋'}));
  const input=await screen.findByRole('textbox',{name:'给伙伴起个名字'});
  const confirm=screen.getByRole('button',{name:'就叫这个名字'});
  expect(input).toHaveValue('原名');
  fireEvent.change(input,{target:{value:'   '}});expect(confirm).toBeDisabled();
  fireEvent.submit(input.closest('form')!);expect(host.rename).not.toHaveBeenCalled();
  fireEvent.change(input,{target:{value:'原名'}});fireEvent.click(confirm);
  await waitFor(()=>expect(host.rename).toHaveBeenCalledExactlyOnceWith('原名',1));
  expect(host.hatch).toHaveBeenCalledOnce();expect(screen.queryByRole('dialog')).toBeNull();
});
it.each([false,true])('retains the naming draft on failure and blocks duplicate submits or dismissal (paid=%s)',async(paid)=>{
  const {host,social}=hosts(paid ? owned : empty);
  let reject!: (cause:Error)=>void;
  host.rename=vi.fn(()=>new Promise<CompanionSnapshot>((_resolve,fail)=>{reject=fail;}));
  render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:paid ? '重新开蛋' : '开蛋'}));
  if(paid)fireEvent.click(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'}));else fireEvent.click(screen.getByRole('button',{name:'开启伙伴蛋'}));
  const input=await screen.findByRole('textbox',{name:'给伙伴起个名字'});
  fireEvent.change(input,{target:{value:'团团'}});
  fireEvent.submit(input.closest('form')!);fireEvent.submit(input.closest('form')!);
  expect(host.rename).toHaveBeenCalledExactlyOnceWith('团团',paid ? 8 : 1);
  expect(screen.getByRole('heading',{name:'遇见了小栓'})).toHaveFocus();
  fireEvent.keyDown(screen.getByRole('dialog'),{key:'Escape'});
  expect(screen.getByRole('dialog')).toBeVisible();
  await act(async()=>reject(new Error('保存失败，请重试')));
  expect(screen.getByRole('alert')).toHaveTextContent('保存失败');expect(input).toHaveValue('团团');
  expect(input).toHaveFocus();
  expect(screen.getByRole('button',{name:'就叫这个名字'})).toBeEnabled();
  expect(host.hatch).toHaveBeenCalledOnce();
});
it('requires a fee confirmation, permits cancellation and keeps the quoted revision fixed', async () => {
  const {host,social} = hosts(owned); render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'重新开蛋'}));
  const confirm=screen.getByRole('dialog',{name:'确认重新开蛋'});
  expect(confirm).toHaveTextContent('73 → 23');
  expect(screen.getByRole('button',{name:'保留当前伙伴'})).toHaveFocus();
  fireEvent.keyDown(screen.getByRole('button',{name:'保留当前伙伴'}),{key:'Tab'});
  expect(screen.getByRole('button',{name:'关闭开蛋仪式'})).toHaveFocus();
  fireEvent.keyDown(screen.getByRole('button',{name:'关闭开蛋仪式'}),{key:'Tab',shiftKey:true});
  expect(screen.getByRole('button',{name:'保留当前伙伴'})).toHaveFocus();
  expect(host.hatch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button',{name:'保留当前伙伴'}));
  expect(screen.queryByRole('dialog',{name:'确认重新开蛋'})).toBeNull();
  expect(host.hatch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button',{name:'重新开蛋'}));
  fireEvent.click(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'}));
  await waitFor(()=>expect(host.hatch).toHaveBeenCalledExactlyOnceWith(7));
  expect(await screen.findByRole('heading',{name:'遇见了小栓'})).toBeVisible();
  expect(screen.getByRole('textbox',{name:'给伙伴起个名字'})).toHaveFocus();
  fireEvent.click(screen.getByRole('button',{name:'关闭开蛋仪式'}));
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(host.rename).not.toHaveBeenCalled();
  await waitFor(()=>expect(screen.getByRole('status')).toHaveTextContent('已消耗 50 点经验'));
});
it('names a newly revealed paid companion using its committed revision without charging again',async()=>{
  const {host,social}=hosts(owned);render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'重新开蛋'}));
  fireEvent.click(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'}));
  const input=await screen.findByRole('textbox',{name:'给伙伴起个名字'});
  expect(input).toHaveFocus();expect(input).toHaveValue('原名');
  fireEvent.change(input,{target:{value:'浪浪'}});fireEvent.submit(input.closest('form')!);
  await waitFor(()=>expect(host.rename).toHaveBeenCalledExactlyOnceWith('浪浪',8));
  expect(screen.queryByRole('dialog')).toBeNull();expect(host.hatch).toHaveBeenCalledExactlyOnceWith(7);
  expect(screen.getByRole('heading',{name:'浪浪'})).toHaveFocus();
  expect(await host.snapshot()).toMatchObject({revision:9,pet:{name:'浪浪',speciesId:'bolt',xp:23}});
});
it('lets an unfunded user inspect the ritual, blocks payment and restores focus on Escape',async()=>{
  const {host,social}=hosts({...owned,canHatch:false,pet:{...owned.pet!,xp:12}});
  render(<CompanionPage host={host} socialHost={social}/>);
  const invitation=await screen.findByRole('button',{name:'重新开蛋'});
  expect(invitation).toBeEnabled();
  expect(invitation.textContent).toBe('');
  expect(invitation).toHaveAccessibleDescription(expect.stringContaining('还差 38 点'));
  invitation.focus();fireEvent.click(invitation);
  expect(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'})).toBeDisabled();
  fireEvent.keyDown(screen.getByRole('dialog'),{key:'Escape'});
  expect(screen.queryByRole('dialog')).toBeNull();expect(invitation).toHaveFocus();
  fireEvent.click(screen.getByRole('button',{name:'修改名字'}));
  fireEvent.change(screen.getByLabelText('伙伴名称'),{target:{value:'新名字'}});
  fireEvent.click(screen.getByRole('button',{name:'保存名称'}));
  await waitFor(()=>expect(host.rename).toHaveBeenCalledExactlyOnceWith('新名字',7));
  expect(host.hatch).not.toHaveBeenCalled();
});
it('prevents another draw during request/reveal and recovers the actual save after failure',async()=>{
  const {host,social}=hosts(owned);
  let reject!: (error:Error)=>void;
  host.hatch=vi.fn(()=>new Promise<CompanionSnapshot>((_resolve,fail)=>{reject=fail;}));
  render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'重新开蛋'}));
  fireEvent.click(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'}));
  expect(screen.queryByRole('button',{name:'重新开蛋'})).toBeNull();
  expect(screen.getByRole('dialog').querySelector('[data-opening="true"]')).not.toBeNull();
  expect(document.querySelector('.oc-hatch-visual')).toHaveAttribute('data-phase','waiting');
  fireEvent.keyDown(screen.getByRole('dialog'),{key:'Escape'});
  expect(screen.getByRole('dialog')).toBeVisible();
  expect(screen.getByRole('button',{name:'关闭开蛋仪式'})).toBeDisabled();
  await act(async()=>reject(new Error('保存失败')));
  expect(await screen.findByRole('alert')).toHaveTextContent('保存失败');
  expect(document.querySelector('.oc-companion-scene img')).toHaveAttribute('data-species','lumen');
  expect(host.hatch).toHaveBeenCalledOnce();
});
it('keeps the committed result behind a short reveal and cancels visual timers on unmount',async()=>{
  const {host,social}=hosts();
  let commit!: ()=>void;
  const commitHatch=host.hatch;
  host.hatch=vi.fn(async revision=>{await new Promise<void>(resolve=>{commit=resolve;});return commitHatch(revision);});
  vi.stubGlobal('matchMedia',()=>({matches:false,addEventListener:vi.fn(),removeEventListener:vi.fn()}));
  vi.useFakeTimers();
  try {
    const view=render(<CompanionPage host={host} socialHost={social}/>);
    await act(async()=>{});
    await act(async()=>{fireEvent.click(screen.getByRole('button',{name:'开蛋'}));});
    expect(host.hatch).not.toHaveBeenCalled();
    await act(async()=>{fireEvent.click(screen.getByRole('button',{name:'开启伙伴蛋'}));});
    expect(screen.getByRole('img',{name:'正在打开的伙伴蛋'})).toBeVisible();
    expect(host.hatch).toHaveBeenCalledOnce();
    expect(document.querySelector('.oc-hatch-visual')).toHaveAttribute('data-phase','waiting');
    await act(async()=>{vi.advanceTimersByTime(3000);});
    expect(screen.queryByRole('textbox',{name:'给伙伴起个名字'})).toBeNull();
    await act(async()=>{commit();});
    expect(document.querySelector('.oc-hatch-visual')).toHaveAttribute('data-phase','cracking');
    expect(document.querySelector('.oc-hatch-visual [data-shaking="true"]')).not.toBeNull();
    await act(async()=>{vi.advanceTimersByTime(HATCH_OPEN_DURATION_MS-1);});
    expect(screen.queryByRole('textbox',{name:'给伙伴起个名字'})).toBeNull();
    await act(async()=>{vi.advanceTimersByTime(1);});
    expect(document.querySelector('.oc-hatch-visual')).toHaveAttribute('data-phase','revealed');
    expect(screen.getByRole('button',{name:'修改名字'})).toBeVisible();
    expect(screen.getByRole('textbox',{name:'给伙伴起个名字'})).toHaveFocus();
    // jsdom dispatches selectionchange on a zero-delay timer after focus.
    await act(async()=>{vi.advanceTimersByTime(0);});
    view.unmount();
    expect(vi.getTimerCount()).toBe(0);
  } finally {vi.useRealTimers();}
});
it('keeps the rename revision fixed when another host changes the save', async () => {
  let current = owned;
  const {host,social} = hosts(current);
  host.snapshot=vi.fn(async()=>current);
  host.rename=vi.fn(async()=>{throw new Error('伙伴状态已被其他窗口修改');});
  render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'修改名字'}));
  fireEvent.change(screen.getByLabelText('伙伴名称'),{target:{value:'我的修改'}});
  current={...current,revision:9,pet:{...current.pet!,name:'其他窗口'}};
  await act(async()=>{document.dispatchEvent(new Event('visibilitychange'));});
  fireEvent.click(screen.getByRole('button',{name:'保存名称'}));
  await waitFor(()=>expect(host.rename).toHaveBeenCalledWith('我的修改',7));
  await screen.findByRole('button',{name:'修改名字'});
  expect(screen.getByRole('alert')).toHaveTextContent('其他窗口');
});
it('keeps the paid quote fixed across polls and never retries a conflicted draw',async()=>{
  let current=owned;
  const {host,social}=hosts(current);
  host.snapshot=vi.fn(async()=>current);
  host.hatch=vi.fn(async()=>{throw new Error('存档已被其他窗口修改');});
  render(<CompanionPage host={host} socialHost={social}/>);
  fireEvent.click(await screen.findByRole('button',{name:'重新开蛋'}));
  current={...owned,revision:9,pet:{...owned.pet!,xp:83},level:9};
  await act(async()=>{document.dispatchEvent(new Event('visibilitychange'));});
  expect(screen.getByRole('dialog')).toHaveTextContent('73 → 23');
  fireEvent.click(screen.getByRole('button',{name:'确认消耗 50 点并开蛋'}));
  await waitFor(()=>expect(host.hatch).toHaveBeenCalledExactlyOnceWith(7));
  expect(await screen.findByRole('alert')).toHaveTextContent('其他窗口');
  expect(screen.queryByRole('dialog')).toBeNull();
  expect(screen.getByRole('region',{name:'我的伙伴'})).toHaveTextContent('成长经验 83');
});
it('does not replace a successful save with an earlier poll and stops reads on unmount', async () => {
  let resolve!: (snapshot: CompanionSnapshot) => void;
  const {host} = hosts(); host.snapshot = vi.fn(() => new Promise<CompanionSnapshot>(done => {resolve=done;}));
  const {result,unmount} = renderHook(() => useCompanion(host));
  await act(async () => {result.current.accept({...empty,revision:5}); resolve(empty);});
  expect(result.current.snapshot?.revision).toBe(5);
  unmount(); document.dispatchEvent(new Event('visibilitychange'));
  expect(host.snapshot).toHaveBeenCalledTimes(1);
});
