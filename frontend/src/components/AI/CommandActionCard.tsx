import React, { useState } from 'react';
import { CommandSuggestion, KnowledgeTarget } from './types';
import { parseKnowledgeSource } from './parseKnowledgeResponse';

interface CommandActionCardProps {
    command: CommandSuggestion;
    onTypeCommand?: (command: string) => void;
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

function getRiskPresentation(risk?: string): { label: string; tone: string } {
    const normalized = String(risk || 'Low').toLowerCase();
    if (normalized === 'high' || normalized === 'critical') return { label: '高风险', tone: 'high' };
    if (normalized === 'medium' || normalized === 'moderate') return { label: '中风险', tone: 'medium' };
    return { label: '低风险', tone: 'low' };
}

const CommandActionCard: React.FC<CommandActionCardProps> = ({ command, onTypeCommand, onOpenSource }) => {
    const [copied, setCopied] = useState(false);
    const risk = getRiskPresentation(command.risk);
    const source = command.source ? parseKnowledgeSource(command.source) : null;

    const copy = async () => {
        await navigator.clipboard.writeText(command.command);
        setCopied(true);
        window.setTimeout(() => setCopied(false), 1200);
    };

    return (
        <section className="ai-command-card">
            <div className="ai-command-header">
                <span className="ai-command-title">{command.description || '建议命令'}</span>
                <span className={`ai-risk ai-risk-${risk.tone}`}>{risk.label}</span>
            </div>
            <pre className="ai-command-code"><code>{command.command}</code></pre>
            <div className="ai-command-footer">
                {source ? (
                    <button
                        className="ai-source-inline"
                        type="button"
                        onClick={() => onOpenSource?.({ path: source.path, line: source.line })}
                        title="在知识库中打开"
                    >
                        ◫ {source.path}{source.line ? ` · L${source.line}` : ''}
                    </button>
                ) : <span className="ai-command-no-source">未标注文档来源</span>}
                <span className="ai-command-actions">
                    <button type="button" onClick={copy}>{copied ? '已复制' : '复制'}</button>
                    {onTypeCommand && <button className="ai-command-type" type="button" onClick={() => onTypeCommand(command.command)}>键入终端</button>}
                </span>
            </div>
        </section>
    );
};

export default CommandActionCard;
