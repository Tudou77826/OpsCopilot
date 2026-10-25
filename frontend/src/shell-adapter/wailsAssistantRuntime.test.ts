import { expect, it, vi } from 'vitest';
import { wailsAssistantRuntime } from './wailsAssistantRuntime';
import { wailsWorkspaceHost } from './wailsWorkspaceHost';
import { provideWorkspaceSnapshot } from './workspaceIdentity';

const api = vi.hoisted(() => ({
    AssistantContextUsage: vi.fn(async () => ({known:false,used:0,size:0})),
    AssistantModelOptions: vi.fn(), SelectAssistantModel: vi.fn(),
    RevealAssistantResult: vi.fn(async () => {}),
    WorkspaceRegisterPage: vi.fn(async () => 'sender-document'),
    WorkspacePull: vi.fn(async () => []), WorkspaceDetach: vi.fn(async () => {}), WorkspaceAcknowledge: vi.fn(),
    StartAssistantTurnOnPage: vi.fn(async () => ({turnId:'turn',phase:'running'})),
    RestoreAssistantConversationOnPage: vi.fn(async () => ({id:'conversation',messages:[],truncated:false})),
    NewAssistantConversation: vi.fn(), AssistantControlState: vi.fn(), StopAssistantTurn: vi.fn(), RespondAssistantInteraction: vi.fn(),
}));
vi.mock('../../wailsjs/go/main/App', () => api);

it('publishes the sender snapshot and includes its document identity in every submission', async () => {
    const view = {tabs:[{tabId:'selected',connectionId:'actual',title:'remote',status:'connected',ready:true}],active:{tabId:'selected',connectionId:'actual'},activeWorkspace:''};
    const dispose = provideWorkspaceSnapshot(() => view);
    const client = await wailsWorkspaceHost.attach();
    await wailsWorkspaceHost.pull(client, view);
    api.WorkspacePull.mockClear();
    await wailsAssistantRuntime.start('这个机器的 TMP 有什么？',view.active);
    expect(api.WorkspacePull).toHaveBeenCalledWith(client,view);
    expect(api.StartAssistantTurnOnPage).toHaveBeenCalledWith('这个机器的 TMP 有什么？','selected','actual',client);
    expect(api.WorkspacePull.mock.invocationCallOrder[0]).toBeLessThan(api.StartAssistantTurnOnPage.mock.invocationCallOrder[0]);
    await wailsAssistantRuntime.restoreConversation?.();
    expect(api.RestoreAssistantConversationOnPage).toHaveBeenCalledWith(client);
    await wailsAssistantRuntime.revealResult?.('record-id');
    expect(api.RevealAssistantResult).toHaveBeenCalledExactlyOnceWith(client,'record-id');
    dispose(); await wailsWorkspaceHost.detach(client);
});
