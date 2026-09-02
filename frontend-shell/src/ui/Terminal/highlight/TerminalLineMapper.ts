import type { IBuffer, IBufferCell, IBufferLine } from '@xterm/xterm';

export interface CellMapping {
    stringStart: number;
    stringEnd: number;
    row: number;
    col: number;
    width: number;
}
export interface LogicalLineSnapshot {
    id: string;
    startRow: number;
    endRow: number;
    text: string;
    hash: number;
    cells: CellMapping[];
}

export interface CellRange {
    row: number;
    col: number;
    width: number;
}

const hashText = (value: string): number => {
    let hash = 2166136261;
    for (let i = 0; i < value.length; i++) {
        hash ^= value.charCodeAt(i);
        hash = Math.imul(hash, 16777619);
    }
    return hash >>> 0;
};

const getLineEnd = (line: IBufferLine, preserveTrailingCells: boolean, reusableCell: IBufferCell): number => {
    if (preserveTrailingCells) return line.length;
    for (let col = line.length - 1; col >= 0; col--) {
        const cell = line.getCell(col, reusableCell);
        if (!cell) continue;
        if (cell.getWidth() === 0) continue;
        if (cell.getCode() !== 0 || cell.getChars().length > 0) {
            return Math.min(line.length, col + Math.max(1, cell.getWidth()));
        }
    }
    return 0;
};

export class TerminalLineMapper {
    constructor(private readonly buffer: IBuffer, private readonly cols: number) {}

    public getLogicalStart(row: number): number {
        let current = Math.max(0, Math.min(row, this.buffer.length - 1));
        while (current > 0 && this.buffer.getLine(current)?.isWrapped) current--;
        return current;
    }

    public getLogicalEnd(startRow: number): number {
        let current = startRow;
        while (current + 1 < this.buffer.length && this.buffer.getLine(current + 1)?.isWrapped) current++;
        return current;
    }

    public collectLogicalStarts(startRow: number, endRow: number): number[] {
        if (this.buffer.length === 0) return [];
        const starts: number[] = [];
        const seen = new Set<number>();
        let row = this.getLogicalStart(Math.max(0, startRow));
        const limit = Math.min(this.buffer.length - 1, endRow);
        while (row <= limit) {
            if (!seen.has(row)) {
                seen.add(row);
                starts.push(row);
            }
            row = this.getLogicalEnd(row) + 1;
        }
        return starts;
    }

    public snapshot(startRow: number): LogicalLineSnapshot | null {
        const first = this.buffer.getLine(startRow);
        if (!first) return null;

        const endRow = this.getLogicalEnd(startRow);
        const cells: CellMapping[] = [];
        let text = '';
        const reusableCell = first.getCell(0);
        if (!reusableCell) {
            return { id: `${startRow}:${endRow}`, startRow, endRow, text, hash: hashText(text), cells };
        }

        for (let row = startRow; row <= endRow; row++) {
            const line = this.buffer.getLine(row);
            if (!line) continue;
            const nextWrapped = row < endRow;
            const limit = Math.min(this.cols, getLineEnd(line, nextWrapped, reusableCell));

            for (let col = 0; col < limit; col++) {
                const cell = line.getCell(col, reusableCell);
                if (!cell || cell.getWidth() === 0) continue;

                const chars = cell.getChars() || ' ';
                const stringStart = text.length;
                text += chars;
                cells.push({
                    stringStart,
                    stringEnd: text.length,
                    row,
                    col,
                    width: Math.max(1, cell.getWidth()),
                });
            }
        }

        return {
            id: `${startRow}:${endRow}`,
            startRow,
            endRow,
            text,
            hash: hashText(text),
            cells,
        };
    }

    public toCellRanges(snapshot: LogicalLineSnapshot, start: number, end: number): CellRange[] {
        if (end <= start) return [];
        const ranges: CellRange[] = [];
        for (const cell of snapshot.cells) {
            if (cell.stringEnd <= start || cell.stringStart >= end) continue;
            const previous = ranges[ranges.length - 1];
            if (previous && previous.row === cell.row && previous.col + previous.width === cell.col) {
                previous.width += cell.width;
            } else {
                ranges.push({ row: cell.row, col: cell.col, width: cell.width });
            }
        }
        return ranges;
    }
}
