import React from 'react';
import ReactMarkdown from 'react-markdown';
import remarkGfm from 'remark-gfm';
import rehypeHighlight from 'rehype-highlight';
import 'highlight.js/styles/atom-one-dark.css';

interface MessageRendererProps {
    content: string;
    role: 'user' | 'ai';
}

const MessageRenderer: React.FC<MessageRendererProps> = ({ content, role }) => {
    return (
        <div className="message-markdown-content">
            <ReactMarkdown
                remarkPlugins={[remarkGfm]}
                rehypePlugins={[rehypeHighlight]}
                components={{
                    // 自定义代码块渲染
                    code({ node, inline, className, children, ...props }: any) {
                        const match = /language-(\w+)/.exec(className || '');
                        const language = match ? match[1] : '';
                        
                        return !inline ? (
                            <div className="code-block-wrapper">
                                {language && (
                                    <div className="code-block-header">
                                        <span className="code-language">{language}</span>
                                    </div>
                                )}
                                <pre className={className}>
                                    <code className={className} {...props}>
                                        {children}
                                    </code>
                                </pre>
                            </div>
                        ) : (
                            <code className="inline-code" {...props}>
                                {children}
                            </code>
                        );
                    },
                    // 自定义链接渲染
                    a({ node, children, href, ...props }: any) {
                        return (
                            <a
                                href={href}
                                target="_blank"
                                rel="noopener noreferrer"
                                {...props}
                            >
                                {children}
                            </a>
                        );
                    },
                    // 自定义表格渲染
                    table({ node, children, ...props }: any) {
                        return (
                            <div className="table-wrapper">
                                <table {...props}>{children}</table>
                            </div>
                        );
                    }
                }}
            >
                {content}
            </ReactMarkdown>
            <style>{`
                .message-markdown-content {
                    font-size: 13px;
                    line-height: 1.6;
                    color: #fff;
                }

                .message-markdown-content h1,
                .message-markdown-content h2,
                .message-markdown-content h3,
                .message-markdown-content h4,
                .message-markdown-content h5,
                .message-markdown-content h6 {
                    margin: 0.8em 0 0.5em;
                    font-weight: 600;
                    line-height: 1.3;
                    color: #fff;
                }

                .message-markdown-content h1 { font-size: 1.6em; }
                .message-markdown-content h2 { font-size: 1.4em; }
                .message-markdown-content h3 { font-size: 1.2em; }
                .message-markdown-content h4 { font-size: 1.1em; }
                .message-markdown-content h5 { font-size: 1em; }
                .message-markdown-content h6 { font-size: 0.95em; }

                .message-markdown-content h1:first-child,
                .message-markdown-content h2:first-child,
                .message-markdown-content h3:first-child,
                .message-markdown-content h4:first-child,
                .message-markdown-content h5:first-child,
                .message-markdown-content h6:first-child {
                    margin-top: 0;
                }

                .message-markdown-content p {
                    margin: 0.5em 0;
                }

                .message-markdown-content p:first-child {
                    margin-top: 0;
                }

                .message-markdown-content p:last-child {
                    margin-bottom: 0;
                }

                .message-markdown-content strong {
                    font-weight: 600;
                    color: #fff;
                }

                .message-markdown-content em {
                    font-style: italic;
                }

                .message-markdown-content ul,
                .message-markdown-content ol {
                    margin: 0.5em 0;
                    padding-left: 1.5em;
                }

                .message-markdown-content li {
                    margin: 0.25em 0;
                }

                .message-markdown-content ul ul,
                .message-markdown-content ol ol,
                .message-markdown-content ul ol,
                .message-markdown-content ol ul {
                    margin: 0.25em 0;
                }

                .message-markdown-content blockquote {
                    margin: 0.5em 0;
                    padding: 0.5em 1em;
                    border-left: 3px solid #555;
                    background: rgba(255, 255, 255, 0.05);
                    color: #ccc;
                }

                .message-markdown-content hr {
                    border: none;
                    border-top: 1px solid #444;
                    margin: 1em 0;
                }

                .message-markdown-content a {
                    color: #4fc3f7;
                    text-decoration: none;
                    transition: color 0.2s;
                }

                .message-markdown-content a:hover {
                    color: #81d4fa;
                    text-decoration: underline;
                }

                .message-markdown-content .inline-code {
                    background: rgba(255, 255, 255, 0.1);
                    padding: 2px 6px;
                    border-radius: 3px;
                    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                    font-size: 0.9em;
                    color: #f8f8f2;
                }

                .message-markdown-content .code-block-wrapper {
                    margin: 0.5em 0;
                    border-radius: 6px;
                    overflow: hidden;
                    background: #282c34;
                }

                .message-markdown-content .code-block-header {
                    background: #21252b;
                    padding: 6px 12px;
                    border-bottom: 1px solid #181a1f;
                    display: flex;
                    justify-content: space-between;
                    align-items: center;
                }

                .message-markdown-content .code-language {
                    font-size: 11px;
                    color: #abb2bf;
                    font-family: 'Consolas', 'Monaco', monospace;
                    text-transform: uppercase;
                    font-weight: 500;
                }

                .message-markdown-content pre {
                    margin: 0;
                    padding: 12px;
                    overflow-x: auto;
                    background: #282c34;
                    font-family: 'Consolas', 'Monaco', 'Courier New', monospace;
                    font-size: 12px;
                    line-height: 1.5;
                }

                .message-markdown-content pre code {
                    background: transparent;
                    padding: 0;
                    border-radius: 0;
                    font-family: inherit;
                    font-size: inherit;
                    color: #abb2bf;
                }

                .message-markdown-content .table-wrapper {
                    overflow-x: auto;
                    margin: 0.5em 0;
                }

                .message-markdown-content table {
                    border-collapse: collapse;
                    width: 100%;
                    font-size: 0.95em;
                }

                .message-markdown-content th,
                .message-markdown-content td {
                    border: 1px solid #444;
                    padding: 6px 12px;
                    text-align: left;
                }

                .message-markdown-content th {
                    background: rgba(255, 255, 255, 0.05);
                    font-weight: 600;
                    color: #fff;
                }

                .message-markdown-content tr:nth-child(even) {
                    background: rgba(255, 255, 255, 0.02);
                }

                .message-markdown-content img {
                    max-width: 100%;
                    height: auto;
                    border-radius: 4px;
                    margin: 0.5em 0;
                }

                .message-markdown-content input[type="checkbox"] {
                    margin-right: 0.5em;
                }

                .message-markdown-content del {
                    color: #999;
                }

                /* 自定义滚动条 */
                .message-markdown-content pre::-webkit-scrollbar {
                    height: 8px;
                }

                .message-markdown-content pre::-webkit-scrollbar-track {
                    background: #21252b;
                }

                .message-markdown-content pre::-webkit-scrollbar-thumb {
                    background: #3e4451;
                    border-radius: 4px;
                }

                .message-markdown-content pre::-webkit-scrollbar-thumb:hover {
                    background: #4e5561;
                }
            `}</style>
        </div>
    );
};

export default React.memo(MessageRenderer);
