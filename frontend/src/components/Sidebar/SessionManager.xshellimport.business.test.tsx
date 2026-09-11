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
type AnalysisShape = {
    total: number; supported: number; unsupported: number; existing: number;
    withPassword: number; passwordDecrypted: number; passwordFailed: number; groups: number;
    protocols: Record<string, number> | null; warnings: string[] | null;
};
const AnalyzeXshellImport = vi.fn(async (_path: string, _opts: unknown): Promise<AnalysisShape> => ({
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
type ReportShape = {
    imported: number; skippedExisting: number; skippedUnsupported: number;
    passwordDecrypted: number; passwordFailed: number; warnings: string[] | null;
};
const ApplyXshellImport = vi.fn(async (_path: string, _opts: unknown): Promise<ReportShape> => ({
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
        warnings: null,
    }));
    ApplyXshellImport.mockImplementation(async () => ({
        imported: 2,
        skippedExisting: 1,
        skippedUnsupported: 0,
        passwordDecrypted: 2,
        passwordFailed: 0,
        warnings: null,
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

    it('密码全部解密时不出现解密失败的提示', async () => {
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        await screen.findByText('导入预览');
        expect(screen.queryByText(/密码无法解密/)).not.toBeInTheDocument();
    });

    it('不提供"导入选项"：导入行为固定为带密码', async () => {
        await openImportDialog();

        // 密码总是要带的，同机场景也不需要源机器 SID / 主密码，因此不暴露这些开关。
        expect(screen.queryByText(/导入选项/)).not.toBeInTheDocument();
        expect(screen.queryByPlaceholderText(/S-1-5-21/)).not.toBeInTheDocument();
        expect(screen.queryByText(/源机器 SID/)).not.toBeInTheDocument();
    });
});

describe('密码解密失败不静默（B5）', () => {
    it('说明失败原因，且这些会话仍然会被导入（密码留空待补）', async () => {
        AnalyzeXshellImport.mockImplementation(async () => ({
            total: 3,
            supported: 2,
            unsupported: 1,
            existing: 0,
            withPassword: 2,
            passwordDecrypted: 0,
            passwordFailed: 2,
            groups: 2,
            protocols: null,
            warnings: null,
        }));
        await openImportDialog();
        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        // 失败原因必须说清（密钥依赖导出机器的账户标识），而不是静默留空。
        expect(await screen.findByText(/有 2 个会话的密码无法解密/)).toBeInTheDocument();
        expect(screen.getByText(/依赖导出那台电脑的 Windows 账户标识/)).toBeInTheDocument();
        // 并且明确后续怎么办：会话照样导入，密码导入后手工补。
        expect(screen.getByText(/仍会被导入/)).toBeInTheDocument();

        // 仍然允许直接导入：这些会话不该因为密码解不开就被丢掉。
        ApplyXshellImport.mockImplementation(async () => ({
            imported: 2,
            skippedExisting: 0,
            skippedUnsupported: 1,
            passwordDecrypted: 0,
            passwordFailed: 2,
            warnings: null,
        }));
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        await waitFor(() => expect(ApplyXshellImport).toHaveBeenCalledTimes(1));
        // 带密码导入是固定行为，不需要用户勾选。
        expect(ApplyXshellImport.mock.calls[0][1]).toMatchObject({ decryptPassword: true });
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
        expect(screen.getByText('密码未解密')).toBeInTheDocument();
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
                warnings: null,
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

describe('导入来源区块', () => {
    it('两种来源收在同一块，底部只有一行"将从此处导入"作为唯一结论', async () => {
        await openImportDialog();

        expect(screen.getByText('导入来源')).toBeInTheDocument();
        expect(screen.getByText('本机检测到的 Xshell 会话目录')).toBeInTheDocument();
        expect(screen.getByText('或从文件 / 目录导入')).toBeInTheDocument();

        // 旧的"已选择："挂在手动选择区下，会把自动探测的结果读成"用户手动选的"，
        // 同一个路径也因此出现两次。现在只保留一行明确的结论。
        expect(screen.queryByText(/已选择/)).not.toBeInTheDocument();
        const summary = screen.getByText(/将从此处导入/);
        expect(summary).toHaveTextContent('NetSarang Computer');
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
