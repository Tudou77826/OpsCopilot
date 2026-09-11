/**
 * Xshell 快捷命令导入业务流程用例。
 *
 * 走真实链路：QuickCommandPanel → CommandGrid → QuickCommandImportDialog →
 * wailsQuickCommandHost → window.go（mock）。重点验证五件事：
 *   1. 入口只在宿主提供能力时出现；
 *   2. 预览逐条列出命令，不可导入的置灰并说明原因；
 *   3. 勾选、改名、改内容、逐条指定分组都真的传到后端；
 *   4. 导入完成后面板随之刷新（后端推送的事件被接住）；
 *   5. 取消不写入任何数据。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import QuickCommandPanel from './QuickCommandPanel';

const QBL_DIR = 'C:\\Users\\15802\\Documents\\NetSarang Computer\\8\\Xshell\\QuickButton Files';
const COMMANDS_QBL = QBL_DIR + '\\commands.qbl';

const LoadQuickCommands = vi.fn(async () => [
    { id: 'g1-1', name: '重启服务', content: 'systemctl restart app', group: '默认' },
]);

const DetectXshellQuickButtonDirs = vi.fn(async () => [
    { path: QBL_DIR, version: 8, sets: 1, buttons: 3 },
]);

const AnalyzeQuickCommandImport = vi.fn(async () => ({
    sets: 1,
    buttons: 3,
    importable: 2,
    unsupported: 1,
    existing: 0,
    groups: ['Xshell'],
    rows: [
        {
            source: COMMANDS_QBL,
            name: 'commands',
            buttons: 3,
            importable: 2,
            unsupported: 1,
            existing: 0,
            group: 'Xshell',
            items: [
                { name: 'tail', content: 'tail -f /var/log/app.log', type: '1', supported: true, existing: false },
                { name: 'df', content: 'df -h', type: '1', supported: true, existing: false },
                {
                    name: '脚本按钮',
                    content: 'echo script',
                    type: '2',
                    supported: false,
                    skipReason: '按钮 脚本按钮 的类型为 2，OpsCopilot 暂只支持「发送字符串」类型，已跳过',
                    existing: false,
                },
            ],
        },
    ],
    warnings: ['commands: 按钮 脚本按钮 的类型为 2，OpsCopilot 暂只支持「发送字符串」类型，已跳过'],
}));

const ApplyQuickCommandImport = vi.fn(async () => ({
    imported: 2,
    skippedExisting: 0,
    skippedUnsupported: 0,
    groups: ['运维命令'],
    warnings: null as unknown as string[],
}));

/** 后端推送事件的处理函数，用于模拟导入后后端 emit 的刷新。 */
let externalEmit: ((cmds: unknown[]) => void) | null = null;

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                LoadQuickCommands,
                DetectXshellQuickButtonDirs,
                SelectQuickCommandImportFile: vi.fn(async () => ''),
                SelectQuickCommandImportDirectory: vi.fn(async () => ''),
                AnalyzeQuickCommandImport,
                ApplyQuickCommandImport,
            },
        },
    };
    (window as any).runtime = {
        EventsOn: (name: string, handler: (cmds: unknown[]) => void) => {
            if (name === 'quick-commands-updated') externalEmit = handler;
            return () => {};
        },
    };
});

beforeEach(() => {
    window.localStorage.clear();
    LoadQuickCommands.mockClear();
    DetectXshellQuickButtonDirs.mockClear();
    AnalyzeQuickCommandImport.mockClear();
    ApplyQuickCommandImport.mockClear();
    externalEmit = null;
});

/** 渲染面板。ToastProvider 与生产一致（main.tsx 里包在 App 外层）。 */
function renderPanel() {
    return render(
        <ToastProvider>
            <QuickCommandPanel isOpen onExecute={vi.fn()} />
        </ToastProvider>,
    );
}

/** 打开面板并进入导入对话框。 */
async function openImportDialog() {
    renderPanel();
    await screen.findByText('重启服务');
    fireEvent.click(screen.getByTestId('command-import-btn'));
    await screen.findByText('导入 Xshell 快捷命令');
}

/** 再进到预览步。 */
async function openImportReview() {
    await openImportDialog();
    await waitFor(() => expect(DetectXshellQuickButtonDirs).toHaveBeenCalled());
    fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
    await screen.findByText('导入预览');
}

