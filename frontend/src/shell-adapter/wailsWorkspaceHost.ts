import { WorkspaceAttach, WorkspaceDetach, WorkspacePull, WorkspaceAcknowledge } from '../../wailsjs/go/main/App';
import type { WorkspaceHost } from '../../../frontend-shell/src/workspace/controller';

export const wailsWorkspaceHost: WorkspaceHost = {
    attach: WorkspaceAttach, detach: WorkspaceDetach, pull: WorkspacePull, acknowledge: WorkspaceAcknowledge,
};
