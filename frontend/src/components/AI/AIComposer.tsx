import React, { useEffect, useId, useRef } from 'react';
import { TbArrowUp, TbClockUp, TbPaperclip, TbPlayerStop, TbTerminal2, TbLayout } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { AIContextChip } from './types';

interface AIComposerProps {
    value: string;
    onChange: (value: string) => void;
    onSend: () => void;
    onAttach?: () => void;
    placeholder?: string;
    contexts?: AIContextChip[];
    disabled?: boolean;
    busy?: boolean;
    onStop?: () => void;
    stopDisabled?: boolean;
    hint?: string;
    queueWhileBusy?: boolean;
    queuePending?: boolean;
    variant?: 'default' | 'conversation';
    header?: React.ReactNode;
    editor?: React.ReactNode;
    sendDisabled?: boolean;
    toolbarLeading?: React.ReactNode;
    toolbarTrailing?: React.ReactNode;
}

const AIComposer: React.FC<AIComposerProps> = ({
    value,
    onChange,
    onSend,
    onAttach,
    placeholder = '补充现象或继续追问…',
    contexts = [],
    disabled = false,
    busy = false,
    onStop,
    stopDisabled = false,
    hint,
    queueWhileBusy = false,
    queuePending = false,
    variant = 'default',
    header,
    editor,
    sendDisabled = false,
    toolbarLeading,
    toolbarTrailing,
}) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);
    const hintId = useId();
    const conversation = variant === 'conversation';
    const queueing = queueWhileBusy && (busy || queuePending);
    const sendLabel = queueing ? '加入队列' : '发送';
    const sendButton = <IconButton className="ai-composer-send" tone="primary" onClick={onSend}
        disabled={disabled || sendDisabled || (busy && !queueWhileBusy) || !value.trim()} label={sendLabel}>
        {queueing ? <TbClockUp size={18} /> : <TbArrowUp size={18} />}
    </IconButton>;

    useEffect(() => {
        const textarea = textareaRef.current;
        if (!textarea) return;
        textarea.style.height = '30px';
        textarea.style.height = `${Math.min(textarea.scrollHeight, 96)}px`;
    }, [value]);

    return (
        <div className={`ai-composer${conversation ? ' is-conversation' : ''}`}>
            {!conversation && contexts.length > 0 && (
                <div className="ai-context-row">
                    {contexts.map(context => <span className={`ai-context-chip ${context.active ? 'is-active' : ''}`} key={context.id}>{context.label}</span>)}
                </div>
            )}
            <div className={conversation ? 'ai-composer-surface' : undefined}>
            {header}
            <div className={`ai-composer-shell${onAttach ? ' has-attachment' : ''}${busy && queueWhileBusy ? ' has-queue' : ''}`}>
                {onAttach && <button className="ai-composer-attach" type="button" onClick={onAttach} title="添加上下文" disabled={disabled}>
                    {TbPaperclip({ size: 15 })}
                </button>}
                {editor ?? <textarea
                    ref={textareaRef}
                    rows={1}
                    value={value}
                    disabled={disabled}
                    onChange={event => onChange(event.target.value)}
                    onKeyDown={event => {
                        if (event.nativeEvent.isComposing) return;
                        if (event.key === 'Enter' && !event.shiftKey) {
                            event.preventDefault();
                            if (value.trim() && !disabled && !sendDisabled && (!busy || queueWhileBusy)) onSend();
                        }
                    }}
                    placeholder={placeholder}
                    aria-label={placeholder}
                    aria-describedby={hintId}
                />}
                {!conversation && busy && onStop ? <IconButton className="ai-composer-stop" label="停止本轮" onClick={onStop} disabled={stopDisabled}>
                    <TbPlayerStop size={17} />
                </IconButton> : null}
                {!conversation && (!busy || !onStop || queueWhileBusy) && sendButton}
            </div>
            {conversation && <div className="ai-composer-toolbar">
                {toolbarLeading}
                <div className="ai-composer-targets">{contexts.length ? contexts.map(context => <span key={context.id} title={context.label} aria-label={context.label}>
                    {context.kind === 'workspace' ? <TbLayout size={14}/> : <TbTerminal2 size={14}/>}<span>{context.label}</span>
                </span>) : !toolbarTrailing && <span className="ai-composer-shortcut" aria-hidden="true">Shift + Enter 换行</span>}</div>
                {toolbarTrailing}
                {sendButton}
            </div>}
            </div>
            <div id={hintId} className={conversation ? 'ai-composer-accessible-hint' : 'ai-composer-hint'}>{hint ?? 'Enter 发送 · Shift + Enter 换行'}</div>
        </div>
    );
};

export default AIComposer;
