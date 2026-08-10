import React from 'react';
import { KnowledgeReference, KnowledgeTarget } from './types';

interface CitationListProps {
    references: KnowledgeReference[];
    onOpenSource?: (target: Omit<KnowledgeTarget, 'requestId'>) => void;
}

const CitationList: React.FC<CitationListProps> = ({ references, onOpenSource }) => {
    if (references.length === 0) return null;
    return (
        <div className="ai-source-list">
            {references.map(reference => (
                <button
                    className="ai-source-row"
                    type="button"
                    key={`${reference.path}:${reference.line ?? ''}`}
                    onClick={() => onOpenSource?.({ path: reference.path, line: reference.line })}
                >
                    <span className="ai-source-index">{reference.id}</span>
                    <span className="ai-source-copy">
                        <span className="ai-source-label">{reference.label}</span>
                        <span className="ai-source-path">
                            {reference.path}{reference.line ? ` · L${reference.line}${reference.lineEnd ? `–L${reference.lineEnd}` : ''}` : ''}
                        </span>
                    </span>
                    {onOpenSource && <span className="ai-source-arrow">↗</span>}
                </button>
            ))}
        </div>
    );
};

export default CitationList;
