import React, { useState, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import logo from '../../assets/images/logo-universal.png';
import { colors, radius, font } from './settingsStyles';

interface ReleaseInfo {
    tag_name: string;
    body: string;
    html_url: string;
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

const releaseAccentBar = (isCurrent: boolean, isNew: boolean): React.CSSProperties => ({
    width: '3px',
    flexShrink: 0,
    backgroundColor: isCurrent ? colors.success : (isNew ? colors.accent : 'transparent'),
});

const AboutPanel: React.FC = () => {
    const [currentVersion, setCurrentVersion] = useState('...');
    const [updateState, setUpdateState] = useState<UpdateState>('idle');
    const [latestVersion, setLatestVersion] = useState('');
    const [changelog, setChangelog] = useState('');
    const [downloadURL, setDownloadURL] = useState('');
    const [errorMsg, setErrorMsg] = useState('');
    const [skippedVersions, setSkippedVersions] = useState<string[]>([]);
    const [progress, setProgress] = useState<DownloadProgress>({ bytesDownloaded: 0, bytesTotal: 0, percentage: 0, speedBps: 0 });
    const [releaseHistory, setReleaseHistory] = useState<ReleaseHistoryItem[]>([]);
    const [historyState, setHistoryState] = useState<'idle' | 'loading' | 'loaded' | 'error'>('idle');
    const [historyError, setHistoryError] = useState('');

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

    const handleCheck = useCallback(async () => {
        setUpdateState('checking');
        setErrorMsg('');
        try {
            // @ts-ignore
            const raw = await window.go?.main?.App?.CheckUpdate?.();
            if (!raw) {
                setUpdateState('error');
                setErrorMsg('未收到响应');
                return;
            }
            const status: UpdateStatus = typeof raw === 'string' ? JSON.parse(raw) : raw;
            if (status.error) {
                setUpdateState('error');
                setErrorMsg(friendlyError(status.error));
                return;
            }
            if (status.hasUpdate && status.downloadUrl) {
                setLatestVersion(status.latestVersion || '');
                setChangelog(status.release?.body || '');
                setDownloadURL(status.downloadUrl);
                setSkippedVersions(status.skippedVersions || []);
                setUpdateState('available');
            } else {
                setLatestVersion(status.latestVersion || status.currentVersion);
                setUpdateState('no-update');
            }
        } catch (e: any) {
            setUpdateState('error');
            setErrorMsg(friendlyError(e.toString()));
        }
    }, []);

    const handleLoadReleases = useCallback(async () => {
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
                setHistoryError(friendlyError(resp.error));
                return;
            }
            setReleaseHistory(resp.releases || []);
            setHistoryState('loaded');
        } catch (e: any) {
            setHistoryState('error');
            setHistoryError(friendlyError(e.toString()));
        }
    }, []);

    const handleCheckAndLoad = useCallback(() => {
        handleCheck();
        if (historyState === 'idle') {
            handleLoadReleases();
        }
    }, [handleCheck, handleLoadReleases, historyState]);

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

    return (
        <div style={styles.container}>
            {/* Hero card */}
            <div style={styles.heroCard}>
                <img src={logo} alt="OpsCopilot" style={styles.heroLogo} />
                <div style={styles.heroInfo}>
                    <div style={styles.heroName}>OpsCopilot</div>
                    <div style={styles.heroDesc}>AI 驱动的智能运维助手</div>
                    <div style={styles.heroVersion}>{currentVersion.startsWith('v') ? currentVersion : `v${currentVersion}`}</div>
                </div>
            </div>

            {/* Info grid */}
            <div style={styles.infoGrid}>
                <div style={styles.infoCard}>
                    <div style={styles.infoLabel}>作者</div>
                    <div style={styles.infoValue}>z-yibo</div>
                </div>
                <div style={styles.infoCard}>
                    <div style={styles.infoLabel}>项目主页</div>
                    <a href={GITHUB_REPO} target="_blank" rel="noopener noreferrer" style={styles.link}>
                        GitHub
                    </a>
                </div>
            </div>

            <div style={styles.divider} />

            {/* 版本与更新（一体化）：更新状态 + 版本日志列表合并展示 */}
            <div style={styles.section}>
                <div style={styles.sectionHeader}>
                    <div style={styles.sectionTitle}>版本与更新</div>
                    {(updateState === 'idle' || updateState === 'no-update' || updateState === 'available') && (
                        <button style={styles.ghostBtn} onClick={handleCheckAndLoad} disabled={historyState === 'loading'}>
                            {updateState === 'idle' ? '检查更新' : '重新检查'}
                        </button>
                    )}
                </div>

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
                {historyState === 'error' && updateState !== 'error' && (
                    <div style={styles.statusBannerError}>
                        <span>{historyError}</span>
                        <button style={styles.linkBtn} onClick={handleLoadReleases}>重试</button>
                    </div>
                )}
                {historyState === 'loaded' && releaseHistory.length === 0 && (
                    <div style={styles.emptyHint}>暂无版本记录</div>
                )}

                {/* 版本日志卡片列表 */}
                {releaseHistory.length > 0 && (
                    <div style={styles.releaseList}>
                        {releaseHistory.map((r) => {
                            const ver = r.tag_name.startsWith('v') ? r.tag_name.slice(1) : r.tag_name;
                            const currentVer = currentVersion.startsWith('v') ? currentVersion.slice(1) : currentVersion;
                            const isCurrent = !!(currentVersion && currentVer === ver);
                            const isNew = updateState === 'available' && latestVersion === r.tag_name;
                            const accentStyle = isCurrent
                                ? styles.releaseCardCurrent
                                : isNew
                                    ? styles.releaseCardNew
                                    : styles.releaseCardNormal;
                            return (
                                <div key={r.tag_name} style={{ ...styles.releaseCard, ...accentStyle }}>
                                    <div style={releaseAccentBar(isCurrent, isNew)} />
                                    <div style={styles.releaseContent}>
                                        <div style={styles.releaseHeader}>
                                            <span style={styles.releaseTag}>{r.tag_name}</span>
                                            {isNew && <span style={styles.badgeNew}>新版本</span>}
                                            {isCurrent && <span style={styles.badgeCurrent}>当前版本</span>}
                                            <span style={styles.releaseDate}>{formatDate(r.published_at)}</span>
                                        </div>
                                        {r.body && (
                                            <div style={styles.releaseBody}>
                                                <ReactMarkdown>{r.body}</ReactMarkdown>
                                            </div>
                                        )}
                                        {isNew && (
                                            <div style={styles.btnRow}>
                                                <button style={styles.updateBtn} onClick={handleUpdate} disabled={updateState !== 'available'}>
                                                    立即更新
                                                </button>
                                                <button style={styles.ghostBtn} onClick={() => setUpdateState('idle')}>
                                                    稍后
                                                </button>
                                            </div>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}
            </div>

            {/* Footer */}
            <div style={styles.footer}>
                Made with dedication for Ops teams
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        gap: '0',
        padding: '4px 0 0 0',
    },
    // Hero
    heroCard: {
        display: 'flex',
        alignItems: 'center',
        gap: '16px',
        padding: '20px',
        backgroundColor: colors.bgPrimary,
        borderRadius: radius.lg,
        border: `1px solid ${colors.borderPrimary}`,
    },
    heroLogo: {
        width: 56,
        height: 56,
        borderRadius: '12px',
        flexShrink: 0,
    },
    heroInfo: {
        display: 'flex',
        flexDirection: 'column',
        gap: '2px',
    },
    heroName: {
        color: colors.textPrimary,
        fontSize: '20px',
        fontWeight: 700,
        letterSpacing: '-0.3px',
    },
    heroDesc: {
        color: colors.textTertiary,
        fontSize: font.base,
    },
    heroVersion: {
        color: colors.accent,
        fontSize: font.sm,
        fontFamily: 'monospace',
        marginTop: '2px',
    },
    // Info grid
    infoGrid: {
        display: 'grid',
        gridTemplateColumns: '1fr 1fr',
        gap: '8px',
        marginTop: '10px',
    },
    infoCard: {
        padding: '10px 12px',
        backgroundColor: colors.bgPrimary,
        borderRadius: radius.md,
        border: `1px solid ${colors.borderPrimary}`,
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
    },
    infoLabel: {
        color: colors.textMuted,
        fontSize: font.xs,
        textTransform: 'uppercase' as const,
        letterSpacing: '0.5px',
    },
    infoValue: {
        color: colors.textSecondary,
        fontSize: font.base,
    },
    link: {
        color: colors.accent,
        fontSize: font.base,
        textDecoration: 'none',
    },
    // Divider
    divider: {
        borderTop: `1px solid ${colors.borderPrimary}`,
        margin: '16px 0',
    },
    // Update section
    section: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
    },
    sectionTitle: {
        color: colors.textPrimary,
        fontSize: font.base,
        fontWeight: 600,
    },
    sectionHeader: {
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        gap: '8px',
        marginBottom: '2px',
    },
    ghostBtn: {
        padding: '5px 12px',
        backgroundColor: 'transparent',
        color: colors.textTertiary,
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
        padding: '8px 12px',
        color: colors.success,
        fontSize: font.sm,
        backgroundColor: 'rgba(76, 175, 80, 0.08)',
        borderRadius: radius.md,
        border: `1px solid rgba(76, 175, 80, 0.25)`,
    },
    statusBannerError: {
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
        padding: '8px 12px',
        color: colors.danger,
        fontSize: font.sm,
        backgroundColor: 'rgba(244, 67, 54, 0.08)',
        borderRadius: radius.md,
        border: `1px solid rgba(244, 67, 54, 0.25)`,
    },
    // 版本日志卡片列表
    releaseList: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
        maxHeight: '460px',
        overflowY: 'auto',
        paddingRight: '4px',
        marginTop: '4px',
    },
    releaseCard: {
        display: 'flex',
        borderRadius: radius.md,
        overflow: 'hidden',
        backgroundColor: colors.bgSecondary,
        border: `1px solid ${colors.borderPrimary}`,
    },
    releaseCardNormal: {
        // 普通（历史）版本卡片
    },
    releaseCardCurrent: {
        borderColor: 'rgba(76, 175, 80, 0.4)',
        backgroundColor: 'rgba(76, 175, 80, 0.04)',
    },
    releaseCardNew: {
        borderColor: 'rgba(0, 122, 204, 0.5)',
        backgroundColor: 'rgba(0, 122, 204, 0.06)',
    },
    releaseContent: {
        flex: 1,
        padding: '12px 14px',
        minWidth: 0,
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
    },
    badgeCurrent: {
        color: colors.success,
        fontSize: font.xs,
        backgroundColor: 'rgba(76, 175, 80, 0.15)',
        padding: '2px 8px',
        borderRadius: radius.full,
        fontWeight: 600,
        border: '1px solid rgba(76, 175, 80, 0.3)',
    },
    badgeNew: {
        color: colors.textPrimary,
        fontSize: font.xs,
        backgroundColor: colors.accent,
        padding: '2px 8px',
        borderRadius: radius.full,
        fontWeight: 600,
    },
    releaseDate: {
        color: colors.textMuted,
        fontSize: font.xs,
        marginLeft: 'auto',
    },
    releaseBody: {
        color: colors.textSecondary,
        fontSize: font.sm,
        lineHeight: 1.6,
    },
    checkBtn: {
        padding: '8px 16px',
        backgroundColor: colors.accent,
        color: colors.textPrimary,
        border: 'none',
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: font.base,
        alignSelf: 'flex-start',
    },
    checkingRow: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
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
    secondaryBtn: {
        padding: '6px 12px',
        backgroundColor: 'transparent',
        color: colors.textTertiary,
        border: `1px solid ${colors.bgHover}`,
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: font.sm,
        alignSelf: 'flex-start',
    },
    updateBtn: {
        padding: '8px 16px',
        backgroundColor: colors.success,
        color: colors.textPrimary,
        border: 'none',
        borderRadius: radius.sm,
        cursor: 'pointer',
        fontSize: font.base,
    },
    resultBox: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
    },
    upToDate: {
        color: colors.success,
        fontSize: font.base,
    },
    newVersion: {
        color: colors.success,
        fontSize: font.lg,
        fontWeight: 600,
    },
    skippedHint: {
        color: colors.textMuted,
        fontSize: font.xs,
        lineHeight: 1.5,
    },
    changelogBox: {
        backgroundColor: colors.bgPrimary,
        border: `1px solid ${colors.borderPrimary}`,
        borderRadius: radius.md,
        padding: '10px 12px',
        maxHeight: '360px',
        overflowY: 'auto',
    },
    changelogTitle: {
        color: colors.textTertiary,
        fontSize: font.xs,
        marginBottom: '6px',
    },
    changelogBody: {
        color: colors.textSecondary,
        fontSize: font.sm,
        lineHeight: 1.6,
    },
    btnRow: {
        display: 'flex',
        gap: '8px',
        alignItems: 'center',
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
    readyMsg: {
        color: colors.success,
        fontSize: font.base,
        fontWeight: 600,
    },
    errorMsg: {
        color: colors.danger,
        fontSize: font.sm,
    },
    // Footer
    footer: {
        color: colors.bgHover,
        fontSize: font.xs,
        textAlign: 'center' as const,
        marginTop: '16px',
        paddingTop: '12px',
        borderTop: `1px solid ${colors.borderPrimary}`,
    },
};

export default AboutPanel;
