/**
 * 会话管理拖拽业务流程用例（Issue #69 / #70 防护 + 多层嵌套与排序）。
 *
 * 走真实链路：SessionManager → wailsSessionRuntime → window.go（mock），
 * 模拟用户在会话树里的拖拽，断言到达 Go 边界的 moveNode 参数。
 *
 * 拖拽采用行高三分区：上 25% 同级插前、下 25% 同级插后、中间 50%（文件夹）移入。
 * 因此用例需要显式指定光标在目标行内的高度比例。
 */
import React from 'react';
import { render, screen, fireEvent, createEvent, waitFor, act } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import SessionManager from './SessionManager';

/**
 * 生产(folder)
 *   华东(folder)
 *     web-1(session)
 *   数据库(session)
 * 测试(folder)
 * root-1(session)
 */
const savedTree = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            {
                id: 'f-east',
                name: '华东',
                type: 'folder',
                children: [
                    { id: 's-web1', name: 'web-1', type: 'session', config: { host: '10.0.0.1', port: 22, user: 'root' } },
                ],
            },
            { id: 's-db', name: '数据库', type: 'session', config: { host: '10.0.0.2', port: 22, user: 'root' } },
        ],
    },
    { id: 'f-test', name: '测试', type: 'folder', children: [] },
    { id: 's-root', name: 'root-1', type: 'session', config: { host: '10.0.0.3', port: 22, user: 'root' } },
];

const GetConnectionTree = vi.fn(async () => savedTree);
const MoveTreeNode = vi.fn(async (_id: string, _parentId: string, _index: number) => undefined);

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                GetConnectionTree,
                MoveTreeNode,
                DeleteTreeNode: vi.fn(async () => undefined),
                RenameTreeNode: vi.fn(async () => undefined),
                CreateSavedFolder: vi.fn(async () => undefined),
                CreateSavedConnection: vi.fn(async () => undefined),
                UpdateSavedConnection: vi.fn(async () => undefined),
                DuplicateSavedConnection: vi.fn(async () => undefined),
                ReorderTreeChildren: vi.fn(async () => undefined),
                // 团队共享会话未启用，面板自行隐藏
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
    MoveTreeNode.mockClear();
    MoveTreeNode.mockImplementation(async () => undefined);
});

function row(id: string): HTMLElement {
    return screen.getByTestId(`tree-row-${id}`);
}

function treeContainer(): HTMLElement {
    return screen.getByTestId('session-tree');
}

function makeDataTransfer(nodeId: string) {
    return {
        setData: vi.fn(),
        getData: vi.fn(() => nodeId),
        effectAllowed: 'move',
        dropEffect: 'move',
    };
}

/** jsdom 无布局：把目标行高度固定为 40，用 clientY 表达落点比例。 */
const ROW_HEIGHT = 40;
function mockRowRect(el: HTMLElement, top = 0) {
    vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
        top,
        bottom: top + ROW_HEIGHT,
        height: ROW_HEIGHT,
        width: 200,
        left: 0,
        right: 200,
        x: 0,
        y: top,
        toJSON: () => ({}),
    } as DOMRect);
}

/** ratio: 0=行顶，1=行底。文件夹行的 0.5 表示"移入"。 */
async function dragTo(sourceId: string, targetId: string, ratio: number) {
    const source = row(sourceId);
    const target = row(targetId);
    mockRowRect(target);
    const dt = makeDataTransfer(sourceId);
    fireEvent.dragStart(source, { dataTransfer: dt } as any);

    const overEvent = createEvent.dragOver(target, { dataTransfer: dt } as any);
    Object.defineProperty(overEvent, 'clientY', { value: ratio * ROW_HEIGHT });
    fireEvent(target, overEvent);

    await act(async () => {
        const dropEvent = createEvent.drop(target, { dataTransfer: dt } as any);
        Object.defineProperty(dropEvent, 'clientY', { value: ratio * ROW_HEIGHT });
        fireEvent(target, dropEvent);
        fireEvent.dragEnd(source, { dataTransfer: dt } as any);
    });
}

