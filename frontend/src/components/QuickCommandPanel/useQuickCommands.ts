import { useMemo } from 'react';
import { useQuickCommands as useShared, type QuickCommandStorageAdapter } from '@opscopilot/shell-terminal/ui';
import { makeWailsQuickCommandHost } from '../../shell-adapter/wailsQuickCommandHost';
export { MemoryAdapter } from '@opscopilot/shell-terminal/ui';
export type { UseQuickCommandsReturn } from '../../../../frontend-shell/src/ui/quickcmd/useQuickCommands';
export function useQuickCommands(options?: { adapter?: QuickCommandStorageAdapter }) {
    const host = useMemo(() => {
        const value = makeWailsQuickCommandHost(() => {});
        if (options?.adapter) value.storage = options.adapter;
        return value;
    }, [options?.adapter]);
    return useShared({ host });
}
