import { renderHook, act } from '@testing-library/react';
import { it, expect, vi } from 'vitest';
import { useGardenAttention } from './useGardenAttention';
import type { GardenHost } from '../ports';

it('does not poll while disabled and cancels polling when disabled again', async () => {
    vi.useFakeTimers();
    const signal = vi.fn().mockResolvedValue(null);
    const host = { signal } as unknown as GardenHost;
    const { rerender, unmount } = renderHook(({ enabled }) => useGardenAttention(enabled ? host : undefined, false), { initialProps: { enabled: false } });
    try {
        await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
        expect(signal).not.toHaveBeenCalled();
        rerender({ enabled: true });
        await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
        expect(signal).toHaveBeenCalled();
        rerender({ enabled: false });
        const calls = signal.mock.calls.length;
        await act(async () => { await vi.advanceTimersByTimeAsync(15000); });
        expect(signal).toHaveBeenCalledTimes(calls);
    } finally { unmount(); vi.useRealTimers(); }
});
