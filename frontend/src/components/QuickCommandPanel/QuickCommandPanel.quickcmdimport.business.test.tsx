/**
 * Xshell 快捷命令导入业务流程用例。
 *
 * 走真实链路：QuickCommandPanel → CommandGrid → QuickCommandImportDialog →
 * wailsQuickCommandHost → window.go（mock）。重点验证四件事：
 *   1. 入口只在宿主提供能力时出现；
 *   2. 导入前能看到每套按钮有多少条能搬、多少条会被跳过；
 *   3. 分组名在面板里可改，改后的名字真的传到后端；
 *   4. 导入完成后面板随之刷新（后端推送的事件被接住）。
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
    { path: QBL_DIR, version: 8, sets: 1, buttons: 2 },
]);

const AnalyzeQuickCommandImport = vi.fn(async () => ({
    sets: 1,
    buttons: 2,
    importable: 1,
    unsupported: 1,
    existing: 0,
    groups: ['Xshell'],
    rows: [
        {
            source: COMMANDS_QBL,
            name: 'commands',
            buttons: 2,
            importable: 1,
            unsupported: 1,
            existing: 0,
            group: 'Xshell',
        },
    ],
    warnings: ['commands: 按钮 脚本按钮 的类型为 2，OpsCopilot 暂只支持「发送字符串」类型，已跳过'],
}));

const ApplyQuickCommandImport = vi.fn(async () => ({
    imported: 1,
    skippedExisting: 0,
    skippedUnsupported: 1,
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

describe('导入入口（B0）', () => {
    it('宿主提供导入能力时，卡片流里出现「导入」入口', async () => {
        renderPanel();
        await screen.findByText('重启服务');
        expect(screen.getByTestId('command-import-btn')).toBeInTheDocument();
    });
});

describe('分析与预览（B1）', () => {
    it('自动探测本机快捷按钮目录并默认选中，分析后逐集合展示可导入与跳过条数', async () => {
        await openImportDialog();

        // 无需用户做任何 Xshell 导出操作，路径已默认选中
        await waitFor(() => expect(DetectXshellQuickButtonDirs).toHaveBeenCalledTimes(1));
        expect(screen.getByText(/Xshell 8/)).toBeInTheDocument();
        expect(screen.getByText(/1 套按钮 \/ 2 条/)).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        await screen.findByText('导入预览');
        // 绑定层给的是驼峰 options 对象（Go 侧 DTO 的 json tag 就是 defaultGroup）
        expect(AnalyzeQuickCommandImport).toHaveBeenCalledWith(QBL_DIR, { defaultGroup: 'Xshell' });
        // 有损映射在写入前就讲清楚：1 条能搬、1 条类型不支持
        expect(screen.getByText(/可导入 1 \/ 共 2 条/)).toBeInTheDocument();
        expect(screen.getByText('类型不支持（跳过）')).toBeInTheDocument();
        expect(screen.getByText(/1 条提示/)).toBeInTheDocument();
    });
});

describe('分组落点（B2）', () => {
    it('在面板里改分组名后导入，改后的名字与写入分组都体现在报告里，且面板随之刷新', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');

        // 默认建议值是 Xshell；用户改成自己的分组名
        const groupInput = screen.getByTestId('import-group-0');
        expect(groupInput).toHaveValue('Xshell');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');
        fireEvent.change(groupInput, { target: { value: '运维命令' } });

        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        await waitFor(() => expect(ApplyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(ApplyQuickCommandImport).toHaveBeenCalledWith(
            QBL_DIR,
            [{ source: COMMANDS_QBL, group: '运维命令' }],
            { defaultGroup: 'Xshell' },
        );

        expect(await screen.findByText('导入结果')).toBeInTheDocument();
        expect(screen.getByText(/已写入分组：运维命令/)).toBeInTheDocument();
        expect(screen.getByText('类型不支持（跳过）')).toBeInTheDocument();

        // 后端写完会 emit 最新列表，面板接住后新分组出现在分组条里
        expect(externalEmit).toBeTypeOf('function');
        externalEmit!([
            { id: 'g1-1', name: '重启服务', content: 'systemctl restart app', group: '默认' },
            { id: 'qc-1-0', name: 'tail', content: 'tail -f /var/log/app.log', group: '运维命令' },
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

describe('重复导入（B3）', () => {
    it('全部重复时报告为无新增，不产生新命令', async () => {
        ApplyQuickCommandImport.mockImplementationOnce(async () => ({
            imported: 0,
            skippedExisting: 2,
            skippedUnsupported: 0,
            groups: [],
            warnings: [],
        }));

        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        await screen.findByText('导入结果');
        expect(screen.getByText('已存在或重复（跳过）')).toBeInTheDocument();
        // 没有新增时不必显示"已写入分组"
        expect(screen.queryByText(/已写入分组/)).not.toBeInTheDocument();
    });
});
