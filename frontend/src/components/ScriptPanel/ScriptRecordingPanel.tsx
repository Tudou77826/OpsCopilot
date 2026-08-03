import React, { useEffect, useState } from 'react';
import { TbPlayerPlay, TbPlayerStop } from 'react-icons/tb';
import { useToast } from '../Toast/Toast';

interface ScriptStatus {
    is_recording: boolean;
    script_id?: string;
    name?: string;
    command_count: number;
    duration: number;
}

interface ScriptRecordingPanelProps {
    activeSessionId: string | null;
    onRecordingComplete?: () => void;
}

const ScriptRecordingPanel: React.FC<ScriptRecordingPanelProps> = ({ activeSessionId, onRecordingComplete }) => {
    const [isRecording, setIsRecording] = useState(false);
    const [status, setStatus] = useState<ScriptStatus>({
        is_recording: false,
        command_count: 0,
        duration: 0
    });
    const [recentCommands, setRecentCommands] = useState<string[]>([]);
    const toast = useToast();

    // 获取录制状态
    const fetchRecordingStatus = async () => {
        try {
            // @ts-ignore
            const result = await window.go.main.App.GetScriptRecordingStatus();
            setStatus(result);
            setIsRecording(result.is_recording);
        } catch (err) {
            // 如果没有录制中的会话，重置状态
            setStatus({
                is_recording: false,
                command_count: 0,
                duration: 0
            });
            setIsRecording(false);
        }
    };

    // 定期更新录制状态
    useEffect(() => {
        fetchRecordingStatus();
        const interval = setInterval(fetchRecordingStatus, 2000); // 每2秒更新一次
        return () => clearInterval(interval);
    }, []);

    const handleStartRecording = async () => {
        if (!activeSessionId) {
            toast.warning('请先连接到SSH会话');
            return;
        }

        const name = prompt('请输入脚本名称:', '脚本_' + new Date().toLocaleString());
        if (!name) return;

        const description = prompt('请输入脚本描述（可选）:', '');

        try {
            // @ts-ignore
            await window.go.main.App.StartScriptRecording(name, description || '', activeSessionId);
            // 立即更新状态
            await fetchRecordingStatus();
        } catch (err: any) {
            // 更友好的错误提示 - Wails 错误可能是字符串或对象
            console.error('StartScriptRecording error:', err);
            let errorMessage = '未知错误';
            if (typeof err === 'string') {
                errorMessage = err;
            } else if (err?.message) {
                errorMessage = err.message;
            } else if (err?.error) {
                errorMessage = err.error;
            } else if (err?.toString && typeof err.toString === 'function') {
                errorMessage = err.toString();
            }
            toast.error(`无法开始录制: ${errorMessage}`);
            await fetchRecordingStatus(); // 刷新状态
        }
    };

    const handleStopRecording = async () => {
        try {
            // @ts-ignore
            const script = await window.go.main.App.StopScriptRecording();

            // 显示简短的成功提示
            toast.success(`录制完成，已保存 ${script.commands.length} 条命令`);

            // 清空最近命令列表并刷新状态
            setRecentCommands([]);
            await fetchRecordingStatus();

            // 通知脚本列表刷新
            if (onRecordingComplete) {
                onRecordingComplete();
            }
        } catch (err: any) {
            toast.error('停止录制失败: ' + err.message);
            await fetchRecordingStatus(); // 刷新状态
        }
    };

    const formatDuration = (seconds: number): string => {
        if (seconds < 60) {
            return `${seconds}秒`;
        }
        const mins = Math.floor(seconds / 60);
        const secs = Math.floor(seconds % 60);
        return `${mins}分${secs}秒`;
    };

    return (
        <div style={styles.container}>
            <div style={styles.header}>
                <h3 style={styles.title}>脚本录制</h3>
                <div style={styles.status}>
                    {isRecording ? (
                        <span style={styles.recordingBadge}>
                            ● 录制中 ({status.command_count} 条命令, {formatDuration(status.duration)})
                        </span>
                    ) : (
                        <span style={styles.idleBadge}>○ 未录制</span>
                    )}
                </div>
            </div>

            <div style={styles.controls}>
                {!isRecording ? (
                    <button
                        style={{
                            ...styles.startButton,
                            opacity: activeSessionId ? 1 : 0.6,
                            cursor: activeSessionId ? 'pointer' : 'not-allowed',
                        }}
                        onClick={handleStartRecording}
                        title={activeSessionId ? undefined : '请先连接到SSH会话'}
                    >
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>{TbPlayerPlay({ size: 14 })} 开始录制</span>
                    </button>
                ) : (
                    <button
                        style={styles.stopButton}
                        onClick={handleStopRecording}
                    >
                        <span style={{ display: 'inline-flex', alignItems: 'center', gap: '6px' }}>{TbPlayerStop({ size: 14 })} 停止录制</span>
                    </button>
                )}
            </div>

            {recentCommands.length > 0 && (
                <div style={styles.recentCommands}>
                    <div style={styles.recentTitle}>最近录制的命令:</div>
                    {recentCommands.map((cmd, idx) => (
                        <div key={idx} style={styles.commandItem}>
                            • {cmd}
                        </div>
                    ))}
                    {recentCommands.length >= 5 && (
                        <div style={styles.moreIndicator}>...</div>
                    )}
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        padding: '16px',
        backgroundColor: 'var(--bg-primary)',
        borderBottom: '1px solid var(--border)',
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '12px',
    },
    title: {
        margin: 0,
        fontSize: '14px',
        fontWeight: 600,
        color: 'var(--text-primary)',
    },
    status: {
        flex: 1,
        textAlign: 'right',
    },
    recordingBadge: {
        display: 'inline-block',
        padding: '4px 12px',
        backgroundColor: 'var(--danger)',
        color: 'var(--text-on-accent)',
        borderRadius: '4px',
        fontSize: '12px',
        fontWeight: 500,
    },
    idleBadge: {
        display: 'inline-block',
        padding: '4px 12px',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-tertiary)',
        borderRadius: '4px',
        fontSize: '12px',
    },
    controls: {
        display: 'flex',
        gap: '8px',
    },
    startButton: {
        padding: '8px 16px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
    },
    stopButton: {
        padding: '8px 16px',
        backgroundColor: 'var(--danger)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '13px',
        fontWeight: 500,
    },
    recentCommands: {
        marginTop: '12px',
        padding: '12px',
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '4px',
    },
    recentTitle: {
        fontSize: '12px',
        color: 'var(--text-tertiary)',
        marginBottom: '8px',
        fontWeight: 500,
    },
    commandItem: {
        fontSize: '12px',
        color: 'var(--text-primary)',
        fontFamily: 'var(--font-mono)',
        padding: '2px 0',
    },
    moreIndicator: {
        fontSize: '12px',
        color: 'var(--text-muted)',
        textAlign: 'center',
        marginTop: '4px',
    },
};

export default ScriptRecordingPanel;
