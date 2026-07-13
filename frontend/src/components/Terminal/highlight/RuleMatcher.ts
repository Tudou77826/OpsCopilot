export interface MatcherRule {
    id: string;
    source: string;
    flags: string;
}
export interface MatcherLine {
    id: string;
    text: string;
}

export interface MatcherResult {
    lineId: string;
    ruleId: string;
    start: number;
    end: number;
}

interface WorkerResponse {
    requestId: number;
    results: MatcherResult[];
}

const MAX_MATCHES_PER_LINE = 100;
const WORKER_TIMEOUT_MS = 75;

const matchSynchronously = (lines: MatcherLine[], rules: MatcherRule[]): MatcherResult[] => {
    const results: MatcherResult[] = [];
    for (const line of lines) {
        const accepted: MatcherResult[] = [];
        for (const rule of rules) {
            const regex = new RegExp(rule.source, rule.flags);
            let match: RegExpExecArray | null;
            while ((match = regex.exec(line.text)) !== null) {
                const start = match.index;
                const end = start + match[0].length;
                if (end > start && !accepted.some(existing => !(end <= existing.start || start >= existing.end))) {
                    accepted.push({ lineId: line.id, ruleId: rule.id, start, end });
                    if (accepted.length >= MAX_MATCHES_PER_LINE) break;
                }
                if (regex.lastIndex === match.index) regex.lastIndex++;
            }
            if (accepted.length >= MAX_MATCHES_PER_LINE) break;
        }
        results.push(...accepted);
    }
    return results;
};

export class RuleMatcher {
    private worker: Worker | null = null;
    private requestId = 0;
    private disposed = false;

    public async match(lines: MatcherLine[], rules: MatcherRule[]): Promise<MatcherResult[]> {
        if (this.disposed || lines.length === 0 || rules.length === 0) return [];
        if (typeof Worker === 'undefined') return matchSynchronously(lines, rules);

        const worker = this.ensureWorker();
        const requestId = ++this.requestId;
        return new Promise<MatcherResult[]>(resolve => {
            const timeout = window.setTimeout(() => {
                worker.removeEventListener('message', onMessage);
                this.resetWorker();
                console.warn('规则高亮匹配超时，本批结果已丢弃');
                resolve([]);
            }, WORKER_TIMEOUT_MS);

            const onMessage = (event: MessageEvent<WorkerResponse>): void => {
                if (event.data.requestId !== requestId) return;
                window.clearTimeout(timeout);
                worker.removeEventListener('message', onMessage);
                resolve(event.data.results);
            };

            worker.addEventListener('message', onMessage);
            worker.postMessage({ requestId, lines, rules, maxMatchesPerLine: MAX_MATCHES_PER_LINE });
        });
    }

    public dispose(): void {
        this.disposed = true;
        this.resetWorker();
    }

    private ensureWorker(): Worker {
        if (!this.worker) {
            this.worker = new Worker(new URL('./matcher.worker.ts', import.meta.url), { type: 'module' });
        }
        return this.worker;
    }

    private resetWorker(): void {
        this.worker?.terminate();
        this.worker = null;
    }
}
