import { useEffect, useRef, useState } from 'react';
import type { AssistantWork, InteractionAnswer } from '../AssistantInteraction/types';

export interface HandoffState {
    sessionId: string;
    connectionId?: string;
    runId: string;
    phase: string;
    owned: boolean;
    ready: boolean;
    message: string;
    exitCode?: number;
    work?: AssistantWork;
}

export interface HandoffHost {
    states(): Promise<HandoffState[]>;
    start(id: string, scenario: string, confirmed: boolean, connectionId: string): Promise<HandoffState>;
    stop(id: string, runId: string): Promise<void>;
    stopWork?(turnId: string): Promise<void>;
    respond?(turnId: string, requestId: string, answer: InteractionAnswer): Promise<void>;
}

interface Binding {
    host: HandoffHost;
    active: boolean;
    values: Record<string, HandoffState>;
    operations: Map<string, number>;
}

const workActive = (state: HandoffState) => state.work?.phase === 'running' || state.work?.phase === 'waiting';
async function releaseState(host: HandoffHost, state: HandoffState) {
    if (state.work && host.stopWork && workActive(state)) await host.stopWork(state.work.turnId);
    else await host.stop(state.sessionId, state.runId);
}

export function useTerminalHandoff(host: HandoffHost) {
    const [states, setStates] = useState<Record<string, HandoffState>>({});
    const [error, setError] = useState('');
    const epoch = useRef(0);
    const binding = useRef<Binding>();

    const publish = (scope: Binding, next: Record<string, HandoffState>) => {
        scope.values = next;
        setStates(previous => JSON.stringify(previous) === JSON.stringify(next) ? previous : next);
    };

    useEffect(() => {
        const scope: Binding = { host, active: true, values: {}, operations: new Map() };
        binding.current = scope;
        epoch.current++;
        setStates({});
        setError('');
        let disposed = false;
        let timer: ReturnType<typeof setTimeout>;
        async function refresh() {
            const observedEpoch = epoch.current;
            try {
                if (!scope.active) return;
                const values = await host.states();
                if (disposed || !scope.active || observedEpoch !== epoch.current) return;
                publish(scope, Object.fromEntries(values.sort((a, b) => a.sessionId.localeCompare(b.sessionId)).map(value => [value.sessionId, value])));
                setError('');
            } catch {
                // Retain the last known lock until the backend can confirm it.
                if (!disposed && scope.active && observedEpoch === epoch.current) setError('暂时无法确认接手状态，请等待连接恢复');
            } finally {
                if (!disposed) timer = setTimeout(refresh, 350);
            }
        }
        const release = () => {
            scope.active = false;
            scope.operations.clear();
            epoch.current++;
            for (const state of Object.values(scope.values)) {
                if (state.owned || workActive(state)) void releaseState(host, state).catch(() => undefined);
            }
        };
        const resume = () => { scope.active = true; epoch.current++; };
        window.addEventListener('pagehide', release);
        window.addEventListener('pageshow', resume);
        void refresh();
        return () => {
            disposed = true;
            clearTimeout(timer);
            window.removeEventListener('pagehide', release);
            window.removeEventListener('pageshow', resume);
            release();
        };
    }, [host]);

    function begin(id: string) {
        const scope = binding.current;
        if (!scope?.active || scope.host !== host) throw Error('页面已离开，请重新打开终端后操作');
        const operation = ++epoch.current;
        scope.operations.set(id, operation);
        return { scope, operation };
    }
    const isCurrent = (scope: Binding, id: string, operation: number) => binding.current === scope && scope.active && scope.operations.get(id) === operation;

    return {
        states, error,
        start: async (id: string, scenario: string, confirmed: boolean) => {
            const { scope, operation } = begin(id);
            const connectionId = scope.values[id]?.connectionId ?? '';
            const value = await host.start(id, scenario, confirmed, connectionId);
            if (!isCurrent(scope, id, operation)) {
                // The page may leave while Start is in flight. Revoke precisely
                // that returned run, never a newer owner on the same tab.
                await releaseState(host, value);
                return;
            }
            epoch.current++;
            const currentConnection = scope.values[id]?.connectionId;
            if (currentConnection && currentConnection !== value.connectionId) {
                await releaseState(host, value);
                return;
            }
            // A poll may already know this run has completed. Start's original
            // preparing snapshot is older than that observed state.
            if (value.work || scope.values[id]?.runId !== value.runId) publish(scope, { ...scope.values, [id]: value });
        },
        respond: async (state: HandoffState, answer: InteractionAnswer) => {
            const work = state.work;
            if (!work?.prompt || !host.respond) throw Error('当前确认已结束');
            const { scope, operation } = begin(state.sessionId);
            await host.respond(work.turnId, work.prompt.id, answer);
            if (!isCurrent(scope, state.sessionId, operation)) return;
            epoch.current++;
            const values = await host.states();
            if (isCurrent(scope, state.sessionId, operation)) publish(scope, Object.fromEntries(values.map(value => [value.sessionId, value])));
        },
        stop: async (state: HandoffState) => {
            const { scope, operation } = begin(state.sessionId);
            await releaseState(host, state);
            if (!isCurrent(scope, state.sessionId, operation)) return;
            epoch.current++;
            const current = scope.values[state.sessionId];
            if (current?.runId !== state.runId || (!current.owned && !workActive(current))) return;
            publish(scope, { ...scope.values, [state.sessionId]: {
                ...current, owned: false, phase: 'stopped',
                message: current.ready ? '已交还终端' : '已交还终端；远端命令可能仍在运行，可按 Ctrl+C 中断',
                work: current.work ? { ...current.work, phase: 'stopped', prompt: undefined, message: '已停止，旧确认已撤销' } : undefined,
            } });
        },
    };
}
