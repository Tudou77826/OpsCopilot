import React from 'react';
import { SessionManager as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsSessionRuntime } from '../../shell-adapter/wailsSessionRuntime';
import { wailsSessionSharedRuntime } from '../../shell-adapter/wailsSessionSharedRuntime';
export default function SessionManager(props: Omit<React.ComponentProps<typeof Shared>, 'runtime' | 'sharedRuntime'>) {
    return <Shared {...props} runtime={wailsSessionRuntime} sharedRuntime={wailsSessionSharedRuntime} />;
}
