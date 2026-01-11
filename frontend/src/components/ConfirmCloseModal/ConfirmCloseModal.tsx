import React from 'react';
import ReactDOM from 'react-dom';

interface ConfirmCloseModalProps {
    isOpen: boolean;
    message: string;
    onConfirm: () => void;
    onCancel: () => void;
}

const ConfirmCloseModal: React.FC<ConfirmCloseModalProps> = ({ isOpen, message, onConfirm, onCancel }) => {
    if (!isOpen) return null;

    return ReactDOM.createPortal(
        <div style={styles.overlay} onClick={(e) => {
            if (e.target === e.currentTarget) {
                onCancel();
            }
        }}>
            <div style={styles.modal}>
                <div style={styles.header}>
                    <div style={styles.iconContainer}>
                        <span style={styles.warningIcon}>⚠️</span>
                    </div>
                    <h3 style={styles.title}>确认关闭</h3>
                </div>

                <div style={styles.body}>
                    <p style={styles.message}>{message}</p>
                    <p style={styles.question}>确定要关闭吗？</p>
                </div>

                <div style={styles.footer}>
                    <button onClick={onCancel} style={styles.cancelButton}>
                        取消
                    </button>
                    <button onClick={onConfirm} style={styles.confirmButton}>
                        关闭应用
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
    warningIcon: {
        fontSize: '48px',
        filter: 'drop-shadow(0 2px 4px rgba(255, 152, 0, 0.3))',
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
    question: {
        color: '#fff',
        fontSize: '15px',
        fontWeight: 500,
        margin: 0,
    },
    footer: {
        padding: '16px 24px',
        borderTop: '1px solid #333',
        display: 'flex',
        justifyContent: 'flex-end',
        gap: '12px',
    },
    cancelButton: {
        padding: '8px 20px',
        backgroundColor: '#333',
        color: '#ccc',
        border: '1px solid #555',
        borderRadius: '4px',
        cursor: 'pointer',
        fontSize: '14px',
        fontWeight: 500,
        transition: 'all 0.2s',
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
`;
document.head.appendChild(styleSheet);

export default ConfirmCloseModal;
