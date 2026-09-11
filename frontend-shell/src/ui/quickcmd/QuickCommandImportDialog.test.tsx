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
type ExistingCommand = { name: string; content: string; group: string };

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

function makeHost(
    overrides: Partial<QuickCommandHost> = {},
    /** OpsCopilot 里现有的命令，面板的"现状"来自它。 */
    existing: ExistingCommand[] = [],
) {
    const dirs: XshellQuickButtonDir[] = [{ path: QBL_DIR, version: 8, sets: 1, buttons: 3 }];
    const detectQuickButtonDirs = vi.fn(async () => dirs);
    const selectImportFile = vi.fn(async () => '');
    const selectImportDirectory = vi.fn(async () => '');
    const load = vi.fn(async () => existing.map((c, i) => ({ id: String(i), ...c })));
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
        storage: { load, add: () => {}, update: () => {}, remove: () => {}, reorder: () => {} },
        detectQuickButtonDirs,
        selectImportFile,
        selectImportDirectory,
        analyzeQuickCommandImport,
        applyQuickCommandImport,
        ...overrides,
    };
    return { host, load, detectQuickButtonDirs, analyzeQuickCommandImport, applyQuickCommandImport };
}

function renderDialog(host: QuickCommandHost) {
    return render(
        <ToastProvider>
            <QuickCommandImportDialog isOpen host={host} onClose={() => {}} />
        </ToastProvider>,
    );
}

/** 渲染并等自动探测落地（分析按钮在此之前是禁用的）。 */
async function renderReady(host: QuickCommandHost) {
    const utils = renderDialog(host);
    await waitFor(() => expect(screen.getByText(/将从此处导入/)).toBeInTheDocument());
    return utils;
}

/** 进到预览步。 */
async function openReview(host: QuickCommandHost) {
    await renderReady(host);
    fireEvent.click(screen.getByRole('button', { name: '分析并预览' }));
    await screen.findByText('导入预览');
}

/** 分组选择器的当前显示值：输入态读 value，按钮态读文本（去掉 ▾）。 */
const pickerText = (testId: string) => {
    const el = screen.getByTestId(testId);
    if (el.tagName === 'INPUT') return (el as HTMLInputElement).value;
    return (el.textContent || '').replace('▾', '').trim();
};

/** 打开分组选择器并点选一项（菜单项是按钮）。 */
function chooseFromPicker(testId: string, label: string) {
    fireEvent.click(screen.getByTestId(testId));
    fireEvent.click(screen.getByRole('button', { name: label }));
}

/** 通过「＋ 新建分组…」把分组设成一个新名字。 */
function pickNewGroup(testId: string, name: string) {
    fireEvent.click(screen.getByTestId(testId));
    fireEvent.click(screen.getByRole('button', { name: '＋ 新建分组…' }));
    fireEvent.change(screen.getByTestId(testId), { target: { value: name } });
}

const readValue = (testId: string) => (screen.getByTestId(testId) as HTMLInputElement).value;

beforeEach(() => {
    vi.clearAllMocks();
});

describe('来源与现状', () => {
    it('自动探测本机目录并默认选中，无需用户做任何导出操作', async () => {
        const { host, detectQuickButtonDirs } = makeHost();
        renderDialog(host);

        await waitFor(() => expect(detectQuickButtonDirs).toHaveBeenCalledTimes(1));
        expect(await screen.findByText(/Xshell 8/)).toBeInTheDocument();
        expect(screen.getByText(/1 套按钮 \/ 3 条/)).toBeInTheDocument();
    });

    it('OpsCopilot 还没有命令时明确说明会新建分组', async () => {
        const { host } = makeHost();
        renderDialog(host);

        expect(await screen.findByTestId('import-library-empty')).toHaveTextContent('目前还没有快捷命令');
        expect(screen.getByText(/导入时会新建分组/)).toBeInTheDocument();
    });

    it('宿主未提供导入能力时整个面板不渲染（sidecar 外壳）', async () => {
        const { host } = makeHost();
        delete (host as any).analyzeQuickCommandImport;

        const { container } = renderDialog(host);
        await waitFor(() => expect(container).toBeEmptyDOMElement());
    });

    it('没探测到本机目录且未手选来源时，分析按钮禁用', async () => {
        const { host, analyzeQuickCommandImport } = makeHost({ detectQuickButtonDirs: vi.fn(async () => []) });
        renderDialog(host);

        expect(await screen.findByText('尚未选择导入来源')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '分析并预览' })).toBeDisabled();
        expect(analyzeQuickCommandImport).not.toHaveBeenCalled();
    });
});

