import React from 'react';
import { SharedSessionPanel as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsSessionSharedRuntime } from '../../shell-adapter/wailsSessionSharedRuntime';

type Props = Omit<React.ComponentProps<typeof Shared>, 'runtime'>;
export default function SharedSessionPanel(props: Props) {
    return <Shared {...props} runtime={wailsSessionSharedRuntime} />;
}
