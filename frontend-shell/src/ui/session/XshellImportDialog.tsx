import React, { useEffect, useState } from 'react';
import { useToast } from '../feedback/Toast';
import type {
    SessionManagerRuntime,
    XshellCredentialStatus,
    XshellImportAnalysis,
    XshellImportOptions,
    XshellImportReport,
    XshellSessionDir,
} from '../ports';

type Props = {
    isOpen: boolean;
    runtime: SessionManagerRuntime;
    onClose: () => void;
    /** 导入成功后通知宿主刷新会话树。 */
    onImported: () => void;
};

type Step = 'source' | 'review' | 'report';

/**
 * 导入行为固定为"解密密码 + 同机凭据"。
 *
 * 不向用户暴露开关：密码就是要带过来的（否则这个功能没有意义），而 Xshell 的密码
 * 密钥依赖导出机器的 Windows 账户标识，跨机器所需的源机器 SID / 主密码不在本功能的
 * 使用场景内。后端仍保留这两个入参，供将来确有需要时使用。
 */
const IMPORT_OPTIONS: XshellImportOptions = { decryptPassword: true };

/**
 * Xshell 导入对话框。
 *
 * 设计要点：
 *  1. 同机场景下"自动检测本机会话目录"是首选入口——用户不需要会任何 Xshell 操作。
 *  2. 导出引导独立成块，因为多数用户不知道 Xshell 的导出在哪；文案已按实际菜单核实，
 *     并刻意避开三个坑：不写"右键会话导出"、不给固定路径让用户粘贴、提醒导出文件含密码。
 *  3. 先分析后导入：写入前就能看到密码解开了多少条，而不是导完才发现是空的。
 */
