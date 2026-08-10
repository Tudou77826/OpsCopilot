import React, { useMemo } from 'react';
import CitationList from './CitationList';
import CommandActionCard from './CommandActionCard';
import DiagnosticReasoningList from './DiagnosticReasoningList';
import RichText from './RichText';
import { KnowledgeReference, KnowledgeResponse, KnowledgeTarget } from './types';

interface KnowledgeAnswerProps {
    response: KnowledgeResponse;
    references?: KnowledgeReference[];
    onTypeCommand?: (command: string) => void;
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

const SectionLabel: React.FC<{ children: React.ReactNode }> = ({ children }) => (
    <h4 className="ai-section-label"><span>{children}</span></h4>
);

const KnowledgeAnswer: React.FC<KnowledgeAnswerProps> = ({ response, references = [], onTypeCommand, onOpenSource }) => {
    const mergedReferences = useMemo(() => {
        const seen = new Set<string>();
        return [...response.references, ...references].reduce<KnowledgeReference[]>((items, reference) => {
            const key = `${reference.path}:${reference.line ?? ''}:${reference.lineEnd ?? ''}`;
            if (seen.has(key)) return items;
            seen.add(key);
            items.push({ ...reference, id: items.length + 1 });
            return items;
        }, []);
    }, [response.references, references]);

    return (
        <article className="ai-knowledge-answer">
            {response.summary && <section><SectionLabel>综合分析</SectionLabel><RichText content={response.summary} /></section>}
            {response.steps.length > 0 && <section><SectionLabel>定位思路</SectionLabel><DiagnosticReasoningList steps={response.steps} /></section>}
            {response.commands.length > 0 && (
                <section>
                    <SectionLabel>建议命令</SectionLabel>
                    <div className="ai-command-stack">
                        {response.commands.map((command, index) => (
                            <CommandActionCard
                                command={command}
                                key={`${command.command}-${index}`}
                                onTypeCommand={onTypeCommand}
                                onOpenSource={onOpenSource}
                            />
                        ))}
                    </div>
                </section>
            )}
            {mergedReferences.length > 0 && <section><SectionLabel>参考文档</SectionLabel><CitationList references={mergedReferences} onOpenSource={onOpenSource} /></section>}
        </article>
    );
};

export default KnowledgeAnswer;
