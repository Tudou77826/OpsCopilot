import React, { useState, useEffect } from 'react';

interface ConnectionConfig {
    name?: string;
    host: string;
    port: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: ConnectionConfig;
}

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
            const configs = await onParse(input);
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
            name: 'New Connection'
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
        const toConnect = parsedConfigs.filter((_, i) => selectedIndices.has(i));
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
                if (!config.name || config.name === config.host || config.name === 'New Connection') {
                    config.name = value;
                }
            }
            (config as any)[field] = value;
        }

        newConfigs[index] = config;
        setParsedConfigs(newConfigs);
    };

    const renderField = (label: string, value: string | number, onChange: (val: string) => void, type: string = "text", placeholder: string = "", id?: string) => (
        <div style={styles.fieldGroup}>
            <label style={styles.fieldLabel} htmlFor={id}>{label}</label>
            <input
                id={id}
                type={type}
                value={value}
                onChange={(e) => onChange(e.target.value)}
                style={styles.input}
                placeholder={placeholder}
            />
        </div>
    );

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <h2 style={styles.title}>New Connection</h2>
                
                {/* AI Input Section - Always visible but compact */}
                <div style={styles.inputSection}>
                    <div style={{display: 'flex', gap: '8px'}}>
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder="AI Magic: 'Connect to 192.168.1.10 using root'..."
                            style={styles.textarea}
                            rows={2}
                        />
                        <button 
                            onClick={handleParse} 
                            style={styles.aiButton}
                            disabled={isLoading || !input.trim()}
                        >
                            {isLoading ? '...' : 'Analyze'}
                        </button>
                    </div>
                    {error && (
                        <div style={styles.errorContainer}>
                            <div style={styles.errorMessage}>
                                <span>⚠️ {error.includes('Raw:') ? 'Parsing Error' : error}</span>
                                {error.includes('Raw:') && (
                                    <span 
                                        style={styles.detailsLink} 
                                        onClick={() => setShowErrorDetails(!showErrorDetails)}
                                    >
                                        {showErrorDetails ? 'Hide Details' : 'Show Details'}
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
                        <h3 style={styles.subtitle}>Connections ({parsedConfigs.length})</h3>
                        {/* "New Search" removed as requested */}
                    </div>
                    
                    <div style={styles.list}>
                        {parsedConfigs.length === 0 && (
                            <div style={{textAlign: 'center', padding: '20px', color: '#666', border: '1px dashed #444', borderRadius: '4px'}}>
                                No connections yet. Use AI above or add manually.
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
                                                placeholder="Connection Name"
                                                onClick={(e) => e.stopPropagation()}
                                            />
                                            <span style={styles.headerHost}>{config.host}</span>
                                        </div>
                                        <div style={{display: 'flex', gap: '8px'}}>
                                            <button onClick={() => toggleExpand(i)} style={styles.iconButton} title="Edit">
                                                {isExpanded ? '🔽' : '✏️'}
                                            </button>
                                            <button onClick={() => handleRemoveConfig(i)} style={styles.iconButton} title="Remove">
                                                🗑️
                                            </button>
                                        </div>
                                    </div>

                                    {/* Expanded Form */}
                                    {isExpanded && (
                                        <div style={styles.cardBody}>
                                            <div style={styles.row}>
                                                <div style={{flex: 2}}>{renderField("Host", config.host, (v) => updateConfig(i, 'host', v), "text", "", `host-${i}`)}</div>
                                                <div style={{flex: 1}}>{renderField("Port", config.port, (v) => updateConfig(i, 'port', parseInt(v) || 22), "number", "", `port-${i}`)}</div>
                                            </div>
                                            <div style={styles.row}>
                                                <div style={{flex: 1}}>{renderField("User", config.user, (v) => updateConfig(i, 'user', v), "text", "", `user-${i}`)}</div>
                                                <div style={{flex: 1}}>{renderField("Password", config.password || '', (v) => updateConfig(i, 'password', v), "password", "", `password-${i}`)}</div>
                                            </div>
                                            <div style={styles.row}>
                                                <div style={{flex: 1}}>{renderField("Root Password", config.rootPassword || '', (v) => updateConfig(i, 'rootPassword', v), "password", "Optional (for sudo)", `root-password-${i}`)}</div>
                                            </div>

                                            {/* Bastion Config */}
                                            <div style={styles.bastionSection}>
                                                <label style={styles.bastionHeader}>
                                                    <input 
                                                        type="checkbox" 
                                                        checked={!!config.bastion}
                                                        onChange={(e) => {
                                                            if (e.target.checked) {
                                                                updateConfig(i, 'bastion.host', ''); // Initialize bastion
                                                            } else {
                                                                const newConfigs = [...parsedConfigs];
                                                                delete newConfigs[i].bastion;
                                                                setParsedConfigs(newConfigs);
                                                            }
                                                        }}
                                                        style={{marginRight: '8px'}}
                                                    />
                                                    <span>Use Bastion Host</span>
                                                </label>
                                                {config.bastion && (
                                                    <div style={styles.bastionBody}>
                                                        <div style={styles.row}>
                                                            <div style={{flex: 2}}>{renderField("Bastion Host", config.bastion.host, (v) => updateConfig(i, 'bastion.host', v), "text", "", `bastion-host-${i}`)}</div>
                                                            <div style={{flex: 1}}>{renderField("Bastion Port", config.bastion.port, (v) => updateConfig(i, 'bastion.port', parseInt(v) || 22), "number", "", `bastion-port-${i}`)}</div>
                                                        </div>
                                                        <div style={styles.row}>
                                                            <div style={{flex: 1}}>{renderField("Bastion User", config.bastion.user, (v) => updateConfig(i, 'bastion.user', v), "text", "", `bastion-user-${i}`)}</div>
                                                            <div style={{flex: 1}}>{renderField("Bastion Password", config.bastion.password || '', (v) => updateConfig(i, 'bastion.password', v), "password", "", `bastion-password-${i}`)}</div>
                                                        </div>
                                                    </div>
                                                )}
                                            </div>
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>
                    
                    <div style={styles.buttonGroup}>
                        <button onClick={handleAddManual} style={styles.secondaryButton}>+ Add Manual Entry</button>
                        <div style={{flex: 1}}></div>
                        <button onClick={onClose} style={styles.cancelButton}>Cancel</button>
                        <button 
                            onClick={handleConnect} 
                            style={styles.submitButton}
                            disabled={selectedIndices.size === 0}
                        >
                            Connect Selected ({selectedIndices.size})
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
