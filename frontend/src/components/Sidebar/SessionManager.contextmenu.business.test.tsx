/**
 * 会话右键菜单业务流程用例（Issue #68 防护 + 新增菜单能力）。
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
                config: { host: '10.0.0.1', port: 22, user: 'root', password: 'pw', rootPassword: 'rp' },
            },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
    { id: 's-root', name: 'root-1', type: 'session', config: { host: '10.0.0.3', port: 22, user: 'root' } },
];

const copiedTree = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            baseTree[0].children![0],
            {
                id: 's-copy-1',
                name: 'web-1-副本',
                type: 'session',
                config: { host: '10.0.0.1', port: 22, user: 'root', password: 'pw', rootPassword: 'rp' },
            },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
    { id: 's-root', name: 'root-1', type: 'session', config: { host: '10.0.0.3', port: 22, user: 'root' } },
];

const GetConnectionTree = vi.fn(async () => baseTree);
const DuplicateSavedConnection = vi.fn(async (_id: string): Promise<void> => {});
const ReorderTreeChildren = vi.fn(async (_parentId: string, _ids: string[]) => undefined);

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                GetConnectionTree,
                DuplicateSavedConnection,
                ReorderTreeChildren,
                UpdateSavedConnection: vi.fn(async () => undefined),
                DeleteTreeNode: vi.fn(async () => undefined),
                RenameTreeNode: vi.fn(async () => undefined),
                MoveTreeNode: vi.fn(async () => undefined),
                CreateSavedFolder: vi.fn(async () => undefined),
                CreateSavedConnection: vi.fn(async () => undefined),
                GetSharedSessions: vi.fn(async () => JSON.stringify({ enabled: false })),
            },
        },
    };
});

beforeEach(() => {
    // 展开态持久化在 localStorage，必须逐用例清空，否则前一个用例展开的文件夹
    // 会让后一个用例的"点击展开"变成"点击折叠"。
    window.localStorage.clear();
    GetConnectionTree.mockClear();
    GetConnectionTree.mockImplementation(async () => baseTree);
    DuplicateSavedConnection.mockClear();
    DuplicateSavedConnection.mockImplementation(async () => undefined);
    ReorderTreeChildren.mockClear();
    ReorderTreeChildren.mockImplementation(async () => undefined);
});

function row(id: string): HTMLElement {
    return screen.getByTestId(`tree-row-${id}`);
}

async function renderTree() {
    render(
        <ToastProvider>
            <SessionManager onConnect={vi.fn()} />
        </ToastProvider>
    );
    await waitFor(() => expect(screen.getByText('生产')).toBeInTheDocument());
    fireEvent.click(row('f-prod'));
    await waitFor(() => expect(screen.getByText('web-1')).toBeInTheDocument());
}

describe('复制连接（#68）', () => {
    it('复制后出现完整配置的新条目，列表刷新显示副本', async () => {
        await renderTree();
        // 模拟后端：复制成功后 listTree 返回含副本的树。
        DuplicateSavedConnection.mockImplementation(async () => {
            GetConnectionTree.mockImplementation(async () => copiedTree);
        });

        fireEvent.contextMenu(row('s-web1'));
        fireEvent.click(screen.getByText('复制连接'));

        await waitFor(() => expect(DuplicateSavedConnection).toHaveBeenCalledWith('s-web1'));
        expect(await screen.findByText('web-1-副本')).toBeInTheDocument();
    });

    it('文件夹行上不提供复制连接入口', async () => {
        await renderTree();

        fireEvent.contextMenu(row('f-prod'));

        expect(screen.getByText('新建子文件夹')).toBeInTheDocument(); // 菜单已弹出
        expect(screen.queryByText('复制连接')).not.toBeInTheDocument();
    });

    it('复制失败时提示错误且列表保持原状', async () => {
        await renderTree();
        DuplicateSavedConnection.mockImplementation(async () => {
            throw new Error('节点不存在');
        });

        fireEvent.contextMenu(row('s-web1'));
        fireEvent.click(screen.getByText('复制连接'));

        expect(await screen.findByText(/节点不存在/)).toBeInTheDocument();
        expect(screen.queryByText('web-1-副本')).not.toBeInTheDocument();
    });
});

describe('菜单项按对象类型生成', () => {
    it('文件夹菜单：新建子文件夹 / 全部连接 / 按名称排序 / 删除（带子项数）', async () => {
        await renderTree();
        fireEvent.contextMenu(row('f-prod'));

        expect(screen.getByText('新建子文件夹')).toBeInTheDocument();
        expect(screen.getByText('全部连接（1）')).toBeInTheDocument();
        expect(screen.getByText('按名称排序')).toBeInTheDocument();
        // 删除文件夹前必须让用户看到会连带删除多少连接。
        expect(screen.getByText('删除（含 1 个连接）')).toBeInTheDocument();
    });

    it('空文件夹的删除项不带子项数', async () => {
        await renderTree();
        fireEvent.contextMenu(row('f-test'));

        expect(screen.getByText('删除')).toBeInTheDocument();
        expect(screen.queryByText(/含 \d+ 个连接/)).not.toBeInTheDocument();
        // 没有子节点时不显示"全部连接"。
        expect(screen.queryByText(/全部连接/)).not.toBeInTheDocument();
    });

    it('会话菜单：打开 / 编辑 / 复制 / 重命名 / 删除', async () => {
        await renderTree();
        fireEvent.contextMenu(row('s-web1'));

        expect(screen.getByText('打开连接')).toBeInTheDocument();
        expect(screen.getByText('编辑连接')).toBeInTheDocument();
        expect(screen.getByText('复制连接')).toBeInTheDocument();
        expect(screen.getByText('重命名')).toBeInTheDocument();
        expect(screen.getByText('删除')).toBeInTheDocument();
    });

    it('已在根目录的节点不显示"移到根目录"', async () => {
        await renderTree();
        fireEvent.contextMenu(row('s-root'));

        expect(screen.queryByText('移到根目录')).not.toBeInTheDocument();
    });

    it('文件夹内的节点提供"移到根目录"', async () => {
        await renderTree();
        fireEvent.contextMenu(row('s-web1'));

        expect(screen.getByText('移到根目录')).toBeInTheDocument();
    });

    it('空白处菜单：新建文件夹 / 新建连接 / 按名称排序', async () => {
        await renderTree();
        fireEvent.contextMenu(screen.getByTestId('session-tree'));

        expect(screen.getByText('新建文件夹')).toBeInTheDocument();
        expect(screen.getByText('新建连接')).toBeInTheDocument();
        expect(screen.getByText('按名称排序')).toBeInTheDocument();
        // Wails 宿主提供导入能力时，空白处菜单出现导入入口。
        expect(screen.getByText('导入 Xshell 会话…')).toBeInTheDocument();
    });
});

describe('新建连接（不建立会话）', () => {
    it('从空白处菜单打开属性弹窗，保存到根目录', async () => {
        await renderTree();
        fireEvent.contextMenu(screen.getByTestId('session-tree'));
        fireEvent.click(screen.getByText('新建连接'));

        expect(await screen.findByText('新建连接', { selector: 'h2' })).toBeInTheDocument();
        expect(screen.getByText('将保存到根目录')).toBeInTheDocument();
    });

    it('从文件夹菜单打开属性弹窗时提示落点', async () => {
        await renderTree();
        fireEvent.contextMenu(row('f-prod'));
        fireEvent.click(screen.getByText('新建连接'));

        expect(await screen.findByText('将保存到「生产」')).toBeInTheDocument();
    });
});

describe('按名称排序', () => {
    it('根层按中文拼音序重排并落库（测试 在 生产 之前）', async () => {
        await renderTree();
        fireEvent.contextMenu(screen.getByTestId('session-tree'));
        fireEvent.click(screen.getByText('按名称排序'));

        await waitFor(() => expect(ReorderTreeChildren).toHaveBeenCalledTimes(1));
        const [parentId, ids] = ReorderTreeChildren.mock.calls[0];
        expect(parentId).toBe('');
        // 文件夹在前（测(cè) < 生(shēng)），会话在后。
        expect(ids).toEqual(['f-test', 'f-prod', 's-root']);
    });
});
