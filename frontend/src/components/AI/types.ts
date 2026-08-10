export interface AgentTraceEvent {
    runId?: string;
    stage: string;
    message: string;
    ts: number;
}

export interface DiagnosticStep {
    step?: number;
    title?: string;
    description: string;
}

export interface CommandSuggestion {
    command: string;
    description?: string;
    risk?: string;
    source?: string;
}

export interface KnowledgeReference {
    id: number;
    label: string;
    path: string;
    line?: number;
    lineEnd?: number;
}

export interface KnowledgeResponse {
    summary?: string;
    steps: DiagnosticStep[];
    commands: CommandSuggestion[];
    references: KnowledgeReference[];
}

export interface KnowledgeTarget {
    path: string;
    line?: number;
    requestId: number;
}

export interface AIContextChip {
    id: string;
    label: string;
    active?: boolean;
}
