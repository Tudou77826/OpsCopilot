import React from 'react';
import { ScriptRecordingPanel as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsScriptRuntime } from '../../shell-adapter/wailsScriptRuntime';

type Props = Omit<React.ComponentProps<typeof Shared>, 'runtime'>;
export default function ScriptRecordingPanel(props: Props) {
    return <Shared {...props} runtime={wailsScriptRuntime} />;
}
