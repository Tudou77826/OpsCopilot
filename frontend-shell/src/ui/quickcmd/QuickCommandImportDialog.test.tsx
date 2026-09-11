import React from 'react';
import { render, screen, fireEvent, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { ToastProvider } from '../feedback/Toast';
import QuickCommandImportDialog from './QuickCommandImportDialog';
import type {
    QuickCommandHost,
    QuickCommandImportAnalysis,
    QuickCommandImportReport,
    QuickCommandImportSelection,
    QuickCommandSetItem,
    QuickCommandSetRow,
    XshellQuickButtonDir,
} from '../ports';

const QBL_DIR = 'C:\\Users\\15802\\Documents\\NetSarang Computer\\8\\Xshell\\QuickButton Files';
const COMMANDS_QBL = QBL_DIR + '\\commands.qbl';
const OPS_QBL = QBL_DIR + '\\ops.qbl';

const UNSUPPORTED_REASON = '按钮 脚本按钮 的类型为 2，OpsCopilot 暂只支持「发送字符串」类型，已跳过';

function item(overrides: Partial<QuickCommandSetItem> = {}): QuickCommandSetItem {
    return {
        name: 'tail',
        content: 'tail -f /var/log/app.log',
        type: '1',
        supported: true,
        existing: false,
        ...overrides,
    };
}

function row(overrides: Partial<QuickCommandSetRow> = {}): QuickCommandSetRow {
    return {
        source: COMMANDS_QBL,
        name: 'commands',
        buttons: 3,
        importable: 2,
        unsupported: 1,
        existing: 0,
        group: 'Xshell',
        items: [
            item(),
            item({ name: 'df', content: 'df -h' }),
            item({ name: '脚本按钮', content: 'echo script', type: '2', supported: false, skipReason: UNSUPPORTED_REASON }),
        ],
        ...overrides,
    };
}

function makeAnalysis(overrides: Partial<QuickCommandImportAnalysis> = {}): QuickCommandImportAnalysis {
    const rows = overrides.rows ?? [row()];
    return {
        sets: rows.length,
        buttons: rows.reduce((n, r) => n + r.buttons, 0),
        importable: rows.reduce((n, r) => n + r.importable, 0),
        unsupported: rows.reduce((n, r) => n + r.unsupported, 0),
        existing: rows.reduce((n, r) => n + r.existing, 0),
        groups: rows.map((r) => r.group),
        rows,
        warnings: [UNSUPPORTED_REASON],
        ...overrides,
    };
}

function makeHost(overrides: Partial<QuickCommandHost> = {}, existingCommands: { name: string; content: string; group?: string }[] = []) {
    const dirs: XshellQuickButtonDir[] = [{ path: QBL_DIR, version: 8, sets: 1, buttons: 3 }];
    const detectQuickButtonDirs = vi.fn(async () => dirs);
    const selectImportFile = vi.fn(async () => '');
    const selectImportDirectory = vi.fn(async () => '');
    const load = vi.fn(async () => existingCommands.map((c, i) => ({ id: String(i), group: c.group ?? 'Xshell', ...c })));
    const analyzeQuickCommandImport = vi.fn(
        async (_path: string, _defaultGroup: string): Promise<QuickCommandImportAnalysis> => makeAnalysis(),
    );
    const applyQuickCommandImport = vi.fn(
        async (
            _path: string,
            _selections: QuickCommandImportSelection[],
            _defaultGroup: string,
        ): Promise<QuickCommandImportReport> => ({
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
            load,
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
    return { host, load, detectQuickButtonDirs, selectImportFile, selectImportDirectory, analyzeQuickCommandImport, applyQuickCommandImport };
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

/** 进到预览步（等待探测完成后分析）。 */
async function openReview(host: QuickCommandHost, existingGroups: string[] = []) {
    await renderReady(host, existingGroups);
    fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
    await screen.findByText('导入预览');
}

beforeEach(() => {
    vi.clearAllMocks();
});

describe('来源', () => {
    it('打开时自动探测本机目录并默认选中第一个，无需用户做任何导出操作', async () => {
        const { host, detectQuickButtonDirs } = makeHost();
        renderDialog(host);

        await waitFor(() => expect(detectQuickButtonDirs).toHaveBeenCalledTimes(1));
        expect(await screen.findByText(/Xshell 8/)).toBeInTheDocument();
        expect(screen.getByText(/1 套按钮 \/ 3 条/)).toBeInTheDocument();
        expect(screen.getByText(/将从此处导入/)).toBeInTheDocument();
    });

    it('宿主未提供导入能力时整个面板不渲染（sidecar 外壳）', async () => {
        const { host } = makeHost();
        delete (host as any).analyzeQuickCommandImport;

        const { container } = renderDialog(host);
        await waitFor(() => expect(container).toBeEmptyDOMElement());
    });

    it('没探测到本机目录且未手选来源时，分析按钮禁用，不发起后端调用', async () => {
        const { host, analyzeQuickCommandImport } = makeHost({
            detectQuickButtonDirs: vi.fn(async () => []),
        });
        renderDialog(host);

        expect(await screen.findByText('尚未选择导入来源')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '分析并预览' })).toBeDisabled();

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        expect(analyzeQuickCommandImport).not.toHaveBeenCalled();
    });
});

describe('命令级明细（本次粒度下钻的核心）', () => {
    it('预览逐条列出命令，含不可导入的条目及其原因', async () => {
        const { host } = makeHost();
        await openReview(host);

        expect(screen.getByTestId('import-item-name-0-0')).toHaveValue('tail');
        expect(screen.getByTestId('import-item-content-0-0')).toHaveValue('tail -f /var/log/app.log');
        expect(screen.getByTestId('import-item-name-0-1')).toHaveValue('df');

        // 不可导入的条目仍然列出来，但置灰且写明原因（不是只给一个跳过总数）
        const off = screen.getByTestId('import-item-unsupported-0-2');
        expect(off).toHaveTextContent('脚本按钮');
        expect(off).toHaveTextContent('类型为 2');
        // 可勾选的条目只有 2 条
        expect(screen.getByTestId('import-item-0-0')).toBeInTheDocument();
        expect(screen.getByTestId('import-item-0-1')).toBeInTheDocument();
        expect(screen.queryByTestId('import-item-0-2')).not.toBeInTheDocument();
    });

    it('默认全部勾选，汇总与逐条勾选状态一致', async () => {
        const { host } = makeHost();
        await openReview(host);

        expect(screen.getByTestId('import-item-0-0')).toBeChecked();
        expect(screen.getByTestId('import-item-0-1')).toBeChecked();
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');
        // 段头给出本套的条数，汇总卡片不再重复"按钮总数"
        expect(screen.getByText(/共 3 条，将导入 2 条/)).toBeInTheDocument();
    });

    it('逐条取消勾选后，汇总与"确认导入"上的条数随之减少', async () => {
        const { host } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-item-0-1'));
        expect(screen.getByTestId('import-item-0-1')).not.toBeChecked();
        expect(screen.getByRole('button', { name: /确认导入 1 条/ })).toBeInTheDocument();

        // 全不选 → 没有可导入项
        fireEvent.click(screen.getByTestId('import-select-none-0'));
        expect(screen.getByTestId('import-item-0-0')).not.toBeChecked();
        expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument();

        // 全选 → 又把两条都选回来
        fireEvent.click(screen.getByTestId('import-select-all-0'));
        expect(screen.getByTestId('import-item-0-0')).toBeChecked();
        expect(screen.getByTestId('import-item-0-1')).toBeChecked();
    });

    it('改名、改内容、逐条指定分组都会随请求回传', async () => {
        const { host, applyQuickCommandImport } = makeHost();
        await openReview(host);

        fireEvent.change(screen.getByTestId('import-item-name-0-0'), { target: { value: '跟踪应用日志' } });
        fireEvent.change(screen.getByTestId('import-item-content-0-0'), { target: { value: 'tail -f /var/log/new.log' } });
        fireEvent.change(screen.getByTestId('import-item-group-0-0'), { target: { value: '日志排查' } });
        fireEvent.click(screen.getByTestId('import-item-0-1')); // 取消 df

        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(applyQuickCommandImport).toHaveBeenCalledWith(
            QBL_DIR,
            [
                {
                    source: COMMANDS_QBL,
                    group: 'Xshell',
                    items: [{ name: '跟踪应用日志', content: 'tail -f /var/log/new.log', group: '日志排查' }],
                },
            ],
            'Xshell',
        );
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('收起/展开命令列表不影响已做的选择', async () => {
        const { host } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-set-toggle-0'));
        expect(screen.queryByTestId('import-item-0-0')).not.toBeInTheDocument();
        fireEvent.click(screen.getByTestId('import-set-toggle-0'));
        expect(screen.getByTestId('import-item-0-0')).toBeChecked();
    });
});

describe('集合级', () => {
    it('取消整个集合后它不参与导入，也不出现在请求里', async () => {
        const { host, applyQuickCommandImport } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    rows: [
                        row(),
                        row({ source: OPS_QBL, name: 'ops', buttons: 1, importable: 1, unsupported: 0, items: [item({ name: 'free', content: 'free -h' })] }),
                    ],
                }),
            ),
        });
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-set-1'));
        expect(screen.getByRole('button', { name: /确认导入 2 条/ })).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));
        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));

        const selections = applyQuickCommandImport.mock.calls[0][1] as any[];
        expect(selections).toHaveLength(2);
        expect(selections[1]).toMatchObject({ source: OPS_QBL, items: [] });
    });

    it('全部集合都不选时禁止确认', async () => {
        const { host } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-set-0'));
        expect(screen.getByRole('button', { name: '确认导入' })).toBeDisabled();
    });

    it('集合分组重名时提示合并或追加，空名给出提示', async () => {
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    rows: [
                        row(),
                        row({ source: OPS_QBL, name: 'ops', buttons: 1, importable: 1, unsupported: 0, items: [item({ name: 'free', content: 'free -h' })] }),
                    ],
                }),
            ),
        });
        await openReview(host, ['容器命令']);

        // 两段默认同名 → 合并提示
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将与本批其它集合合并到同一分组');
        // 改成已有分组 → 追加提示
        fireEvent.change(screen.getByTestId('import-group-1'), { target: { value: '容器命令' } });
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('将追加到已有分组');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');
        // 清空 → 提示不能为空
        fireEvent.change(screen.getByTestId('import-group-1'), { target: { value: '' } });
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('集合分组不能为空');
    });
});

