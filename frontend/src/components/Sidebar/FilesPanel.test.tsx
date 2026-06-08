import React from 'react';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import FilesPanel, { getFileTransferLayoutMode, getStableFileTransferLayoutMode } from './FilesPanel';

const json = (value: unknown) => Promise.resolve(JSON.stringify(value));

const makeBackend = () => ({
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
});

const renderPanel = (width: number, backend = makeBackend()) => {
    Object.defineProperty(window, 'innerWidth', { value: width, configurable: true });
    render(
        <FilesPanel
            activeTerminalId="session-1"
            terminals={[{ id: 'session-1', title: 'prod-01' }]}
            backend={backend}
        />,
    );
    return backend;
};

describe('FilesPanel responsive layout', () => {
    beforeEach(() => {
        vi.stubGlobal('runtime', undefined);
    });

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
        const firstCol = localPane.querySelector('col') as HTMLTableColElement;
        expect(firstCol.getAttribute('style')).toContain('280px');

        const handle = screen.getAllByTestId('column-resize-name')[0];
        fireEvent.mouseDown(handle, { clientX: 100 });
        fireEvent.mouseMove(window, { clientX: 145 });
        fireEvent.mouseUp(window);

        expect(firstCol.getAttribute('style')).toContain('325px');
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
});
