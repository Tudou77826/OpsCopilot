import React from 'react';
import { render, screen, fireEvent, within, cleanup } from '@testing-library/react';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { ProductFrame, ProductToolbar } from './ProductChrome';
import CommandGrid from '../quickcmd/CommandGrid';
import QuickCommandPanel from '../quickcmd/QuickCommandPanel';

afterEach(cleanup);
describe('desktop product reuse', () => {
 it('uses theme border colors for the quick-command panel and group dividers', async () => {
   const host={execute:vi.fn(),storage:{load:vi.fn(async()=>[{id:'one',name:'测试命令',content:'pwd',group:'default'}]),add:vi.fn(),update:vi.fn(),remove:vi.fn(),reorder:vi.fn()}};
   render(<QuickCommandPanel host={host} isOpen onExecute={host.execute}/>);
   await screen.findByText('测试命令');
   expect(screen.getByTestId('quick-command-panel').style.borderTop).toBe('1px solid var(--border-strong)');
   expect(screen.getByTestId('group-strip').style.borderLeft).toBe('1px solid var(--border)');
 });
 it('keeps quick commands below the terminal and beside, not inside, the sidebar', () => {
   render(<ProductFrame toolbar={<div>toolbar</div>} terminal={<div>terminal</div>} quickCommands={<div>commands</div>} sidebar={<div>sidebar</div>} navigation={<div>navigation</div>} footer={<div>footer</div>}/>);
   const terminalContainer=screen.getByText('terminal').parentElement!;
   const column=terminalContainer.parentElement!;
   expect(column).toHaveStyle({display:'flex',flexDirection:'column'});
   expect(column.children[1]).toBe(screen.getByText('commands'));
   expect(column.parentElement).toHaveStyle({flexDirection:'row'});
   expect(column.nextElementSibling).toBe(screen.getByText('sidebar'));
   expect(screen.getByText('sidebar').nextElementSibling).toBe(screen.getByText('navigation'));
 });
 it('uses the desktop logo and left toolbar action order', () => {
   const connect=vi.fn();
   render(<ProductToolbar theme="light" onNewConnection={connect} onThemeToggle={vi.fn()} onSettings={vi.fn()}/>);
   expect(screen.getByAltText('OpsCopilot')).toHaveClass('shell-brand-logo');
   expect(screen.getAllByRole('button').map(b=>b.textContent || b.title)).toEqual(['+ 新建连接','切换到暗色','设置']);
   fireEvent.click(screen.getByText('+ 新建连接')); expect(connect).toHaveBeenCalledOnce();
 });
 it('allows editing a quick command through its original menu inside Shadow DOM', () => {
   const host=document.createElement('div'); document.body.appendChild(host);
   const shadow=host.attachShadow({mode:'open'}); const mount=document.createElement('div');shadow.appendChild(mount);
   const edit=vi.fn();
   const view=render(<CommandGrid commands={[{id:'one',name:'查看目录',content:'pwd',group:'default'}]} onExecute={vi.fn()} onEdit={edit} onDelete={vi.fn()} onAdd={vi.fn()} searchQuery="" onSearchChange={vi.fn()} onReorder={vi.fn()}/>,{container:mount});
   fireEvent.contextMenu(within(mount).getByText('查看目录'),{clientX:10,clientY:20});
   const item=within(mount).getByText('编辑',{exact:true});
   fireEvent(item,new Event('pointerdown',{bubbles:true,composed:true}));
   expect(within(mount).getByText('编辑',{exact:true})).toBeTruthy();
   fireEvent.click(item); expect(edit).toHaveBeenCalledOnce();
   view.unmount();host.remove();
 });
});
