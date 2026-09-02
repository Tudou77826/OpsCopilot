import React, { useMemo } from 'react';
import { FilesPanel as Shared, type FileTransferHost } from '@opscopilot/shell-terminal/ui';
import { wailsFileTransferHost } from '../../shell-adapter/wailsFileTransferHost';

type Props = Omit<React.ComponentProps<typeof Shared>, 'host'> & { backend?: Partial<FileTransferHost> };
export default function FilesPanel(props: Props) {
    const host = useMemo(() => ({ ...wailsFileTransferHost, ...props.backend }), [props.backend]);
    return <Shared {...props} host={host} />;
}
export { getFileTransferLayoutMode, getStableFileTransferLayoutMode } from '../../../../frontend-shell/src/ui/filetransfer/FilesPanel';