describe('数字必须与实际写入一致', () => {
    it('已存在的条目不计入"将导入"，也不让按钮谎报条数', async () => {
        // tail 已经在 Xshell 分组里了
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: 'Xshell' }]);
        await openReview(host);

        // 3 条里：tail 已存在、df 可导入、脚本按钮不可导入
        expect(screen.getByTestId('import-item-existing-0-0')).toHaveTextContent('已存在');
        expect(screen.queryByTestId('import-item-existing-0-1')).not.toBeInTheDocument();
        expect(screen.getByText(/共 3 条，将导入 1 条/)).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 1 条' })).toBeInTheDocument();
    });

    it('改集合分组后"已存在"实时重算——换了分组就不再是重复', async () => {
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: 'Xshell' }]);
        await openReview(host);

        expect(screen.getByTestId('import-item-existing-0-0')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 1 条' })).toBeInTheDocument();

        // 落到一个新分组：tail 不再与已有命令重复，两条都可导入
        fireEvent.change(screen.getByTestId('import-group-0'), { target: { value: '全新分组' } });
        expect(screen.queryByTestId('import-item-existing-0-0')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 2 条' })).toBeInTheDocument();
        expect(screen.getByText(/共 3 条，将导入 2 条/)).toBeInTheDocument();
    });

    it('逐条改名后"已存在"标记随之消失', async () => {
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: 'Xshell' }]);
        await openReview(host);

        expect(screen.getByTestId('import-item-existing-0-0')).toBeInTheDocument();
        fireEvent.change(screen.getByTestId('import-item-name-0-0'), { target: { value: '跟踪日志' } });
        expect(screen.queryByTestId('import-item-existing-0-0')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 2 条' })).toBeInTheDocument();
    });

    it('逐条指定分组时按该条自己的分组判断重复', async () => {
        // 已有命令在"日志排查"分组里
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: '日志排查' }]);
        await openReview(host);

        // 集合默认分组是 Xshell，此时不重复
        expect(screen.queryByTestId('import-item-existing-0-0')).not.toBeInTheDocument();

        // 把这条指到"日志排查"就重复了
        fireEvent.change(screen.getByTestId('import-item-group-0-0'), { target: { value: '日志排查' } });
        expect(screen.getByTestId('import-item-existing-0-0')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 1 条' })).toBeInTheDocument();
    });

    it('同一批内重复只算一条（与后端同规则）', async () => {
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    rows: [
                        row({ items: [item(), item({ name: 'df', content: 'df -h' })] }),
                        row({ source: OPS_QBL, name: 'ops', buttons: 1, importable: 1, unsupported: 0, items: [item()] }),
                    ],
                }),
            ),
        });
        await openReview(host);

        // tail 在两套按钮里都有，只能写一次
        expect(screen.getByRole('button', { name: '确认导入 2 条' })).toBeInTheDocument();
    });

    it('取消勾选的条目不计入"将导入"', async () => {
        const { host } = makeHost();
        await openReview(host);

        expect(screen.getByRole('button', { name: '确认导入 2 条' })).toBeInTheDocument();
        fireEvent.click(screen.getByTestId('import-item-0-1'));
        expect(screen.getByRole('button', { name: '确认导入 1 条' })).toBeInTheDocument();
        expect(screen.getByText(/共 3 条，将导入 1 条/)).toBeInTheDocument();
    });
});

