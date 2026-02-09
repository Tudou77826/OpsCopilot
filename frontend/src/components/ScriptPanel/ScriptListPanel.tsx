import React, { useEffect, useState, useMemo, forwardRef, useImperativeHandle } from 'react';

interface Script {
    id: string;
    name: string;
    description: string;
    start_time: string;
    command_count: number;
    host: string;
    user: string;
}

interface ScriptListPanelProps {
    activeSessionId: string | null;
    onEditScript: (scriptId: string) => void;
    onReplayScript: (scriptId: string) => void;
}

const ScriptListPanel = forwardRef<any, ScriptListPanelProps>(({
    activeSessionId,
    onEditScript,
    onReplayScript
}, ref) => {
    const [scripts, setScripts] = useState<Script[]>([]);
    const [loading, setLoading] = useState(false);
    const [searchQuery, setSearchQuery] = useState('');

    useEffect(() => {
        loadScripts();
    }, []);

    const loadScripts = async () => {
        setLoading(true);
        try {
            // @ts-ignore
            const result = await window.go.main.App.GetScriptList();
            // 按名称排序
            const sorted = (result || []).sort((a: Script, b: Script) =>
                a.name.localeCompare(b.name, 'zh-CN')
            );
            setScripts(sorted);
        } catch (err: any) {
            console.error('Failed to load scripts:', err);
        } finally {
            setLoading(false);
        }
    };

    // 暴露 loadScripts 方法给父组件
    useImperativeHandle(ref, () => ({
        loadScripts
    }));

    const handleDelete = async (scriptId: string, scriptName: string) => {
        if (!confirm(`确定要删除脚本 "${scriptName}" 吗？`)) {
            return;
        }

        try {
            // @ts-ignore
            await window.go.main.App.DeleteScript(scriptId);
            await loadScripts();
        } catch (err: any) {
            alert('删除失败: ' + err.message);
        }
    };

    const handleExport = async (scriptId: string, scriptName: string) => {
        try {
            // @ts-ignore
            const shellScript = await window.go.main.App.ExportScript(scriptId);

            // 创建下载链接
            const blob = new Blob([shellScript], { type: 'text/plain' });
            const url = URL.createObjectURL(blob);
            const a = document.createElement('a');
            a.href = url;
            a.download = `${scriptName}.sh`;
            document.body.appendChild(a);
            a.click();
            document.body.removeChild(a);
            URL.revokeObjectURL(url);
        } catch (err: any) {
            alert('导出失败: ' + err.message);
        }
    };

    const formatDate = (dateStr: string): string => {
        const date = new Date(dateStr);
        const now = new Date();
        const diffMs = now.getTime() - date.getTime();
        const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24));

        if (diffDays === 0) {
            return '今天';
        } else if (diffDays === 1) {
            return '昨天';
        } else if (diffDays < 7) {
            return `${diffDays}天前`;
        } else {
            return date.toLocaleDateString('zh-CN', {
                month: '2-digit',
                day: '2-digit'
            });
        }
    };

    // 过滤脚本
    const filteredScripts = useMemo(() => {
        if (!searchQuery.trim()) {
            return scripts;
        }
        const query = searchQuery.toLowerCase();
        return scripts.filter(script =>
            script.name.toLowerCase().includes(query) ||
            (script.description && script.description.toLowerCase().includes(query))
        );
    }, [scripts, searchQuery]);

    return (
        <div style={styles.container}>
            <div style={styles.header}>
                <h3 style={styles.title}>我的脚本 ({scripts.length})</h3>
                <button style={styles.refreshButton} onClick={loadScripts}>
                    🔄
                </button>
            </div>

            {/* 搜索框 */}
            <input
                type="text"
                placeholder="搜索脚本..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                style={styles.searchInput}
            />

            {loading ? (
                <div style={styles.loading}>加载中...</div>
            ) : filteredScripts.length === 0 ? (
                <div style={styles.empty}>
                    {scripts.length === 0 ? (
                        <>
                            <div style={styles.emptyIcon}>📜</div>
                            <div style={styles.emptyText}>还没有录制的脚本</div>
                            <div style={styles.emptyHint}>使用脚本录制功能记录您的操作</div>
                        </>
                    ) : (
                        <div style={styles.emptyText}>没有找到匹配的脚本</div>
                    )}
                </div>
            ) : (
                <div style={styles.scriptList}>
                    {filteredScripts.map((script) => (
                        <div key={script.id} style={styles.scriptItem}>
                            <div style={styles.scriptInfo}>
                                <div style={styles.scriptName} title={script.description || script.name}>
                                    {script.name}
                                </div>
                                <div style={styles.scriptMeta}>
                                    <span style={styles.metaText} title={`录制于 ${new Date(script.start_time).toLocaleString('zh-CN')}`}>
                                        📅 {formatDate(script.start_time)}
                                    </span>
                                    <span style={styles.metaText}>
                                        📋 {script.command_count}
                                    </span>
                                </div>
                            </div>
                            <div style={styles.scriptActions}>
                                <button
                                    style={styles.iconButton}
                                    onClick={() => onEditScript(script.id)}
                                    title="编辑"
                                >
                                    ✏️
                                </button>
                                <button
                                    style={{
                                        ...styles.iconButton,
                                        opacity: activeSessionId ? 1 : 0.5,
                                        cursor: activeSessionId ? 'pointer' : 'not-allowed'
                                    }}
                                    onClick={() => activeSessionId && onReplayScript(script.id)}
                                    title="回放"
                                    disabled={!activeSessionId}
                                >
                                    ▶️
                                </button>
                                <button
                                    style={styles.iconButton}
                                    onClick={() => handleExport(script.id, script.name)}
                                    title="导出"
                                >
                                    📤
                                </button>
                                <button
                                    style={{...styles.iconButton, color: '#ff6b6b'}}
                                    onClick={() => handleDelete(script.id, script.name)}
                                    title="删除"
                                >
                                    🗑️
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    container: {
        padding: '12px 16px',
        backgroundColor: '#1e1e1e',
        flex: 1,
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column' as const,
    },
    header: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        marginBottom: '8px',
    },
    title: {
        margin: 0,
        fontSize: '13px',
        fontWeight: 600,
        color: '#e0e0e0',
    },
    refreshButton: {
        padding: '4px 8px',
        backgroundColor: '#424242',
        color: '#e0e0e0',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
    },
    searchInput: {
        width: '100%',
        padding: '6px 10px',
        marginBottom: '8px',
        backgroundColor: '#252526',
        border: '1px solid #3c3c3c',
        borderRadius: '4px',
        color: '#e0e0e0',
        fontSize: '12px',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    searchInputFocus: {
        borderColor: '#007acc',
    },
    loading: {
        textAlign: 'center',
        padding: '40px',
        color: '#b0b0b0',
        fontSize: '12px',
    },
    empty: {
        textAlign: 'center',
        padding: '40px 20px',
    },
    emptyIcon: {
        fontSize: '36px',
        marginBottom: '12px',
    },
    emptyText: {
        fontSize: '13px',
        color: '#e0e0e0',
        marginBottom: '4px',
    },
    emptyHint: {
        fontSize: '11px',
        color: '#757575',
    },
    scriptList: {
        flex: 1,
        overflowY: 'auto' as const,
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    scriptItem: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        padding: '8px 10px',
        backgroundColor: '#252526',
        border: '1px solid #2d2d2d',
        borderRadius: '4px',
        cursor: 'pointer',
        transition: 'background-color 0.15s',
    },
    scriptInfo: {
        flex: 1,
        minWidth: 0,
    },
    scriptName: {
        fontSize: '13px',
        fontWeight: 500,
        color: '#e0e0e0',
        overflow: 'hidden',
        textOverflow: 'ellipsis',
        whiteSpace: 'nowrap' as const,
        marginBottom: '2px',
    },
    scriptMeta: {
        display: 'flex',
        gap: '8px',
    },
    metaText: {
        fontSize: '10px',
        color: '#757575',
    },
    scriptActions: {
        display: 'flex',
        gap: '2px',
        marginLeft: '8px',
    },
    iconButton: {
        width: '24px',
        height: '24px',
        padding: '0',
        backgroundColor: 'transparent',
        border: 'none',
        borderRadius: '3px',
        cursor: 'pointer',
        fontSize: '12px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        color: '#b0b0b0',
        transition: 'background-color 0.15s',
    },
};

export default ScriptListPanel;
