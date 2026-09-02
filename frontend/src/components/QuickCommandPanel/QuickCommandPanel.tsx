import React, { useMemo } from 'react';
import { QuickCommandPanel as Shared } from '@opscopilot/shell-terminal/ui';
import { makeWailsQuickCommandHost } from '../../shell-adapter/wailsQuickCommandHost';
type Props = Omit<React.ComponentProps<typeof Shared>, 'host'>;
export default function QuickCommandPanel(props: Props) {
    const host = useMemo(() => makeWailsQuickCommandHost(props.onExecute), [props.onExecute]);
    return <Shared {...props} host={host} />;
}