describe('落点参考现状', () => {
    const opsLibrary: ExistingCommand[] = [
        { name: '看磁盘', content: 'df -h', group: '普通运维' },
        { name: '看日志', content: 'tail -f /var/log/app.log', group: '普通运维' },
        { name: '看 Pod', content: 'kubectl get pods', group: 'K8S运维' },
    ];

    it('同内容的命令已在某个分组时，默认就落到那个分组', async () => {
        const { host } = makeHost({}, opsLibrary);
        await openReview(host);

        // 两条可导入命令同内容都命中「普通运维」，集合默认分组因此是它，而不是凭空新建
        expect(pickerText('import-group-0')).toBe('普通运维');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将追加到已有分组');
    });

    it('没有同内容命令时保持默认的新分组名', async () => {
        const { host } = makeHost({}, [{ name: '别的', content: 'echo other', group: '普通运维' }]);
        await openReview(host);

        expect(pickerText('import-group-0')).toBe('Xshell');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');
    });

    it('标出"已在某分组"，因为同内容命令已经存在于那里', async () => {
        const { host } = makeHost({}, opsLibrary);
        await openReview(host);

        // tail 与「看日志」内容相同但名字不同：不算已存在，但要点明它在哪儿
        expect(screen.getByTestId('import-item-content-hit-0-0')).toHaveTextContent('已在 普通运维');
        expect(screen.queryByTestId('import-item-existing-0-0')).not.toBeInTheDocument();
    });

    it('名称与内容都相同时标为"已存在"，不计入将导入', async () => {
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: '普通运维' }]);
        await openReview(host);

        expect(screen.getByTestId('import-item-existing-0-0')).toHaveTextContent('已存在');
        expect(screen.queryByTestId('import-item-content-hit-0-0')).not.toBeInTheDocument();
        expect(screen.getByText(/共 3 条，将导入 1 条/)).toBeInTheDocument();
    });

    it('分组下拉列出现有分组，可直接选中', async () => {
        const { host } = makeHost({}, opsLibrary);
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-group-0'));
        // 菜单列出全部现有分组，当前值带勾
        expect(screen.getByRole('button', { name: '✓ 普通运维' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: 'K8S运维' })).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '＋ 新建分组…' })).toBeInTheDocument();

        fireEvent.click(screen.getByRole('button', { name: 'K8S运维' }));
        expect(pickerText('import-group-0')).toBe('K8S运维');
    });

    it('选「＋ 新建分组…」切到输入框，↩ 能回到下拉', async () => {
        const { host } = makeHost({}, opsLibrary);
        await openReview(host);

        pickNewGroup('import-group-0', '我的新分组');
        expect(screen.getByTestId('import-group-0').tagName).toBe('INPUT');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');

        fireEvent.click(screen.getByTestId('import-group-0-back'));
        expect(screen.getByTestId('import-group-0').tagName).toBe('BUTTON');
        expect(pickerText('import-group-0')).toBe('普通运维');
    });

    it('逐条分组默认跟随集合，也可选现有分组', async () => {
        const { host } = makeHost({}, opsLibrary);
        await openReview(host);

        expect(pickerText('import-item-group-0-0')).toBe('跟随集合');

        fireEvent.click(screen.getByTestId('import-item-group-0-0'));
        expect(screen.getByRole('button', { name: '✓ 跟随集合' })).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', { name: 'K8S运维' }));
        expect(pickerText('import-item-group-0-0')).toBe('K8S运维');
    });
});

