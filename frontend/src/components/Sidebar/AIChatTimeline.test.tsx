import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { AssistantMessage } from '../../../../frontend-shell/src/assistant/runtime';
import { AIChatTimeline } from './AIChatTimeline';

function message(id: string, kind: string, content: string, turnId = 'turn'): AssistantMessage {
    return { id, kind, content, turnId, role: kind === 'answer' ? 'user' : 'ai', time: '2026-10-23T07:29:07Z' };
}

function execution(id: string, status = 'completed', turnId = 'turn'): AssistantMessage {
    return {...message(id, 'operation', `检查 ${id}`, turnId), operation: {
        kind: 'terminal', workspaceId: 'ssh', status, action: `ls /tmp/${id}`, result: `${id}.log`,
    }};
}

describe('应用读取记录与耗时', () => {
    it('读取默认合成一行，展开后显示各自结果及分开的整轮耗时', () => {
        const timing='本轮 18.00 秒 · 助手响应 17.99 秒 · 工具执行 0.010 秒 · 等待确认 0.00 秒';
        render(<AIChatTimeline messages={[
            {...execution('list'),content:'查询快捷指令列表',operation:{kind:'reference',workspaceId:'',status:'completed',action:'查询快捷指令列表',result:'共 2 条\nkk\naa',measured:true,durationMs:7}},
            {...execution('read'),content:'读取快捷指令',operation:{kind:'reference',workspaceId:'',status:'completed',action:'读取快捷指令',result:'aa\naa test',measured:true,durationMs:3}},
            message('reply','text','新增了 aa，现在共 2 条。'),message('timing','timing',timing),
        ]}/>);
        const summary=screen.getByRole('button',{name:/已完成 2 项操作/});
        expect(summary).toHaveAttribute('title',expect.stringContaining(timing));
        expect(screen.getByText(timing)).not.toBeVisible();
        expect(screen.getByText('新增了 aa，现在共 2 条。')).toBeVisible();
        expect(screen.queryByRole('button',{name:'查看终端'})).not.toBeInTheDocument();
        fireEvent.click(summary);
        expect(screen.getByText(timing)).toBeVisible();
        expect(screen.getByText('7ms')).toBeVisible();
        fireEvent.click(screen.getByRole('button',{name:/读取快捷指令，完成，展开执行详情/}));
        expect(screen.getByText(/aa\s+aa test/)).toBeVisible();
    });
    it('读取失败保留在主线，不混入成功的折叠组', () => {
        render(<AIChatTimeline messages={[
            {...execution('list'),content:'查询快捷指令列表'},
            {...execution('failed','failed'),content:'读取快捷指令',operation:{kind:'reference',workspaceId:'',status:'failed',action:'读取快捷指令',outcome:'本次调用未完成。',detail:'source unavailable',next:'核对原因后再继续。'}},
        ]}/>);
        expect(screen.getByText('核对原因后再继续。')).toBeVisible();
        expect(screen.queryByRole('button',{name:/已完成 2 项操作/})).not.toBeInTheDocument();
    });
});

