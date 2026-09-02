import { afterEach, describe, expect, it, vi } from 'vitest';
import type { SidecarClient } from '../../core/sidecarClient';
import { makeSidecarConfigRuntime } from './SidecarConfigAdapter';

describe('makeSidecarConfigRuntime quick-command synchronization', () => {
  afterEach(() => {
    vi.useRealTimers();
  });

  it('pushes only changed command lists and stops after unsubscribe', async () => {
    vi.useFakeTimers();
    const first = [{ id: 'one', name: 'One', content: 'date', group: 'default' }];
    const second = [...first, { id: 'two', name: 'Two', content: 'uptime', group: 'ops' }];
    const quickcmdsList = vi.fn()
      .mockResolvedValueOnce({ commands: first })
      .mockResolvedValueOnce({ commands: first })
      .mockResolvedValue({ commands: second });
    const client = { quickcmdsList } as unknown as SidecarClient;
    const host = makeSidecarConfigRuntime(client).quickCommandHost(() => undefined);
    const handler = vi.fn();

    const unsubscribe = host.onExternalChange!(handler);
    await vi.advanceTimersByTimeAsync(0);
    expect(handler).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1000);
    expect(handler).not.toHaveBeenCalled();

    await vi.advanceTimersByTimeAsync(1000);
    expect(handler).toHaveBeenCalledOnce();
    expect(handler).toHaveBeenCalledWith(second);

    unsubscribe();
    await vi.advanceTimersByTimeAsync(2000);
    expect(quickcmdsList).toHaveBeenCalledTimes(3);
  });
});
