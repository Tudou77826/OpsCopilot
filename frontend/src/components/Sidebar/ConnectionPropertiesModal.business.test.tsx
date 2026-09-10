/**
 * 连接属性弹窗业务流程用例（Issue #71 / #62 防护 + 新建连接入口）。
 *
 * 用例走前端真实链路：ConnectionPropertiesModal → wailsSessionRuntime → window.go（mock），
 * 断言的是"用户操作后到达 Go 边界的载荷与结果"，而不是组件内部实现细节，
 * 保证重构组件时业务语义不被悄悄破坏。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import ConnectionPropertiesModal from './ConnectionPropertiesModal';

const UpdateSavedConnection = vi.fn(async (_id: string, _config: unknown) => undefined);
const CreateSavedConnection = vi.fn(async (_config: unknown, _parentId: string) => undefined);

beforeAll(() => {
    (window as any).go = { main: { App: { UpdateSavedConnection, CreateSavedConnection } } };
});

beforeEach(() => {
    UpdateSavedConnection.mockClear();
    CreateSavedConnection.mockClear();
    UpdateSavedConnection.mockImplementation(async () => undefined);
    CreateSavedConnection.mockImplementation(async () => undefined);
});

// 遮罩层无语义标签，从弹窗标题向上找 position:fixed + zIndex 1100 的容器。
function getOverlay(title: string): HTMLElement {
    let el: HTMLElement | null = screen.getByText(title);
    while (el) {
        if (el.style?.position === 'fixed' && el.style?.zIndex === '1100') return el;
        el = el.parentElement;
    }
    throw new Error('未找到弹窗遮罩层');
}

const baseConfig = {
    name: 'db-1',
    host: '10.0.0.1',
    port: 22,
    user: 'ops',
    password: 'pw-keep',
    rootPassword: 'rp-keep',
};

function renderEdit(configOverrides: Record<string, unknown> = {}) {
    const onClose = vi.fn();
    const onSaved = vi.fn();
    render(
        <ToastProvider>
            <ConnectionPropertiesModal
                isOpen={true}
                mode="edit"
                sessionId="sess-1"
                initialConfig={{ ...baseConfig, ...configOverrides } as any}
                onClose={onClose}
                onSaved={onSaved}
            />
        </ToastProvider>
    );
    return { onClose, onSaved };
}

function renderCreate(parentId = '', parentLabel?: string) {
    const onClose = vi.fn();
    const onSaved = vi.fn();
    render(
        <ToastProvider>
            <ConnectionPropertiesModal
                isOpen={true}
                mode="create"
                initialConfig={{ host: '10.0.0.9', port: 22, user: 'root' } as any}
                parentId={parentId}
                parentLabel={parentLabel}
                onClose={onClose}
                onSaved={onSaved}
            />
        </ToastProvider>
    );
    return { onClose, onSaved };
}

describe('编辑连接业务流程', () => {
    it('修改基本信息并保存：完整字段（含未触碰的 root 密码）以驼峰载荷到达 Go 边界', async () => {
        const { onClose, onSaved } = renderEdit();

        fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: 'db-1-renamed' } });
        fireEvent.change(screen.getByLabelText('端口'), { target: { value: '2222' } });
        fireEvent.change(screen.getByLabelText('用户名'), { target: { value: 'ops2' } });

        fireEvent.click(screen.getByRole('button', { name: '保存修改' }));

        await waitFor(() => expect(UpdateSavedConnection).toHaveBeenCalledTimes(1));
        const [id, payload] = UpdateSavedConnection.mock.calls[0] as [string, Record<string, unknown>];
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

    it('不再向前端回传分组字段：归属只由树里的位置决定', async () => {
        renderEdit({ group: '生产' });
        fireEvent.click(screen.getByRole('button', { name: '保存修改' }));

        await waitFor(() => expect(UpdateSavedConnection).toHaveBeenCalledTimes(1));
        const [, payload] = UpdateSavedConnection.mock.calls[0] as [string, Record<string, unknown>];
        expect(payload).not.toHaveProperty('group');
    });

    it('后端报错（如端点冲突）时弹窗保持打开、恢复可编辑，不丢编辑内容', async () => {
        UpdateSavedConnection.mockImplementation(async () => {
            throw new Error('已存在相同协议、主机和端口的连接');
        });
        const { onClose } = renderEdit();

        fireEvent.click(screen.getByRole('button', { name: '保存修改' }));
        await waitFor(() => expect(UpdateSavedConnection).toHaveBeenCalledTimes(1));

        expect(await screen.findByText(/已存在相同协议/)).toBeInTheDocument();
        expect(onClose).not.toHaveBeenCalled();
        expect(screen.getByRole('button', { name: '保存修改' })).toBeEnabled();
    });

    it('主机为空时拦截保存：不发起写请求、不关闭弹窗', () => {
        const { onClose } = renderEdit({ host: '   ' });

        fireEvent.click(screen.getByRole('button', { name: '保存修改' }));

        expect(screen.getByText('主机地址不能为空')).toBeInTheDocument();
        expect(UpdateSavedConnection).not.toHaveBeenCalled();
        expect(onClose).not.toHaveBeenCalled();
    });

    it('从输入框拖选文本、越过弹窗边界松开：不关闭弹窗且内容保留（#62 误关防护）', () => {
        const { onClose } = renderEdit();
        const overlay = getOverlay('编辑连接');
        const hostInput = screen.getByLabelText('主机地址');

        // 浏览器行为：mousedown 在输入框、mouseup 在遮罩时，click 派发到二者的
        // 公共祖先——恰好是遮罩。修好的弹窗必须忽略这种"路过式" click。
        fireEvent.mouseDown(hostInput);
        fireEvent.mouseUp(overlay);
        fireEvent.click(overlay);

        expect(onClose).not.toHaveBeenCalled();
        expect(screen.getByLabelText('主机地址')).toHaveValue('10.0.0.1');
    });

    it('按下与点击都发生在遮罩空白处：保留点击外部关闭的手势', () => {
        const { onClose } = renderEdit();
        const overlay = getOverlay('编辑连接');

        fireEvent.mouseDown(overlay);
        fireEvent.click(overlay);

        expect(onClose).toHaveBeenCalledTimes(1);
    });

    it('点击取消按钮：不发起保存、直接关闭', () => {
        const { onClose } = renderEdit();

        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(UpdateSavedConnection).not.toHaveBeenCalled();
        expect(onClose).toHaveBeenCalledTimes(1);
    });
});

describe('新建连接业务流程', () => {
    it('保存新连接：不连接任何服务器，只把配置写到指定文件夹', async () => {
        const { onClose, onSaved } = renderCreate('f-prod', '生产');

        fireEvent.change(screen.getByLabelText('连接名称'), { target: { value: 'web-9' } });
        fireEvent.click(screen.getByRole('button', { name: '保存' }));

        await waitFor(() => expect(CreateSavedConnection).toHaveBeenCalledTimes(1));
        const [payload, parentId] = CreateSavedConnection.mock.calls[0] as [Record<string, unknown>, string];
        expect(parentId).toBe('f-prod');
        expect(payload).toMatchObject({ name: 'web-9', host: '10.0.0.9', user: 'root' });
        // "新建会话"的关键语义：不建立任何会话/连接。
        expect(UpdateSavedConnection).not.toHaveBeenCalled();
        expect(onSaved).toHaveBeenCalled();
        expect(onClose).toHaveBeenCalled();
    });

    it('落点为空时表示保存到根目录，并提示保存位置', () => {
        renderCreate('', undefined);
        expect(screen.getByText('将保存到根目录')).toBeInTheDocument();
    });

    it('落在文件夹时提示具体位置', () => {
        renderCreate('f-prod', '生产 / 华东');
        expect(screen.getByText('将保存到「生产 / 华东」')).toBeInTheDocument();
    });

    it('主机关空时拦截保存', () => {
        renderCreate();
        fireEvent.change(screen.getByLabelText('主机地址'), { target: { value: '' } });
        fireEvent.click(screen.getByRole('button', { name: '保存' }));

        expect(screen.getByText('主机地址不能为空')).toBeInTheDocument();
        expect(CreateSavedConnection).not.toHaveBeenCalled();
    });
});
