import React, { useState, useEffect } from 'react';

interface AppConfig {
    llm: {
        APIKey: string;
        BaseURL: string;
        Model: string;
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
}

interface SettingsModalProps {
    isOpen: boolean;
    onClose: () => void;
}

const SettingsModal: React.FC<SettingsModalProps> = ({ isOpen, onClose }) => {
    const [config, setConfig] = useState<AppConfig | null>(null);
    const [loading, setLoading] = useState(false);
    const [msg, setMsg] = useState('');
    const [activeTab, setActiveTab] = useState<'llm' | 'prompts' | 'system'>('llm');

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
                setConfig(cfg);
            }
        } catch (e) {
            console.error(e);
            setMsg('加载设置失败');
        } finally {
            setLoading(false);
        }
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
        setConfig({
            ...config,
            [section]: {
                ...config[section],
                [key]: value
            }
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
                                <label style={styles.label}>模型名称 (Model)</label>
                                <input 
                                    style={styles.input}
                                    value={config.llm.Model}
                                    onChange={(e) => handleChange('llm', 'Model', e.target.value)}
                                    placeholder="gpt-3.5-turbo"
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
                                <label style={styles.label}>问答系统提示词 (QA Agent)</label>
                                <textarea 
                                    style={styles.textarea}
                                    value={config.prompts['qa_prompt'] || ''}
                                    onChange={(e) => handlePromptChange('qa_prompt', e.target.value)}
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
};

export default SettingsModal;
