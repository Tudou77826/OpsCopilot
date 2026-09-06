/**
 * 编辑连接业务流程用例（Issue #71 / #62 防护）。
 *
 * 用例走前端真实链路：EditSavedSessionModal → wailsSessionRuntime → window.go（mock），
 * 断言的是"用户操作后到达 Go 边界的载荷与结果"，而不是组件内部实现细节，
 * 保证重构组件时业务语义不被悄悄破坏。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import EditSavedSessionModal from './EditSavedSessionModal';

const UpdateSavedSession = vi.fn(async (_id: string, _config: unknown) => '');

beforeAll(() => {
    (window as any).go = { main: { App: { UpdateSavedSession } } };
});

beforeEach(() => {
    UpdateSavedSession.mockClear();
    UpdateSavedSession.mockImplementation(async () => '');
});

// 遮罩层无语义标签，从弹窗标题向上找 position:fixed + zIndex 1100 的容器。
function getOverlay(): HTMLElement {
    let el: HTMLElement | null = screen.getByText('编辑连接');
    while (el) {
        if (el.style?.position === 'fixed' && el.style?.zIndex === '1100') return el;
        el = el.parentElement;
    }
    throw new Error('未找到编辑弹窗遮罩层');
}

const baseConfig = {
    name: 'db-1',
    host: '10.0.0.1',
    port: 22,
    user: 'ops',
    password: 'pw-keep',
    rootPassword: 'rp-keep',
};

function renderModal(configOverrides: Record<string, unknown> = {}) {
    const onClose = vi.fn();
    const onSaved = vi.fn();
    render(
        <ToastProvider>
            <EditSavedSessionModal
                isOpen={true}
                sessionId="sess-1"
                initialConfig={{ ...baseConfig, ...configOverrides } as any}
                onClose={onClose}
                onSaved={onSaved}
            />
        </ToastProvider>
    );
    return { onClose, onSaved };
}

describe('编辑连接业务流程', () => {
    it('修改基本信息并保存：完整字段（含未触碰的 root 密码）以驼峰载荷到达 Go 边界', async () => {
        const { onClose, onSaved } = renderModal();

        fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: 'db-1-renamed' } });
        fireEvent.change(screen.getByLabelText('端口'), { target: { value: '2222' } });
        fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'ops2' } });

        fireEvent.click(screen.getByRole('button', { name: '保存' }));

        await waitFor(() => expect(UpdateSavedSession).toHaveBeenCalledTimes(1));
        const [id, payload] = UpdateSavedSession.mock.calls[0];
        expect(id).toBe('sess-1');
        expect(payload).toMatchObject({
            name: 'db-1-renamed',
            port: 2222,
            user: 'ops2',
            host: '10.0.0.1',
            // 表单预填了已存密码，用户没改就必须原样回传——
            // #71 的病灶正是 rootPassword 在边界处被丢弃、保存后清空。
            password: 'pw-keep',
            rootPassword: 'rp-keep',
        });
        expect(onSaved).toHaveBeenCalled();
        expect(onClose).toHaveBeenCalled();
    });

    it('后端报错（如主机重名）时弹窗保持打开、恢复可编辑，不丢编辑内容', async () => {
        UpdateSavedSession.mockImplementation(
            async () => 'Error: a session with the same host already exists'
        );
        const { onClose } = renderModal();

        fireEvent.click(screen.getByRole('button', { name: '保存' }));
        await waitFor(() => expect(UpdateSavedSession).toHaveBeenCalledTimes(1));

        expect(await screen.findByText(/same host/)).toBeInTheDocument();
        expect(onClose).not.toHaveBeenCalled();
        expect(screen.getByRole('button', { name: '保存' })).toBeEnabled();
    });

    it('主机为空时拦截保存：不发起写请求、不关闭弹窗', () => {
        const { onClose } = renderModal({ host: '   ' });

        fireEvent.click(screen.getByRole('button', { name: '保存' }));

        expect(screen.getByText('主机地址不能为空')).toBeInTheDocument();
        expect(UpdateSavedSession).not.toHaveBeenCalled();
        expect(onClose).not.toHaveBeenCalled();
    });

    it('从输入框拖选文本、越过弹窗边界松开：不关闭弹窗且内容保留（#62 误关防护）', () => {
        const { onClose } = renderModal();
        const overlay = getOverlay();
        const hostInput = screen.getByLabelText('主机地址');

        // 浏览器行为：mousedown 在输入框、mouseup 在遮罩时，click 派发到二者
        // 的公共祖先——恰好是遮罩。修好的弹窗必须忽略这种"路过式" click。
        fireEvent.mouseDown(hostInput);
        fireEvent.mouseUp(overlay);
        fireEvent.click(overlay);

        expect(onClose).not.toHaveBeenCalled();
        expect(screen.getByLabelText('主机地址')).toHaveValue('10.0.0.1');
    });

    it('按下与点击都发生在遮罩空白处：保留点击外部关闭的手势', () => {
        const { onClose } = renderModal();
        const overlay = getOverlay();

        fireEvent.mouseDown(overlay);
        fireEvent.click(overlay);

        expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('点击取消按钮：不发起保存、直接关闭', () => {
        const { onClose } = renderModal();

        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(UpdateSavedSession).not.toHaveBeenCalled();
        expect(onClose).toHaveBeenCalledTimes(1);
    });
});
