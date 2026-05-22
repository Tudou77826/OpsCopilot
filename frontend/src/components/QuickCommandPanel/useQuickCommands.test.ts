import { renderHook, act } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { useQuickCommands, MemoryAdapter } from './useQuickCommands';

describe('useQuickCommands', () => {
    it('loads commands from adapter', async () => {
        const adapter = new MemoryAdapter();
        adapter.data = [
            { id: '1', name: 'List Files', content: 'ls -la', group: 'default' },
            { id: '2', name: 'Check Disk', content: 'df -h', group: 'system' },
        ];

        const { result } = renderHook(() => useQuickCommands({ adapter }));

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

        const { result } = renderHook(() => useQuickCommands({ adapter }));

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
        const { result } = renderHook(() => useQuickCommands({ adapter }));

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

        const { result } = renderHook(() => useQuickCommands({ adapter }));

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

        const { result } = renderHook(() => useQuickCommands({ adapter }));

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

        const { result } = renderHook(() => useQuickCommands({ adapter }));

        await vi.waitFor(() => expect(result.current.loaded).toBe(true));

        result.current.setSelectedGroup('b');

        // Delete the only command in group b
        act(() => {
            result.current.deleteCommand('2');
        });

        // Should fall back to 'a'
        expect(result.current.selectedGroup).toBe('a');
    });
});
