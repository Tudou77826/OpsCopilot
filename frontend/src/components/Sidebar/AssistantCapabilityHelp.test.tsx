import { act, cleanup, fireEvent, render, screen } from '@testing-library/react';
import { afterEach, expect, it, vi } from 'vitest';
import { AssistantCapabilityHelp } from './AssistantCapabilityHelp';

afterEach(() => {
    cleanup();
    vi.useRealTimers();
});

it('shows an unavailable state if the older host binding throws synchronously', async () => {
    render(<AssistantCapabilityHelp load={() => { throw new TypeError('binding unavailable'); }} />);
    fireEvent.mouseEnter(screen.getByRole('button', { name: 'Agent 可用能力' }));

    expect(await screen.findByText('暂时无法读取能力列表')).toBeVisible();
});

it('times out and reloads the capability list when hovered again', async () => {
    vi.useFakeTimers();
    const load = vi.fn<() => Promise<string[]>>()
        .mockImplementationOnce(() => new Promise(() => {}))
        .mockResolvedValueOnce(['workspace_create']);
    render(<AssistantCapabilityHelp load={load} />);
    const trigger = screen.getByRole('button', { name: 'Agent 可用能力' });
    fireEvent.mouseEnter(trigger);
    expect(screen.getByText('正在读取能力列表…')).toBeVisible();

    await act(async () => { await vi.advanceTimersByTimeAsync(5000); });
    expect(screen.getByText('暂时无法读取能力列表')).toBeVisible();

    fireEvent.mouseLeave(trigger);
    await act(async () => { fireEvent.mouseEnter(trigger); });
    expect(screen.getByText('新建 SSH 配置')).toBeVisible();
});
