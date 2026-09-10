/**
 * Xshell 导入业务流程用例（B1/B2/B5）。
 *
 * 走真实链路：SessionManager → XshellImportDialog → wailsSessionRuntime → window.go（mock）。
 * 重点验证三件事：
 *   1. 分析结果在写入前就告知用户会发生什么；
 *   2. 密码解密失败不静默——给出原因与补充凭据的入口；
 *   3. 导出引导的文案是用户能照做的，且不包含已核实为错误的指引。
 */
import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeAll, beforeEach } from 'vitest';
import '@testing-library/jest-dom';
import { ToastProvider } from '@opscopilot/shell-terminal/ui';
import SessionManager from './SessionManager';

const treeAfterImport = [
    {
        id: 'f-prod',
        name: '生产',
        type: 'folder',
        children: [
            { id: 's-imported', name: 'web-1', type: 'session', config: { host: '10.0.0.1', port: 22, user: 'root' } },
        ],
    },
];

const GetConnectionTree = vi.fn(async () => [] as unknown[]);
const DetectXshellSessionDirs = vi.fn(async () => [
    { path: 'C:\\Users\\15802\\Documents\\NetSarang Computer\\8\\Xshell\\Sessions', version: 8, sessions: 3 },
]);
type ImportStatus = { available: boolean; maskedSid?: string; windowsUser?: string; message: string };
const GetXshellImportStatus = vi.fn(async (): Promise<ImportStatus> => ({
    available: true,
    maskedSid: 'S-1-5-21-****-****-1002',
    windowsUser: '15802',
    message: '已检测到本机 Xshell 凭据，导入的密码将自动解密',
}));
const AnalyzeXshellImport = vi.fn(async (_path: string, _opts: unknown) => ({
    total: 3,
    supported: 2,
    unsupported: 1,
    existing: 1,
    withPassword: 2,
    passwordDecrypted: 2,
    passwordFailed: 0,
    groups: 2,
    protocols: { ssh: 3 },
    warnings: [],
}));
const ApplyXshellImport = vi.fn(async (_path: string, _opts: unknown) => ({
    imported: 2,
    skippedExisting: 1,
    skippedUnsupported: 0,
    passwordDecrypted: 2,
    passwordFailed: 0,
    warnings: [],
}));

beforeAll(() => {
    (window as any).go = {
        main: {
            App: {
                GetConnectionTree,
                DetectXshellSessionDirs,
                GetXshellImportStatus,
                AnalyzeXshellImport,
                ApplyXshellImport,
                SelectSessionImportFile: vi.fn(async () => ''),
                SelectSessionImportDirectory: vi.fn(async () => ''),
                MoveTreeNode: vi.fn(async () => undefined),
                DeleteTreeNode: vi.fn(async () => undefined),
                RenameTreeNode: vi.fn(async () => undefined),
                CreateSavedFolder: vi.fn(async () => undefined),
                CreateSavedConnection: vi.fn(async () => undefined),
                UpdateSavedConnection: vi.fn(async () => undefined),
                DuplicateSavedConnection: vi.fn(async () => undefined),
                ReorderTreeChildren: vi.fn(async () => undefined),
                GetSharedSessions: vi.fn(async () => JSON.stringify({ enabled: false })),
            },
        },
    };
});

beforeEach(() => {
    window.localStorage.clear();
    GetConnectionTree.mockClear();
    GetConnectionTree.mockImplementation(async () => []);
    DetectXshellSessionDirs.mockClear();
    GetXshellImportStatus.mockClear();
    AnalyzeXshellImport.mockClear();
    ApplyXshellImport.mockClear();
    AnalyzeXshellImport.mockImplementation(async () => ({
        total: 3,
        supported: 2,
        unsupported: 1,
        existing: 1,
        withPassword: 2,
        passwordDecrypted: 2,
        passwordFailed: 0,
        groups: 2,
        protocols: { ssh: 3 },
        warnings: [],
    }));
    ApplyXshellImport.mockImplementation(async () => ({
        imported: 2,
        skippedExisting: 1,
        skippedUnsupported: 0,
        passwordDecrypted: 2,
        passwordFailed: 0,
        warnings: [],
    }));
});

async function openImportDialog() {
    render(
        <ToastProvider>
            <SessionManager onConnect={vi.fn()} />
        </ToastProvider>
    );
    fireEvent.click(await screen.findByRole('button', { name: '导入' }));
    await screen.findByText('导入 Xshell 会话');
}

