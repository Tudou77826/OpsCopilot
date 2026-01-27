import React, { useState, useEffect } from 'react';
import { ConnectionConfig } from '../../types';
import ConnectionConfigForm from '../ConnectionConfigForm/ConnectionConfigForm';

interface SmartConnectModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConnect: (configs: ConnectionConfig[]) => void;
    onParse: (input: string) => Promise<ConnectionConfig[]>;
}

const SmartConnectModal: React.FC<SmartConnectModalProps> = ({ isOpen, onClose, onConnect, onParse }) => {
    const [input, setInput] = useState('');
    const [isLoading, setIsLoading] = useState(false);
    const [parsedConfigs, setParsedConfigs] = useState<ConnectionConfig[]>([]);
    const [selectedIndices, setSelectedIndices] = useState<Set<number>>(new Set());
    const [expandedIndices, setExpandedIndices] = useState<Set<number>>(new Set());
    const [error, setError] = useState('');
    const [showErrorDetails, setShowErrorDetails] = useState(false);

    // Reset state when modal opens/closes
    useEffect(() => {
        if (!isOpen) {
            setInput('');
            setParsedConfigs([]);
            setSelectedIndices(new Set());
            setExpandedIndices(new Set());
            setError('');
        }
    }, [isOpen]);

    if (!isOpen) return null;

    const handleParse = async () => {
        if (!input.trim()) return;
        
        setIsLoading(true);
        setError('');
        setShowErrorDetails(false);
        try {
            const result = await onParse(input);
            // Prevent crash if result is null, and handle empty results
            const configs = result || [];
            
            if (configs.length === 0) {
                throw new Error("未识别到连接信息。请尝试提供更详细的信息（例如：'连接到 192.168.1.1 用户 root'）。");
            }

            const configsWithName = configs.map(c => ({
                ...c,
                name: c.name || c.host
            }));
            
            // Append new configs to existing ones
            const startIndex = parsedConfigs.length;
            setParsedConfigs(prev => [...prev, ...configsWithName]);
            
            // Select the newly added configs
            setSelectedIndices(prev => {
                const newSet = new Set(prev);
                configsWithName.forEach((_, i) => newSet.add(startIndex + i));
                return newSet;
            });
            
            // If it's the first batch, expand the first one
            if (parsedConfigs.length === 0 && configsWithName.length === 1) {
                setExpandedIndices(new Set([0]));
            }

            // Clear input after successful parse
            setInput('');
        } catch (e: any) {
            let errorMsg = e.message || e.toString();
            errorMsg = errorMsg.replace(/^Error: /, '');
            
            // Friendly error messages
            if (errorMsg.includes("TLS handshake timeout") || errorMsg.includes("timeout")) {
                errorMsg = "连接超时：无法连接到 AI 服务，请检查您的网络。";
            } else if (errorMsg.includes("Cannot read properties of null")) {
                errorMsg = "内部错误：收到无效的 AI 服务响应。";
            }
            
            setError(errorMsg);
        } finally {
            setIsLoading(false);
        }
    };

    const handleAddManual = () => {
        const newConfig: ConnectionConfig = {
            host: '',
            port: 22,
            user: 'root',
            name: '新连接'
        };
        const newIndex = parsedConfigs.length;
        setParsedConfigs(prev => [...prev, newConfig]);
        setSelectedIndices(prev => new Set(prev).add(newIndex));
        setExpandedIndices(prev => new Set(prev).add(newIndex));
    };

    const handleRemoveConfig = (index: number) => {
        const newConfigs = parsedConfigs.filter((_, i) => i !== index);
        setParsedConfigs(newConfigs);
        
        // Re-calculate selected/expanded indices is tricky because indices shift.
        // For simplicity, we just clear selections or try to preserve valid ones.
        // A robust way requires IDs, but index is simple for now.
        // Let's just clear selection/expansion to avoid bugs for this iteration.
        setSelectedIndices(new Set()); 
        setExpandedIndices(new Set());
    };

    const handleConnect = () => {
        // Clone selected configs to avoid mutating state directly
        const toConnect = parsedConfigs
            .filter((_, i) => selectedIndices.has(i))
            .map(c => ({ ...c }));
        
        // Grouping Logic for multiple connections
        if (toConnect.length > 1) {
            let groupName = `Batch-${new Date().toISOString().slice(0, 10)}`; // Default: Batch-YYYY-MM-DD
            
            // User requirement: Use Bastion IP if available
            const withBastion = toConnect.find(c => c.bastion && c.bastion.host);
            if (withBastion && withBastion.bastion) {
                groupName = withBastion.bastion.host;
            }
            
            // Assign group to all
            toConnect.forEach(c => {
                c.group = groupName;
            });
        }

        onConnect(toConnect);
        onClose();
    };

    const toggleSelection = (index: number) => {
        const newSet = new Set(selectedIndices);
        if (newSet.has(index)) {
            newSet.delete(index);
        } else {
            newSet.add(index);
        }
        setSelectedIndices(newSet);
    };

    const toggleExpand = (index: number) => {
        const newSet = new Set(expandedIndices);
        if (newSet.has(index)) {
            newSet.delete(index);
        } else {
            newSet.add(index);
        }
        setExpandedIndices(newSet);
    };

    const updateConfig = (index: number, field: keyof ConnectionConfig | 'bastion.host' | 'bastion.user' | 'bastion.password' | 'bastion.port', value: any) => {
        const newConfigs = [...parsedConfigs];
        const config = { ...newConfigs[index] };

        // Handle nested bastion fields
        if (field.startsWith('bastion.')) {
            if (!config.bastion) {
                config.bastion = { host: '', port: 22, user: '', name: 'Bastion' };
            }
            const bastionField = field.split('.')[1] as keyof ConnectionConfig;
            config.bastion = { ...config.bastion, [bastionField]: value };
        } else {
            // Handle root level fields
            // Name sync logic: if editing Host, and Name equals old Host (or is empty), update Name too
            if (field === 'host') {
                if (!config.name || config.name === config.host || config.name === '新连接') {
                    config.name = value;
                }
            }
            (config as any)[field] = value;
        }

        newConfigs[index] = config;
        setParsedConfigs(newConfigs);
    };

    const updateConfigObject = (index: number, next: ConnectionConfig) => {
        const newConfigs = [...parsedConfigs];
        newConfigs[index] = next;
        setParsedConfigs(newConfigs);
    };

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <h2 style={styles.title}>新建连接</h2>
                
                {/* AI Input Section - Always visible but compact */}
                <div style={styles.inputSection}>
                    <div style={{display: 'flex', gap: '8px'}}>
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder="你可以使用自然输入连接要求，如 '连接到 192.168.1.10 使用 root 用户， 密码是：xxx'..."
                            style={styles.textarea}
                            rows={2}
                        />
                        <button 
                            onClick={handleParse} 
                            style={styles.aiButton}
                            disabled={isLoading || !input.trim()}
                        >
                            {isLoading ? '分析中...' : '智能分析'}
                        </button>
                    </div>
                    {error && (
                        <div style={styles.errorContainer}>
                            <div style={styles.errorMessage}>
                                <span>⚠️ {error.includes('Raw:') ? '解析错误' : error}</span>
                                {error.includes('Raw:') && (
                                    <span 
                                        style={styles.detailsLink} 
                                        onClick={() => setShowErrorDetails(!showErrorDetails)}
                                    >
                                        {showErrorDetails ? '隐藏详情' : '显示详情'}
                                    </span>
                                )}
                            </div>
                            {showErrorDetails && error.includes('Raw:') && (
                                <pre style={styles.errorDetails}>
                                    {error}
                                </pre>
                            )}
                        </div>
                    )}
                </div>

                {/* Results List */}
                <div style={styles.resultSection}>
                    <div style={styles.resultHeader}>
                        <h3 style={styles.subtitle}>连接列表 ({parsedConfigs.length})</h3>
                        {/* "New Search" removed as requested */}
                    </div>
                    
                    <div style={styles.list}>
                        {parsedConfigs.length === 0 && (
                            <div style={{textAlign: 'center', padding: '20px', color: '#666', border: '1px dashed #444', borderRadius: '4px'}}>
                                暂无连接信息。请使用上方 AI 分析或手动添加。
                            </div>
                        )}
                        {parsedConfigs.map((config, i) => {
                            const isExpanded = expandedIndices.has(i);
                            const isSelected = selectedIndices.has(i);
                            return (
                                <div key={i} style={{...styles.card, borderLeft: isSelected ? '4px solid #007acc' : '4px solid #444'}}>
                                    {/* Card Header */}
                                    <div style={styles.cardHeader}>
                                        <input
                                            type="checkbox"
                                            checked={isSelected}
                                            onChange={() => toggleSelection(i)}
                                            style={styles.checkbox}
                                        />
                                        <div style={styles.headerInfo}>
                                            <input 
                                                style={styles.headerNameInput}
                                                value={config.name || ''}
                                                onChange={(e) => updateConfig(i, 'name', e.target.value)}
                                                placeholder="连接名称"
                                                onClick={(e) => e.stopPropagation()}
                                            />
                                            <span style={styles.headerHost}>{config.host}</span>
                                        </div>
                                        <div style={{display: 'flex', gap: '8px'}}>
                                            <button onClick={() => toggleExpand(i)} style={styles.iconButton} title="编辑">
                                                {isExpanded ? '🔽' : '✏️'}
                                            </button>
                                            <button onClick={() => handleRemoveConfig(i)} style={styles.iconButton} title="移除">
                                                🗑️
                                            </button>
                                        </div>
                                    </div>

                                    {/* Expanded Form */}
                                    {isExpanded && (
                                        <div style={styles.cardBody}>
                                            <ConnectionConfigForm
                                                config={config}
                                                onChange={(next) => updateConfigObject(i, next)}
                                                idPrefix={`smart-${i}`}
                                                showName={false}
                                                showGroup={false}
                                            />
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>
                    
                    <div style={styles.buttonGroup}>
                        <button onClick={handleAddManual} style={styles.secondaryButton}>+ 手动添加</button>
                        <div style={{flex: 1}}></div>
                        <button onClick={onClose} style={styles.cancelButton}>取消</button>
                        <button 
                            onClick={handleConnect} 
                            style={styles.submitButton}
                            disabled={selectedIndices.size === 0}
                        >
                            连接选中项 ({selectedIndices.size})
                        </button>
                    </div>
                </div>
            </div>
        </div>
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0, left: 0, right: 0, bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.7)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 1000,
    },
    modal: {
        backgroundColor: '#2d2d2d',
        padding: '24px',
        borderRadius: '8px',
        width: '700px',
        maxHeight: '90vh',
        display: 'flex',
        flexDirection: 'column' as const,
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: '#fff',
    },
    title: {
        marginTop: 0,
        marginBottom: '16px',
        fontSize: '1.5rem',
    },
    subtitle: {
        margin: 0,
        fontSize: '1rem',
        color: '#ccc',
    },
    inputSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
        marginBottom: '16px',
        borderBottom: '1px solid #444',
        paddingBottom: '16px',
    },
    textarea: {
        flex: 1,
        padding: '8px',
        borderRadius: '4px',
        border: '1px solid #444',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        fontSize: '0.9rem',
        resize: 'vertical' as const,
        boxSizing: 'border-box' as const,
        fontFamily: 'inherit',
    },
    aiButton: {
        padding: '0 16px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#8e44ad',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
    errorContainer: {
        backgroundColor: 'rgba(255, 107, 107, 0.1)',
        border: '1px solid #ff6b6b',
        borderRadius: '4px',
        padding: '8px',
    },
    errorMessage: {
        color: '#ff6b6b',
        fontSize: '0.9rem',
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    detailsLink: {
        textDecoration: 'underline',
        cursor: 'pointer',
        fontSize: '0.8rem',
        color: '#ff6b6b',
        marginLeft: '8px',
    },
    errorDetails: {
        marginTop: '8px',
        padding: '8px',
        backgroundColor: '#1e1e1e',
        borderRadius: '4px',
        fontSize: '0.8rem',
        color: '#ccc',
        whiteSpace: 'pre-wrap' as const,
        wordBreak: 'break-word' as const,
        maxHeight: '200px',
        overflowY: 'auto' as const,
        border: '1px solid #444',
    },
    resultSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '16px',
        overflow: 'hidden',
        flex: 1,
    },
    resultHeader: {
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center',
    },
    resetButton: {
        background: 'none',
        border: 'none',
        color: '#007acc',
        cursor: 'pointer',
        textDecoration: 'underline',
        fontSize: '0.9rem',
    },
    list: {
        overflowY: 'auto' as const,
        paddingRight: '4px',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
        minHeight: '200px',
    },
    card: {
        backgroundColor: '#383838',
        borderRadius: '4px',
        overflow: 'hidden',
    },
    cardHeader: {
        display: 'flex',
        alignItems: 'center',
        padding: '10px 12px',
        backgroundColor: '#444',
        cursor: 'pointer',
    },
    checkbox: {
        marginRight: '12px',
        cursor: 'pointer',
        width: '16px',
        height: '16px',
    },
    headerInfo: {
        flex: 1,
        display: 'flex',
        alignItems: 'center',
        gap: '12px',
    },
    headerNameInput: {
        backgroundColor: 'transparent',
        border: 'none',
        borderBottom: '1px solid #666',
        color: '#fff',
        fontSize: '1rem',
        fontWeight: 'bold' as const,
        width: '150px',
        padding: '2px 0',
    },
    headerHost: {
        color: '#aaa',
        fontSize: '0.9rem',
    },
    iconButton: {
        background: 'none',
        border: 'none',
        color: '#ccc',
        padding: '4px',
        fontSize: '1rem',
        cursor: 'pointer',
        marginLeft: '4px',
    },
    expandButton: {
        background: 'none',
        border: '1px solid #666',
        borderRadius: '4px',
        color: '#ccc',
        padding: '4px 8px',
        fontSize: '0.8rem',
        cursor: 'pointer',
    },
    cardBody: {
        padding: '16px',
        backgroundColor: '#333',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    row: {
        display: 'flex',
        gap: '16px',
    },
    fieldGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '4px',
    },
    fieldLabel: {
        fontSize: '0.8rem',
        color: '#aaa',
    },
    input: {
        padding: '8px',
        borderRadius: '4px',
        border: '1px solid #555',
        backgroundColor: '#222',
        color: '#fff',
        fontSize: '0.9rem',
    },
    bastionSection: {
        marginTop: '8px',
        borderTop: '1px solid #555',
        paddingTop: '8px',
    },
    bastionHeader: {
        display: 'flex',
        alignItems: 'center',
        marginBottom: '8px',
        color: '#e67e22',
        fontWeight: 'bold' as const,
        fontSize: '0.9rem',
    },
    bastionBody: {
        paddingLeft: '16px',
        borderLeft: '2px solid #e67e22',
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '12px',
    },
    buttonGroup: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        marginTop: '8px',
    },
    secondaryButton: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: '1px solid #555',
        backgroundColor: '#333',
        color: '#fff',
        cursor: 'pointer',
    },
    cancelButton: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: '1px solid #444',
        backgroundColor: 'transparent',
        color: '#fff',
        cursor: 'pointer',
    },
    submitButton: {
        padding: '8px 16px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    },
};

export default SmartConnectModal;
