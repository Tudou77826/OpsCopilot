import { useEffect, useRef, useState } from 'react';
import { isEditableTarget, matchesShortcut } from '../command/shortcut';
import type { CommandQueryResult } from '../command/CommandQueryOverlay';

export interface CommandQueryHost {
  generate(query: string): Promise<CommandQueryResult>;
  type(command: string): void;
  copy(command: string): Promise<void>;
  warn(message: string): void;
}

/** One command-generation lifecycle for desktop and embedded product entries. */
export function useCommandQuery(host: CommandQueryHost, activeTerminalId: string | null, shortcut = 'Ctrl+K') {
  const [visible, updateVisible] = useState(false), [query, setQuery] = useState('');
  const [result, setResult] = useState<CommandQueryResult | null>(null);
  const [loading, setLoading] = useState(false), [error, setError] = useState('');
  const epoch = useRef(0), currentHost = useRef(host);
  currentHost.current = host;
  const setVisible = (value: boolean) => { epoch.current++; setLoading(false); updateVisible(value); };
  useEffect(() => { setVisible(false); return () => { epoch.current++; }; }, [activeTerminalId]);
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => {
      if (!matchesShortcut(event, shortcut) || isEditableTarget(event.composedPath()[0] ?? event.target)) return;
      event.preventDefault(); event.stopPropagation();
      if (!activeTerminalId) { currentHost.current.warn('请先选择一个激活的终端'); return; }
      setQuery(''); setResult(null); setError(''); setVisible(true);
    };
    window.addEventListener('keydown', keydown, true);
    return () => window.removeEventListener('keydown', keydown, true);
  }, [activeTerminalId, shortcut]);
  const generate = async (override?: string) => {
    const text = (override ?? query).trim(); if (!text) return;
    const generation = ++epoch.current;
    setQuery(override ?? query); setLoading(true); setError(''); setResult(null);
    try {
      const value = await currentHost.current.generate(text);
      if (!value || typeof value.command !== 'string') throw new Error('命令生成结果无效');
      if (generation === epoch.current) setResult(value);
    } catch (err) {
      if (generation === epoch.current) setError(err instanceof Error ? err.message : String(err));
    } finally { if (generation === epoch.current) setLoading(false); }
  };
  const copy = async () => {
    const command = result?.command?.trim(); if (!command) return;
    try { await currentHost.current.copy(command); } catch (err) { setError(String(err)); }
  };
  const type = () => {
    const command = result?.command?.trim(); if (!command || !activeTerminalId) return;
    try { currentHost.current.type(command); setVisible(false); } catch (err) { setError(String(err)); }
  };
  return { visible, setVisible, query, setQuery, result, loading, error, generate, copy, type };
}
