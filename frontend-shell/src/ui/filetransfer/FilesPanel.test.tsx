import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import FilesPanel, { getFileTransferLayoutMode, getStableFileTransferLayoutMode } from './FilesPanel';

const json = (value: unknown) => Promise.resolve(JSON.stringify(value));

// 构造完整宿主（对齐 Wails 全量能力）。事件订阅在 host 上捕获，
// 测试经 emitProgress / emitDone 模拟后端推送 —— 组件不再接触 window.runtime。
const makeBackend = () => {
    const progressHandlers = new Set<(p: any) => void>();
    const doneHandlers = new Set<(d: any) => void>();
    const backend = {
        FTCheck: vi.fn(() => json({ ok: true, message: 'sftp(login)' })),
        FTList: vi.fn((_sessionId: string, remotePath: string) => json({
            ok: true,
            entries: [
                { path: `${remotePath}/remote.log`, name: 'remote.log', isDir: false, size: 2048, modTime: '2026-06-08T00:00:00Z', mode: 0, owner: 'root', group: 'root' },
                { path: `${remotePath}/tmpdir`, name: 'tmpdir', isDir: true, size: 0, modTime: '2026-06-08T00:00:00Z', mode: 0, owner: 'root', group: 'root' },
            ],
        })),
        FTStat: vi.fn(() => json({ ok: true })),
        FTUpload: vi.fn(() => json({ ok: true, taskId: 'upload-1' })),
        FTDownload: vi.fn(() => json({ ok: true, taskId: 'download-1' })),
        FTCancel: vi.fn(() => json({ ok: true })),
        FTRemoteMkdir: vi.fn(() => json({ ok: true })),
        FTRemoteRemove: vi.fn(() => json({ ok: true })),
        FTRemoteRename: vi.fn(() => json({ ok: true })),
        FTRemoteReadFile: vi.fn(() => json({ ok: true, result: { bytes: 0 }, message: '' })),
        FTRemoteWriteFile: vi.fn(() => json({ ok: true })),
        LocalList: vi.fn(() => json({
            ok: true,
            entries: [
                { path: 'C:\\Users\\tester\\local.txt', name: 'local.txt', isDir: false, size: 1536, modTime: '2026-06-08T00:00:00Z', mode: 0 },
                { path: 'C:\\Users\\tester\\workspace', name: 'workspace', isDir: true, size: 0, modTime: '2026-06-08T00:00:00Z', mode: 0 },
            ],
        })),
        LocalMkdir: vi.fn(() => json({ ok: true })),
        LocalRemove: vi.fn(() => json({ ok: true })),
        LocalRename: vi.fn(() => json({ ok: true })),
        LocalCopy: vi.fn(() => json({ ok: true })),
        LocalStat: vi.fn(() => json({ ok: false })),
        SelectSavePath: vi.fn(() => json('C:\\Users\\tester\\local-1.txt')),
        onProgress: vi.fn((cb: (p: any) => void) => {
            progressHandlers.add(cb);
            return () => progressHandlers.delete(cb);
        }),
        onDone: vi.fn((cb: (d: any) => void) => {
            doneHandlers.add(cb);
            return () => doneHandlers.delete(cb);
        }),
        // 默认无 OS 拖放能力（Sidecar 形态）；需要时由具体测试覆写。
        onFileDrop: undefined as ((handler: (x: number, y: number, paths: string[]) => void) => () => void) | undefined,
        emitProgress: (data: any) => act(() => { progressHandlers.forEach(cb => cb(data)); }),
        emitDone: (data: any) => act(() => { doneHandlers.forEach(cb => cb(data)); }),
    };
    return backend;
};

const renderPanel = (width: number, backend = makeBackend()) => {
    Object.defineProperty(window, 'innerWidth', { value: width, configurable: true });
    render(
        <FilesPanel
            activeTerminalId="session-1"
            terminals={[{ id: 'session-1', title: 'prod-01' }]}
            host={backend}
        />,
    );
    return backend;
};

const getPaneContainingText = (text: string) => {
    const node = screen.getByText(text);
    const pane = node.closest('[data-testid^="file-pane-"]');
    if (!pane) throw new Error(`No file pane found for ${text}`);
    return pane as HTMLElement;
};

