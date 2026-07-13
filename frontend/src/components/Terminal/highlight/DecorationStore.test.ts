import { describe, expect, it, vi } from 'vitest';
import { DecorationStore, RenderRange } from './DecorationStore';

const range = (sourceKey = 'line:1'): RenderRange => ({
    key: 'rule:0:5',
    sourceKey,
    row: 3,
    col: 2,
    width: 5,
    backgroundColor: '#112233',
});
describe('DecorationStore', () => {
    it('reuses an unchanged row and replaces it when the source changes', () => {
        const decorationDispose = vi.fn();
        const markerDispose = vi.fn();
        const registerMarker = vi.fn(() => ({ line: 3, dispose: markerDispose }));
        const registerDecoration = vi.fn(() => ({ dispose: decorationDispose }));
        const terminal = {
            buffer: { active: { baseY: 0, cursorY: 0 } },
            registerMarker,
            registerDecoration,
        } as any;
        const store = new DecorationStore(terminal);

        store.update([range()]);
        store.update([range()]);
        expect(registerMarker).toHaveBeenCalledTimes(1);
        expect(registerDecoration).toHaveBeenCalledTimes(1);

        store.update([range('line:2')]);
        expect(decorationDispose).toHaveBeenCalledTimes(1);
        expect(markerDispose).toHaveBeenCalledTimes(1);
        expect(registerDecoration).toHaveBeenCalledTimes(2);

        store.dispose();
        expect(decorationDispose).toHaveBeenCalledTimes(2);
        expect(markerDispose).toHaveBeenCalledTimes(2);
    });
});
