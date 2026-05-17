import React, { useState, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import { EventsOn } from '../../../wailsjs/runtime/runtime';
import logo from '../../assets/images/logo-universal.png';

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
        setUpdateState('downloading');

        let offProgress: (() => void) | undefined;
        let offReady: (() => void) | undefined;

        try {
            // @ts-ignore
            if (window.runtime?.EventsOn) {
                // @ts-ignore
                offProgress = EventsOn('update-download-progress', (data: DownloadProgress) => {
                    setProgress(data);
                });
                // @ts-ignore
                offReady = EventsOn('update-ready', () => {
                    setUpdateState('ready');
                });
            }

            // @ts-ignore
            const raw = await window.go?.main?.App?.DoUpdate?.(downloadURL);
            if (raw) {
                const result = typeof raw === 'string' ? JSON.parse(raw) : raw;
                if (!result.ok) {
                    setUpdateState('error');
                    setErrorMsg(friendlyError(result.error || ''));
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
                    <div style={styles.heroVersion}>v{currentVersion}</div>
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
                        <div style={styles.progressLabel}>正在下载更新...</div>
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
        backgroundColor: '#1e1e1e',
        borderRadius: '8px',
        border: '1px solid #333',
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
        color: '#fff',
        fontSize: '20px',
        fontWeight: 700,
        letterSpacing: '-0.3px',
    },
    heroDesc: {
        color: '#888',
        fontSize: '13px',
    },
    heroVersion: {
        color: '#58a6ff',
        fontSize: '12px',
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
        backgroundColor: '#1e1e1e',
        borderRadius: '6px',
        border: '1px solid #2a2a2a',
        display: 'flex',
        flexDirection: 'column',
        gap: '6px',
    },
    infoLabel: {
        color: '#666',
        fontSize: '11px',
        textTransform: 'uppercase' as const,
        letterSpacing: '0.5px',
    },
    infoValue: {
        color: '#ccc',
        fontSize: '13px',
    },
    link: {
        color: '#58a6ff',
        fontSize: '13px',
        textDecoration: 'none',
    },
    // Divider
    divider: {
        borderTop: '1px solid #333',
        margin: '16px 0',
    },
    // Update section
    section: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
    },
    sectionTitle: {
        color: '#fff',
        fontSize: '13px',
        fontWeight: 600,
    },
    checkBtn: {
        padding: '8px 16px',
        backgroundColor: '#007acc',
        color: '#fff',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
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
        border: '2px solid #444',
        borderTopColor: '#007acc',
        borderRadius: '50%',
        animation: 'about-spin 0.8s linear infinite',
        flexShrink: 0,
    },
    checkingText: {
        color: '#aaa',
        fontSize: '13px',
    },
    secondaryBtn: {
        padding: '6px 12px',
        backgroundColor: 'transparent',
        color: '#aaa',
        border: '1px solid #444',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        alignSelf: 'flex-start',
    },
    updateBtn: {
        padding: '8px 16px',
        backgroundColor: '#4caf50',
        color: '#fff',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
    },
    resultBox: {
        display: 'flex',
        flexDirection: 'column',
        gap: '10px',
    },
    upToDate: {
        color: '#4caf50',
        fontSize: '13px',
    },
    newVersion: {
        color: '#4caf50',
        fontSize: '14px',
        fontWeight: 600,
    },
    changelogBox: {
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '6px',
        padding: '10px 12px',
        maxHeight: '200px',
        overflowY: 'auto',
    },
    changelogTitle: {
        color: '#aaa',
        fontSize: '11px',
        marginBottom: '6px',
    },
    changelogBody: {
        color: '#ccc',
        fontSize: '12px',
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
        backgroundColor: '#333',
        borderRadius: '3px',
        overflow: 'hidden',
    },
    progressBarFill: {
        height: '100%',
        backgroundColor: '#007acc',
        borderRadius: '3px',
        transition: 'width 0.3s ease',
    },
    progressLabel: {
        color: '#aaa',
        fontSize: '12px',
    },
    progressInfo: {
        display: 'flex',
        justifyContent: 'space-between',
        color: '#888',
        fontSize: '11px',
    },
    readyMsg: {
        color: '#4caf50',
        fontSize: '13px',
        fontWeight: 600,
    },
    errorMsg: {
        color: '#f44336',
        fontSize: '12px',
    },
    // Footer
    footer: {
        color: '#444',
        fontSize: '11px',
        textAlign: 'center' as const,
        marginTop: '16px',
        paddingTop: '12px',
        borderTop: '1px solid #222',
    },
};

export default AboutPanel;
