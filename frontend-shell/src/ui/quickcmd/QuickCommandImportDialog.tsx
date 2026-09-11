import React, { useEffect, useRef, useState } from 'react';
import { useToast } from '../feedback/Toast';
import {
    ImportDialogShell,
    ImportSection,
    ImportStat,
    ImportStatGrid,
    ImportWarningList,
    importStyles,
} from '../common/ImportParts';
import type {
    QuickCommandHost,
    QuickCommandImportAnalysis,
    QuickCommandImportReport,
    QuickCommandImportSelection,
    XshellQuickButtonDir,
} from '../ports';

type Props = {
    isOpen: boolean;
    host: QuickCommandHost;
    /** 面板里已有的分组名，用于提示"将追加到已有分组"。 */
    existingGroups: string[];
    onClose: () => void;
};

type Step = 'source' | 'review' | 'report';

/** 一条命令的编辑状态。名称、内容、分组都可以在预览里改。 */
type ItemPlan = {
    name: string;
    content: string;
    /** 空表示跟随所属集合的分组。 */
    group: string;
    included: boolean;
    supported: boolean;
    skipReason: string;
    type: string;
};

/** 一个 .qbl 集合的编辑状态。 */
type SetPlan = {
    included: boolean;
    expanded: boolean;
    group: string;
    items: ItemPlan[];
};

/**
 * 集合的默认分组名。
 *
 * Xshell 的 .qbl 里没有集合的显示名（[Info] 只有 Version/Count/Expanded），文件名又
 * 常常是 commands 这种无意义的名字，所以分组名不能由后端猜。默认值取 Xshell 而不是
 * 文件名，是为了让最常见的单集合场景一按就得到一个像样的分组名。
 */
const DEFAULT_GROUP = 'Xshell';

/**
 * 判重键：分组 + 名称 + 内容，三者全同才算同一条命令。
 *
 * 与后端（pkg/xshellimport 的 commandKey、pkg/config 的 quickCommandKey）保持一致。
 * 这里重复一次是刻意的：预览里的"已存在"必须随用户在界面上的改动实时变化，而它取决于
 * 目标分组，所以不能用分析阶段算好的静态标记（那份标记是按默认分组、全选算出来的快照）。
 */
function commandKey(group: string, name: string, content: string): string {
    return `${group}\u0000${name}\u0000${content}`;
}

type Evaluated = {
    /** 每个集合里逐条命令的"是否已被占位"标记（与 items 同序）。 */
    bySource: Record<string, { existing: boolean[]; includeCount: number }>;
    willImport: number;
    existing: number;
    unsupported: number;
};

/**
 * 按当前分组与勾选算出"到底会写入哪些"。
 *
 * 必须是纯函数在渲染时现算，而不是用分析阶段的静态标记：用户在预览里改分组、改名、
 * 改内容都会改变某条命令是否已存在，静态标记会让"将导入 N 条"与实际写入数对不上。
 * 这里同时做批内去重，与后端同规则。
 */
function evaluatePlan(
    rows: QuickCommandImportAnalysis['rows'],
    plan: Record<string, SetPlan>,
    existingKeys: Set<string>,
): Evaluated {
    const seen = new Set(existingKeys);
    const bySource: Evaluated['bySource'] = {};
    let willImport = 0;
    let existing = 0;
    let unsupported = 0;

    for (const row of rows) {
        const set = plan[row.source];
        if (!set) continue;
        const setGroup = set.group.trim() || DEFAULT_GROUP;
        const flags: boolean[] = [];
        let includeCount = 0;

        set.items.forEach((item) => {
            if (!item.supported) {
                unsupported++;
                flags.push(false);
                return;
            }
            const active = set.included && item.included;
            const key = commandKey(item.group.trim() || setGroup, item.name.trim(), item.content.trim());
            const duplicate = active && seen.has(key);
            flags.push(duplicate);
            if (!active) return;
            if (duplicate) {
                existing++;
                return;
            }
            seen.add(key); // 批内去重：同一命令出现在多套按钮里时只算一条
            willImport++;
            includeCount++;
        });

        bySource[row.source] = { existing: flags, includeCount };
    }
    return { bySource, willImport, existing, unsupported };
}

