import { useEffect, useRef } from 'react';
import { startWorkspaceController, type WorkspaceView } from '../../../../frontend-shell/src/workspace/controller';
import { wailsWorkspaceHost } from '../../shell-adapter/wailsWorkspaceHost';
import { provideWorkspaceSnapshot } from '../../shell-adapter/workspaceIdentity';

export function useWorkspaceBridge(view: WorkspaceView,onClient?: (client:string)=>void) {
    const current = useRef(view);
    current.current = view;
    useEffect(() => provideWorkspaceSnapshot(() => current.current.snapshot()), []);
    useEffect(() => startWorkspaceController(wailsWorkspaceHost, () => current.current,onClient), [onClient]);
}
