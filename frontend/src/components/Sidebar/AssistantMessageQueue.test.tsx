import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AssistantMessageQueue } from './AssistantMessageQueue';

describe('输入区待发消息', () => {
    it('默认展示两条，更多消息可以展开；即时操作仍对应原消息编号', () => {
        const onSendNow = vi.fn(), onRemove = vi.fn();
        render(<AssistantMessageQueue messages={Array.from({length:4},(_,i)=>({id:i,question:`补充 ${i}`,title:'订单服务'}))}
            paused={false} disabled={false} currentTitle="订单服务" onResume={vi.fn()} onSendNow={onSendNow} onRemove={onRemove}/>);
        expect(screen.queryByText('补充 2')).not.toBeInTheDocument();
        expect(screen.queryByText('订单服务')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button',{name:'还有 2 条'}));
        expect(screen.getByText('补充 2')).toBeVisible();
        fireEvent.click(screen.getAllByRole('button',{name:'立即发送'})[2]);
        expect(onSendNow).toHaveBeenCalledWith(2);
        fireEvent.click(screen.getByRole('button',{name:'移除消息：补充 3'}));
        expect(onRemove).toHaveBeenCalledWith(3);
    });
    it('跨终端待发消息仍显示目标，取消期间禁止发送和移除', () => {
        render(<AssistantMessageQueue messages={[{id:7,question:'查日志',title:'支付节点'}]} paused disabled currentTitle="订单服务"
            onResume={vi.fn()} onSendNow={vi.fn()} onRemove={vi.fn()}/>);
        expect(screen.getByText('支付节点')).toBeVisible();
        expect(screen.getByRole('button',{name:'继续发送'})).toBeDisabled();
        expect(screen.getByRole('button',{name:'立即发送'})).toBeDisabled();
        expect(screen.getByRole('button',{name:'移除消息：查日志'})).toBeDisabled();
    });
});
