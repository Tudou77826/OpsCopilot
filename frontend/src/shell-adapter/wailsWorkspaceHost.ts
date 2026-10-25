import { WorkspaceRegisterPage, WorkspaceDetach, WorkspacePull, WorkspaceAcknowledge } from '../../wailsjs/go/main/App';
import type { WorkspaceHost, WorkspaceSnapshot, WorkspacePresentation } from '../../../frontend-shell/src/workspace/controller';
import { workspace } from '../../wailsjs/go/models';
import { workspacePageKey, registerWorkspaceIdentity, markWorkspaceReady, clearWorkspaceIdentity } from './workspaceIdentity';

export const wailsWorkspaceHost: WorkspaceHost = {
    attach: async () => { const client = await WorkspaceRegisterPage(workspacePageKey); registerWorkspaceIdentity(client); return client; },
    detach: async client => { clearWorkspaceIdentity(client); await WorkspaceDetach(client); },
    pull: pullWorkspace,
    acknowledge: WorkspaceAcknowledge,
};

export async function pullWorkspace(client: string, view: WorkspaceSnapshot): Promise<WorkspacePresentation[]> {
    const commands = await WorkspacePull(client, new workspace.Snapshot(view));
    markWorkspaceReady(client);
    return commands.map(command => {
        if (command.action !== 'open' && command.action !== 'select') throw Error('未知工作台呈现操作');
        return {...command, action: command.action};
    });
}
