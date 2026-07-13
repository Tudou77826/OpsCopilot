/// <reference lib="webworker" />

import type { MatcherLine, MatcherResult, MatcherRule } from './RuleMatcher';

interface MatchRequest {
    requestId: number;
    lines: MatcherLine[];
    rules: MatcherRule[];
    maxMatchesPerLine: number;
}

const scope = self as unknown as DedicatedWorkerGlobalScope;

scope.onmessage = (event: MessageEvent<MatchRequest>): void => {
    const { requestId, lines, rules, maxMatchesPerLine } = event.data;
    const compiled = rules.map(rule => ({ rule, regex: new RegExp(rule.source, rule.flags) }));
    const results: MatcherResult[] = [];

    for (const line of lines) {
        const accepted: MatcherResult[] = [];
        for (const { rule, regex } of compiled) {
            regex.lastIndex = 0;
            let match: RegExpExecArray | null;
            while ((match = regex.exec(line.text)) !== null) {
                const start = match.index;
                const end = start + match[0].length;
                if (end > start && !accepted.some(existing => !(end <= existing.start || start >= existing.end))) {
                    accepted.push({ lineId: line.id, ruleId: rule.id, start, end });
                    if (accepted.length >= maxMatchesPerLine) break;
                }
                if (regex.lastIndex === match.index) regex.lastIndex++;
            }
            if (accepted.length >= maxMatchesPerLine) break;
        }
        results.push(...accepted);
    }

    scope.postMessage({ requestId, results });
};
export {};