describe('对话内容与过程记录', () => {
    it('软件过程折叠，执行摘要紧凑呈现，业务问题与回答保留主线', () => {
        render(<AIChatTimeline messages={[
            {...message('open','operation','文件页签已打开'),operation:{kind:'file',workspaceId:'w',status:'completed'}},
            message('permit','permission','允许助手读取目录？'),
            {...message('permit-answer','permission_answer','允许本次'),role:'user'},
            {...message('cmd','operation','检查支付服务的错误日志'),operation:{kind:'terminal',workspaceId:'ssh',status:'completed',action:"grep -n ERROR /var/log/payments.log",result:'32: upstream timeout'}},
            message('q','question','检查上午还是下午？'),message('a','answer','上午'),
            message('finding','text','超时集中在支付上游，请进一步核对它的响应时间。'),
        ]}/>);
        expect(screen.getByText('文件页签已打开')).not.toBeVisible();
        expect(screen.getByText('允许助手读取目录？')).not.toBeVisible();
        expect(screen.getByText('允许本次')).not.toBeVisible();
        expect(screen.getByText('grep -n ERROR /var/log/payments.log')).not.toBeVisible();
        const execution = screen.getByRole('button',{name:/检查支付服务的错误日志.*展开执行详情/});
        expect(within(execution).getByText('32: upstream timeout')).toBeVisible();
        expect(screen.getByText('上午')).toBeVisible();
        expect(screen.getByText('超时集中在支付上游，请进一步核对它的响应时间。')).toBeVisible();
        fireEvent.click(screen.getByText('过程记录'));
        expect(screen.getByText('允许本次')).toBeVisible();
        fireEvent.click(execution);
        expect(screen.getByText('grep -n ERROR /var/log/payments.log')).toBeVisible();
    });
    it('连续自审默认折叠，展开可查原文与时间，助手结果始终可见', () => {
        const view = render(<AIChatTimeline messages={[
            message('r1', 'review', '只读自审通过'),
            message('r2', 'review', '下一条只读检查通过'),
            message('result', 'text', '发现两个日志文件，共 46 字节。'),
        ]}/>);
        expect(screen.getByText('发现两个日志文件，共 46 字节。')).toBeVisible();
        expect(screen.getByText('过程记录')).toBeVisible();
        expect(screen.getByText('2 项')).toBeVisible();
        expect(screen.getByText('只读自审通过')).not.toBeVisible();
        fireEvent.click(screen.getByText('过程记录'));
        expect(screen.getByText('只读自审通过')).toBeVisible();
        expect(view.container.querySelector('details time')).toHaveAttribute('datetime', '2026-10-23T07:29:07Z');
    });

    it('提问、已选答案、失败与普通模型文字不折叠，也不改变顺序', () => {
        render(<AIChatTimeline messages={[
            message('r1', 'review', '自审无法确认'),
            message('q1', 'question', '是否继续检查？'),
            message('a1', 'answer', '只检查今天'),
            message('text', 'text', '命令失败了，正在定位原因。'),
            message('error', 'notice', '连接已断开，本轮未完成'),
            message('r2', 'review', '第二次检查通过'),
        ]}/>);
        const records = screen.getAllByTestId('message-item').filter(element => !element.closest('details'));
        expect(records).toHaveLength(4);
        expect(records.map(element => element.textContent)).toEqual([
            expect.stringContaining('是否继续检查？'),
            expect.stringContaining('只检查今天'),
            expect.stringContaining('命令失败了'),
            expect.stringContaining('连接已断开'),
        ]);
        records.forEach(element => expect(element).toBeVisible());
        expect(screen.getAllByText('过程记录')).toHaveLength(2);
    });

    it('降级提示保持可见，完整原因收进记录；新增记录不收起已展开内容', () => {
        const fallback = message('f1', 'review_fallback', '快速模型访问失败，本次改用隔离的 iCode 会话。');
        const view = render(<AIChatTimeline messages={[fallback]}/>);
        expect(screen.getByRole('status')).toHaveTextContent('建议配置可用的快速模型');
        expect(screen.getByText(fallback.content)).not.toBeVisible();
        fireEvent.click(screen.getByText('过程记录'));
        view.rerender(<AIChatTimeline messages={[fallback, message('r1', 'review', '自审通过')]}/>);
        expect(screen.getByText(fallback.content)).toBeVisible();
        expect(screen.getByText('自审通过')).toBeVisible();
        expect(within(view.container.querySelector('summary')!).getByText('2 项')).toBeVisible();
    });

    it('不合并不同轮次的过程，也不把用户文本识别为过程指令', () => {
        const review = message('r2', 'review', '另一轮自审', 'another-turn');
        render(<AIChatTimeline messages={[
            message('r1', 'review', '第一轮自审'), review,
            { ...message('u1', 'review', '只读自审通过'), role: 'user' },
        ]}/>);
        expect(screen.getAllByText('过程记录')).toHaveLength(2);
        expect(screen.getByText('只读自审通过')).toBeVisible();
    });

    it('连续操作跨自审记录归组，计数不包含授权；原文与定位仍可用', async () => {
        const reveal = vi.fn().mockResolvedValue(undefined);
        render(<AIChatTimeline messages={[
            execution('one'), message('r1', 'review', '只读自审通过'),
            execution('two'), message('p1', 'permission', '历史操作确认'), execution('three'),
            message('finding', 'text', '发现连接等待超时。'),
        ]} onReveal={reveal}/>);
        const summary = screen.getByRole('button', {name: /已完成 3 项操作/});
        expect(summary).toHaveAttribute('aria-expanded', 'false');
        expect(screen.getByText('检查 one')).not.toBeVisible();
        expect(screen.getByText('发现连接等待超时。')).toBeVisible();
        fireEvent.click(summary);
        const row = screen.getByRole('button', {name: /检查 two.*展开执行详情/});
        fireEvent.click(row);
        expect(screen.getByText('ls /tmp/two')).toBeVisible();
        expect(screen.getByText('历史操作确认')).not.toBeVisible();
        const group = summary.parentElement!;
        fireEvent.click(within(group).getAllByRole('button', {name: '查看终端'})[1]);
        await waitFor(() => expect(reveal).toHaveBeenCalledExactlyOnceWith('two'));
        fireEvent.click(summary);
        expect(screen.getByText('ls /tmp/two')).not.toBeVisible();
        fireEvent.click(summary);
        expect(screen.getByText('ls /tmp/two')).toBeVisible();
    });

    it('助手发言、提问、用户介入和轮次边界结束归组', () => {
        render(<AIChatTimeline messages={[
            execution('a'), execution('b'), message('text', 'text', '先解释当前发现。'),
            execution('c'), execution('d'), message('q', 'question', '看哪个时间段？'),
            execution('e'), execution('f'), message('user-answer', 'answer', '只查今天'),
            execution('g'), execution('h'), execution('i', 'completed', 'next'), execution('j', 'completed', 'next'),
            {...execution('user'), role: 'user'},
        ]}/>);
        expect(screen.getAllByRole('button', {name: /已完成 2 项操作/})).toHaveLength(5);
        expect(screen.getByText('先解释当前发现。')).toBeVisible();
        expect(screen.getByText('看哪个时间段？')).toBeVisible();
        expect(screen.getByText('只查今天')).toBeVisible();
        expect(screen.getByText('检查 user')).toBeVisible();
    });

    it.each(['failed', 'warning', 'unknown', 'cancelled', 'cancelling', 'not_executed', 'unrecognised'])('状态 %s 不进入成功或进行中的操作组', status => {
        render(<AIChatTimeline messages={[
            execution('one'), execution('two'),
            {...execution('problem', status), operation: {...execution('problem', status).operation!, next: '先核对实际状态。'}},
            execution('three'), execution('four'),
        ]}/>);
        expect(screen.getAllByRole('button', {name: /已完成 2 项操作/})).toHaveLength(2);
        expect(screen.getByText('检查 problem')).toBeVisible();
        if (['failed', 'warning', 'unknown', 'cancelled'].includes(status)) expect(screen.getByText('先核对实际状态。')).toBeVisible();
    });

    it('运行状态原位更新，展开后新增操作与完成通知不收起详情', () => {
        const messages = [execution('one'), execution('two', 'running')];
        const view = render(<AIChatTimeline messages={messages}/>);
        const summary = screen.getByRole('button', {name: /正在执行 · 已完成 1 项/});
        fireEvent.click(summary);
        fireEvent.click(screen.getByRole('button', {name: /检查 one.*展开执行详情/}));
        view.rerender(<AIChatTimeline messages={[execution('one'), execution('two'), execution('three', 'running')]}/>);
        expect(screen.getByRole('button', {name: /正在执行 · 已完成 2 项/})).toBe(summary);
        expect(screen.getByText('ls /tmp/one')).toBeVisible();
        view.rerender(<AIChatTimeline messages={[execution('one'), execution('two'), execution('three')]}/>);
        expect(screen.getByRole('button', {name: /已完成 3 项操作/})).toBe(summary);
        expect(summary).toHaveAttribute('aria-expanded', 'true');
        expect(screen.getByText('ls /tmp/one')).toBeVisible();
    });

    it('单条仍直接显示；查看中的单条详情不会在形成分组时消失', () => {
        const view = render(<AIChatTimeline messages={[execution('one')]}/>);
        fireEvent.click(screen.getByRole('button', {name: /检查 one.*展开执行详情/}));
        const command = screen.getByText('ls /tmp/one');
        view.rerender(<AIChatTimeline messages={[execution('one'), execution('two', 'running')]}/>);
        expect(screen.getByRole('button', {name: /正在执行 · 已完成 1 项/})).toHaveAttribute('aria-expanded', 'true');
        expect(command).toBeVisible();
        expect(screen.getByText('ls /tmp/one')).toBe(command);
    });

    it('组内降级提示不被折叠吞掉；展开后不重复显示提示', () => {
        render(<AIChatTimeline messages={[
            execution('one'), message('fallback', 'review_fallback', '快速模型访问失败'), execution('two'),
        ]}/>);
        expect(screen.getAllByRole('status')).toHaveLength(1);
        fireEvent.click(screen.getByRole('button', {name: /已完成 2 项操作/}));
        expect(screen.getAllByRole('status')).toHaveLength(1);
        expect(screen.getByRole('status')).toHaveTextContent('建议配置可用的快速模型');
    });

    it('运行中失败自动退出折叠组，已完成的事实仍保留', () => {
        const view = render(<AIChatTimeline messages={[execution('one'), execution('two'), execution('three', 'running')]}/>);
        view.rerender(<AIChatTimeline messages={[
            execution('one'), execution('two'),
            {...execution('three', 'failed'), operation: {...execution('three', 'failed').operation!, next: '连接断开，检查未完成。'}},
        ]}/>);
        expect(screen.getByRole('button', {name: /已完成 2 项操作/})).toHaveAttribute('aria-expanded', 'false');
        expect(screen.getByText('检查 three')).toBeVisible();
        expect(screen.getByText('连接断开，检查未完成。')).toBeVisible();
    });
});