const XshellImportDialog: React.FC<Props> = ({ isOpen, runtime, onClose, onImported }) => {
    const toast = useToast();

    const [step, setStep] = useState<Step>('source');
    const [status, setStatus] = useState<XshellCredentialStatus | null>(null);
    const [dirs, setDirs] = useState<XshellSessionDir[]>([]);
    const [selectedPath, setSelectedPath] = useState('');
    const [analysis, setAnalysis] = useState<XshellImportAnalysis | null>(null);
    const [report, setReport] = useState<XshellImportReport | null>(null);
    const [busy, setBusy] = useState('');
    const [guideOpen, setGuideOpen] = useState(false);
    const [error, setError] = useState('');

    const canAnalyze = typeof runtime.analyzeXshellImport === 'function';
    const canApply = typeof runtime.applyXshellImport === 'function';

    useEffect(() => {
        if (!isOpen) return;
        setStep('source');
        setSelectedPath('');
        setAnalysis(null);
        setReport(null);
        setError('');

        void (async () => {
            try {
                if (runtime.xshellImportStatus) setStatus(await runtime.xshellImportStatus());
                if (runtime.detectXshellDirs) {
                    const found = await runtime.detectXshellDirs();
                    setDirs(found);
                    // 默认选中最新版本的会话目录，用户点一下就能分析。
                    if (found.length > 0) setSelectedPath(found[0].path);
                }
            } catch (e: any) {
                setError(e?.toString?.() || '检测本机 Xshell 环境失败');
            }
        })();
    }, [isOpen, runtime]);

    if (!isOpen) return null;
    if (!canAnalyze) {
        return null; // 宿主未提供导入能力（如 sidecar），入口本就不该出现
    }

    const runAnalyze = async () => {
        if (!selectedPath) {
            setError('请先选择要导入的 Xshell 导出文件或会话目录');
            return;
        }
        setBusy('分析中...');
        setError('');
        try {
            const result = await runtime.analyzeXshellImport!(selectedPath, IMPORT_OPTIONS);
            setAnalysis(result);
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
            const result = await runtime.applyXshellImport!(selectedPath, IMPORT_OPTIONS);
            setReport(result);
            setStep('report');
            onImported();
            if (result.imported > 0) {
                toast.success(`已导入 ${result.imported} 个连接`);
            } else {
                toast.warning('没有新增连接');
            }
        } catch (e: any) {
            setError(e?.toString?.() || '导入失败');
        } finally {
            setBusy('');
        }
    };

    const pickFile = async () => {
        if (!runtime.selectImportFile) return;
        const path = await runtime.selectImportFile();
        if (path) {
            setSelectedPath(path);
            setError('');
        }
    };

    const pickDirectory = async () => {
        if (!runtime.selectImportDirectory) return;
        const path = await runtime.selectImportDirectory();
        if (path) {
            setSelectedPath(path);
            setError('');
        }
    };

    return (
        <div style={styles.overlay} onClick={(e) => e.target === e.currentTarget && !busy && onClose()}>
            <div style={styles.modal} onClick={(e) => e.stopPropagation()}>
                <div style={styles.header}>
                    <h2 style={styles.title}>导入 Xshell 会话</h2>
                    <button style={styles.closeButton} onClick={onClose} disabled={!!busy}>✕</button>
                </div>

                <div style={styles.body}>
                    {status && (
                        <div style={{ ...styles.statusBanner, ...(status.available ? styles.statusOk : styles.statusWarn) }}>
                            {status.message}
                            {status.available && status.maskedSid && (
                                <span style={styles.statusDetail}>（{status.windowsUser} / {status.maskedSid}）</span>
                            )}
                        </div>
                    )}

                    {step === 'source' && (
                        <>
                            {/* 两种来源放在同一块里，底部只给一行"将从此处导入"作为唯一的选择结论。
                                此前"已选择"挂在"或手动选择"下面，自动探测到的路径显示在那里会读成
                                "用户手动选的"，同一个路径也因此出现两次。 */}
                            <section style={styles.section}>
                                <div style={styles.sectionTitle}>导入来源</div>

                                {dirs.length > 0 && (
                                    <>
                                        <div style={styles.subTitle}>本机检测到的 Xshell 会话目录</div>
                                        <div style={styles.sectionHint}>
                                            直接选它即可，无需先去 Xshell 做任何导出操作。
                                        </div>
                                        {dirs.map((dir) => (
                                            <label key={dir.path} style={styles.radioRow}>
                                                <input
                                                    type="radio"
                                                    name="xsh-dir"
                                                    checked={selectedPath === dir.path}
                                                    onChange={() => setSelectedPath(dir.path)}
                                                />
                                                <span style={styles.radioLabel}>
                                                    Xshell {dir.version || '未知版本'}
                                                    <span style={styles.muted}> · {dir.sessions} 个会话</span>
                                                </span>
                                                <span style={styles.pathText} title={dir.path}>{dir.path}</span>
                                            </label>
                                        ))}
                                        <div style={styles.divider} />
                                    </>
                                )}

                                <div style={styles.subTitle}>或从文件 / 目录导入</div>
                                <div style={styles.sectionHint}>
                                    支持 Xshell 导出的 .xts 备份包、单个 .xsh 文件，或包含 .xsh 的目录。
                                </div>
                                <div style={styles.buttonRow}>
                                    <button style={styles.secondaryButton} onClick={pickFile} disabled={!!busy}>
                                        选择文件（.xts / .xsh）
                                    </button>
                                    <button style={styles.secondaryButton} onClick={pickDirectory} disabled={!!busy}>
                                        选择目录
                                    </button>
                                </div>

                                <div style={selectedPath ? styles.selectionLine : styles.selectionLineEmpty}>
                                    {selectedPath ? (
                                        <>
                                            将从此处导入：
                                            <span style={styles.selectionPath} title={selectedPath}>
                                                {selectedPath}
                                            </span>
                                        </>
                                    ) : (
                                        '尚未选择导入来源'
                                    )}
                                </div>
                            </section>

                            <GuidePanel open={guideOpen} onToggle={() => setGuideOpen((v) => !v)} />
                        </>
                    )}

                    {(step === 'review' || step === 'report') && (
                        <section style={styles.section}>
                            <div style={styles.sectionTitle}>
                                {step === 'review' ? '导入预览' : '导入结果'}
                            </div>
                            {step === 'review' && analysis && <AnalysisTable analysis={analysis} />}
                            {step === 'report' && report && <ReportTable report={report} />}
                        </section>
                    )}

                    {error && <div style={styles.error}>{error}</div>}
                </div>

                <div style={styles.footer}>
                    {step === 'report' ? (
                        <button style={styles.primaryButton} onClick={onClose}>完成</button>
                    ) : (
                        <>
                            <button style={styles.cancelButton} onClick={onClose} disabled={!!busy}>取消</button>
                            {step === 'review' ? (
                                <>
                                    <button style={styles.secondaryButton} onClick={() => setStep('source')} disabled={!!busy}>
                                        返回
                                    </button>
                                    <button style={styles.secondaryButton} onClick={runAnalyze} disabled={!!busy}>
                                        重新分析
                                    </button>
                                    <button style={styles.primaryButton} onClick={runImport} disabled={!!busy || !canApply}>
                                        {busy || '确认导入'}
                                    </button>
                                </>
                            ) : (
                                <button style={styles.primaryButton} onClick={runAnalyze} disabled={!!busy || !selectedPath}>
                                    {busy || '分析并预览'}
                                </button>
                            )}
                        </>
                    )}
                </div>
            </div>
        </div>
    );
};

