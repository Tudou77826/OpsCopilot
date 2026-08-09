import React, { useState } from 'react';
import { QuickCommand } from './types';
import { useQuickCommands } from './useQuickCommands';
import GroupStrip from './GroupStrip';
import CommandGrid from './CommandGrid';
import CommandEditModal from './CommandEditModal';

interface QuickCommandPanelProps {
    isOpen: boolean;
    onExecute: (content: string) => void;
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
    } = useQuickCommands();

    const [editingCmd, setEditingCmd] = useState<QuickCommand | null>(null);
    const [isNewCommand, setIsNewCommand] = useState(false);
    // 搜索关键字：在当前分组内进一步过滤（按 name/content 匹配，大小写不敏感）（issue #56）
    const [searchQuery, setSearchQuery] = useState('');

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
            addCommand(cmd.name, cmd.content, cmd.group || selectedGroup);
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

    return (
        <div style={{
            ...styles.container,
            maxHeight: isOpen ? '200px' : '0px',
        }} data-testid="quick-command-panel">
            {isOpen && (
                <div style={styles.body}>
                    <CommandGrid
                        commands={visibleCommands}
                        onExecute={onExecute}
                        onEdit={handleEdit}
                        onDelete={deleteCommand}
                        onAdd={handleAdd}
                        searchQuery={searchQuery}
                        onSearchChange={setSearchQuery}
                    />
                    <GroupStrip
                        groups={availableGroups}
                        selectedGroup={selectedGroup}
                        onSelectGroup={handleSelectGroup}
                        onAddGroup={handleAddGroup}
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
        transition: 'max-height 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
        borderTop: '1px solid var(--bg-elevated)',
    },
    body: {
        display: 'flex',
        flexDirection: 'row' as const,
        flexShrink: 0,
        overflow: 'hidden',
    },
};

export default QuickCommandPanel;