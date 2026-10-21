import React, { useEffect, useRef, useState, type ComponentProps } from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/atom-one-dark.css';
import './ai-components.css';

interface RichTextProps {
    content: string;
    className?: string;
}

const MarkdownCodeBlock = ({ children, ...props }: ComponentProps<'pre'>) => {
    const [copied, setCopied] = useState(false);
    const [copyError, setCopyError] = useState(false);
    const pre = useRef<HTMLPreElement>(null);
    const code = React.Children.toArray(children).find(child => React.isValidElement<{ className?: string }>(child));
    const className = React.isValidElement<{ className?: string }>(code) ? code.props.className : '';
    const language = /language-([\w-]+)/.exec(className || '')?.[1] || 'text';
    useEffect(() => {
        if (!copied) return;
        const timer = window.setTimeout(() => setCopied(false), 1200);
        return () => window.clearTimeout(timer);
    }, [copied]);

    const copy = async () => {
        setCopyError(false);
        try {
            // Syntax highlighting turns children into elements. Copy rendered
            // text, never String(children), which produces [object Object].
            await navigator.clipboard.writeText((pre.current?.textContent || '').replace(/\n$/, ''));
            setCopied(true);
        } catch { setCopyError(true); }
    };

    return (
        <div className="ai-code-block">
            <div className="ai-code-head">
                <span>{language}</span>
                <button type="button" onClick={() => void copy()}>{copied ? '已复制' : '复制'}</button>
            </div>
            <pre ref={pre} {...props}>{children}</pre>
            {copyError && <p role="alert">复制失败，请选择代码复制。</p>}
        </div>
    );
};

const RichText: React.FC<RichTextProps> = ({ content, className = '' }) => (
    <div className={`ai-rich-text ${className}`.trim()}>
        <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            rehypePlugins={[rehypeHighlight]}
            components={{
                pre: ({ node, ...props }) => <MarkdownCodeBlock {...props} />,
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
