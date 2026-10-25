import { useEffect, useRef, useState, type ReactNode } from 'react';
import './terminalHandoff.css';
import { AssistantInteractionCard } from '../AssistantInteraction/AssistantInteractionCard';
import type { InteractionAnswer } from '../AssistantInteraction/types';

import type { HandoffState } from './useTerminalHandoff';
export { useTerminalHandoff, type HandoffState, type HandoffHost } from './useTerminalHandoff';

interface Props {
    state?: HandoffState;
    transportError: string;
    connected: boolean;
    children: ReactNode;
    onStart(scenario: string, confirmed: boolean): Promise<void>;
    onStop(state: HandoffState): Promise<void>;
    onRespond?(state: HandoffState, answer: InteractionAnswer): Promise<void>;
    onFocus(): void;
    experimental?: boolean;
}

export function TerminalHandoff({ state, transportError, connected, children, onStart, onStop, onRespond, onFocus, experimental = false }: Props) {
    const [expanded, setExpanded] = useState(false);
    const [confirmed, setConfirmed] = useState(false);
    const [scenario, setScenario] = useState('inspect');
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const content = useRef<HTMLDivElement>(null);
    const workActive = state?.work?.phase === 'running' || state?.work?.phase === 'waiting';
    const assistantWork = state?.work?.source === 'assistant';
    const blocked = !!state?.owned || busy;
    useEffect(() => {
        // inert also prevents a previously focused xterm textarea from accepting
        // keyboard input. The backend gate remains authoritative for every writer.
        content.current?.toggleAttribute('inert', blocked);
    }, [blocked]);

    useEffect(() => { setExpanded(false); setConfirmed(false); setError(''); }, [connected, state?.sessionId, state?.connectionId]);

    function toggleSetup() {
        setExpanded(value => !value);
        setConfirmed(false);
        setError('');
    }

    async function start() {
        setBusy(true); setError('');
        try { await onStart(scenario, confirmed); setExpanded(false); setConfirmed(false); }
        catch (reason) { setError(String(reason)); }
        finally { setBusy(false); }
    }
    async function stop() {
        if (!state) return;
        setError('');
        try { await onStop(state); requestAnimationFrame(onFocus); }
        catch (reason) { setError(String(reason)); }
    }
    return <section className="oc-handoff" aria-label="终端">
        {connected && state && (experimental || state.owned) && <div className="oc-handoff-bar">
            <span className={`oc-handoff-dot${state.owned ? ' is-active' : ''}`} aria-hidden="true" />
            <span className="oc-handoff-summary" role="status">{assistantWork ? (state.owned ? state.message : '当前终端') : state.work?.message || (state.phase === 'idle' ? '当前终端' : state.message)}{state.exitCode !== undefined && state.phase === 'completed' ? ` · 退出码 ${state.exitCode}` : ''}</span>
            {state.owned || (workActive && !assistantWork) ? <button type="button" onClick={() => void stop()}>停止</button> : experimental && <button type="button" disabled={workActive} aria-expanded={expanded} onClick={toggleSetup}>接手测试 <span className="oc-handoff-lab">实验</span></button>}
        </div>}
        {connected && expanded && !state?.owned && !workActive && <div className="oc-handoff-setup" aria-label="开始接手测试">
            <div className="oc-handoff-setup-heading">沿用当前命令行</div>
            <p>先确认终端已空闲。测试会临时启用状态标记，并在同一个 Shell 中执行固定命令。</p>
            <div className="oc-handoff-scenarios" role="radiogroup" aria-label="测试内容">
                <label><input type="radio" name={`scenario-${state?.sessionId}`} checked={scenario === 'inspect'} onChange={() => setScenario('inspect')} /><span>查看当前现场<small>查看账号、目录与 Shell 进程</small></span></label>
                <label><input type="radio" name={`scenario-${state?.sessionId}`} checked={scenario === 'slow'} onChange={() => setScenario('slow')} /><span>体验停止与接回<small>等待 12 秒，期间可随时接管</small></span></label>
                <label><input type="radio" name={`scenario-${state?.sessionId}`} checked={scenario === 'approval'} onChange={() => setScenario('approval')} /><span>体验操作确认<small>只读自动继续，另一条打印命令由你明确批准</small></span></label>
            </div>
            <label className="oc-handoff-confirm"><input type="checkbox" checked={confirmed} onChange={event => setConfirmed(event.target.checked)} />当前为空闲命令行，没有正在运行的程序或未提交的输入</label>
            <div className="oc-handoff-setup-actions"><button type="button" onClick={toggleSetup} disabled={busy}>取消</button><button className="oc-handoff-primary" type="button" disabled={!confirmed || busy || !!transportError} onClick={() => void start()}>{busy ? '正在接手…' : '开始测试'}</button></div>
        </div>}
        {(error || transportError) && <div className="oc-handoff-error" role="alert">{error || transportError}</div>}
        {state?.work?.prompt && state.work.source !== 'assistant' && onRespond && <AssistantInteractionCard key={state.work.prompt.id} prompt={state.work.prompt} onRespond={answer => onRespond(state, answer)} onStop={() => void stop()} />}
        <div className="oc-handoff-viewport">
            <div ref={content} className="oc-handoff-content" aria-disabled={blocked}>{children}</div>
            {blocked && <div className="oc-handoff-shield" aria-label="接手期间终端由测试助手独占">
                <div className="oc-handoff-control"><span className="oc-handoff-orbit" aria-hidden="true" /><div><strong>{state?.work?.source === 'assistant' ? 'AI 正在使用此终端' : '接手测试进行中'}</strong><span>你可以查看过程，或随时接回终端</span></div><button type="button" onClick={() => void stop()} disabled={!state?.owned && !workActive}>手动接管</button></div>
            </div>}
        </div>
    </section>;
}
