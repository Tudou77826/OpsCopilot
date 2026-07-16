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
    private readonly addon = new SearchAddon({ highlightLimit: HIGHLIGHT_LIMIT });
    private readonly resultDisposable: IDisposable;
    private disposed = false;

    constructor(terminal: Terminal, onResults: (results: SearchResults) => void) {
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
        return this.addon.findNext(query, this.options(options, incremental));
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
