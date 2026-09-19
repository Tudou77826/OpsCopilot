import React from 'react';
import { fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import CommandGrid from './CommandGrid';
import type { QuickCommand } from '../ports';

const commands: QuickCommand[] = [
    { id: 'c1', name: '重启服务', content: 'systemctl restart app', group: '默认' },
    { id: 'c2', name: '跟踪日志', content: 'tail -f /var/log/syslog', group: '默认' },
];

const renderGrid = (onBroadcast?: (content: string) => void) => render(
    <CommandGrid
        commands={commands}
        onExecute={vi.fn()}
        onBroadcast={onBroadcast}
        onEdit={vi.fn()}
        onDelete={vi.fn()}
        onAdd={vi.fn()}
        searchQuery=""
        onSearchChange={vi.fn()}
        onReorder={vi.fn()}
    />,
);

describe('CommandGrid context menu broadcast (#76)', () => {
    it('shows 发送广播 first in the card context menu when broadcast is available', () => {
        renderGrid(vi.fn());
        fireEvent.contextMenu(screen.getByTestId('command-card-c1'));
        const menu = screen.getByTestId('command-context-menu');
        const items = Array.from(menu.querySelectorAll('div')).map(d => d.textContent);
        expect(items).toContain('发送广播');
        expect(items).toContain('编辑');
        expect(items).toContain('删除');
        expect(items.indexOf('发送广播')).toBeLessThan(items.indexOf('编辑'));
    });

    it('invokes onBroadcast with the command content and closes the menu', () => {
        const onBroadcast = vi.fn();
        renderGrid(onBroadcast);
        fireEvent.contextMenu(screen.getByTestId('command-card-c2'));
        fireEvent.click(screen.getByText('发送广播'));
        expect(onBroadcast).toHaveBeenCalledWith('tail -f /var/log/syslog');
        expect(screen.queryByTestId('command-context-menu')).toBeNull();
    });

    it('hides 发送广播 when the host provides no broadcast capability', () => {
        renderGrid(undefined);
        fireEvent.contextMenu(screen.getByTestId('command-card-c1'));
        expect(screen.queryByText('发送广播')).toBeNull();
        expect(screen.getByText('编辑')).toBeTruthy();
        expect(screen.getByText('删除')).toBeTruthy();
    });
});
