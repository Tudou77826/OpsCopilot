/** Assistant presentation depends on this port, independently of pet storage or transport. */
export interface AssistantStatus {
  stage: string;
  message: string;
  runId?: string;
}
export interface AssistantRuntime {
  ask(question: string): Promise<string>;
  subscribeStatus?(listener: (event: AssistantStatus) => void): () => void;
}
