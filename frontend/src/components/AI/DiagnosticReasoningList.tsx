import React from 'react';
import { DiagnosticStep } from './types';

interface DiagnosticReasoningListProps {
    steps: DiagnosticStep[];
}

const DiagnosticReasoningList: React.FC<DiagnosticReasoningListProps> = ({ steps }) => (
    <ol className="ai-reasoning-list">
        {steps.map((step, index) => (
            <li className="ai-reasoning-item" key={`${step.step ?? index}-${step.title ?? step.description}`}>
                <span className="ai-reason-index">{String(step.step ?? index + 1).padStart(2, '0')}</span>
                <span className="ai-reason-body">
                    {step.title && <span className="ai-reason-title">{step.title}</span>}
                    <span className="ai-reason-description">{step.description}</span>
                </span>
            </li>
        ))}
    </ol>
);

export default DiagnosticReasoningList;
