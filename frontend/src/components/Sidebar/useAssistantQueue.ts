import { useEffect, useRef, useState } from 'react';
import type { AssistantMessage, AssistantRuntime, AssistantTarget } from '../../../../frontend-shell/src/assistant/runtime';
import { useAssistantTurn } from './useAssistantTurn';

export interface QueuedAssistantMessage {
    id: number;
    question: string;
    target?: AssistantTarget;
    title?: string;
}

/** Window-local FIFO. The host remains the authority for single-turn admission.
 * Queued messages contain resource candidates, never permissions or approvals. */
export function useAssistantQueue(runtime: AssistantRuntime) {
    const turn = useAssistantTurn(runtime);
    const [pending, setPending] = useState<QueuedAssistantMessage[]>([]);
    const [messages, setMessages] = useState<AssistantMessage[]>([]);
    const [restoring, setRestoring] = useState(!!runtime.restoreConversation);
    const [restoreError, setRestoreError] = useState('');
    const [truncated, setTruncated] = useState(false);
    const [recoveryNote, setRecoveryNote] = useState('');
    const restoreBlocked = useRef(!!runtime.restoreConversation);
    const restoreGeneration = useRef(0);
    const [current, setCurrent] = useState<QueuedAssistantMessage>();
    const [paused, setPaused] = useState(false);
    const [transitioning, setTransitioning] = useState(false);
    const queue = useRef<QueuedAssistantMessage[]>([]);
    const worker = useRef<Promise<void>>();
    const blocked = useRef(false);
    const transition = useRef(false);
    const mounted = useRef(true);
    const sequence = useRef(0);

    useEffect(() => {
        mounted.current = true;
        void restore();
        return () => { mounted.current = false; restoreGeneration.current++; };
    }, [runtime]);

    async function restore() {
        if (!runtime.restoreConversation) return;
        const generation = ++restoreGeneration.current;
        restoreBlocked.current = true;
        setRestoring(true); setRestoreError('');
        try {
            const snapshot = await runtime.restoreConversation();
            if (!mounted.current || generation !== restoreGeneration.current) return;
            setMessages(snapshot.messages);
            setTruncated(snapshot.truncated);
            setRecoveryNote(snapshot.recoveryNote || '');
            restoreBlocked.current = false;
        } catch (error) {
            if (mounted.current && generation === restoreGeneration.current)
                setRestoreError(`恢复对话失败：${error instanceof Error ? error.message : String(error)}`);
        } finally {
            if (mounted.current && generation === restoreGeneration.current) setRestoring(false);
        }
    }

    function publishQueue() { if (mounted.current) setPending([...queue.current]); }
    function pause(value: boolean) {
        blocked.current = value;
        if (mounted.current) setPaused(value);
    }
    function pump() {
        if (restoreBlocked.current || worker.current || blocked.current || transition.current || !mounted.current || !queue.current.length) return;
        worker.current = (async () => {
            while (mounted.current && !blocked.current && !transition.current && queue.current.length) {
                const message = queue.current.shift()!;
                publishQueue();
                setCurrent(message);
                const submitted: AssistantMessage = { id: `local-${++sequence.current}`, role: 'user', content: message.question, time:new Date().toISOString() };
                setMessages(previous => [...previous, submitted]);
                try {
                    const result = await turn.ask(message.question, message.target);
                    if (!mounted.current) return;
                    const answer: AssistantMessage = { id: `local-${++sequence.current}`, role: 'ai', content: result.output, phase: result.phase, time:new Date().toISOString() };
                    setMessages(previous => [...previous, ...(result.messages?.length ? result.messages : [answer])]);
                    // Manual takeover and timeout must not dispatch the next message.
                    if (result.phase !== 'completed') pause(true);
                } catch (error) {
                    if (!mounted.current) return;
                    const failure: AssistantMessage = { id: `local-${++sequence.current}`, role: 'ai', kind: 'notice', content: `请求未完成：${error instanceof Error ? error.message : String(error)}`, phase: 'failed' };
                    setMessages(previous => [...previous, failure]);
                    pause(true);
                }
            }
        })().finally(() => {
            worker.current = undefined;
            if (mounted.current) setCurrent(undefined);
        });
    }
    function enqueue(question: string, target?: AssistantTarget, title?: string) {
        if (restoreBlocked.current) throw Error('请先完成对话恢复，或新建对话');
        const text = question.trim();
        if (!text) return;
        if (new TextEncoder().encode(text).length > 16384) throw Error('请输入不超过 16 KB 的问题');
        if (queue.current.length >= 20) throw Error('最多排队 20 条消息，请先处理或移除已有消息');
        // A fresh explicit message after an empty stopped queue is a new intent.
        if (!worker.current && !queue.current.length && !transition.current) pause(false);
        queue.current.push({ id: ++sequence.current, question: text, target: target ? { ...target } : undefined, title: target ? title : undefined });
        publishQueue();
        pump();
    }
    async function stop() {
        if (transition.current) return;
        transition.current = true;
        setTransitioning(true);
        pause(true);
        try {
            // Includes late start and host cleanup, not just the visible stopped state.
            await turn.stop();
            await worker.current;
        } finally {
            transition.current = false;
            if (mounted.current) setTransitioning(false);
        }
    }
    async function sendNow(id: number) {
        if (transition.current || !queue.current.some(message => message.id === id)) return;
        await stop();
        if (!mounted.current) return;
        const index = queue.current.findIndex(message => message.id === id);
        if (index < 0) return;
        const [message] = queue.current.splice(index, 1);
        queue.current.unshift(message);
        pause(false);
        publishQueue();
        pump();
    }
    function remove(id: number) {
        if (transition.current) return;
        queue.current = queue.current.filter(message => message.id !== id);
        publishQueue();
    }
    function resume() {
        if (worker.current || transition.current) return;
        pause(false);
        pump();
    }
    async function newConversation() {
        if (worker.current || transition.current || restoring) throw Error('请等待当前操作结束');
        restoreGeneration.current++;
        transition.current = true;
        setTransitioning(true);
        try {
            await runtime.newConversation();
            if (!mounted.current) return;
            queue.current = [];
            publishQueue();
            setMessages([]);
            setTruncated(false); setRestoreError(''); restoreBlocked.current = false;
            setRecoveryNote('');
            pause(false);
        } finally {
            transition.current = false;
            if (mounted.current) setTransitioning(false);
        }
    }
    return { state: turn.state, respond: turn.respond, current, pending, messages, paused, transitioning, restoring, restoreError, truncated, recoveryNote, restore, enqueue, sendNow, remove, resume, stop, newConversation };
}
