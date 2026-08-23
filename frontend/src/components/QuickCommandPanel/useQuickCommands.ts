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

    add(cmd: QuickCommand): void {
        // @ts-ignore
        if (window.go?.main?.App?.AddQuickCommand) {
            // @ts-ignore
            window.go.main.App.AddQuickCommand(cmd);
        }
    }

    update(id: string, updates: Partial<QuickCommand>): void {
        // @ts-ignore
        if (window.go?.main?.App?.UpdateQuickCommand) {
            // @ts-ignore
            window.go.main.App.UpdateQuickCommand(id, { ...updates, id });
        }
    }

    remove(id: string): void {
        // @ts-ignore
        if (window.go?.main?.App?.DeleteQuickCommand) {
            // @ts-ignore
            window.go.main.App.DeleteQuickCommand(id);
        }
    }

    reorder(ids: string[]): void {
        // @ts-ignore
        if (window.go?.main?.App?.ReorderQuickCommands) {
            // @ts-ignore
            window.go.main.App.ReorderQuickCommands(ids);
        }
    }
}

export class MemoryAdapter implements QuickCommandStorageAdapter {
    data: QuickCommand[] = [];

    async load(): Promise<QuickCommand[]> {
        return this.data;
    }

    add(cmd: QuickCommand): void {
        this.data = [...this.data, cmd];
    }

    update(id: string, updates: Partial<QuickCommand>): void {
        this.data = this.data.map(c => c.id === id ? { ...c, ...updates } : c);
    }

    remove(id: string): void {
        this.data = this.data.filter(c => c.id !== id);
    }

    reorder(ids: string[]): void {
        const set = new Set(ids);
        const byId = new Map(this.data.filter(c => set.has(c.id)).map(c => [c.id, c]));
        let k = 0;
        this.data = this.data.map(c => set.has(c.id) ? (byId.get(ids[k++]) ?? c) : c);
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
    /** 按给定 id 顺序重排（拖拽排序），其余命令位置不变 */
    reorderCommands: (ids: string[]) => void;
    /** 外部（多窗口热加载事件）推送的最新列表，直接替换本地状态 */
    applyExternalCommands: (cmds: QuickCommand[]) => void;
}

export function useQuickCommands(options?: UseQuickCommandsOptions): UseQuickCommandsReturn {
    // adapter 惰性初始化：组件每次渲染都新建实例会让依赖它的 effect 反复执行
    //（旧实现因此把「切分组/输入搜索」也变成全量刷盘）
    const adapterRef = useRef<QuickCommandStorageAdapter>();
    if (!adapterRef.current) {
        adapterRef.current = options?.adapter || new WailsAdapter();
    }
    const adapter = adapterRef.current;

    const [commands, setCommands] = useState<QuickCommand[]>([]);
    const [loaded, setLoaded] = useState(false);
    const [selectedGroup, setSelectedGroup] = useState<string>('default');

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
            setCommands(fixedCmds);
            setLoaded(true);
        }).catch(() => {
            setLoaded(true);
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // 热加载：本进程写盘或其他窗口（进程）修改配置文件时，后端推送最新列表。
    // 只替换本地状态，不回写，避免与单条意图化操作形成回环。
    useEffect(() => {
        // @ts-ignore
        const runtime = window.runtime;
        if (!runtime?.EventsOn) return;
        const off = runtime.EventsOn('quick-commands-updated', (cmds: QuickCommand[]) => {
            if (!Array.isArray(cmds)) return;
            setCommands(cmds.map(cmd => ({
                ...cmd,
                group: cmd.group === '__new__' ? 'default' : (cmd.group || 'default'),
            })));
        });
        return () => {
            if (typeof off === 'function') off();
        };
    }, []);

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
        adapter.add(newCmd);
    };

    const updateCommand = (id: string, updates: Partial<QuickCommand>) => {
        // 确保 __new__ 不被保存为实际分组
        const fixedUpdates = { ...updates };
        if (fixedUpdates.group === '__new__') {
            fixedUpdates.group = 'default';
        }
        setCommands(prev => prev.map(c => c.id === id ? { ...c, ...fixedUpdates } : c));
        adapter.update(id, fixedUpdates);
    };

    const deleteCommand = (id: string) => {
        setCommands(prev => prev.filter(c => c.id !== id));
        adapter.remove(id);
    };

    const reorderCommands = (ids: string[]) => {
        if (ids.length < 2) return;
        // 乐观重排：ids 涉及的槽位按新顺序回填，后端事件到达后以权威列表为准
        setCommands(prev => {
            const set = new Set(ids);
            const byId = new Map(prev.filter(c => set.has(c.id)).map(c => [c.id, c]));
            let k = 0;
            return prev.map(c => set.has(c.id) ? (byId.get(ids[k++]) ?? c) : c);
        });
        adapter.reorder(ids);
    };

    const applyExternalCommands = (cmds: QuickCommand[]) => {
        setCommands(cmds.map(cmd => ({
            ...cmd,
            group: cmd.group === '__new__' ? 'default' : (cmd.group || 'default'),
        })));
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
        reorderCommands,
        applyExternalCommands,
    };
}
