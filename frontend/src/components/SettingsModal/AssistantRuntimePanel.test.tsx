import { act, fireEvent, render, screen } from '@testing-library/react';
import { describe, expect, it, vi } from 'vitest';
import AssistantRuntimePanel, { type RuntimeHost, type RuntimeReport } from './AssistantRuntimePanel';

vi.mock('../../../wailsjs/go/main/App', () => ({ CheckAssistantRuntime: vi.fn(), CheckAssistantAgentRuntime: vi.fn(), CheckCustomAssistantRuntime: vi.fn(), SelectAssistantRuntimeExecutable: vi.fn(), InspectAssistantModel: vi.fn() }));

const ready: RuntimeReport = { ready: true, executable: 'C:\\Tools\\icode.exe', name: 'iCode', version: '0.30.1', protocolVersion: 1, extensionApiVersion: 1,
    checks: [{ id: 'handshake', label: 'ACP 握手', status: 'passed' }] };

describe('iCode runtime settings', () => {
    it('checks a custom ACP executable and separate arguments without saving', async () => {
        const result: RuntimeReport = {ready:true,executable:'C:\\Tools\\agent.exe',name:'Agent',checks:[],advice:'工具调用尚未验证'};
        const runtime: RuntimeHost = {check:vi.fn(),checkCustom:vi.fn().mockResolvedValue(result),select:vi.fn()};
        render(<AssistantRuntimePanel value={result.executable} kind="custom" arguments={['acp','--stdio']} onChange={vi.fn()} onKindChange={vi.fn()} runtime={runtime}/>);
        expect(screen.getByLabelText('Agent 启动参数')).toHaveValue('acp\n--stdio');
        expect(screen.getByText(/快速检测不能证明/)).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button',{name:'检测环境'}));
        expect(await screen.findByText('工具调用尚未验证')).toBeInTheDocument();
        expect(runtime.checkCustom).toHaveBeenCalledWith(result.executable,['acp','--stdio']);
    });
    it('switches between independently named custom Agent configurations', () => {
        const change = vi.fn();
        const agents = [{id:'one',name:'Agent A',executable:'C:\\a.exe',arguments:['acp']}, {id:'two',name:'Agent B',executable:'C:\\b.exe',arguments:['serve']}];
        render(<AssistantRuntimePanel value="C:\\a.exe" kind="custom" arguments={['acp']} onChange={vi.fn()} onKindChange={vi.fn()} customAgents={agents} customAgentId="one" onCustomAgentsChange={change} runtime={{check:vi.fn(),select:vi.fn()}}/>);
        fireEvent.change(screen.getByLabelText('当前 Agent'), {target:{value:'custom:two'}});
        expect(change).toHaveBeenCalledWith(agents,'two');
    });
    it('keeps the profile actions available by accessible icon labels', () => {
        const change = vi.fn();
        const agents = [{id:'one',name:'Agent A',executable:'C:\\a.exe',arguments:['acp']}];
        render(<AssistantRuntimePanel value="C:\\a.exe" kind="custom" arguments={['acp']} onChange={vi.fn()} onKindChange={vi.fn()} customAgents={agents} customAgentId="one" onCustomAgentsChange={change} runtime={{check:vi.fn(),select:vi.fn()}}/>);
        expect(screen.getByRole('button', {name:'添加 Agent'})).toBeEnabled();
        fireEvent.click(screen.getByRole('button', {name:'移除 Agent'}));
        expect(change).toHaveBeenCalledWith([], '');
    });
    it('offers presets and saved agents in one selector and preserves a legacy draft when adding', () => {
        const change = vi.fn();
        const agents = [{id:'one',name:'内部 Agent',executable:'C:\\a.exe',arguments:['acp']}];
        render(<AssistantRuntimePanel value={'C:\\draft.exe'} kind="custom" arguments={['serve']} onChange={vi.fn()} onKindChange={vi.fn()} customAgents={agents} onCustomAgentsChange={change} runtime={{check:vi.fn(),select:vi.fn()}}/>);
        expect(screen.getAllByRole('combobox')).toHaveLength(1);
        expect(screen.getByRole('option', {name:'iCode'})).toBeInTheDocument();
        expect(screen.getByRole('option', {name:'内部 Agent'})).toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name:'添加 Agent'}));
        expect(change).toHaveBeenCalledWith([...agents, expect.objectContaining({name:'新 Agent',executable:'C:\\draft.exe',arguments:['serve']})], expect.any(String));
    });
    it('checks the selected OpenCode draft without changing the saved path', async () => {
        const result: RuntimeReport = {ready:true,executable:'C:\\Tools\\opencode.exe',name:'OpenCode',version:'1.19.5',protocolVersion:1,checks:[]};
        const runtime: RuntimeHost = {check:vi.fn(),checkAgent:vi.fn().mockResolvedValue(result),select:vi.fn()};
        const changeKind = vi.fn();
        render(<AssistantRuntimePanel value={result.executable} kind="opencode" onChange={vi.fn()} onKindChange={changeKind} runtime={runtime}/>);
        expect(screen.getByLabelText('当前 Agent')).toHaveValue('opencode');
        fireEvent.click(screen.getByRole('button',{name:'检测环境'}));
        expect(await screen.findByText('运行环境可用')).toBeInTheDocument();
        expect(runtime.checkAgent).toHaveBeenCalledWith('opencode',result.executable);
        expect(runtime.check).not.toHaveBeenCalled();
    });
    it('makes the model source explicit and checks metadata without exposing credentials', async () => {

        const runtime: RuntimeHost = { check: vi.fn().mockResolvedValue(ready), select: vi.fn(), model: vi.fn().mockResolvedValue({ready:true,name:'Intranet',detail:'已读取默认模型'}) };
        render(<AssistantRuntimePanel value="" onChange={vi.fn()} runtime={runtime} />);
        expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
        fireEvent.click(screen.getByRole('button', {name:'检测环境'}));
        expect(await screen.findByText('模型配置已读取 · Intranet')).toBeInTheDocument();
        expect(runtime.model).toHaveBeenCalledWith('icode');

    });
    it('checks the draft path and shows readiness without saving', async () => {
        let resolve!: (report: RuntimeReport) => void;
        const runtime: RuntimeHost = { check: vi.fn(() => new Promise<RuntimeReport>(done => { resolve = done; })), select: vi.fn() };
        const change = vi.fn();
        render(<AssistantRuntimePanel value={ready.executable} onChange={change} runtime={runtime} />);
        fireEvent.click(screen.getByRole('button', { name: '检测环境' }));
        expect(runtime.check).toHaveBeenCalledWith('C:\\Tools\\icode.exe');
        expect(screen.getByLabelText('程序路径')).toBeDisabled();
        expect(screen.getByText(/最多等待约 45 秒/)).toBeInTheDocument();
        await act(async () => resolve(ready));
        expect(screen.getByText('运行环境可用')).toBeInTheDocument();
        expect(screen.getByText('iCode 0.30.1 · ACP 1 · 扩展 1')).toBeInTheDocument();
        expect(change).not.toHaveBeenCalled();
    });

    it('shows the failing stage, remediation and skipped checks', async () => {
        const report: RuntimeReport = { ready: false, executable: 'C:\\Old\\icode.exe', version: '0.26.0', checks: [
            { id: 'handshake', label: 'ACP 握手', status: 'passed' },
            { id: 'capabilities', label: '接口兼容性', status: 'failed', detail: '缺少 settings/reload 接口' },
            { id: 'session', label: '独立会话初始化', status: 'skipped' },
        ], advice: '请更新所选 iCode' };
        const runtime: RuntimeHost = { check: vi.fn().mockResolvedValue(report), select: vi.fn() };
        render(<AssistantRuntimePanel value="" onChange={vi.fn()} runtime={runtime} />);
        fireEvent.click(screen.getByRole('button', { name: '检测环境' }));
        expect(await screen.findByText('运行环境未就绪')).toBeInTheDocument();
        expect(screen.getByText('缺少 settings/reload 接口')).toBeInTheDocument();
        expect(screen.getByText('未检查')).toBeInTheDocument();
        expect(screen.getByText('请更新所选 iCode')).toBeInTheDocument();
        expect(runtime.check).toHaveBeenCalledWith('');
    });

    it('ignores stale checks when the edited path changes', async () => {
        let resolve!: (report: RuntimeReport) => void;
        const runtime: RuntimeHost = { check: vi.fn(() => new Promise<RuntimeReport>(done => { resolve = done; })), select: vi.fn() };
        const { rerender } = render(<AssistantRuntimePanel value="C:\\one.exe" onChange={vi.fn()} runtime={runtime} />);
        fireEvent.click(screen.getByRole('button', { name: '检测环境' }));
        rerender(<AssistantRuntimePanel value="C:\\two.exe" onChange={vi.fn()} runtime={runtime} />);
        await act(async () => resolve(ready));
        expect(screen.queryByText('运行环境可用')).not.toBeInTheDocument();
        expect(screen.getByText('未检测')).toBeInTheDocument();
    });

    it('keeps the path on picker cancellation and provides automatic discovery', async () => {
        const runtime: RuntimeHost = { check: vi.fn(), select: vi.fn().mockResolvedValue('') };
        const change = vi.fn();
        render(<AssistantRuntimePanel value="C:\\one.exe" onChange={change} runtime={runtime} />);
        await act(async () => fireEvent.click(screen.getByRole('button', { name: '选择 iCode 程序' })));
        expect(change).not.toHaveBeenCalled();
        fireEvent.click(screen.getByRole('button', { name: '使用自动查找' }));
        expect(change).toHaveBeenCalledWith('');
    });
});
