import React, { useEffect } from 'react';
import FilesPanel from '../Sidebar/FilesPanel';

interface TerminalSessionLite {
    id: string;
    title: string;
}

interface FileTransferWindowProps {
    isOpen: boolean;
    onClose: () => void;
    activeTerminalId: string | null;
    terminals: TerminalSessionLite[];
}

const FileTransferWindow: React.FC<FileTransferWindowProps> = ({ isOpen, onClose, activeTerminalId, terminals }) => {
    useEffect(() => {
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') {
                onClose();
            }
        };
        if (isOpen) {
            window.addEventListener('keydown', onKeyDown);
        }
        return () => window.removeEventListener('keydown', onKeyDown);
    }, [isOpen, onClose]);

    if (!isOpen) return null;

    return (
        <div style={styles.overlay}>
            <div style={styles.window}>
                <div style={styles.header}>
                    <div style={styles.title}>文件传输</div>
                    <div style={{ flex: 1 }} />
                    <button style={styles.closeBtn} onClick={onClose} aria-label="关闭">
                        ×
                    </button>
                </div>
                <div style={styles.body}>
                    <FilesPanel activeTerminalId={activeTerminalId} terminals={terminals} />
                </div>
            </div>
        </div>
    );
};

const styles: Record<string, React.CSSProperties> = {
    overlay: {
        position: 'fixed',
        left: 0,
        top: 0,
        right: 0,
        bottom: 0,
        zIndex: 4000,
        backgroundColor: 'rgba(0,0,0,0.6)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center'
    },
    window: {
        width: 'calc(100vw - 80px)',
        height: 'calc(100vh - 80px)',
        backgroundColor: '#1e1e1e',
        border: '1px solid #333',
        borderRadius: '10px',
        overflow: 'hidden',
        display: 'flex',
        flexDirection: 'column'
    },
    header: {
        height: '44px',
        display: 'flex',
        alignItems: 'center',
        padding: '0 12px',
        backgroundColor: '#252526',
        borderBottom: '1px solid #333',
        gap: '10px'
    },
    title: {
        fontSize: '13px',
        color: '#fff',
        fontWeight: 600
    },
    closeBtn: {
        width: '32px',
        height: '28px',
        borderRadius: '6px',
        border: '1px solid #3c3c3c',
        backgroundColor: '#1e1e1e',
        color: '#ddd',
        cursor: 'pointer',
        fontSize: '18px',
        lineHeight: '18px'
    },
    body: {
        flex: 1,
        minHeight: 0,
        overflow: 'auto'
    }
};

export default FileTransferWindow;