const AnalysisTable: React.FC<{ analysis: XshellImportAnalysis }> = ({ analysis }) => {
    // 集合字段做容错：后端契约是空数组/空对象，但一旦传来 null，Object.keys 与
    // .length 会抛异常，而渲染异常会让 React 卸载整棵树（表现为整个应用黑屏）。
    // 这里的代价只是两个空值兜底，收益是任何一边出错都不会让应用不可用。
    const protocolEntries = Object.entries(analysis.protocols ?? {});
    return (
        <>
            <div style={styles.statGrid}>
                <Stat label="解析到会话" value={analysis.total} />
                <Stat label="将导入" value={analysis.supported} tone="ok" />
                <Stat label="已存在（跳过）" value={analysis.existing} />
                <Stat label="协议不支持（跳过）" value={analysis.unsupported} />
                <Stat label="分组层级" value={analysis.groups} />
                <Stat label="含密码" value={analysis.withPassword} />
                <Stat label="密码已解密" value={analysis.passwordDecrypted} tone="ok" />
                <Stat
                    label="密码未解密"
                    value={analysis.passwordFailed}
                    tone={analysis.passwordFailed > 0 ? 'warn' : undefined}
                />
            </div>
            {analysis.passwordFailed > 0 && (
                <div style={styles.hintBox}>
                    有 {analysis.passwordFailed} 个会话的密码无法解密。Xshell 的密码密钥依赖导出那台电脑的
                    Windows 账户标识，所以不是在本机导出的会话解不开。这些会话仍会被导入，但密码需要导入后手工补充。
                </div>
            )}
            {protocolEntries.length > 0 && (
                <div style={styles.protocolLine}>
                    协议分布：{protocolEntries.map(([p, n]) => `${p} ${n}`).join('，')}
                </div>
            )}
            <WarningList warnings={analysis.warnings} />
        </>
    );
};

const ReportTable: React.FC<{ report: XshellImportReport }> = ({ report }) => (
    <>
        <div style={styles.statGrid}>
            <Stat label="已导入" value={report.imported} tone="ok" />
            <Stat label="已存在（跳过）" value={report.skippedExisting} />
            <Stat label="不支持（跳过）" value={report.skippedUnsupported} />
            <Stat label="密码已解密" value={report.passwordDecrypted} tone="ok" />
            <Stat
                label="密码未解密"
                value={report.passwordFailed}
                tone={report.passwordFailed > 0 ? 'warn' : undefined}
            />
        </div>
        <WarningList warnings={report.warnings} />
    </>
);

