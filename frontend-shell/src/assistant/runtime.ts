/** Conversation is independent of resource availability. Tool targets are optional. */
export interface AssistantTarget { tabId: string; connectionId: string; }
// Transport values mirror the generated Go bindings; the host validates kinds.
export interface AssistantPageReference extends AssistantTarget { kind: string; title: string; }
export interface AssistantPageOption extends AssistantPageReference { kind: 'terminal' | 'database' | 'file'; unavailable?: string; description?: string; }
export interface AssistantAnswer { text: string; selected: string[]; }
export interface AssistantPrompt {
  data?: unknown;
  id: string; kind: string; title: string; description: string; command?: string;
  details?: {label: string; value: string}[]; choices?: {id: string; label: string}[];
  multiple: boolean; expires: string;
}
export interface AssistantTurnState {
  turnId: string; target: AssistantTarget; phase: string; message: string;
  prompt?: AssistantPrompt; output?: string; source?: string;
  activity?: string;
  messages?: AssistantMessage[];
  saveWarning?: string;
}
export interface AssistantMessage {
  id: string; turnId?: string; role: string; content: string; phase?: string;
  kind?: string; time?: string;
  operation?: AssistantOperation;
  references?: AssistantPageReference[];
}
export interface AssistantOperation {
	measured?: boolean; durationMs?: number;
  kind: string; workspaceId: string; itemId?: string; connectionId?: string;
  status: string; stage?: string; outcome?: string; next?: string; detail?: string;
  action?: string; result?: string; location?: string;
}
export interface AssistantConversationSnapshot {
  id: string; messages: AssistantMessage[]; truncated: boolean;
  recoveryNote?: string;
}
export interface AssistantModelOption { id: string; name: string; modelId: string; provider: string; }
export interface AssistantModels { models: AssistantModelOption[]; current?: AssistantModelOption; detail?: string; }
export interface AssistantContextUsage { known: boolean; used: number; size: number; }
export interface AssistantRuntime {
	contextUsage?(): Promise<AssistantContextUsage>;
  revealResult?(messageId: string): Promise<void>;
  modelOptions?(): Promise<AssistantModels>;
  selectModel?(id: string): Promise<AssistantModels>;
  restoreConversation?(): Promise<AssistantConversationSnapshot>;
  newConversation(): Promise<void>;
  start(question: string, target?: AssistantTarget, references?: AssistantPageReference[]): Promise<AssistantTurnState>;
  state(): Promise<AssistantTurnState>;
  stop(turnId: string): Promise<void>;
  respond(turnId: string, requestId: string, answer: AssistantAnswer): Promise<void>;
}
