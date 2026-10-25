import { useEffect, useRef, useState } from 'react';
import type { AssistantAnswer, AssistantTarget, AssistantTurnState, AssistantRuntime, AssistantMessage } from '../../../../frontend-shell/src/assistant/runtime';

interface Operation {
    live: boolean;
    turnId?: string;
    stopRequested: boolean;
    ready: Promise<void>;
    markReady: () => void;
    stopping?: Promise<void>;
}
export interface AssistantTurnResult { phase: string; output: string; messages?: AssistantMessage[]; }
const active = (state: AssistantTurnState) => state.phase === 'running' || state.phase === 'waiting';

/** Owns the exact turn returned by start. A late response never targets a new turn. */
export function useAssistantTurn(runtime?: AssistantRuntime) {
    const [state, setState] = useState<AssistantTurnState>();
    const current = useRef<Operation>();
    const mounted = useRef(true);
    useEffect(() => {
        mounted.current = true;
        const release = () => {
            const operation = current.current;
            if (operation?.live) {
                operation.live = false;
                if (operation.turnId) void Promise.resolve().then(() => runtime?.stop(operation.turnId!)).catch(() => undefined);
            }
        };
        window.addEventListener('pagehide', release);
        return () => { mounted.current = false; release(); window.removeEventListener('pagehide', release); };
    }, [runtime]);

    async function ask(question: string, target?: AssistantTarget) {
        if (!runtime) throw Error('AI 助手暂不可用');
        let markReady!: () => void;
        const ready = new Promise<void>(resolve => { markReady = resolve; });
        const operation: Operation = { live: true, stopRequested: false, ready, markReady };
        if (current.current?.live) throw Error('请先停止当前操作');
        current.current = operation;
        let observed:AssistantTurnState | undefined;
        try {
            observed = await runtime.start(question, target ? { ...target } : undefined);
            operation.turnId = observed.turnId;
            operation.markReady();
            if (!operation.live || !mounted.current) {
                await runtime.stop(observed.turnId);
                throw Error('本轮已停止');
            }
            if (operation.stopRequested) await stopOperation(operation);
            let failures = 0;
            while (operation.live && mounted.current) {
                if (!active(observed)) {
                    if (observed.phase === 'stopped') {
                        // Takeover publishes revocation before ACP cancellation
                        // drains. Do not expose an idle UI before host admission.
                        setState(observed);
                        await stopOperation(operation);
                    }
                    await operation.stopping;
                    const messages = observed.messages?.length ? [...observed.messages] : undefined;
                    if (messages && observed.phase !== 'completed') messages.push({id:`${observed.turnId}:outcome`,role:'ai',kind:'notice',content:observed.message,phase:observed.phase,time:new Date().toISOString()});
                    return { phase: observed.phase, output: observed.phase === 'failed' ? [observed.output, observed.message].filter(Boolean).join('\n\n') : observed.output || observed.message, messages };
                }
                setState(observed);
                await new Promise(resolve => setTimeout(resolve, 250));
                if (!operation.live || !mounted.current) break;
                try {
                    const next = await runtime.state();
                    if (next.turnId !== operation.turnId) throw Error('本轮已结束，请查看最新操作');
                    observed = next; failures = 0;
                } catch (error) {
                    if (++failures >= 3) throw error;
                }
            }
            return { phase: 'stopped', output: '本轮已停止' };
        } catch (error) {
            if (operation.turnId) await runtime.stop(operation.turnId).catch(() => undefined);
            if (observed?.messages?.length) return {phase:'failed',output:observed.output || '',messages:[...observed.messages,{id:`${observed.turnId}:outcome`,role:'ai',kind:'notice',phase:'failed',content:`请求未完成：${error instanceof Error ? error.message : String(error)}`,time:new Date().toISOString()}]};
            throw error;
        } finally {
            operation.markReady();
            operation.live = false;
            if (current.current === operation && mounted.current) setState(undefined);
        }
    }
    function stopOperation(operation: Operation): Promise<void> {
        operation.stopRequested = true;
        operation.stopping ??= (async () => {
            await operation.ready;
            if (operation.turnId) await runtime!.stop(operation.turnId);
        })();
        return operation.stopping;
    }
    async function stop() {
        const operation = current.current;
        if (!operation?.live || !runtime) return;
        await stopOperation(operation);
        // Keep polling the exact turn until its stopped state is confirmed.
    }
    async function respond(answer: AssistantAnswer) {
        const operation = current.current;
        if (!runtime || !operation?.live || !state?.prompt || state.turnId !== operation.turnId) throw Error('这条确认已结束');
        await runtime.respond(state.turnId, state.prompt.id, answer);
    }
    return { state, ask, stop, respond };
}
