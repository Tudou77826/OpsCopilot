import { act, renderHook, waitFor } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import type { AssistantConversationSnapshot, AssistantRuntime, AssistantTurnState } from '../../../../frontend-shell/src/assistant/runtime';
import { useAssistantQueue } from './useAssistantQueue';

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (error: Error) => void;
    const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
    return { promise, resolve, reject };
}
function fixture() {
    let latest: AssistantTurnState = { turnId: '', phase: 'running', target: { tabId: '', connectionId: '' }, message: '' };
    let serial = 0;
    const host: AssistantRuntime = {
        start: vi.fn(async (_question, target) => {
            latest = { ...latest, turnId: `turn-${++serial}`, target: target || { tabId: '', connectionId: '' }, phase: 'running', output: '' };
            return latest;
        }),
        state: vi.fn(async () => latest),
        stop: vi.fn(async () => { latest = { ...latest, phase: 'stopped', message: '已停止' }; }),
        respond: vi.fn(async () => undefined),
        newConversation: vi.fn(async () => undefined),
    };
    return { host, finish(phase = 'completed') { latest = { ...latest, phase, output: `结果 ${latest.turnId}` }; } };
}

describe('助手消息调度', () => {
    it('忙时新建明确失败，不让界面误以为上下文已清空', async () => {
        const f=fixture();
        const {result}=renderHook(()=>useAssistantQueue(f.host));
        act(()=>result.current.enqueue('正在执行的问题'));
        await act(async()=>{await expect(result.current.newConversation()).rejects.toThrow('请等待当前操作结束');});
        expect(f.host.newConversation).not.toHaveBeenCalled();
        expect(result.current.messages[0].content).toBe('正在执行的问题');
        await act(async()=>{await result.current.stop();});
    });
    it('恢复完成前禁止发送，恢复不建立新轮次或重放审批和待发消息', async () => {
        const f = fixture(), restore = deferred<AssistantConversationSnapshot>();
        f.host.restoreConversation = vi.fn(()=>restore.promise);
        const {result} = renderHook(()=>useAssistantQueue(f.host));
        expect(result.current.restoring).toBe(true);
        expect(()=>result.current.enqueue('不能抢先发送')).toThrow('请先完成对话恢复');
        await act(async()=>{restore.resolve({id:'conversation',messages:[{id:'one:user',role:'user',content:'已发送消息'}],truncated:false});});
        expect(result.current.messages.map(m=>m.content)).toEqual(['已发送消息']);
        expect(result.current.pending).toHaveLength(0);
        expect(result.current.restoring).toBe(false);
        expect(f.host.start).not.toHaveBeenCalled();
        expect(f.host.respond).not.toHaveBeenCalled();
    });
    it('离开页面后迟到的恢复快照不覆盖替代页面', async () => {
        const f=fixture(), old=deferred<AssistantConversationSnapshot>();
        f.host.restoreConversation=vi.fn(()=>old.promise);
        const first=renderHook(()=>useAssistantQueue(f.host));
        first.unmount();
        const next=fixture();
        next.host.restoreConversation=vi.fn(async()=>({id:'new',messages:[],truncated:false}));
        const {result}=renderHook(()=>useAssistantQueue(next.host));
        await waitFor(()=>expect(result.current.restoring).toBe(false));
        await act(async()=>{old.resolve({id:'old',messages:[{id:'old',role:'ai',content:'迟到内容'}],truncated:false});});
        expect(result.current.messages).toEqual([]);
    });
    it('按顺序执行，每条消息保留提交时的连接候选，不并行启动', async () => {
        const f = fixture();
        const { result } = renderHook(() => useAssistantQueue(f.host));
        const target = { tabId: 'two', connectionId: 'connection-two' };
        act(() => { result.current.enqueue('一'); result.current.enqueue('二', target, '第二终端'); result.current.enqueue('三'); });
        target.connectionId = 'replacement';
        expect(f.host.start).toHaveBeenCalledTimes(1);
        expect(result.current.pending.map(m => m.question)).toEqual(['二', '三']);
        f.finish();
        await waitFor(() => expect(f.host.start).toHaveBeenCalledTimes(2));
        expect(f.host.start).toHaveBeenLastCalledWith('二', { tabId: 'two', connectionId: 'connection-two' });
        f.finish();
        await waitFor(() => expect(f.host.start).toHaveBeenCalledTimes(3));
        f.finish();
        await waitFor(() => expect(result.current.current).toBeUndefined());
        expect(result.current.messages.filter(m => m.role === 'user').map(m => m.content)).toEqual(['一', '二', '三']);
    });

    it('启动尚未返回时也能立即发送，必须等旧轮次停止和收尾后才能继续', async () => {
        const f = fixture();
        const start = deferred<AssistantTurnState>();
        const cleanup = deferred<void>();
        const original = f.host.start;
        const originalStop = f.host.stop;
        f.host.start = vi.fn().mockImplementationOnce(() => start.promise).mockImplementation(original);
        f.host.stop = vi.fn(async id => { await cleanup.promise; await originalStop(id); });
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('旧要求'); result.current.enqueue('稍后'); result.current.enqueue('新要求'); });
        const id = result.current.pending[1].id;
        let interrupt!: Promise<void>;
        act(() => { interrupt = result.current.sendNow(id); });
        expect(f.host.stop).not.toHaveBeenCalled();
        await act(async () => { start.resolve({ turnId: 'late-turn', target: { tabId: '', connectionId: '' }, phase: 'stopped', message: '已停止' }); });
        expect(f.host.stop).toHaveBeenCalledWith('late-turn');
        expect(f.host.start).toHaveBeenCalledTimes(1);
        await act(async () => { cleanup.resolve(); await interrupt; });
        expect(f.host.start).toHaveBeenLastCalledWith('新要求', undefined);
        expect(f.host.newConversation).not.toHaveBeenCalled();
        expect(result.current.pending.map(m => m.question)).toEqual(['稍后']);
        await act(async () => { await result.current.stop(); });
    });

    it('手动停止后保留并暂停队列，移除消息和继续均由用户明确操作', async () => {
        const f = fixture();
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('一'); result.current.enqueue('二'); result.current.enqueue('三'); });
        await act(async () => { await result.current.stop(); });
        expect(result.current.paused).toBe(true);
        expect(f.host.start).toHaveBeenCalledTimes(1);
        act(() => { result.current.remove(result.current.pending[0].id); result.current.resume(); });
        expect(f.host.start).toHaveBeenLastCalledWith('三', undefined);
        await act(async () => { await result.current.stop(); });
    });

    it.each(['stopped', 'failed'])('宿主返回 %s 时暂停后续消息', async phase => {
        const f = fixture();
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('一'); result.current.enqueue('二'); });
        f.finish(phase);
        await waitFor(() => expect(result.current.current).toBeUndefined());
        expect(result.current.paused).toBe(true);
        expect(f.host.start).toHaveBeenCalledTimes(1);
        expect(result.current.pending).toHaveLength(1);
    });

    it('停止失败时不发送新要求，不丢失队列', async () => {
        const f = fixture();
        const cleanup = deferred<void>();
        f.host.stop = vi.fn(() => cleanup.promise);
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('一'); result.current.enqueue('二'); });
        let failure!: Promise<unknown>;
        act(() => { failure = result.current.sendNow(result.current.pending[0].id).catch(error => error); });
        await act(async () => { cleanup.reject(Error('仍在停止')); await failure; });
        expect(f.host.start).toHaveBeenCalledTimes(1);
        expect(result.current.pending).toHaveLength(1);
        expect(result.current.paused).toBe(true);
        f.finish('stopped');
        await waitFor(() => expect(result.current.current).toBeUndefined());
    });

    it('离开页面后撤销本轮，迟到启动不会执行排队消息', async () => {
        const f = fixture();
        const start = deferred<AssistantTurnState>();
        f.host.start = vi.fn(() => start.promise);
        const { result, unmount } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('一'); result.current.enqueue('二'); });
        unmount();
        start.resolve({ turnId: 'late', target: { tabId: '', connectionId: '' }, phase: 'running', message: '' });
        await waitFor(() => expect(f.host.stop).toHaveBeenCalledWith('late'));
        expect(f.host.start).toHaveBeenCalledTimes(1);
    });

    it('新建对话成功才清空已暂停队列，失败保留消息', async () => {
        const f = fixture();
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => { result.current.enqueue('一'); result.current.enqueue('二'); });
        await act(async () => { await result.current.stop(); });
        f.host.newConversation = vi.fn(async () => { throw Error('重置失败'); });
        await act(async () => { await result.current.newConversation().catch(() => undefined); });
        expect(result.current.pending).toHaveLength(1);
        expect(result.current.messages).not.toHaveLength(0);
        f.host.newConversation = vi.fn(async () => undefined);
        await act(async () => { await result.current.newConversation(); });
        expect(result.current.pending).toHaveLength(0);
        expect(result.current.messages).toHaveLength(0);
    });
    it('停止后没有积压消息时，新输入可直接开始，不必额外继续空队列', async () => {
        const f = fixture();
        const { result } = renderHook(() => useAssistantQueue(f.host));
        act(() => result.current.enqueue('一'));
        await act(async () => { await result.current.stop(); });
        act(() => result.current.enqueue('二'));
        expect(f.host.start).toHaveBeenLastCalledWith('二', undefined);
        await act(async () => { await result.current.stop(); });
    });
});
