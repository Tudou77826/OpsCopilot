import React, { useState } from 'react';

interface CommandCardProps {
    command: string;
    description: string;
}

const CommandCard: React.FC<CommandCardProps> = ({ command, description }) => {
    const [copied, setCopied] = useState(false);
    const [isHovered, setIsHovered] = useState(false);

    const handleCopy = () => {
        navigator.clipboard.writeText(command);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
    };

    return (
        <div 
            style={styles.card}
            onMouseEnter={() => setIsHovered(true)}
            onMouseLeave={() => setIsHovered(false)}
        >
            <style>{`
                .hide-scrollbar::-webkit-scrollbar {
                    display: none;
                }
                .hide-scrollbar {
                    -ms-overflow-style: none;
                    scrollbar-width: none;
                }
            `}</style>
            <div style={styles.header}>
                <span style={styles.description}>{description}</span>
            </div>
            <div style={styles.codeContainer}>
                <code style={styles.commandText} className="hide-scrollbar">{command}</code>
                {isHovered && (
                    <button onClick={handleCopy} style={styles.copyButton}>
                        {copied ? '已复制' : '复制'}
                    </button>
                )}
            </div>
        </div>
    );
};

const styles = {
    card: {
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '8px',
        padding: '12px',
        marginBottom: '12px',
        marginTop: '8px',
    },
    header: {
        marginBottom: '8px',
    },
    description: {
        color: '#aaa',
        fontSize: '12px',
        fontWeight: 'bold' as const,
    },
    codeContainer: {
        backgroundColor: '#000',
        padding: '8px',
        borderRadius: '4px',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between',
        minHeight: '36px',
    },
    commandText: {
        fontFamily: 'Consolas, monospace',
        fontSize: '13px',
        color: '#ce9178',
        flex: 1,
        overflowX: 'auto' as const,
        whiteSpace: 'nowrap' as const,
        marginRight: '8px',
    },
    copyButton: {
        padding: '4px 12px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '12px',
        whiteSpace: 'nowrap' as const,
    }
};

export default CommandCard;
