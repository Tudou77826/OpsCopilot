import type { Terminal } from '@xterm/xterm';
import { HighlightRule } from '../highlightTypes';
import { DecorationStore, RenderRange } from './DecorationStore';
import { TerminalLineMapper } from './TerminalLineMapper';
import { MatcherRule, RuleMatcher } from './RuleMatcher';
import { assessPattern } from './regexSafety';

interface CompiledRule {
    rule: HighlightRule;
    matcher: MatcherRule;
}

const MAX_DECORATIONS = 500;
const MAX_LOGICAL_LINE_LENGTH = 32 * 1024;
const FRAME_BUDGET_MS = 4;

const validColor = (value?: string): string | undefined =>
    value && /^#[0-9a-f]{6}$/i.test(value) ? value : undefined;

// 规则丢弃标准与编辑界面同源（assessPattern）：severe（确证灾难性回溯）或语法
// 非法时丢弃，防老鼠屎正则卡死共享 worker。safe/moderate/high 一律保留——
// 此前用本地 hasUnsafeRegexShape 会误杀 (a+)+、(error|fail)+ 等安全正则，导致
// 「界面判定安全、执行侧却静默丢弃，该亮的不亮」，现在两处共用同一判断。
const compileRules = (rules: HighlightRule[]): CompiledRule[] => rules
    .filter(rule => rule?.is_enabled && rule.pattern && assessPattern(rule.pattern).canEnable)
    .sort((a, b) => (a.priority ?? 0) - (b.priority ?? 0))
    .map(rule => {
        let pattern = rule.pattern;
        // 默认大小写不敏感：运维关键词（error/Error/ERROR）通常希望都能高亮。
        // 用户仍可在 pattern 内书写显式大小写（如 [Ee]rror）来覆盖此默认。
        if (pattern.startsWith('(?i)')) {
            pattern = pattern.slice(4);
        }
        try {
            const flags = 'gi';
            new RegExp(pattern, flags);
            return { rule, matcher: { id: rule.id, source: pattern, flags } };
        } catch {
            return null;
        }
    })
    .filter((value): value is CompiledRule => value !== null);

export class RuleHighlightController {
    private readonly decorations: DecorationStore;
    private readonly matcher = new RuleMatcher();
    private rules: CompiledRule[] = [];
    private enabled = true;
    private visible = true;
    private generation = 0;
    private timer: number | null = null;
    private disposed = false;

    constructor(private readonly terminal: Terminal) {
        this.decorations = new DecorationStore(terminal);
    }

    public setRules(rules: HighlightRule[] | undefined, enabled: boolean): void {
        this.enabled = enabled;
        this.rules = compileRules(rules || []);
        this.invalidate();
        this.schedule('rules');
    }

    public setVisible(visible: boolean): void {
        if (this.visible === visible) return;
        this.visible = visible;
        if (!visible) {
            this.invalidate();
            if (this.timer !== null) window.clearTimeout(this.timer);
            this.timer = null;
            return;
        }
        this.schedule('resize');
    }

    public schedule(_reason: 'output' | 'scroll' | 'resize' | 'rules' | 'buffer', delay = 0): void {
        if (this.disposed || !this.visible) return;
        if (this.timer !== null) window.clearTimeout(this.timer);
        const generation = ++this.generation;
        this.timer = window.setTimeout(() => {
            this.timer = null;
            this.scan(generation);
        }, delay);
    }

    public invalidate(): void {
        this.generation++;
        this.decorations.clear();
    }

    public dispose(): void {
        this.disposed = true;
        this.generation++;
        if (this.timer !== null) window.clearTimeout(this.timer);
        this.timer = null;
        this.decorations.dispose();
        this.matcher.dispose();
    }

    private scan(generation: number): void {
        if (this.disposed || !this.visible || generation !== this.generation) return;
        if (!this.enabled || this.rules.length === 0) {
            this.decorations.clear();
            return;
        }

        const buffer = this.terminal.buffer.active;
        const viewportStart = buffer.viewportY;
        const start = Math.max(0, viewportStart - this.terminal.rows);
        const end = Math.min(buffer.length - 1, viewportStart + this.terminal.rows * 2 - 1);
        const mapper = new TerminalLineMapper(buffer, this.terminal.cols);
        const starts = mapper.collectLogicalStarts(start, end);
        const snapshots = new Map<string, ReturnType<TerminalLineMapper['snapshot']> & {}>();
        let index = 0;

        const step = async (): Promise<void> => {
            if (this.disposed || generation !== this.generation) return;
            const startedAt = performance.now();

            for (; index < starts.length; index++) {
                const snapshot = mapper.snapshot(starts[index]);
                if (!snapshot) continue;
                if (snapshot.text.length <= MAX_LOGICAL_LINE_LENGTH) snapshots.set(snapshot.id, snapshot);
                if (performance.now() - startedAt >= FRAME_BUDGET_MS) {
                    index++;
                    window.setTimeout(() => { void step(); }, 0);
                    return;
                }
            }

            const matches = await this.matcher.match(
                [...snapshots.values()].map(snapshot => ({ id: snapshot.id, text: snapshot.text })),
                this.rules.map(rule => rule.matcher)
            );
            if (this.disposed || generation !== this.generation) return;

            const rulesById = new Map(this.rules.map(compiled => [compiled.rule.id, compiled.rule]));
            const renderRanges: RenderRange[] = [];
            for (const match of matches) {
                const snapshot = snapshots.get(match.lineId);
                const rule = rulesById.get(match.ruleId);
                if (!snapshot || !rule) continue;
                for (const range of mapper.toCellRanges(snapshot, match.start, match.end)) {
                    if (renderRanges.length >= MAX_DECORATIONS) break;
                    renderRanges.push({
                        key: `${rule.id}:${match.start}:${match.end}`,
                        sourceKey: `${snapshot.id}:${snapshot.hash}`,
                        row: range.row,
                        col: range.col,
                        width: range.width,
                        backgroundColor: validColor(rule.style?.background_color),
                        foregroundColor: validColor(rule.style?.color),
                    });
                }
                if (renderRanges.length >= MAX_DECORATIONS) break;
            }
            this.decorations.update(renderRanges);
        };

        void step();
    }
}
