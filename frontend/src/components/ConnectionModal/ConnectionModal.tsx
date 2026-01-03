import React, { useState } from 'react';

interface ConnectionConfig {
    host: string;
    port: number;
    user: string;
    password?: string;
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
        password: 'zhangyibo123.'
    });

    if (!isOpen) return null;

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        onConnect(config);
        onClose();
    };

    const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
        const { name, value } = e.target;
        setConfig(prev => ({
            ...prev,
            [name]: name === 'port' ? parseInt(value) || 22 : value
        }));
    };

    return (
        <div style={styles.overlay}>
            <div style={styles.modal}>
                <h2 style={styles.title}>New Connection</h2>
                <form onSubmit={handleSubmit} style={styles.form}>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>Host</label>
                        <input
                            type="text"
                            name="host"
                            value={config.host}
                            onChange={handleChange}
                            style={styles.input}
                            required
                        />
                    </div>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>Port</label>
                        <input
                            type="number"
                            name="port"
                            value={config.port}
                            onChange={handleChange}
                            style={styles.input}
                            required
                        />
                    </div>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>User</label>
                        <input
                            type="text"
                            name="user"
                            value={config.user}
                            onChange={handleChange}
                            style={styles.input}
                            required
                        />
                    </div>
                    <div style={styles.formGroup}>
                        <label style={styles.label}>Password</label>
                        <input
                            type="password"
                            name="password"
                            value={config.password}
                            onChange={handleChange}
                            style={styles.input}
                        />
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
        width: '400px',
        boxShadow: '0 4px 6px rgba(0, 0, 0, 0.1)',
        color: '#fff',
    },
    title: {
        marginTop: 0,
        marginBottom: '20px',
        fontSize: '1.5rem',
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