describe('容错与结果', () => {
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

        expect(await screen.findByText('导入预览')).toBeInTheDocument();
        expect(screen.getByText('将导入')).toBeInTheDocument();
    });

    it('集合里的 items 为 null 时不会崩，只是没有可勾选的命令', async () => {
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({ rows: [row({ items: null as unknown as QuickCommandSetItem[] })] }),
            ),
        });
        await openReview(host);

        expect(screen.getByTestId('import-set-0')).toBeChecked();
        expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument();
    });

    it('展示导入结果与写入的分组', async () => {
        const { host } = makeHost({
            applyQuickCommandImport: vi.fn(async () => ({
                imported: 0,
                skippedExisting: 2,
                skippedUnsupported: 0,
                groups: [],
                warnings: [],
            })),
        });
        await openReview(host);
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        expect(await screen.findByText('导入结果')).toBeInTheDocument();
        expect(screen.getByText('已存在或重复（跳过）')).toBeInTheDocument();
        expect(screen.queryByText(/已写入分组/)).not.toBeInTheDocument();
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
        await openReview(host);
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('未执行导入就取消时，不发起任何写入', async () => {
        const { host, applyQuickCommandImport, analyzeQuickCommandImport } = makeHost();
        renderDialog(host);

        fireEvent.click(screen.getByRole('button', { name: '取消' }));

        expect(applyQuickCommandImport).not.toHaveBeenCalled();
        expect(analyzeQuickCommandImport).not.toHaveBeenCalled();
    });

    it('宿主对象在流程中途被重建时，不会把界面重置回来源步', async () => {
        const { host, applyQuickCommandImport } = makeHost();
        const { rerender } = await renderReady(host);

        fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
        await screen.findByText('导入预览');
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));
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
});
