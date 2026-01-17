import React, { useState, useEffect } from 'react';
import KeysMap from './KeysMap';

interface AppConfig {
    llm: {
        APIKey: string;
        BaseURL: string;
        FastModel: string;
        ComplexModel: string;
        Model?: string;
    };
    prompts: {
        [key: string]: string;
    };
    log: {
        dir: string;
    };
    docs: {
        dir: string;
    };
    completion_delay: number;
    command_query_shortcut: string;
}

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
    isBroadcastMode?: boolean;
    onToggleBroadcast?: (enabled: boolean) => void;
    onCompletionDelayChange?: (delay: number) => void;
}

const SettingsModal: React.FC<SettingsModalProps> = ({ isOpen, onClose, isBroadcastMode, onToggleBroadcast, onCompletionDelayChange }) => {
    const [config, setConfig] = useState<AppConfig | null>(null);
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');
    const [activeTab, setActiveTab] = useState<'llm' | 'prompts' | 'system' | 'app'>('llm');

    useEffect(() => {
        if (isOpen) {
            loadSettings();
            setMsg('');
        }
    }, [isOpen]);

    const loadSettings = async () => {
        setLoading(true);
        try {
            // @ts-ignore
            if (window.go && window.go.main && window.go.main.App && window.go.main.App.GetSettings) {
                // @ts-ignore
                const cfg = await window.go.main.App.GetSettings();
                const llmCfg = cfg.llm || {};
                const fastModel = llmCfg.FastModel || llmCfg.Model || '';
                const complexModel = llmCfg.ComplexModel || '';
                setConfig({
                    ...cfg,
                    llm: {
                        ...llmCfg,
                        FastModel: fastModel,
                        ComplexModel: complexModel,
                    },
                    command_query_shortcut: cfg.command_query_shortcut || 'Ctrl+K',
                });
            }
        } catch (e) {
            console.error(e);
            setMsg('加载设置失败');
        } finally {
            setLoading(false);
        }
    };

    const formatShortcutLabel = (shortcut: string) => {
        const normalized = (shortcut || '').trim();
        return normalized || 'Ctrl+K';
    };

    const handleSave = async () => {
        if (!config) return;
        setLoading(true);
        try {
            // @ts-ignore
            const err = await window.go.main.App.SaveSettings(config);
            if (err) {
                setMsg('错误: ' + err);
            } else {
                setMsg('设置已保存！');
                if (onCompletionDelayChange && config.completion_delay !== undefined) {
                    onCompletionDelayChange(config.completion_delay);
                }
                setTimeout(() => {
                    setMsg('');
                    onClose();
                }, 1000);
            }
        } catch (e: any) {
            setMsg('错误: ' + e.toString());
        } finally {
            setLoading(false);
        }
    };

    const handleChange = (section: keyof AppConfig, key: string, value: string) => {
        if (!config) return;
        const sectionValue = config[section];
        if (typeof sectionValue === 'object' && sectionValue !== null) {
            setConfig({
                ...config,
                [section]: {
                    ...sectionValue,
                    [key]: value
                }
            });
        }
    };

    const handleCompletionDelayChange = (value: number) => {
        if (!config) return;
        setConfig({
            ...config,
            completion_delay: value
        });
    };

    const handlePromptChange = (key: string, value: string) => {
        if (!config) return;
        setConfig({
            ...config,
            prompts: {
                ...config.prompts,
                [key]: value
            }
        });
    };

    if (!isOpen || !config) return null;

    return (
        <div style={styles.overlay}>
             <style>{`
                input:checked + span {
                    background-color: #2196F3 !important;
                }
                input:focus + span {
                    box-shadow: 0 0 1px #2196F3;
                }
                input:checked + span:before {
                    transform: translateX(20px);
                }
                /* Hide default checkbox */
                label input {
                    opacity: 0;
                    width: 0;
                    height: 0;
                }
            `}</style>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <h2 style={styles.title}>系统设置</h2>
                    <button onClick={onClose} style={styles.closeBtn}>×</button>
                </div>

                <div style={styles.tabs}>
                    <button 
                        style={activeTab === 'llm' ? styles.activeTab : styles.tab}
                        onClick={() => setActiveTab('llm')}
                    >
                        模型服务
                    </button>
                    <button 
                        style={activeTab === 'prompts' ? styles.activeTab : styles.tab}
                        onClick={() => setActiveTab('prompts')}
                    >
                        AI 提示词
                    </button>
                    <button 
                        style={activeTab === 'system' ? styles.activeTab : styles.tab}
                        onClick={() => setActiveTab('system')}
                    >
                        系统选项
                    </button>
                    <button 
                        style={activeTab === 'app' ? styles.activeTab : styles.tab}
                        onClick={() => setActiveTab('app')}
                    >
                        应用选项
                    </button>
                </div>

                <div style={styles.content}>
                    {activeTab === 'llm' && (
                        <div style={styles.formSection}>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>API 地址 (Base URL)</label>
                                <input 
                                    style={styles.input}
                                    value={config.llm.BaseURL}
                                    onChange={(e) => handleChange('llm', 'BaseURL', e.target.value)}
                                    placeholder="https://api.openai.com/v1"
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>API 密钥 (API Key)</label>
                                <input 
                                    style={styles.input}
                                    type="password"
                                    value={config.llm.APIKey}
                                    onChange={(e) => handleChange('llm', 'APIKey', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>快速模型（简单任务）</label>
                                <input 
                                    style={styles.input}
                                    value={config.llm.FastModel}
                                    onChange={(e) => handleChange('llm', 'FastModel', e.target.value)}
                                    placeholder="deepseek-chat"
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>复杂模型（长上下文任务）</label>
                                <input 
                                    style={styles.input}
                                    value={config.llm.ComplexModel}
                                    onChange={(e) => handleChange('llm', 'ComplexModel', e.target.value)}
                                    placeholder="glm46"
                                />
                            </div>
                        </div>
                    )}

                    {activeTab === 'prompts' && (
                        <div style={styles.formSection}>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>智能连接系统提示词 (Smart Connect)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['smart_connect'] || ''}
                                    onChange={(e) => handlePromptChange('smart_connect', e.target.value)}
                                    rows={10}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>AI 问答提示词 (AI Chat Agent)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['qa_prompt'] || ''}
                                    onChange={(e) => handlePromptChange('qa_prompt', e.target.value)}
                                    rows={10}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>问题排查提示词 (Troubleshooting Agent)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['troubleshoot_prompt'] || ''}
                                    onChange={(e) => handlePromptChange('troubleshoot_prompt', e.target.value)}
                                    rows={10}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>故障总结提示词 (Conclusion Agent)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['conclusion_prompt'] || ''}
                                    onChange={(e) => handlePromptChange('conclusion_prompt', e.target.value)}
                                    rows={10}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>内容润色提示词 (Polishing Agent)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['polish_prompt'] || ''}
                                    onChange={(e) => handlePromptChange('polish_prompt', e.target.value)}
                                    rows={10}
                                />
                            </div>
                        </div>
                    )}

                    {activeTab === 'system' && (
                        <div style={styles.formSection}>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>日志存储目录</label>
                                <input 
                                    style={styles.input}
                                    value={config.log.dir}
                                    onChange={(e) => handleChange('log', 'dir', e.target.value)}
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>业务文档&定位手册目录 (Docs Dir)</label>
                                <input 
                                    style={styles.input}
                                    value={config.docs?.dir || ''}
                                    onChange={(e) => handleChange('docs', 'dir', e.target.value)}
                                    placeholder="默认使用程序同级目录下的 docs"
                                />
                            </div>
                        </div>
                    )}

                    {activeTab === 'app' && (
                        <div style={styles.formSection}>
                            <KeysMap commandQueryShortcut={formatShortcutLabel(config.command_query_shortcut)} />
                            <div style={styles.formGroup}>
                                <label style={styles.label}>命令补全延迟时间 (毫秒)</label>
                                <input 
                                    style={styles.input}
                                    type="number"
                                    min="0"
                                    max="2000"
                                    step="50"
                                    value={config.completion_delay || 150}
                                    onChange={(e) => {
                                        const value = parseInt(e.target.value) || 150;
                                        setConfig({
                                            ...config,
                                            completion_delay: Math.max(0, Math.min(2000, value))
                                        });
                                    }}
                                />
                                <div style={{ color: '#888', fontSize: '0.8rem', marginTop: '4px' }}>
                                    设置命令自动补全的触发延迟时间（毫秒）。设置为 0 表示立即触发，设置为 2000 表示延迟 2 秒触发。
                                </div>
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label}>多窗口广播模式</label>
                                <div style={{ display: 'flex', alignItems: 'center', gap: '10px' }}>
                                    <label style={styles.switch}>
                                        <input 
                                            type="checkbox" 
                                            checked={!!isBroadcastMode}
                                            onChange={(e) => {
                                                if (onToggleBroadcast) {
                                                    onToggleBroadcast(e.target.checked);
                                                }
                                            }}
                                        />
                                        <span style={styles.slider}></span>
                                    </label>
                                    <span style={{ color: '#ccc', fontSize: '0.9rem' }}>
                                        {isBroadcastMode ? '已开启 (输入将同步到所有广播组终端)' : '已关闭'}
                                    </span>
                                </div>
                                <div style={{ color: '#888', fontSize: '0.8rem', marginTop: '4px' }}>
                                    开启后，默认将当前所有打开的终端加入广播组。您可以在标签页上单独切换每个终端的广播状态。
                                </div>
                            </div>
                        </div>
                    )}
                </div>

                <div style={styles.footer}>
                    <div style={styles.statusMsg}>{msg}</div>
                    <button onClick={handleSave} style={styles.saveBtn} disabled={loading}>
                        {loading ? '正在保存...' : '保存设置'}
                    </button>
                </div>
            </div>
        </div>
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 2000,
    },
    modal: {
        backgroundColor: '#252526',
        borderRadius: '8px',
        width: '700px',
        height: '600px',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 12px rgba(0, 0, 0, 0.5)',
        color: '#ccc',
        overflow: 'hidden',
    },
    header: {
        padding: '16px 24px',
        borderBottom: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
    },
    title: {
        margin: 0,
        fontSize: '1.2rem',
        color: '#fff',
    },
    closeBtn: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        fontSize: '1.5rem',
        cursor: 'pointer',
    },
    tabs: {
        display: 'flex',
        backgroundColor: '#1e1e1e',
        padding: '0 24px',
        borderBottom: '1px solid #3c3c3c',
    },
    tab: {
        padding: '12px 16px',
        background: 'none',
        border: 'none',
        borderBottom: '2px solid transparent',
        color: '#888',
        cursor: 'pointer',
        fontSize: '0.9rem',
    },
    activeTab: {
        padding: '12px 16px',
        background: 'none',
        border: 'none',
        borderBottom: '2px solid #007acc',
        color: '#fff',
        cursor: 'pointer',
        fontSize: '0.9rem',
        fontWeight: 'bold' as const,
    },
    content: {
        flex: 1,
        padding: '24px',
        overflowY: 'auto' as const,
    },
    formSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '16px',
    },
    formGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
    },
    label: {
        fontSize: '0.9rem',
        fontWeight: 'bold' as const,
        color: '#ddd',
    },
    input: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
    },
    textarea: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
        fontFamily: 'monospace',
        fontSize: '0.85rem',
        resize: 'vertical' as const,
    },
    footer: {
        padding: '16px 24px',
        borderTop: '1px solid #3c3c3c',
        display: 'flex',
        justifyContent: 'flex-end',
        alignItems: 'center',
        backgroundColor: '#1e1e1e',
        gap: '16px',
    },
    statusMsg: {
        color: '#4caf50',
        fontSize: '0.9rem',
    },
    saveBtn: {
        padding: '8px 24px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
    switch: {
        position: 'relative' as const,
        display: 'inline-block',
        width: '40px',
        height: '20px',
    },
    slider: {
        position: 'absolute' as const,
        cursor: 'pointer',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: '#ccc',
        transition: '.4s',
        borderRadius: '20px',
        ':before': {
            position: 'absolute' as const,
            content: '""',
            height: '16px',
            width: '16px',
            left: '2px',
            bottom: '2px',
            backgroundColor: 'white',
            transition: '.4s',
            borderRadius: '50%',
        }
    }
};

// Add style for checked state using a style tag in component since we can't use pseudo-classes easily in inline styles
// Or we can use conditional styling in render.
// Let's use a simple <style> tag in the component return.

export default SettingsModal;
