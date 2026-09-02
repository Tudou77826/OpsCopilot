import React, { useEffect, useRef, useState } from 'react';
import { colors, radius, font } from '../settings/settingsStyles';

/**
 * AI 故障诊断面板（迭代 C）。
 *
 * 事件契约（中性形状，方案遗留 R7 约束）：宿主只提供
 * {runId, kind: status|context|done|canceled|error, stage?, message?,
 *   usedTokens?, maxTokens?, result?}；
 * 宿主内部 Agent 的工具调用轨迹不进入共享 UI。
 * result 为 JSON 字符串，规整后恒含 summary/steps/commands（pkg/ai 契约）。
 */
export interface DiagnoseEvent {
    runId: string;
    kind: 'status' | 'context' | 'token' | 'done' | 'concl-done' | 'canceled' | 'error';
    stage?: string;
    message?: string;
    usedTokens?: string;
    maxTokens?: string;
    result?: string;
    /** token 事件的增量文本 / concl-done 的结案全文。 */
    text?: string;
    /** done 事件携带绑定的案例 ID（未绑定为空）。 */
    caseId?: string;
}

export interface DiagnoseStartOptions {
    terminalId?: string;
    host?: string;
    user?: string;
}

export interface DiagnoseArchiveInput {
    service: string;
    module?: string;
    conclusion: string;
}

/** 案例能力（可选）：未提供时结束/归档区块不渲染。 */
export interface DiagnoseCaseRuntime {
    stop(rootCause: string, conclusion: string): Promise<void>;
    conclusion(rootCause: string): Promise<void>;
    archive(input: DiagnoseArchiveInput): Promise<{ filePath: string }>;
}

export interface DiagnoseRuntime {
    start(problem: string, opts?: DiagnoseStartOptions): Promise<{ runId: string; caseId?: string }>;
    cancel(runId: string): Promise<void>;
    onEvent(handler: (event: DiagnoseEvent) => void): () => void;
    /** 案例能力端口；缺省时面板隐藏结案/归档区块（能力边界纪律）。 */
    cases?: DiagnoseCaseRuntime;
}

interface DiagnoseResult {
    summary?: string;
    steps?: { step?: string; description?: string; [k: string]: unknown }[];
    commands?: { command?: string; description?: string; risk?: string }[];
}

const parseResult = (raw?: string): DiagnoseResult | null => {
    if (!raw) return null;
    try {
        const parsed = JSON.parse(raw) as DiagnoseResult;
        if (parsed && typeof parsed === 'object') return parsed;
    } catch { /* 非 JSON 结果按原文展示 */ }
    return null;
};

export interface DiagnosePanelProps {
    runtime: DiagnoseRuntime;
    /** 案例绑定信息（当前激活终端）；缺省不绑定案例。 */
    bindTo?: { terminalId: string; host?: string; user?: string };
}

