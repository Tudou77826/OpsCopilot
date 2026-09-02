import { describe, expect, it } from 'vitest';
import { assessPattern } from './regexSafety';

describe('rule regex safety', () => {
    it('flags catastrophic-backtracking shapes as non-enabling', () => {
        // 确证的灾难性回溯形态必须不能启用（执行侧丢弃 / 界面禁用启用）
        expect(assessPattern('(.*)+').canEnable).toBe(false);
        expect(assessPattern('(.+)').canEnable).toBe(true); // (.+) 单独是安全的
        expect(assessPattern('(.+)+').canEnable).toBe(false);
        expect(assessPattern('(?:.*)*').canEnable).toBe(false);
        expect(assessPattern('.*.*').canEnable).toBe(false);
        expect(assessPattern('.*+').canEnable).toBe(false); // 语法非法 + 灾难
    });

    it('allows ordinary operational matching rules', () => {
        // 正常运维匹配规则应当可启用
        expect(assessPattern('(?i)\\berror\\b').canEnable).toBe(true);
        expect(assessPattern('HTTP/[12]\\.[01] 5\\d{2}').canEnable).toBe(true);
        expect(assessPattern('[0-9]+').canEnable).toBe(true);
        expect(assessPattern('(\\d+)').canEnable).toBe(true);
    });

    it('no longer false-positives on safe nested quantifiers', () => {
        // 回归保护：此前 hasUnsafeRegexShape 误杀的形态，现在必须可启用。
        // 这些正是「界面判定安全、执行侧却丢弃」导致「该亮的不亮」的根因样本。
        expect(assessPattern('(a+)+').canEnable).toBe(true);
        expect(assessPattern('(a+)+$').canEnable).toBe(true);
        expect(assessPattern('(a|b)+').canEnable).toBe(true);
        expect(assessPattern('(error|fail)+').canEnable).toBe(true);
        expect(assessPattern('(foo|fo)+$').canEnable).toBe(true);
    });

    it('rejects syntactically invalid regex', () => {
        // 语法非法 → 不能启用，且给出错误信息
        const r = assessPattern('(');
        expect(r.canEnable).toBe(false);
        expect(r.syntaxError).toBeTruthy();
    });
});
