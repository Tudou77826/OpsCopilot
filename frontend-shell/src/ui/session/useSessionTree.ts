import { useCallback, useEffect, useRef, useState } from 'react';
import type { SessionManagerRuntime, SessionNode } from '../ports';

const EXPANDED_STORAGE_KEY = 'opscopilot.sessionTree.expanded';

function loadExpanded(): Set<string> {
    try {
        const raw = window.localStorage.getItem(EXPANDED_STORAGE_KEY);
        if (!raw) return new Set();
        const parsed = JSON.parse(raw);
        return Array.isArray(parsed) ? new Set(parsed as string[]) : new Set();
    } catch {
        return new Set();
    }
}

function saveExpanded(expanded: Set<string>): void {
    try {
        window.localStorage.setItem(EXPANDED_STORAGE_KEY, JSON.stringify([...expanded]));
    } catch {
        // 存储不可用时忽略：展开态丢失不影响功能。
    }
}

/**
 * 会话树的数据与展开态。
 *
 * 刷新策略刻意不用定时轮询：此前的 5 秒轮询每次都会用服务端顺序整体替换本地数据，
 * 把用户刚拖出来的顺序和折叠状态一起抹掉。改为"每次变更后刷新 + 窗口重新聚焦时刷新"，
 * 既不再有 5 秒一次的无谓重排，也能在别处改动后保持一致。
 */
export function useSessionTree(runtime: SessionManagerRuntime) {
    const [nodes, setNodes] = useState<SessionNode[]>([]);
    const [expanded, setExpanded] = useState<Set<string>>(() => loadExpanded());
    const [loadError, setLoadError] = useState('');

    const runtimeRef = useRef(runtime);
    runtimeRef.current = runtime;

    const refresh = useCallback(async () => {
        try {
            setNodes(await runtimeRef.current.listTree());
            setLoadError('');
        } catch (e: any) {
            setLoadError(e?.toString?.() ?? '加载会话列表失败');
        }
    }, []);

    useEffect(() => {
        void refresh();
        const onFocus = () => void refresh();
        window.addEventListener('focus', onFocus);
        return () => window.removeEventListener('focus', onFocus);
    }, [refresh]);

    const toggle = useCallback((id: string) => {
        setExpanded((prev) => {
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            saveExpanded(next);
            return next;
        });
    }, []);

    const expand = useCallback((id: string) => {
        setExpanded((prev) => {
            if (prev.has(id)) return prev;
            const next = new Set(prev).add(id);
            saveExpanded(next);
            return next;
        });
    }, []);

    const expandMany = useCallback((ids: string[]) => {
        if (ids.length === 0) return;
        setExpanded((prev) => {
            let changed = false;
            const next = new Set(prev);
            for (const id of ids) {
                if (!next.has(id)) {
                    next.add(id);
                    changed = true;
                }
            }
            if (!changed) return prev;
            saveExpanded(next);
            return next;
        });
    }, []);

    return { nodes, expanded, toggle, expand, expandMany, refresh, loadError };
}
