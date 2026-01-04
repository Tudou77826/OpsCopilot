import React from 'react';

interface TroubleshootingStepProps {
    step: string;
    index: number;
}

const TroubleshootingStep: React.FC<TroubleshootingStepProps> = ({ step, index }) => {
    return (
        <div style={styles.stepContainer}>
            <div style={styles.stepIndex}>{index + 1}</div>
            <div style={styles.stepContent}>{step}</div>
        </div>
    );
};

const styles = {
    stepContainer: {
        display: 'flex',
        alignItems: 'flex-start',
        marginBottom: '8px',
        padding: '8px',
        backgroundColor: '#2d2d2d',
        borderRadius: '6px',
    },
    stepIndex: {
        width: '24px',
        height: '24px',
        borderRadius: '12px',
        backgroundColor: '#007acc',
        color: 'white',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        marginRight: '12px',
        flexShrink: 0,
        fontSize: '12px',
        fontWeight: 'bold' as const,
    },
    stepContent: {
        flex: 1,
        color: '#e0e0e0',
        fontSize: '13px',
        lineHeight: '1.5',
    }
};

export default TroubleshootingStep;
