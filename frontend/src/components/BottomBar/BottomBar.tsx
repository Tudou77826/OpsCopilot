import React, { useEffect, useState } from 'react';
import { TbBulb } from 'react-icons/tb';

export const BOTTOM_BAR_TIPS = [
    'Ctrl + 滚轮：单独调整终端字号',
    'Ctrl + 0：恢复默认终端字号',
    'Ctrl + F：搜索当前终端输出',
    'Ctrl + K：用自然语言生成 Linux 命令',
    '广播模式可将输入同步发送到多个终端',
    '右键终端标签可快速开启广播（仅含当前标签）',
    '选中时间戳可以快速查看对应时间',
    '网络设备用 Telnet 连接，创建后标签带橙色标识区分',
    'Telnet 连接不支持文件传输，需要传文件请用 SSH',
];

export const BOTTOM_BAR_TIP_INTERVAL_MS = 5000;

const BottomBar: React.FC = () => {
    const [version, setVersion] = useState('');
    const [tipIndex, setTipIndex] = useState(0);
    const [tipVisible, setTipVisible] = useState(true);
    const [paused, setPaused] = useState(false);
    const [reducedMotion, setReducedMotion] = useState(false);

    useEffect(() => {
        (async () => {
            try {
                // @ts-ignore
                const v = await window.go?.main?.App?.GetVersion?.();
                if (v) setVersion(v);
            } catch { /* ignore */ }
        })();
    }, []);

    useEffect(() => {
        const media = window.matchMedia?.('(prefers-reduced-motion: reduce)');
        if (!media) return;
        const update = () => setReducedMotion(media.matches);
        update();
        media.addEventListener?.('change', update);
        return () => media.removeEventListener?.('change', update);
    }, []);

    useEffect(() => {
        if (paused || document.hidden) return;
        let fadeTimer: number | undefined;
        const interval = window.setInterval(() => {
            if (reducedMotion) {
                setTipIndex(index => (index + 1) % BOTTOM_BAR_TIPS.length);
                return;
            }
            setTipVisible(false);
            fadeTimer = window.setTimeout(() => {
                setTipIndex(index => (index + 1) % BOTTOM_BAR_TIPS.length);
                setTipVisible(true);
            }, 180);
        }, BOTTOM_BAR_TIP_INTERVAL_MS);
        return () => {
            window.clearInterval(interval);
            if (fadeTimer !== undefined) window.clearTimeout(fadeTimer);
        };
    }, [paused, reducedMotion]);

    return (
        <div style={styles.container} data-testid="bottom-bar">
            <div
                style={styles.tipArea}
                onMouseEnter={() => setPaused(true)}
                onMouseLeave={() => setPaused(false)}
                onFocusCapture={() => setPaused(true)}
                onBlurCapture={() => setPaused(false)}
                title={BOTTOM_BAR_TIPS[tipIndex]}
            >
                <span style={styles.tipIcon} aria-hidden="true">{TbBulb({ size: 13 })}</span>
                <span
                    data-testid="bottom-bar-tip"
                    style={{
                        ...styles.tipText,
                        opacity: tipVisible ? 1 : 0,
                        transition: reducedMotion ? 'none' : 'opacity 180ms ease',
                    }}
                >
                    {BOTTOM_BAR_TIPS[tipIndex]}
                </span>
            </div>

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
        height: '26px',
        backgroundColor: '#252526',
        borderTop: '1px solid #343434',
        flexShrink: 0,
        display: 'flex',
        alignItems: 'center',
        padding: '0 10px',
        gap: '10px',
        color: '#a9a9a9',
        position: 'relative' as const,
        zIndex: 20,
    },
    tipArea: {
        minWidth: 0,
        flex: 1,
        height: '100%',
        display: 'flex',
        alignItems: 'center',
        gap: '6px',
        overflow: 'hidden',
    },
    tipIcon: {
        color: '#d7ba7d',
        display: 'flex',
        alignItems: 'center',
        flexShrink: 0,
    },
    tipText: {
        color: '#bcbcbc',
        fontSize: '11px',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden',
        textOverflow: 'ellipsis',
    },
    version: {
        color: '#777777',
        fontSize: '11px',
        fontFamily: 'var(--font-mono)',
        flexShrink: 0,
    },
};

export default BottomBar;
