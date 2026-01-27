import React from 'react';
import ReactDOM from 'react-dom';

interface ErrorModalProps {
    isOpen: boolean;
    title: string;
    message: string;
    onConfirm: () => void;
    showDetails?: boolean;
    details?: string;
}

const ErrorModal: React.FC<ErrorModalProps> = ({ 
    isOpen, 
    title, 
    message, 
    onConfirm, 
    showDetails = false, 
    details 
}) => {
    if (!isOpen) return null;

    return ReactDOM.createPortal(
        <div style={styles.overlay} onClick={(e) => {
            if (e.target === e.currentTarget) {
                onConfirm();
            }
        }}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <div style={styles.iconContainer}>
                        <span style={styles.errorIcon}>❌</span>
                    </div>
                    <h3 style={styles.title}>{title}</h3>
                </div>

                <div style={styles.body}>
                    <p style={styles.message}>{message}</p>
                    
                    {showDetails && details && (
                        <details style={styles.details}>
                            <summary style={styles.detailsSummary}>查看详情</summary>
                            <pre style={styles.detailsContent}>
                                {details}
                            </pre>
                        </details>
                    )}
                </div>

                <div style={styles.footer}>
                    <button onClick={onConfirm} style={styles.confirmButton}>
                        确定
                    </button>
                </div>
            </div>
        </div>,
        document.body
    );
};

const styles = {
    overlay: {
        position: 'fixed' as const,
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.85)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        zIndex: 999999,
        backdropFilter: 'blur(4px)',
        animation: 'fadeIn 0.2s ease-out',
    },
    modal: {
        backgroundColor: '#252526',
        width: '460px',
        maxWidth: '90%',
        borderRadius: '8px',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.6)',
        border: '1px solid #444',
        animation: 'slideIn 0.2s ease-out',
    },
    header: {
        padding: '24px 24px 16px 24px',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
    },
    iconContainer: {
        marginBottom: '12px',
    },
    errorIcon: {
        fontSize: '48px',
        filter: 'drop-shadow(0 2px 4px rgba(221, 44, 44, 0.3))',
    },
    title: {
        margin: 0,
        color: '#fff',
        fontSize: '20px',
        fontWeight: 600,
    },
    body: {
        padding: '0 24px 24px 24px',
        textAlign: 'center' as const,
    },
    message: {
        color: '#ccc',
        fontSize: '14px',
        lineHeight: '1.6',
        margin: '0 0 16px 0',
    },
    details: {
        marginTop: '16px',
        textAlign: 'left' as const,
        backgroundColor: '#1e1e1e',
        padding: '12px',
        borderRadius: '4px',
        overflow: 'auto',
        maxHeight: '200px',
        border: '1px solid #333',
    },
    detailsSummary: {
        color: '#ccc',
        fontSize: '12px',
        cursor: 'pointer',
        outline: 'none',
        fontWeight: 'normal' as const,
    },
    detailsContent: {
        margin: '8px 0 0 0',
        color: '#aaa',
        fontSize: '12px',
        whiteSpace: 'pre-wrap' as const,
        fontFamily: 'monospace',
        overflow: 'auto',
        maxHeight: '150px',
        padding: '8px',
        backgroundColor: '#1a1a1a',
        borderRadius: '2px',
        border: '1px solid #333',
    },
    footer: {
        padding: '16px 24px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
    },
    confirmButton: {
        padding: '8px 20px',
        backgroundColor: '#d32f2f',
        color: 'white',
        border: 'none',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '14px',
        fontWeight: 600,
        transition: 'all 0.2s',
    },
};

// Add CSS animations
const styleSheet = document.createElement("style");
styleSheet.textContent = `
    @keyframes fadeIn {
        from {
            opacity: 0;
        }
        to {
            opacity: 1;
        }
    }
    
    @keyframes slideIn {
        from {
            transform: translateY(-20px);
            opacity: 0;
        }
        to {
            transform: translateY(0);
            opacity: 1;
        }
    }
    
    button:hover {
        filter: brightness(1.1);
    }
    
    button:active {
        transform: scale(0.98);
    }
    
    details > summary {
        list-style: none;
    }
    
    details > summary::-webkit-details-marker {
        display: none;
    }
`;
document.head.appendChild(styleSheet);

export default ErrorModal;