async function dragToBlank(sourceId: string) {
    const source = row(sourceId);
    const container = treeContainer();
    const dt = makeDataTransfer(sourceId);
    fireEvent.dragStart(source, { dataTransfer: dt } as any);
    fireEvent.dragOver(container, { dataTransfer: dt } as any);
    await act(async () => {
        fireEvent.drop(container, { dataTransfer: dt } as any);
        fireEvent.dragEnd(source, { dataTransfer: dt } as any);
    });
}

async function renderTree() {
    render(
        <ToastProvider>
            <SessionManager onConnect={vi.fn()} />
        </ToastProvider>
    );
    await waitFor(() => expect(screen.getByText('生产')).toBeInTheDocument());
    // 与真实用户一致：展开才能看到深层节点。
    fireEvent.click(row('f-prod'));
    await waitFor(() => expect(screen.getByText('华东')).toBeInTheDocument());
    fireEvent.click(row('f-east'));
    await waitFor(() => expect(screen.getByText('web-1')).toBeInTheDocument());
}

describe('会话拖拽业务流程', () => {
    it('会话拖到文件夹中间：移入该文件夹，且只提交一次（#70 核心回归）', async () => {
        await renderTree();

        await dragTo('s-web1', 'f-test', 0.5);

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        // 冒泡修复前：文件夹行提交后，容器兜底处理器还会再提交一次"移到根"，
        // 两次写入最终把会话弹回根目录。
        await new Promise((r) => setTimeout(r, 30));
        expect(MoveTreeNode).toHaveBeenCalledTimes(1);
        expect(MoveTreeNode.mock.calls[0]).toEqual(['s-web1', 'f-test', 0]);
    });

    it('会话拖到它自己所在文件夹：原地不动，不发起移动', async () => {
        await renderTree();

        await dragTo('s-web1', 'f-east', 0.5);

        await new Promise((r) => setTimeout(r, 30));
        expect(MoveTreeNode).not.toHaveBeenCalled();
    });

    it('根目录会话拖进嵌套文件夹：正常入组', async () => {
        await renderTree();

        await dragTo('s-root', 'f-east', 0.5);

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        expect(MoveTreeNode.mock.calls[0]).toEqual(['s-root', 'f-east', 1]);
    });

    it('会话拖到树空白处：移到根目录末尾（保留"移出分组"手势）', async () => {
        await renderTree();

        await dragToBlank('s-web1');

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        const [id, parentId] = MoveTreeNode.mock.calls[0];
        expect(id).toBe('s-web1');
        expect(parentId).toBe('');
    });
});

describe('同级排序', () => {
    it('拖到目标行上半部：插到它之前', async () => {
        await renderTree();

        // 根层 [生产, 测试, root-1]，把"测试"拖到"生产"之前 -> index 0。
        await dragTo('f-test', 'f-prod', 0.1);

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        expect(MoveTreeNode.mock.calls[0]).toEqual(['f-test', '', 0]);
    });

    it('拖到目标行下半部：插到它之后（同层向后移动需扣掉自身偏移）', async () => {
        await renderTree();

        // 根层 [生产, 测试, root-1]，把"生产"拖到"测试"之后。
        // 摘除"生产"后目标层为 [测试, root-1]，插入位置是 1。
        await dragTo('f-prod', 'f-test', 0.9);

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        expect(MoveTreeNode.mock.calls[0]).toEqual(['f-prod', '', 1]);
    });

    it('文件夹内会话拖到兄弟行上方：在同层内插到它之前', async () => {
        await renderTree();

        // 生产层 [华东, 数据库]，把"数据库"拖到"华东"之前 -> index 0。
        await dragTo('s-db', 'f-east', 0.1);

        await waitFor(() => expect(MoveTreeNode).toHaveBeenCalledTimes(1));
        expect(MoveTreeNode.mock.calls[0]).toEqual(['s-db', 'f-prod', 0]);
    });
});

