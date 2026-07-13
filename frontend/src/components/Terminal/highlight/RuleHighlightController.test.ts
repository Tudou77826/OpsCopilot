import { describe, expect, it } from 'vitest';
import { hasUnsafeRegexShape } from './RuleHighlightController';

describe('rule regex safety', () => {
    it('rejects common catastrophic-backtracking shapes', () => {
        expect(hasUnsafeRegexShape('(a+)+$')).toBe(true);
        expect(hasUnsafeRegexShape('(.*.*)+')).toBe(true);
        expect(hasUnsafeRegexShape('(foo|fo)+$')).toBe(true);
    });

    it('allows ordinary operational matching rules', () => {
        expect(hasUnsafeRegexShape('(?i)\\berror\\b')).toBe(false);
        expect(hasUnsafeRegexShape('HTTP/[12]\\.[01] 5\\d{2}')).toBe(false);
    });
});
