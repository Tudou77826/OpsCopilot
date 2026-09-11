import React, { useEffect, useState } from 'react';
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
    QuickCommandSetRow,
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

/**
 * 导入的落点：一个分组名。
 *
 * Xshell 的 .qbl 里没有集合的显示名（[Info] 只有 Version/Count/Expanded），文件名
 * 又常常是 commands 这种无意义的名字，所以分组名不能由后端猜——放在界面上让用户确认。
 * 默认值取 Xshell 而不是文件名，是为了让最常见的单集合场景一按就得到一个像样的分组名。
 */
const DEFAULT_GROUP = 'Xshell';

/**
 * Xshell 快捷命令导入对话框。
 *
 * 设计要点：
 *  1. 同机场景下"自动检测本机 QuickButton Files 目录"是首选入口，用户不需要在
 *     Xshell 里做任何导出操作（.qbl 也没有导出向导，它就是个文件）。
 *  2. 先分析后导入：导入前就能看到每套按钮里有多少条能搬、多少条因类型不支持被跳过。
 *  3. 分组名逐个集合可改：填成同一个名字即合并，填不同即分开——一个机制同时表达
 *     "合并"和"分开"，只选一个集合时就只有一行。
 *  4. 有损映射要讲清楚：只搬名称与命令文本，Type≠1 的按钮（脚本/菜单等）会跳过。
 */
const QuickCommandImportDialog: React.FC<Props> = ({ isOpen, host, existingGroups, onClose }) => {
    const toast = useToast();

    const [step, setStep] = useState<Step>('source');
    const [dirs, setDirs] = useState<XshellQuickButtonDir[]>([]);
    const [selectedPath, setSelectedPath] = useState('');
    const [analysis, setAnalysis] = useState<QuickCommandImportAnalysis | null>(null);
    const [groups, setGroups] = useState<Record<string, string>>({});
    const [report, setReport] = useState<QuickCommandImportReport | null>(null);
    const [busy, setBusy] = useState('');
    const [error, setError] = useState('');

    const canAnalyze = typeof host.analyzeQuickCommandImport === 'function';
    const canApply = typeof host.applyQuickCommandImport === 'function';

    useEffect(() => {
        if (!isOpen) return;
        setStep('source');
        setSelectedPath('');
        setAnalysis(null);
        setReport(null);
        setGroups({});
        setError('');

        void (async () => {
            try {
                if (host.detectQuickButtonDirs) {
                    const found = await host.detectQuickButtonDirs();
                    setDirs(Array.isArray(found) ? found : []);
                    // 默认选中最新版本的目录，用户点一下就能分析。
                    if (found.length > 0) setSelectedPath(found[0].path);
                }
            } catch (e: any) {
                setError(e?.toString?.() || '检测本机 Xshell 快捷按钮目录失败');
            }
        })();
    }, [isOpen, host]);

    if (!isOpen) return null;
    if (!canAnalyze) {
        return null; // 宿主未提供导入能力（如 sidecar），入口本就不该出现
    }

    /** 分组输入框的实时提示：与已有分组合并，还是与本批其它集合合并。 */
    const groupHint = (row: QuickCommandSetRow, index: number): string => {
        const name = (groups[row.source] ?? row.group).trim();
        if (!name) return '分组名不能为空';
        const sameBatch = (analysis?.rows ?? []).some(
            (other, i) => i !== index && (groups[other.source] ?? other.group).trim() === name,
        );
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
            // 每行的分组输入框以建议值为初值；用户改过的值在重新分析后重置，
            // 因为集合本身可能已经变了（换了来源）。
            const initial: Record<string, string> = {};
            for (const row of result.rows ?? []) {
                initial[row.source] = row.group || DEFAULT_GROUP;
            }
            setGroups(initial);
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
            const assignments = Object.entries(groups).map(([source, group]) => ({ source, group: group.trim() }));
            const result = await host.applyQuickCommandImport!(selectedPath, assignments, DEFAULT_GROUP);
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

    const assignableRows = (analysis?.rows ?? []).length > 0;

    return (
        <ImportDialogShell
            title="导入 Xshell 快捷命令"
            busy={busy}
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
                                <button style={importStyles.primaryButton} onClick={runImport} disabled={!!busy || !canApply}>
                                    {busy || '确认导入'}
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
                        <ImportStat label="按钮集" value={analysis.sets} />
                        <ImportStat label="按钮总数" value={analysis.buttons} />
                        <ImportStat label="将导入" value={analysis.importable} tone="ok" />
                        <ImportStat label="类型不支持（跳过）" value={analysis.unsupported} />
                        <ImportStat label="已存在或重复（跳过）" value={analysis.existing} />
                    </ImportStatGrid>

                    {assignableRows && (
                        <div style={styles.assignList}>
                            {(analysis.rows ?? []).map((row, index) => (
                                <div key={row.source} style={styles.assignRow}>
                                    <div style={styles.assignMeta}>
                                        <span style={styles.assignName}>{row.name}</span>
                                        <span style={importStyles.muted}>
                                            {' '}· 可导入 {row.importable} / 共 {row.buttons} 条
                                        </span>
                                        <div style={styles.assignPath} title={row.source}>{row.source}</div>
                                    </div>
                                    <label style={styles.assignField}>
                                        <span style={styles.assignLabel}>导入到分组</span>
                                        <input
                                            style={styles.input}
                                            value={groups[row.source] ?? row.group}
                                            onChange={(e) => setGroups((prev) => ({ ...prev, [row.source]: e.target.value }))}
                                            aria-label={`${row.name} 的目标分组`}
                                            data-testid={`import-group-${index}`}
                                        />
                                        <span style={styles.assignHint} data-testid={`import-group-hint-${index}`}>
                                            {groupHint(row, index)}
                                        </span>
                                    </label>
                                </div>
                            ))}
                        </div>
                    )}

                    <div style={styles.noteBox}>
                        .qbl 里只有按钮名称与命令文本，因此只搬这两样。类型不是「发送字符串」的按钮
                        （脚本、菜单等）会被跳过并在下方列出原因。
                    </div>

                    <ImportWarningList warnings={analysis.warnings} />
                </ImportSection>
            )}

            {step === 'report' && report && (
                <ImportSection title="导入结果">
                    <ImportStatGrid>
                        <ImportStat label="已导入" value={report.imported} tone="ok" />
                        <ImportStat label="已存在或重复（跳过）" value={report.skippedExisting} />
                        <ImportStat label="类型不支持（跳过）" value={report.skippedUnsupported} />
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
    assignList: { marginTop: 10, display: 'flex', flexDirection: 'column', gap: 8 },
    assignRow: {
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        gap: 12,
        padding: '8px 10px',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: 6,
    },
    assignMeta: { minWidth: 0, flex: '1 1 auto' },
    assignName: { fontSize: 13, fontWeight: 600 },
    assignPath: {
        marginTop: 2,
        fontSize: 11,
        color: 'var(--text-muted)',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        direction: 'rtl',
    },
    assignField: { display: 'flex', flexDirection: 'column', gap: 2, flex: '0 0 220px' },
    assignLabel: { fontSize: 11, color: 'var(--text-secondary)' },
    assignHint: { fontSize: 11, color: 'var(--text-muted)' },
    input: {
        padding: '6px 8px',
        borderRadius: 4,
        border: '1px solid var(--border)',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        outline: 'none',
        fontSize: 12,
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
