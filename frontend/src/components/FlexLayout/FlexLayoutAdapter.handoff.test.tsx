import React from 'react';
import { render, screen } from '@testing-library/react';
import { expect, it, vi } from 'vitest';
import type { HandoffState } from '../TerminalHandoff/TerminalHandoff';
import FlexLayoutAdapter from './FlexLayoutAdapter';

const fixture = vi.hoisted(() => ({ states: {} as Record<string, HandoffState> }));
vi.mock('@opscopilot/shell-terminal/ui', () => ({
    FlexLayoutAdapter: ({ renderTerminal, terminalBadges }: any) => <div><span>{terminalBadges.one}</span>{renderTerminal('one', () => {})}</div>,
    FilesPanel: () => null,
}));
vi.mock('../Terminal/Terminal', () => ({ default: React.forwardRef(() => <textarea aria-label="终端输入" />) }));
vi.mock('../../shell-adapter/wailsTerminalRuntime', () => ({ wailsTerminalRuntime: {} }));
vi.mock('../../shell-adapter/wailsFileTransferHost', () => ({ wailsFileTransferHost: {} }));
vi.mock('../../shell-adapter/wailsTerminalHandoffHost', () => ({ wailsTerminalHandoffHost: {} }));
vi.mock('../TerminalHandoff/TerminalHandoff', async original => ({
    ...await original<typeof import('../TerminalHandoff/TerminalHandoff')>(),
    useTerminalHandoff: () => ({ states: fixture.states, error: '', start: vi.fn(), stop: vi.fn() }),
}));

it('重连快照尚未到达时，不显示旧连接的接手入口、占用标记或遮罩', () => {
    const old: HandoffState = { sessionId: 'one', connectionId: 'old', runId: 'r1', phase: 'running', owned: true, ready: false, message: '' };
    fixture.states = { one: old };
    const props = { terminals: [{ id: 'one', connectionId: 'new', title: '终端', status: 'connected' as any }], onTerminalData: vi.fn(), terminalRefs: { current: new Map() }, onCloseTerminal: vi.fn(), onRenameTerminal: vi.fn() };
    const view = render(<FlexLayoutAdapter {...props} />);
    expect(screen.queryByText('接手测试中')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /接手测试/ })).not.toBeInTheDocument();
    expect(screen.getByLabelText('终端输入').parentElement).not.toHaveAttribute('inert');
    fixture.states = { one: { ...old, connectionId: 'new', runId: '', phase: 'idle', owned: false } };
    view.rerender(<FlexLayoutAdapter {...props} />);
    expect(screen.queryByRole('button', { name: /接手测试/ })).not.toBeInTheDocument();
    fixture.states = { one: { ...old, connectionId: 'new', runId: 'r2' } };
    view.rerender(<FlexLayoutAdapter {...props} />);
    expect(screen.getByText('接手测试中')).toBeInTheDocument();
    expect(screen.getByLabelText('终端输入').parentElement).toHaveAttribute('inert');
    view.unmount();
});

it('闲聊和等待回答不标记页签占用，模型接手后才显示 AI 操作中',()=>{
    const state:HandoffState={sessionId:'one',connectionId:'c1',runId:'',phase:'idle',owned:false,ready:false,message:'',work:{turnId:'a1',source:'assistant',target:{tabId:'one',connectionId:'c1'},phase:'running',message:'思考中'}};
    fixture.states={one:state};
    const props={terminals:[{id:'one',connectionId:'c1',title:'终端',status:'connected' as any}],onTerminalData:vi.fn(),terminalRefs:{current:new Map()},onCloseTerminal:vi.fn(),onRenameTerminal:vi.fn()};
    const view=render(<FlexLayoutAdapter {...props}/>);
    expect(screen.queryByText('AI 处理中')).not.toBeInTheDocument();
    expect(screen.getByLabelText('终端输入').parentElement).not.toHaveAttribute('inert');
    fixture.states={one:{...state,owned:true}};view.rerender(<FlexLayoutAdapter {...props}/>);
    expect(screen.getByText('AI 操作中')).toBeInTheDocument();
    fixture.states={one:state};view.rerender(<FlexLayoutAdapter {...props}/>);
    expect(screen.queryByText('AI 操作中')).not.toBeInTheDocument();
});
