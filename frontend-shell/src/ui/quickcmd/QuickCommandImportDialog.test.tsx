import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ToastProvider } from '../feedback/Toast';
import QuickCommandImportDialog from './QuickCommandImportDialog';
import type { QuickCommandHost, QuickCommandImportAnalysis, QuickCommandImportReport, XshellQuickButtonDir } from '../ports';

const QBL_DIR = 'C:\\Users\\15802\\Documents\\NetSarang Computer\\8\\Xshell\\QuickButton Files';
const COMMANDS_QBL = QBL_DIR + '\\commands.qbl';

type AnalysisOverrides = Partial<QuickCommandImportAnalysis>;

function makeAnalysis(overrides: AnalysisOverrides = {}): QuickCommandImportAnalysis {
    return {
        sets: 1,
        buttons: 2,
        importable: 2,
        unsupported: 0,
        existing: 0,
        groups: ['Xshell'],
        rows: [
            {
                source: COMMANDS_QBL,
                name: 'commands',
                buttons: 2,
                importable: 2,
                unsupported: 0,
                existing: 0,
                group: 'Xshell',
            },
        ],
        warnings: [],
        ...overrides,
    };
}

function makeHost(overrides: Partial<QuickCommandHost> = {}) {
    const dirs: XshellQuickButtonDir[] = [{ path: QBL_DIR, version: 8, sets: 1, buttons: 2 }];
    const detectQuickButtonDirs = vi.fn(async () => dirs);
    const selectImportFile = vi.fn(async () => '');
    const selectImportDirectory = vi.fn(async () => '');
    const analyzeQuickCommandImport = vi.fn(async () => makeAnalysis());
    const applyQuickCommandImport = vi.fn(
        async (): Promise<QuickCommandImportReport> => ({
            imported: 2,
            skippedExisting: 0,
            skippedUnsupported: 0,
            groups: ['Xshell'],
            warnings: [],
        }),
    );

    const host: QuickCommandHost = {
        execute: () => {},
        storage: {
            load: async () => [],
            add: () => {},
            update: () => {},
            remove: () => {},
            reorder: () => {},
        },
        detectQuickButtonDirs,
        selectImportFile,
        selectImportDirectory,
        analyzeQuickCommandImport,
        applyQuickCommandImport,
        ...overrides,
    };
    return { host, detectQuickButtonDirs, selectImportFile, selectImportDirectory, analyzeQuickCommandImport, applyQuickCommandImport };
}

function renderDialog(host: QuickCommandHost, existingGroups: string[] = []) {
    return render(
        <ToastProvider>
            <QuickCommandImportDialog isOpen host={host} existingGroups={existingGroups} onClose={() => {}} />
        </ToastProvider>,
    );
}

/** 渲染并等自动探测落地（默认选中第一个目录）——分析按钮在此之前是禁用的。 */
async function renderReady(host: QuickCommandHost, existingGroups: string[] = []) {
    const utils = renderDialog(host, existingGroups);
    await waitFor(() => expect(screen.getByText(/将从此处导入/)).toBeInTheDocument());
    return utils;
}

beforeEach(() => {
    vi.clearAllMocks();
});

describe('入口与来源', () => {
    it('打开时自动探测本机目录并默认选中第一个，无需用户做任何导出操作', async () => {
        const { host, detectQuickButtonDirs } = makeHost();
        await renderReady(host);

        await waitFor(() => expect(detectQuickButtonDirs).toHaveBeenCalledTimes(1));
        expect(await screen.findByText(/Xshell 8/)).toBeInTheDocument();
        expect(screen.getByText(/1 套按钮 \/ 2 条/)).toBeInTheDocument();
        // 默认选中 → 底部结论行直接给出路径，用户点一下就能分析
        expect(screen.getByText(/将从此处导入/)).toBeInTheDocument();
    });

    it('宿主未提供导入能力时整个面板不渲染（sidecar 外壳）', async () => {
        const { host } = makeHost();
        delete (host as any).analyzeQuickCommandImport;

        const { container } = renderDialog(host);
        await waitFor(() => expect(container).toBeEmptyDOMElement());
    });
});

describe('分析并预览', () => {
    it('按默认分组分析，并把每行集合渲染成可编辑的分组输入框', async () => {
        const { host, analyzeQuickCommandImport } = makeHost();
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        await screen.findByText('导入预览');
        // 默认分组名由前端给定，后端据此计算"已存在"
        expect(analyzeQuickCommandImport).toHaveBeenCalledWith(QBL_DIR, 'Xshell');
        expect(screen.getByTestId('import-group-0')).toHaveValue('Xshell');
        expect(screen.getByText('commands')).toBeInTheDocument();
        expect(screen.getByText(/可导入 2 \/ 共 2 条/)).toBeInTheDocument();
    });

    it('后端返回 null 的集合字段不会让面板崩掉', async () => {
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () => ({
                sets: 1,
                buttons: 0,
                importable: 0,
                unsupported: 0,
                existing: 0,
                groups: null as unknown as string[],
                rows: null as unknown as QuickCommandImportAnalysis['rows'],
                warnings: null as unknown as string[],
            })),
        });
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));

        // 依然进入预览，且没有抛异常（render 抛异常时本用例会直接失败）
        expect(await screen.findByText('导入预览')).toBeInTheDocument();
        expect(screen.getByText('按钮总数')).toBeInTheDocument();
    });

    it('没探测到本机目录且未手选来源时，分析按钮禁用，不发起后端调用', async () => {
        const { host, analyzeQuickCommandImport } = makeHost({
            detectQuickButtonDirs: vi.fn(async () => []),
        });
        renderDialog(host);

        expect(await screen.findByText('尚未选择导入来源')).toBeInTheDocument();
        // 禁用而不是点了再报错：让"没得可导"这件事在操作前就可见
        expect(screen.getByRole('button', { name: '分析并预览' })).toBeDisabled();

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        expect(analyzeQuickCommandImport).not.toHaveBeenCalled();
    });
});

