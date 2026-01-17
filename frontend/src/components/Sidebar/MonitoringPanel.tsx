import React, { useEffect, useMemo, useState } from 'react';
import Card from '../Monitoring/Card';
import StatTile from '../Monitoring/StatTile';
import CommandBlock from '../Monitoring/CommandBlock';
import Sparkline from '../Monitoring/Sparkline';
import { JavaProcess, Snapshot, TerminalSessionLite } from '../Monitoring/monitoringTypes';
import { extractVmVersionShort, parseJstatGcutilOnce, parseNumberLoose, parsePercent, toneForPercent } from '../Monitoring/parse';

interface MonitoringPanelProps {
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
}

const MonitoringPanel: React.FC<MonitoringPanelProps> = ({ activeTerminalId, terminals }) => {
    const defaultSessionId = useMemo(() => {
        if (activeTerminalId) return activeTerminalId;
        if (terminals.length > 0) return terminals[0].id;
        return '';
    }, [activeTerminalId, terminals]);

    const [sessionId, setSessionId] = useState<string>(defaultSessionId);
    const [processes, setProcesses] = useState<JavaProcess[]>([]);
    const [selectedPid, setSelectedPid] = useState<number | null>(null);
    const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
    const [loadingList, setLoadingList] = useState(false);
    const [loadingSnap, setLoadingSnap] = useState(false);
    const [error, setError] = useState('');
    const [showDetails, setShowDetails] = useState(false);
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [refreshIntervalSec, setRefreshIntervalSec] = useState(5);
    const [history, setHistory] = useState<Array<{
        t: number;
        cpu: number | null;
        mem: number | null;
        threads: number | null;
        fd: number | null;
        old: number | null;
        gct: number | null;
        gctDelta: number | null;
    }>>([]);

    useEffect(() => {
        setSessionId(defaultSessionId);
    }, [defaultSessionId]);

    useEffect(() => {
        setProcesses([]);
        setSelectedPid(null);
        setSnapshot(null);
        setError('');
        setHistory([]);
        if (!sessionId) return;
        void refreshProcessList(sessionId);
    }, [sessionId]);

    useEffect(() => {
        setHistory([]);
        if (!sessionId || !selectedPid) return;
        void refreshSnapshot(sessionId, selectedPid, true);
    }, [sessionId, selectedPid]);

    useEffect(() => {
        if (!autoRefresh) return;
        if (!sessionId || !selectedPid) return;
        const intervalMs = Math.max(2, refreshIntervalSec) * 1000;
        const id = window.setInterval(() => {
            refreshSnapshot(sessionId, selectedPid, false);
        }, intervalMs);
        return () => window.clearInterval(id);
    }, [autoRefresh, refreshIntervalSec, sessionId, selectedPid]);

    const refreshProcessList = async (sid: string) => {
        setLoadingList(true);
        setError('');
        try {
            // @ts-ignore
            const fn = window.go?.main?.App?.ListJavaProcesses;
            if (!fn) {
                setError('后端接口未就绪：ListJavaProcesses');
                return;
            }
            // @ts-ignore
            const resp = await window.go.main.App.ListJavaProcesses(sid);
            if (typeof resp === 'string' && resp.startsWith('Error:')) {
                setError(resp);
                return;
            }
            const list = JSON.parse(resp) as JavaProcess[];
            setProcesses(Array.isArray(list) ? list : []);
        } catch (e: any) {
            setError(e?.toString?.() || '加载失败');
        } finally {
            setLoadingList(false);
        }
    };

    const refreshSnapshot = async (sid: string, pid: number, resetHistory: boolean) => {
        setLoadingSnap(true);
        setError('');
        try {
            // @ts-ignore
            const fn = window.go?.main?.App?.GetJavaMonitorSnapshot;
            if (!fn) {
                setError('后端接口未就绪：GetJavaMonitorSnapshot');
                return;
            }
            // @ts-ignore
            const resp = await window.go.main.App.GetJavaMonitorSnapshot(sid, pid);
            if (typeof resp === 'string' && resp.startsWith('Error:')) {
                setError(resp);
                return;
            }
            const parsed = JSON.parse(resp) as Snapshot;
            setSnapshot(parsed);
            setShowDetails(false);

            setHistory(prev => {
                const nextPrev = resetHistory ? [] : prev.slice();
                const cpu = parsePercent(parsed.process.cpu);
                const mem = parsePercent(parsed.process.mem);
                const threads = parsed.process.threads != null ? Number(parsed.process.threads) : null;
                const fd = parsed.process.fd_count != null ? Number(parsed.process.fd_count) : null;
                const gc = parseJstatGcutilOnce(parsed.jvm.gcutil_once);
                const old = gc ? parseNumberLoose(gc.O) : null;
                const gct = gc ? parseNumberLoose(gc.GCT) : null;

                const last = nextPrev.length > 0 ? nextPrev[nextPrev.length - 1] : null;
                const gctDelta = (gct != null && last?.gct != null) ? Math.max(0, gct - last.gct) : null;
                const point = { t: Date.now(), cpu, mem, threads, fd, old, gct, gctDelta };

                const appended = [...nextPrev, point];
                const maxPoints = 60;
                if (appended.length > maxPoints) return appended.slice(appended.length - maxPoints);
                return appended;
            });
        } catch (e: any) {
            setError(e?.toString?.() || '刷新失败');
        } finally {
            setLoadingSnap(false);
        }
    };

    const selectPid = (pid: number) => {
        setSelectedPid(pid);
        setSnapshot(null);
        if (!sessionId) return;
        void refreshSnapshot(sessionId, pid, true);
    };

    const activeTitle = terminals.find(t => t.id === sessionId)?.title || '';
    const cpuPct = useMemo(() => parsePercent(snapshot?.process.cpu), [snapshot?.process.cpu]);
    const memPct = useMemo(() => parsePercent(snapshot?.process.mem), [snapshot?.process.mem]);
    const gcutil = useMemo(() => snapshot ? parseJstatGcutilOnce(snapshot.jvm.gcutil_once) : null, [snapshot]);
    const vmShort = useMemo(() => snapshot ? extractVmVersionShort(snapshot.jvm.vm_version) : '', [snapshot]);
    const lastGctDeltaMs = useMemo(() => {
        const last = history.length > 0 ? history[history.length - 1] : null;
        if (!last || last.gctDelta == null) return null;
        return Math.round(last.gctDelta * 1000);
    }, [history]);
    const jvmTools = useMemo(() => {
        if (!snapshot) return '';
        const parts: string[] = [];
        parts.push(snapshot.tools.jcmd ? 'jcmd ✓' : 'jcmd ×');
        parts.push(snapshot.tools.jstat ? 'jstat ✓' : 'jstat ×');
        return parts.join('  ');
    }, [snapshot]);

    const series = useMemo(() => {
        return {
            cpu: history.map(p => p.cpu),
            mem: history.map(p => p.mem),
            old: history.map(p => p.old),
            gctDeltaMs: history.map(p => (p.gctDelta == null ? null : p.gctDelta * 1000))
        };
    }, [history]);

    return (
        <div style={styles.container}>
            <div style={styles.headerRow}>
                <div style={styles.pageTitle}>
                    <div style={styles.pageTitleTop}>Java 监控</div>
                    <div style={styles.pageTitleSub}>以卡片形式展示关键状态（无新增依赖）</div>
                </div>
                <div style={styles.headerActions}>
                    <button
                        style={styles.secondaryButton}
                        onClick={() => sessionId && refreshProcessList(sessionId)}
                        disabled={!sessionId || loadingList}
                    >
                        {loadingList ? '加载中...' : '刷新进程'}
                    </button>
                </div>
            </div>

            <Card title="目标会话">
                <div style={styles.row}>
                    <select
                        value={sessionId}
                        onChange={(e) => setSessionId(e.target.value)}
                        style={styles.select}
                        disabled={terminals.length === 0}
                    >
                        {terminals.length === 0 ? (
                            <option value="">未发现已连接会话</option>
                        ) : (
                            terminals.map(t => (
                                <option key={t.id} value={t.id}>{t.title}</option>
                            ))
                        )}
                    </select>
                </div>
                <div style={styles.metaLine}>
                    <div style={styles.metaChip}>当前：{activeTitle || '-'}</div>
                    <div style={styles.metaChip}>工具：{jvmTools || '-'}</div>
                    {snapshot?.pid ? <div style={styles.metaChip}>PID：{snapshot.pid}</div> : <div style={styles.metaChip}>PID：-</div>}
                </div>
            </Card>

            {!sessionId && (
                <div style={styles.empty}>
                    请先建立 SSH 连接并选中一个终端会话。
                </div>
            )}

            {sessionId && (
                <Card
                    title="Java 进程"
                    right={<div style={styles.smallHint}>点击一个 PID 查看快照</div>}
                    style={{ minHeight: 0 }}
                >
                    {processes.length === 0 ? (
                        <div style={styles.smallHint}>未发现 java 进程（或权限不足）。</div>
                    ) : (
                        <div style={styles.procList} className="hide-scrollbar">
                            {processes.map((p: JavaProcess) => (
                                <div
                                    key={p.pid}
                                    style={{
                                        ...styles.procItem,
                                        backgroundColor: selectedPid === p.pid ? '#202020' : 'transparent',
                                        borderColor: selectedPid === p.pid ? '#007acc' : '#2a2a2a'
                                    }}
                                    onClick={() => selectPid(p.pid)}
                                    title={p.cmd}
                                >
                                    <div style={styles.procLeft}>
                                        <div style={styles.procPid}>PID {p.pid}</div>
                                        <div style={styles.procCmd}>{p.cmd}</div>
                                    </div>
                                    <div style={styles.procRight}>
                                        <div style={styles.procTag}>{p.user}</div>
                                        <div style={styles.procTag}>{p.etime}</div>
                                    </div>
                                </div>
                            ))}
                        </div>
                    )}
                </Card>
            )}

            {sessionId && selectedPid && (
                <Card
                    title={`快照（PID ${selectedPid}）`}
                    right={
                        <div style={styles.row}>
                            <label style={styles.toggle}>
                                <input
                                    type="checkbox"
                                    checked={autoRefresh}
                                    onChange={(e) => setAutoRefresh(e.target.checked)}
                                    style={styles.toggleInput}
                                />
                                <span style={styles.toggleText}>自动刷新</span>
                            </label>
                            <select
                                value={String(refreshIntervalSec)}
                                onChange={(e) => setRefreshIntervalSec(parseInt(e.target.value) || 5)}
                                style={styles.smallSelect}
                                disabled={!autoRefresh}
                            >
                                <option value="3">3s</option>
                                <option value="5">5s</option>
                                <option value="10">10s</option>
                                <option value="15">15s</option>
                            </select>
                            <button
                                style={styles.secondaryButton}
                                onClick={() => setShowDetails(v => !v)}
                                disabled={!snapshot}
                            >
                                {showDetails ? '隐藏详情' : '显示详情'}
                            </button>
                            <button
                                style={styles.primaryButton}
                                onClick={() => refreshSnapshot(sessionId, selectedPid, false)}
                                disabled={loadingSnap}
                            >
                                {loadingSnap ? '刷新中...' : '刷新快照'}
                            </button>
                        </div>
                    }
                    style={{ minHeight: 0 }}
                >
                    {!snapshot && <div style={styles.smallHint}>加载快照中...</div>}
                    {snapshot && (
                        <div style={styles.snapshotWrap}>
                            <div style={styles.tileGrid}>
                                <StatTile
                                    icon="⚙️"
                                    label="CPU"
                                    value={snapshot.process.cpu ? `${snapshot.process.cpu}%` : '-'}
                                    sub={snapshot.process.etime || '-'}
                                    tone={toneForPercent(cpuPct, 60, 85)}
                                />
                                <StatTile
                                    icon="🧠"
                                    label="内存"
                                    value={snapshot.process.mem ? `${snapshot.process.mem}%` : '-'}
                                    sub="进程占比"
                                    tone={toneForPercent(memPct, 60, 85)}
                                />
                                <StatTile
                                    icon="🧵"
                                    label="线程"
                                    value={snapshot.process.threads != null ? String(snapshot.process.threads) : '-'}
                                    sub="Threads"
                                    tone={(snapshot.process.threads ?? 0) >= 800 ? 'warn' : 'neutral'}
                                />
                                <StatTile
                                    icon="📁"
                                    label="FD"
                                    value={snapshot.process.fd_count != null ? String(snapshot.process.fd_count) : '-'}
                                    sub="文件描述符"
                                    tone={(snapshot.process.fd_count ?? 0) >= 4096 ? 'warn' : 'neutral'}
                                />
                                {gcutil && (
                                    <StatTile
                                        icon="📦"
                                        label="Old 使用"
                                        value={gcutil.O ? `${gcutil.O}%` : '-'}
                                        sub={`FGC ${gcutil.FGC || '-'}`}
                                        tone={toneForPercent(parsePercent(gcutil.O), 70, 85)}
                                    />
                                )}
                                <StatTile
                                    icon="🧯"
                                    label="GC 开销"
                                    value={lastGctDeltaMs != null ? `${lastGctDeltaMs}ms` : '-'}
                                    sub="最近一次间隔"
                                    tone={(lastGctDeltaMs ?? 0) >= 300 ? 'warn' : 'neutral'}
                                />
                            </div>

                            <Card
                                title="趋势图（最近 60 个点）"
                                right={<div style={styles.smallHint}>{vmShort || ''}</div>}
                                style={styles.innerCard}
                            >
                                <div style={styles.chartGrid}>
                                    <div style={styles.chartTile}>
                                        <div style={styles.chartTitle}>CPU %</div>
                                        <Sparkline data={series.cpu} min={0} max={100} stroke="#4da3ff" />
                                        <div style={styles.chartMeta}>{cpuPct != null ? `${cpuPct.toFixed(1)}%` : '-'}</div>
                                    </div>
                                    <div style={styles.chartTile}>
                                        <div style={styles.chartTitle}>内存 %</div>
                                        <Sparkline data={series.mem} min={0} max={100} stroke="#9bdb74" fill="rgba(155,219,116,0.16)" />
                                        <div style={styles.chartMeta}>{memPct != null ? `${memPct.toFixed(1)}%` : '-'}</div>
                                    </div>
                                    <div style={styles.chartTile}>
                                        <div style={styles.chartTitle}>Old 使用 % (jstat)</div>
                                        <Sparkline data={series.old} min={0} max={100} stroke="#ffbf66" fill="rgba(255,191,102,0.14)" />
                                        <div style={styles.chartMeta}>{gcutil?.O ? `${gcutil.O}%` : 'jstat 缺失/无数据'}</div>
                                    </div>
                                    <div style={styles.chartTile}>
                                        <div style={styles.chartTitle}>GC 开销 ms/间隔</div>
                                        <Sparkline data={series.gctDeltaMs} min={0} stroke="#ff7a7a" fill="rgba(255,122,122,0.12)" showArea={false} />
                                        <div style={styles.chartMeta}>{lastGctDeltaMs != null ? `${lastGctDeltaMs}ms` : '等待第 2 个点'}</div>
                                    </div>
                                </div>
                            </Card>

                            <div style={styles.cmdRow}>
                                <CommandBlock title="主机概览" result={snapshot.host.uptime} />
                                <CommandBlock title="内存信息" result={snapshot.host.meminfo} />
                            </div>

                            {showDetails && (
                                <div style={styles.detailsGrid}>
                                    <CommandBlock title="JVM 版本（原始输出）" result={snapshot.jvm.vm_version} defaultOpen />
                                    <CommandBlock title="堆与 GC 摘要（原始输出）" result={snapshot.jvm.heap_info} />
                                    <CommandBlock title="GC Util（原始输出）" result={snapshot.jvm.gcutil_once} />
                                    <CommandBlock title="进程命令行（原始输出）" result={{ command: 'ps/procfs', output: snapshot.process.cmd || '' }} />
                                </div>
                            )}
                        </div>
                    )}
                </Card>
            )}

            {error && (
                <div style={styles.error}>
                    {error}
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        height: 'auto',
        color: '#ddd',
        padding: '12px',
        gap: '12px',
        boxSizing: 'border-box',
        overflow: 'visible'
    },
    headerRow: {
        display: 'flex',
        alignItems: 'flex-start',
        justifyContent: 'space-between',
        gap: '12px'
    },
    pageTitle: {
        display: 'flex',
        flexDirection: 'column',
        gap: '2px'
    },
    pageTitleTop: {
        fontSize: '14px',
        fontWeight: 800,
        color: '#fff'
    },
    pageTitleSub: {
        fontSize: '12px',
        color: '#9a9a9a'
    },
    headerActions: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center'
    },
    row: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center'
    },
    select: {
        flex: 1,
        backgroundColor: '#333',
        color: '#ddd',
        border: '1px solid #444',
        borderRadius: '4px',
        padding: '6px 8px',
        outline: 'none'
    },
    smallSelect: {
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '4px',
        padding: '6px 8px',
        outline: 'none',
        fontSize: '12px'
    },
    toggle: {
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        padding: '6px 8px',
        border: '1px solid #2a2a2a',
        borderRadius: '6px',
        backgroundColor: '#141414',
        color: '#bbb',
        fontSize: '12px'
    },
    toggleInput: {
        margin: 0
    },
    toggleText: {
        color: '#bbb'
    },
    primaryButton: {
        padding: '6px 10px',
        backgroundColor: '#007acc',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        fontWeight: 600
    },
    secondaryButton: {
        padding: '6px 10px',
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px'
    },
    smallHint: {
        fontSize: '12px',
        color: '#8a8a8a'
    },
    empty: {
        padding: '10px',
        border: '1px dashed #444',
        borderRadius: '6px',
        color: '#aaa'
    },
    metaLine: {
        display: 'flex',
        flexWrap: 'wrap',
        gap: '8px',
        marginTop: '10px'
    },
    metaChip: {
        fontSize: '11px',
        color: '#a8a8a8',
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
        borderRadius: '999px',
        padding: '4px 8px'
    },
    procList: {
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        overflow: 'auto',
        maxHeight: '260px',
        paddingRight: '4px'
    },
    procItem: {
        border: '1px solid #2a2a2a',
        borderRadius: '10px',
        padding: '10px',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '10px'
    },
    procLeft: {
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        minWidth: 0
    },
    procRight: {
        display: 'flex',
        gap: '6px',
        alignItems: 'center',
        flexShrink: 0
    },
    procTag: {
        fontSize: '11px',
        color: '#a8a8a8',
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
        borderRadius: '999px',
        padding: '4px 8px'
    },
    procPid: { color: '#fff', fontWeight: 800, fontSize: '12px' },
    procCmd: {
        fontSize: '12px',
        color: '#ddd',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    snapshotWrap: {
        display: 'flex',
        flexDirection: 'column',
        gap: '12px',
        overflow: 'visible',
        minHeight: 0,
        paddingRight: '4px'
    },
    tileGrid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
        gap: '10px'
    },
    innerCard: {
        padding: 0
    },
    chartGrid: {
        display: 'grid',
        gridTemplateColumns: 'repeat(2, minmax(0, 1fr))',
        gap: '10px'
    },
    chartTile: {
        borderRadius: '10px',
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
        padding: '10px',
        display: 'flex',
        flexDirection: 'column',
        gap: '8px',
        minWidth: 0
    },
    chartTitle: {
        fontSize: '11px',
        color: '#a8a8a8'
    },
    chartMeta: {
        fontSize: '11px',
        color: '#8a8a8a',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    cmdRow: {
        display: 'grid',
        gridTemplateColumns: '1fr',
        gap: '10px'
    },
    detailsGrid: {
        display: 'grid',
        gridTemplateColumns: '1fr',
        gap: '10px'
    },
    error: {
        padding: '10px',
        borderRadius: '6px',
        backgroundColor: '#3a1f1f',
        border: '1px solid #6a2a2a',
        color: '#ffd0d0',
        fontSize: '12px',
        whiteSpace: 'pre-wrap'
    }
};

export default MonitoringPanel;

