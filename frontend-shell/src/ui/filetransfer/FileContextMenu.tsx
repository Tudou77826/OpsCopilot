import React, { useEffect, useLayoutEffect, useRef, useState } from 'react';
import ReactDOM from 'react-dom';
import { usePortalRoot } from '../Surface';

export interface ContextMenuItem {
    label: string;
    danger?: boolean;
    disabled?: boolean;
    onClick: () => void;
}

interface FileContextMenuProps {
    x: number;
    y: number;
    items: ContextMenuItem[];
    onClose: () => void;
}

const MENU_WIDTH = 168;

// 可复用的右键菜单：fixed 定位、边缘翻转、点击外部 / ESC 关闭。
const FileContextMenu: React.FC<FileContextMenuProps> = ({ x, y, items, onClose }) => {
    const portalRoot = usePortalRoot();
    const ref = useRef<HTMLDivElement | null>(null);
    const [pos, setPos] = useState<{ left: number; top: number }>({ left: x, top: y });

    useEffect(() => {
        const onGlobalClick = () => onClose();
        const onKeyDown = (e: KeyboardEvent) => {
            if (e.key === 'Escape') onClose();
        };
        const onContextMenuElsewhere = () => onClose();
        window.addEventListener('mousedown', onGlobalClick);
        window.addEventListener('keydown', onKeyDown);
        window.addEventListener('contextmenu', onContextMenuElsewhere);
        window.addEventListener('blur', onGlobalClick);
        return () => {
            window.removeEventListener('mousedown', onGlobalClick);
            window.removeEventListener('keydown', onKeyDown);
            window.removeEventListener('contextmenu', onContextMenuElsewhere);
            window.removeEventListener('blur', onGlobalClick);
        };
    }, [onClose]);

    // 计算翻转位置，避免菜单超出窗口
    useLayoutEffect(() => {
        const el = ref.current;
        if (!el) return;
        const rect = el.getBoundingClientRect();
        let left = x;
        let top = y;
        if (left + rect.width > window.innerWidth - 4) {
            left = Math.max(4, window.innerWidth - rect.width - 4);
        }
        if (top + rect.height > window.innerHeight - 4) {
            top = Math.max(4, window.innerHeight - rect.height - 4);
        }
        setPos({ left, top });
    }, [x, y]);

    return ReactDOM.createPortal(
        <div
            ref={ref}
            style={{ ...styles.menu, left: pos.left, top: pos.top }}
            onContextMenu={(e) => e.stopPropagation()}
            onMouseDown={(e) => e.stopPropagation()}
            data-testid="file-context-menu"
        >
            {items.map((item, idx) => (
                <button
                    key={idx}
                    style={{
                        ...styles.item,
                        ...(item.danger ? styles.itemDanger : null),
                        ...(item.disabled ? styles.itemDisabled : null),
                    }}
                    disabled={item.disabled}
                    onClick={(e) => {
                        e.stopPropagation();
                        onClose();
                        if (!item.disabled) item.onClick();
                    }}
                >
                    {item.label}
                </button>
            ))}
        </div>,
        portalRoot
    );
};

const styles: Record<string, React.CSSProperties> = {
    menu: {
        position: 'fixed',
        zIndex: 999999,
        minWidth: `${MENU_WIDTH}px`,
        backgroundColor: 'var(--bg-secondary)',
        border: '1px solid var(--border)',
        borderRadius: '8px',
        boxShadow: '0 8px 24px rgba(0,0,0,0.5)',
        padding: '4px',
        display: 'flex',
        flexDirection: 'column',
    },
    item: {
        textAlign: 'left',
        padding: '7px 10px',
        borderRadius: '5px',
        border: 'none',
        backgroundColor: 'transparent',
        color: 'var(--text-primary)',
        fontSize: '12px',
        cursor: 'pointer',
        width: '100%',
    },
    itemDanger: {
        color: 'var(--severity-danger)',
    },
    itemDisabled: {
        opacity: 0.4,
        cursor: 'not-allowed',
    },
};

export default FileContextMenu;
