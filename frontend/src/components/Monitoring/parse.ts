import { CommandResult } from './monitoringTypes';

export function parsePercent(s?: string): number | null {
    if (!s) return null;
    const v = Number(String(s).replace('%', '').trim());
    if (Number.isFinite(v)) return v;
    return null;
}

export function toneForPercent(v: number | null, warn: number, bad: number): 'neutral' | 'good' | 'warn' | 'bad' {
    if (v == null) return 'neutral';
    if (v >= bad) return 'bad';
    if (v >= warn) return 'warn';
    return 'good';
}

export function extractVmVersionShort(r: CommandResult): string {
    const t = (r.output || '').trim();
    if (!t) return '';
    const lines = t.split('\n').map(x => x.trim()).filter(Boolean);
    const first = lines[0] || '';
    if (first.length <= 60) return first;
    return first.slice(0, 60) + '...';
}

export function parseJstatGcutilOnce(r: CommandResult): Record<string, string> | null {
    const out = (r.output || '').trim();
    if (!out) return null;
    const lines = out.split('\n').map(x => x.trim()).filter(Boolean);
    if (lines.length < 2) return null;
    const header = lines[0].split(/\s+/);
    const values = lines[1].split(/\s+/);
    if (header.length !== values.length) return null;
    const res: Record<string, string> = {};
    for (let i = 0; i < header.length; i++) {
        res[header[i]] = values[i];
    }
    return res;
}

export function parseNumberLoose(v?: string): number | null {
    if (!v) return null;
    const n = Number(String(v).trim());
    if (Number.isFinite(n)) return n;
    return null;
}

export function formatBytesFromKB(kb?: number): string {
    if (kb == null || !Number.isFinite(kb)) return '-';
    const bytes = kb * 1024;
    const gb = 1024 * 1024 * 1024;
    const mb = 1024 * 1024;
    if (bytes >= gb) return `${(bytes / gb).toFixed(2)} GB`;
    if (bytes >= mb) return `${(bytes / mb).toFixed(0)} MB`;
    return `${Math.max(0, Math.round(bytes / 1024))} KB`;
}
