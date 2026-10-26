import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import DesktopRuntimeNotice from './DesktopRuntimeNotice';

const bridge = vi.hoisted(() => ({
    get: vi.fn(), retry: vi.fn(), listeners: new Set<(value: any) => void>(),
}));
vi.mock('../../wailsjs/go/main/App', () => ({
    GetDesktopRuntimeStatus: bridge.get,
    RetryDesktopRuntime: bridge.retry,
}));
vi.mock('../../wailsjs/runtime/runtime', () => ({
    EventsOn: (_name: string, listener: (value: any) => void) => {
        bridge.listeners.add(listener);
        return () => bridge.listeners.delete(listener);
    },
}));

describe('DesktopRuntimeNotice', () => {
    beforeEach(() => { bridge.get.mockReset(); bridge.retry.mockReset(); bridge.listeners.clear(); });
    afterEach(cleanup);

    it('recovers a failure received before the UI mounted and updates both views after retry', async () => {
        bridge.get.mockResolvedValue({ phase: 'error', message: '组件准备失败', detail: '载荷损坏' });
        bridge.retry.mockResolvedValue(undefined);
        render(<><DesktopRuntimeNotice /><DesktopRuntimeNotice compact /></>);
        expect(await screen.findAllByRole('alert')).toHaveLength(2);
        fireEvent.click(screen.getAllByRole('button', { name: '重试' })[0]);
        expect(bridge.retry).toHaveBeenCalledTimes(1);
        await act(async () => {
            bridge.listeners.forEach(listener => listener({ phase: 'ready', message: '数据库组件已就绪' }));
        });
        await waitFor(() => expect(screen.queryAllByRole('alert')).toHaveLength(0));
        expect(screen.getAllByText('数据库组件已就绪')).toHaveLength(1);
    });

    it('stays hidden in an unmanaged development build', async () => {
        bridge.get.mockResolvedValue({ phase: 'unmanaged', message: '' });
        const { container } = render(<DesktopRuntimeNotice />);
        await waitFor(() => expect(bridge.get).toHaveBeenCalled());
        expect(container).toBeEmptyDOMElement();
    });
});
