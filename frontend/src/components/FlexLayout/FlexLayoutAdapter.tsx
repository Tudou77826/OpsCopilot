import React from 'react';
import { FlexLayoutAdapter as Shared, FilesPanel, type FlexLayoutAdapterProps } from '@opscopilot/shell-terminal/ui';
import { wailsTerminalRuntime } from '../../shell-adapter/wailsTerminalRuntime';
import { wailsFileTransferHost } from '../../shell-adapter/wailsFileTransferHost';
import Terminal from '../Terminal/Terminal';
import { TerminalHandoff, useTerminalHandoff } from '../TerminalHandoff/TerminalHandoff';
import { wailsTerminalHandoffHost } from '../../shell-adapter/wailsTerminalHandoffHost';
import { isTerminalStatusReply } from '../../../../frontend-shell/src/ui/Terminal/statusReplies';
export default function FlexLayoutAdapter(props: Omit<FlexLayoutAdapterProps, 'terminalRuntime'>) {
    const handoff = useTerminalHandoff(wailsTerminalHandoffHost);
    const currentStates = Object.fromEntries(props.terminals.map(terminal => {
        const state = handoff.states[terminal.id];
        return [terminal.id, !terminal.connectionId || state?.connectionId === terminal.connectionId ? state : undefined] as const;
    }));
    const badges = Object.fromEntries(Object.values(currentStates).flatMap(state => state?.owned ? [[state.sessionId, state.work?.source === 'assistant' ? 'AI 操作中' : '接手测试中']] : state?.work?.source === 'assistant' ? [] : state?.work?.phase === 'running' ? [[state.sessionId, '授权体验中']] : state?.work?.phase === 'waiting' ? [[state.sessionId, '等待确认']] : []));
    return <Shared {...props} terminalRuntime={wailsTerminalRuntime} terminalBadges={{...props.terminalBadges,...badges}}
        renderTerminal={(id, attachRef) => <TerminalHandoff state={currentStates[id]} connected={props.terminals.find(terminal => terminal.id === id)?.status === 'connected'} transportError={currentStates[id] ? handoff.error : ''}
            onStart={(scenario, confirmed) => handoff.start(id, scenario, confirmed)} onStop={handoff.stop} onRespond={handoff.respond} onFocus={() => props.terminalRefs.current.get(id)?.focus()}>
            <Terminal id={id} sessionID={id} ref={attachRef} onData={data => { if (!currentStates[id]?.owned || isTerminalStatusReply(data)) props.onTerminalData(id, data); }} completionDelay={props.completionDelay} terminalConfig={props.terminalConfig} onFontSizeChange={props.onTerminalFontSizeChange} highlightRules={props.highlightRules} theme={props.theme} onSelectionParsed={props.onSelectionParsed} />
        </TerminalHandoff>}
        renderFileTransfer={(activeTerminalId, terminals) => <FilesPanel activeTerminalId={activeTerminalId} terminals={terminals} host={wailsFileTransferHost} />} />;
}
