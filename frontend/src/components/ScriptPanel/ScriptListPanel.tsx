import React, { forwardRef } from 'react';
import { ScriptListPanel as Shared } from '@opscopilot/shell-terminal/ui';
import { wailsScriptRuntime } from '../../shell-adapter/wailsScriptRuntime';
type Props = Omit<React.ComponentPropsWithoutRef<typeof Shared>, 'runtime'>;
export default forwardRef<{ loadScripts(): void }, Props>((props, ref) => <Shared {...props} ref={ref} runtime={wailsScriptRuntime} />);
