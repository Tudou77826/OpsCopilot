/**
 * 会话管理拖拽业务流程用例（Issue #70 防护）。
 *
 * 走真实链路：SessionManager → wailsSessionRuntime → window.go（mock），
 * 模拟用户在会话树里的拖拽操作，断言到达 Go 边界的移动请求。
 * 核心回归约束：文件夹内会话无论拖到哪里，都绝不允许被"移出分组"覆盖。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import SessionManager from './SessionManager';

const savedTree = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            {
                id: 's-web1',
                name: 'web-1',
                type: 'session',
                config: { host: '10.0.0.1', port: 22, user: 'root', group: '生产' },
            },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
    {
        id: 's-db1',
        name: 'db-1',
        type: 'session',
        config: { host: '10.0.0.2', port: 22, user: 'root' },
    },
];

const nodeIdByName: Record<string, string> = { 'web-1': 's-web1', 'db-1': 's-db1' };

const GetSavedSessions = vi.fn(async () => savedTree);
const UpdateSavedSession = vi.fn(async (_id: string, _config: unknown) => '');

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                GetSavedSessions,
                UpdateSavedSession,
                DeleteSavedSession: vi.fn(async () => ''),
                RenameSavedSession: vi.fn(async () => ''),
                CreateSavedFolder: vi.fn(async () => ''),
                // 团队共享会话未启用，面板自行隐藏
                GetSharedSessions: vi.fn(async () => JSON.stringify({ enabled: false })),
            },
        },
    };
});

beforeEach(() => {
    GetSavedSessions.mockClear();
    UpdateSavedSession.mockClear();
    UpdateSavedSession.mockImplementation(async () => '');
});

// 会话行节点无语义标签：名称 span 的最近 div 祖先即树节点行。
function rowOf(name: string): HTMLElement {
    const row = screen.getByText(name).closest('div');
    if (!row) throw new Error(`未找到会话行: ${name}`);
    return row;
}

// 树容器：从任意行向上找第一个 overflowY:auto 的元素（拖到空白处的落点）。
function treeContainer(): HTMLElement {
    let el: HTMLElement | null = rowOf('web-1');
    while (el) {
        if (el.style?.overflowY === 'auto') return el;
        el = el.parentElement;
    }
    throw new Error('未找到会话树容器');
}

function makeDataTransfer(sessionId: string) {
    return {
        setData: vi.fn(),
        getData: vi.fn(() => sessionId),
        effectAllowed: 'move',
        dropEffect: 'move',
    };
}

async function dragDrop(sourceName: string, target: HTMLElement) {
    const row = rowOf(sourceName);
    const dt = makeDataTransfer(nodeIdByName[sourceName]);
    fireEvent.dragStart(row, { dataTransfer: dt } as any);
    fireEvent.dragOver(target, { dataTransfer: dt } as any);
    await act(async () => {
        fireEvent.drop(target, { dataTransfer: dt } as any);
        fireEvent.dragEnd(row, { dataTransfer: dt } as any);
    });
}

async function renderTree() {
    render(
        <ToastProvider>
            <SessionManager onConnect={vi.fn()} />
        </ToastProvider>
    );
    await waitFor(() => expect(screen.getByText('生产')).toBeInTheDocument());
    // 与真实用户一致：先展开"生产"文件夹才能看到其中的会话。
    fireEvent.click(rowOf('生产'));
    await waitFor(() => expect(screen.getByText('web-1')).toBeInTheDocument());
}

describe('会话拖拽业务流程', () => {
    it('文件夹内会话拖到另一个文件夹：恰好一次移动请求，绝不被移出分组（#70 核心回归）', async () => {
        await renderTree();

        await dragDrop('web-1', rowOf('测试'));

        await waitFor(() => expect(UpdateSavedSession).toHaveBeenCalledTimes(1));
        // 冒泡修复前：文件夹行提交移动后，树容器兜底处理器还会再提交一次
        // group='' 的"移出分组"，两次写入最终把会话弹回根目录。
        await new Promise((r) => setTimeout(r, 30));
        expect(UpdateSavedSession).toHaveBeenCalledTimes(1);
        const [id, payload] = UpdateSavedSession.mock.calls[0];
        expect(id).toBe('s-web1');
        expect(payload).toMatchObject({ host: '10.0.0.1', group: '测试' });
    });

    it('文件夹内会话拖到它自己所在的文件夹：原地不动，不发起移动', async () => {
        await renderTree();

        await dragDrop('web-1', rowOf('生产'));

        await new Promise((r) => setTimeout(r, 30));
        expect(UpdateSavedSession).not.toHaveBeenCalled();
    });

    it('根目录会话拖进文件夹：正常入组', async () => {
        await renderTree();

        await dragDrop('db-1', rowOf('测试'));

        await waitFor(() => expect(UpdateSavedSession).toHaveBeenCalledTimes(1));
        const [id, payload] = UpdateSavedSession.mock.calls[0];
        expect(id).toBe('s-db1');
        expect(payload).toMatchObject({ host: '10.0.0.2', group: '测试' });
    });

    it('文件夹内会话拖到树空白处：保留"移出分组"手势', async () => {
        await renderTree();

        await dragDrop('web-1', treeContainer());

        await waitFor(() => expect(UpdateSavedSession).toHaveBeenCalledTimes(1));
        const [id, payload] = UpdateSavedSession.mock.calls[0];
        expect(id).toBe('s-web1');
        expect(payload).toMatchObject({ group: '' });
    });

    it('文件夹内会话拖到另一个会话行上：不做任何事，不误触发移出分组', async () => {
        await renderTree();

        await dragDrop('web-1', rowOf('db-1'));

        await new Promise((r) => setTimeout(r, 30));
        expect(UpdateSavedSession).not.toHaveBeenCalled();
    });
});
