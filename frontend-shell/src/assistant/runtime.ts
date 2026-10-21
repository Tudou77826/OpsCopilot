/** Conversation is independent of resource availability. Tool targets are optional. */
export interface AssistantTarget { tabId: string; connectionId: string; }
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
}
export interface AssistantMessage {
  id: string; turnId?: string; role: 'user' | 'ai'; content: string; phase?: string;
}
export interface AssistantConversationSnapshot {
  id: string; messages: AssistantMessage[]; truncated: boolean;
}
export interface AssistantRuntime {
  restoreConversation?(): Promise<AssistantConversationSnapshot>;
  newConversation(): Promise<void>;
  start(question: string, target?: AssistantTarget): Promise<AssistantTurnState>;
  state(): Promise<AssistantTurnState>;
  stop(turnId: string): Promise<void>;
  respond(turnId: string, requestId: string, answer: AssistantAnswer): Promise<void>;
}