describe('browser transfer capabilities in the original FilesPanel', () => {
    it('does not add browser controls to the desktop host', async () => {
        renderPanel(1200);
        await screen.findByText('local.txt');
        expect(screen.queryByRole('button', { name: '导入本机文件' })).toBeNull();
        expect(screen.queryByRole('button', { name: '下载到本机' })).toBeNull();
    });
    it('imports through the host port and refreshes the original file list', async () => {
        const backend = { ...makeBackend(), ImportFile: vi.fn(async (_dir: string, _file: File, _signal: AbortSignal, progress: (value: number) => void) => { progress(100); }) };
        renderPanel(1200, backend);
        await screen.findByText('local.txt');
        const before = backend.LocalList.mock.calls.length;
        const file = new File(['test'], 'test.txt', { type: 'text/plain' });
        fireEvent.change(screen.getByLabelText('导入本机文件'), { target: { files: [file] } });
        await waitFor(() => expect(backend.ImportFile).toHaveBeenCalledWith(expect.any(String), file, expect.any(AbortSignal), expect.any(Function)));
        await screen.findByText('已导入 test.txt');
        expect(backend.LocalList.mock.calls.length).toBeGreaterThan(before);
    });
    it('downloads the selected local file through the host port', async () => {
        const backend = { ...makeBackend(), ExportFile: vi.fn(async (_path: string) => {}) };
        renderPanel(1200, backend);
        fireEvent.click(await screen.findByText('local.txt'));
        fireEvent.click(screen.getByRole('button', { name: '下载到本机' }));
        await waitFor(() => expect(backend.ExportFile).toHaveBeenCalledWith('C:\\Users\\tester\\local.txt'));
    });
});

