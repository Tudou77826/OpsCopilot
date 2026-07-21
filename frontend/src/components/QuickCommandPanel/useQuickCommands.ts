import { useState, useEffect, useMemo, useRef } from 'react';
import { QuickCommand, QuickCommandStorageAdapter } from './types';

class WailsAdapter implements QuickCommandStorageAdapter {
    async load(): Promise<QuickCommand[]> {
        // @ts-ignore
        if (window.go?.main?.App?.LoadQuickCommands) {
            // @ts-ignore
            const cmds = await window.go.main.App.LoadQuickCommands();
            return cmds || [];
        }
        return [];
    }

    save(commands: QuickCommand[]): void {
        // @ts-ignore
        if (window.go?.main?.App?.SaveQuickCommands) {
            // @ts-ignore
            window.go.main.App.SaveQuickCommands(commands);
        }
    }
}

export class MemoryAdapter implements QuickCommandStorageAdapter {
    data: QuickCommand[] = [];

    async load(): Promise<QuickCommand[]> {
        return this.data;
    }

    save(commands: QuickCommand[]): void {
        this.data = [...commands];
    }
}

export interface UseQuickCommandsOptions {
    adapter?: QuickCommandStorageAdapter;
}

export interface UseQuickCommandsReturn {
    commands: QuickCommand[];
    loaded: boolean;
    availableGroups: string[];
    selectedGroup: string;
    setSelectedGroup: (group: string) => void;
    filteredCommands: QuickCommand[];
    addCommand: (name: string, content: string, group: string) => void;
    updateCommand: (id: string, updates: Partial<QuickCommand>) => void;
    deleteCommand: (id: string) => void;
}

export function useQuickCommands(options?: UseQuickCommandsOptions): UseQuickCommandsReturn {
    const adapter = options?.adapter || new WailsAdapter();

    const [commands, setCommands] = useState<QuickCommand[]>([]);
    const [loaded, setLoaded] = useState(false);
    const [selectedGroup, setSelectedGroup] = useState<string>('default');
    // 标记本次 commands 变化是否由用户操作触发；加载完成后首次赋值应跳过保存
    const skipNextSaveRef = useRef(false);

    const availableGroups = useMemo(() => {
        const groupSet = new Set<string>();
        commands.forEach(cmd => {
            const g = cmd.group || 'default';
            // 过滤掉特殊标记值
            if (g !== '__new__') {
                groupSet.add(g);
            }
        });
        if (groupSet.size === 0) groupSet.add('default');
        return Array.from(groupSet).sort();
    }, [commands]);

    useEffect(() => {
        if (availableGroups.length > 0 && !availableGroups.includes(selectedGroup)) {
            setSelectedGroup(availableGroups[0]);
        }
    }, [availableGroups, selectedGroup]);

    useEffect(() => {
        adapter.load().then(cmds => {
            // 加载时修复无效分组值
            const fixedCmds = (cmds || []).map(cmd => ({
                ...cmd,
                group: cmd.group === '__new__' ? 'default' : (cmd.group || 'default'),
            }));
            // 加载完成后的首次 setCommands 不是用户操作，跳过保存
            skipNextSaveRef.current = true;
            setCommands(fixedCmds);
            setLoaded(true);
        }).catch(() => {
            setLoaded(true);
        });
    }, []);

    useEffect(() => {
        if (!loaded) return;
        // 跳过加载完成后的首次回写，避免无谓刷盘
        if (skipNextSaveRef.current) {
            skipNextSaveRef.current = false;
            return;
        }
        adapter.save(commands);
    }, [commands, loaded, adapter]);

    const filteredCommands = useMemo(() => {
        return commands.filter(cmd => {
            const g = cmd.group || 'default';
            // 带有 __new__ 标记的命令归类到 default 分组显示
            const effectiveGroup = g === '__new__' ? 'default' : g;
            return effectiveGroup === selectedGroup;
        });
    }, [commands, selectedGroup]);

    const addCommand = (name: string, content: string, group: string) => {
        // 确保 __new__ 不被保存为实际分组
        const effectiveGroup = group === '__new__' ? 'default' : group;
        const newCmd: QuickCommand = {
            id: Date.now().toString(),
            name,
            content,
            group: effectiveGroup,
        };
        setCommands(prev => [...prev, newCmd]);
    };

    const updateCommand = (id: string, updates: Partial<QuickCommand>) => {
        // 确保 __new__ 不被保存为实际分组
        const fixedUpdates = { ...updates };
        if (fixedUpdates.group === '__new__') {
            fixedUpdates.group = 'default';
        }
        setCommands(prev => prev.map(c => c.id === id ? { ...c, ...fixedUpdates } : c));
    };

    const deleteCommand = (id: string) => {
        setCommands(prev => prev.filter(c => c.id !== id));
    };

    return {
        commands,
        loaded,
        availableGroups,
        selectedGroup,
        setSelectedGroup,
        filteredCommands,
        addCommand,
        updateCommand,
        deleteCommand,
    };
}
