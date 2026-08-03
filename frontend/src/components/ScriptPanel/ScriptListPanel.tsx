import React, { useEffect, useState, useMemo, forwardRef, useImperativeHandle } from 'react';
import { TbRefresh, TbFileText, TbEdit, TbPlayerPlay, TbFileExport, TbTrash, TbPlus } from 'react-icons/tb';
import VariableInputDialog from './VariableInputDialog';
import { useToast } from '../Toast/Toast';
import { confirmDialog } from '../ConfirmDialog/ConfirmDialog';

interface ScriptVariable {
    name: string;
    display_name: string;
    default_value: string;
    required: boolean;
    description: string;
}

interface Script {
    steps?: { command?: string }[];
    commands?: { content: string }[];
    id: string;
    name: string;
    description: string;
    start_time: string;
    command_count: number;
    host: string;
    user: string;
    variables?: ScriptVariable[];
}

interface ScriptListPanelProps {
    activeSessionId: string | null;
    onEditScript: (scriptId: string) => void;
    onReplayScript: (scriptId: string) => void;
}

const ScriptListPanel = forwardRef<{
    loadScripts: () => void;
}, ScriptListPanelProps>(({
    activeSessionId,
    onEditScript,
    onReplayScript
}, ref) => {
    const [scripts, setScripts] = useState<Script[]>([]);
    const [loading, setLoading] = useState(false);
    const [searchQuery, setSearchQuery] = useState('');
    const [varDialog, setVarDialog] = useState<{ scriptId: string; variables: ScriptVariable[] } | null>(null);
    const [showCreate, setShowCreate] = useState(false);
    const [newName, setNewName] = useState('');
    const [newDesc, setNewDesc] = useState('');
    const [creating, setCreating] = useState(false);
    const toast = useToast();

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
        const ok = await confirmDialog.show({
            message: `确定要删除脚本 "${scriptName}" 吗？`,
            danger: true,
        });
        if (!ok) return;

        try {
            // @ts-ignore
            await window.go.main.App.DeleteScript(scriptId);
            await loadScripts();
        } catch (err: any) {
            toast.error('删除失败: ' + err.message);
        }
    };

    const handleExport = async (scriptId: string) => {
        try {
            // @ts-ignore
            await window.go.main.App.ExportScript(scriptId);
        } catch (err: any) {
            if (err.message) {
                toast.error('导出失败: ' + err.message);
            }
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

    const handleReplay = async (scriptId: string) => {
        if (!activeSessionId) return;

        const s = scripts.find((s) => s.id === scriptId);
        if (s?.variables && s.variables.length > 0) {
            setVarDialog({ scriptId, variables: s.variables });
            return;
        }

        onReplayScript(scriptId);
    };

    const handleVarSubmit = async (values: Record<string, string>) => {
        if (!varDialog || !activeSessionId) return;
        const scriptId = varDialog.scriptId;
        setVarDialog(null);

        try {
            // @ts-ignore
            await window.go.main.App.ReplayScriptWithVars(scriptId, activeSessionId, values);
        } catch (err: any) {
            toast.error('回放失败: ' + (err?.message || err));
        }
    };

    const handleCreate = async () => {
        const name = newName.trim();
        if (!name) return;
        setCreating(true);
        try {
            // @ts-ignore
            await window.go.main.App.CreateScript(name, newDesc.trim());
            setShowCreate(false);
            setNewName('');
            setNewDesc('');
            await loadScripts();
        } catch (err: any) {
            toast.error('创建失败: ' + (err?.message || err));
        } finally {
            setCreating(false);
        }
    };

    return (
        <div style={styles.container}>
            <div style={styles.header}>
                <h3 style={styles.title}>我的脚本 ({scripts.length})</h3>
                <div style={{ display: 'flex', gap: '4px' }}>
                    <button style={styles.addButton} onClick={() => setShowCreate(!showCreate)} title="新建脚本">
                        {TbPlus({ size: 14 })}
                    </button>
                    <button style={styles.refreshButton} onClick={loadScripts} title="刷新">
                        {TbRefresh({ size: 14 })}
                    </button>
                </div>
            </div>

            {/* 新建脚本表单 */}
            {showCreate && (
                <div style={styles.createForm}>
                    <input
                        type="text"
                        placeholder="脚本名称（必填）"
                        value={newName}
                        onChange={(e) => setNewName(e.target.value)}
                        onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                        style={styles.createInput}
                        autoFocus
                    />
                    <input
                        type="text"
                        placeholder="描述说明（可选）"
                        value={newDesc}
                        onChange={(e) => setNewDesc(e.target.value)}
                        onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
                        style={styles.createInput}
                    />
                    <div style={{ display: 'flex', gap: '4px', justifyContent: 'flex-end' }}>
                        <button style={styles.createCancelBtn} onClick={() => { setShowCreate(false); setNewName(''); setNewDesc(''); }}>
                            取消
                        </button>
                        <button style={{
                            ...styles.createConfirmBtn,
                            opacity: newName.trim() && !creating ? 1 : 0.5,
                            cursor: newName.trim() && !creating ? 'pointer' : 'not-allowed',
                        }} onClick={handleCreate} disabled={!newName.trim() || creating}>
                            {creating ? '创建中...' : '创建'}
                        </button>
                    </div>
                </div>
            )}

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
                            <div style={{ color: 'var(--text-disabled)', marginBottom: '12px' }}>{TbFileText({ size: 36 })}</div>
                            <div style={styles.emptyText}>还没有脚本</div>
                            <div style={styles.emptyHint}>点击上方 + 按钮手动创建，或使用录制功能</div>
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
                                        {formatDate(script.start_time)}
                                    </span>
                                    <span style={styles.metaText}>
                                        {(script.steps?.length || script.commands?.length || 0)} 条命令
                                    </span>
                                    {script.variables && script.variables.length > 0 && (
                                        <span style={styles.varBadge}>
                                            {script.variables.length} 个变量
                                        </span>
                                    )}
                                </div>
                            </div>
                            <div style={styles.scriptActions}>
                                <button
                                    style={styles.iconButton}
                                    onClick={() => onEditScript(script.id)}
                                    title="编辑"
                                >
                                    {TbEdit({ size: 14 })}
                                </button>
                                <button
                                    style={{
                                        ...styles.iconButton,
                                        opacity: activeSessionId ? 1 : 0.5,
                                        cursor: activeSessionId ? 'pointer' : 'not-allowed'
                                    }}
                                    onClick={() => activeSessionId && handleReplay(script.id)}
                                    title="回放"
                                    disabled={!activeSessionId}
                                >
                                    {TbPlayerPlay({ size: 14 })}
                                </button>
                                <button
                                    style={styles.iconButton}
                                    onClick={() => handleExport(script.id)}
                                    title="导出"
                                >
                                    {TbFileExport({ size: 14 })}
                                </button>
                                <button
                                    style={{...styles.iconButton, color: 'var(--severity-danger)'}}
                                    onClick={() => handleDelete(script.id, script.name)}
                                    title="删除"
                                >
                                    {TbTrash({ size: 14 })}
                                </button>
                            </div>
                        </div>
                    ))}
                </div>
            )}

            <VariableInputDialog
                isOpen={varDialog !== null}
                variables={varDialog?.variables || []}
                onSubmit={handleVarSubmit}
                onCancel={() => setVarDialog(null)}
            />
        </div>
    );
});