describe('FilesPanel responsive layout', () => {
    afterEach(() => {
        vi.restoreAllMocks();
    });

    it('maps container widths to stable layout modes', () => {
        expect(getFileTransferLayoutMode(1200)).toBe('wide');
        expect(getFileTransferLayoutMode(760)).toBe('medium');
        expect(getFileTransferLayoutMode(520)).toBe('narrow');
    });

    it('keeps the current layout mode inside breakpoint hysteresis bands', () => {
        expect(getStableFileTransferLayoutMode(652, 'narrow')).toBe('narrow');
        expect(getStableFileTransferLayoutMode(656, 'narrow')).toBe('medium');
        expect(getStableFileTransferLayoutMode(632, 'medium')).toBe('medium');
        expect(getStableFileTransferLayoutMode(623, 'medium')).toBe('narrow');

        expect(getStableFileTransferLayoutMode(908, 'wide')).toBe('wide');
        expect(getStableFileTransferLayoutMode(903, 'wide')).toBe('medium');
        expect(getStableFileTransferLayoutMode(928, 'medium')).toBe('medium');
        expect(getStableFileTransferLayoutMode(936, 'medium')).toBe('wide');
    });

    it('renders both file panes in wide mode with full table columns', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'wide'));
        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());
        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());

        expect(screen.getByTestId('file-pane-本地')).toBeInTheDocument();
        expect(screen.getByTestId('file-pane-远端')).toBeInTheDocument();
        expect(screen.getAllByText('所属').length).toBeGreaterThanOrEqual(2);
        expect(screen.getByText('1.5 KB')).toBeInTheDocument();
        expect(screen.getAllByText('root:root').length).toBeGreaterThanOrEqual(2);
    });

    it('uses icons instead of raw DIR and FILE markers', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());

        expect(screen.queryByText('DIR')).not.toBeInTheDocument();
        expect(screen.queryByText('FILE')).not.toBeInTheDocument();
        expect(screen.getAllByTestId('file-kind-file').length).toBeGreaterThanOrEqual(2);
        expect(screen.getAllByTestId('file-kind-directory').length).toBeGreaterThanOrEqual(2);
    });

    it('does not mark every row selected when backend entries have empty paths', async () => {
        const backend = makeBackend();
        backend.FTList = vi.fn((_sessionId: string, _remotePath: string) => json({
            ok: true,
            entries: [
                { path: '', name: '.bash_history', isDir: false, size: 2048, modTime: '2026-06-08T00:00:00Z', mode: 0 },
                { path: '', name: '.bashrc', isDir: false, size: 176, modTime: '2026-06-08T00:00:00Z', mode: 0 },
            ],
        }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'wide'));

        // 隐藏文件默认不显示（.bash_history 在远端 pane），打开远端"显示隐藏"后再断言
        const remotePaneEl = screen.getByTestId('file-pane-远端');
        fireEvent.click(within(remotePaneEl).getByText('显示隐藏'));

        await waitFor(() => expect(screen.getByText('.bash_history')).toBeInTheDocument());

        const row = screen.getByText('.bash_history').closest('tr') as HTMLTableRowElement;
        expect(row.getAttribute('style') || '').not.toContain('rgb(33, 50, 68)');
        expect(row.getAttribute('style') || '').not.toContain('box-shadow');
    });

    it('uses single-pane segmented navigation in narrow mode', async () => {
        renderPanel(520);

        await waitFor(() => expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'narrow'));
        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());

        expect(screen.getByTestId('file-pane-本地')).toBeInTheDocument();
        expect(screen.queryByTestId('file-pane-远端')).not.toBeInTheDocument();
        expect(screen.queryByText('所属')).not.toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: '远端' }));

        await waitFor(() => expect(screen.getByTestId('file-pane-远端')).toBeInTheDocument());
        expect(screen.queryByTestId('file-pane-本地')).not.toBeInTheDocument();
        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());
        expect(screen.getByText(/root:root · 2.0 KB/)).toBeInTheDocument();
    });

    it('keeps column resize behavior available outside narrow mode', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());
        const localPane = screen.getByTestId('file-pane-本地');
        const cols = localPane.querySelectorAll('col');
        // 第一列是复选框列(32px)，第二列才是名称列（内容自适应，短文件名收敛到最小值160px）
        expect(cols[0].getAttribute('style')).toContain('32px');
        expect(cols[1].getAttribute('style')).toContain('160px');

        const handle = screen.getAllByTestId('column-resize-name')[0];
        fireEvent.mouseDown(handle, { clientX: 100 });
        fireEvent.mouseMove(window, { clientX: 145 });
        fireEvent.mouseUp(window);

        // 名称列(第二列)宽度应随拖拽变化（自适应起始 160px + 45px）
        const colsAfter = localPane.querySelectorAll('col');
        expect(colsAfter[1].getAttribute('style')).toContain('205px');
    });

    it('shows a clear remote upload overlay when dragging a local file', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());
        const remotePane = screen.getByTestId('file-pane-远端');

        fireEvent.dragOver(remotePane, {
            dataTransfer: {
                types: ['application/x-opscopilot-local-file'],
                dropEffect: 'copy',
            },
        });

        const overlay = screen.getByTestId('file-drop-overlay');
        expect(overlay).toBeInTheDocument();
        expect(screen.getByText('上传到远端目录')).toBeInTheDocument();
        expect(within(overlay).getByText('/root')).toBeInTheDocument();
    });

    it('explains why drag upload is blocked when transfer is unsupported', async () => {
        const backend = makeBackend();
        backend.FTCheck = vi.fn(() => json({ ok: true, message: 'unsupported' }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('unsupported')).toBeInTheDocument());
        const remotePane = screen.getByTestId('file-pane-远端');

        fireEvent.dragOver(remotePane, {
            dataTransfer: {
                types: ['application/x-opscopilot-local-file'],
                dropEffect: 'copy',
            },
        });

        const overlay = screen.getByTestId('file-drop-overlay');
        expect(within(overlay).getByText('当前连接不支持文件上传')).toBeInTheDocument();
        expect(within(overlay).getByText('unsupported')).toBeInTheDocument();
    });

    it('uploads an internal local-list drag when the drop payload is lost', async () => {
        const backend = makeBackend();
        backend.FTStat = vi.fn(() => json({ ok: false }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());
        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());

        const localRow = screen.getByText('local.txt').closest('tr') as HTMLTableRowElement;
        const remotePane = getPaneContainingText('remote.log');
        const dragData = {
            types: ['application/x-opscopilot-local-file'],
            dropEffect: 'copy',
            effectAllowed: 'copy',
            setData: vi.fn(),
            getData: vi.fn(() => ''),
        };

        fireEvent.dragStart(localRow, { dataTransfer: dragData });
        fireEvent.drop(remotePane, {
            dataTransfer: {
                types: ['application/x-opscopilot-local-file'],
                dropEffect: 'copy',
                getData: vi.fn(() => ''),
            },
        });

        await waitFor(() => expect(backend.FTUpload).toHaveBeenCalledWith('session-1', 'C:\\Users\\tester\\local.txt', '/root/local.txt'));
    });

    it('prevents browser defaults for internal file drags outside managed panes', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByText('local.txt')).toBeInTheDocument());
        const localRow = screen.getByText('local.txt').closest('tr') as HTMLTableRowElement;
        fireEvent.dragStart(localRow, {
            dataTransfer: {
                types: ['application/x-opscopilot-local-file'],
                dropEffect: 'copy',
                effectAllowed: 'copy',
                setData: vi.fn(),
            },
        });

        const dropEvent = new Event('drop', { bubbles: true, cancelable: true });
        Object.defineProperty(dropEvent, 'dataTransfer', {
            value: { types: ['application/x-opscopilot-local-file'] },
        });
        document.dispatchEvent(dropEvent);

        expect(dropEvent.defaultPrevented).toBe(true);
    });

    it('uses host file-drop paths for external files dropped on the remote pane', async () => {
        const backend = makeBackend();
        backend.FTStat = vi.fn(() => json({ ok: false }));
        let onFileDrop: ((x: number, y: number, paths: string[]) => void) | undefined;
        backend.onFileDrop = vi.fn((callback: typeof onFileDrop) => {
            onFileDrop = callback;
            return () => {};
        });

        renderPanel(1200, backend);
        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());
        const remotePane = getPaneContainingText('remote.log');
        remotePane.getBoundingClientRect = vi.fn(() => ({
            left: 10,
            top: 20,
            right: 510,
            bottom: 420,
            width: 500,
            height: 400,
            x: 10,
            y: 20,
            toJSON: () => ({}),
        }));

        await act(async () => {
            onFileDrop?.(100, 100, ['C:\\Users\\tester\\Desktop\\outside.log']);
        });

        await waitFor(() => expect(backend.FTUpload).toHaveBeenCalledWith('session-1', 'C:\\Users\\tester\\Desktop\\outside.log', '/root/outside.log'));
    });

    it('uses border-box width so padding changes do not oscillate near breakpoints', async () => {
        let resizeCallback: ResizeObserverCallback | undefined;
        class MockResizeObserver {
            constructor(callback: ResizeObserverCallback) {
                resizeCallback = callback;
            }
            observe = vi.fn();
            disconnect = vi.fn();
        }

        vi.stubGlobal('ResizeObserver', MockResizeObserver);
        renderPanel(700);

        await waitFor(() => expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'medium'));

        const target = screen.getByTestId('files-panel');
        const makeEntry = (contentWidth: number): ResizeObserverEntry => ({
            target,
            contentRect: { width: contentWidth, height: 400, x: 0, y: 0, top: 0, right: contentWidth, bottom: 400, left: 0, toJSON: () => ({}) },
            borderBoxSize: [{ inlineSize: 657, blockSize: 424 }],
            contentBoxSize: [{ inlineSize: contentWidth, blockSize: 400 }],
            devicePixelContentBoxSize: [{ inlineSize: contentWidth, blockSize: 400 }],
        } as ResizeObserverEntry);

        act(() => {
            resizeCallback?.([makeEntry(633)], {} as ResizeObserver);
        });
        expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'medium');

        act(() => {
            resizeCallback?.([makeEntry(641)], {} as ResizeObserver);
        });
        expect(screen.getByTestId('files-panel')).toHaveAttribute('data-layout-mode', 'medium');
    });

    it('supports ctrl multi-select and batch upload via context menu', async () => {
        const backend = makeBackend();
        backend.LocalList = vi.fn(() => json({
            ok: true,
            entries: [
                { path: 'C:\\a.txt', name: 'a.txt', isDir: false, size: 100, modTime: '2026-06-08T00:00:00Z', mode: 0 },
                { path: 'C:\\b.txt', name: 'b.txt', isDir: false, size: 200, modTime: '2026-06-08T00:00:00Z', mode: 0 },
            ],
        }));
        backend.FTStat = vi.fn(() => json({ ok: false }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('a.txt')).toBeInTheDocument());
        await waitFor(() => expect(screen.getByText('b.txt')).toBeInTheDocument());

        const rowA = screen.getByText('a.txt').closest('tr') as HTMLTableRowElement;
        const rowB = screen.getByText('b.txt').closest('tr') as HTMLTableRowElement;
        fireEvent.click(rowA);
        fireEvent.click(rowB, { ctrlKey: true });

        // 右键任一所选行 → 菜单出现"上传所选 2 项"
        fireEvent.contextMenu(rowB, { clientX: 100, clientY: 100 });
        const menu = await screen.findByTestId('file-context-menu');
        fireEvent.click(within(menu).getByText('上传所选 2 项'));

        await waitFor(() => expect(backend.FTUpload).toHaveBeenCalledTimes(2));
        expect(backend.FTUpload).toHaveBeenCalledWith('session-1', 'C:\\a.txt', '/root/a.txt');
        expect(backend.FTUpload).toHaveBeenCalledWith('session-1', 'C:\\b.txt', '/root/b.txt');
    });

    it('asks for local overwrite confirmation when the destination file exists', async () => {
        const backend = makeBackend();
        backend.LocalStat = vi.fn(() => json({ ok: true }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());
        const remoteRow = screen.getByText('remote.log').closest('tr') as HTMLTableRowElement;
        fireEvent.doubleClick(remoteRow);

        await waitFor(() => expect(backend.FTDownload).not.toHaveBeenCalled());
    });

    it('shows a right-click context menu on remote entries', async () => {
        renderPanel(1200);

        await waitFor(() => expect(screen.getByText('remote.log')).toBeInTheDocument());
        const remoteRow = screen.getByText('remote.log').closest('tr') as HTMLTableRowElement;

        fireEvent.contextMenu(remoteRow, { clientX: 300, clientY: 200 });

        const menu = await screen.findByTestId('file-context-menu');
        expect(menu).toBeInTheDocument();
        expect(within(menu).getByText('下载')).toBeInTheDocument();
        expect(within(menu).getByText('删除')).toBeInTheDocument();
        expect(within(menu).getByText('复制路径')).toBeInTheDocument();
    });

    it('enables batch buttons via per-row checkboxes and header select-all', async () => {
        const backend = makeBackend();
        backend.LocalList = vi.fn(() => json({
            ok: true,
            entries: [
                { path: 'C:\\a.txt', name: 'a.txt', isDir: false, size: 100, modTime: '2026-06-08T00:00:00Z', mode: 0 },
                { path: 'C:\\b.txt', name: 'b.txt', isDir: false, size: 200, modTime: '2026-06-08T00:00:00Z', mode: 0 },
            ],
        }));
        backend.FTStat = vi.fn(() => json({ ok: false }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('a.txt')).toBeInTheDocument());

        // 勾选第一行的复选框（点击 cell，不触发行单选）
        const localPane = screen.getByTestId('file-pane-本地');
        const check0 = localPane.querySelector('tbody input[type="checkbox"]') as HTMLInputElement;
        fireEvent.click(check0);

        // 勾选后右键菜单应出现批量"上传"项
        const rowA = screen.getByText('a.txt').closest('tr') as HTMLTableRowElement;
        fireEvent.contextMenu(rowA, { clientX: 100, clientY: 100 });
        const menu = await screen.findByTestId('file-context-menu');
        expect(within(menu).getByText('上传')).toBeInTheDocument();

        // 表头全选：应勾选所有可见行
        const checkAll = localPane.querySelector('thead input[type="checkbox"]') as HTMLInputElement;
        fireEvent.click(checkAll);
        const tbodyChecks = localPane.querySelectorAll('tbody input[type="checkbox"]');
        expect((tbodyChecks[0] as HTMLInputElement).checked).toBe(true);
        expect((tbodyChecks[1] as HTMLInputElement).checked).toBe(true);
    });

    it('sizes the name column from content, ignoring the longest 20% outliers', async () => {
        const backend = makeBackend();
        // 10 个条目：1 个超长 + 9 个普通；超长项应被前 20% 剔除，宽度取剩余最大值
        const entries = [
            { path: 'C:\\z'.padEnd(96, 'z') + '.txt', name: 'z'.repeat(90) + '.txt', isDir: false, size: 10, modTime: '2026-06-08T00:00:00Z', mode: 0 },
        ];
        for (let i = 1; i <= 9; i++) {
            entries.push({ path: `C:\\file-1234-${i}.txt`, name: `file-1234-${i}.txt`, isDir: false, size: 10, modTime: '2026-06-08T00:00:00Z', mode: 0 });
        }
        backend.LocalList = vi.fn(() => json({ ok: true, entries }));
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('file-1234-1.txt')).toBeInTheDocument());
        const localPane = screen.getByTestId('file-pane-本地');
        const cols = localPane.querySelectorAll('col');
        // 名称列(第二列)宽度来自 9 个普通文件名最大值(≈190px)，而非超长项(≈677px)
        expect(cols[1].getAttribute('style')).toContain('190px');
        expect(cols[1].getAttribute('style')).not.toContain('677px');
    });

    it('copies all selected paths as a newline list when multiple are selected', async () => {
        const backend = makeBackend();
        backend.LocalList = vi.fn(() => json({
            ok: true,
            entries: [
                { path: 'C:\\a.txt', name: 'a.txt', isDir: false, size: 100, modTime: '2026-06-08T00:00:00Z', mode: 0 },
                { path: 'C:\\b.txt', name: 'b.txt', isDir: false, size: 200, modTime: '2026-06-08T00:00:00Z', mode: 0 },
            ],
        }));
        const writeText = vi.fn();
        Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true });
        renderPanel(1200, backend);

        await waitFor(() => expect(screen.getByText('a.txt')).toBeInTheDocument());
        await waitFor(() => expect(screen.getByText('b.txt')).toBeInTheDocument());

        const rowA = screen.getByText('a.txt').closest('tr') as HTMLTableRowElement;
        const rowB = screen.getByText('b.txt').closest('tr') as HTMLTableRowElement;
        fireEvent.click(rowA);
        fireEvent.click(rowB, { ctrlKey: true });

        // 右键选中行 → 菜单出现"复制路径 (2)"
        fireEvent.contextMenu(rowB, { clientX: 100, clientY: 100 });
        const menu = await screen.findByTestId('file-context-menu');
        const copyItem = within(menu).getByText('复制路径 (2)');
        fireEvent.click(copyItem);

        expect(writeText).toHaveBeenCalledWith('C:\\a.txt\nC:\\b.txt');
    });

    // 宿主事件经 host 捕获；emit 保持与旧 Wails 事件名相同的调用形状，
    // 便于对照历史断言（file-transfer-progress / file-transfer-done）。
    const stubRuntimeEvents = (backend: ReturnType<typeof makeBackend>) => ({
        emit: (name: string, data: any) => {
            if (name === 'file-transfer-progress') backend.emitProgress(data);
            else backend.emitDone(data);
        },
    });

    const downloadRemoteLog = async () => {
        // 文件名可能同时出现在远端列表与传输队列任务行，取表格行（有 tr 祖先）的那个
        await waitFor(() => {
            const rows = screen.getAllByText('remote.log')
                .map(n => n.closest('tr'))
                .filter((n): n is HTMLTableRowElement => !!n);
            expect(rows.length).toBeGreaterThan(0);
            fireEvent.doubleClick(rows[0]);
        });
        await waitFor(() => expect(screen.getByTestId('queue-summary')).toBeInTheDocument());
    };

    it('aggregates queue summary across task states', async () => {
        const backend = makeBackend();
        let seq = 0;
        backend.FTDownload = vi.fn(() => json({ ok: true, taskId: `download-${++seq}` }));
        const events = stubRuntimeEvents(backend);
        renderPanel(1200, backend);

        await downloadRemoteLog();
        await downloadRemoteLog();

        events.emit('file-transfer-progress', { taskId: 'download-2', sessionId: 'session-1', step: '排队等待其他传输完成...' });
        events.emit('file-transfer-done', { taskId: 'download-1', sessionId: 'session-1', ok: true });

        const summary = screen.getByTestId('queue-summary');
        expect(within(summary).getByText('共 2')).toBeInTheDocument();
        expect(within(summary).getByText('✓ 1')).toBeInTheDocument();
        expect(within(summary).getByText('排队 1')).toBeInTheDocument();
        // 任务行显示文件名而不是 taskId 前缀
        expect(screen.getAllByText('remote.log').length).toBeGreaterThanOrEqual(1);
    });

    it('keeps the queue hidden after the user closes it, even on new done events', async () => {
        const backend = makeBackend();
        const events = stubRuntimeEvents(backend);
        renderPanel(1200, backend);

        await downloadRemoteLog();
        fireEvent.click(screen.getByText('收起'));
        expect(screen.queryByText('传输队列')).not.toBeInTheDocument();

        // 隐藏后仍有任务完成：队列不得被强制弹出
        events.emit('file-transfer-done', { taskId: 'download-1', sessionId: 'session-1', ok: true });
        expect(screen.queryByText('传输队列')).not.toBeInTheDocument();
        // 顶栏按钮不再带失败徽标（该任务成功）
        expect(screen.getByText('显示队列')).toBeInTheDocument();
    });

    it('surfaces a failed badge on the show-queue button when hidden tasks fail', async () => {
        const backend = makeBackend();
        const events = stubRuntimeEvents(backend);
        renderPanel(1200, backend);

        await downloadRemoteLog();
        fireEvent.click(screen.getByText('收起'));

        events.emit('file-transfer-done', { taskId: 'download-1', sessionId: 'session-1', ok: false, message: '网络异常' });
        expect(screen.queryByText('传输队列')).not.toBeInTheDocument();
        expect(screen.getByText('显示队列 (1 失败)')).toBeInTheDocument();

        // 用户主动打开后，事件恢复自动弹出且徽标消失
        fireEvent.click(screen.getByText('显示队列 (1 失败)'));
        expect(screen.getByText('传输队列')).toBeInTheDocument();
    });

    it('clears the queued hint once byte progress arrives', async () => {
        const backend = makeBackend();
        const events = stubRuntimeEvents(backend);
        renderPanel(1200, backend);

        await downloadRemoteLog();
        events.emit('file-transfer-progress', { taskId: 'download-1', sessionId: 'session-1', step: '排队等待其他传输完成...' });
        expect(screen.getByText('排队等待其他传输完成...')).toBeInTheDocument();

        // 拿到槽位后的字节进度必须清掉排队提示（后端会显式下发空 step）
        events.emit('file-transfer-progress', { taskId: 'download-1', sessionId: 'session-1', step: '', bytesDone: 1024, bytesTotal: 4096, speedBps: 2048 });
        expect(screen.queryByText('排队等待其他传输完成...')).not.toBeInTheDocument();
        expect(screen.getByText('传输中')).toBeInTheDocument();
        expect(screen.getByText('1.0 KB / 4.0 KB · 2.0 KB/s')).toBeInTheDocument();

        const summary = screen.getByTestId('queue-summary');
        expect(within(summary).queryByText(/^排队/)).not.toBeInTheDocument();
    });

    it('clears finished tasks from the queue but keeps failed ones', async () => {
        const backend = makeBackend();
        const events = stubRuntimeEvents(backend);
        renderPanel(1200, backend);

        await downloadRemoteLog();
        await downloadRemoteLog();
        events.emit('file-transfer-done', { taskId: 'download-1', sessionId: 'session-1', ok: true });
        events.emit('file-transfer-done', { taskId: 'download-2', sessionId: 'session-1', ok: false, message: '失败' });

        fireEvent.click(screen.getByText('清空已完成'));
        const summary = screen.getByTestId('queue-summary');
        await waitFor(() => expect(within(summary).getByText('共 1')).toBeInTheDocument());
        expect(within(summary).getByText('✗ 1')).toBeInTheDocument();
    });
});