describe('导入入口（B0）', () => {
    it('宿主提供导入能力时，卡片流里出现「导入」入口', async () => {
        renderPanel();
        await screen.findByText('重启服务');
        expect(screen.getByTestId('command-import-btn')).toBeInTheDocument();
    });
});

describe('分析与逐条预览（B1）', () => {
    it('自动探测本机目录并默认选中，预览逐条列出命令且不可导入项置灰', async () => {
        await openImportReview();

        expect(AnalyzeQuickCommandImport).toHaveBeenCalledWith(QBL_DIR, { defaultGroup: 'Xshell' });

        // 逐条可见、可编辑
        expect(screen.getByTestId('import-item-name-0-0')).toHaveValue('tail');
        expect(screen.getByTestId('import-item-content-0-0')).toHaveValue('tail -f /var/log/app.log');
        // 不可导入的类型置灰并写明原因，且没有勾选框
        expect(screen.getByTestId('import-item-unsupported-0-2')).toHaveTextContent('类型为 2');
        expect(screen.queryByTestId('import-item-0-2')).not.toBeInTheDocument();
    });
});

describe('命令级取舍与编辑（B2）', () => {
    it('取消一条、改名、改内容、逐条指定分组，全部随请求到后端', async () => {
        await openImportReview();

        fireEvent.click(screen.getByTestId('import-item-0-1')); // 不要 df
        fireEvent.change(screen.getByTestId('import-item-name-0-0'), { target: { value: '跟踪应用日志' } });
        fireEvent.change(screen.getByTestId('import-item-content-0-0'), { target: { value: 'tail -F /var/log/app.log' } });
        fireEvent.change(screen.getByTestId('import-item-group-0-0'), { target: { value: '日志排查' } });
        fireEvent.change(screen.getByTestId('import-group-0'), { target: { value: '运维命令' } });

        fireEvent.click(screen.getByRole('button', { name: /确认导入 1 条/ }));

        await waitFor(() => expect(ApplyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(ApplyQuickCommandImport).toHaveBeenCalledWith(
            QBL_DIR,
            [
                {
                    source: COMMANDS_QBL,
                    group: '运维命令',
                    items: [{ name: '跟踪应用日志', content: 'tail -F /var/log/app.log', group: '日志排查' }],
                },
            ],
            { defaultGroup: 'Xshell' },
        );
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('取消整套按钮后它不参与导入，且禁止在一条都不选时确认', async () => {
        await openImportReview();

        fireEvent.click(screen.getByTestId('import-set-0'));
        expect(screen.getByRole('button', { name: '确认导入' })).toBeDisabled();
        expect(ApplyQuickCommandImport).not.toHaveBeenCalled();
    });
});

describe('落盘与刷新（B3）', () => {
    it('导入后在报告里给出写入的分组，面板随事件刷新出新分组', async () => {
        await openImportReview();
        fireEvent.change(screen.getByTestId('import-group-0'), { target: { value: '运维命令' } });
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        await screen.findByText('导入结果');
        expect(screen.getByText(/已写入分组：运维命令/)).toBeInTheDocument();

        // 后端写完会 emit 最新列表，面板接住后新分组出现在分组条里
        expect(externalEmit).toBeTypeOf('function');
        externalEmit!([
            { id: 'g1-1', name: '重启服务', content: 'systemctl restart app', group: '默认' },
            { id: 'qc-1-0', name: '跟踪应用日志', content: 'tail -F /var/log/app.log', group: '运维命令' },
        ]);
        expect(await screen.findByTestId('group-item-运维命令')).toBeInTheDocument();
    });

    it('取消导入不写入任何数据', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(ApplyQuickCommandImport).not.toHaveBeenCalled();
        expect(AnalyzeQuickCommandImport).not.toHaveBeenCalled();
    });
});

describe('重复导入（B4）', () => {
    it('全部重复时报告为无新增，不显示写入的分组', async () => {
        ApplyQuickCommandImport.mockImplementationOnce(async () => ({
            imported: 0,
            skippedExisting: 2,
            skippedUnsupported: 0,
            groups: [],
            warnings: [],
        }));

        await openImportReview();
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        await screen.findByText('导入结果');
        expect(screen.getByText('已存在或重复（跳过）')).toBeInTheDocument();
        expect(screen.queryByText(/已写入分组/)).not.toBeInTheDocument();
    });
});