ScriptListPanel.displayName = 'ScriptListPanel';

const styles: Record<string, React.CSSProperties> = {
    container: {
        padding: '12px 16px',
        backgroundColor: 'var(--bg-primary)',
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
        color: 'var(--text-primary)',
    },
    refreshButton: {
        padding: '4px 8px',
        backgroundColor: 'var(--bg-input)',
        color: 'var(--text-primary)',
        border: '1px solid var(--border-strong)',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
    },
    addButton: {
        padding: '4px 8px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: '1px solid var(--accent)',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
    },
    createForm: {
        padding: '10px',
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '4px',
        marginBottom: '8px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '6px',
    },
    createInput: {
        width: '100%',
        padding: '6px 10px',
        backgroundColor: 'var(--bg-primary)',
        border: '1px solid var(--border)',
        borderRadius: '4px',
        color: 'var(--text-primary)',
        fontSize: '12px',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    createCancelBtn: {
        padding: '4px 10px',
        backgroundColor: 'transparent',
        color: 'var(--text-tertiary)',
        border: '1px solid var(--border-strong)',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '11px',
    },
    createConfirmBtn: {
        padding: '4px 10px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '11px',
    },
    searchInput: {
        width: '100%',
        padding: '6px 10px',
        marginBottom: '8px',
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '4px',
        color: 'var(--text-primary)',
        fontSize: '12px',
        outline: 'none',
        boxSizing: 'border-box' as const,
    },
    searchInputFocus: {
        borderColor: 'var(--accent)',
    },
    loading: {
        textAlign: 'center',
        padding: '40px',
        color: 'var(--text-tertiary)',
        fontSize: '12px',
    },
    empty: {
        textAlign: 'center',
        padding: '40px 20px',
    },
    emptyText: {
        fontSize: '13px',
        color: 'var(--text-primary)',
        marginBottom: '4px',
    },
    emptyHint: {
        fontSize: '11px',
        color: 'var(--text-muted)',
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
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--bg-tertiary)',
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
        color: 'var(--text-primary)',
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
        color: 'var(--text-muted)',
    },
    varBadge: {
        fontSize: '10px',
        color: 'var(--chip-purple-fg)',
        backgroundColor: 'var(--chip-purple-bg)',
        padding: '1px 5px',
        borderRadius: '8px',
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
        color: 'var(--text-tertiary)',
        transition: 'background-color 0.15s',
    },
};

export default ScriptListPanel;
