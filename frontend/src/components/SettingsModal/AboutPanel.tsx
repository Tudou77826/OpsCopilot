import React, { useState, useEffect, useCallback, useRef } from 'react';
import ReactMarkdown from 'react-markdown';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import logo from '../../assets/images/logo-universal.png';
import { colors, radius, font, btnSecondary } from './settingsStyles';

interface ReleaseInfo {
    tag_name: string;
    name?: string;
    body: string;
    html_url: string;
    published_at?: string;
}

interface ReleaseHistoryItem {
    tag_name: string;
    name: string;
    body: string;
    html_url: string;
    published_at: string;
}

interface ReleaseHistoryResponse {
    releases?: ReleaseHistoryItem[];
    error?: string;
}

interface UpdateStatus {
    hasUpdate: boolean;
    currentVersion: string;
    latestVersion?: string;
    release?: ReleaseInfo;
    downloadUrl?: string;
    skippedVersions?: string[];
    error?: string;
}

interface DownloadProgress {
    bytesDownloaded: number;
    bytesTotal: number;
    percentage: number;
    speedBps: number;
}

type UpdateState = 'idle' | 'checking' | 'available' | 'no-update' | 'error' | 'downloading' | 'ready';

const GITHUB_REPO = 'https://github.com/Tudou77826/OpsCopilot';

const friendlyError = (raw: string): string => {
    const lower = raw.toLowerCase();
    if (lower.includes('timeout') || lower.includes('deadline')) return '连接超时，请检查网络后重试';
    if (lower.includes('no such host') || lower.includes('dns') || lower.includes('lookup')) return '无法连接到更新服务器，请检查网络连接';
    if (lower.includes('connection refused') || lower.includes('network is unreachable')) return '网络不可用，请检查网络连接';
    if (lower.includes('tls') || lower.includes('certificate') || lower.includes('x509')) return '安全连接失败，请检查系统时间或网络代理设置';
    if (lower.includes('403') || lower.includes('rate limit')) return '更新服务暂不可用，请稍后再试';
    if (lower.includes('404')) return '未找到可用更新';
    if (lower.includes('dial tcp') || lower.includes('connect')) return '无法连接到更新服务器，请检查网络连接';
    if (lower.includes('no network') || lower.includes('offline')) return '当前无网络连接';
    return '检查更新失败，请稍后再试';
};

const friendlyHistoryError = (raw: string): string => {
    const lower = raw.toLowerCase();
    if (lower.includes('timeout') || lower.includes('deadline')) return '版本日志加载超时，请稍后再试';
    if (lower.includes('403') || lower.includes('rate limit')) return '版本日志暂不可用，请稍后再试';
    if (lower.includes('dial tcp') || lower.includes('connect') || lower.includes('dns') || lower.includes('lookup')) {
        return '版本日志加载失败，请检查网络连接';
    }
    return '版本日志暂不可用，请稍后再试';
};

const releaseAccentBar = (isCurrent: boolean, isNew: boolean): React.CSSProperties => ({
    width: '3px',
    flexShrink: 0,
    backgroundColor: isCurrent ? colors.success : (isNew ? colors.accent : 'transparent'),
});

const escapeRegExp = (value: string) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');

// 数值化比较点分版本号（如 1.9.0.0 vs 1.8.9.5），短的按 0 补齐
export const compareVersions = (a: string, b: string): number => {
    const pa = a.split('.').map(n => parseInt(n, 10) || 0);
    const pb = b.split('.').map(n => parseInt(n, 10) || 0);
    const len = Math.max(pa.length, pb.length);
    for (let i = 0; i < len; i++) {
        const d = (pa[i] || 0) - (pb[i] || 0);
        if (d !== 0) return d;
    }
    return 0;
};

const stripLeadingVersionHeading = (body: string, tagName: string) => {
    const version = tagName.trim().replace(/^v/i, '');
    if (!body || !version) return body;
    return body.replace(new RegExp(`^\\s{0,3}#{1,6}\\s+v?${escapeRegExp(version)}\\s*\\n+`, 'i'), '');
};

const extractSingleReleaseBody = (body: string, tagName: string) => {
    const firstSection = body.split(/\n\s*---\s*\n/)[0] || body;
    return stripLeadingVersionHeading(firstSection, tagName);
};

