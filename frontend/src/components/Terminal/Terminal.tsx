import React, { forwardRef } from 'react';
import { TerminalComponent, type TerminalProps, type TerminalRef } from '@opscopilot/shell-terminal/ui';
import { wailsTerminalRuntime } from '../../shell-adapter/wailsTerminalRuntime';
export type { TerminalProps, TerminalRef };
export default forwardRef<TerminalRef, TerminalProps>((props, ref) => <TerminalComponent {...props} ref={ref} runtime={props.runtime ?? wailsTerminalRuntime} />);
