import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AIComposer from './AIComposer';

describe('助手输入与中断', () => {
    it('中文输入法确认候选词时不触发发送', () => {
        const send=vi.fn();
        render(<AIComposer value="排查订单" onChange={vi.fn()} onSend={send}/>);
        const input=screen.getByRole('textbox');
        fireEvent.keyDown(input,{key:'Enter',isComposing:true});
        expect(send).not.toHaveBeenCalled();
        fireEvent.keyDown(input,{key:'Enter'});
        expect(send).toHaveBeenCalledOnce();
    });
    it('运行时输入保持可编辑，只有停止按钮能够中断', () => {
        const send=vi.fn(),stop=vi.fn(),change=vi.fn();
        render(<AIComposer value="下一步" onChange={change} onSend={send} busy onStop={stop}/>);
        const input=screen.getByRole('textbox');
        expect(input).toBeEnabled();
        fireEvent.change(input,{target:{value:'补充说明'}});
        expect(change).toHaveBeenCalledWith('补充说明');
        fireEvent.keyDown(input,{key:'Enter'});
        expect(send).not.toHaveBeenCalled();
        expect(stop).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button',{name:'停止本轮'}));
        expect(stop).toHaveBeenCalledOnce();
    });
    it('启用队列后回车仅排队，停止按钮仍独立可用，输入法确认不排队', () => {
        const send=vi.fn(),stop=vi.fn();
        render(<AIComposer value="补充现象" onChange={vi.fn()} onSend={send} busy queueWhileBusy onStop={stop}/>);
        const input=screen.getByRole('textbox');
        fireEvent.keyDown(input,{key:'Enter',isComposing:true});
        expect(send).not.toHaveBeenCalled();
        fireEvent.keyDown(input,{key:'Enter'});
        expect(send).toHaveBeenCalledOnce();
        expect(stop).not.toHaveBeenCalled();
        expect(screen.getByRole('button',{name:'加入队列'})).toBeEnabled();
        fireEvent.click(screen.getByRole('button',{name:'停止本轮'}));
        expect(stop).toHaveBeenCalledOnce();
    });
});