/**
 * Xshell 快捷命令导入对话框。
 *
 * 粒度分三层，且每一层都能改：
 *  1. 集合级——勾选要导哪几套按钮、每套的默认分组；
 *  2. 命令级——逐条勾选，取消掉不想要的那几条；
 *  3. 字段级——逐条改名称与命令内容，逐条覆盖目标分组（空 = 跟随集合）。
 *
 * 之所以要这么细：Xshell 里按钮的名字常常很潦草、一套按钮里往往混着不同类别的命令，
 * 而导入后逐条改要重复打开编辑弹窗。放在写入之前一次做完，用户只需要在一个地方对齐。
 *
 * 不支持导入的类型（脚本、菜单等）仍然列出来但置灰并写明原因——比只在提示里给一个
 * 跳过总数清楚。
 */
const QuickCommandImportDialog: React.FC<Props> = ({ isOpen, host, existingGroups, onClose }) => {
    const toast = useToast();

    const [step, setStep] = useState<Step>('source');
    const [dirs, setDirs] = useState<XshellQuickButtonDir[]>([]);
    const [selectedPath, setSelectedPath] = useState('');
    const [analysis, setAnalysis] = useState<QuickCommandImportAnalysis | null>(null);
    const [plan, setPlan] = useState<Record<string, SetPlan>>({});
    /** 现有命令的判重键，用于实时判断"已存在"。 */
    const [existingKeys, setExistingKeys] = useState<Set<string>>(() => new Set());
    const [report, setReport] = useState<QuickCommandImportReport | null>(null);
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');

    const canAnalyze = typeof host.analyzeQuickCommandImport === 'function';
    const canApply = typeof host.applyQuickCommandImport === 'function';

    // host 的对象身份可能在流程中途变化（Wails 适配器由外层 useMemo 按 onExecute
    // 重建，而 onExecute 未必是稳定引用）。流程重置只应由 isOpen 触发：否则导入一完成
    // 就跳回来源步，用户永远看不到结果报告——实机上就是这样。
    const hostRef = useRef(host);
    useEffect(() => {
        hostRef.current = host;
    }, [host]);

    useEffect(() => {
        if (!isOpen) return;
        setStep('source');
        setSelectedPath('');
        setAnalysis(null);
        setReport(null);
        setPlan({});
        setError('');

        void (async () => {
            try {
                // 读一次现有命令：预览里的"已存在"要随分组/名称/内容的改动实时重算
                const commands = (await hostRef.current.storage.load()) ?? [];
                setExistingKeys(
                    new Set(commands.map((c) => commandKey(c.group || 'default', c.name, c.content))),
                );
            } catch {
                setExistingKeys(new Set());
            }
            try {
                const detect = hostRef.current.detectQuickButtonDirs;
                if (!detect) return;
                const found = await detect();
                setDirs(Array.isArray(found) ? found : []);
                // 默认选中最新版本的目录，用户点一下就能分析。
                if (found.length > 0) setSelectedPath(found[0].path);
            } catch (e: any) {
                setError(e?.toString?.() || '检测本机 Xshell 快捷按钮目录失败');
            }
        })();
    }, [isOpen]);

    if (!isOpen) return null;
    if (!canAnalyze) {
        return null; // 宿主未提供导入能力（如 sidecar），入口本就不该出现
    }

    const rows = analysis?.rows ?? [];
    const evaluated = evaluatePlan(rows, plan, existingKeys);

    const updateSet = (source: string, patch: Partial<SetPlan>) => {
        setPlan((prev) => (prev[source] ? { ...prev, [source]: { ...prev[source], ...patch } } : prev));
    };

    const updateItem = (source: string, index: number, patch: Partial<ItemPlan>) => {
        setPlan((prev) => {
            const set = prev[source];
            if (!set) return prev;
            const items = set.items.map((item, i) => (i === index ? { ...item, ...patch } : item));
            return { ...prev, [source]: { ...set, items } };
        });
    };

    /** 集合分组输入框的实时提示：与已有分组合并，还是与本批其它集合合并。 */
    const groupHint = (set: SetPlan, index: number): string => {
        const name = set.group.trim();
        if (!name) return '集合分组不能为空';
        const sameBatch = rows.some((other, i) => i !== index && (plan[other.source]?.group ?? '').trim() === name);
        if (sameBatch) return '将与本批其它集合合并到同一分组';
        if (existingGroups.includes(name)) return '将追加到已有分组';
        return '将新建分组';
    };

    const runAnalyze = async () => {
        if (!selectedPath) {
            setError('请先选择要导入的 Xshell 快捷按钮文件或目录');
            return;
        }
        setBusy('分析中...');
        setError('');
        try {
            const result = await host.analyzeQuickCommandImport!(selectedPath, DEFAULT_GROUP);
            setAnalysis(result);

            // 逐条以"可导入且默认勾选"为初值；分组留空表示跟随集合。
            const initial: Record<string, SetPlan> = {};
            for (const row of result.rows ?? []) {
                initial[row.source] = {
                    included: true,
                    expanded: true,
                    group: row.group || DEFAULT_GROUP,
                    items: (row.items ?? []).map((item) => ({
                        name: item.name,
                        content: item.content,
                        group: '',
                        included: item.supported,
                        supported: item.supported,
                        skipReason: item.skipReason ?? '',
                        type: item.type,
                    })),
                };
            }
            setPlan(initial);
            setStep('review');
        } catch (e: any) {
            setError(e?.toString?.() || '分析失败');
        } finally {
            setBusy('');
        }
    };

    const runImport = async () => {
        setBusy('导入中...');
        setError('');
        try {
            // 回传的条目就是用户最终的决定：取消的集合不带条目，取消的条目不出现在列表里，
            // 名称/内容/分组用的是界面上改过的值。
            const selections: QuickCommandImportSelection[] = rows.map((row) => {
                const set = plan[row.source];
                if (!set || !set.included) {
                    return { source: row.source, group: set?.group ?? row.group, items: [] };
                }
                return {
                    source: row.source,
                    group: set.group.trim() || DEFAULT_GROUP,
                    items: set.items
                        .filter((item) => item.included && item.supported)
                        .map((item) => ({ name: item.name, content: item.content, group: item.group.trim() })),
                };
            });

            const result = await host.applyQuickCommandImport!(selectedPath, selections, DEFAULT_GROUP);
            setReport(result);
            setStep('report');
            if (result.imported > 0) {
                toast.success(`已导入 ${result.imported} 条快捷命令`);
            } else {
                toast.warning('没有新增快捷命令');
            }
        } catch (e: any) {
            setError(e?.toString?.() || '导入失败');
        } finally {
            setBusy('');
        }
    };

    const pickFile = async () => {
        if (!host.selectImportFile) return;
        const path = await host.selectImportFile();
        if (path) {
            setSelectedPath(path);
            setError('');
        }
    };

    const pickDirectory = async () => {
        if (!host.selectImportDirectory) return;
        const path = await host.selectImportDirectory();
        if (path) {
            setSelectedPath(path);
            setError('');
        }
    };

    const pickedSets = Object.values(plan).filter((s) => s.included).length;

    return (
        <ImportDialogShell
            title="导入 Xshell 快捷命令"
            busy={busy}
            width={880}
            onClose={onClose}
            footer={
                step === 'report' ? (
                    <button style={importStyles.primaryButton} onClick={onClose}>完成</button>
                ) : (
                    <>
                        <button style={importStyles.cancelButton} onClick={onClose} disabled={!!busy}>取消</button>
                        {step === 'review' ? (
                            <>
                                <button style={importStyles.secondaryButton} onClick={() => setStep('source')} disabled={!!busy}>
                                    返回
                                </button>
                                <button
                                    style={importStyles.primaryButton}
                                    onClick={runImport}
                                    disabled={!!busy || !canApply || pickedSets === 0}
                                    title={pickedSets === 0 ? '至少要勾选一套按钮' : undefined}
                                >
                                    {busy || `确认导入${evaluated.willImport > 0 ? ` ${evaluated.willImport} 条` : ''}`}
                                </button>
                            </>
                        ) : (
                            <button style={importStyles.primaryButton} onClick={runAnalyze} disabled={!!busy || !selectedPath}>
                                {busy || '分析并预览'}
                            </button>
                        )}
                    </>
                )
            }
        >
            {step === 'source' && (
                <ImportSection title="导入来源">
                    {dirs.length > 0 && (
                        <>
                            <div style={importStyles.subTitle}>本机检测到的 Xshell 快捷按钮目录</div>
                            <div style={importStyles.sectionHint}>
                                直接选它即可，无需在 Xshell 里做任何导出操作。
                            </div>
                            {dirs.map((dir) => (
                                <label key={dir.path} style={importStyles.radioRow}>
                                    <input
                                        type="radio"
                                        name="qbl-dir"
                                        checked={selectedPath === dir.path}
                                        onChange={() => setSelectedPath(dir.path)}
                                    />
                                    <span style={importStyles.radioLabel}>
                                        Xshell {dir.version || '未知版本'}
                                        <span style={importStyles.muted}>
                                            {' '}· {dir.sets} 套按钮 / {dir.buttons} 条
                                        </span>
                                    </span>
                                    <span style={importStyles.pathText} title={dir.path}>{dir.path}</span>
                                </label>
                            ))}
                            <div style={importStyles.divider} />
                        </>
                    )}

                    <div style={importStyles.subTitle}>或从文件 / 目录导入</div>
                    <div style={importStyles.sectionHint}>
                        支持单个 .qbl 文件，或包含 .qbl 的目录（子目录也会一并扫描）。
                    </div>
                    <div style={importStyles.buttonRow}>
                        <button style={importStyles.secondaryButton} onClick={pickFile} disabled={!!busy}>
                            选择文件（.qbl）
                        </button>
                        <button style={importStyles.secondaryButton} onClick={pickDirectory} disabled={!!busy}>
                            选择目录
                        </button>
                    </div>

                    <div style={selectedPath ? importStyles.selectionLine : importStyles.selectionLineEmpty}>
                        {selectedPath ? (
                            <>
                                将从此处导入：
                                <span style={importStyles.selectionPath} title={selectedPath}>
                                    {selectedPath}
                                </span>
                            </>
                        ) : (
                            '尚未选择导入来源'
                        )}
                    </div>
                </ImportSection>
            )}

            {step === 'review' && analysis && (
                <ImportSection title="导入预览">
                    <ImportStatGrid>
                        <ImportStat label="将导入" value={evaluated.willImport} tone="ok" />
                        <ImportStat
                            label="已存在（跳过）"
                            value={evaluated.existing}
                            tone={evaluated.existing > 0 ? 'warn' : undefined}
                        />
                        <ImportStat label="类型不支持（跳过）" value={evaluated.unsupported} />
                    </ImportStatGrid>

                    <div style={styles.setList}>
                        {rows.map((row, setIndex) => {
                            const set = plan[row.source];
                            if (!set) return null;
                            const setEval = evaluated.bySource[row.source];
                            return (
                                <div
                                    key={row.source}
                                    style={set.included ? styles.setBlock : styles.setBlockOff}
                                    data-testid={`import-set-block-${setIndex}`}
                                >
                                    <div style={styles.setHead}>
                                        <label style={styles.setCheck}>
                                            <input
                                                type="checkbox"
                                                checked={set.included}
                                                onChange={(e) => updateSet(row.source, { included: e.target.checked })}
                                                aria-label={`导入 ${row.name} 这套按钮`}
                                                data-testid={`import-set-${setIndex}`}
                                            />
                                            <span style={styles.setName}>{row.name}</span>
                                            <span style={importStyles.muted}>
                                                {' '}· 共 {row.buttons} 条，将导入 {setEval?.includeCount ?? 0} 条
                                            </span>
                                        </label>

                                        <span style={styles.setActions}>
                                            <button
                                                style={styles.linkButton}
                                                onClick={() => updateSet(row.source, { items: set.items.map((i) => ({ ...i, included: i.supported })) })}
                                                data-testid={`import-select-all-${setIndex}`}
                                            >
                                                全选
                                            </button>
                                            <button
                                                style={styles.linkButton}
                                                onClick={() => updateSet(row.source, { items: set.items.map((i) => ({ ...i, included: false })) })}
                                                data-testid={`import-select-none-${setIndex}`}
                                            >
                                                全不选
                                            </button>
                                            <button
                                                style={styles.linkButton}
                                                onClick={() => updateSet(row.source, { expanded: !set.expanded })}
                                                data-testid={`import-set-toggle-${setIndex}`}
                                            >
                                                {set.expanded ? '收起' : '展开'}
                                            </button>
                                        </span>
                                    </div>

                                    <div style={styles.setMeta}>
                                        <label style={styles.setGroupField}>
                                            <span style={styles.fieldLabel}>集合分组</span>
                                            <input
                                                style={styles.input}
                                                value={set.group}
                                                onChange={(e) => updateSet(row.source, { group: e.target.value })}
                                                aria-label={`${row.name} 的集合分组`}
                                                data-testid={`import-group-${setIndex}`}
                                            />
                                        </label>
                                        <span style={styles.setHint} data-testid={`import-group-hint-${setIndex}`}>
                                            {groupHint(set, setIndex)}
                                        </span>
                                    </div>

                                    {set.expanded && (
                                        <div style={styles.itemList}>
                                            <div style={styles.itemHead}>
                                                <span />
                                                <span>名称</span>
                                                <span>命令内容</span>
                                                <span>分组（空＝跟随集合）</span>
                                            </div>
                                            {set.items.map((item, itemIndex) =>
                                                item.supported ? (
                                                    <div key={`${item.name}-${itemIndex}`} style={styles.itemRow}>
                                                        <input
                                                            type="checkbox"
                                                            checked={set.included && item.included}
                                                            disabled={!set.included}
                                                            onChange={(e) => updateItem(row.source, itemIndex, { included: e.target.checked })}
                                                            aria-label={`导入 ${item.name}`}
                                                            data-testid={`import-item-${setIndex}-${itemIndex}`}
                                                        />
                                                        <span style={styles.nameCell}>
                                                            <input
                                                                style={styles.itemName}
                                                                value={item.name}
                                                                onChange={(e) => updateItem(row.source, itemIndex, { name: e.target.value })}
                                                                aria-label={`${item.name} 的名称`}
                                                                data-testid={`import-item-name-${setIndex}-${itemIndex}`}
                                                            />
                                                            {setEval?.existing[itemIndex] && (
                                                                <span
                                                                    style={styles.tag}
                                                                    title="目标分组里已有同名同内容的命令，导入时会跳过"
                                                                    data-testid={`import-item-existing-${setIndex}-${itemIndex}`}
                                                                >
                                                                    已存在
                                                                </span>
                                                            )}
                                                        </span>
                                                        <input
                                                            style={styles.itemContent}
                                                            value={item.content}
                                                            onChange={(e) => updateItem(row.source, itemIndex, { content: e.target.value })}
                                                            aria-label={`${item.name} 的命令内容`}
                                                            data-testid={`import-item-content-${setIndex}-${itemIndex}`}
                                                        />
                                                        <input
                                                            style={styles.itemGroup}
                                                            value={item.group}
                                                            placeholder="跟随集合"
                                                            onChange={(e) => updateItem(row.source, itemIndex, { group: e.target.value })}
                                                            aria-label={`${item.name} 的命令分组`}
                                                            data-testid={`import-item-group-${setIndex}-${itemIndex}`}
                                                        />
                                                    </div>
                                                ) : (
                                                    <div
                                                        key={`${item.name}-${itemIndex}`}
                                                        style={styles.itemRowOff}
                                                        data-testid={`import-item-unsupported-${setIndex}-${itemIndex}`}
                                                    >
                                                        <input type="checkbox" checked={false} disabled readOnly aria-hidden />
                                                        <span style={styles.itemNameOff}>{item.name || '（无名称）'}</span>
                                                        <span style={styles.itemSkipReason} title={item.skipReason}>
                                                            {item.skipReason}
                                                        </span>
                                                    </div>
                                                ),
                                            )}
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>

                    <div style={styles.noteBox}>
                        名称、命令内容与分组都可以直接改；类型不是「发送字符串」的按钮不可勾选，原因见置灰行。
                    </div>

                    <ImportWarningList warnings={analysis.warnings} />
                </ImportSection>
            )}

            {step === 'report' && report && (
                <ImportSection title="导入结果">
                    <ImportStatGrid>
                        <ImportStat label="已导入" value={report.imported} tone="ok" />
                        <ImportStat label="已存在或重复（跳过）" value={report.skippedExisting} />
                        <ImportStat label="不可导入（跳过）" value={report.skippedUnsupported} />
                    </ImportStatGrid>
                    <WrittenGroups groups={report.groups} />
                    <ImportWarningList warnings={report.warnings} />
                </ImportSection>
            )}

            {error && <div style={importStyles.error}>{error}</div>}
        </ImportDialogShell>
    );
};

/** groups 允许为 null：与 warnings 同理，边界异常不应升级为整页白屏。 */
const WrittenGroups: React.FC<{ groups: string[] | null | undefined }> = ({ groups }) => {
    const list = Array.isArray(groups) ? groups : [];
    if (list.length === 0) return null;
    return <div style={styles.groupLine}>已写入分组：{list.join('、')}</div>;
};

// 本对话框特有的样式；公共部分（外壳、分区、数字网格、提示列表）在 ui/common/ImportParts。
const styles: Record<string, React.CSSProperties> = {
    setList: { marginTop: 10, display: 'flex', flexDirection: 'column', gap: 10 },
    setBlock: {
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '8px 10px',
        backgroundColor: 'var(--bg-primary)',
    },
    setBlockOff: {
        border: '1px dashed var(--border)',
        borderRadius: 6,
        padding: '8px 10px',
        backgroundColor: 'transparent',
        opacity: 0.7,
    },
    setHead: { display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' },
    setCheck: { display: 'flex', alignItems: 'center', gap: 8, fontSize: 13, cursor: 'pointer', minWidth: 0 },
    setName: { fontWeight: 600 },
    setActions: { display: 'flex', gap: 10, flexShrink: 0 },
    setMeta: { display: 'flex', alignItems: 'center', gap: 8, marginTop: 6 },
    setGroupField: { display: 'flex', alignItems: 'center', gap: 6, flex: '0 0 auto' },
    setHint: { fontSize: 11, color: 'var(--text-muted)' },
    linkButton: {
        background: 'transparent',
        border: 'none',
        color: 'var(--accent)',
        fontSize: 11,
        cursor: 'pointer',
        padding: 0,
    },
    fieldLabel: { fontSize: 11, color: 'var(--text-secondary)', flexShrink: 0 },
    input: {
        padding: '5px 8px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
        width: 150,
    },
    itemList: { marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 },
    // 列宽：勾选框 / 名称（含"已存在"标记）/ 命令内容 / 分组
    itemHead: {
        display: 'grid',
        gridTemplateColumns: '22px 190px 1fr 140px',
        gap: 6,
        alignItems: 'center',
        fontSize: 10,
        color: 'var(--text-muted)',
        paddingBottom: 2,
    },
    itemRow: {
        display: 'grid',
        gridTemplateColumns: '22px 190px 1fr 140px',
        gap: 6,
        alignItems: 'center',
    },
    itemRowOff: {
        display: 'grid',
        gridTemplateColumns: '22px 190px 1fr 140px',
        gap: 6,
        alignItems: 'center',
        fontSize: 12,
        color: 'var(--text-disabled)',
    },
    nameCell: { display: 'flex', alignItems: 'center', gap: 4, minWidth: 0 },
    itemName: {
        padding: '4px 6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
        width: '100%',
        minWidth: 0,
    },
    itemContent: {
        padding: '4px 6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
        fontFamily: 'monospace',
        width: '100%',
        minWidth: 0,
    },
    itemGroup: {
        padding: '4px 6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
        width: '100%',
        minWidth: 0,
    },
    itemNameOff: { textDecoration: 'line-through' },
    itemSkipReason: { gridColumn: '3 / 5', fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
    tag: {
        flexShrink: 0,
        fontSize: 10,
        padding: '1px 4px',
        borderRadius: 3,
        border: '1px solid var(--warning)',
        color: 'var(--warning)',
        whiteSpace: 'nowrap',
    },
    noteBox: {
        marginTop: 10,
        fontSize: 12,
        lineHeight: 1.6,
        color: 'var(--text-secondary)',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '8px 10px',
    },
    groupLine: { marginTop: 8, fontSize: 12, color: 'var(--text-secondary)' },
};

export default QuickCommandImportDialog;