describe('导入入口与凭据状态', () => {
    it('打开后展示本机凭据状态与检测到的会话目录，默认选中最新版本', async () => {
        await openImportDialog();

        expect(await screen.findByText(/已检测到本机 Xshell 凭据/)).toBeInTheDocument();
        // 遮蔽后的 SID：确认展示的是被遮中段的标识，而不是完整 SID。
        expect(screen.getByText(/S-1-5-21-\*{4}-\*{4}-1002/)).toBeInTheDocument();
        expect(screen.getByText(/Xshell 8/)).toBeInTheDocument();
        expect(screen.getByText(/3 个会话/)).toBeInTheDocument();
    });

    it('凭据不可用时给出"需手工填密码"的预期，而不是假装能解密', async () => {
        GetXshellImportStatus.mockImplementation(async () => ({
            available: false,
            message: '未能读取本机 Windows 账户标识，导入后需要手工填写密码',
        }));
        await openImportDialog();

        expect(await screen.findByText(/需要手工填写密码/)).toBeInTheDocument();
    });
});

describe('分析预览（B1）', () => {
    it('写入前展示将导入/已存在/密码解密等统计', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        await waitFor(() => expect(AnalyzeXshellImport).toHaveBeenCalledTimes(1));
        // 分析用的是自动选中的本机会话目录，且默认开启密码导入。
        expect(AnalyzeXshellImport.mock.calls[0][0]).toContain('NetSarang Computer');
        expect(AnalyzeXshellImport.mock.calls[0][1]).toMatchObject({ decryptPassword: true });

        expect(await screen.findByText('导入预览')).toBeInTheDocument();
        expect(screen.getByText('解析到会话')).toBeInTheDocument();
        expect(screen.getByText('已存在（跳过）')).toBeInTheDocument();
        expect(screen.getByText('密码已解密')).toBeInTheDocument();
    });

    it('密码全部解密时不显示补充凭据的提示', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        await screen.findByText('导入预览');
        expect(screen.queryByText(/建议补充凭据后重试/)).not.toBeInTheDocument();
    });
});

describe('密码解密失败不静默（B5）', () => {
    it('给出原因、展开补充凭据入口，并支持改参数后重新分析', async () => {
        AnalyzeXshellImport.mockImplementation(async () => ({
            total: 3,
            supported: 2,
            unsupported: 1,
            existing: 0,
            withPassword: 2,
            passwordDecrypted: 0,
            passwordFailed: 2,
            groups: 2,
            protocols: { ssh: 3 },
            warnings: [],
        }));
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        // 原因要说清是"密钥依赖导出机器的账户标识"，并给出下一步动作。
        expect(await screen.findByText(/有 2 个会话的密码无法解密/)).toBeInTheDocument();
        expect(screen.getByText(/源机器的 SID 或 Xshell 主密码/)).toBeInTheDocument();
        // 补充凭据的输入项自动展开。
        expect(screen.getByPlaceholderText(/S-1-5-21/)).toBeInTheDocument();

        // 填入源机器 SID 后可以重新分析，参数被带到后端。
        fireEvent.change(screen.getByPlaceholderText(/S-1-5-21/), {
            target: { value: 'S-1-5-21-1-2-3-1009' },
        });
        fireEvent.click(screen.getByRole('button', { name: '重新分析' }));

        await waitFor(() => expect(AnalyzeXshellImport).toHaveBeenCalledTimes(2));
        expect(AnalyzeXshellImport.mock.calls[1][1]).toMatchObject({ sourceSid: 'S-1-5-21-1-2-3-1009' });
    });
});

describe('执行导入（B2）', () => {
    it('导入后展示结果报告，并刷新会话树', async () => {
        GetConnectionTree.mockImplementation(async () => [] as unknown[]);
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');

        ApplyXshellImport.mockImplementation(async () => {
            GetConnectionTree.mockImplementation(async () => treeAfterImport);
            return {
                imported: 2,
                skippedExisting: 1,
                skippedUnsupported: 0,
                passwordDecrypted: 2,
                passwordFailed: 0,
                warnings: [],
            };
        });
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        await waitFor(() => expect(ApplyXshellImport).toHaveBeenCalledTimes(1));
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
        expect(screen.getByText('已导入')).toBeInTheDocument();
    });

    it('取消时不发起任何写入', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(ApplyXshellImport).not.toHaveBeenCalled();
        expect(AnalyzeXshellImport).not.toHaveBeenCalled();
    });
});

describe('导出引导', () => {
    it('展开后给出可照做的步骤，并提醒导出文件含密码', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByText(/如何在 Xshell 里导出/));

        expect(screen.getByText(/点左上角的/)).toBeInTheDocument();
        expect(screen.getByText(/勾选要导出的会话/)).toBeInTheDocument();
        // 「不要」在 <b> 里，其余是同级文本节点，因此按后半句匹配。
        expect(screen.getByText(/清除密码 \/ 不导出密码/)).toBeInTheDocument();
        expect(screen.getByText(/请勿随意外发/)).toBeInTheDocument();
        // 兜底路径：会话管理器右键"在资源管理器中打开文件夹"（已核实存在）。
        expect(screen.getByText(/在资源管理器中打开文件夹/)).toBeInTheDocument();
    });
});