const AboutPanel: React.FC = () => {
    const [currentVersion, setCurrentVersion] = useState('...');
    const [updateState, setUpdateState] = useState<UpdateState>('idle');
    const [latestVersion, setLatestVersion] = useState('');
    const [downloadURL, setDownloadURL] = useState('');
    const [errorMsg, setErrorMsg] = useState('');
    const [progress, setProgress] = useState<DownloadProgress>({ bytesDownloaded: 0, bytesTotal: 0, percentage: 0, speedBps: 0 });
    const [releaseHistory, setReleaseHistory] = useState<ReleaseHistoryItem[]>([]);
    const [historyState, setHistoryState] = useState<'idle' | 'loading' | 'loaded' | 'error'>('idle');
    const [historyError, setHistoryError] = useState('');
    const [releaseIndex, setReleaseIndex] = useState(0);
    const [pageDirection, setPageDirection] = useState<'next' | 'prev' | 'none'>('none');
    // 落后版本数（比当前版本新的 release 个数）；null = 未知，不显示徽标
    const [versionsBehind, setVersionsBehind] = useState<number | null>(null);
    // AI 升级摘要：CheckUpdate 返回的累积变更日志原文 + 流式摘要状态
    const [cumulativeBody, setCumulativeBody] = useState('');
    const [summaryState, setSummaryState] = useState<'idle' | 'streaming' | 'done' | 'error'>('idle');
    const [summaryText, setSummaryText] = useState('');
    const [summaryError, setSummaryError] = useState('');
    const summaryOffRef = useRef<Array<() => void>>([]);

    // 卸载时取消摘要事件订阅
    useEffect(() => () => {
        summaryOffRef.current.forEach(off => off());
        summaryOffRef.current = [];
    }, []);

    useEffect(() => {
        const loadVersion = async () => {
            try {
                // @ts-ignore
                const v = await window.go?.main?.App?.GetVersion?.();
                if (v) setCurrentVersion(v);
            } catch { /* ignore */ }
        };
        loadVersion();
    }, []);

    const handleCheck = useCallback(async (): Promise<UpdateStatus | null> => {
        setUpdateState('checking');
        setErrorMsg('');
        setVersionsBehind(null);
        try {
            // @ts-ignore
            const raw = await window.go?.main?.App?.CheckUpdate?.();
            if (!raw) {
                setUpdateState('error');
                setErrorMsg('未收到响应');
                return null;
            }
            const status: UpdateStatus = typeof raw === 'string' ? JSON.parse(raw) : raw;
            if (status.error) {
                setUpdateState('error');
                setErrorMsg(friendlyError(status.error));
                return null;
            }
            if (status.hasUpdate && status.downloadUrl) {
                setLatestVersion(status.latestVersion || '');
                setDownloadURL(status.downloadUrl);
                // 保存累积变更日志原文（后端已合并多版本），供 AI 总结使用
                setCumulativeBody(status.release?.body || '');
                setSummaryState('idle');
                setSummaryText('');
                setSummaryError('');
                if (status.release) {
                    const latestTag = status.latestVersion || status.release.tag_name;
                    const latestRelease: ReleaseHistoryItem = {
                        tag_name: latestTag,
                        name: status.release.name || latestTag,
                        body: extractSingleReleaseBody(status.release.body || '', latestTag),
                        html_url: status.release.html_url || '',
                        published_at: status.release.published_at || '',
                    };
                    setReleaseHistory(prev => [latestRelease, ...prev.filter(r => r.tag_name !== latestTag)]);
                    setReleaseIndex(0);
                    setPageDirection('none');
                    setHistoryState('loaded');
                    setHistoryError('');
                }
                setUpdateState('available');
                // 有更新时后台统计落后版本数（仅计数，不影响日志展示状态）。
                // 优先用 CheckUpdate 返回的后端版本号，避免前端状态尚未就绪
                void fetchVersionsBehind(status.currentVersion);
            } else {
                setLatestVersion(status.latestVersion || status.currentVersion);
                setUpdateState('no-update');
            }
            return status;
        } catch (e: any) {
            setUpdateState('error');
            setErrorMsg(friendlyError(e.toString()));
            return null;
        }
    }, []);

    // AI 升级摘要：把累积变更日志交给用户配置的快速模型流式总结。
    // 总览作为第一张卡插入版本卡片组（deck），生成时自动翻到该卡。
    const handleSummarize = useCallback(async () => {
        // 已生成过：不重复调用模型，直接翻到总览卡片
        if (summaryState === 'done') {
            setReleaseIndex(0);
            setPageDirection('none');
            return;
        }
        if (summaryState === 'streaming' || !cumulativeBody) return;
        summaryOffRef.current.forEach(off => off());
        summaryOffRef.current = [];
        setSummaryState('streaming');
        setSummaryText('');
        setSummaryError('');
        setReleaseIndex(0); // 立即切到总览卡片（位于 deck 第一张）
        setPageDirection('none');

        try {
            if (EventsOn) {
                summaryOffRef.current.push(EventsOn('update-summary:token', (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    if (data.token) setSummaryText(prev => prev + data.token);
                }));
                summaryOffRef.current.push(EventsOn('update-summary:done', (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    if (data.summary) setSummaryText(String(data.summary));
                    setSummaryState('done');
                }));
                summaryOffRef.current.push(EventsOn('update-summary:error', (...args: any[]) => {
                    const data = args?.[0] ?? {};
                    setSummaryError(data.error || 'AI 总结失败');
                    setSummaryState('error');
                }));
            }
            // @ts-ignore
            const err = await window.go?.main?.App?.SummarizeUpdateNotes?.(cumulativeBody);
            if (err) {
                setSummaryError(String(err));
                setSummaryState('error');
            }
        } catch (e: any) {
            setSummaryError(e?.message || String(e));
            setSummaryState('error');
        }
    }, [cumulativeBody, summaryState]);

    // 后台拉取发布历史，统计比当前版本新的 release 个数。
    // 列表按新→旧排序：当前版本所在下标即落后数；当前版本不在列表
    // （过于久远或列表被截断）时退回数值版本比较。
    // 通过 ref 读取最新版本号：handleCheck（deps=[]）持有本函数的引用，
    // 直接闭包 state 会停留在首次渲染的占位值，导致生产环境永远不触发。
    const currentVersionRef = useRef(currentVersion);
    currentVersionRef.current = currentVersion;
    // 检查更新发现新版本后的一次后台请求，承担两件事：
    // 1. 补全版本日志——deck 此前只有最新一张卡，补全后才能逐版本翻页；
    // 2. 统计落后版本数（列表新→旧，当前版本下标即落后数；不在列表时退回数值比较）。
    // 版本号为非数字（dev 构建）时只做第 1 件事。
    const fetchVersionsBehind = useCallback(async (versionOverride?: string) => {
        let list: ReleaseHistoryItem[] = [];
        try {
            // @ts-ignore
            const raw = await window.go?.main?.App?.GetReleaseHistory?.();
            if (raw) {
                const resp: ReleaseHistoryResponse = typeof raw === 'string' ? JSON.parse(raw) : raw;
                list = resp.releases || [];
                if (list.length > 0) {
                    setReleaseHistory(list);
                    setHistoryState('loaded');
                    setHistoryError('');
                    setReleaseIndex(0); // 定位最新版本（列表新→旧，第 0 项）
                    setPageDirection('none');
                }
            }
        } catch { /* 静默：拿不到历史时 deck 保持单卡 */ }

        const current = (versionOverride || currentVersionRef.current || '').replace(/^v/i, '');
        // 非数字版本号（如 dev 构建 "vdev"）无法比较，不显示徽标
        if (!current || current === '...' || !/^\d/.test(current)) return;
        const norm = (t: string) => t.replace(/^v/i, '');
        const idx = list.findIndex(r => norm(r.tag_name) === current);
        if (idx >= 0) {
            setVersionsBehind(idx);
            return;
        }
        const newer = list.filter(r => compareVersions(norm(r.tag_name), current) > 0);
        if (newer.length > 0) {
            setVersionsBehind(newer.length);
        }
    }, []);

    const handleLoadReleases = useCallback(async (preferredVersion = '') => {
        setHistoryState('loading');
        setHistoryError('');
        try {
            // @ts-ignore
            const raw = await window.go?.main?.App?.GetReleaseHistory?.();
            if (!raw) {
                setHistoryState('error');
                setHistoryError('未收到响应');
                return;
            }
            const resp: ReleaseHistoryResponse = typeof raw === 'string' ? JSON.parse(raw) : raw;
            if (resp.error) {
                setHistoryState('error');
                setHistoryError(friendlyHistoryError(resp.error));
                return;
            }
            setReleaseHistory(resp.releases || []);
            setHistoryState('loaded');
            // 默认定位：有新版本→新版本；否则→当前版本；否则→最新一条。
            // 总览卡片占据 deck 第 0 位（若激活），release 定位需 +1 偏移
            const list = resp.releases || [];
            const norm = (t: string) => (t.startsWith('v') ? t.slice(1) : t);
            const currentVer = norm(currentVersion);
            const targetVersion = preferredVersion || latestVersion;
            let targetIdx = -1;
            if (targetVersion) {
                targetIdx = list.findIndex(r => r.tag_name === targetVersion);
            }
            if (targetIdx < 0 && currentVersion) {
                targetIdx = list.findIndex(r => norm(r.tag_name) === currentVer);
            }
            if (targetIdx < 0) targetIdx = 0;
            const offset = summaryState !== 'idle' && versionsBehind != null && versionsBehind >= 2 ? 1 : 0;
            setReleaseIndex(Math.min(targetIdx + offset, list.length + offset - 1));
            setPageDirection('none');
        } catch (e: any) {
            setHistoryState('error');
            setHistoryError(friendlyHistoryError(e.toString()));
        }
    }, [currentVersion, latestVersion, summaryState, versionsBehind]);

    const formatDate = (iso: string) => {
        if (!iso) return '';
        const d = new Date(iso);
        if (isNaN(d.getTime())) return iso;
        return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
    };

    const handleUpdate = useCallback(async () => {
        if (!downloadURL) return;
        setProgress({ bytesDownloaded: 0, bytesTotal: 0, percentage: 0, speedBps: 0 });
        setUpdateState('downloading');

        let offProgress: (() => void) | undefined;
        let offReady: (() => void) | undefined;

        try {
            offProgress = EventsOn('update-download-progress', (data: DownloadProgress) => {
                setProgress(data);
            });
            offReady = EventsOn('update-ready', () => {
                setUpdateState('ready');
            });

            // @ts-ignore
            const raw = await window.go?.main?.App?.DoUpdate?.(downloadURL);
            if (raw) {
                const result = typeof raw === 'string' ? JSON.parse(raw) : raw;
                if (!result.ok) {
                    setUpdateState('error');
                    setErrorMsg(friendlyError(result.error || ''));
                } else {
                    // Wails event delivery can lag behind the RPC response. Mark ready
                    // here as a fallback so the UI doesn't remain stuck at 100%.
                    setProgress((prev) => ({
                        ...prev,
                        percentage: 100,
                    }));
                    setUpdateState('ready');
                }
            }
        } catch (e: any) {
            setUpdateState('error');
            setErrorMsg(friendlyError(e.toString()));
        } finally {
            offProgress?.();
            offReady?.();
        }
    }, [downloadURL]);

    const pageRelease = useCallback((delta: number, total: number) => {
        setPageDirection(delta > 0 ? 'next' : 'prev');
        setReleaseIndex(i => Math.max(0, Math.min(total - 1, i + delta)));
    }, []);

    const formatBytes = (b: number) => {
        if (b < 1024) return `${b} B`;
        if (b < 1048576) return `${(b / 1024).toFixed(1)} KB`;
        return `${(b / 1048576).toFixed(1)} MB`;
    };

    const formatSpeed = (bps: number) => {
        if (bps < 1024) return `${bps.toFixed(0)} B/s`;
        if (bps < 1048576) return `${(bps / 1024).toFixed(0)} KB/s`;
        return `${(bps / 1048576).toFixed(1)} MB/s`;
    };

    const releaseMarkdownComponents = {
        h1: ({ children }: { children?: React.ReactNode }) => <div style={styles.releaseHeading}>{children}</div>,
        h2: ({ children }: { children?: React.ReactNode }) => <div style={styles.releaseHeading}>{children}</div>,
        h3: ({ children }: { children?: React.ReactNode }) => <div style={styles.releaseSubheading}>{children}</div>,
        p: ({ children }: { children?: React.ReactNode }) => <p style={styles.releaseParagraph}>{children}</p>,
        ul: ({ children }: { children?: React.ReactNode }) => <ul style={styles.releaseList}>{children}</ul>,
        li: ({ children }: { children?: React.ReactNode }) => <li style={styles.releaseListItem}>{children}</li>,
        strong: ({ children }: { children?: React.ReactNode }) => <strong style={styles.releaseStrong}>{children}</strong>,
        a: ({ href, children }: { href?: string; children?: React.ReactNode }) => (
            <a href={href} target="_blank" rel="noopener noreferrer" style={styles.releaseLink}>{children}</a>
        ),
    };

    return (
        <div style={styles.container}>
            {/* 动画 keyframes（about-spin 原先缺失定义导致 spinner 不转；blink 用于流式光标） */}
            <style>{`
                @keyframes about-spin { to { transform: rotate(360deg); } }
                @keyframes about-blink { 0%, 100% { opacity: 1; } 50% { opacity: 0; } }
            `}</style>
            <div style={styles.productHeader}>
                <div style={styles.productIdentity}>
                    <img src={logo} alt="OpsCopilot" style={styles.productLogo} />
                    <div style={styles.productMain}>
                        <div style={styles.productTitleRow}>
                            <div style={styles.productName}>OpsCopilot</div>
                            <span style={styles.versionBadge}>{currentVersion.startsWith('v') ? currentVersion : `v${currentVersion}`}</span>
                        </div>
                        <div style={styles.productDesc}>AI 驱动的智能运维助手</div>
                    </div>
                </div>
                <div style={styles.updateSummary}>
                    <div style={styles.sectionTitle}>版本与更新</div>
                    <div style={styles.sectionHint}>
                        <span>{updateState === 'idle' ? '尚未检查更新' : '发布记录与可用更新'}</span>
                        {updateState === 'available' && (
                            <span style={styles.updateMiniWarning}>新版本 {latestVersion}，重启会丢失会话</span>
                        )}
                    </div>
                </div>
                <div style={styles.headerActions}>
                    <a href={GITHUB_REPO} target="_blank" rel="noopener noreferrer" style={styles.projectLink}>项目主页</a>
                    <button style={styles.ghostBtn} onClick={() => handleLoadReleases()} disabled={historyState === 'loading'}>
                        {historyState === 'loading' ? '加载中...' : '更新日志'}
                    </button>
                    {(updateState === 'idle' || updateState === 'no-update' || updateState === 'available' || updateState === 'error') && (
                        <button style={styles.ghostBtn} onClick={handleCheck}>
                            {updateState === 'idle' ? '检查更新' : '重新检查'}
                        </button>
                    )}
                </div>
            </div>

            {(updateState !== 'idle' || historyState !== 'idle' || releaseHistory.length > 0) && (
                <div style={styles.section}>
                {/* 更新执行中状态：下载/就绪/错误 —— 作为列表上方的紧凑横幅 */}
                {updateState === 'checking' && (
                    <div style={styles.statusBanner}>
                        <div style={styles.loadingSpinner} />
                        <span style={styles.checkingText}>正在检查...</span>
                    </div>
                )}
                {updateState === 'no-update' && (
                    <div style={styles.statusBannerOk}>已是最新版本 ({latestVersion})</div>
                )}
                {updateState === 'available' && releaseHistory.length === 0 && (
                    <div style={styles.updateFallback}>
                        <span>
                            可更新至 {latestVersion}
                            {versionsBehind != null && versionsBehind >= 2 ? `（落后 ${versionsBehind} 个版本）` : ''}
                        </span>
                        <button style={styles.updateBtn} onClick={handleUpdate} disabled={!downloadURL}>
                            更新并重启
                        </button>
                    </div>
                )}
                {updateState === 'downloading' && (
                    <div style={styles.statusBanner}>
                        <div style={styles.progressLabel}>
                            {progress.percentage >= 100 ? '下载完成，正在准备安装...' : '正在下载更新...'}
                        </div>
                        <div style={styles.progressBarBg}>
                            <div style={{ ...styles.progressBarFill, width: `${Math.min(progress.percentage, 100)}%` }} />
                        </div>
                        <div style={styles.progressInfo}>
                            <span>{formatBytes(progress.bytesDownloaded)} / {formatBytes(progress.bytesTotal)}</span>
                            <span>{formatSpeed(progress.speedBps)}</span>
                        </div>
                    </div>
                )}
                {updateState === 'ready' && (
                    <div style={styles.statusBannerOk}>下载完成，正在重启应用...</div>
                )}
                {updateState === 'error' && (
                    <div style={styles.statusBannerError}>
                        <span>{errorMsg}</span>
                        <button style={styles.linkBtn} onClick={handleCheck}>重试</button>
                    </div>
                )}

                {/* 版本日志列表 loading */}
                {historyState === 'loading' && updateState !== 'checking' && (
                    <div style={styles.statusBanner}>
                        <div style={styles.loadingSpinner} />
                        <span style={styles.checkingText}>正在加载版本日志...</span>
                    </div>
                )}
                {historyState === 'error' && updateState !== 'error' && updateState !== 'available' && (
                    <div style={styles.statusBannerError}>
                        <span>{historyError}</span>
                        <button style={styles.linkBtn} onClick={() => handleLoadReleases()}>重试</button>
                    </div>
                )}
                {historyState === 'loaded' && releaseHistory.length === 0 && (
                    <div style={styles.emptyHint}>暂无版本记录</div>
                )}

                {/* 版本日志：单卡 + 左右翻页。AI 总览生成后作为第一张卡加入 deck，
                    与单版本卡片共用同一卡片体系（宽度/高度/叠卡/翻页一致） */}
                {releaseHistory.length > 0 && (() => {
                    const summaryInDeck = summaryState !== 'idle' && versionsBehind != null && versionsBehind >= 2;
                    const total = releaseHistory.length + (summaryInDeck ? 1 : 0);
                    const safeIdx = Math.min(releaseIndex, total - 1);
                    const isSummaryCard = summaryInDeck && safeIdx === 0;
                    const r = isSummaryCard ? null : releaseHistory[safeIdx - (summaryInDeck ? 1 : 0)];
                    const releaseBody = r ? stripLeadingVersionHeading(r.body, r.tag_name) : '';
                    const ver = r ? (r.tag_name.startsWith('v') ? r.tag_name.slice(1) : r.tag_name) : '';
                    const currentVer = currentVersion.startsWith('v') ? currentVersion.slice(1) : currentVersion;
                    const isCurrent = !!(r && currentVersion && currentVer === ver);
                    const isNew = !!(r && updateState === 'available' && latestVersion === r.tag_name);
                    const accentStyle = isSummaryCard || isCurrent
                        ? styles.releaseCardCurrent
                        : isNew
                            ? styles.releaseCardNew
                            : styles.releaseCardNormal;
                    return (
                        <div style={styles.releasePager}>
                            {total > 1 && (
                                <button
                                    style={{ ...styles.pagerArrow, ...styles.pagerArrowPrev }}
                                    onClick={() => pageRelease(-1, total)}
                                    disabled={safeIdx === 0}
                                    aria-label="上一项"
                                >‹</button>
                            )}

                            <div style={styles.releaseDeck}>
                                {total > 1 && (
                                    <>
                                        <div style={{ ...styles.releaseStackCard, ...styles.releaseStackCardBack }} />
                                        <div style={{ ...styles.releaseStackCard, ...styles.releaseStackCardMid }} />
                                    </>
                                )}
                                {isSummaryCard ? (
                                    <div
                                        key="ai-summary"
                                        className={`about-release-card about-release-card-${pageDirection}`}
                                        style={{ ...styles.releaseCard, ...styles.releaseCardNew }}
                                    >
                                        <div style={{ width: '3px', flexShrink: 0, backgroundColor: colors.accent }} />
                                        <div style={styles.releaseContent}>
                                            <div style={styles.releaseHeader}>
                                                <span style={styles.aiChip}>AI</span>
                                                <span style={styles.releaseTag}>更新总览</span>
                                                <span style={styles.summaryRange}>{currentVersion} → {latestVersion} · {versionsBehind} 个版本</span>
                                                {summaryState === 'streaming' && <div style={styles.loadingSpinner} />}
                                                {total > 1 && <span style={styles.releasePagerInfo}>{safeIdx + 1} / {total}</span>}
                                                <button
                                                    style={styles.summaryCloseBtn}
                                                    aria-label="关闭总览"
                                                    onClick={() => { setSummaryState('idle'); setSummaryText(''); setSummaryError(''); setReleaseIndex(0); setPageDirection('none'); }}
                                                >×</button>
                                            </div>
                                            {summaryState === 'error' ? (
                                                <div style={styles.summaryError}>
                                                    {summaryError}
                                                    <button style={styles.linkBtn} onClick={handleSummarize}>重试</button>
                                                </div>
                                            ) : summaryText ? (
                                                <div style={styles.releaseBody}>
                                                    <ReactMarkdown components={releaseMarkdownComponents}>{summaryText}</ReactMarkdown>
                                                    {summaryState === 'streaming' && <span style={styles.summaryCaret}>▍</span>}
                                                </div>
                                            ) : (
                                                <div style={styles.summaryGenerating}>
                                                    <div style={styles.loadingSpinner} />
                                                    <span>正在整合 {versionsBehind} 个版本的更新...</span>
                                                </div>
                                            )}
                                            {summaryState === 'done' && (
                                                <div style={styles.summaryFooter}>
                                                    <span style={{ flex: 1 }}>由 AI 整合生成，可能存在遗漏；逐版本详情可翻页查看。</span>
                                                    <button style={styles.summaryRegenBtn} onClick={handleSummarize}>重新生成</button>
                                                </div>
                                            )}
                                        </div>
                                    </div>
                                ) : r && (
                                    <div
                                        key={r.tag_name}
                                        className={`about-release-card about-release-card-${pageDirection}`}
                                        style={{ ...styles.releaseCard, ...accentStyle }}
                                    >
                                        <div style={releaseAccentBar(isCurrent, isNew)} />
                                        <div style={styles.releaseContent}>
                                            <div style={styles.releaseHeader}>
                                                <span style={styles.releaseTag}>{r.tag_name}</span>
                                                {isNew && <span style={styles.badgeNew}>新版本</span>}
                                                {isNew && versionsBehind != null && versionsBehind >= 2 && (
                                                    <span style={styles.badgeBehind} title={`当前 ${currentVersion}，最新 ${r.tag_name}，之间共 ${versionsBehind} 个版本`}>
                                                        落后 {versionsBehind} 个版本
                                                    </span>
                                                )}
                                                {isNew && versionsBehind != null && versionsBehind >= 2 && (
                                                    <button
                                                        style={styles.summaryTriggerBtn}
                                                        onClick={handleSummarize}
                                                        disabled={summaryState === 'streaming' || !cumulativeBody}
                                                        title="用 AI 把落后的多个版本更新整合成一张总览卡片"
                                                    >
                                                        {summaryState === 'streaming' ? 'AI 总览生成中...' : summaryState === 'done' ? '查看 AI 总览' : 'AI 总览'}
                                                    </button>
                                                )}
                                                {isCurrent && <span style={styles.badgeCurrent}>当前版本</span>}
                                                {total > 1 && <span style={styles.releasePagerInfo}>{safeIdx + 1} / {total}</span>}
                                                <span style={styles.releaseDate}>{formatDate(r.published_at)}</span>
                                            </div>
                                            {releaseBody && (
                                                <div style={styles.releaseBody}>
                                                    <ReactMarkdown components={releaseMarkdownComponents}>{releaseBody}</ReactMarkdown>
                                                </div>
                                            )}
                                            {isNew && (
                                                <div style={styles.btnRow}>
                                                    <button style={styles.updateBtn} onClick={handleUpdate} disabled={updateState !== 'available'}>
                                                        更新并重启
                                                    </button>
                                                </div>
                                            )}
                                        </div>
                                    </div>
                                )}
                            </div>

                            {total > 1 && (
                                <button
                                    style={{ ...styles.pagerArrow, ...styles.pagerArrowNext }}
                                    onClick={() => pageRelease(1, total)}
                                    disabled={safeIdx === total - 1}
                                    aria-label="下一项"
                                >›</button>
                            )}
                        </div>
                    );
                })()}
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        gap: '14px',
        padding: '2px 0 24px 0',
    },
    productHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
        padding: '10px 2px 14px',
        borderBottom: `1px solid ${colors.borderSubtle}`,
    },
    productIdentity: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        flexShrink: 0,
    },
    productLogo: {
        width: 42,
        height: 42,
        borderRadius: radius.md,
        flexShrink: 0,
    },
    productMain: {
        display: 'flex',
        flexDirection: 'column',
        gap: '3px',
        minWidth: 0,
    },
    productTitleRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '10px',
        flexWrap: 'wrap',
    },
    productName: {
        color: colors.textPrimary,
        fontSize: '18px',
        fontWeight: 700,
    },
    productDesc: {
        color: colors.textTertiary,
        fontSize: font.sm,
    },
    versionBadge: {
        color: colors.accent,
        fontSize: font.xs,
        fontFamily: 'monospace',
        backgroundColor: 'var(--info-tint)',
        border: '1px solid var(--info-tint-border)',
        borderRadius: radius.full,
        padding: '2px 8px',
    },
    projectLink: {
        color: colors.textSecondary,
        fontSize: font.sm,
        textDecoration: 'none',
        padding: '5px 10px',
        borderRadius: radius.sm,
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: colors.bgPrimary,
        flexShrink: 0,
    },
    updateSummary: {
        display: 'flex',
        flexDirection: 'column',
        gap: '2px',
        flex: 1,
        minWidth: '120px',
        paddingLeft: '16px',
        borderLeft: `1px solid ${colors.borderSubtle}`,
    },
    headerActions: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        gap: '8px',
        flexShrink: 0,
    },
    section: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        padding: '8px 0 0',
    },
    sectionTitle: {
        color: colors.textPrimary,
        fontSize: font.lg,
        fontWeight: 600,
    },
    sectionHint: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        flexWrap: 'wrap',
        color: colors.textMuted,
        fontSize: font.xs,
        marginTop: '3px',
    },
    updateMiniWarning: {
        color: colors.warning,
        backgroundColor: 'var(--warning-tint)',
        border: '1px solid var(--warning-tint-border)',
        borderRadius: radius.full,
        padding: '1px 7px',
    },
    ghostBtn: {
        padding: '5px 12px',
        backgroundColor: colors.bgPrimary,
        color: colors.textSecondary,
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: font.sm,
        fontWeight: 500,
    } as React.CSSProperties,
    linkBtn: {
        background: 'none',
        border: 'none',
        color: colors.accent,
        cursor: 'pointer',
        fontSize: font.sm,
        padding: 0,
        textDecoration: 'underline',
    },
    emptyHint: {
        color: colors.textMuted,
        fontSize: font.sm,
        padding: '12px 0',
    },
    // 状态横幅（更新流程的紧凑展示，不占独立空间）
    statusBanner: {
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
        padding: '10px 12px',
        backgroundColor: colors.bgPrimary,
        borderRadius: radius.md,
        border: `1px solid ${colors.borderPrimary}`,
    },
    statusBannerOk: {
        padding: '9px 12px',
        color: colors.success,
        fontSize: font.sm,
        backgroundColor: 'var(--success-tint)',
        borderRadius: radius.md,
        border: `1px solid var(--success-tint-border)`,
    },
    statusBannerError: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        padding: '9px 12px',
        color: colors.danger,
        fontSize: font.sm,
        backgroundColor: 'var(--danger-tint)',
        borderRadius: radius.md,
        border: `1px solid var(--danger-tint-border)`,
    },
    // 版本日志翻页容器：左右按钮 + 单张卡片
    // 约束：卡片高度按窗口大小计算（100vh - 固定占位）——同一窗口下恒定，翻页/加载横幅
    // 出现消失都不会改变高度，左右箭头（top:50% 定位）位置始终稳定
    releasePager: {
        position: 'relative',
        display: 'flex',
        alignItems: 'stretch',
        marginTop: '2px',
        minHeight: '304px',
        height: 'calc(100vh - 295px)',
        padding: '0 42px',
    },
    pagerArrow: {
        position: 'absolute',
        top: '50%',
        transform: 'translateY(-50%)',
        zIndex: 5,
        width: '28px',
        height: '42px',
        border: `1px solid ${colors.borderPrimary}`,
        backgroundColor: colors.bgPrimary,
        color: colors.textSecondary,
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: '20px',
        fontWeight: 300,
        lineHeight: '40px',
        padding: 0,
        textAlign: 'center' as const,
        transition: 'background-color 0.15s, color 0.15s',
    } as React.CSSProperties,
    pagerArrowPrev: {
        left: '0',
    },
    pagerArrowNext: {
        right: '0',
    },
    releasePagerInfo: {
        color: colors.textMuted,
        fontSize: font.xs,
        fontFamily: 'monospace',
        marginLeft: 'auto',
    },
    releaseDeck: {
        position: 'relative',
        flex: 1,
        minWidth: 0,
        minHeight: '292px',
        padding: '0 14px 10px 0',
    },
    releaseStackCard: {
        position: 'absolute',
        inset: '8px 4px 2px 12px',
        borderRadius: radius.md,
        border: `1px solid ${colors.borderSubtle}`,
        backgroundColor: colors.bgPrimary,
        pointerEvents: 'none',
    },
    releaseStackCardBack: {
        transform: 'translate(14px, 10px) rotate(1.2deg)',
        opacity: 0.28,
    },
    releaseStackCardMid: {
        transform: 'translate(7px, 5px) rotate(0.6deg)',
        opacity: 0.42,
    },
    releaseCard: {
        position: 'relative',
        zIndex: 2,
        display: 'flex',
        minHeight: '272px',
        height: '100%',
        borderRadius: radius.md,
        overflow: 'hidden',
        backgroundColor: colors.bgPrimary,
        border: `1px solid ${colors.borderPrimary}`,
        boxShadow: 'inset 0 1px 0 rgba(255, 255, 255, 0.02)',
    },
    releaseCardNormal: {
        // 普通（历史）版本卡片
    },
    releaseCardCurrent: {
        borderColor: 'var(--success-tint-border)',
        backgroundColor: 'var(--success-tint)',
    },
    releaseCardNew: {
        borderColor: 'var(--info-tint-border)',
        backgroundColor: colors.bgPrimary,
    },
    releaseContent: {
        flex: 1,
        display: 'flex',
        flexDirection: 'column',
        padding: '12px 14px 14px',
        minWidth: 0,
        minHeight: 0,
    },
    releaseHeader: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        marginBottom: '8px',
    },
    releaseTag: {
        color: colors.textPrimary,
        fontSize: font.base,
        fontWeight: 600,
        fontFamily: 'monospace',
        letterSpacing: 0,
    },
    badgeCurrent: {
        color: colors.success,
        fontSize: font.xs,
        backgroundColor: 'var(--success-tint)',
        padding: '2px 8px',
        borderRadius: radius.full,
        fontWeight: 600,
        border: '1px solid var(--success-tint-border)',
    },
    badgeNew: {
        color: colors.textPrimary,
        fontSize: font.xs,
        backgroundColor: colors.accent,
        padding: '2px 8px',
        borderRadius: radius.full,
        fontWeight: 600,
    },
    // 落后版本数徽标：警示色调，提示用户实际落后的不止最新一个版本
    badgeBehind: {
        color: 'var(--warning)',
        fontSize: font.xs,
        backgroundColor: 'var(--warning-tint)',
        padding: '2px 8px',
        borderRadius: radius.full,
        fontWeight: 600,
        border: '1px solid var(--warning-tint-border, transparent)',
    },
    // AI 总览相关：总览卡片复用 releaseCard 体系，此处仅补充差异样式
    summaryRange: {
        fontSize: font.xs,
        color: colors.textMuted,
    },
    // 新版本卡片头部的 AI 总览触发按钮（link 风格，低视觉重量）
    summaryTriggerBtn: {
        border: 'none',
        backgroundColor: 'transparent',
        color: colors.accent,
        fontSize: font.xs,
        cursor: 'pointer',
        padding: '2px 6px',
        borderRadius: radius.sm,
        fontWeight: 600,
    },
    // 生成中的占位内容（居中于卡片内容区）
    summaryGenerating: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        gap: '8px',
        color: colors.textMuted,
        fontSize: font.sm,
    },
    summaryFooter: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
        fontSize: font.xs,
        color: colors.textMuted,
    },
    // 重新生成：明确的按钮形态（区别于说明文字），右对齐脱离句尾
    summaryRegenBtn: {
        ...btnSecondary,
        flexShrink: 0,
        padding: '2px 10px',
        fontSize: font.xs,
    },
    // 小型 AI 徽标，标记内容由模型生成
    aiChip: {
        fontSize: '10px',
        fontWeight: 700,
        color: 'var(--text-on-accent, #fff)',
        backgroundColor: colors.accent,
        borderRadius: radius.sm,
        padding: '1px 6px',
        letterSpacing: '0.5px',
    },
    // 流式输出光标
    summaryCaret: {
        display: 'inline-block',
        color: colors.accent,
        animation: 'about-blink 0.9s step-end infinite',
    },
    summaryCloseBtn: {
        marginLeft: 'auto',
        border: 'none',
        backgroundColor: 'transparent',
        color: colors.textMuted,
        cursor: 'pointer',
        fontSize: '15px',
        lineHeight: 1,
        padding: '0 2px',
    },
    summaryError: {
        fontSize: font.sm,
        color: colors.danger,
    },
    releaseDate: {
        color: colors.textMuted,
        fontSize: font.xs,
        marginLeft: 'auto',
    },
    releaseBody: {
        flex: 1,
        minHeight: 0,
        color: colors.textSecondary,
        fontSize: font.sm,
        lineHeight: 1.6,
        overflowY: 'auto',
        paddingRight: '6px',
    },
    releaseHeading: {
        color: colors.textPrimary,
        fontSize: font.base,
        fontWeight: 700,
        margin: '2px 0 7px',
    },
    releaseSubheading: {
        color: colors.textSecondary,
        fontSize: font.base,
        fontWeight: 600,
        margin: '8px 0 6px',
    },
    releaseParagraph: {
        margin: '6px 0',
        color: colors.textSecondary,
        fontSize: font.sm,
        lineHeight: 1.55,
    },
    releaseList: {
        margin: '6px 0',
        paddingLeft: '18px',
        color: colors.textSecondary,
    },
    releaseListItem: {
        margin: '2px 0',
        lineHeight: 1.55,
    },
    releaseStrong: {
        color: colors.textPrimary,
        fontWeight: 600,
    },
    releaseLink: {
        color: colors.accent,
        textDecoration: 'none',
    },
    loadingSpinner: {
        width: '14px',
        height: '14px',
        border: `2px solid ${colors.bgHover}`,
        borderTopColor: colors.accent,
        borderRadius: '50%',
        animation: 'about-spin 0.8s linear infinite',
        flexShrink: 0,
    },
    checkingText: {
        color: colors.textTertiary,
        fontSize: font.base,
    },
    updateBtn: {
        padding: '7px 14px',
        backgroundColor: colors.accent,
        color: colors.textPrimary,
        border: 'none',
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: font.sm,
        fontWeight: 600,
    },
    updateFallback: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '10px',
        padding: '9px 12px',
        color: colors.warning,
        fontSize: font.sm,
        backgroundColor: 'var(--warning-tint)',
        border: '1px solid var(--warning-tint-border)',
        borderRadius: radius.md,
    },
    btnRow: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
        marginTop: '10px',
    },
    progressBarBg: {
        width: '100%',
        height: '6px',
        backgroundColor: colors.borderPrimary,
        borderRadius: '3px',
        overflow: 'hidden',
    },
    progressBarFill: {
        height: '100%',
        backgroundColor: colors.accent,
        borderRadius: '3px',
        transition: 'width 0.3s ease',
    },
    progressLabel: {
        color: colors.textTertiary,
        fontSize: font.sm,
    },
    progressInfo: {
        display: 'flex',
        justifyContent: 'space-between',
        color: colors.textTertiary,
        fontSize: font.xs,
    },
};

export default AboutPanel;
