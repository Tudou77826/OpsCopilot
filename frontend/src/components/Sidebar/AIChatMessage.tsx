import { useState } from 'react';
import { TbCheck, TbCopy } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { RichText } from '../AI';
import type { AssistantOperation } from '../../../../frontend-shell/src/assistant/runtime';
import { AssistantOperationReceipt } from './AssistantOperationReceipt';

export function AIChatMessage({ id = '', content, user = false, phase, kind, time, operation, onReveal, onExecutionExpand, timing }: { id?: string; content: string; user?: boolean; phase?: string; kind?: string; time?: string; operation?: AssistantOperation; onReveal?: (id:string)=>Promise<void>; onExecutionExpand?: () => void; timing?: string }) {
    const [copied, setCopied] = useState(false);
    const [error, setError] = useState(false);
    async function copy() {
        setError(false);
        try { await navigator.clipboard.writeText(content); setCopied(true); }
        catch { setError(true); }
    }
    if (!user && kind === 'operation' && operation?.kind) return <AssistantOperationReceipt id={id} content={content} operation={operation} time={time} onReveal={onReveal} onExpand={onExecutionExpand} timing={timing}/>;
    if (!user && phase === 'failed' && kind === 'notice') return <AssistantOperationReceipt id={id} content="本轮请求未完成" operation={{kind:'assistant',workspaceId:'',status:'failed',stage:'助手请求',outcome:'此前已确认的操作与结果仍保留；未确认的执行结果不会自动重试。',next:'核对下方原因及最近一条操作记录，再继续处理。',detail:content}}/>;
    const date = time ? new Date(time) : undefined;
    const stamp = date && !Number.isNaN(date.getTime()) ? <time dateTime={time} title={[date.toLocaleString(),timing].filter(Boolean).join(' · ')}>{date.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'})}</time> : null;
    if (kind && kind !== 'text') return <article className={`ai-interaction-record is-${kind}`} data-testid="message-item"><div className="ai-message-meta"><span>{kind === 'answer' ? '你的回答' : kind === 'question' ? '助手提问' : kind === 'permission' ? '操作确认' : kind === 'permission_answer' ? '你的选择' : kind === 'review' ? '自审' : kind === 'review_fallback' ? '自审方式' : '操作状态'}</span>{stamp}</div><p>{content}</p></article>;
    if (user) return <div className="ai-user-message" data-testid="message-item">{stamp}<RichText content={content} /></div>;
    return <article className="ai-assistant-message" data-testid="message-item">
        <div className="ai-message-meta"><span>助手</span>{stamp}<IconButton label="复制回答" title={copied ? '已复制' : '复制回答'} onClick={() => void copy()}>{copied ? <TbCheck size={16} /> : <TbCopy size={16} />}</IconButton></div>
        {copied && <span className="ai-composer-accessible-hint" role="status">已复制</span>}
        <RichText content={content} />
        {(phase === 'stopped' || phase === 'failed') && <p className="ai-message-outcome">{phase === 'stopped' ? '本轮已停止，未自动续跑' : '本轮未完成'}</p>}
        {error && <p className="ai-chat-error" role="alert">复制失败，请选择文本复制。</p>}
    </article>;
}