/** warnings 允许为 null：后端契约应为空数组，但边界异常不应升级为整页白屏。 */
const WarningList: React.FC<{ warnings: string[] | null | undefined }> = ({ warnings }) => {
    const list = Array.isArray(warnings) ? warnings : [];
    if (list.length === 0) return null;
    return (
        <details style={styles.warningBox}>
            <summary style={styles.warningSummary}>{list.length} 条提示</summary>
            <ul style={styles.warningList}>
                {list.map((w, i) => (
                    <li key={i}>{w}</li>
                ))}
            </ul>
        </details>
    );
};

const Stat: React.FC<{ label: string; value: number; tone?: 'ok' | 'warn' }> = ({ label, value, tone }) => (
    <div style={styles.statCard}>
        <div style={{ ...styles.statValue, color: tone === 'ok' ? 'var(--success)' : tone === 'warn' ? 'var(--warning)' : 'var(--text-primary)' }}>
            {value}
        </div>
        <div style={styles.statLabel}>{label}</div>
    </div>
);

const GuidePanel: React.FC<{ open: boolean; onToggle: () => void }> = ({ open, onToggle }) => (
    <section style={styles.section}>
        <button style={styles.collapseHeader} onClick={onToggle}>
            {open ? '▾' : '▸'} 如何在 Xshell 里导出？（点这里看步骤）
        </button>
        {open && (
            <div style={styles.guideBody}>
                <ol style={styles.guideList}>
                    <li>打开 Xshell，点左上角的<b>「文件」</b>。</li>
                    <li>在下拉菜单里点<b>「导出」</b>（部分版本叫「导出会话」，效果一样）。</li>
                    <li>在弹出的「导出会话」窗口里，<b>勾选要导出的会话</b>（可以全选）。</li>
                    <li>点<b>「目标文件」</b>右边的按钮，把保存位置改到<b>桌面</b>这类好找的地方。这一步最容易卡住。</li>
                    <li>
                        <b>不要</b>勾选「清除密码 / 不导出密码」——勾了我们就拿不到密码。
                    </li>
                    <li>点<b>「下一步」</b>，再点<b>「完成」</b>。</li>
                    <li>到桌面找到生成的<b>单个 .xts 文件</b>，回到本窗口用「选择文件」选中它。</li>
                </ol>
                <div style={styles.guideFallback}>
                    找不到导出入口？在 Xshell 左侧会话列表里右键任意会话 →「在资源管理器中打开文件夹」，
                    那个目录可以直接用上面的「选择目录」导入，不需要导出。
                </div>
                <div style={styles.guideWarning}>
                    提示：导出文件里含有服务器密码（加密存储），请勿随意外发。
                </div>
            </div>
        )}
    </section>
);

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        inset: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1100,
    },
    modal: {
        backgroundColor: 'var(--bg-tertiary)',
        borderRadius: 8,
        width: 720,
        maxWidth: '92vw',
        maxHeight: '88vh',
        display: 'flex',
        flexDirection: 'column',
        color: 'var(--text-primary)',
        boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
    },
    header: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        padding: '16px 20px 8px',
    },
    title: { margin: 0, fontSize: '1.15rem' },
    closeButton: {
        background: 'transparent',
        border: 'none',
        color: 'var(--text-muted)',
        fontSize: 16,
        cursor: 'pointer',
    },
    body: { overflowY: 'auto', padding: '0 20px', display: 'flex', flexDirection: 'column', gap: 14 },
    statusBanner: { padding: '10px 12px', borderRadius: 6, fontSize: 13 },
    statusOk: { backgroundColor: 'rgba(64, 160, 96, 0.12)', color: 'var(--text-primary)', border: '1px solid var(--success)' },
    statusWarn: { backgroundColor: 'rgba(200, 150, 60, 0.12)', color: 'var(--text-primary)', border: '1px solid var(--warning)' },
    statusDetail: { color: 'var(--text-muted)', marginLeft: 6 },
    section: {
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '12px 14px',
        backgroundColor: 'var(--bg-secondary)',
    },
    sectionTitle: { fontSize: 13, fontWeight: 600, marginBottom: 6 },
    subTitle: { fontSize: 12, fontWeight: 600, color: 'var(--text-primary)' },
    divider: { height: 1, backgroundColor: 'var(--border)', margin: '10px 0' },
    sectionHint: { fontSize: 12, color: 'var(--text-muted)', marginBottom: 8 },
    radioRow: { display: 'flex', alignItems: 'center', gap: 8, padding: '4px 0', fontSize: 13, cursor: 'pointer' },
    radioLabel: { flexShrink: 0 },
    pathText: {
        color: 'var(--text-muted)',
        fontSize: 11,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap',
        direction: 'rtl',
    },
    muted: { color: 'var(--text-muted)' },
    buttonRow: { display: 'flex', gap: 8, flexWrap: 'wrap' },
    // 全块唯一的"选择结论"：把两种来源收敛成一行，避免同一个路径在多处出现、
    // 让人误以为手动选择区也参与了选择。
    selectionLine: {
        marginTop: 10,
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
        fontSize: 12,
        color: 'var(--text-secondary)',
    },
    selectionLineEmpty: {
        marginTop: 10,
        paddingTop: 10,
        borderTop: '1px solid var(--border)',
        fontSize: 12,
        color: 'var(--text-muted)',
    },
    selectionPath: {
        marginLeft: 6,
        color: 'var(--text-primary)',
        wordBreak: 'break-all',
    },
    statGrid: { display: 'grid', gridTemplateColumns: 'repeat(auto-fit, minmax(120px, 1fr))', gap: 8 },
    statCard: {
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: 6,
        padding: '8px 10px',
    },
    statValue: { fontSize: 18, fontWeight: 600 },
    statLabel: { fontSize: 11, color: 'var(--text-muted)', marginTop: 2 },
    hintBox: {
        marginTop: 10,
        fontSize: 12,
        lineHeight: 1.6,
        color: 'var(--text-secondary)',
        backgroundColor: 'rgba(200, 150, 60, 0.10)',
        border: '1px solid var(--warning)',
        borderRadius: 6,
        padding: '8px 10px',
    },
    protocolLine: { marginTop: 8, fontSize: 12, color: 'var(--text-muted)' },
    warningBox: { marginTop: 8, fontSize: 12 },
    warningSummary: { cursor: 'pointer', color: 'var(--text-muted)' },
    warningList: { margin: '6px 0 0', paddingLeft: 18, color: 'var(--text-secondary)', lineHeight: 1.7 },
    collapseHeader: {
        background: 'transparent',
        border: 'none',
        color: 'var(--text-primary)',
        fontSize: 13,
        fontWeight: 600,
        cursor: 'pointer',
        padding: 0,
    },
    guideBody: { marginTop: 10, fontSize: 12, lineHeight: 1.7, color: 'var(--text-secondary)' },
    guideList: { margin: 0, paddingLeft: 20 },
    guideFallback: { marginTop: 8, padding: '8px 10px', backgroundColor: 'var(--bg-primary)', borderRadius: 6 },
    guideWarning: { marginTop: 8, color: 'var(--warning)' },
    error: { color: 'var(--danger)', fontSize: 12 },
    footer: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: 10,
        padding: '12px 20px 16px',
    },
    cancelButton: {
        padding: '8px 16px',
        borderRadius: 6,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    secondaryButton: {
        padding: '8px 16px',
        borderRadius: 6,
        border: '1px solid var(--border-strong)',
        backgroundColor: 'var(--bg-elevated)',
        color: 'var(--text-primary)',
        cursor: 'pointer',
    },
    primaryButton: {
        padding: '8px 18px',
        borderRadius: 6,
        border: 'none',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        cursor: 'pointer',
        fontWeight: 600,
    },
};

export default XshellImportDialog;
