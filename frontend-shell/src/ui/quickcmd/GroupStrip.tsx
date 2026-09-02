import React, { useRef, useCallback, useState } from 'react';

interface GroupStripProps {
    groups: string[];
    selectedGroup: string;
    onSelectGroup: (group: string) => void;
    onAddGroup: () => void;
    /** 分组条宽度（px），可由面板拖拽调整；卡片宽度随之自适应 */
    width?: number;
}

// 3D 滚筒参数：所有分组以绝对角度 i*STEP 固定在圆筒表面（rotateX(i*STEP) translateZ(R)），
// 切换 = 只旋转滚筒本体。小角度 + 轻透视：静止时与旧版"蓝卡+灰字"几乎一致，
// 转动瞬间才有摆动纵深（近大远小），停稳后回归低干扰状态。
// 交互刻意只保留滚轮和点击，不做拖拽。
const STEP = 32;          // 相邻分组夹角（度）
const RADIUS = 52;        // 圆筒半径（px）
const PERSPECTIVE = 500;  // 透视距离（px）
// 滚轮累计触发一档的阈值。鼠标一格 deltaY≈100/120，一滚即切；
// 触控板单事件约 10~40，2~4 个事件触发，跟手但不至于太灵。
const WHEEL_THRESHOLD = 50;
// 滚轮手势间隔：停顿超过该时长视为新手势，累计清零，
// 避免很久以前轻轻滚过的残留累计被后续滚动误触发。
const WHEEL_IDLE_RESET_MS = 250;
const SNAP_TRANSITION = 'transform 0.4s cubic-bezier(0.25, 0.9, 0.3, 1)';

const GroupStrip: React.FC<GroupStripProps> = ({ groups, selectedGroup, onSelectGroup, onAddGroup, width = 64 }) => {
    const idx = Math.max(0, groups.indexOf(selectedGroup));
    const [hoveredGroup, setHoveredGroup] = useState<string | null>(null);
    // 卡片宽度随条宽自适应：前卡经透视放大约 1.12 倍，系数保证不溢出条外
    const cardW = Math.max(48, Math.min(110, Math.round(width * 0.87)));

    const wheelAccum = useRef(0);
    const lastWheelAt = useRef(0);
    const handleWheel = useCallback((e: React.WheelEvent) => {
        const now = Date.now();
        // 新手势（停顿超时）或方向反转时清零累计：
        // 反方向滚动不必先抵消旧累计，边界处也不留残值。
        if (now - lastWheelAt.current > WHEEL_IDLE_RESET_MS
            || (wheelAccum.current > 0) !== (e.deltaY > 0)) {
            wheelAccum.current = 0;
        }
        lastWheelAt.current = now;
        wheelAccum.current += e.deltaY;

        if (wheelAccum.current > WHEEL_THRESHOLD && idx < groups.length - 1) {
            onSelectGroup(groups[idx + 1]);
            wheelAccum.current = 0;
        } else if (wheelAccum.current < -WHEEL_THRESHOLD && idx > 0) {
            onSelectGroup(groups[idx - 1]);
            wheelAccum.current = 0;
        } else if (Math.abs(wheelAccum.current) > WHEEL_THRESHOLD) {
            // 已在边界还继续同向滚：清零，避免之后反向要先抵消一大段
            wheelAccum.current = 0;
        }
    }, [idx, groups, onSelectGroup]);

    return (
        <div
            style={{ ...styles.wrap, width: `${width}px`, flex: `0 0 ${width}px` }}
            data-testid="group-strip"
            onWheel={handleWheel}
        >
            <div style={styles.drumView}>
                <div
                    style={{
                        ...styles.drum,
                        transform: `rotateX(${-idx * STEP}deg)`,
                        transition: SNAP_TRANSITION,
                    }}
                >
                    {groups.map((g, i) => {
                        const d = Math.abs(i - idx);
                        const isFront = i === idx;
                        return (
                            <div
                                key={g}
                                data-testid={`group-item-${g}`}
                                // 前卡不给出 title：滚轮时光标常停在前卡上，
                                // 悬停描述会不停弹出形成干扰；邻卡保留（名称可能被截断）
                                title={isFront ? undefined : g}
                                onClick={() => { if (i !== idx) onSelectGroup(groups[i]); }}
                                onMouseEnter={() => setHoveredGroup(g)}
                                onMouseLeave={() => setHoveredGroup(null)}
                                style={{
                                    ...styles.item,
                                    width: `${cardW}px`,
                                    marginLeft: `${-Math.round(cardW / 2)}px`,
                                    transform: `rotateX(${i * STEP}deg) translateZ(${RADIUS}px)`,
                                    zIndex: 10 - Math.min(d, 9),
                                    opacity: d === 0 ? 1 : d === 1 ? 0.85 : d === 2 ? 0.4 : 0,
                                    pointerEvents: d > 1 ? 'none' : 'auto',
                                    ...(isFront ? {
                                        ...styles.itemFront,
                                        boxShadow: hoveredGroup === g
                                            ? '0 0 12px rgba(0,122,204,0.3)'
                                            : '0 2px 6px rgba(0,122,204,0.2)',
                                        cursor: 'default',
                                    } : {
                                        color: hoveredGroup === g ? 'var(--text-muted)' : 'var(--text-disabled)',
                                    }),
                                }}
                            >
                                {g}
                            </div>
                        );
                    })}
                </div>
                <div style={styles.fadeTop} />
                <div style={styles.fadeBottom} />
            </div>

            <div style={styles.addBtn} onClick={onAddGroup} data-testid="group-add-btn" title="新建分组">
                <svg width="12" height="12" viewBox="0 0 12 12" fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round">
                    <line x1="6" y1="2" x2="6" y2="10" />
                    <line x1="2" y1="6" x2="10" y2="6" />
                </svg>
            </div>
        </div>
    );
};

