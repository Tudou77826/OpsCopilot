import type { IDecoration, IMarker, Terminal } from '@xterm/xterm';

export interface RenderRange {
    key: string;
    sourceKey: string;
    row: number;
    col: number;
    width: number;
    backgroundColor?: string;
    foregroundColor?: string;
}
interface RowEntry {
    signature: string;
    marker: IMarker;
    decorations: IDecoration[];
}

const safeDispose = (value: { dispose(): void } | undefined): void => {
    try { value?.dispose(); } catch { /* xterm may already have disposed a marker during trim */ }
};

export class DecorationStore {
    private readonly rows = new Map<number, RowEntry>();

    constructor(private readonly terminal: Terminal) {}

    public update(ranges: RenderRange[]): void {
        const grouped = new Map<number, RenderRange[]>();
        for (const range of ranges) {
            const row = grouped.get(range.row) || [];
            row.push(range);
            grouped.set(range.row, row);
        }

        for (const [row, entry] of this.rows) {
            if (!grouped.has(row)) {
                this.disposeEntry(entry);
                this.rows.delete(row);
            }
        }

        const cursorRow = this.terminal.buffer.active.baseY + this.terminal.buffer.active.cursorY;
        for (const [row, rowRanges] of grouped) {
            rowRanges.sort((a, b) => a.col - b.col || a.width - b.width || a.key.localeCompare(b.key));
            const signature = rowRanges.map(range => [
                range.sourceKey,
                range.key,
                range.col,
                range.width,
                range.backgroundColor || '',
                range.foregroundColor || '',
            ].join(':')).join('|');
            const old = this.rows.get(row);
            if (old && old.signature === signature && old.marker.line === row) continue;
            if (old) this.disposeEntry(old);

            const marker = this.terminal.registerMarker(row - cursorRow);
            if (!marker) {
                this.rows.delete(row);
                continue;
            }

            const decorations: IDecoration[] = [];
            for (const range of rowRanges) {
                const decoration = this.terminal.registerDecoration({
                    marker,
                    x: range.col,
                    width: range.width,
                    backgroundColor: range.backgroundColor,
                    foregroundColor: range.foregroundColor,
                    layer: 'bottom',
                });
                if (decoration) decorations.push(decoration);
            }

            if (decorations.length === 0) {
                safeDispose(marker);
                this.rows.delete(row);
                continue;
            }
            this.rows.set(row, { signature, marker, decorations });
        }
    }

    public clear(): void {
        for (const entry of this.rows.values()) this.disposeEntry(entry);
        this.rows.clear();
    }

    public dispose(): void {
        this.clear();
    }

    public get size(): number {
        let size = 0;
        for (const entry of this.rows.values()) size += entry.decorations.length;
        return size;
    }

    private disposeEntry(entry: RowEntry): void {
        for (const decoration of entry.decorations) safeDispose(decoration);
        safeDispose(entry.marker);
    }
}
