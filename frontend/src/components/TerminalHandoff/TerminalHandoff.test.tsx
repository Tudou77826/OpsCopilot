import React from 'react';
import { act, fireEvent, render, renderHook, screen, waitFor } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { TerminalHandoff, useTerminalHandoff, type HandoffState } from './TerminalHandoff';

const base: HandoffState = { sessionId: 'one', runId: '', phase: 'idle', owned: false, ready: false, message: '' };

function deferred<T>() {
    let resolve!: (value: T) => void;
    const promise = new Promise<T>(done => { resolve = done; });
    return { promise, resolve };
}

describe('终端接手交接', () => {
    it('模型对话和思考不锁终端，只有真实占用才遮罩，释放后立即恢复', () => {
        const work = {turnId:'assistant',source:'assistant',target:{tabId:'one',connectionId:'c1'},phase:'running',message:'正在思考'};
        const state: HandoffState = {...base,work};
        const props = {transportError:'',connected:true,onStart:vi.fn(),onStop:vi.fn(),onFocus:vi.fn()};
        const view=render(<TerminalHandoff experimental {...props} state={state}><textarea aria-label="手动终端"/></TerminalHandoff>);
        expect(screen.getByLabelText('手动终端').parentElement).not.toHaveAttribute('inert');
        expect(screen.queryByLabelText('接手期间终端由测试助手独占')).not.toBeInTheDocument();
        expect(screen.queryByText('正在思考')).not.toBeInTheDocument();
        view.rerender(<TerminalHandoff experimental {...props} state={{...state,owned:true}}><textarea aria-label="手动终端"/></TerminalHandoff>);
        expect(screen.getByLabelText('手动终端').parentElement).toHaveAttribute('inert');
        expect(screen.getByText('AI 正在使用此终端')).toBeInTheDocument();
        view.rerender(<TerminalHandoff experimental {...props} state={{...state,owned:false}}><textarea aria-label="手动终端"/></TerminalHandoff>);
        expect(screen.getByLabelText('手动终端').parentElement).not.toHaveAttribute('inert');
        expect(screen.queryByText('AI 正在使用此终端')).not.toBeInTheDocument();
    });
    it('等待确认时终端可用，并能从停止入口撤销整轮', async () => {
        const stop = vi.fn().mockResolvedValue(undefined);
        const respond = vi.fn().mockResolvedValue(undefined);
        const waiting: HandoffState = { ...base, work: { turnId: 'work1', target: { tabId: 'one', connectionId: 'c1' }, phase: 'waiting', message: '等待确认', prompt: { id: 'p1', kind: 'approval', title: '确认命令', description: '', multiple: false, expires: new Date(Date.now() + 60000).toISOString() } } };
        render(<TerminalHandoff experimental state={waiting} transportError="" connected onStart={vi.fn()} onStop={stop} onRespond={respond} onFocus={vi.fn()}><textarea aria-label="手动终端" /></TerminalHandoff>);
        expect(screen.getByLabelText('手动终端').parentElement).not.toHaveAttribute('inert');
        fireEvent.click(screen.getByRole('button', { name: '允许本次' }));
        await waitFor(() => expect(respond).toHaveBeenCalledWith(waiting, { text: '', selected: ['allow'] }));
        fireEvent.click(screen.getByRole('button', { name: '停止本轮' }));
        await waitFor(() => expect(stop).toHaveBeenCalledWith(waiting));
    });
    it('页面离开时撤销不占用页签的待批轮次', async () => {
        const waiting: HandoffState = { ...base, work: { turnId: 'work1', target: { tabId: 'one', connectionId: 'c1' }, phase: 'waiting', message: '等待确认' } };
        const host = { states: vi.fn().mockResolvedValue([waiting]), start: vi.fn(), stop: vi.fn(), stopWork: vi.fn().mockResolvedValue(undefined) };
        const view = renderHook(() => useTerminalHandoff(host));
        await waitFor(() => expect(view.result.current.states.one?.work?.turnId).toBe('work1'));
        fireEvent(window, new Event('pagehide'));
        await waitFor(() => expect(host.stopWork).toHaveBeenCalledWith('work1'));
        expect(host.stop).not.toHaveBeenCalled();
        view.unmount();
    });
    it('同一页签换了底层连接后，必须重新确认空闲', () => {
        const props = { transportError: '', connected: true, onStart: vi.fn(), onStop: vi.fn(), onFocus: vi.fn() };
        const view = render(<TerminalHandoff experimental {...props} state={{ ...base, connectionId: 'c1' }}><textarea /></TerminalHandoff>);
        fireEvent.click(screen.getByRole('button', { name: /接手测试/ }));
        fireEvent.click(screen.getByRole('checkbox'));
        view.rerender(<TerminalHandoff experimental {...props} state={{ ...base, connectionId: 'c2' }}><textarea /></TerminalHandoff>);
        expect(screen.queryByLabelText('开始接手测试')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: /接手测试/ }));
        expect(screen.getByRole('checkbox')).not.toBeChecked();
    });

    it('重连后旧启动响应不能覆盖新连接，也不能占用新连接', async () => {
        const response = deferred<HandoffState>();
        const first = { ...base, connectionId: 'c1' };
        const second = { ...base, connectionId: 'c2' };
        const host = { states: vi.fn().mockResolvedValueOnce([first]).mockResolvedValue([second]), start: vi.fn().mockReturnValue(response.promise), stop: vi.fn().mockResolvedValue(undefined) };
        vi.useFakeTimers();
        const view = renderHook(() => useTerminalHandoff(host));
        try {
            await act(async () => { await vi.advanceTimersByTimeAsync(1); });
            let pending!: Promise<void>;
            act(() => { pending = view.result.current.start('one', 'inspect', true); });
            expect(host.start).toHaveBeenCalledWith('one', 'inspect', true, 'c1');
            await act(async () => { await vi.advanceTimersByTimeAsync(400); });
            await act(async () => { response.resolve({ ...first, runId: 'r1', owned: true }); await pending; });
            expect(view.result.current.states.one).toMatchObject({ connectionId: 'c2', owned: false });
            expect(host.stop).toHaveBeenCalledWith('one', 'r1');
        } finally { view.unmount(); vi.useRealTimers(); }
    });

    it('页面恢复后忽略离开前发起的轮询，使用新快照', async () => {
        const response = deferred<HandoffState[]>();
        const running = { ...base, runId: 'r1', owned: true };
        const host = { states: vi.fn().mockReturnValueOnce(response.promise).mockResolvedValue([base]), start: vi.fn(), stop: vi.fn().mockResolvedValue(undefined) };
        vi.useFakeTimers();
        const view = renderHook(() => useTerminalHandoff(host));
        try {
            fireEvent(window, new Event('pagehide'));
            fireEvent(window, new Event('pageshow'));
            await act(async () => { response.resolve([running]); await Promise.resolve(); });
            expect(view.result.current.states.one).toBeUndefined();
            await act(async () => { await vi.advanceTimersByTimeAsync(400); });
            expect(view.result.current.states.one).toMatchObject({ owned: false });
        } finally { view.unmount(); vi.useRealTimers(); }
    });

    it('仅在用户确认普通空闲命令行后允许启动固定测试', async () => {
        const start = vi.fn().mockResolvedValue(undefined);
        render(<TerminalHandoff experimental state={base} transportError="" connected onStart={start} onStop={vi.fn()} onFocus={vi.fn()}><textarea aria-label="终端输入" /></TerminalHandoff>);
        fireEvent.click(screen.getByRole('button', { name: /接手测试/ }));
        expect(screen.getByRole('button', { name: '开始测试' })).toBeDisabled();
        fireEvent.click(screen.getByRole('checkbox'));
        fireEvent.click(screen.getByRole('radio', { name: /体验停止/ }));
        fireEvent.click(screen.getByRole('button', { name: '开始测试' }));
        await waitFor(() => expect(start).toHaveBeenCalledWith('slow', true));
        expect(screen.queryByLabelText('开始接手测试')).not.toBeInTheDocument();
    });

    it('占用期间封锁终端并可手动接回，释放后移除 inert', async () => {
        const stop = vi.fn().mockResolvedValue(undefined);
        const running = { ...base, runId: 'r1', phase: 'running', owned: true, message: '正在执行' };
        const props = { transportError: '', connected: true, onStart: vi.fn(), onStop: stop, onFocus: vi.fn() };
        const view = render(<TerminalHandoff experimental {...props} state={running}><textarea aria-label="终端输入" /></TerminalHandoff>);
        expect(screen.getByLabelText('终端输入').parentElement).toHaveAttribute('inert');
        expect(screen.getByLabelText('接手期间终端由测试助手独占')).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: '手动接管' }));
        await waitFor(() => expect(stop).toHaveBeenCalledWith(running));
        view.rerender(<TerminalHandoff experimental {...props} state={{ ...running, phase: 'stopped', owned: false, message: '远端命令可能仍在运行' }}><textarea aria-label="终端输入" /></TerminalHandoff>);
        expect(screen.queryByLabelText('接手期间终端由测试助手独占')).not.toBeInTheDocument();
        expect(screen.getByLabelText('终端输入').parentElement).not.toHaveAttribute('inert');
        expect(screen.getByRole('status')).toHaveTextContent('远端命令可能仍在运行');
    });

    it('状态通信失败时保留已知占用，不能在界面假报已释放', async () => {
        let fail = false;
        const host = {
            states: vi.fn(async () => { if (fail) throw Error('offline'); return [{ ...base, runId: 'r1', phase: 'running', owned: true }]; }),
            start: vi.fn(), stop: vi.fn().mockResolvedValue(undefined),
        };
        function Probe() { const value = useTerminalHandoff(host); return <div>{value.states.one?.owned ? '仍占用' : '未占用'}{value.error}</div>; }
        vi.useFakeTimers();
        try {
            render(<Probe />);
            await act(async () => { await vi.advanceTimersByTimeAsync(1); });
            expect(screen.getByText('仍占用')).toBeInTheDocument();
            fail = true;
            await act(async () => { await vi.advanceTimersByTimeAsync(400); });
            expect(screen.getByText(/仍占用.*暂时无法确认/)).toBeInTheDocument();
        } finally { vi.useRealTimers(); }
    });

    it('停止成功后不被迟到的旧状态重新锁住', async () => {
        const running = { ...base, runId: 'r1', phase: 'running', owned: true };
        let resolveOld: (values: HandoffState[]) => void = () => undefined;
        const old = new Promise<HandoffState[]>(resolve => { resolveOld = resolve; });
        const host = { states: vi.fn().mockResolvedValueOnce([running]).mockReturnValueOnce(old).mockResolvedValue([]), start: vi.fn(), stop: vi.fn().mockResolvedValue(undefined) };
        function Probe() { const value = useTerminalHandoff(host); return <button onClick={() => void value.stop(value.states.one)}>{value.states.one?.owned ? '占用中' : '已释放'}</button>; }
        vi.useFakeTimers();
        try {
            const view = render(<Probe />);
            await act(async () => { await vi.advanceTimersByTimeAsync(1); });
            await act(async () => { await vi.advanceTimersByTimeAsync(360); });
            fireEvent.click(screen.getByRole('button', { name: '占用中' }));
            await act(async () => { await Promise.resolve(); });
            expect(screen.getByRole('button')).toHaveTextContent('已释放');
            await act(async () => { resolveOld([running]); await Promise.resolve(); });
            expect(screen.getByRole('button')).toHaveTextContent('已释放');
            view.unmount();
        } finally { vi.useRealTimers(); }
    });

    it('旧轮次的停止响应不能覆盖新轮次', async () => {
        const first = { ...base, runId: 'r1', phase: 'running', owned: true };
        const second = { ...first, runId: 'r2' };
        const stopped = deferred<void>();
        const host = { states: vi.fn().mockResolvedValueOnce([first]).mockResolvedValue([second]), start: vi.fn(), stop: vi.fn().mockReturnValueOnce(stopped.promise).mockResolvedValue(undefined) };
        vi.useFakeTimers();
        const view = renderHook(() => useTerminalHandoff(host));
        try {
            await act(async () => { await vi.advanceTimersByTimeAsync(1); });
            let pending!: Promise<void>;
            act(() => { pending = view.result.current.stop(first); });
            await act(async () => { await vi.advanceTimersByTimeAsync(400); });
            expect(view.result.current.states.one.runId).toBe('r2');
            await act(async () => { stopped.resolve(); await pending; });
            expect(view.result.current.states.one).toMatchObject({ runId: 'r2', owned: true });
        } finally { view.unmount(); vi.useRealTimers(); }
    });

    it('页面离开期间启动才返回，也必须交还该轮终端', async () => {
        const started = deferred<HandoffState>();
        const running = { ...base, runId: 'r1', phase: 'preparing', owned: true };
        const host = { states: vi.fn().mockResolvedValue([]), start: vi.fn().mockReturnValue(started.promise), stop: vi.fn().mockResolvedValue(undefined) };
        const view = renderHook(() => useTerminalHandoff(host));
        let pending!: Promise<void>;
        act(() => { pending = view.result.current.start('one', 'inspect', true); });
        fireEvent(window, new Event('pagehide'));
        await act(async () => { started.resolve(running); await pending; });
        expect(host.stop).toHaveBeenCalledWith('one', 'r1');
        expect(view.result.current.states.one?.owned).not.toBe(true);
        view.unmount();
    });

    it('取消后重新打开需要重新确认命令行空闲', () => {
        render(<TerminalHandoff experimental state={base} transportError="" connected onStart={vi.fn()} onStop={vi.fn()} onFocus={vi.fn()}><textarea /></TerminalHandoff>);
        fireEvent.click(screen.getByRole('button', { name: /接手测试/ }));
        fireEvent.click(screen.getByRole('checkbox'));
        fireEvent.click(screen.getByRole('button', { name: '取消' }));
        fireEvent.click(screen.getByRole('button', { name: /接手测试/ }));
        expect(screen.getByRole('checkbox')).not.toBeChecked();
        expect(screen.getByRole('button', { name: '开始测试' })).toBeDisabled();
    });

    it('启动响应比完成状态晚到时不能重新宣称占用', async () => {
        const started = deferred<HandoffState>();
        const running = { ...base, runId: 'r1', phase: 'preparing', owned: true };
        const completed = { ...running, phase: 'completed', owned: false };
        const host = { states: vi.fn().mockResolvedValueOnce([]).mockResolvedValue([completed]), start: vi.fn().mockReturnValue(started.promise), stop: vi.fn().mockResolvedValue(undefined) };
        vi.useFakeTimers();
        const view = renderHook(() => useTerminalHandoff(host));
        try {
            await act(async () => { await vi.advanceTimersByTimeAsync(1); });
            let pending!: Promise<void>;
            act(() => { pending = view.result.current.start('one', 'inspect', true); });
            await act(async () => { await vi.advanceTimersByTimeAsync(400); });
            await act(async () => { started.resolve(running); await pending; });
            expect(view.result.current.states.one).toMatchObject({ phase: 'completed', owned: false });
        } finally { view.unmount(); vi.useRealTimers(); }
    });

    it('更换宿主后迟到的启动只交还旧宿主，不污染新页面', async () => {
        const started = deferred<HandoffState>();
        const oldHost = { states: vi.fn().mockResolvedValue([]), start: vi.fn().mockReturnValue(started.promise), stop: vi.fn().mockResolvedValue(undefined) };
        const newHost = { states: vi.fn().mockResolvedValue([]), start: vi.fn(), stop: vi.fn() };
        const view = renderHook(({ host }) => useTerminalHandoff(host), { initialProps: { host: oldHost } });
        let pending!: Promise<void>;
        act(() => { pending = view.result.current.start('one', 'inspect', true); });
        view.rerender({ host: newHost });
        await act(async () => { started.resolve({ ...base, runId: 'r1', owned: true }); await pending; });
        expect(oldHost.stop).toHaveBeenCalledWith('one', 'r1');
        expect(newHost.stop).not.toHaveBeenCalled();
        expect(view.result.current.states).toEqual({});
        view.unmount();
    });
});