describe('分组落点', () => {
    it('改过的分组名会随 assignments 带到后端', async () => {
        const { host, applyQuickCommandImport } = makeHost();
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');

        fireEvent.change(screen.getByTestId('import-group-0'), { target: { value: '运维命令' } });
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(applyQuickCommandImport).toHaveBeenCalledWith(
            QBL_DIR,
            [{ source: COMMANDS_QBL, group: '运维命令' }],
            'Xshell',
        );
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('提示会说明是新建分组、追加到已有分组，还是与本批其它集合合并', async () => {
        const second = COMMANDS_QBL.replace('commands.qbl', 'ops.qbl');
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    sets: 2,
                    buttons: 4,
                    importable: 4,
                    rows: [
                        { source: COMMANDS_QBL, name: 'commands', buttons: 2, importable: 2, unsupported: 0, existing: 0, group: 'Xshell' },
                        { source: second, name: 'ops', buttons: 2, importable: 2, unsupported: 0, existing: 0, group: 'Xshell' },
                    ],
                }),
            ),
        });
        await renderReady(host, ['容器命令']);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');

        // 两个集合默认同名 → 合并提示
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将与本批其它集合合并到同一分组');
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('将与本批其它集合合并到同一分组');

        // 其中一个改成已有分组名 → 追加提示
        fireEvent.change(screen.getByTestId('import-group-1'), { target: { value: '容器命令' } });
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('将追加到已有分组');
        // 另一个仍是默认名 → 变成新建
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');

        // 改成全新名字 → 新建提示
        fireEvent.change(screen.getByTestId('import-group-1'), { target: { value: '全新分组' } });
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('将新建分组');
    });

    it('分组名清空时提示不能为空', async () => {
        const { host } = makeHost();
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');

        fireEvent.change(screen.getByTestId('import-group-0'), { target: { value: '' } });
        expect(screen.getByText('分组名不能为空')).toBeInTheDocument();
    });
});

describe('结果与取消', () => {
    it('展示导入结果与写入的分组，并在没有新增时给出提示', async () => {
        const { host } = makeHost({
            applyQuickCommandImport: vi.fn(async () => ({
                imported: 0,
                skippedExisting: 2,
                skippedUnsupported: 0,
                groups: [],
                warnings: [],
            })),
        });
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        expect(await screen.findByText('导入结果')).toBeInTheDocument();
        expect(screen.getByText('已存在或重复（跳过）')).toBeInTheDocument();
    });

    it('后端返回 null 的 groups/warnings 不会让结果页崩掉', async () => {
        const { host } = makeHost({
            applyQuickCommandImport: vi.fn(async () => ({
                imported: 1,
                skippedExisting: 0,
                skippedUnsupported: 0,
                groups: null as unknown as string[],
                warnings: null as unknown as string[],
            })),
        });
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));

        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('宿主对象在流程中途被重建时，不会把界面重置回来源步', async () => {
        const { host, applyQuickCommandImport } = makeHost();
        const { rerender } = renderDialog(host);
        await waitFor(() => expect(screen.getByText(/将从此处导入/)).toBeInTheDocument());

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');
        fireEvent.click(screen.getByRole('button', { name: '确认导入' }));
        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(await screen.findByText('导入结果')).toBeInTheDocument();

        // 实机上外层每次渲染都会重建 host（useMemo 依赖 onExecute，而它不是稳定引用），
        // 若重置逻辑跟着 host 走，导入一完成就跳回来源步，报告一闪而过。
        rerender(
            <ToastProvider>
                <QuickCommandImportDialog isOpen host={{ ...host }} existingGroups={[]} onClose={() => {}} />
            </ToastProvider>,
        );

        expect(screen.getByText('导入结果')).toBeInTheDocument();
        expect(screen.queryByText('导入来源')).not.toBeInTheDocument();
    });

    it('未执行导入就取消时，不发起任何写入', async () => {
        const { host, applyQuickCommandImport, analyzeQuickCommandImport } = makeHost();
        await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(applyQuickCommandImport).not.toHaveBeenCalled();
        expect(analyzeQuickCommandImport).not.toHaveBeenCalled();
    });
});
