import React, { useMemo } from 'react';

interface SparklineProps {
    data: Array<number | null | undefined>;
    height?: number;
    stroke?: string;
    fill?: string;
    min?: number;
    max?: number;
    showArea?: boolean;
}

export default function Sparkline({
    data,
    height = 44,
    stroke = '#4da3ff',
    fill = 'rgba(77,163,255,0.18)',
    min,
    max,
    showArea = true
}: SparklineProps) {
    const width = 160;
    const padding = 4;

    const points = useMemo(() => {
        const vals = data.map(v => (v == null ? null : Number(v))).filter(v => v != null) as number[];
        if (vals.length <= 1) return { line: '', area: '', minV: 0, maxV: 1 };
        const minV = min != null ? min : Math.min(...vals);
        const maxV = max != null ? max : Math.max(...vals);
        const span = maxV - minV || 1;

        const stepX = (width - padding * 2) / Math.max(1, data.length - 1);
        const toY = (v: number) => {
            const ratio = (v - minV) / span;
            const y = padding + (1 - ratio) * (height - padding * 2);
            return Math.max(padding, Math.min(height - padding, y));
        };

        let lastValid: { x: number; y: number } | null = null;
        const segments: string[] = [];
        const areaPoints: Array<{ x: number; y: number }> = [];

        for (let i = 0; i < data.length; i++) {
            const v = data[i];
            if (v == null) continue;
            const x = padding + i * stepX;
            const y = toY(Number(v));
            const cmd = lastValid ? `L ${x.toFixed(2)} ${y.toFixed(2)}` : `M ${x.toFixed(2)} ${y.toFixed(2)}`;
            segments.push(cmd);
            lastValid = { x, y };
            areaPoints.push({ x, y });
        }

        let area = '';
        if (areaPoints.length >= 2) {
            const first = areaPoints[0];
            const last = areaPoints[areaPoints.length - 1];
            const d = [
                `M ${first.x.toFixed(2)} ${(height - padding).toFixed(2)}`,
                `L ${first.x.toFixed(2)} ${first.y.toFixed(2)}`,
                ...areaPoints.slice(1).map(p => `L ${p.x.toFixed(2)} ${p.y.toFixed(2)}`),
                `L ${last.x.toFixed(2)} ${(height - padding).toFixed(2)}`,
                'Z'
            ];
            area = d.join(' ');
        }

        return { line: segments.join(' '), area, minV, maxV };
    }, [data, height, min, max]);

    return (
        <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} style={styles.svg}>
            {showArea && points.area && <path d={points.area} fill={fill} stroke="none" />}
            {points.line && <path d={points.line} fill="none" stroke={stroke} strokeWidth="2" strokeLinejoin="round" strokeLinecap="round" />}
        </svg>
    );
}

const styles: Record<string, React.CSSProperties> = {
    svg: {
        display: 'block',
        width: '100%',
        height: '44px'
    }
};

