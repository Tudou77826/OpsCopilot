import { CommandSuggestion, DiagnosticStep, KnowledgeReference, KnowledgeResponse } from './types';

const SOURCE_RE = /^(.*?)(?:#L(\d+)(?:[-–]L?(\d+))?)?$/;

export function parseKnowledgeSource(source: string): Omit<KnowledgeReference, 'id'> | null {
    const normalized = String(source || '').trim();
    if (!normalized) return null;

    const match = normalized.match(SOURCE_RE);
    if (!match) {
        return { label: normalized, path: normalized };
    }

    const path = match[1].trim();
    if (!path) return null;
    const line = match[2] ? Number(match[2]) : undefined;
    const lineEnd = match[3] ? Number(match[3]) : undefined;
    return {
        label: path,
        path,
        ...(Number.isFinite(line) ? { line } : {}),
        ...(Number.isFinite(lineEnd) ? { lineEnd } : {}),
    };
}

function normalizeStep(value: unknown, index: number): DiagnosticStep | null {
    if (typeof value === 'string') {
        const description = value.trim();
        return description ? { step: index + 1, description } : null;
    }
    if (!value || typeof value !== 'object') return null;

    const raw = value as Record<string, unknown>;
    const description = String(raw.description ?? raw.content ?? '').trim();
    const title = String(raw.title ?? '').trim();
    if (!description && !title) return null;
    const step = Number(raw.step);
    return {
        step: Number.isFinite(step) ? step : index + 1,
        ...(title ? { title } : {}),
        description: description || title,
    };
}

function normalizeCommand(value: unknown): CommandSuggestion | null {
    if (typeof value === 'string') {
        const command = value.trim();
        return command ? { command } : null;
    }
    if (!value || typeof value !== 'object') return null;

    const raw = value as Record<string, unknown>;
    const command = String(raw.command ?? '').trim();
    if (!command) return null;
    const description = String(raw.description ?? '').trim();
    const risk = String(raw.risk ?? '').trim();
    const source = String(raw.source ?? '').trim();
    return {
        command,
        ...(description ? { description } : {}),
        ...(risk ? { risk } : {}),
        ...(source ? { source } : {}),
    };
}

function stripMarkdownFence(content: string): string {
    const trimmed = String(content ?? '').trim();
    const match = trimmed.match(/^```(?:json)?\s*([\s\S]*?)\s*```$/i);
    return match ? match[1].trim() : trimmed;
}

export function parseKnowledgeResponse(content: string): KnowledgeResponse | null {
    const candidate = stripMarkdownFence(content);
    if (!candidate.startsWith('{')) return null;

    let raw: Record<string, unknown>;
    try {
        raw = JSON.parse(candidate) as Record<string, unknown>;
    } catch {
        return null;
    }

    const hasKnowledgeShape = 'summary' in raw || Array.isArray(raw.steps) || Array.isArray(raw.commands);
    if (!hasKnowledgeShape) return null;

    const steps = (Array.isArray(raw.steps) ? raw.steps : [])
        .map(normalizeStep)
        .filter((step): step is DiagnosticStep => step !== null);
    const commands = (Array.isArray(raw.commands) ? raw.commands : [])
        .map(normalizeCommand)
        .filter((command): command is CommandSuggestion => command !== null);

    const referenceMap = new Map<string, Omit<KnowledgeReference, 'id'>>();
    commands.forEach(command => {
        if (!command.source) return;
        const parsed = parseKnowledgeSource(command.source);
        if (parsed) referenceMap.set(`${parsed.path}:${parsed.line ?? ''}:${parsed.lineEnd ?? ''}`, parsed);
    });

    return {
        summary: typeof raw.summary === 'string' ? raw.summary : undefined,
        steps,
        commands,
        references: Array.from(referenceMap.values()).map((reference, index) => ({
            ...reference,
            id: index + 1,
        })),
    };
}

export function referencesFromDocuments(documents: string[]): KnowledgeReference[] {
    const seen = new Set<string>();
    return documents.reduce<KnowledgeReference[]>((references, document) => {
        const parsed = parseKnowledgeSource(document);
        if (!parsed || seen.has(parsed.path)) return references;
        seen.add(parsed.path);
        references.push({ ...parsed, id: references.length + 1 });
        return references;
    }, []);
}
