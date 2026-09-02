import React from 'react';
import { ScriptEditorModal as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsScriptRuntime } from '../../shell-adapter/wailsScriptRuntime';

type Props = Omit<React.ComponentProps<typeof Shared>, 'runtime'>;
export default function ScriptEditorModal(props: Props) {
    return <Shared {...props} runtime={wailsScriptRuntime} />;
}
