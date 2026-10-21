import { useEffect, useRef } from 'react';
import { startWorkspaceController, type WorkspaceView } from '../../../../frontend-shell/src/workspace/controller';
import { wailsWorkspaceHost } from '../../shell-adapter/wailsWorkspaceHost';

export function useWorkspaceBridge(view: WorkspaceView,onClient?: (client:string)=>void) {
    const current = useRef(view);
    current.current = view;
    useEffect(() => startWorkspaceController(wailsWorkspaceHost, () => current.current,onClient), [onClient]);
}