describe('环检测', () => {
    it('文件夹拖进自己的子文件夹：不发起请求（不允许形成环）', async () => {
        await renderTree();

        await dragTo('f-prod', 'f-east', 0.5);

        await new Promise((r) => setTimeout(r, 30));
        expect(MoveTreeNode).not.toHaveBeenCalled();
    });

    it('文件夹拖到自己的子行上（同级插入）：同样不发起请求', async () => {
        await renderTree();

        await dragTo('f-prod', 'f-east', 0.1);

        await new Promise((r) => setTimeout(r, 30));
        expect(MoveTreeNode).not.toHaveBeenCalled();
    });
});

describe('拖拽边缘自动滚动（#69）', () => {
    function installFrameDriver() {
        const pending = new Map<number, FrameRequestCallback>();
        let seed = 0;
        const rafSpy = vi.spyOn(window, 'requestAnimationFrame').mockImplementation(((cb: FrameRequestCallback) => {
            seed += 1;
            pending.set(seed, cb);
            return seed;
        }) as typeof window.requestAnimationFrame);
        const cafSpy = vi.spyOn(window, 'cancelAnimationFrame').mockImplementation(((id: number) => {
            pending.delete(id);
        }) as typeof window.cancelAnimationFrame);
        const runFrames = async (n: number) => {
            await act(async () => {
                for (let i = 0; i < n; i++) {
                    const cbs = [...pending.values()];
                    pending.clear();
                    cbs.forEach((cb) => cb(performance.now()));
                }
                await Promise.resolve();
            });
        };
        return {
            runFrames,
            restore: () => {
                rafSpy.mockRestore();
                cafSpy.mockRestore();
            },
        };
    }

    function mockContainerRect(el: HTMLElement, top: number, bottom: number) {
        vi.spyOn(el, 'getBoundingClientRect').mockReturnValue({
            top,
            bottom,
            height: bottom - top,
            width: 100,
            left: 0,
            right: 100,
            x: 0,
            y: top,
            toJSON: () => ({}),
        } as DOMRect);
    }

    // jsdom 的 DragEvent 不支持 clientY 初始化（真实浏览器无此问题）。
    function dragOverContainerAt(el: HTMLElement, clientY: number, dt: unknown) {
        const event = createEvent.dragOver(el, { dataTransfer: dt } as any);
        Object.defineProperty(event, 'clientY', { value: clientY });
        fireEvent(el, event);
    }

    it('光标停在容器底部边缘带时持续向下滚动，dragEnd 后停止', async () => {
        const driver = installFrameDriver();
        try {
            await renderTree();
            const container = treeContainer();
            mockContainerRect(container, 0, 200);

            const source = row('s-web1');
            const dt = makeDataTransfer('s-web1');
            fireEvent.dragStart(source, { dataTransfer: dt } as any);
            dragOverContainerAt(container, 185, dt);

            await driver.runFrames(2);
            expect(container.scrollTop).toBeGreaterThan(0);
            const scrolled = container.scrollTop;

            fireEvent.dragEnd(source, { dataTransfer: dt } as any);
            await driver.runFrames(2);
            expect(container.scrollTop).toBe(scrolled);
        } finally {
            driver.restore();
        }
    });

    it('光标在容器中部时不滚动', async () => {
        const driver = installFrameDriver();
        try {
            await renderTree();
            const container = treeContainer();
            mockContainerRect(container, 0, 200);

            const source = row('s-web1');
            const dt = makeDataTransfer('s-web1');
            fireEvent.dragStart(source, { dataTransfer: dt } as any);
            dragOverContainerAt(container, 100, dt);

            await driver.runFrames(2);
            expect(container.scrollTop).toBe(0);
        } finally {
            driver.restore();
        }
    });
});
