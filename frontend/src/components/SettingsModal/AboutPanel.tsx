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

const AboutPanel: React.FC = () => {
    const [currentVersion, setCurrentVersion] = useState('...');
    const [updateState, setUpdateState] = useState<UpdateState>('idle');
    const [latestVersion, setLatestVersion] = useState('');
    const [changelog, setChangelog] = useState('');
    const [downloadURL, setDownloadURL] = useState('');
    const [errorMsg, setErrorMsg] = useState('');
    const [skippedVersions, setSkippedVersions] = useState<string[]>([]);
    const [progress, setProgress] = useState<DownloadProgress>({ bytesDownloaded: 0, bytesTotal: 0, percentage: 0, speedBps: 0 });

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

            {/* Update section */}
            <div style={styles.section}>
                <div style={styles.sectionTitle}>更新</div>

                {updateState === 'idle' && (
                    <button style={styles.checkBtn} onClick={handleCheck}>
                        检查更新
                    </button>
                )}
                {updateState === 'checking' && (
                    <div style={styles.checkingRow}>
                        <div style={styles.loadingSpinner} />
                        <span style={styles.checkingText}>正在检查...</span>
                    </div>
                )}

                {updateState === 'no-update' && (
                    <div style={styles.resultBox}>
                        <div style={styles.upToDate}>已是最新版本 ({latestVersion})</div>
                        <button style={styles.secondaryBtn} onClick={() => setUpdateState('idle')}>
                            重新检查
                        </button>
                    </div>
                )}

                {updateState === 'available' && (
                    <div style={styles.resultBox}>
                        <div style={styles.newVersion}>发现新版本: {latestVersion}</div>
                        {skippedVersions.length > 0 && (
                            <div style={styles.skippedHint}>
                                包含 {skippedVersions.length + 1} 个版本的更新: {latestVersion}, {skippedVersions.slice(0, 2).join(', ')}{skippedVersions.length > 2 ? ' ...' : ''}
                            </div>
                        )}
                        {changelog && (
                            <div style={styles.changelogBox}>
                                <div style={styles.changelogTitle}>更新内容</div>
                                <div style={styles.changelogBody}>
                                    <ReactMarkdown>{changelog}</ReactMarkdown>
                                </div>
                            </div>
                        )}
                        <div style={styles.btnRow}>
                            <button style={styles.updateBtn} onClick={handleUpdate}>
                                立即更新
                            </button>
                            <button style={styles.secondaryBtn} onClick={() => setUpdateState('idle')}>
                                稍后再说
                            </button>
                        </div>
                    </div>
                )}

                {updateState === 'downloading' && (
                    <div style={styles.resultBox}>
                        <div style={styles.progressLabel}>
                            {progress.percentage >= 100 ? '下载完成，正在准备安装...' : '正在下载更新...'}
                        </div>
                        <div style={styles.progressBarBg}>
                            <div style={{
                                ...styles.progressBarFill,
                                width: `${Math.min(progress.percentage, 100)}%`,
                            }} />
                        </div>
                        <div style={styles.progressInfo}>
                            <span>{formatBytes(progress.bytesDownloaded)} / {formatBytes(progress.bytesTotal)}</span>
                            <span>{formatSpeed(progress.speedBps)}</span>
                        </div>
                    </div>
                )}

                {updateState === 'ready' && (
                    <div style={styles.resultBox}>
                        <div style={styles.readyMsg}>下载完成，正在重启应用...</div>
                    </div>
                )}

                {updateState === 'error' && (
                    <div style={styles.resultBox}>
                        <div style={styles.errorMsg}>{errorMsg}</div>
                        <button style={styles.secondaryBtn} onClick={handleCheck}>
                            重试
                        </button>
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
