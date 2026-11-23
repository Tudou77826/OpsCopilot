import type { AssistantMessage, AssistantOperation } from '../../../../frontend-shell/src/assistant/runtime';

// Only display fields saved by the host with the operation. In particular,
// model prose and result output are not evidence of what was executed.
function compact(value?: string): string {
    return (value || '').replace(/\s+/g, ' ').trim();
}

export function operationSummary(content: string, operation: AssistantOperation): string {
    const title = compact(content);
    const location = compact(operation.location);
    const lines = (operation.action || '').split(/\r?\n/).map(compact).filter(Boolean);
    // General application receipts repeat their title as the first action line.
    if (lines[0] === title) lines.shift();
    const action = lines.join(' ');
    return [title, location !== 'OpsCopilot' && location, action !== title && action]
        .filter(Boolean).join(' · ');
}

export function latestOperationSummary(messages: AssistantMessage[]): string {
    const active = [...messages].reverse().find(message => message.operation && !['completed', 'done'].includes(message.operation.status));
    const meaningful = [...messages].reverse().find(message => {
        const action = compact(message.operation?.action);
        return action && action !== compact(message.content);
    });
    const selected = active || meaningful || messages[messages.length - 1];
    return selected?.operation ? operationSummary(selected.content, selected.operation) : '';
}
