import type { AssistantRuntime } from '../../../frontend-shell/src/assistant/runtime';
import { AssistantContextUsage, AssistantModelOptions, SelectAssistantModel, NewAssistantConversation, RestoreAssistantConversationOnPage, StartAssistantTurnOnPage, AssistantControlState, StopAssistantTurn, RespondAssistantInteraction } from '../../wailsjs/go/main/App';
import { currentWorkspaceIdentity, currentWorkspaceSnapshot } from './workspaceIdentity';
import { pullWorkspace } from './wailsWorkspaceHost';
import { RevealAssistantResult } from '../../wailsjs/go/main/App';

export const wailsAssistantRuntime: AssistantRuntime = {
	contextUsage: AssistantContextUsage,
    revealResult: async (id) => RevealAssistantResult(await currentWorkspaceIdentity(), id),
    modelOptions: AssistantModelOptions,
    selectModel: SelectAssistantModel,
    restoreConversation: async () => {
        const snapshot = await RestoreAssistantConversationOnPage(await currentWorkspaceIdentity());
        return {...snapshot, messages: snapshot.messages.map(message => {
            if (message.role !== 'user' && message.role !== 'ai') throw Error('助手历史消息角色无效');
            return {...message, role: message.role};
        })};
    },
    newConversation: NewAssistantConversation,
    start: async (question, target) => {
        const view = currentWorkspaceSnapshot();
        const client = await currentWorkspaceIdentity();
        await pullWorkspace(client, view);
        return StartAssistantTurnOnPage(question, target?.tabId || '', target?.connectionId || '', client);
    },
    state: AssistantControlState,
    stop: StopAssistantTurn,
    respond: RespondAssistantInteraction,
};
