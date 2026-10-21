import { describe, expect, it } from 'vitest';
import { isTerminalStatusReply } from '../../../../frontend-shell/src/ui/Terminal/statusReplies';

describe('终端协议回复与人工输入分离', () => {
    it('仅豁免完整状态回复，方向键、混合输入和命令不豁免', () => {
        for (const input of ['\x1b[2;6R', '\x1b[?2;6R', '\x1b[0n', '\x1b[?1;2c', '\x1b[>0;1;0c']) expect(isTerminalStatusReply(input)).toBe(true);
        for (const input of ['\x1b[A', '\x1b[2;', '\x1b[2;6Rls\n', '\x1b[2;6R\n', 'ls\n', '']) expect(isTerminalStatusReply(input)).toBe(false);
    });
});
