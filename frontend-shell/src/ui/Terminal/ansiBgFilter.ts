/**
 * 有状态的 ANSI「设置背景色 SGR」流式过滤器。
 *
 * 用途：亮色主题下，远端 shell 的 motd/bashrc/PROMPT 常会发设置黑色或深色背景的
 * SGR（如 `\x1b[40m`、`\x1b[48;5;0m`、`\x1b[48;2;0;0;0m`），导致整屏变黑、亮色失效。
 * 本过滤器在写入 xterm 前剥离这些「设置背景色」参数，保留前景色/粗体/下划线等其它属性，
 * 使背景始终由 xterm theme.background（跟随主题）决定。
 *
 * 暗色主题下不启用（直接透传），避免影响现有体验。
 *
 * 设计要点：
 * - 有状态：ANSI 转义可能跨 write chunk 分片（如 `\x1b[4` 和 `0m` 分两次到达），
 *   需在 partial buffer 里暂存未完成的转义，下次 feed 续判。
 * - 只处理 CSI 序列里的 SGR（参数以 'm' 结尾，CSI = `\x1b[` 或 `\x9b`）。
 *   其它 CSI（光标移动/清屏等）和 OSC 等原样透传，不误伤。
 * - SGR 参数解析：按 ';' 分段。识别并剔除背景色参数段：
 *     40..47        标准 ANSI 背景（40=黑,41=红,...,47=白）
 *     48            下一段是背景色扩展（;5;N 256色 或 ;2;R;G;B 真彩色），连同扩展参数一并剔除
 *     49            默认背景 —— 保留（正是用来清除远端背景色的）
 *     100..107      bright 背景（亮色变体）
 *   剔除后若该 SGR 还有其它参数（如前景 31、粗体 1），保留剩余；全空则整条丢弃。
 */

export class AnsiBackgroundFilter {
    private partial = '';

    /**
     * 处理一段写入数据，返回剥离「设置背景色 SGR」后的数据。
     * 末尾若包含未完成的转义序列，会暂存到内部，下次 feed 续判。
     */
    feed(input: string): string {
        // 与上次残留的未完成转义拼接后统一处理
        const data = this.partial + input;
        this.partial = '';

        let out = '';
        let i = 0;
        const len = data.length;

        while (i < len) {
            const escStart = findEscape(data, i);
            if (escStart === -1) {
                // 剩余全是普通文本
                out += data.slice(i);
                break;
            }

            // 普通文本先透传
            out += data.slice(i, escStart);

            const parsed = parseEscape(data, escStart);
            if (!parsed.complete) {
                // 转义序列未结束，暂存等待下次 feed
                this.partial = data.slice(escStart);
                break;
            }

            if (parsed.kind === 'sgr') {
                out += rewriteSgr(parsed.params);
            } else {
                // 非 SGR 的 CSI / OSC / 其它转义，原样保留
                out += data.slice(escStart, parsed.end);
            }
            i = parsed.end;
        }

        return out;
    }

    /** 切换主题/重连时清空内部状态 */
    reset(): void {
        this.partial = '';
    }
}

// —— 解析辅助 ——

/** 从 from 起找下一个转义起始（ESC `[` = CSI，ESC `]` = OSC，单独 ESC，或单字节 CSI \x9b） */
function findEscape(data: string, from: number): number {
    for (let i = from; i < data.length; i++) {
        const ch = data.charCodeAt(i);
        // 0x1b = ESC, 0x9b = 单字节 CSI（C1 控制字符）
        if (ch === 0x1b || ch === 0x9b) return i;
    }
    return -1;
}

interface ParsedEscape {
    complete: boolean;
    end: number;          // 序列结束位置（exclusive）
    kind: 'sgr' | 'other';
    params: string;       // SGR 的参数串（不含末尾 m），仅 kind==='sgr' 有意义
}

/** 从 escStart 起解析一个转义序列，判断是否完整、是否 SGR */
function parseEscape(data: string, escStart: number): ParsedEscape {
    const len = data.length;
    const first = data.charCodeAt(escStart);

    // 单字节 CSI (0x9b)：等价于 ESC [
    if (first === 0x9b) {
        return parseCsi(data, escStart + 1, escStart);
    }

    // ESC (0x1b) 开头，需看第二个字节
    if (escStart + 1 >= len) {
        return { complete: false, end: len, kind: 'other', params: '' };
    }
    const second = data.charCodeAt(escStart + 1);

    if (second === 0x5b /* [ */) {
        return parseCsi(data, escStart + 2, escStart);
    }
    if (second === 0x5d /* ] */) {
        // OSC：以 BEL(0x07) 或 ST(ESC \) 结束。这里不关心内容，只找结尾原样透传。
        return parseOsc(data, escStart + 2, escStart);
    }

    // 其它两字节 ESC 序列（如 ESC c 重置、ESC = 等），原样保留
    return { complete: true, end: escStart + 2, kind: 'other', params: '' };
}