describe('命令级明细', () => {
    it('预览逐条列出命令，含不可导入的条目及其原因', async () => {
        const { host } = makeHost();
        await openReview(host);

        expect(readValue('import-item-name-0-0')).toBe('tail');
        expect(readValue('import-item-content-0-0')).toBe('tail -f /var/log/app.log');
        expect(readValue('import-item-name-0-1')).toBe('df');

        const off = screen.getByTestId('import-item-unsupported-0-2');
        expect(off).toHaveTextContent('脚本按钮');
        expect(off).toHaveTextContent('类型为 2');
        expect(screen.queryByTestId('import-item-0-2')).not.toBeInTheDocument();
    });

    it('默认全部勾选，段头给出本套条数', async () => {
        const { host } = makeHost();
        await openReview(host);

        expect(screen.getByTestId('import-item-0-0')).toBeChecked();
        expect(screen.getByTestId('import-item-0-1')).toBeChecked();
        expect(screen.getByText(/共 3 条，将导入 2 条/)).toBeInTheDocument();
    });

    it('逐条取消、全选、全不选都会同步汇总与按钮文案', async () => {
        const { host } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-item-0-1'));
        expect(screen.getByRole('button', { name: /确认导入 1 条/ })).toBeInTheDocument();

        fireEvent.click(screen.getByTestId('import-select-none-0'));
        expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument();

        fireEvent.click(screen.getByTestId('import-select-all-0'));
        expect(screen.getByTestId('import-item-0-1')).toBeChecked();
        expect(screen.getByRole('button', { name: /确认导入 2 条/ })).toBeInTheDocument();
    });

    it('改名、改内容、逐条指定分组都会随请求回传', async () => {
        const { host, applyQuickCommandImport } = makeHost({}, [{ name: 'x', content: 'x', group: '日志排查' }]);
        await openReview(host);

        fireEvent.change(screen.getByTestId('import-item-name-0-0'), { target: { value: '跟踪应用日志' } });
        fireEvent.change(screen.getByTestId('import-item-content-0-0'), { target: { value: 'tail -F /var/log/new.log' } });
        chooseFromPicker('import-item-group-0-0', '日志排查');
        fireEvent.click(screen.getByTestId('import-item-0-1')); // 取消 df

        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));

        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));
        expect(applyQuickCommandImport).toHaveBeenCalledWith(
            QBL_DIR,
            [
                {
                    source: COMMANDS_QBL,
                    group: 'Xshell',
                    items: [{ name: '跟踪应用日志', content: 'tail -F /var/log/new.log', group: '日志排查' }],
                },
            ],
            'Xshell',
        );
        expect(await screen.findByText('导入结果')).toBeInTheDocument();
    });

    it('条数多的集合默认收起，展开后能看到逐条', async () => {
        const many = Array.from({ length: 30 }, (_, i) => item({ name: `批量任务 ${i + 1}`, content: `sh /opt/ops/task-${i + 1}.sh` }));
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    rows: [row({ name: 'batch-30', buttons: 30, importable: 30, unsupported: 0, items: many })],
                }),
            ),
        });
        await openReview(host);

        // 默认收起：30 条不该一次性铺开（否则后面的集合要翻很久才够得着）
        expect(screen.queryByTestId('import-item-0-0')).not.toBeInTheDocument();
        expect(screen.getByTestId('import-set-toggle-0')).toHaveTextContent('展开（30）');
        // 汇总与集合头的条数仍然可见
        expect(screen.getByText(/共 30 条，将导入 30 条/)).toBeInTheDocument();

        fireEvent.click(screen.getByTestId('import-set-toggle-0'));
        expect(screen.getByTestId('import-item-name-0-0')).toHaveValue('批量任务 1');
        expect(screen.getByTestId('import-set-toggle-0')).toHaveTextContent('收起');
    });

    it('收起/展开不影响已做的选择', async () => {
        const { host } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-set-toggle-0'));
        expect(screen.queryByTestId('import-item-0-0')).not.toBeInTheDocument();
        fireEvent.click(screen.getByTestId('import-set-toggle-0'));
        expect(screen.getByTestId('import-item-0-0')).toBeChecked();
    });
});

describe('集合级', () => {
    it('取消整个集合后它不参与导入，且禁止在一条都不选时确认', async () => {
        const { host, applyQuickCommandImport } = makeHost();
        await openReview(host);

        fireEvent.click(screen.getByTestId('import-set-0'));
        expect(screen.getByRole('button', { name: '确认导入' })).toBeDisabled();
        expect(applyQuickCommandImport).not.toHaveBeenCalled();
    });

    it('取消某套按钮时该套不出现在请求的条目里', async () => {
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
        fireEvent.click(screen.getByRole('button', { name: /确认导入/ }));
        await waitFor(() => expect(applyQuickCommandImport).toHaveBeenCalledTimes(1));

        const selections = applyQuickCommandImport.mock.calls[0][1];
        expect(selections).toHaveLength(2);
        expect(selections[1]).toMatchObject({ source: OPS_QBL, items: [] });
    });

    it('集合分组的三种提示：合并同批、追加已有、为空', async () => {
        const { host } = makeHost(
            {
                analyzeQuickCommandImport: vi.fn(async () =>
                    makeAnalysis({
                        rows: [
                            row(),
                            row({ source: OPS_QBL, name: 'ops', buttons: 1, importable: 1, unsupported: 0, items: [item({ name: 'free', content: 'free -h' })] }),
                        ],
                    }),
                ),
            },
            // 现有分组里有「容器命令」，下拉才选得到它；内容刻意不与被导入的命令重合，
            // 否则默认落点会被"同内容已在哪儿"接管
            [{ name: '别的', content: 'echo other', group: '容器命令' }],
        );
        await openReview(host);

        // 两套默认同名 → 合并提示
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将与本批其它集合合并到同一分组');

        chooseFromPicker('import-group-1', '容器命令');
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('将追加到已有分组');
        expect(screen.getByTestId('import-group-hint-0')).toHaveTextContent('将新建分组');

        // 切到新建但还没输入名字 → 提示不能为空
        fireEvent.click(screen.getByTestId('import-group-1'));
        fireEvent.click(screen.getByRole('button', { name: '＋ 新建分组…' }));
        expect(screen.getByTestId('import-group-1').tagName).toBe('INPUT');
        expect(screen.getByTestId('import-group-hint-1')).toHaveTextContent('集合分组不能为空');
    });
});

