import type { AssistantRuntime } from '../../../frontend-shell/src/assistant/runtime';
import { EventsOn } from '../../wailsjs/runtime/runtime';

export const wailsAssistantRuntime: AssistantRuntime = {
  async ask(question) {
    const backend = window.go?.main?.App;
    if (!backend?.AskCompanionAI) throw new Error('AI 服务未连接');
    return String(await backend.AskCompanionAI(question) ?? '');
  },
  subscribeStatus(listener) {
    return EventsOn('agent:status', (event: unknown) => {
      if (!event || typeof event !== 'object') return;
      const data = event as Record<string, unknown>;
      if (typeof data.stage === 'string' && typeof data.message === 'string') {
        listener({ stage: data.stage, message: data.message, runId: typeof data.runId === 'string' ? data.runId : undefined });
      }
    });
  },
};
