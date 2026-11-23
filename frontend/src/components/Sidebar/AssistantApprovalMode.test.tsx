import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import type { AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import { AssistantApprovalMode } from './AssistantApprovalMode';

it('完全访问须经确认并以红色警示', async () => {
    const runtime: AssistantRuntime = {newConversation:vi.fn(),start:vi.fn(),state:vi.fn(),stop:vi.fn(),respond:vi.fn(),
        approvalMode:vi.fn(async () => 'auto'),selectApprovalMode:vi.fn(async mode => mode)};
    const props = {runtime, busy:false, revision:0, onChanging:vi.fn(), onError:vi.fn()};
    const view = render(<AssistantApprovalMode {...props}/>);
    await waitFor(() => expect(screen.getByRole('button', {name:'权限模式：自动审批'})).toBeEnabled());
    fireEvent.click(screen.getByRole('button', {name:'权限模式：自动审批'}));
    expect(screen.getByText('执行操作时不再请求审批')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('menuitemradio', {name:/完全访问/}));
    expect(screen.getByRole('alertdialog', {name:'启用完全访问？'})).toBeInTheDocument();
    expect(runtime.selectApprovalMode).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {name:'取消'}));
    expect(runtime.selectApprovalMode).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {name:'权限模式：自动审批'}));
    fireEvent.click(screen.getByRole('menuitemradio', {name:/完全访问/}));
    fireEvent.keyDown(document, {key:'Escape'});
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
    expect(runtime.selectApprovalMode).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', {name:'权限模式：自动审批'}));
    fireEvent.click(screen.getByRole('menuitemradio', {name:/完全访问/}));
    fireEvent.click(screen.getByRole('button', {name:'确认启用'}));
    await waitFor(() => expect(screen.getByRole('button', {name:'权限模式：完全访问'})).toHaveClass('is-bypass'));
    expect(runtime.selectApprovalMode).toHaveBeenCalledWith('bypass');
    view.rerender(<AssistantApprovalMode {...props} busy/>);
    expect(screen.getByRole('button', {name:'权限模式：完全访问'})).toBeDisabled();
});

it('自定义 Agent 清楚标明本机审批未验证', async () => {
    const runtime: AssistantRuntime = {newConversation:vi.fn(),start:vi.fn(),state:vi.fn(),stop:vi.fn(),respond:vi.fn(),
        agentKind:vi.fn(async () => 'custom'),approvalMode:vi.fn(async () => 'auto'),selectApprovalMode:vi.fn(async mode => mode)};
    render(<AssistantApprovalMode runtime={runtime} busy={false} revision={0} onChanging={vi.fn()} onError={vi.fn()}/>);
    const button = await screen.findByRole('button', {name:'权限模式：自动审批，本机操作是否受控未验证'});
    fireEvent.click(button);
    expect(screen.getByText('Ops 操作受控；Agent 本机操作是否受控未验证')).toBeInTheDocument();
});
