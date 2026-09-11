import React, { useEffect, useRef, useState } from 'react';
import { useToast } from '../feedback/Toast';
import FileContextMenu from '../filetransfer/FileContextMenu';
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
    QuickCommandSetItem,
    XshellQuickButtonDir,
} from '../ports';

type Props = {
    isOpen: boolean;
    host: QuickCommandHost;
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

/** 现有命令库的快照，用于"落点参考现状"。 */
type Library = {
    total: number;
    /** 按条数从多到少。 */
    groups: { name: string; count: number }[];
    /** 全键（分组 + 名称 + 内容）命中 → 导入时会跳过。 */
    keys: Set<string>;
    /** 按命令内容索引，用于识别"同一条运维命令已经在哪儿"。 */
    byContent: Map<string, { name: string; group: string }[]>;
};

/**
 * 集合的默认分组名。
 *
 * Xshell 的 .qbl 里没有集合的显示名（[Info] 只有 Version/Count/Expanded），文件名又
 * 常常是 commands 这种无意义的名字，所以分组名不能由后端猜。没有任何可参考的现有分组
 * 时用它，是为了让最常见的单集合场景一按就得到一个像样的分组名。
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

function emptyLibrary(): Library {
    return { total: 0, groups: [], keys: new Set(), byContent: new Map() };
}

function buildLibrary(commands: { name: string; content: string; group?: string }[]): Library {
    const counts = new Map<string, number>();
    const keys = new Set<string>();
    const byContent = new Map<string, { name: string; group: string }[]>();

    for (const cmd of commands) {
        const group = cmd.group || 'default';
        counts.set(group, (counts.get(group) ?? 0) + 1);
        keys.add(commandKey(group, cmd.name, cmd.content));
        const list = byContent.get(cmd.content) ?? [];
        list.push({ name: cmd.name, group });
        byContent.set(cmd.content, list);
    }

    const groups = [...counts.entries()]
        .map(([name, count]) => ({ name, count }))
        .sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));

    return { total: commands.length, groups, keys, byContent };
}

/**
 * 按现有命令的内容给出建议落点。
 *
 * 这是"落点参考现状"的核心：导入的这批里如果有同内容的命令已经存在于某个分组，默认就
 * 落到那个分组。否则本该进「普通运维」的命令会被放进一个凭空新建的分组，同一件事散成两处。
 */
function suggestGroup(items: QuickCommandSetItem[], library: Library, fallback: string): string {
    const tally = new Map<string, number>();
    for (const item of items) {
        if (!item.supported) continue;
        const hit = library.byContent.get(item.content.trim())?.[0];
        if (!hit) continue;
        tally.set(hit.group, (tally.get(hit.group) ?? 0) + 1);
    }
    let best = '';
    let bestCount = 0;
    for (const [group, count] of tally) {
        if (count > bestCount) {
            best = group;
            bestCount = count;
        }
    }
    return best || fallback;
}

type Evaluated = {
    bySource: Record<
        string,
        {
            /** 与 items 同序：该条是否已被同分组同名的命令占位（导入会跳过）。 */
            existing: boolean[];
            /** 与 items 同序：同内容的命令已经存在的分组（用于提示"这条其实在哪儿"）。 */
            contentHit: (string | null)[];
            includeCount: number;
        }
    >;
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
    library: Library,
): Evaluated {
    const seen = new Set(library.keys);
    const bySource: Evaluated['bySource'] = {};
    let willImport = 0;
    let existing = 0;
    let unsupported = 0;

    for (const row of rows) {
        const set = plan[row.source];
        if (!set) continue;
        const setGroup = set.group.trim() || DEFAULT_GROUP;
        const flags: boolean[] = [];
        const hits: (string | null)[] = [];
        let includeCount = 0;

        set.items.forEach((item) => {
            if (!item.supported) {
                unsupported++;
                flags.push(false);
                hits.push(null);
                return;
            }
            const active = set.included && item.included;
            const key = commandKey(item.group.trim() || setGroup, item.name.trim(), item.content.trim());
            const duplicate = active && seen.has(key);
            flags.push(duplicate);

            const sibling = library.byContent.get(item.content.trim())?.[0];
            hits.push(!duplicate && active && sibling ? sibling.group : null);

            if (!active) return;
            if (duplicate) {
                existing++;
                return;
            }
            seen.add(key); // 批内去重：同一命令出现在多套按钮里时只算一条
            willImport++;
            includeCount++;
        });

        bySource[row.source] = { existing: flags, contentHit: hits, includeCount };
    }
    return { bySource, willImport, existing, unsupported };
}

type GroupPickerProps = {
    value: string;
    /** 现有分组（按条数从多到少）。 */
    groups: string[];
    /** 提供「跟随集合」选项——逐条分组用。 */
    allowFollow?: boolean;
    onChange: (next: string) => void;
    testId: string;
    ariaLabel: string;
    newPlaceholder: string;
    /** 固定宽度；不传则填满所在格。 */
    width?: number;
};

/**
 * 分组选择器：从现有分组里选、跟随集合、或新建。
 *
 * 之所以不是纯粹的文本框：用户看不到 OpsCopilot 里已有哪些分组，只能盲打，于是同一件事
 * 容易散成「K8S 运维」和「K8s 操作」两个分组。把现有分组列出来，选择本身就是看一眼现状。
 *
 * 用项目已有的 FileContextMenu（portal + 边缘翻转）而不是原生 <select>：原生下拉的弹出层
 * 由浏览器进程绘制，在这种"固定定位弹窗 + 面板过渡"的结构里会脱位到弹窗外面去。
 */
const GroupPicker: React.FC<GroupPickerProps> = ({
    value,
    groups,
    allowFollow,
    onChange,
    testId,
    ariaLabel,
    newPlaceholder,
    width,
}) => {
    // 只有显式点了「＋ 新建分组…」才进入输入态；否则始终可下拉，保证现有分组可达。
    // 库里一个分组都没有时直接进输入态——没有可选项，让用户少点两下。
    const [newMode, setNewMode] = useState(() => !allowFollow && groups.length === 0);
    const [menuAt, setMenuAt] = useState<{ x: number; y: number } | null>(null);
    const isCustom = value !== '' && !groups.includes(value);

    const display = allowFollow && value === '' ? '跟随集合' : value;

    return (
        <span style={{ ...styles.pickerWrap, ...(width ? { width } : null) }}>
            {newMode ? (
                <>
                    <input
                        style={styles.pickerInput}
                        value={value}
                        placeholder={newPlaceholder}
                        onChange={(e) => onChange(e.target.value)}
                        aria-label={ariaLabel}
                        data-testid={testId}
                    />
                    {groups.length > 0 && (
                        <button
                            style={styles.backLink}
                            title="从现有分组中选择"
                            onClick={() => {
                                setNewMode(false);
                                onChange(groups[0]);
                            }}
                            data-testid={`${testId}-back`}
                        >
                            ↩
                        </button>
                    )}
                </>
            ) : (
                <button
                    type="button"
                    style={styles.pickerButton}
                    title={display}
                    aria-label={ariaLabel}
                    data-testid={testId}
                    onClick={(e) => {
                        const rect = e.currentTarget.getBoundingClientRect();
                        setMenuAt({ x: rect.left, y: rect.bottom + 2 });
                    }}
                >
                    <span style={styles.pickerValue}>{display}</span>
                    <span style={styles.pickerCaret}>▾</span>
                </button>
            )}

            {menuAt && (
                <FileContextMenu
                    x={menuAt.x}
                    y={menuAt.y}
                    onClose={() => setMenuAt(null)}
                    items={[
                        ...(allowFollow
                            ? [{ label: value === '' ? '✓ 跟随集合' : '跟随集合', onClick: () => onChange('') }]
                            : []),
                        ...groups.map((g) => ({
                            label: g === value ? `✓ ${g}` : g,
                            onClick: () => onChange(g),
                        })),
                        {
                            label: '＋ 新建分组…',
                            onClick: () => {
                                setNewMode(true);
                                if (!isCustom) onChange('');
                            },
                        },
                    ]}
                />
            )}
        </span>
    );
};

/**
 * Xshell 快捷命令导入对话框。
 *
 * 粒度分三层，且每一层都能改：
 *  1. 集合级——勾选要导哪几套按钮、每套的默认分组；
 *  2. 命令级——逐条勾选，取消掉不想要的那几条；
 *  3. 字段级——逐条改名称与命令内容，逐条覆盖目标分组（空 = 跟随集合）。
 *
 * 落点不只看导入的东西，也看 OpsCopilot 现状：现有分组列在来源页与选择器里，"同一内容的
 * 命令已经存在于某个分组"会作为默认落点并标出来，避免同一件事散成多个分组。
 *
 * 不支持导入的类型（脚本、菜单等）仍然列出来但置灰并写明原因——比只在提示里给一个
 * 跳过总数清楚。
 */
const QuickCommandImportDialog: React.FC<Props> = ({ isOpen, host, onClose }) => {
    const toast = useToast();

    const [step, setStep] = useState<Step>('source');
    const [dirs, setDirs] = useState<XshellQuickButtonDir[]>([]);
    const [selectedPath, setSelectedPath] = useState('');
    const [analysis, setAnalysis] = useState<QuickCommandImportAnalysis | null>(null);
    const [plan, setPlan] = useState<Record<string, SetPlan>>({});
    const [library, setLibrary] = useState<Library>(emptyLibrary);
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
                // 读一次现有命令：落点建议与"已存在"判断都基于它
                const commands = (await hostRef.current.storage.load()) ?? [];
                setLibrary(buildLibrary(commands));
            } catch {
                setLibrary(emptyLibrary());
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
    const evaluated = evaluatePlan(rows, plan, library);
    const groupNames = library.groups.map((g) => g.name);

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
        if (groupNames.includes(name)) return '将追加到已有分组';
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

            // 每套按钮的默认分组参考现状：同内容的命令已经在哪个分组，就默认落到那里。
            const initial: Record<string, SetPlan> = {};
            for (const row of result.rows ?? []) {
                initial[row.source] = {
                    included: true,
                    expanded: true,
                    group: suggestGroup(row.items ?? [], library, row.group || DEFAULT_GROUP),
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
                <>
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

                        {/* 现有分组不在这里列：预览页的分组选择器本身就是那份清单，
                            列在这儿只会让人先背一遍再翻页。这里只说"库里有没有东西"。 */}
                        {library.total === 0 && (
                            <div style={styles.libraryEmpty} data-testid="import-library-empty">
                                OpsCopilot 目前还没有快捷命令，导入时会新建分组。
                            </div>
                        )}
                    </ImportSection>
                </>
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
                                            <GroupPicker
                                                value={set.group}
                                                groups={groupNames}
                                                onChange={(next) => updateSet(row.source, { group: next })}
                                                testId={`import-group-${setIndex}`}
                                                ariaLabel={`${row.name} 的集合分组`}
                                                newPlaceholder="新分组名"
                                                width={150}
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
                                                                    title="该分组里已有同名同内容的命令，导入时会跳过"
                                                                    data-testid={`import-item-existing-${setIndex}-${itemIndex}`}
                                                                >
                                                                    已存在
                                                                </span>
                                                            )}
                                                            {!setEval?.existing[itemIndex] && setEval?.contentHit[itemIndex] && (
                                                                <span
                                                                    style={styles.tagInfo}
                                                                    title={`OpsCopilot 的该分组里已有同内容的命令（名称可能不同），可取消勾选或改名称`}
                                                                    data-testid={`import-item-content-hit-${setIndex}-${itemIndex}`}
                                                                >
                                                                    已在{setEval.contentHit[itemIndex]}
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
                                                        <GroupPicker
                                                            value={item.group}
                                                            groups={groupNames}
                                                            allowFollow
                                                            onChange={(next) => updateItem(row.source, itemIndex, { group: next })}
                                                            testId={`import-item-group-${setIndex}-${itemIndex}`}
                                                            ariaLabel={`${item.name} 的命令分组`}
                                                            newPlaceholder="新分组名"
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
    pickerWrap: { display: 'inline-flex', alignItems: 'center', gap: 4, width: '100%', minWidth: 0 },
    pickerButton: {
        display: 'flex',
        alignItems: 'center',
        gap: 4,
        padding: '5px 6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        fontSize: 12,
        cursor: 'pointer',
        width: '100%',
        minWidth: 0,
        textAlign: 'left',
    },
    pickerValue: { flex: '1 1 auto', minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' },
    pickerCaret: { flexShrink: 0, color: 'var(--text-muted)', fontSize: 10 },
    pickerInput: {
        padding: '4px 6px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
        width: '100%',
        minWidth: 0,
        // 本仓库没有全局 border-box，输入框必须自己声明：否则 width:100% 会
        // 再加上 padding 与边框的宽度，溢出所在栅格列并与右侧控件重叠。
        boxSizing: 'border-box',
    },
    backLink: {
        background: 'transparent',
        border: 'none',
        color: 'var(--accent)',
        fontSize: 12,
        cursor: 'pointer',
        padding: 0,
        flexShrink: 0,
    },
    libraryEmpty: { marginTop: 6, fontSize: 12, color: 'var(--text-muted)' },
    itemList: { marginTop: 8, display: 'flex', flexDirection: 'column', gap: 4 },
    // 列宽：勾选框 / 名称（含标记）/ 命令内容 / 分组
    itemHead: {
        display: 'grid',
        gridTemplateColumns: '22px 210px 1fr 150px',
        gap: 6,
        alignItems: 'center',
        fontSize: 10,
        color: 'var(--text-muted)',
        paddingBottom: 2,
    },
    itemRow: {
        display: 'grid',
        gridTemplateColumns: '22px 210px 1fr 150px',
        gap: 6,
        alignItems: 'center',
    },
    itemRowOff: {
        display: 'grid',
        gridTemplateColumns: '22px 210px 1fr 150px',
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
        boxSizing: 'border-box',
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
        boxSizing: 'border-box',
        fontSize: 12,
        fontFamily: 'monospace',
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
    tagInfo: {
        flexShrink: 0,
        fontSize: 10,
        padding: '1px 4px',
        borderRadius: 3,
        border: '1px solid var(--border-strong)',
        color: 'var(--text-muted)',
        whiteSpace: 'nowrap',
        maxWidth: 96,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
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
