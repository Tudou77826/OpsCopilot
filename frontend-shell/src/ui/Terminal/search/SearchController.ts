import type { IDisposable, Terminal } from '@xterm/xterm';
import { SearchAddon, type ISearchDecorationOptions, type ISearchOptions } from '@xterm/addon-search';

export interface SearchQueryOptions {
    caseSensitive: boolean;
    regex: boolean;
    wholeWord: boolean;
}
export interface SearchResults {
    resultIndex: number;
    resultCount: number;
    limitReached: boolean;
}

const HIGHLIGHT_LIMIT = 10_000;

const decorations: ISearchDecorationOptions = {
    matchBackground: '#665c00',
    matchBorder: '#d7ba00',
    matchOverviewRuler: '#d7ba00',
    activeMatchBackground: '#f59e0b',
    activeMatchBorder: '#ffffff',
    activeMatchColorOverviewRuler: '#f59e0b',
};

export class SearchController {
    private readonly terminal: Terminal;
    private readonly addon = new SearchAddon({ highlightLimit: HIGHLIGHT_LIMIT });
    private readonly resultDisposable: IDisposable;
    private disposed = false;

    constructor(terminal: Terminal, onResults: (results: SearchResults) => void) {
        this.terminal = terminal;
        terminal.loadAddon(this.addon);
        this.resultDisposable = this.addon.onDidChangeResults(({ resultIndex, resultCount }) => {
            onResults({
                resultIndex,
                resultCount,
                limitReached: resultIndex === -1 && resultCount >= HIGHLIGHT_LIMIT,
            });
        });
    }

    public findNext(query: string, options: SearchQueryOptions, incremental = false): boolean {
        if (this.disposed || query.length === 0) return false;
        // 增量搜索（用户正在输入）时保持当前滚动位置，避免被第一个匹配拽到缓冲区顶部。
        // 高亮照常渲染全部匹配；只有用户主动按 下一个/上一个 时才允许跳转滚动。
        const savedViewportY = incremental ? this.terminal.buffer.active.viewportY : null;
        const found = this.addon.findNext(query, this.options(options, incremental));
        if (savedViewportY !== null && this.terminal.buffer.active.viewportY !== savedViewportY) {
            this.terminal.scrollToLine(savedViewportY);
        }
        return found;
    }

    public findPrevious(query: string, options: SearchQueryOptions): boolean {
        if (this.disposed || query.length === 0) return false;
        return this.addon.findPrevious(query, this.options(options, false));
    }

    public clear(): void {
        if (!this.disposed) this.addon.clearDecorations();
    }

    public dispose(): void {
        if (this.disposed) return;
        this.disposed = true;
        this.resultDisposable.dispose();
        this.addon.dispose();
    }

    private options(options: SearchQueryOptions, incremental: boolean): ISearchOptions {
        return {
            caseSensitive: options.caseSensitive,
            regex: options.regex,
            wholeWord: options.wholeWord,
            incremental,
            decorations,
        };
    }
}
