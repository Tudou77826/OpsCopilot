import { describe, expect, it } from 'vitest';
import { TerminalLineMapper } from './TerminalLineMapper';

interface CellSpec {
    chars: string;
    width: number;
}

const makeLine = (cells: CellSpec[], isWrapped = false) => ({
    isWrapped,
    length: cells.length,
    getCell: (col: number) => {
        const cell = cells[col];
        if (!cell) return undefined;
        return {
            getChars: () => cell.chars,
            getWidth: () => cell.width,
            getCode: () => cell.chars ? cell.chars.codePointAt(0)! : 0,
        };
    },
});
const makeBuffer = (lines: ReturnType<typeof makeLine>[]) => ({
    length: lines.length,
    getLine: (row: number) => lines[row],
});

describe('TerminalLineMapper', () => {
    it('maps offsets after a double-width CJK cell to terminal columns', () => {
        const buffer = makeBuffer([makeLine([
            { chars: 'A', width: 1 },
            { chars: '中', width: 2 },
            { chars: '', width: 0 },
            { chars: 'B', width: 1 },
        ])]);
        const mapper = new TerminalLineMapper(buffer as any, 4);
        const snapshot = mapper.snapshot(0)!;

        expect(snapshot.text).toBe('A中B');
        expect(mapper.toCellRanges(snapshot, 2, 3)).toEqual([{ row: 0, col: 3, width: 1 }]);
    });

    it('maps one match across soft-wrapped physical rows', () => {
        const buffer = makeBuffer([
            makeLine([
                { chars: 'E', width: 1 },
                { chars: 'R', width: 1 },
                { chars: 'R', width: 1 },
            ]),
            makeLine([
                { chars: 'O', width: 1 },
                { chars: 'R', width: 1 },
            ], true),
        ]);
        const mapper = new TerminalLineMapper(buffer as any, 3);
        const snapshot = mapper.snapshot(0)!;

        expect(snapshot.text).toBe('ERROR');
        expect(mapper.toCellRanges(snapshot, 0, 5)).toEqual([
            { row: 0, col: 0, width: 3 },
            { row: 1, col: 0, width: 2 },
        ]);
    });

    it('keeps combining characters attached to one terminal cell', () => {
        const buffer = makeBuffer([makeLine([
            { chars: 'e\u0301', width: 1 },
            { chars: 'X', width: 1 },
        ])]);
        const mapper = new TerminalLineMapper(buffer as any, 2);
        const snapshot = mapper.snapshot(0)!;

        expect(snapshot.text).toBe('e\u0301X');
        expect(mapper.toCellRanges(snapshot, 0, 2)).toEqual([{ row: 0, col: 0, width: 1 }]);
        expect(mapper.toCellRanges(snapshot, 2, 3)).toEqual([{ row: 0, col: 1, width: 1 }]);
    });
});
