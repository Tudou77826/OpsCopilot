import { act, fireEvent, renderHook } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import { useTerminalReconnect } from './useTerminalReconnect';

function deferred<T>() {
    let resolve!: (value: T) => void;
    let reject!: (reason: unknown) => void;
    const promise = new Promise<T>((yes, no) => { resolve = yes; reject = no; });
    return { promise, resolve, reject };
}

describe('重连与页签生命周期', () => {
    it('重复点击只建立一次连接，其他页签可独立重连', async () => {
        const first = deferred<boolean>();
        const request = vi.fn(() => first.promise);
        const accept = vi.fn();
        const reject = vi.fn();
        const view = renderHook(useTerminalReconnect);
        let pending!: Promise<void>;
        act(() => { pending = view.result.current.run('one', request, accept, reject); });
        await act(async () => {
            await view.result.current.run('one', request, accept, reject);
            await view.result.current.run('two', async () => true, accept, reject);
        });
        expect(request).toHaveBeenCalledTimes(1);
        expect(accept).toHaveBeenCalledTimes(1);
        await act(async () => { first.resolve(true); await pending; });
        expect(accept).toHaveBeenCalledTimes(2);
        expect(reject).not.toHaveBeenCalled();
        view.unmount();
    });

    it.each(['success', 'failure'] as const)('关闭页签后忽略迟到的%s，不安装监听器或弹出错误', async kind => {
        const response = deferred<boolean>();
        const accept = vi.fn();
        const reject = vi.fn();
        const view = renderHook(useTerminalReconnect);
        let pending!: Promise<void>;
        act(() => { pending = view.result.current.run('one', () => response.promise, accept, reject); });
        act(() => view.result.current.cancel('one'));
        await act(async () => {
            if (kind === 'success') response.resolve(true); else response.reject(Error('old connection failed'));
            await pending;
        });
        expect(accept).not.toHaveBeenCalled();
        expect(reject).not.toHaveBeenCalled();
        view.unmount();
    });

    it('页面重载或卸载使未完成请求失效，新请求不被旧结果替换', async () => {
        const response = deferred<boolean>();
        const oldAccept = vi.fn();
        const accept = vi.fn();
        const reject = vi.fn();
        const view = renderHook(useTerminalReconnect);
        let pending!: Promise<void>;
        act(() => { pending = view.result.current.run('one', () => response.promise, oldAccept, reject); });
        fireEvent(window, new Event('pagehide'));
        fireEvent(window, new Event('pageshow'));
        await act(async () => { await view.result.current.run('one', async () => true, accept, reject); });
        view.unmount();
        await act(async () => { response.resolve(true); await pending; });
        expect(accept).toHaveBeenCalledWith(true);
        expect(oldAccept).not.toHaveBeenCalled();
        expect(reject).not.toHaveBeenCalled();
    });
});
