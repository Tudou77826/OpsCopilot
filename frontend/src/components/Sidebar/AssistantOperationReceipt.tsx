import { useId, useState } from 'react';
import { TbArrowUpRight, TbCheck, TbCopy, TbDatabase, TbTransfer, TbTerminal2, TbAlertCircle, TbChevronRight, TbBook2, TbCommand, TbLayout, TbCircleMinus } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { AssistantOperation } from '../../../../frontend-shell/src/assistant/runtime';

/** Compact, factual host receipt; model markdown cannot create an action. */
export function AssistantOperationReceipt({ id, content, operation, time, onReveal, onExpand, timing }: {
    id: string; content: string; operation: AssistantOperation; time?: string; onReveal?: (id: string) => Promise<void>; onExpand?: () => void; timing?:string;
}) {
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [copied, setCopied] = useState(false);
    const execution = !!operation.action;
    const [expanded, setExpanded] = useState(false);
    const detailsId = useId();
    const preview = operation.result?.split('\n').find(line => line.trim())?.replace(/^结果 \d+：/, '') || '';
    // Earlier saved conversations recorded an explicit refusal as a generic
    // file failure. Match only that exact host-generated error, not free text.
    const denied = operation.status === 'denied' || (operation.status === 'failed' && operation.detail === '用户未批准本次操作');
    const status = denied ? '已拒绝' : ({completed:'完成',done:'完成',failed:'失败',warning:'需核对',unknown:'结果未确认',not_executed:'未执行',running:'进行中',cancelling:'停止中',cancelled:'已取消'} as Record<string,string>)[operation.status];
    const problem = !denied && ['failed', 'cancelled', 'unknown', 'warning'].includes(operation.status);
    const label = operation.kind === 'database' ? operation.itemId ? '查看 SQL' : '查看数据库页签' : operation.kind === 'file' ? '查看传输' : '查看终端';
    const Icon = operation.kind === 'database' ? TbDatabase : operation.kind === 'file' ? TbTransfer : operation.kind === 'knowledge' ? TbBook2 : operation.kind === 'reference' ? TbCommand : operation.kind === 'application' ? TbLayout : TbTerminal2;
    const elapsed = operation.measured && Number.isSafeInteger(operation.durationMs ?? 0) && (operation.durationMs ?? 0)>=0
        ? (operation.durationMs || 0)<1 ? '<1ms' : operation.durationMs!<1000 ? `${operation.durationMs}ms` : `${(operation.durationMs!/1000).toFixed(2)}s` : undefined;
    async function reveal() {
        if (!onReveal || busy) return;
        setBusy(true); setError('');
        try { await onReveal(id); }
        catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
        finally { setBusy(false); }
    }
    async function copy() {
        try { await navigator.clipboard.writeText([content, operation.action, operation.result, operation.stage, operation.outcome, operation.next, operation.detail].filter(Boolean).join('\n')); setCopied(true); }
        catch { setError('复制失败，请选择诊断文字复制。'); }
    }
    const diagnostic = operation.detail && <details className="ai-operation-diagnostic"><summary>诊断详情{!execution && operation.stage && <small> · {operation.stage}</small>}</summary>
        <pre>{operation.detail}</pre>
        <IconButton label="复制诊断" title={copied ? '已复制' : '复制诊断'} onClick={() => void copy()}>{copied ? <TbCheck size={15} /> : <TbCopy size={15} />}</IconButton>
    </details>;
    const navigation = operation.workspaceId && onReveal && <IconButton className="ai-execution-reveal" label={label} title={busy ? '正在定位…' : label} disabled={busy} onClick={() => void reveal()}><TbArrowUpRight size={16} /></IconButton>;
    if (denied && !execution) {
        const action = operation.kind === 'file' && (operation.stage === '传输准备' || content === '文件传输') ? '文件传输' : content.replace(/未完成$/, '').trim();
        return <article className="ai-operation-denied" data-testid="message-item">
            <TbCircleMinus size={14} aria-hidden="true" />
            <span>{action} · 已拒绝</span>
            {time && <time dateTime={time} title={new Date(time).toLocaleString()}>{new Date(time).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'})}</time>}
        </article>;
    }
    if (execution) return <article className={`ai-operation-receipt is-execution${problem ? ' has-problem' : ''}`} data-testid="message-item">
        <div className="ai-execution-row">
            <button className="ai-execution-toggle" type="button" aria-expanded={expanded} aria-controls={detailsId}
                aria-label={`${content}，${status || ''}，${expanded ? '收起' : '展开'}执行详情`}
                title={[content, operation.location, preview, time && new Date(time).toLocaleString(),timing].filter(Boolean).join(' · ')}
                onClick={() => { if (!expanded) onExpand?.(); setExpanded(value => !value); }}>
                <TbChevronRight size={13} className="ai-execution-chevron" aria-hidden="true" />
                <Icon size={14} aria-hidden="true" />
                <span className="ai-execution-title">{content}</span>
                {preview && <span className="ai-execution-preview">{preview}</span>}
                {elapsed && <small className="ai-execution-duration" title="本次调用用时（含调用中的确认等待）">{elapsed}</small>}
                {operation.status === 'completed' || operation.status === 'done'
                    ? <TbCheck size={14} className="ai-execution-complete" aria-hidden="true" />
                    : <small className="ai-execution-status">{status}</small>}
            </button>
            {navigation}
        </div>
        {problem && operation.next && <p className="ai-operation-next">{operation.next}</p>}
        <div id={detailsId} hidden={!expanded} className="ai-execution-details">
            {timing && <p className="ai-execution-timing" title="助手响应包含启动、网络和生成等等待，不是模型服务端推理计时。工具执行不含等待你确认的时间。">{timing}</p>}
            <div className="ai-execution-location">{[operation.location, time && new Date(time).toLocaleString()].filter(Boolean).join(' · ')}</div>
            <pre className="ai-execution-action">{operation.action}</pre>
            {operation.result && <div className="ai-execution-result">
                {operation.kind === 'terminal' && <small>终端片段</small>}
                <pre>{operation.result}</pre>
            </div>}
            {problem && operation.outcome && <p>{operation.outcome}</p>}
            {diagnostic}
            <IconButton label="复制执行记录" title={copied ? '已复制' : '复制执行记录'} onClick={() => void copy()}>{copied ? <TbCheck size={15} /> : <TbCopy size={15} />}</IconButton>
        </div>
        {error && <p className="ai-chat-error" role="alert">{error}</p>}
    </article>;
    return <article className={`ai-operation-receipt${problem ? ' has-problem' : ''}`} data-testid="message-item">
        <div className="ai-operation-line">
            {problem ? <TbAlertCircle size={15} aria-hidden="true" /> : <Icon size={15} aria-hidden="true" />}
            <span>{content}</span>
            {time && <time dateTime={time} title={new Date(time).toLocaleString()}>{new Date(time).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit',second:'2-digit'})}</time>}
            {navigation}
        </div>
        {(problem || operation.status === 'not_executed' || operation.status === 'running' || operation.status === 'cancelling') && operation.outcome && <p>{operation.outcome}</p>}
        {operation.next && problem && <p className="ai-operation-next">{operation.next}</p>}
        {diagnostic}
        {error && <p className="ai-chat-error" role="alert">{error}</p>}
    </article>;
}
