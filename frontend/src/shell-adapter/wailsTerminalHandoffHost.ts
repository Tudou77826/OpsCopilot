import type { HandoffHost } from '../components/TerminalHandoff/TerminalHandoff';
import { TerminalHandoffStates, StartTerminalHandoffTest, StopTerminalHandoffTest, StartTerminalApprovalTrial, AssistantControlState, RespondAssistantInteraction, StopAssistantTurn } from '../../wailsjs/go/main/App';
async function states() {
    const [terminals, work] = await Promise.all([TerminalHandoffStates(), AssistantControlState()]);
    return terminals.map(terminal => ({ ...terminal, work: work.target?.tabId === terminal.sessionId && work.target.connectionId === terminal.connectionId ? work : undefined }));
}
export const wailsTerminalHandoffHost: HandoffHost = {
    states,
    start: async (id, scenario, confirmed, connectionId) => {
        if (scenario !== 'approval') return StartTerminalHandoffTest(id, scenario, confirmed, connectionId);
        const started = await StartTerminalApprovalTrial(id, connectionId, confirmed);
        try {
            const current = (await states()).find(value => value.sessionId === id && value.connectionId === connectionId);
            if (!current) throw Error('终端连接已改变');
            return { ...current, work: current.work?.turnId === started.turnId ? current.work : started };
        } catch (error) {
            await StopAssistantTurn(started.turnId).catch(() => undefined);
            throw error;
        }
    },
    stop: (id, runId) => StopTerminalHandoffTest(id, runId),
    stopWork: StopAssistantTurn,
    respond: RespondAssistantInteraction,
};
