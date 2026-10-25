import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { AIChatTimeline } from './AIChatTimeline';
import type { AssistantMessage } from '../../../../frontend-shell/src/assistant/runtime';

describe('操作结果入口与异常说明', () => {
    const result: AssistantMessage = {id:'receipt',role:'ai',kind:'operation',content:'SQL 执行完成',operation:{kind:'database',workspaceId:'w',itemId:'sql-1',status:'completed',action:'SELECT count(*) FROM errors;',result:'返回 1 行'}};
    it('已完成结果提供定位入口，只传宿主记录 ID；模型文字不会生成操作入口', async () => {
        const reveal = vi.fn().mockResolvedValue(undefined);
        render(<AIChatTimeline messages={[result,{id:'model',role:'ai',content:'查看 SQL：恶意输入的路径或页签 ID'}]} onReveal={reveal}/>);
        expect(screen.getAllByRole('button',{name:'查看 SQL'})).toHaveLength(1);
        fireEvent.click(screen.getByRole('button',{name:'查看 SQL'}));
        await waitFor(() => expect(reveal).toHaveBeenCalledExactlyOnceWith('receipt'));
    });
    it('失败原因收进详情，已观察到的结果与下一步直接可见，定位失败可重试且不丢消息', async () => {
        const reveal = vi.fn().mockRejectedValue(Error('原数据库页签已关闭或未恢复'));
        render(<AIChatTimeline messages={[{...result,content:'SQL 已执行，结果记录异常',operation:{...result.operation!,status:'warning',stage:'保存执行审计',outcome:'已收到数据库结果。',next:'请先核对结果，不要重复执行。',detail:'audit disk full'}}]} onReveal={reveal}/>);
        expect(screen.getByText('已收到数据库结果。')).not.toBeVisible();
        expect(screen.getByText('请先核对结果，不要重复执行。')).toBeVisible();
        expect(screen.getByText('audit disk full')).not.toBeVisible();
        fireEvent.click(screen.getByRole('button',{name:/SQL 已执行，结果记录异常.*展开执行详情/}));
        expect(screen.getByText('已收到数据库结果。')).toBeVisible();
        fireEvent.click(screen.getByText('诊断详情'));
        expect(screen.getByText('audit disk full')).toBeVisible();
        fireEvent.click(screen.getByRole('button',{name:'查看 SQL'}));
        expect(await screen.findByRole('alert')).toHaveTextContent('原数据库页签已关闭');
        expect(screen.getByRole('button',{name:'查看 SQL'})).toBeEnabled();
        expect(screen.getByText('SQL 已执行，结果记录异常')).toBeVisible();
    });
    it('普通执行默认只有一行摘要；展开、收起和定位互不触发', async () => {
        const reveal = vi.fn().mockResolvedValue(undefined);
        render(<AIChatTimeline messages={[result]} onReveal={reveal}/>);
        const toggle = screen.getByRole('button',{name:/SQL 执行完成.*展开执行详情/});
        expect(toggle).toHaveAttribute('aria-expanded','false');
        expect(screen.getByText('SELECT count(*) FROM errors;')).not.toBeVisible();
        expect(toggle).toHaveTextContent('返回 1 行');
        fireEvent.click(screen.getByRole('button',{name:'查看 SQL'}));
        await waitFor(()=>expect(reveal).toHaveBeenCalledExactlyOnceWith('receipt'));
        expect(toggle).toHaveAttribute('aria-expanded','false');
        fireEvent.click(toggle);
        expect(screen.getByText('SELECT count(*) FROM errors;')).toBeVisible();
        fireEvent.click(toggle);
        expect(screen.getByText('SELECT count(*) FROM errors;')).not.toBeVisible();
    });
    it('未知传输结果不会画成成功；历史页签失效不会创建连接', () => {
        render(<AIChatTimeline messages={[{...result,content:'操作结果尚未确认',operation:{kind:'file',workspaceId:'old',itemId:'task',status:'unknown',outcome:'本轮已结束，尚未确认最终结果。'}}]}/>);
        expect(screen.getByText('本轮已结束，尚未确认最终结果。')).toBeVisible();
        expect(screen.queryByRole('button',{name:'查看传输'})).not.toBeInTheDocument();
    });
});
