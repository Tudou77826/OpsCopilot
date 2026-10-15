import { useCallback, useEffect, useRef, useState } from 'react';
import type { CompanionHost, CompanionSnapshot } from '../../companion/types';

/** Reads only while mounted and visible. No polling or writes after feature close. */
export function useCompanion(host: CompanionHost) {
  const [snapshot, setSnapshot] = useState<CompanionSnapshot>(), [error, setError] = useState('');
  const live = useRef(true), sequence = useRef(0);
  const refresh = useCallback(async () => {
    const request = ++sequence.current;
    try { const next = await host.snapshot(); if (live.current && request === sequence.current) { setSnapshot(next); setError(''); } return next; }
    catch (cause) { if (live.current && request === sequence.current) setError(String(cause instanceof Error ? cause.message : cause)); throw cause; }
  }, [host]);
  const accept = useCallback((next: CompanionSnapshot) => { sequence.current++; if (live.current) { setSnapshot(next); setError(''); } }, []);
  useEffect(() => {
    live.current = true; void refresh().catch(() => {});
    const visible = () => { if (!document.hidden) void refresh().catch(() => {}); };
    const timer = setInterval(visible, 5000); document.addEventListener('visibilitychange', visible);
    return () => { live.current = false; sequence.current++; clearInterval(timer); document.removeEventListener('visibilitychange', visible); };
  }, [refresh]);
  return { snapshot, error, refresh, accept };
}
