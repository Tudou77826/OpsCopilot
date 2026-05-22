import React from 'react';

const BottomBar: React.FC = () => {
    return (
        <div style={styles.container} data-testid="bottom-bar">
        </div>
    );
};

const styles = {
    container: {
        height: '24px',
        backgroundColor: '#2b2b2b',
        borderTop: '1px solid #1e1e1e',
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        padding: '0 12px',
    },
};

export default BottomBar;
