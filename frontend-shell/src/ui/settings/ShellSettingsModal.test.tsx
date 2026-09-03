import React from 'react';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { afterEach, describe, it, expect, vi } from 'vitest';
import ShellSettingsModal, { type ShellSettings } from './ShellSettingsModal';
import { normalizeTerminalConfig } from '../Terminal/terminalAppearance';

afterEach(cleanup);
const initial: ShellSettings={theme:'light',terminal:normalizeTerminalConfig(),completionDelay:150,highlightRules:[],commandQueryShortcut:'Ctrl+K'};
function setup() {
 const runtime={load:vi.fn(async()=>initial),save:vi.fn(async()=>{})};
 const onClose=vi.fn(),onApply=vi.fn();
 render(<ShellSettingsModal embedded isOpen onClose={onClose} runtime={runtime} initial={initial} onApply={onApply}/>);
 return {runtime,onClose,onApply};
}
describe('shared desktop settings UX', () => {
 it('opens the desktop settings frame with original appearance page and navigation', async()=>{
   setup();
   expect(await screen.findByText('终端外观',{exact:true})).toBeTruthy();
   expect(screen.getByText('系统设置')).toBeTruthy();
   expect(screen.queryByText('Shell 设置')).toBeNull();
   fireEvent.click(screen.getByText('快捷键',{exact:true}));
   expect(screen.getByText('快捷键说明')).toBeTruthy();
 });
 it('keeps zero completion delay and uses the original save action', async()=>{
   const {runtime}=setup(); await screen.findByText('终端外观',{exact:true});
   fireEvent.click(screen.getByText('高级选项',{exact:true}));
   fireEvent.change(screen.getByRole('spinbutton'),{target:{value:'0'}});
   fireEvent.click(screen.getByText('保存更改'));
   await waitFor(()=>expect(runtime.save).toHaveBeenCalledWith(expect.objectContaining({completionDelay:0})));
 });
 it('asks about unsaved edits and restores the applied appearance on discard', async()=>{
   const {onClose,onApply}=setup(); await screen.findByText('终端外观',{exact:true});
   fireEvent.click(screen.getByRole('radio',{name:'暗色'}));
   fireEvent.click(screen.getByText('取消',{exact:true}));
   expect(screen.getByText('有未保存的更改')).toBeTruthy(); expect(onClose).not.toHaveBeenCalled();
   fireEvent.click(screen.getByText('放弃更改'));
   expect(onApply).toHaveBeenLastCalledWith(initial);expect(onClose).toHaveBeenCalledOnce();
 });
});
