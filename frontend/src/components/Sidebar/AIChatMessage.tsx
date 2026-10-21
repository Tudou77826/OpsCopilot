import { useState } from 'react';
import { TbCheck, TbCopy } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { RichText } from '../AI';

export function AIChatMessage({ content, user = false, phase }: { content: string; user?: boolean; phase?: string }) {
    const [copied, setCopied] = useState(false);
    const [error, setError] = useState(false);
    async function copy() {
        setError(false);
        try { await navigator.clipboard.writeText(content); setCopied(true); }
        catch { setError(true); }
    }
    if (user) return <div className="ai-user-message" data-testid="message-item"><RichText content={content} /></div>;
    return <article className="ai-assistant-message" data-testid="message-item">
        <div className="ai-message-meta"><span>助手</span><IconButton label="复制回答" title={copied ? '已复制' : '复制回答'} onClick={() => void copy()}>{copied ? <TbCheck size={16} /> : <TbCopy size={16} />}</IconButton></div>
        {copied && <span className="ai-composer-accessible-hint" role="status">已复制</span>}
        <RichText content={content} />
        {(phase === 'stopped' || phase === 'failed') && <p className="ai-message-outcome">{phase === 'stopped' ? '本轮已停止，未自动续跑' : '本轮未完成'}</p>}
        {error && <p className="ai-chat-error" role="alert">复制失败，请选择文本复制。</p>}
    </article>;
}
