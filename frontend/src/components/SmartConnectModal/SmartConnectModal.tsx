import React, { useState } from 'react';

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
    const [error, setError] = useState('');
    const [showErrorDetails, setShowErrorDetails] = useState(false);

    if (!isOpen) return null;

    const handleParse = async () => {
        if (!input.trim()) return;
        
        setIsLoading(true);
        setError('');
        setShowErrorDetails(false);
        try {
            const configs = await onParse(input);
            setParsedConfigs(configs);
            // Default select all
            setSelectedIndices(new Set(configs.map((_, i) => i)));
        } catch (e: any) {
            // Backend returns "AI provider error: ..." or "failed to parse..." or "config #... missing..."
            // Wails wraps errors in "Error: " prefix sometimes
            let errorMsg = e.message || e.toString();
            
            // Strip common prefixes to make it cleaner
            errorMsg = errorMsg.replace(/^Error: /, '');
            
            setError(errorMsg);
        } finally {
            setIsLoading(false);
        }
    };

    const handleConnect = () => {
        const toConnect = parsedConfigs.filter((_, i) => selectedIndices.has(i));
        onConnect(toConnect);
        onClose();
        // Reset state
        setInput('');
        setParsedConfigs([]);
        setSelectedIndices(new Set());
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

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <h2 style={styles.title}>Smart Connect (AI)</h2>
                
                {parsedConfigs.length === 0 ? (
                    <div style={styles.inputSection}>
                        <textarea
                            value={input}
                            onChange={(e) => setInput(e.target.value)}
                            placeholder="Describe your connection intent...&#10;e.g. 'Connect to 192.168.1.10 and 1.11 using user root password 123'"
                            style={styles.textarea}
                            rows={4}
                        />
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
                        <div style={styles.buttonGroup}>
                            <button onClick={onClose} style={styles.cancelButton}>Cancel</button>
                            <button 
                                onClick={handleParse} 
                                style={styles.submitButton}
                                disabled={isLoading || !input.trim()}
                            >
                                {isLoading ? 'Parsing...' : 'Analyze Intent'}
                            </button>
                        </div>
                    </div>
                ) : (
                    <div style={styles.resultSection}>
                        <h3 style={styles.subtitle}>Found {parsedConfigs.length} connections:</h3>
                        <div style={styles.list}>
                            {parsedConfigs.map((config, i) => (
                                <div key={i} style={styles.listItem}>
                                    <input
                                        type="checkbox"
                                        checked={selectedIndices.has(i)}
                                        onChange={() => toggleSelection(i)}
                                        style={{marginRight: '10px'}}
                                    />
                                    <div style={{flex: 1}}>
                                        <div style={{fontWeight: 'bold', display: 'flex', justifyContent: 'space-between'}}>
                                            <span>{config.host}</span>
                                            <span style={{fontSize: '0.8rem', color: '#888', fontWeight: 'normal'}}>{config.name || ''}</span>
                                        </div>
                                        <div style={{fontSize: '0.85rem', color: '#aaa', marginTop: '4px'}}>
                                            <span style={{color: '#4caf50'}}>{config.user}</span>
                                            <span style={{margin: '0 4px'}}>@</span>
                                            <span>{config.port}</span>
                                            {config.password && <span style={{marginLeft: '8px', color: '#888'}}>(pwd: {config.password})</span>}
                                            {config.rootPassword && <span style={{marginLeft: '8px', color: '#e74c3c'}}>(root: {config.rootPassword})</span>}
                                        </div>
                                        {config.bastion && (
                                            <div style={{fontSize: '0.8rem', color: '#d35400', marginTop: '2px', paddingLeft: '8px', borderLeft: '2px solid #555'}}>
                                                Via: {config.bastion.host} ({config.bastion.user})
                                            </div>
                                        )}
                                    </div>
                                </div>
                            ))}
                        </div>
                        <div style={styles.buttonGroup}>
                            <button onClick={() => setParsedConfigs([])} style={styles.cancelButton}>Back</button>
                            <button 
                                onClick={handleConnect} 
                                style={styles.submitButton}
                                disabled={selectedIndices.size === 0}
                            >
                                Connect Selected ({selectedIndices.size})
                            </button>
                        </div>
                    </div>
                )}
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
        zIndex: 1000,
    },
    modal: {
        backgroundColor: '#2d2d2d',
        padding: '24px',
        borderRadius: '8px',
        width: '600px',
        maxHeight: '90vh',
        overflowY: 'auto' as const,
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: '#fff',
    },
    title: {
        marginTop: 0,
        marginBottom: '16px',
        fontSize: '1.5rem',
    },
    subtitle: {
        margin: '0 0 10px 0',
        fontSize: '1rem',
        color: '#ccc',
    },
    inputSection: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '16px',
    },
    textarea: {
        width: '100%',
        padding: '12px',
        borderRadius: '4px',
        border: '1px solid #444',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        fontSize: '1rem',
        resize: 'vertical' as const,
        boxSizing: 'border-box' as const,
        fontFamily: 'inherit',
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
    },
    list: {
        maxHeight: '300px',
        overflowY: 'auto' as const,
        border: '1px solid #444',
        borderRadius: '4px',
        backgroundColor: '#1e1e1e',
    },
    listItem: {
        display: 'flex',
        alignItems: 'center',
        padding: '10px 12px',
        borderBottom: '1px solid #333',
    },
    buttonGroup: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
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
