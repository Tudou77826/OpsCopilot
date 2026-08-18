import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import SharedSessionPanel from './SharedSessionPanel';

// Mock Toast（组件在 Provider 外渲染，避免依赖上下文）
const mockToastError = vi.fn();
const mockToastSuccess = vi.fn();
vi.mock('../Toast/Toast', () => ({
    useToast: () => ({ error: mockToastError, success: mockToastSuccess, info: vi.fn(), warning: vi.fn() }),
}));

// Mock Wails bindings
const mockGetSharedSessions = vi.fn();

function setSharedResponse(enabled: boolean, sessions: any[]) {
    mockGetSharedSessions.mockResolvedValue(JSON.stringify({ enabled, owner: 'me', sessions }));
}

function makeEntry(overrides: Partial<any> = {}) {
    return {
        entryKey: 'me|ssh|10.0.0.1|22|root',
        owner: 'me',
        name: 'web-01',
        protocol: 'ssh',
        host: '10.0.0.1',
        port: 22,
        user: 'root',
        lastLoginAt: '2026-08-18 10:00:00',
        own: true,
        hasSecrets: true,
        decryptable: true,
        ...overrides,
    };
}

describe('SharedSessionPanel', () => {
    beforeEach(() => {
        vi.clearAllMocks();
        localStorage.clear();
        window.go = { main: { App: { GetSharedSessions: mockGetSharedSessions } } } as any;
    });

    it('renders nothing when sharing disabled', async () => {
        setSharedResponse(false, []);
        const { container } = render(<SharedSessionPanel onConnect={vi.fn()} />);
        await waitFor(() => expect(mockGetSharedSessions).toHaveBeenCalled());
        expect(container.querySelector('div')).toBeNull();
    });

    it('renders entries with owner and endpoint info', async () => {
        setSharedResponse(true, [makeEntry({ owner: 'alice', own: false })]);
        render(<SharedSessionPanel onConnect={vi.fn()} />);

        expect(await screen.findByText('web-01')).toBeInTheDocument();
        expect(screen.getByText(/root@10\.0\.0\.1/)).toBeInTheDocument();
        expect(screen.getByText(/alice/)).toBeInTheDocument();
        expect(screen.getByText('团队共享')).toBeInTheDocument();
    });

    it('shows empty hint when enabled but no entries', async () => {
        setSharedResponse(true, []);
        render(<SharedSessionPanel onConnect={vi.fn()} />);
        expect(await screen.findByText('暂无共享会话')).toBeInTheDocument();
    });

    it('hands decrypted config to unified connect flow on double click', async () => {
        const mockConnect = vi.fn().mockResolvedValue({
            success: true,
            config: { name: 'web-01', host: '10.0.0.1', port: 22, user: 'root', password: 'decrypted' },
        });
        window.go = { main: { App: { GetSharedSessions: mockGetSharedSessions, ConnectSharedSession: mockConnect } } } as any;
        setSharedResponse(true, [makeEntry()]);
        const onConnect = vi.fn();

        render(<SharedSessionPanel onConnect={onConnect} />);
        fireEvent.doubleClick(await screen.findByText('web-01'));

        await waitFor(() => expect(mockConnect).toHaveBeenCalledWith('me|ssh|10.0.0.1|22|root'));
        await waitFor(() => expect(onConnect).toHaveBeenCalledWith(
            expect.objectContaining({ host: '10.0.0.1', password: 'decrypted' })
        ));
    });

    it('shows toast and does not connect on failure', async () => {
        const mockConnect = vi.fn().mockResolvedValue({ success: false, message: '解密失败：共享密钥可能不正确' });
        window.go = { main: { App: { GetSharedSessions: mockGetSharedSessions, ConnectSharedSession: mockConnect } } } as any;
        setSharedResponse(true, [makeEntry()]);
        const onConnect = vi.fn();

        render(<SharedSessionPanel onConnect={onConnect} />);
        fireEvent.doubleClick(await screen.findByText('web-01'));

        await waitFor(() => expect(mockToastError).toHaveBeenCalledWith(expect.stringContaining('共享密钥可能不正确')));
        expect(onConnect).not.toHaveBeenCalled();
    });

    it('offers delete only for own entries in context menu', async () => {
        const mockRemove = vi.fn().mockResolvedValue('');
        window.go = { main: { App: { GetSharedSessions: mockGetSharedSessions, RemoveSharedSession: mockRemove } } } as any;
        setSharedResponse(true, [makeEntry({ own: false, owner: 'alice' })]);

        render(<SharedSessionPanel onConnect={vi.fn()} />);
        fireEvent.contextMenu(await screen.findByText('web-01'));

        expect(await screen.findByText('打开连接')).toBeInTheDocument();
        expect(screen.getByText('保存到我的会话')).toBeInTheDocument();
        // 他人条目不提供删除入口
        expect(screen.queryByText('删除共享')).not.toBeInTheDocument();
    });

    it('filters entries by search term from session manager', async () => {
        setSharedResponse(true, [
            makeEntry({ entryKey: 'me|ssh|10.0.0.1|22|root', name: 'web-01', host: '10.0.0.1' }),
            makeEntry({ entryKey: 'me|ssh|10.0.0.2|22|root', name: 'db-01', host: '10.0.0.2', owner: 'bob' }),
        ]);

        const { rerender } = render(<SharedSessionPanel onConnect={vi.fn()} searchTerm="" />);
        await screen.findByText('web-01');
        expect(screen.getByText('db-01')).toBeInTheDocument();

        // 按名称过滤
        rerender(<SharedSessionPanel onConnect={vi.fn()} searchTerm="db" />);
        expect(screen.queryByText('web-01')).not.toBeInTheDocument();
        expect(screen.getByText('db-01')).toBeInTheDocument();

        // 按共享者过滤
        rerender(<SharedSessionPanel onConnect={vi.fn()} searchTerm="bob" />);
        expect(screen.getByText('db-01')).toBeInTheDocument();
        expect(screen.queryByText('web-01')).not.toBeInTheDocument();

        // 无匹配时的空态提示
        rerender(<SharedSessionPanel onConnect={vi.fn()} searchTerm="nonexistent" />);
        expect(await screen.findByText(/无匹配的共享会话/)).toBeInTheDocument();
    });

    it('collapses via chevron, hides list, and persists preference', async () => {
        localStorage.removeItem('opscopilot:sharedPanelCollapsed');
        setSharedResponse(true, [makeEntry()]);
        render(<SharedSessionPanel onConnect={vi.fn()} />);

        await screen.findByText('web-01');
        const collapseBtn = screen.getByTitle('折叠团队共享');
        fireEvent.click(collapseBtn);

        // 列表隐藏，标题保留
        expect(screen.queryByText('web-01')).not.toBeInTheDocument();
        expect(screen.getByText('团队共享')).toBeInTheDocument();
        expect(localStorage.getItem('opscopilot:sharedPanelCollapsed')).toBe('1');

        // 再次点击展开
        fireEvent.click(screen.getByTitle('展开团队共享'));
        expect(await screen.findByText('web-01')).toBeInTheDocument();
        expect(localStorage.getItem('opscopilot:sharedPanelCollapsed')).toBe('0');
    });

    it('resizes panel height by dragging the top handle and persists it', async () => {
        localStorage.removeItem('opscopilot:sharedPanelHeight');
        setSharedResponse(true, [makeEntry()]);
        const { container } = render(<SharedSessionPanel onConnect={vi.fn()} />);
        await screen.findByText('web-01');

        // jsdom 的 getBoundingClientRect 全 0，mock 父容器几何信息
        const panel = container.querySelector('[data-testid="shared-panel"]') as HTMLElement;
        vi.spyOn(panel.parentElement as Element, 'getBoundingClientRect').mockReturnValue({
            bottom: 500, height: 600, top: -100, left: 0, right: 300, width: 300, x: 0, y: -100, toJSON: () => ({}),
        } as DOMRect);

        // 上边缘 5px 拖拽热区
        const handles = container.querySelectorAll('div[style*="ns-resize"]');
        expect(handles.length).toBe(1);
        fireEvent.mouseDown(handles[0]);
        expect(document.body.style.cursor).toBe('ns-resize');

        // clientY=300 → 新高度 500-300=200
        fireEvent.mouseMove(document, { clientY: 300 });
        expect(panel.style.height).toBe('200px');

        // 超出上边界 → 钳制在最大值 600-140=460
        fireEvent.mouseMove(document, { clientY: -999 });
        expect(panel.style.height).toBe('460px');

        fireEvent.mouseUp(document);
        expect(document.body.style.cursor).toBe('default');
        expect(localStorage.getItem('opscopilot:sharedPanelHeight')).toBe('460');
    });
});
