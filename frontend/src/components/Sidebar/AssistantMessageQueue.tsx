import { useState } from 'react';
import { TbArrowUp, TbChevronDown, TbPlayerPlay, TbX } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { QueuedAssistantMessage } from './useAssistantQueue';

interface Props {
    messages: QueuedAssistantMessage[];
    paused: boolean;
    disabled: boolean;
    resumeDisabled?: boolean;
    currentTitle?: string;
    onResume: () => void;
    onSendNow: (id: number) => void;
    onRemove: (id: number) => void;
}

export function AssistantMessageQueue({ messages, paused, disabled, resumeDisabled, currentTitle, onResume, onSendNow, onRemove }: Props) {
    const [expanded, setExpanded] = useState(false);
    if (!messages.length) return null;
    const visible = expanded ? messages : messages.slice(0, 2);
    return <section className="ai-chat-queue" aria-label="待发送消息">
        <div className="ai-chat-queue-heading">
            <span>{paused ? '队列已暂停' : '待发送'}<span className="ai-chat-queue-count">{messages.length}</span></span>
            {paused ? <button type="button" title="按顺序发送剩余消息，不重试已经中断的任务" disabled={disabled || resumeDisabled} onClick={onResume}><TbPlayerPlay size={14}/>继续发送</button> : <small>本轮结束后依次发送</small>}
        </div>
        <ol>{visible.map(message => <li key={message.id}>
            <div className="ai-chat-queue-content"><p title={message.question}>{message.question}</p>
                {message.title && message.title !== currentTitle && <small title={message.title}>{message.title}</small>}
            </div>
            <div className="ai-chat-queue-actions">
                <IconButton title="立即发送：先停止本轮，再处理这条消息" label="立即发送" disabled={disabled} onClick={() => onSendNow(message.id)}><TbArrowUp size={16}/></IconButton>
                <IconButton title="移除消息" label={`移除消息：${message.question}`} disabled={disabled} onClick={() => onRemove(message.id)}><TbX size={16}/></IconButton>
            </div>
        </li>)}</ol>
        {messages.length > 2 && <button className="ai-chat-queue-expand" type="button" aria-expanded={expanded} onClick={() => setExpanded(!expanded)}>
            {expanded ? '收起' : `还有 ${messages.length - 2} 条`}{TbChevronDown({ size: 13, style: { transform: expanded ? 'rotate(180deg)' : undefined } })}
        </button>}
    </section>;
}
