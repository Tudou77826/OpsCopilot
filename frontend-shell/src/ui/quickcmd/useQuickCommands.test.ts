import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { useQuickCommands } from './useQuickCommands';
import { MemoryAdapter } from './useQuickCommands';

describe('useQuickCommands', () => {
    it('loads commands from adapter', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'List Files', content: 'ls -la', group: 'default' },
            { id: '2', name: 'Check Disk', content: 'df -h', group: 'system' },
        ];

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        // Wait for async load
        await vi.waitFor(() => {
            expect(result.current.loaded).toBe(true);
        });

        expect(result.current.commands).toHaveLength(2);
        expect(result.current.availableGroups).toEqual(['default', 'system']);
    });

    it('computes filteredCommands by selectedGroup', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'cmd1', content: 'echo 1', group: 'a' },
            { id: '2', name: 'cmd2', content: 'echo 2', group: 'b' },
            { id: '3', name: 'cmd3', content: 'echo 3', group: 'a' },
        ];

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        act(() => {
            result.current.setSelectedGroup('a');
        });
        expect(result.current.filteredCommands).toHaveLength(2);

        act(() => {
            result.current.setSelectedGroup('b');
        });
        expect(result.current.filteredCommands).toHaveLength(1);
    });

    it('adds a command', async () => {
        const adapter = new MemoryAdapter();
        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        act(() => {
            result.current.addCommand('New Cmd', 'echo hello', 'default');
        });

        expect(result.current.commands).toHaveLength(1);
        expect(result.current.commands[0].name).toBe('New Cmd');
    });

    it('updates a command', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'Old', content: 'old', group: 'default' },
        ];

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        act(() => {
            result.current.updateCommand('1', { name: 'New' });
        });

        expect(result.current.commands[0].name).toBe('New');
    });

    it('deletes a command', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'cmd1', content: 'echo 1', group: 'default' },
            { id: '2', name: 'cmd2', content: 'echo 2', group: 'default' },
        ];

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        act(() => {
            result.current.deleteCommand('1');
        });

        expect(result.current.commands).toHaveLength(1);
        expect(result.current.commands[0].id).toBe('2');
    });

    it('falls back to first group when selected group is removed', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'cmd1', content: 'echo 1', group: 'a' },
            { id: '2', name: 'cmd2', content: 'echo 2', group: 'b' },
        ];

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        result.current.setSelectedGroup('b');

        // Delete the only command in group b
        act(() => {
            result.current.deleteCommand('2');
        });

        // Should fall back to 'a'
        expect(result.current.selectedGroup).toBe('a');
    });

    it('persists add/update/delete through intent-based adapter calls', async () => {
        const adapter: MemoryAdapter = new MemoryAdapter();
        adapter.data = [{ id: '1', name: 'old', content: 'old', group: 'default' }];
        const addSpy = vi.spyOn(adapter, 'add');
        const updateSpy = vi.spyOn(adapter, 'update');
        const removeSpy = vi.spyOn(adapter, 'remove');

        const { result } = renderHook(() => useQuickCommands({ host: { storage: adapter, execute: () => {} } }));
        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        act(() => { result.current.addCommand('n', 'c', 'default'); });
        expect(addSpy).toHaveBeenCalledTimes(1);
        expect(addSpy.mock.calls[0][0]).toMatchObject({ name: 'n', content: 'c' });

        act(() => { result.current.updateCommand('1', { name: 'new' }); });
        expect(updateSpy).toHaveBeenCalledWith('1', { name: 'new' });

        act(() => { result.current.deleteCommand('1'); });
        expect(removeSpy).toHaveBeenCalledWith('1');
        expect(adapter.data).toHaveLength(1); // 仅剩新加的一条
    });

    it('applies external hot-reload updates without persisting back', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [{ id: '1', name: 'old', content: 'old', group: 'default' }];
        const addSpy = vi.spyOn(adapter, 'add');
        const updateSpy = vi.spyOn(adapter, 'update');
        const removeSpy = vi.spyOn(adapter, 'remove');

        let onUpdate: ((cmds: any[]) => void) | undefined;
        const host = {
            storage: adapter,
            execute: () => {},
            onExternalChange: vi.fn((cb: (cmds: any[]) => void) => {
                onUpdate = cb;
                return () => {};
            }),
        };

        const { result } = renderHook(() => useQuickCommands({ host }));
        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        // 其他窗口写入后的推送：本地状态整体刷新
        act(() => {
            onUpdate?.([
                { id: '1', name: 'edited-elsewhere', content: 'x', group: 'default' },
                { id: '2', name: 'added-elsewhere', content: 'y', group: 'default' },
            ]);
        });

        expect(result.current.commands).toHaveLength(2);
        expect(result.current.commands[0].name).toBe('edited-elsewhere');
        // 热加载只更新内存，不得回写存储形成回环
        expect(addSpy).not.toHaveBeenCalled();
        expect(updateSpy).not.toHaveBeenCalled();
        expect(removeSpy).not.toHaveBeenCalled();
    });
});
