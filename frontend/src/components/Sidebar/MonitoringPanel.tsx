import React, { useEffect, useMemo, useState } from 'react';
import Card from '../Monitoring/Card';
import StatTile from '../Monitoring/StatTile';
import CommandBlock from '../Monitoring/CommandBlock';
import Sparkline from '../Monitoring/Sparkline';
import { JavaProcess, Snapshot, TerminalSessionLite, ThreadStateCounts, TopThread } from '../Monitoring/monitoringTypes';
import { extractVmVersionShort, formatBytesFromKB, parseJstatGcutilOnce, parseNumberLoose, parsePercent, toneForPercent } from '../Monitoring/parse';

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
    const [processQuery, setProcessQuery] = useState('');
    const [selectedPid, setSelectedPid] = useState<number | null>(null);
    const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
    const [loadingList, setLoadingList] = useState(false);
    const [loadingSnap, setLoadingSnap] = useState(false);
    const [error, setError] = useState('');
    const [autoRefresh, setAutoRefresh] = useState(true);
    const [refreshIntervalSec, setRefreshIntervalSec] = useState(5);
    const [timeRangeMin, setTimeRangeMin] = useState(15);
    const [history, setHistory] = useState<Array<{
        t: number;
        cpu: number | null;
        rssMB: number | null;
        threads: number | null;
        fd: number | null;
        fdLimit: number | null;
        old: number | null;
        gct: number | null;
        gctDelta: number | null;
    }>>([]);
    const [topThreads, setTopThreads] = useState<TopThread[] | null>(null);
    const [loadingTopThreads, setLoadingTopThreads] = useState(false);
    const [threadStates, setThreadStates] = useState<ThreadStateCounts | null>(null);
    const [loadingThreadStates, setLoadingThreadStates] = useState(false);
    const [autoThreadStateHint, setAutoThreadStateHint] = useState('');
    const [autoThreadStateArmed, setAutoThreadStateArmed] = useState(true);

    useEffect(() => {
        setSessionId(defaultSessionId);
    }, [defaultSessionId]);

    useEffect(() => {
        setProcesses([]);
        setProcessQuery('');
        setSelectedPid(null);
        setSnapshot(null);
        setError('');
        setHistory([]);
        setTopThreads(null);
        setThreadStates(null);
        setAutoThreadStateHint('');
        setAutoThreadStateArmed(true);
        if (!sessionId) return;
        void refreshProcessList(sessionId);
    }, [sessionId]);

    useEffect(() => {
        setHistory([]);
        if (!sessionId || !selectedPid) return;
        void refreshSnapshot(sessionId, selectedPid, true);
        setTopThreads(null);
        setThreadStates(null);
        setAutoThreadStateHint('');
        setAutoThreadStateArmed(true);
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

            setHistory(prev => {
                const nextPrev = resetHistory ? [] : prev.slice();
                const cpu = parsePercent(parsed.process.cpu);
                const threads = parsed.process.threads != null ? Number(parsed.process.threads) : null;
                const fd = parsed.process.fd_count != null ? Number(parsed.process.fd_count) : null;
                const fdLimit = parsed.process.fd_limit != null ? Number(parsed.process.fd_limit) : null;
                const rssMB = parsed.process.vm_rss_kb != null ? (Number(parsed.process.vm_rss_kb) / 1024) : null;
                const gc = parseJstatGcutilOnce(parsed.jvm.gcutil_once);
                const old = gc ? parseNumberLoose(gc.O) : null;
                const gct = gc ? parseNumberLoose(gc.GCT) : null;

                const last = nextPrev.length > 0 ? nextPrev[nextPrev.length - 1] : null;
                const gctDelta = (gct != null && last?.gct != null) ? Math.max(0, gct - last.gct) : null;
                const point = { t: Date.now(), cpu, rssMB, threads, fd, fdLimit, old, gct, gctDelta };

                const appended = [...nextPrev, point];
                const cap = Math.max(2, Math.round((timeRangeMin * 60) / Math.max(2, refreshIntervalSec)));
                if (appended.length > cap) return appended.slice(appended.length - cap);
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
    };

    const activeTitle = terminals.find(t => t.id === sessionId)?.title || '';
    const cpuPct = useMemo(() => parsePercent(snapshot?.process.cpu), [snapshot?.process.cpu]);
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
            rssMB: history.map(p => p.rssMB),
            threads: history.map(p => p.threads),
            fd: history.map(p => p.fd),
            old: history.map(p => p.old),
            gctDeltaMs: history.map(p => (p.gctDelta == null ? null : p.gctDelta * 1000))
        };
    }, [history]);

    const filteredProcesses = useMemo(() => {
        const q = processQuery.trim().toLowerCase();
        if (!q) return processes;
        return processes.filter(p => {
            const hay = `${p.pid} ${p.user} ${p.etime} ${p.cmd}`.toLowerCase();
            return hay.includes(q);
        });
    }, [processQuery, processes]);

    useEffect(() => {
        if (!autoRefresh) return;
        if (!selectedPid || !sessionId) return;
        if (!autoThreadStateArmed) return;
        if (loadingThreadStates) return;
        if (history.length < 3) return;
        const last3 = history.slice(history.length - 3);
        const high = last3.every(p => (p.cpu != null && p.cpu >= 85));
        if (!high) return;
        void fetchThreadStates('auto');
    }, [autoRefresh, autoThreadStateArmed, history, loadingThreadStates, selectedPid, sessionId]);

    const fetchTopThreads = async () => {
        if (!sessionId || !selectedPid) return;
        setLoadingTopThreads(true);
        setError('');
        try {
            // @ts-ignore
            const fn = window.go?.main?.App?.GetJavaTopCPUThreads;
            if (!fn) {
                setError('后端接口未就绪：GetJavaTopCPUThreads');
                return;
            }
            // @ts-ignore
            const resp = await window.go.main.App.GetJavaTopCPUThreads(sessionId, selectedPid);
            if (typeof resp === 'string' && resp.startsWith('Error:')) {
                setError(resp);
                return;
            }
            const parsed = JSON.parse(resp) as TopThread[];
            setTopThreads(Array.isArray(parsed) ? parsed : []);
        } catch (e: any) {
            setError(e?.toString?.() || '采样失败');
        } finally {
            setLoadingTopThreads(false);
        }
    };

    const fetchThreadStates = async (mode: 'manual' | 'auto') => {
        if (!sessionId || !selectedPid) return;
        setLoadingThreadStates(true);
        setError('');
        try {
            // @ts-ignore
            const fn = window.go?.main?.App?.GetJavaThreadStateCounts;
            if (!fn) {
                setError('后端接口未就绪：GetJavaThreadStateCounts');
                return;
            }
            // @ts-ignore
            const resp = await window.go.main.App.GetJavaThreadStateCounts(sessionId, selectedPid);
            if (typeof resp === 'string' && resp.startsWith('Error:')) {
                setError(resp);
                return;
            }
            const parsed = JSON.parse(resp) as ThreadStateCounts;
            setThreadStates(parsed);
            if (mode === 'auto') {
                setAutoThreadStateHint('CPU 持续超阈，已自动采样一次线程状态（可手动再次采样）');
                setAutoThreadStateArmed(false);
            } else {
                setAutoThreadStateHint('');
            }
        } catch (e: any) {
            setError(e?.toString?.() || '采样失败');
        } finally {
            setLoadingThreadStates(false);
        }
    };

    return (
        <div style={styles.container}>
            <div style={styles.stickyHeader}>
                <div style={styles.headerRow}>
                    <div style={styles.pageTitle}>
                        <div style={styles.pageTitleTop}>Java 性能看板</div>
                        <div style={styles.pageTitleSub}>开发定位向 · 默认近 15m · 单机单 PID 深挖</div>
                    </div>
                </div>

                <Card title="上下文">
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
                        <button
                            style={styles.secondaryButton}
                            onClick={() => sessionId && refreshProcessList(sessionId)}
                            disabled={!sessionId || loadingList}
                        >
                            {loadingList ? '加载中...' : '刷新进程'}
                        </button>
                    </div>
                    <div style={styles.row}>
                        <input
                            value={processQuery}
                            onChange={(e) => setProcessQuery(e.target.value)}
                            placeholder="筛选 Java 进程（PID/命令/用户）"
                            style={styles.input}
                            disabled={!sessionId}
                        />
                        <select value={String(timeRangeMin)} onChange={(e) => setTimeRangeMin(parseInt(e.target.value) || 15)} style={styles.smallSelect}>
                            <option value="5">近 5m</option>
                            <option value="15">近 15m</option>
                            <option value="60">近 1h</option>
                        </select>
                        <select value={String(refreshIntervalSec)} onChange={(e) => setRefreshIntervalSec(parseInt(e.target.value) || 5)} style={styles.smallSelect}>
                            <option value="3">3s</option>
                            <option value="5">5s</option>
                            <option value="10">10s</option>
                        </select>
                        <label style={styles.toggle}>
                            <input
                                type="checkbox"
                                checked={autoRefresh}
                                onChange={(e) => setAutoRefresh(e.target.checked)}
                                style={styles.toggleInput}
                            />
                            <span style={styles.toggleText}>自动</span>
                        </label>
                    </div>
                    <div style={styles.metaLine}>
                        <div style={styles.metaChip}>会话：{activeTitle || '-'}</div>
                        <div style={styles.metaChip}>PID：{snapshot?.pid || '-'}</div>
                        <div style={styles.metaChip}>工具：{jvmTools || '-'}</div>
                    </div>
                </Card>
            </div>

            {!sessionId && (
                <div style={styles.empty}>
                    请先建立 SSH 连接并选中一个终端会话。
                </div>
            )}

            {sessionId && (
                <Card
                    title="Java 进程"
                    right={<div style={styles.smallHint}>点击一个 PID 进入深挖</div>}
                    style={{ minHeight: 0 }}
                >
                    {filteredProcesses.length === 0 ? (
                        <div style={styles.smallHint}>未发现 java 进程（或权限不足）。</div>
                    ) : (
                        <div style={styles.procList} className="hide-scrollbar">
                            {filteredProcesses.map((p: JavaProcess) => (
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
                    title="核心生命体征"
                    style={{ minHeight: 0 }}
                >
                    {!snapshot && <div style={styles.smallHint}>加载快照中...</div>}
                    {snapshot && (
                        <div style={styles.tileGrid}>
                            <StatTile icon="⚙️" label="CPU%" value={snapshot.process.cpu ? `${snapshot.process.cpu}%` : '-'} sub={snapshot.process.etime || '-'} tone={toneForPercent(cpuPct, 60, 85)} />
                            <StatTile icon="🧠" label="RSS" value={formatBytesFromKB(snapshot.process.vm_rss_kb)} sub="物理内存" />
                            <StatTile icon="🧵" label="Threads" value={snapshot.process.threads != null ? String(snapshot.process.threads) : '-'} sub="线程总数" tone={(snapshot.process.threads ?? 0) >= 800 ? 'warn' : 'neutral'} />
                            <StatTile icon="📁" label="FD" value={snapshot.process.fd_count != null ? String(snapshot.process.fd_count) : '-'} sub={snapshot.process.fd_limit ? `上限 ${snapshot.process.fd_limit}` : '上限未知'} tone={(snapshot.process.fd_limit && snapshot.process.fd_count) ? (snapshot.process.fd_count / snapshot.process.fd_limit >= 0.8 ? 'warn' : 'neutral') : 'neutral'} />
                        </div>
                    )}
                </Card>
            )}

            {sessionId && selectedPid && (
                <Card title={`趋势（近 ${timeRangeMin}m，${refreshIntervalSec}s 间隔）`} style={{ minHeight: 0 }}>
                    <div style={styles.chartGrid}>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>CPU %</div>
                            <Sparkline data={series.cpu} min={0} max={100} thresholds={[{ value: 85, color: '#ff7a7a' }]} stroke="#4da3ff" />
                            <div style={styles.chartMeta}>{cpuPct != null ? `${cpuPct.toFixed(1)}%` : '-'}</div>
                        </div>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>RSS (MB)</div>
                            <Sparkline data={series.rssMB} min={0} stroke="#9bdb74" fill="rgba(155,219,116,0.16)" />
                            <div style={styles.chartMeta}>{snapshot?.process.vm_rss_kb != null ? formatBytesFromKB(snapshot.process.vm_rss_kb) : '-'}</div>
                        </div>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>Threads</div>
                            <Sparkline data={series.threads} min={0} stroke="#ffbf66" fill="rgba(255,191,102,0.14)" />
                            <div style={styles.chartMeta}>{snapshot?.process.threads != null ? String(snapshot.process.threads) : '-'}</div>
                        </div>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>FD</div>
                            <Sparkline
                                data={series.fd}
                                min={0}
                                stroke="#c78dff"
                                fill="rgba(199,141,255,0.14)"
                                thresholds={snapshot?.process.fd_limit ? [{ value: snapshot.process.fd_limit, color: '#8a8a8a' }] : undefined}
                            />
                            <div style={styles.chartMeta}>{snapshot?.process.fd_count != null ? String(snapshot.process.fd_count) : '-'}</div>
                        </div>
                    </div>
                </Card>
            )}

            {sessionId && selectedPid && (
                <Card
                    title="Top CPU Threads（默认可见，手动采样）"
                    right={
                        <button style={styles.primaryButton} onClick={fetchTopThreads} disabled={loadingTopThreads}>
                            {loadingTopThreads ? '采样中...' : '采样'}
                        </button>
                    }
                    style={{ minHeight: 0 }}
                >
                    {!topThreads && <div style={styles.smallHint}>点击“采样”获取当前最忙的线程，并尽力映射到 Java 线程信息（若 jstack 可用）。</div>}
                    {Array.isArray(topThreads) && topThreads.length === 0 && <div style={styles.smallHint}>暂无数据。</div>}
                    {Array.isArray(topThreads) && topThreads.length > 0 && (
                        <div style={styles.topList}>
                            {topThreads.map((t, idx) => (
                                <div key={idx} style={styles.topItem}>
                                    <div style={styles.topMain}>
                                        <div style={styles.topName}>{t.java_name || '(unknown)'} <span style={styles.topSub}>({t.tid_hex})</span></div>
                                        <div style={styles.topMeta}>CPU {t.cpu}% · {t.java_state || 'STATE?'}</div>
                                    </div>
                                    {t.stack_top && <div style={styles.topStack}>{t.stack_top}</div>}
                                </div>
                            ))}
                        </div>
                    )}
                </Card>
            )}

            {sessionId && selectedPid && (
                <Card
                    title="线程状态（手动为主，CPU 连续超阈会自动采样一次）"
                    right={
                        <button style={styles.secondaryButton} onClick={() => fetchThreadStates('manual')} disabled={loadingThreadStates}>
                            {loadingThreadStates ? '采样中...' : '采样'}
                        </button>
                    }
                    style={{ minHeight: 0 }}
                >
                    {autoThreadStateHint && <div style={styles.banner}>{autoThreadStateHint}</div>}
                    {!threadStates && <div style={styles.smallHint}>需要 jstack 可用且具备 attach 权限。</div>}
                    {threadStates && (
                        <div style={styles.tileGrid}>
                            <StatTile icon="🏃" label="RUNNABLE" value={String(threadStates.runnable)} sub="运行中" />
                            <StatTile icon="🧱" label="BLOCKED" value={String(threadStates.blocked)} sub="锁竞争" tone={threadStates.blocked > 0 ? 'warn' : 'good'} />
                            <StatTile icon="⏳" label="WAITING" value={String(threadStates.waiting)} sub="等待" />
                            <StatTile icon="🕰️" label="TIMED_WAITING" value={String(threadStates.timed_waiting)} sub="定时等待" />
                        </div>
                    )}
                </Card>
            )}

            {sessionId && selectedPid && snapshot && snapshot.tools.jstat && (
                <Card title="GC（仅在 jstat 可用时显示）" style={{ minHeight: 0 }}>
                    <div style={styles.chartGrid}>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>Old 使用 %</div>
                            <Sparkline data={series.old} min={0} max={100} stroke="#ffbf66" fill="rgba(255,191,102,0.14)" />
                            <div style={styles.chartMeta}>{gcutil?.O ? `${gcutil.O}%` : '-'}</div>
                        </div>
                        <div style={styles.chartTile}>
                            <div style={styles.chartTitle}>GC interval cost (ms)</div>
                            <Sparkline data={series.gctDeltaMs} min={0} stroke="#ff7a7a" fill="rgba(255,122,122,0.12)" showArea={false} />
                            <div style={styles.chartMeta}>{lastGctDeltaMs != null ? `${lastGctDeltaMs}ms` : '等待第 2 个点'}</div>
                        </div>
                    </div>
                </Card>
            )}

            {sessionId && selectedPid && snapshot && (
                <Card title="证据（可折叠）" style={{ minHeight: 0 }}>
                    <div style={styles.detailsGrid}>
                        <CommandBlock title="进程命令行" result={{ command: 'ps/procfs', output: snapshot.process.cmd || '' }} />
                        <CommandBlock title="JVM 版本" result={snapshot.jvm.vm_version} />
                        {snapshot.tools.jcmd && <CommandBlock title="Heap 摘要" result={snapshot.jvm.heap_info} />}
                        {snapshot.tools.jstat && <CommandBlock title="jstat -gcutil" result={snapshot.jvm.gcutil_once} />}
                    </div>
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
    stickyHeader: {
        position: 'sticky',
        top: 0,
        zIndex: 2,
        backgroundColor: '#252526',
        paddingBottom: '12px',
        borderBottom: '1px solid #1f1f1f'
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
    input: {
        flex: 1,
        backgroundColor: '#202020',
        color: '#ddd',
        border: '1px solid #2a2a2a',
        borderRadius: '4px',
        padding: '6px 8px',
        outline: 'none',
        fontSize: '12px'
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
    topList: {
        display: 'flex',
        flexDirection: 'column',
        gap: '8px'
    },
    topItem: {
        border: '1px solid #2a2a2a',
        borderRadius: '10px',
        backgroundColor: '#141414',
        padding: '10px',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px'
    },
    topMain: {
        display: 'flex',
        flexDirection: 'column',
        gap: '4px'
    },
    topName: {
        fontSize: '12px',
        fontWeight: 800,
        color: '#f2f2f2'
    },
    topSub: {
        fontSize: '11px',
        color: '#8a8a8a',
        fontWeight: 500
    },
    topMeta: {
        fontSize: '11px',
        color: '#a8a8a8'
    },
    topStack: {
        fontSize: '11px',
        color: '#cfcfcf',
        fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace',
        whiteSpace: 'nowrap',
        overflow: 'hidden',
        textOverflow: 'ellipsis'
    },
    banner: {
        padding: '8px 10px',
        borderRadius: '8px',
        border: '1px solid #2a2a2a',
        backgroundColor: '#141414',
        color: '#cfcfcf',
        fontSize: '12px'
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