const DiagnosePanel: React.FC<DiagnosePanelProps> = ({ runtime, bindTo }) => {
    const [problem, setProblem] = useState('');
    const [running, setRunning] = useState(false);
    const [statusText, setStatusText] = useState('');
    const [contextText, setContextText] = useState('');
    const [errorText, setErrorText] = useState('');
    const [canceled, setCanceled] = useState(false);
    const [result, setResult] = useState<DiagnoseResult | null>(null);
    const [resultRaw, setResultRaw] = useState('');
    const [elapsed, setElapsed] = useState(0);
    // 活跃/已结束 run 集合（诊断/结案可并行）。终态事件可能先于 start 响应到达
    //（服务端先返回 runId、事件 goroutine 可能跑赢响应）：已结束的 run 拒绝晚到的注册，
    // 避免复活进活跃集挡掉后续流式事件。
    const activeRunsRef = useRef<Set<string>>(new Set());
    const finishedRunsRef = useRef<Set<string>>(new Set());
    // C2：案例绑定与结案/归档
    const [caseId, setCaseId] = useState('');
    const [rootCause, setRootCause] = useState('');
    const [conclusion, setConclusion] = useState('');
    const [conclRunning, setConclRunning] = useState(false);
    const [caseStopped, setCaseStopped] = useState(false);
    const [archived, setArchived] = useState('');
    const [service, setService] = useState('');
    const [module_, setModule] = useState('');

    // 订阅中性事件；面板卸载/换 runtime 时退订。
    useEffect(() => {
        const off = runtime.onEvent((event) => {
            const known = activeRunsRef.current;
            const finished = finishedRunsRef.current;
            if (known.size > 0 && !known.has(event.runId) && !finished.has(event.runId)) return;
            const finish = () => { finished.add(event.runId); known.delete(event.runId); };
            switch (event.kind) {
                case 'status':
                    setStatusText(event.message ? `${event.stage ?? ''} ${event.message}`.trim() : (event.stage ?? ''));
                    break;
                case 'context':
                    setContextText(event.usedTokens ? `上下文 ${event.usedTokens} / ${event.maxTokens ?? '?'} tokens` : '');
                    break;
                case 'done':
                    finish();
                    setRunning(false);
                    setStatusText('诊断完成');
                    setResultRaw(event.result ?? '');
                    setResult(parseResult(event.result));
                    if (event.caseId) setCaseId(event.caseId);
                    if (!rootCause) {
                        const parsed = parseResult(event.result);
                        if (parsed?.summary) setRootCause(parsed.summary);
                    }
                    break;
                case 'token':
                    setConclusion((prev) => prev + (event.text ?? ''));
                    break;
                case 'concl-done':
                    finish();
                    setConclusion(event.text ?? '');
                    setConclRunning(false);
                    break;
                case 'canceled':
                    finish();
                    setRunning(false);
                    setCanceled(true);
                    setStatusText('已取消');
                    break;
                case 'error':
                    finish();
                    setRunning(false);
                    setConclRunning(false);
                    setErrorText(event.message ?? '诊断失败');
                    break;
            }
        });
        return off;
    }, [runtime]);

    // 运行计时
    useEffect(() => {
        if (!running) return;
        const started = Date.now();
        setElapsed(0);
        const timer = window.setInterval(() => setElapsed(Math.round((Date.now() - started) / 1000)), 500);
        return () => window.clearInterval(timer);
    }, [running]);

    const handleStart = async () => {
        const text = problem.trim();
        if (!text || running) return;
        setErrorText('');
        setCanceled(false);
        setResult(null);
        setResultRaw('');
        setCaseId('');
        setConclusion('');
        setCaseStopped(false);
        setArchived('');
        setStatusText('启动中…');
        setContextText('');
        setRunning(true);
        try {
            const started = await runtime.start(text, bindTo ? { terminalId: bindTo.terminalId, host: bindTo.host, user: bindTo.user } : undefined);
            const runId = started.runId;
            if (started.caseId) setCaseId(started.caseId);
            if (!finishedRunsRef.current.has(runId)) activeRunsRef.current.add(runId);
        } catch (e) {
            setRunning(false);
            setErrorText((e as Error)?.message || String(e));
        }
    };

    const handleCancel = async () => {
        const runId = [...activeRunsRef.current].find((id) => id.startsWith('diag-'));
        if (!runId) return;
        try { await runtime.cancel(runId); } catch { /* 取消失败时等待 error 事件 */ }
    };

    const handleConclusion = async () => {
        if (!runtime.cases || conclRunning || !rootCause.trim()) return;
        setConclRunning(true);
        setConclusion('');
        try {
            await runtime.cases.conclusion(rootCause.trim());
        } catch (e) {
            setConclRunning(false);
            setErrorText((e as Error)?.message || String(e));
        }
    };

    const handleStopCase = async () => {
        if (!runtime.cases || !caseId) return;
        try {
            await runtime.cases.stop(rootCause.trim(), conclusion);
            setCaseStopped(true);
            setStatusText('案例已保存，可归档到知识库');
        } catch (e) {
            setErrorText((e as Error)?.message || String(e));
        }
    };

    const handleArchive = async () => {
        if (!runtime.cases || !service.trim()) return;
        try {
            const { filePath } = await runtime.cases.archive({ service: service.trim(), module: module_.trim(), conclusion });
            setArchived(filePath);
        } catch (e) {
            setErrorText((e as Error)?.message || String(e));
        }
    };

    const parsed = result ?? parseResult(resultRaw);

    return (
        <div style={styles.panel}>
            <div style={styles.head}>
                <div style={styles.title}>AI 故障诊断</div>
                <div style={styles.meta}>
                    {running && <span style={styles.running}>● 运行中 {elapsed}s</span>}
                    {contextText && <span style={styles.muted}>{contextText}</span>}
                </div>
            </div>
            <textarea
                style={styles.input}
                value={problem}
                onChange={(e) => setProblem(e.target.value)}
                placeholder="描述故障现象，例如：磁盘打满，需要定位大文件"
                rows={3}
                disabled={running}
            />
            <div style={styles.actions}>
                <button
                    style={running ? { ...styles.primary, opacity: 0.55 } : styles.primary}
                    onClick={() => void handleStart()}
                    disabled={running || !problem.trim()}
                >
                    开始诊断
                </button>
                {running && (
                    <button style={styles.ghost} onClick={() => void handleCancel()}>
                        取消
                    </button>
                )}
            </div>
            {statusText && <div style={styles.status}>{statusText}</div>}
            {errorText && <div style={styles.error}>{errorText}</div>}
            {canceled && !errorText && <div style={styles.muted}>本次诊断已取消，未消耗完整 Agent 循环。</div>}

            {parsed && (
                <div style={styles.result}>
                    {parsed.summary && <div style={styles.summary}>{parsed.summary}</div>}
                    {(parsed.steps?.length ?? 0) > 0 && (
                        <div>
                            <div style={styles.sectionTitle}>排查步骤</div>
                            {parsed.steps!.map((step, i) => (
                                <div key={i} style={styles.step}>
                                    <span style={styles.stepIndex}>{i + 1}</span>
                                    <span>{String(step.step ?? step.description ?? '')}</span>
                                </div>
                            ))}
                        </div>
                    )}
                    {(parsed.commands?.length ?? 0) > 0 && (
                        <div>
                            <div style={styles.sectionTitle}>建议命令</div>
                            {parsed.commands!.map((cmd, i) => (
                                <div key={i} style={styles.cmdRow}>
                                    <code style={styles.cmd}>{cmd.command}</code>
                                    {cmd.description && <span style={styles.muted}> — {cmd.description}</span>}
                                    {cmd.risk && <span style={styles.risk}>{cmd.risk}</span>}
                                </div>
                            ))}
                        </div>
                    )}
                </div>
            )}

            {runtime.cases && parsed && (
                <div style={styles.caseBox}>
                    <div style={styles.sectionTitle}>结案与归档{caseId ? '（案例已绑定）' : '（未绑定案例）'}</div>
                    <textarea
                        style={styles.input}
                        value={rootCause}
                        onChange={(e) => setRootCause(e.target.value)}
                        placeholder="根因分析（可编辑，默认取诊断摘要）"
                        rows={2}
                        disabled={caseStopped}
                    />
                    <div style={styles.actions}>
                        <button
                            style={conclRunning ? { ...styles.ghost, opacity: 0.55 } : styles.ghost}
                            onClick={() => void handleConclusion()}
                            disabled={conclRunning || !rootCause.trim()}
                        >
                            {conclRunning ? '生成中…' : '生成结案报告'}
                        </button>
                        <button
                            style={styles.ghost}
                            onClick={() => void handleStopCase()}
                            disabled={caseStopped || !caseId || conclRunning}
                        >
                            {caseStopped ? '案例已保存' : '结束案例'}
                        </button>
                    </div>
                    {(conclusion || conclRunning) && (
                        <div style={styles.conclusion}>
                            <div style={styles.sectionTitle}>结案报告{conclRunning ? '（流式生成中…）' : ''}</div>
                            <div style={styles.conclusionBody}>{conclusion || '…'}</div>
                        </div>
                    )}
                    {caseStopped && !archived && (
                        <div style={styles.archiveRow}>
                            <input
                                style={styles.input}
                                value={service}
                                onChange={(e) => setService(e.target.value)}
                                placeholder="微服务名（必填）"
                            />
                            <input
                                style={styles.input}
                                value={module_}
                                onChange={(e) => setModule(e.target.value)}
                                placeholder="模块（可选）"
                            />
                            <button
                                style={styles.primary}
                                onClick={() => void handleArchive()}
                                disabled={!service.trim()}
                            >
                                归档到知识库
                            </button>
                        </div>
                    )}
                    {archived && (
                        <div style={styles.muted}>已归档：{archived}</div>
                    )}
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    panel: { display: 'flex', flexDirection: 'column', gap: '10px', padding: '12px', overflowY: 'auto', height: '100%' },
    head: { display: 'flex', justifyContent: 'space-between', alignItems: 'center' },
    title: { fontSize: font.lg, fontWeight: 600, color: colors.textPrimary },
    meta: { display: 'flex', gap: '10px', alignItems: 'center' },
    running: { color: colors.accent, fontSize: font.sm },
    muted: { color: colors.textTertiary, fontSize: font.sm },
    input: {
        width: '100%', padding: '8px 10px', resize: 'vertical',
        borderRadius: radius.sm, border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'var(--bg-primary)', color: colors.textPrimary,
        fontSize: font.base, outline: 'none', boxSizing: 'border-box',
    },
    actions: { display: 'flex', gap: '8px' },
    primary: {
        padding: '7px 16px', borderRadius: radius.sm, border: 'none',
        backgroundColor: colors.accent, color: 'var(--text-on-accent)',
        cursor: 'pointer', fontSize: font.base, fontWeight: 500,
    },
    ghost: {
        padding: '7px 14px', borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: 'transparent', color: colors.textSecondary, cursor: 'pointer', fontSize: font.base,
    },
    status: { fontSize: font.sm, color: colors.textSecondary },
    error: { fontSize: font.sm, color: colors.danger },
    result: {
        display: 'flex', flexDirection: 'column', gap: '10px',
        padding: '12px', borderRadius: radius.md,
        border: `1px solid ${colors.borderSubtle}`, backgroundColor: 'var(--bg-secondary)',
    },
    summary: { fontSize: font.base, color: colors.textPrimary, lineHeight: 1.6 },
    sectionTitle: { fontSize: font.sm, fontWeight: 600, color: colors.textSecondary, marginBottom: '6px' },
    step: { display: 'flex', gap: '8px', fontSize: font.sm, color: colors.textPrimary, marginBottom: '4px', lineHeight: 1.5 },
    stepIndex: {
        flexShrink: 0, width: '16px', height: '16px', borderRadius: '50%',
        backgroundColor: 'var(--bg-hover)', color: colors.textSecondary,
        fontSize: '10px', display: 'inline-flex', alignItems: 'center', justifyContent: 'center', marginTop: '2px',
    },
    cmdRow: { display: 'flex', gap: '6px', alignItems: 'baseline', marginBottom: '4px', flexWrap: 'wrap' },
    cmd: {
        padding: '2px 6px', borderRadius: radius.sm,
        backgroundColor: 'var(--bg-hover)', color: colors.textPrimary,
        fontSize: font.sm, fontFamily: 'var(--font-mono, monospace)',
    },
    risk: { fontSize: font.xs, color: colors.warning },
    caseBox: {
        display: 'flex', flexDirection: 'column', gap: '8px',
        padding: '12px', borderRadius: radius.md,
        border: `1px solid ${colors.borderSubtle}`, backgroundColor: 'var(--bg-secondary)',
    },
    conclusion: {
        padding: '10px', borderRadius: radius.sm,
        backgroundColor: 'var(--bg-hover)',
    },
    conclusionBody: { fontSize: font.sm, color: colors.textPrimary, lineHeight: 1.6, whiteSpace: 'pre-wrap' },
    archiveRow: { display: 'flex', gap: '6px', flexWrap: 'wrap' },
};

export default DiagnosePanel;
