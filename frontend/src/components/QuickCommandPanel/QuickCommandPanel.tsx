import React, { useState, useRef, useCallback } from 'react';
import { QuickCommand } from './types';
import { useQuickCommands } from './useQuickCommands';
import GroupStrip from './GroupStrip';
import CommandGrid from './CommandGrid';
import CommandEditModal from './CommandEditModal';

interface QuickCommandPanelProps {
    isOpen: boolean;
    onExecute: (content: string) => void;
}

// —— 面板高度 / 分组条宽度：可拖拽调整并持久化（与文件传输队列的调高交互一致）——
const PANEL_HEIGHT_KEY = 'opscopilot-quickcmd-panel-height';
const STRIP_WIDTH_KEY = 'opscopilot-quickcmd-strip-width';
const PANEL_MIN_HEIGHT = 96;
const PANEL_MAX_HEIGHT = 400;
const STRIP_MIN_WIDTH = 56;
const STRIP_MAX_WIDTH = 140;
const STRIP_DEFAULT_WIDTH = 64;

function readStoredNumber(key: string, min: number, max: number): number | null {
    try {
        const v = Number(localStorage.getItem(key));
        return Number.isFinite(v) && v >= min && v <= max ? v : null;
    } catch {
        return null;
    }
}

const QuickCommandPanel: React.FC<QuickCommandPanelProps> = ({ isOpen, onExecute }) => {
    const {
        availableGroups,
        selectedGroup,
        setSelectedGroup,
        filteredCommands,
        addCommand,
        updateCommand,
        deleteCommand,
        reorderCommands,
    } = useQuickCommands();

    const [editingCmd, setEditingCmd] = useState<QuickCommand | null>(null);
    const [isNewCommand, setIsNewCommand] = useState(false);
    // 搜索关键字：在当前分组内进一步过滤（按 name/content 匹配，大小写不敏感）（issue #56）
    const [searchQuery, setSearchQuery] = useState('');

    // null = 未自定义：面板保持内容自适应（上限 200px）的现有行为
    const [panelHeight, setPanelHeight] = useState<number | null>(
        () => readStoredNumber(PANEL_HEIGHT_KEY, PANEL_MIN_HEIGHT, PANEL_MAX_HEIGHT));
    const [stripWidth, setStripWidth] = useState<number>(
        () => readStoredNumber(STRIP_WIDTH_KEY, STRIP_MIN_WIDTH, STRIP_MAX_WIDTH) ?? STRIP_DEFAULT_WIDTH);
    const [resizing, setResizing] = useState(false);
    const panelRef = useRef<HTMLDivElement>(null);
    const resizeSession = useRef({ mode: '' as '' | 'height' | 'width', startX: 0, startY: 0, startH: 0, startW: 0 });

    const beginResize = useCallback((mode: 'height' | 'width') => (e: React.PointerEvent) => {
        e.preventDefault();
        resizeSession.current = {
            mode,
            startX: e.clientX,
            startY: e.clientY,
            // 高度以面板当前真实高度为基准：未自定义时也能从内容高度直接拖
            startH: mode === 'height' ? (panelRef.current?.clientHeight ?? 200) : 0,
            startW: stripWidth,
        };
        setResizing(true);
        try { (e.currentTarget as Element).setPointerCapture(e.pointerId); } catch { /* jsdom */ }
    }, [stripWidth]);

    const handleResizeMove = useCallback((e: React.PointerEvent) => {
        const s = resizeSession.current;
        if (s.mode === 'height') {
            // 上拖调高
            const h = Math.round(Math.min(PANEL_MAX_HEIGHT, Math.max(PANEL_MIN_HEIGHT, s.startH - (e.clientY - s.startY))));
            setPanelHeight(h);
        } else if (s.mode === 'width') {
            // 左拖调宽（手柄在分组条左缘）
            const w = Math.round(Math.min(STRIP_MAX_WIDTH, Math.max(STRIP_MIN_WIDTH, s.startW - (e.clientX - s.startX))));
            setStripWidth(w);
        }
    }, []);

    const endResize = useCallback(() => {
        const s = resizeSession.current;
        if (s.mode === 'height' && panelHeight != null) {
            try { localStorage.setItem(PANEL_HEIGHT_KEY, String(panelHeight)); } catch { /* 忽略 */ }
        } else if (s.mode === 'width') {
            try { localStorage.setItem(STRIP_WIDTH_KEY, String(stripWidth)); } catch { /* 忽略 */ }
        }
        resizeSession.current.mode = '';
        setResizing(false);
    }, [panelHeight, stripWidth]);

    // 切换分组时清空搜索词：搜索作用于「当前分组」，换组后旧关键词通常无意义
    const handleSelectGroup = (group: string) => {
        setSelectedGroup(group);
        setSearchQuery('');
    };

    // 在当前分组命令之上，按关键字过滤。空关键字时显示全部当前分组命令。
    const visibleCommands = (() => {
        const q = searchQuery.trim().toLowerCase();
        if (!q) return filteredCommands;
        return filteredCommands.filter(cmd =>
            cmd.name.toLowerCase().includes(q) || cmd.content.toLowerCase().includes(q)
        );
    })();

    const handleAdd = () => {
        setEditingCmd({
            id: Date.now().toString(),
            name: '',
            content: '',
            group: selectedGroup,
        });
        setIsNewCommand(true);
    };

    const handleAddGroup = () => {
        setEditingCmd({
            id: Date.now().toString(),
            name: '',
            content: '',
            group: '__new__',
        });
        setIsNewCommand(true);
    };

    const handleEdit = (cmd: QuickCommand) => {
        setEditingCmd({ ...cmd });
        setIsNewCommand(false);
    };

    const handleSave = (cmd: QuickCommand) => {
        if (isNewCommand) {
            const group = cmd.group || selectedGroup;
            addCommand(cmd.name, cmd.content, group);
            // 新建后落到该命令所在的分组（尤其是新建分组：不切换的话
            // 用户会以为保存没生效）
            setSelectedGroup(group);
        } else {
            updateCommand(cmd.id, {
                name: cmd.name,
                content: cmd.content,
                group: cmd.group,
            });
        }
        setEditingCmd(null);
        setIsNewCommand(false);
    };

    const handleCancel = () => {
        setEditingCmd(null);
        setIsNewCommand(false);
    };

    // 高度未自定义时保持内容自适应（maxHeight 200）；自定义后为精确高度，开合动画照常
    const sizeStyle = !isOpen
        ? { maxHeight: '0px' }
        : panelHeight != null
            ? { height: `${panelHeight}px` }
            : { maxHeight: '200px' };

    return (
        <div
            ref={panelRef}
            style={{
                ...styles.container,
                ...sizeStyle,
                transition: resizing ? 'none' : styles.container.transition,
            }}
            data-testid="quick-command-panel"
        >
            {isOpen && (
                <div style={styles.body}>
                    <div
                        style={styles.heightHandle}
                        data-testid="quickcmd-height-handle"
                        title="拖动调整面板高度"
                        onPointerDown={beginResize('height')}
                        onPointerMove={handleResizeMove}
                        onPointerUp={endResize}
                        onPointerCancel={endResize}
                    />
                    <CommandGrid
                        commands={visibleCommands}
                        onExecute={onExecute}
                        onEdit={handleEdit}
                        onDelete={deleteCommand}
                        onAdd={handleAdd}
                        searchQuery={searchQuery}
                        onSearchChange={setSearchQuery}
                        onReorder={(ordered) => reorderCommands(ordered.map(c => c.id))}
                    />
                    <div
                        style={styles.widthHandle}
                        data-testid="quickcmd-width-handle"
                        title="拖动调整分组栏宽度"
                        onPointerDown={beginResize('width')}
                        onPointerMove={handleResizeMove}
                        onPointerUp={endResize}
                        onPointerCancel={endResize}
                    />
                    <GroupStrip
                        groups={availableGroups}
                        selectedGroup={selectedGroup}
                        onSelectGroup={handleSelectGroup}
                        onAddGroup={handleAddGroup}
                        width={stripWidth}
                    />
                </div>
            )}

            <CommandEditModal
                isOpen={editingCmd !== null}
                command={editingCmd}
                isNew={isNewCommand}
                availableGroups={availableGroups}
                onSave={handleSave}
                onCancel={handleCancel}
                defaultGroup={selectedGroup}
            />
        </div>
    );
};

const styles = {
    container: {
        display: 'flex',
        flexDirection: 'column' as const,
        minHeight: '0px',
        backgroundColor: 'var(--bg-primary)',
        flexShrink: 0,
        overflow: 'hidden',
        transition: 'max-height 0.3s cubic-bezier(0.4, 0, 0.2, 1), height 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
        borderTop: '1px solid var(--bg-elevated)',
    },
    body: {
        display: 'flex',
        flexDirection: 'row' as const,
        flex: '1 1 auto',
        minHeight: 0,
        overflow: 'hidden',
        position: 'relative' as const,
    },
    // 面板上缘调高手柄：一条极窄的悬停区，平时不可见
    heightHandle: {
        position: 'absolute' as const,
        top: 0,
        left: 0,
        right: 0,
        height: '5px',
        cursor: 'ns-resize',
        zIndex: 10,
    },
    // 分组条左缘调宽手柄：紧贴分隔线
    widthHandle: {
        flex: '0 0 4px',
        cursor: 'ew-resize',
    },
};

export default QuickCommandPanel;