describe('空集合', () => {
    it('没有按钮的集合不显示列头与全选，直接给出说明', async () => {
        const { host } = makeHost({
            analyzeQuickCommandImport: vi.fn(async () =>
                makeAnalysis({
                    rows: [row({ name: 'empty', buttons: 0, importable: 0, unsupported: 0, items: [] })],
                }),
            ),
        });
        await openReview(host);

        expect(screen.getByTestId('import-set-empty-0')).toHaveTextContent('没有可导入的命令');
        // 没有可操作项时不给全选/全不选/展开，免得像一套正常按钮
        expect(screen.queryByTestId('import-select-all-0')).not.toBeInTheDocument();
        expect(screen.queryByTestId('import-set-toggle-0')).not.toBeInTheDocument();
        // 集合本身仍可勾选/取消，落点提示也在
        expect(screen.getByTestId('import-set-0')).toBeInTheDocument();
        expect(screen.getByTestId('import-group-hint-0')).toBeInTheDocument();
    });
});

describe('数字必须与实际写入一致', () => {
    it('改集合分组后"已存在"实时重算——换了分组就不再是重复', async () => {
        const { host } = makeHost({}, [{ name: 'tail', content: 'tail -f /var/log/app.log', group: 'Xshell' }]);
        await openReview(host);

        expect(screen.getByTestId('import-item-existing-0-0')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 1 条' })).toBeInTheDocument();

        // 落到一个新分组：tail 不再与已有命令重复，两条都可导入
        pickNewGroup('import-group-0', '全新分组');
        expect(screen.queryByTestId('import-item-existing-0-0')).not.toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入 2 条' })).toBeInTheDocument();
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
        // 两条命令分别已经住在不同分组：tail 在「日志排查」、df 在「普通运维」
        const { host } = makeHost({}, [
            { name: 'tail', content: 'tail -f /var/log/app.log', group: '日志排查' },
            { name: 'df', content: 'df -h', group: '普通运维' },
        ]);
        await openReview(host);

        // tail 与已有命令同名同内容、且默认落点已被引到「日志排查」→ 直接标为已存在
        expect(screen.getByTestId('import-item-existing-0-0')).toBeInTheDocument();

        // df 的默认落点是集合分组「日志排查」，那里没有它；把它指回它真正所在的分组才成重复
        expect(screen.queryByTestId('import-item-existing-0-1')).not.toBeInTheDocument();
        expect(screen.getByTestId('import-item-content-hit-0-1')).toHaveTextContent('已在 普通运维');

        chooseFromPicker('import-item-group-0-1', '普通运维');
        expect(screen.getByTestId('import-item-existing-0-1')).toBeInTheDocument();
        expect(screen.getByRole('button', { name: '确认导入' })).toBeInTheDocument();
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

describe('布局', () => {
    // jsdom 没有布局引擎，量不出"两个控件重叠"；这里钉住导致重叠的那个属性本身：
    // 本仓库没有全局 border-box，输入框声明 width:100% 时若用 content-box，实际宽度会
    // 再加上 padding 与边框，溢出所在栅格列并与右侧的分组选择器叠在一起（实机上发生过）。
    it('逐条命令的输入框声明 border-box，避免溢出所在列', async () => {
        const { host } = makeHost();
        await openReview(host);

        for (const testId of ['import-item-name-0-0', 'import-item-content-0-0']) {
            expect((screen.getByTestId(testId) as HTMLElement).style.boxSizing).toBe('border-box');
        }

        // 库里没有任何分组时集合分组直接是输入态
        expect((screen.getByTestId('import-group-0') as HTMLElement).style.boxSizing).toBe('border-box');
    });

    it('从菜单新建分组时，新分组输入框同样声明 border-box', async () => {
        const { host } = makeHost({}, [{ name: '看磁盘', content: 'df -h', group: '普通运维' }]);
        await openReview(host);

        pickNewGroup('import-group-0', '新名字');
        expect((screen.getByTestId('import-group-0') as HTMLElement).style.boxSizing).toBe('border-box');
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

    it('命令库读取失败时不崩，只是没有现状可参考', async () => {
        const { host } = makeHost({
            storage: {
                load: vi.fn(async () => {
                    throw new Error('读库失败');
                }),
                add: () => {},
                update: () => {},
                remove: () => {},
                reorder: () => {},
            },
        });
        renderDialog(host);

        expect(await screen.findByTestId('import-library-empty')).toBeInTheDocument();
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
                <QuickCommandImportDialog isOpen host={{ ...host }} onClose={() => {}} />
            </ToastProvider>,
        );

        expect(screen.getByText('导入结果')).toBeInTheDocument();
        expect(screen.queryByText('导入来源')).not.toBeInTheDocument();
    });
});
