import React, { useState, useEffect, useCallback } from 'react';
import ReactMarkdown from 'react-markdown';
import { EventsOn } from '../../../wailsjs/runtime/runtime';

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
                setErrorMsg('无响应');
                return;
            }
            const status: UpdateStatus = typeof raw === 'string' ? JSON.parse(raw) : raw;
            if (status.error) {
                setUpdateState('error');
                setErrorMsg(status.error);
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
            setErrorMsg(e.toString());
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
                    setErrorMsg(result.error || '更新失败');
                }
            }
        } catch (e: any) {
            setUpdateState('error');
            setErrorMsg(e.toString());
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
            {/* App info */}
            <div style={styles.appInfo}>
                <div style={styles.appName}>OpsCopilot</div>
                <div style={styles.version}>当前版本: {currentVersion}</div>
            </div>

            {/* Check button */}
            {updateState === 'idle' && (
                <button style={styles.checkBtn} onClick={handleCheck}>
                    检查更新
                </button>
            )}
            {updateState === 'checking' && (
                <button style={{ ...styles.checkBtn, opacity: 0.6 }} disabled>
                    检查中...
                </button>
            )}

            {/* No update */}
            {updateState === 'no-update' && (
                <div style={styles.resultBox}>
                    <div style={styles.upToDate}>已是最新版本 ({latestVersion})</div>
                    <button style={styles.secondaryBtn} onClick={() => setUpdateState('idle')}>
                        重新检查
                    </button>
                </div>
            )}

            {/* Update available */}
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
                            取消
                        </button>
                    </div>
                </div>
            )}

            {/* Downloading */}
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

            {/* Ready to restart */}
            {updateState === 'ready' && (
                <div style={styles.resultBox}>
                    <div style={styles.readyMsg}>下载完成，正在重启应用...</div>
                </div>
            )}

            {/* Error */}
            {updateState === 'error' && (
                <div style={styles.resultBox}>
                    <div style={styles.errorMsg}>更新失败: {errorMsg}</div>
                    <button style={styles.secondaryBtn} onClick={() => setUpdateState('idle')}>
                        重试
                    </button>
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        display: 'flex',
        flexDirection: 'column',
        gap: '16px',
        padding: '8px 0',
    },
    appInfo: {
        display: 'flex',
        flexDirection: 'column',
        gap: '4px',
    },
    appName: {
        color: '#fff',
        fontSize: '18px',
        fontWeight: 600,
    },
    version: {
        color: '#aaa',
        fontSize: '13px',
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
    secondaryBtn: {
        padding: '6px 12px',
        backgroundColor: 'transparent',
        color: '#aaa',
        border: '1px solid #444',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
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
};

export default AboutPanel;
