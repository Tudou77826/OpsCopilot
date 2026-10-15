import React from 'react';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { afterEach, it, expect, vi } from 'vitest';
import ShellSettingsModal from './ShellSettingsModal';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';
afterEach(cleanup);
function setup() {
  const assistantSettings={load:vi.fn(async()=>({personaPrompt:'',defaultPrompt:'温和、可靠的运维伙伴',revision:'v1'})),save:vi.fn(async (personaPrompt:string)=>({personaPrompt,defaultPrompt:'温和、可靠的运维伙伴',revision:'v2'}))};
  const runtime={load:vi.fn(async()=>({theme:'light' as const,terminal:normalizeTerminalConfig(),completionDelay:150,highlightRules:[]})),save:vi.fn(async()=>{})};
  const onClose=vi.fn();
  render(<ShellSettingsModal isOpen runtime={runtime} assistantSettings={assistantSettings} onApply={()=>{}} onClose={onClose}/>);
  return {assistantSettings,runtime,onClose};
}
it('saves persona independently and offers default restoration with unsaved-close protection',async()=>{
  const {assistantSettings,runtime,onClose}=setup();
  await screen.findByText('终端外观',{exact:true});
  fireEvent.click(screen.getByText('伙伴与奖章',{exact:true}));
  expect(screen.getByRole('navigation')).not.toHaveTextContent('伙伴人设');
  expect(screen.getByLabelText('人设提示词')).not.toBeVisible();
  fireEvent.click(screen.getByText('伙伴人设',{exact:true}));
  const input=await screen.findByLabelText('人设提示词');
  await waitFor(()=>expect(input).toHaveValue('温和、可靠的运维伙伴'));
  fireEvent.change(input,{target:{value:'你叫小圆，直接回答。'}});
  expect(assistantSettings.save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('取消',{exact:true}));
  expect(screen.getByText('有未保存的更改')).toBeTruthy();expect(onClose).not.toHaveBeenCalled();
  fireEvent.click(screen.getByText('继续编辑'));
  fireEvent.click(screen.getByText('保存更改'));
  await waitFor(()=>expect(assistantSettings.save).toHaveBeenCalledWith('你叫小圆，直接回答。','v1'));
  expect(runtime.save).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole('button',{name:'恢复默认'}));
  expect(input).toHaveValue('温和、可靠的运维伙伴');
  fireEvent.click(screen.getByText('保存更改'));
  await waitFor(()=>expect(assistantSettings.save).toHaveBeenLastCalledWith('温和、可靠的运维伙伴','v2'));
});
it('applies one of seven personality templates only to the unsaved draft',async()=>{
 const {assistantSettings}=setup();await screen.findByText('终端外观',{exact:true});fireEvent.click(screen.getByText('伙伴与奖章',{exact:true}));
  expect(screen.getByRole('navigation')).not.toHaveTextContent('伙伴人设');
  expect(screen.getByLabelText('人设提示词')).not.toBeVisible();
  fireEvent.click(screen.getByText('伙伴人设',{exact:true}));
 const input=await screen.findByLabelText('人设提示词');await waitFor(()=>expect(input).toHaveValue('温和、可靠的运维伙伴'));
 const choices=screen.getByLabelText('性格模板');expect(choices.querySelectorAll('option')).toHaveLength(7);
 fireEvent.change(choices,{target:{value:'bolt'}});expect(input).toHaveValue('温和、可靠的运维伙伴');
 fireEvent.click(screen.getByRole('button',{name:'使用模板'}));expect((input as HTMLTextAreaElement).value).toContain('小栓');
 expect(assistantSettings.save).not.toHaveBeenCalled();
 fireEvent.click(screen.getByRole('button',{name:'保存更改'}));await waitFor(()=>expect(assistantSettings.save).toHaveBeenCalledWith(expect.stringContaining('严谨'),'v1'));
});
it('keeps a failed save draft and the original revision for retry',async()=>{
  const {assistantSettings}=setup();await screen.findByText('终端外观',{exact:true});
  fireEvent.click(screen.getByText('伙伴与奖章',{exact:true}));
  expect(screen.getByRole('navigation')).not.toHaveTextContent('伙伴人设');
  expect(screen.getByLabelText('人设提示词')).not.toBeVisible();
  fireEvent.click(screen.getByText('伙伴人设',{exact:true}));
  const input=await screen.findByLabelText('人设提示词');await waitFor(()=>expect(input).toHaveValue('温和、可靠的运维伙伴'));
  assistantSettings.save.mockRejectedValueOnce(new Error('磁盘不可写'));
  fireEvent.change(input,{target:{value:'自定义'}});fireEvent.click(screen.getByText('保存更改'));
  await screen.findByText(/磁盘不可写/);expect(input).toHaveValue('自定义');
  fireEvent.click(screen.getByText('保存更改'));
  await waitFor(()=>expect(assistantSettings.save).toHaveBeenCalledTimes(2));
  expect(assistantSettings.save).toHaveBeenLastCalledWith('自定义','v1');
});

it('retains a collapsed draft and saves it with Ctrl+S while growth is disabled',async()=>{
  const {assistantSettings,runtime}=setup();await screen.findByText('终端外观',{exact:true});
  fireEvent.click(screen.getByText('伙伴与奖章',{exact:true}));
  expect(screen.queryByRole('button',{name:'保存更改'})).toBeNull();
  const entry=screen.getByText('伙伴人设',{exact:true});fireEvent.click(entry);
  const input=screen.getByLabelText('人设提示词');await waitFor(()=>expect(input).toHaveValue('温和、可靠的运维伙伴'));
  fireEvent.change(input,{target:{value:'保留折叠草稿'}});fireEvent.click(entry);
  expect(input).not.toBeVisible();expect(input).toHaveValue('保留折叠草稿');
  expect(screen.getByRole('button',{name:'保存更改'})).toBeVisible();
  fireEvent.keyDown(window,{key:'s',ctrlKey:true});
  await waitFor(()=>expect(assistantSettings.save).toHaveBeenCalledWith('保留折叠草稿','v1'));
  expect(runtime.save).not.toHaveBeenCalled();
  await waitFor(()=>expect(screen.queryByRole('button',{name:'保存更改'})).toBeNull());
});
