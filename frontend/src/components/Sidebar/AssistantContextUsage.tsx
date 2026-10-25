import { useEffect, useState } from 'react';
import type { AssistantContextUsage as Usage, AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import './assistantContextUsage.css';

interface Props { runtime: Pick<AssistantRuntime, 'contextUsage'>; busy: boolean; revision?: string; }

function validUsage(usage?: Usage): usage is Usage {
    return !!usage?.known && Number.isSafeInteger(usage.used) && Number.isSafeInteger(usage.size) && usage.used >= 0 && usage.size > 0;
}

/** A compact session-window gauge. Counts come exclusively from iCode. */
export function AssistantContextUsage({ runtime, busy, revision }: Props) {
    const [usage, setUsage] = useState<Usage>();
    const [unavailable, setUnavailable] = useState(false);
    useEffect(() => {
        let live = true;
        let pending = false;
        setUsage(undefined);
        async function refresh() {
            if (pending || !runtime.contextUsage || document.hidden) return;
            pending = true;
            try {
                const next = await runtime.contextUsage();
                if (live) { setUsage(validUsage(next) ? next : undefined); setUnavailable(false); }
            } catch {
                if (live) { setUsage(undefined); setUnavailable(true); }
            } finally { pending = false; }
        }
        void refresh();
        const timer = busy ? window.setInterval(() => void refresh(), 1000) : undefined;
        const visible = () => { if (!document.hidden) void refresh(); };
        document.addEventListener('visibilitychange', visible);
        return () => { live = false; if (timer !== undefined) window.clearInterval(timer); document.removeEventListener('visibilitychange', visible); };
    }, [runtime, busy, revision]);

    const known = validUsage(usage);
    const percent = known ? usage.used / usage.size * 100 : 0;
    const filled = Math.min(percent, 100);
    const label = known ? `上下文占用 ${percent.toFixed(1).replace(/\.0$/, '')}%` : unavailable ? '上下文用量暂不可用' : '上下文用量未知';
    const title = known ? `${label}\n${usage.used.toLocaleString('en-US')} / ${usage.size.toLocaleString('en-US')} tokens\niCode 回传的当前上下文用量` : `${label} · 等待 iCode 回传，不代表 0%`;
    const level = !known ? 'unknown' : percent >= 90 ? 'high' : percent >= 75 ? 'warning' : 'normal';
    return <span className={`ai-context-usage is-${level}`} tabIndex={0} title={title}
        role={known ? 'meter' : 'img'} aria-label={label}
        aria-valuemin={known ? 0 : undefined} aria-valuemax={known ? 100 : undefined}
        aria-valuenow={known ? Math.round(filled * 10) / 10 : undefined}
        aria-valuetext={known ? title : undefined}>
        <svg width="20" height="20" viewBox="0 0 20 20" aria-hidden="true">
            <circle className="ai-context-track" cx="10" cy="10" r="7.5" fill="none" strokeWidth="2.5"/>
            {known ? <circle className="ai-context-fill" cx="10" cy="10" r="7.5" fill="none" strokeWidth="2.5" pathLength="100"
                strokeDasharray={`${filled} 100`} transform="rotate(-90 10 10)"/> : <path d="M8 10h4" className="ai-context-unknown"/>}
        </svg>
    </span>;
}
