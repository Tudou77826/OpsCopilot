import React from 'react';
import { FlexLayoutAdapter as Shared, FilesPanel, type FlexLayoutAdapterProps } from '@opscopilot/shell-terminal/ui';
import { wailsTerminalRuntime } from '../../shell-adapter/wailsTerminalRuntime';
import { wailsFileTransferHost } from '../../shell-adapter/wailsFileTransferHost';
export default function FlexLayoutAdapter(props: Omit<FlexLayoutAdapterProps, 'terminalRuntime'>) {
    return <Shared {...props} terminalRuntime={wailsTerminalRuntime} renderFileTransfer={(activeTerminalId, terminals) => <FilesPanel activeTerminalId={activeTerminalId} terminals={terminals} host={wailsFileTransferHost} />} />;
}
