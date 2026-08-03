import React from 'react';

interface StepData {
    step: number;
    title: string;
    description: string;
}

interface TroubleshootingStepProps {
    step: string | StepData;
    index: number;
}

const TroubleshootingStep: React.FC<TroubleshootingStepProps> = ({ step, index }) => {
    // Handle both string (legacy/simple) and object (structured) formats
    const isObject = typeof step === 'object' && step !== null;
    const content = isObject ? (step as StepData).description : (step as string);
    const title = isObject ? (step as StepData).title : `Step ${index + 1}`;

    return (
        <div style={styles.stepContainer}>
            <div style={styles.stepIndex}>{index + 1}</div>
            <div style={styles.stepContentWrapper}>
                {isObject && <div style={styles.stepTitle}>{title}</div>}
                <div style={styles.stepContent}>{content}</div>
            </div>
        </div>
    );
};

const styles = {
    stepContainer: {
        display: 'flex',
        alignItems: 'flex-start',
        marginBottom: '12px',
        padding: '12px',
        backgroundColor: 'var(--bg-tertiary)',
        borderRadius: '6px',
        border: '1px solid var(--border)',
    },
    stepIndex: {
        width: '24px',
        height: '24px',
        borderRadius: '12px',
        backgroundColor: 'var(--accent)',
        color: 'var(--text-on-accent)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        marginRight: '12px',
        flexShrink: 0,
        fontSize: '12px',
        fontWeight: 'bold' as const,
    },
    stepContentWrapper: {
        flex: 1,
        minWidth: 0,
    },
    stepTitle: {
        fontWeight: 'bold' as const,
        color: 'var(--text-primary)',
        marginBottom: '4px',
        fontSize: '14px',
    },
    stepContent: {
        color: 'var(--text-primary)',
        fontSize: '13px',
        lineHeight: '1.5',
    },
    commandBox: {
        marginTop: '8px',
        backgroundColor: 'var(--bg-primary)',
        padding: '6px 10px',
        borderRadius: '4px',
        border: '1px solid var(--border)',
        fontFamily: 'var(--font-mono)',
        fontSize: '12px',
        color: 'var(--stage-orange)',
        whiteSpace: 'pre-wrap' as const,
        wordBreak: 'break-all' as const,
    }
};

export default TroubleshootingStep;
