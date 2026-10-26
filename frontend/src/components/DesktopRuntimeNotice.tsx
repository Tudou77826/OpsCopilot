import { useEffect, useState } from 'react';
import { GetDesktopRuntimeStatus, RetryDesktopRuntime } from '../../wailsjs/go/main/App';
import { EventsOn } from '../../wailsjs/runtime/runtime';

type Status = { phase: string; message: string; detail?: string };

// Polling preserves startup status even if the UI mounts after preparation.
export default function DesktopRuntimeNotice({ compact = false }: { compact?: boolean }) {
    const [status, setStatus] = useState<Status | null>(null);
    useEffect(() => {
        let disposed = false;
        let timer: ReturnType<typeof setTimeout> | undefined;
        const off = EventsOn('desktop-runtime-status', (next: Status) => {
            if (!disposed) setStatus(next);
        });
        const refresh = async () => {
            try {
                const next = await GetDesktopRuntimeStatus();
                if (disposed) return;
                setStatus(next);
                if (next.phase === 'idle' || next.phase === 'preparing') timer = setTimeout(refresh, 500);
            } catch { /* Browser-only previews may not expose the desktop bridge. */ }
        };
        void refresh();
        return () => { disposed = true; clearTimeout(timer); off(); };
    }, []);
    if (!status || status.phase === 'idle' || status.phase === 'unmanaged' || (!compact && status.phase === 'ready')) return null;
    const failed = status.phase === 'error';
    return <div role={failed ? 'alert' : 'status'} style={{
        ...(compact ? {} : { position: 'fixed', bottom: 36, left: 16, zIndex: 1500, maxWidth: 'min(480px, calc(100vw - 32px))' }),
        padding: '10px 14px', borderRadius: 8, background: 'var(--bg-secondary)',
        border: '1px solid var(--border)', color: failed ? 'var(--warning)' : 'var(--text-secondary)', fontSize: 12,
    }}>
        <span>{status.message}</span>
        {failed && <>
            <button type="button" onClick={() => {
                setStatus({ phase: 'retry', message: '正在重试…' });
                void RetryDesktopRuntime().catch(() => setStatus(status));
            }} style={{ marginLeft: 12 }}>重试</button>
            <details style={{ marginTop: 6 }}><summary>查看原因</summary><div style={{ overflowWrap: 'anywhere' }}>{status.detail}</div></details>
        </>}
    </div>;
}