const styles = {
    wrap: {
        width: '64px',
        display: 'flex',
        flexDirection: 'column' as const,
        alignItems: 'center',
        padding: '10px 4px',
        backgroundColor: 'var(--bg-primary)',
        borderLeft: '1px solid var(--border-subtle)',
        userSelect: 'none' as const,
        flex: '0 0 64px',
    },
    drumView: {
        position: 'relative' as const,
        flex: '1 1 auto',
        // 面板高度由内容自适应：命令卡只有一行时面板很矮。
        // 滚筒卡片是绝对定位、不撑高度，必须显式给最小高度，
        // 否则视口被压到个位数像素，卡片全挤在一起。
        minHeight: '88px',
        width: '100%',
        overflow: 'hidden' as const,
        perspective: `${PERSPECTIVE}px`,
        perspectiveOrigin: '50% 50%',
    },
    drum: {
        position: 'absolute' as const,
        left: '50%',
        top: '50%',
        width: 0,
        height: 0,
        transformStyle: 'preserve-3d' as const,
    },
    // 前卡规格：默认 52px 宽（width=64 时）/ 7px 上下 padding / 12px 字号。
    // 宽度与左偏移由渲染时按 strip width 动态覆盖。
    // 邻卡与旧版 peek 一致：透明底、无描边、灰字。
    item: {
        position: 'absolute' as const,
        left: 0,
        top: 0,
        width: '56px',
        padding: '7px 0',
        marginLeft: '-28px',
        marginTop: '-16px',
        borderRadius: '6px',
        fontSize: '12px',
        fontWeight: 600,
        textAlign: 'center' as const,
        letterSpacing: '0.3px',
        whiteSpace: 'nowrap' as const,
        overflow: 'hidden' as const,
        textOverflow: 'ellipsis',
        color: 'var(--text-disabled)',
        backgroundColor: 'transparent',
        cursor: 'pointer',
        backfaceVisibility: 'hidden' as const,
        transition: 'color 0.2s ease',
    },
    itemFront: {
        color: 'var(--text-primary)',
        backgroundColor: 'var(--accent)',
    },
    fadeTop: {
        position: 'absolute' as const,
        left: 0,
        right: 0,
        top: 0,
        height: '12px',
        background: 'linear-gradient(var(--bg-primary), transparent)',
        pointerEvents: 'none' as const,
        zIndex: 2,
    },
    fadeBottom: {
        position: 'absolute' as const,
        left: 0,
        right: 0,
        bottom: 0,
        height: '12px',
        background: 'linear-gradient(transparent, var(--bg-primary))',
        pointerEvents: 'none' as const,
        zIndex: 2,
    },
    addBtn: {
        marginTop: '4px',
        color: 'var(--text-disabled)',
        cursor: 'pointer',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        flexShrink: 0,
        transition: 'color 0.15s',
    },
};

export default GroupStrip;
