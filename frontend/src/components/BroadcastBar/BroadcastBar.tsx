import React, { useState } from 'react';

interface BroadcastBarProps {
    onBroadcast: (command: string) => void;
}

const BroadcastBar: React.FC<BroadcastBarProps> = ({ onBroadcast }) => {
    const [command, setCommand] = useState('');

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault();
        if (command.trim()) {
            onBroadcast(command);
            setCommand('');
        }
    };

    return (
        <form onSubmit={handleSubmit} style={styles.container}>
            <span style={styles.label}>Broadcast:</span>
            <input
                type="text"
                value={command}
                onChange={(e) => setCommand(e.target.value)}
                placeholder="Type command to send to all active terminals..."
                style={styles.input}
            />
            <button type="submit" style={styles.button}>Send</button>
        </form>
    );
};

const styles = {
    container: {
        display: 'flex',
        alignItems: 'center',
        padding: '8px 16px',
        backgroundColor: '#252526',
        borderTop: '1px solid #1e1e1e',
        gap: '12px',
    },
    label: {
        color: '#ccc',
        fontSize: '0.9rem',
        fontWeight: 'bold' as const,
    },
    input: {
        flex: 1,
        padding: '6px 12px',
        borderRadius: '4px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#fff',
        outline: 'none',
    },
    button: {
        padding: '6px 16px',
        borderRadius: '4px',
        border: 'none',
        backgroundColor: '#007acc',
        color: '#fff',
        cursor: 'pointer',
        fontWeight: 'bold' as const,
    }
};

export default BroadcastBar;