/** 解析 CSI：从 paramStart 起读参数直到 final byte（0x40..0x7e） */
function parseCsi(data: string, paramStart: number, escStart: number): ParsedEscape {
    void escStart;
    const len = data.length;
    let i = paramStart;
    // 参数字节区间：0x30..0x3f，中间字节 0x20..0x2f；final byte 0x40..0x7e
    while (i < len) {
        const ch = data.charCodeAt(i);
        if (ch >= 0x40 && ch <= 0x7e) {
            const params = data.slice(paramStart, i);
            const isSgr = ch === 0x6d; // 'm'
            return { complete: true, end: i + 1, kind: isSgr ? 'sgr' : 'other', params };
        }
        i++;
    }
    return { complete: false, end: len, kind: 'other', params: '' };
}

/** 解析 OSC：找 BEL 或 ST 结尾，内容不解析，原样透传 */
function parseOsc(data: string, bodyStart: number, escStart: number): ParsedEscape {
    void escStart;
    const len = data.length;
    let i = bodyStart;
    while (i < len) {
        const ch = data.charCodeAt(i);
        if (ch === 0x07 /* BEL */) {
            return { complete: true, end: i + 1, kind: 'other', params: '' };
        }
        if (ch === 0x1b /* ESC */ && i + 1 < len && data.charCodeAt(i + 1) === 0x5c /* \ */) {
            return { complete: true, end: i + 2, kind: 'other', params: '' };
        }
        i++;
    }
    return { complete: false, end: len, kind: 'other', params: '' };
}

/**
 * 重写 SGR 参数串：剔除所有「设置背景色」的参数段，保留其余。
 * 参数串形如 "1;31;40" 或 "48;5;0" 或 "0" 或 ""（空表示 reset）。
 * 返回完整 SGR 序列（含 \x1b[ 和 m），或空串（整条剔除）。
 */
function rewriteSgr(params: string): string {
    if (params === '') {
        // `\x1b[m` 等价于 `\x1b[0m`（全局 reset），保留——它正是用来清除远端背景色的
        return '\x1b[m';
    }

    const segs = params.split(';');
    const kept: string[] = [];

    let i = 0;
    while (i < segs.length) {
        const raw = segs[i];
        const n = parseDec(raw);

        // 不可识别段（非纯数字，如 '?' 私有序列残留）——保守保留
        if (n === null) {
            kept.push(raw);
            i++;
            continue;
        }

        // 背景色相关：40..49、48（扩展）、100..109
        if (n >= 40 && n <= 49) {
            if (n === 48) {
                // 48 后面跟扩展参数：;5;N（256色，2 段）或 ;2;R;G;B（真彩色，4 段）
                const ext = skipExtendedColor(segs, i + 1);
                i = ext;
                continue;
            }
            if (n === 49) {
                // 49 = 默认背景 —— 保留（清除远端背景色）
                kept.push(raw);
                i++;
                continue;
            }
            // 40..47 标准 ANSI 背景：剔除
            i++;
            continue;
        }

        if (n >= 100 && n <= 109) {
            // 100..107 bright 背景：剔除。108/109 极少见，一并按背景处理。
            i++;
            continue;
        }

        // 非背景色参数（前景、粗体、斜体、下划线、reset 0 等）：保留
        kept.push(raw);
        i++;
    }

    if (kept.length === 0) {
        // 整条 SGR 全是背景色参数 → 整条丢弃（不输出任何字节）
        return '';
    }
    return '\x1b[' + kept.join(';') + 'm';
}

/** 跳过 48 之后的扩展色参数（;5;N 或 ;2;R;G;B），返回跳完后下一个段的索引 */
function skipExtendedColor(segs: string[], start: number): number {
    if (start >= segs.length) return start;
    const mode = parseDec(segs[start]);
    if (mode === 5) {
        // ;5;N —— 再消费 1 段
        return Math.min(segs.length, start + 2);
    }
    if (mode === 2) {
        // ;2;R;G;B —— 再消费 3 段
        return Math.min(segs.length, start + 4);
    }
    // 未知扩展模式，保守只跳过 mode 本身那一段
    return start + 1;
}

function parseDec(s: string): number | null {
    if (s === '') return null;
    let v = 0;
    for (let i = 0; i < s.length; i++) {
        const c = s.charCodeAt(i);
        if (c < 0x30 || c > 0x39) return null; // 非数字
        v = v * 10 + (c - 0x30);
    }
    return v;
}
