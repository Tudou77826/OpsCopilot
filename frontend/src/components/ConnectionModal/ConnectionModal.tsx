import React, { useState } from 'react';

interface ConnectionConfig {
    host: string;
    port: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: ConnectionConfig;
}

interface ConnectionModalProps {
    isOpen: boolean;
    onClose: () => void;
    onConnect: (config: ConnectionConfig) => void;
}

const ConnectionModal: React.FC<ConnectionModalProps> = ({ isOpen, onClose, onConnect }) => {
    const [config, setConfig] = useState<ConnectionConfig>({
        host: '39.108.107.148',
        port: 22,
        user: 'root',
        password: 'zhangyibo123.',
        rootPassword: '',
    });
    
    const [enableBastion, setEnableBastion] = useState(false);
    const [bastionConfig, setBastionConfig] = useState<ConnectionConfig>({
        host: '',
        port: 22,
        user: '',
        password: ''
    });

    if (!isOpen) return null;

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        const finalConfig = { ...config };
        if (enableBastion) {
            finalConfig.bastion = bastionConfig;
        }
        onConnect(finalConfig);
        onClose();
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value } = e.target;
        setConfig(prev => ({
            ...prev,
            [name]: name === 'port' ? parseInt(value) || 22 : value
        }));
    };
    
    const handleBastionChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value } = e.target;
        setBastionConfig(prev => ({
            ...prev,
            [name]: name === 'port' ? parseInt(value) || 22 : value
        }));
    };

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <h2 style={styles.title}>New Connection</h2>
                <form onSubmit={handleSubmit} style={styles.form}>
                    {/* Main Connection */}
                    <div style={styles.section}>
                        <h3 style={styles.subtitle}>Target Host</h3>
                        <div style={styles.formGroup}>
                            <label style={styles.label} htmlFor="host">Host</label>
                            <input
                                id="host"
                                type="text"
                                name="host"
                                value={config.host}
                                onChange={handleChange}
                                style={styles.input}
                                required
                            />
                        </div>
                        <div style={styles.row}>
                            <div style={styles.formGroup}>
                                <label style={styles.label} htmlFor="port">Port</label>
                                <input
                                    id="port"
                                    type="number"
                                    name="port"
                                    value={config.port}
                                    onChange={handleChange}
                                    style={styles.input}
                                    required
                                />
                            </div>
                            <div style={styles.formGroup}>
                                <label style={styles.label} htmlFor="user">User</label>
                                <input
                                    id="user"
                                    type="text"
                                    name="user"
                                    value={config.user}
                                    onChange={handleChange}
                                    style={styles.input}
                                    required
                                />
                            </div>
                        </div>
                        <div style={styles.formGroup}>
                            <label style={styles.label} htmlFor="password">Password</label>
                            <input
                                id="password"
                                type="password"
                                name="password"
                                value={config.password}
                                onChange={handleChange}
                                style={styles.input}
                            />
                        </div>
                        <div style={styles.formGroup}>
                            <label style={styles.label} htmlFor="rootPassword">Root Password (Optional)</label>
                            <input
                                id="rootPassword"
                                type="password"
                                name="rootPassword"
                                value={config.rootPassword}
                                onChange={handleChange}
                                style={styles.input}
                                placeholder="For auto-sudo"
                            />
                        </div>
                    </div>

                    {/* Bastion Toggle */}
                    <div style={styles.section}>
                        <div style={styles.checkboxGroup}>
                            <input 
                                type="checkbox" 
                                id="enableBastion"
                                checked={enableBastion}
                                onChange={e => setEnableBastion(e.target.checked)}
                            />
                            <label htmlFor="enableBastion" style={styles.subtitle}>Bastion Host (Optional)</label>
                        </div>
                        <span style={{fontSize: '0.8rem', color: '#888'}}>Enable Bastion</span>
                        
                        {enableBastion && (
                            <div style={{marginTop: '10px', paddingLeft: '10px', borderLeft: '2px solid #444'}}>
                                <div style={styles.formGroup}>
                                    <label style={styles.label} htmlFor="bastion-host">Host</label>
                                    <input
                                        id="bastion-host"
                                        type="text"
                                        name="host"
                                        value={bastionConfig.host}
                                        onChange={handleBastionChange}
                                        style={styles.input}
                                        required={enableBastion}
                                    />
                                </div>
                                <div style={styles.row}>
                                    <div style={styles.formGroup}>
                                        <label style={styles.label} htmlFor="bastion-port">Port</label>
                                        <input
                                            id="bastion-port"
                                            type="number"
                                            name="port"
                                            value={bastionConfig.port}
                                            onChange={handleBastionChange}
                                            style={styles.input}
                                            required={enableBastion}
                                        />
                                    </div>
                                    <div style={styles.formGroup}>
                                        <label style={styles.label} htmlFor="bastion-user">User</label>
                                        <input
                                            id="bastion-user"
                                            type="text"
                                            name="user"
                                            value={bastionConfig.user}
                                            onChange={handleBastionChange}
                                            style={styles.input}
                                            required={enableBastion}
                                        />
                                    </div>
                                </div>
                                <div style={styles.formGroup}>
                                    <label style={styles.label} htmlFor="bastion-password">Password</label>
                                    <input
                                        id="bastion-password"
                                        type="password"
                                        name="password"
                                        value={bastionConfig.password}
                                        onChange={handleBastionChange}
                                        style={styles.input}
                                    />
                                </div>
                            </div>
                        )}
                    </div>

                    <div style={styles.buttonGroup}>
                        <button type="button" onClick={onClose} style={styles.cancelButton}>Cancel</button>
                        <button type="submit" style={styles.submitButton}>Connect</button>
                    </div>
                </form>
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
        width: '500px',
        maxHeight: '90vh',
        overflowY: 'auto' as const,
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: '#fff',
    },
    title: {
        marginTop: 0,
        marginBottom: '20px',
        fontSize: '1.5rem',
    },
    subtitle: {
        fontSize: '1.1rem',
        margin: '0',
        color: '#eee',
    },
    section: {
        marginBottom: '16px',
    },
    form: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '16px',
    },
    formGroup: {
        display: 'flex',
        flexDirection: 'column' as const,
        gap: '8px',
        marginBottom: '8px',
        flex: 1,
    },
    row: {
        display: 'flex',
        gap: '16px',
    },
    label: {
        fontSize: '0.9rem',
        color: '#ccc',
    },
    input: {
        padding: '8px 12px',
        borderRadius: '4px',
        border: '1px solid #444',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        fontSize: '1rem',
        outline: 'none',
        width: '100%',
        boxSizing: 'border-box' as const,
    },
    checkboxGroup: {
        display: 'flex',
        alignItems: 'center',
        gap: '8px',
    },
    buttonGroup: {
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
        marginTop: '8px',
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

export default ConnectionModal;
