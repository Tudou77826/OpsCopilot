import { useId, useState } from 'react';
import type { AssistantMessage } from '../../../../frontend-shell/src/assistant/runtime';
import { TbCheck, TbChevronRight, TbInfoCircle } from 'react-icons/tb';
import { AIChatMessage } from './AIChatMessage';

const isProcess = (message: AssistantMessage) => message.kind === 'permission_answer' || (message.role !== 'user' &&
    (message.kind === 'review' || message.kind === 'review_fallback' || message.kind === 'permission' ||
        (message.kind === 'operation' && !message.operation?.action && ['completed','done'].includes(message.operation?.status || ''))));

// Only known successful or active host operations can join a group. Problems,
// unrecognised states and user-authored messages always remain on the main line.
const isExecution = (message: AssistantMessage) => message.role !== 'user' && message.kind === 'operation' &&
    !!message.operation?.action && ['completed', 'done', 'running', 'pending', 'in_progress'].includes(message.operation.status);

type TimelineEntry = { id: string; messages: AssistantMessage[]; activity: boolean };

/** Presentation only: preserve event order and never fold questions, answers,
 * failures or model prose into the process log based on text heuristics. */
export function AIChatTimeline({ messages, onReveal }: { messages: AssistantMessage[]; onReveal?: (id:string)=>Promise<void> }) {
    // A live event briefly leaves state before it is committed to history.
    // Keep disclosure state above the rows so that handoff does not close it.
    const [expanded, setExpanded] = useState<ReadonlySet<string>>(() => new Set());
    function toggle(id: string) {
        setExpanded(previous => {
            const next = new Set(previous);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            // Match the live timeline's bound without retaining old rows forever.
            if (next.size > 512) next.delete(next.values().next().value!);
            return next;
        });
    }
    function expand(id: string) {
        setExpanded(previous => previous.has(id) ? previous : new Set([...previous, id].slice(-512)));
    }
    const entries: TimelineEntry[] = [];
    const timings = new Map(messages.filter(message=>message.kind==='timing').map(message=>[message.turnId,message.content]));
    for (const message of messages) {
        if(message.kind==='timing') continue;
        const activity = isProcess(message) || isExecution(message);
        const previous = entries[entries.length - 1];
        if (activity && previous?.activity && previous.messages[0].turnId === message.turnId) {
            previous.messages.push(message);
        } else {
            entries.push({ id: message.id, messages: [message], activity });
        }
    }
    const timedTurns = new Set<string|undefined>();
    return <>{entries.map(entry => {
        const turn=entry.messages[0].turnId;
        const timing=timedTurns.has(turn)?undefined:timings.get(turn);
        if(entry.activity) timedTurns.add(turn);
        return entry.activity
        ? <ActivityBlock key={entry.id} messages={entry.messages} expanded={expanded} onToggle={toggle} onExpand={expand} onReveal={onReveal} timing={timing}/>
        : <AIChatMessage key={entry.id} {...entry.messages[0]} user={entry.messages[0].role === 'user'} onReveal={onReveal} timing={timing}/>;
    })}</>;
}

function ActivityBlock({ messages, expanded, onToggle, onExpand, onReveal, timing }: {
    messages: AssistantMessage[]; expanded: ReadonlySet<string>; onToggle: (id: string) => void;
    onExpand: (id: string) => void; onReveal?: (id: string) => Promise<void>;
    timing?:string;
}) {
    const detailsId = useId();
    const groupId = `execution-${messages[0].id}`;
    const executions = messages.filter(isExecution);
    const grouped = executions.length > 1;
    const active = [...executions].reverse().find(message => !['completed', 'done'].includes(message.operation!.status));
    const completed = executions.filter(message => ['completed', 'done'].includes(message.operation!.status)).length;
    const latest = active || executions[executions.length - 1];
    const label = active ? `正在执行 · 已完成 ${completed} 项` : `已完成 ${completed} 项操作`;
    const rows: { id: string; messages: AssistantMessage[]; process: boolean }[] = [];
    for (const message of messages) {
        const process = isProcess(message);
        const previous = rows[rows.length - 1];
        if (process && previous?.process) previous.messages.push(message);
        else rows.push({ id: message.id, messages: [message], process });
    }
    // Keep children mounted, even before a second execution forms the group.
    // Expanding a child opts this block open so arriving updates cannot hide it.
    return <div className={`ai-activity-block${grouped ? ' is-grouped' : ''}`}>
        {grouped && <button type="button" className="ai-execution-group-toggle" aria-expanded={expanded.has(groupId)} aria-controls={detailsId}
            title={[label, latest?.content,timing].filter(Boolean).join(' · ')} onClick={() => onToggle(groupId)}>
            <TbChevronRight size={13} className="ai-execution-chevron" aria-hidden="true" />
            {active ? <i className="ai-execution-group-running" aria-hidden="true" /> : <TbCheck size={14} aria-hidden="true" />}
            <span>{label}</span>
            <small>{latest?.content}</small>
            {timing && <span className="ai-execution-duration">{timing.split(' · ')[0]}</span>}
        </button>}
        <div id={detailsId} className="ai-activity-items" hidden={grouped && !expanded.has(groupId)}>
            {grouped && timing && <p className="ai-execution-timing" title="助手响应包含启动、网络和生成等等待，不是模型服务端推理计时。工具执行不含等待你确认的时间。">{timing}</p>}
            {rows.map(row => row.process
                ? <ProcessLog key={row.id} messages={row.messages} expanded={expanded.has(`process-${row.id}`)} onToggle={() => {
                    if (!expanded.has(`process-${row.id}`)) onExpand(groupId);
                    onToggle(`process-${row.id}`);
                }} onReveal={onReveal} />
                : <AIChatMessage key={row.id} {...row.messages[0]} onReveal={onReveal} onExecutionExpand={() => onExpand(groupId)} timing={!grouped?timing:undefined}/>)}
        </div>
        {grouped && !expanded.has(groupId) && messages.some(message => message.kind === 'review_fallback') &&
            <ReviewFallback content={messages.find(message => message.kind === 'review_fallback')?.content} />}
    </div>;
}

function ReviewFallback({ content }: { content?: string }) {
    return <p className="ai-process-fallback" role="status" title={content}>
        <TbInfoCircle size={14} aria-hidden="true" />
        <span>自审较慢 · 建议配置可用的快速模型</span>
    </p>;
}

function ProcessLog({ messages, expanded, onToggle, onReveal }: { messages: AssistantMessage[]; expanded: boolean; onToggle: () => void; onReveal?: (id:string)=>Promise<void> }) {
    const fallback = messages.find(message => message.kind === 'review_fallback');
    return <div className="ai-process-log">
        <details open={expanded}>
            <summary onClick={event => { event.preventDefault(); onToggle(); }}>
                <TbChevronRight className="ai-process-chevron" size={14} aria-hidden="true" />
                <span>过程记录</span><small>{messages.length} 项</small>
            </summary>
            <ol>{messages.map(message => <li key={message.id}>
                <AIChatMessage {...message} onReveal={onReveal} />
            </li>)}</ol>
        </details>
        {fallback && <ReviewFallback content={fallback.content} />}
    </div>;
}
