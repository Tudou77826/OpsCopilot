import React from 'react';
import { EditSavedSessionModal as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsSessionRuntime } from '../../shell-adapter/wailsSessionRuntime';

type Props = Omit<React.ComponentProps<typeof Shared>, 'runtime'>;
export default function EditSavedSessionModal(props: Props) {
    return <Shared {...props} runtime={wailsSessionRuntime} />;
}
