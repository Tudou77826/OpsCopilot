/**
 * 会话右键菜单"复制连接信息"业务流程用例（Issue #68 防护）。
 *
 * 语义：复制 = 整条克隆出新的连接条目（完整配置副本、落同一文件夹、随后可编辑），
 * 而不是把 user@host:port 文本写入剪贴板（该旧交互已删除）。
 * 走真实链路：SessionManager → wailsSessionRuntime → window.go（mock）。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import SessionManager from './SessionManager';

const baseTree = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            {
                id: 's-web1',
                name: 'web-1',
                type: 'session',
                config: { host: '10.0.0.1', port: 22, user: 'root', password: 'pw', rootPassword: 'rp', group: '生产' },
            },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
];

const copiedTree = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            baseTree[0].children[0],
            {
                id: 's-copy-1',
                name: 'web-1-副本',
                type: 'session',
                config: { host: '10.0.0.1', port: 22, user: 'root', password: 'pw', rootPassword: 'rp', group: '生产' },
            },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
];

const GetSavedSessions = vi.fn(async () => baseTree);
const DuplicateSavedSession = vi.fn(async (_id: string) => '');

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                GetSavedSessions,
                UpdateSavedSession: vi.fn(async () => ''),
                DeleteSavedSession: vi.fn(async () => ''),
                RenameSavedSession: vi.fn(async () => ''),
                CreateSavedFolder: vi.fn(async () => ''),
                DuplicateSavedSession,
                GetSharedSessions: vi.fn(async () => JSON.stringify({ enabled: false })),
            },
        },
    };
});

beforeEach(() => {
    GetSavedSessions.mockClear();
    GetSavedSessions.mockImplementation(async () => baseTree);
    DuplicateSavedSession.mockClear();
    DuplicateSavedSession.mockImplementation(async () => '');
});

function rowOf(name: string): HTMLElement {
    const row = screen.getByText(name).closest('div');
    if (!row) throw new Error(`未找到会话行: ${name}`);
    return row;
}

async function renderTree() {
    render(
        <ToastProvider>
            <SessionManager onConnect={vi.fn()} />
        </ToastProvider>
    );
    await waitFor(() => expect(screen.getByText('生产')).toBeInTheDocument());
    fireEvent.click(rowOf('生产'));
    await waitFor(() => expect(screen.getByText('web-1')).toBeInTheDocument());
}

describe('会话右键菜单复制连接信息（#68）', () => {
    it('复制后出现完整配置的新条目，列表刷新显示副本', async () => {
        await renderTree();
        // 模拟后端：复制成功后 listSessions 返回含副本的树。
        DuplicateSavedSession.mockImplementation(async () => {
            GetSavedSessions.mockImplementation(async () => copiedTree);
            return '';
        });

        fireEvent.contextMenu(rowOf('web-1'));
        fireEvent.click(screen.getByText('复制连接信息'));

        await waitFor(() => expect(DuplicateSavedSession).toHaveBeenCalledWith('s-web1'));
        // 副本作为新的连接条目出现在会话管理列表里。
        expect(await screen.findByText('web-1-副本')).toBeInTheDocument();
    });

    it('文件夹行上不提供复制连接信息入口', async () => {
        await renderTree();

        fireEvent.contextMenu(rowOf('生产'));

        expect(screen.getByText('新建文件夹')).toBeInTheDocument(); // 菜单已弹出
        expect(screen.queryByText('复制连接信息')).not.toBeInTheDocument();
    });

    it('复制失败时提示错误且列表保持原状', async () => {
        const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {});
        await renderTree();
        DuplicateSavedSession.mockImplementation(async () => 'Error: session not found');

        fireEvent.contextMenu(rowOf('web-1'));
        fireEvent.click(screen.getByText('复制连接信息'));

        await waitFor(() => expect(alertSpy).toHaveBeenCalledWith(expect.stringContaining('session not found')));
        expect(screen.queryByText('web-1-副本')).not.toBeInTheDocument();
        alertSpy.mockRestore();
    });
});
