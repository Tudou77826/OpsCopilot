import React, { useEffect, useState } from 'react';

const BottomBar: React.FC = () => {
    const [version, setVersion] = useState('');

    useEffect(() => {
        (async () => {
            try {
                // @ts-ignore
                const v = await window.go?.main?.App?.GetVersion?.();
                if (v) setVersion(v);
            } catch { /* ignore */ }
        })();
    }, []);

    return (
        <div style={styles.container} data-testid="bottom-bar">
            <div style={{ flex: 1 }} />
            {version && (
                <span style={styles.version}>
                    {version.startsWith('v') ? version : `v${version}`}
                </span>
            )}
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
    version: {
        color: '#666',
        fontSize: '11px',
        fontFamily: 'monospace',
    },
};

export default BottomBar;
