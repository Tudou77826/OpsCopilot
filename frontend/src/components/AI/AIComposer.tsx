import React, { useEffect, useRef } from 'react';
import { TbArrowUp, TbPaperclip } from 'react-icons/tb';
import { AIContextChip } from './types';

interface AIComposerProps {
    value: string;
    onChange: (value: string) => void;
    onSend: () => void;
    onAttach?: () => void;
    placeholder?: string;
    contexts?: AIContextChip[];
    disabled?: boolean;
}

const AIComposer: React.FC<AIComposerProps> = ({
    value,
    onChange,
    onSend,
    onAttach,
    placeholder = '补充现象或继续追问…',
    contexts = [],
    disabled = false,
}) => {
    const textareaRef = useRef<HTMLTextAreaElement>(null);

    useEffect(() => {
        const textarea = textareaRef.current;
        if (!textarea) return;
        textarea.style.height = '30px';
        textarea.style.height = `${Math.min(textarea.scrollHeight, 96)}px`;
    }, [value]);

    return (
        <div className="ai-composer">
            {contexts.length > 0 && (
                <div className="ai-context-row">
                    {contexts.map(context => <span className={`ai-context-chip ${context.active ? 'is-active' : ''}`} key={context.id}>{context.label}</span>)}
                </div>
            )}
            <div className="ai-composer-shell">
                <button className="ai-composer-attach" type="button" onClick={onAttach} title="添加上下文" disabled={!onAttach}>
                    {TbPaperclip({ size: 15 })}
                </button>
                <textarea
                    ref={textareaRef}
                    rows={1}
                    value={value}
                    disabled={disabled}
                    onChange={event => onChange(event.target.value)}
                    onKeyDown={event => {
                        if (event.key === 'Enter' && !event.shiftKey) {
                            event.preventDefault();
                            if (value.trim() && !disabled) onSend();
                        }
                    }}
                    placeholder={placeholder}
                    aria-label={placeholder}
                />
                <button className="ai-composer-send" type="button" onClick={onSend} disabled={disabled || !value.trim()} title="发送">
                    {TbArrowUp({ size: 16 })}
                </button>
            </div>
            <div className="ai-composer-hint">Enter 发送 · Shift + Enter 换行</div>
        </div>
    );
};

export default AIComposer;
