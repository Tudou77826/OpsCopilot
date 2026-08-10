import React, { useState } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/atom-one-dark.css';
import './ai-components.css';

interface RichTextProps {
    content: string;
    className?: string;
}

const MarkdownCode: React.FC<any> = ({ inline, className, children, ...props }) => {
    const [copied, setCopied] = useState(false);
    const language = /language-(\w+)/.exec(className || '')?.[1] || '';
    const text = String(children ?? '').replace(/\n$/, '');

    if (inline) {
        return <code className="ai-inline-code" {...props}>{children}</code>;
    }

    const copy = async () => {
        await navigator.clipboard.writeText(text);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1200);
    };

    return (
        <div className="ai-code-block">
            <div className="ai-code-head">
                <span>{language || 'text'}</span>
                <button type="button" onClick={copy}>{copied ? '已复制' : '复制'}</button>
            </div>
            <pre><code className={className} {...props}>{children}</code></pre>
        </div>
    );
};

const RichText: React.FC<RichTextProps> = ({ content, className = '' }) => (
    <div className={`ai-rich-text ${className}`.trim()}>
        <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeHighlight]}
            components={{
                code: MarkdownCode,
                a: ({ node, children, href, ...props }: any) => (
                    <a href={href} target="_blank" rel="noopener noreferrer" {...props}>{children}</a>
                ),
                table: ({ node, children, ...props }: any) => (
                    <div className="ai-table-wrap"><table {...props}>{children}</table></div>
                ),
            }}
        >
            {content}
        </ReactMarkdown>
    </div>
);

export default React.memo(RichText);
