import type { AssistantRuntime } from '../../../frontend-shell/src/assistant/runtime';
import { NewAssistantConversation, RestoreAssistantConversation, StartAssistantTurn, AssistantControlState, StopAssistantTurn, RespondAssistantInteraction } from '../../wailsjs/go/main/App';

export const wailsAssistantRuntime: AssistantRuntime = {
    restoreConversation: RestoreAssistantConversation,
    newConversation: NewAssistantConversation,
    start: (question, target) => StartAssistantTurn(question, target?.tabId || '', target?.connectionId || ''),
    state: AssistantControlState,
    stop: StopAssistantTurn,
    respond: RespondAssistantInteraction,
};
