import { useEffect, useRef, useState } from 'react';
import { TbCheck, TbCircleX, TbCopy, TbFolderOpen, TbMinus, TbPlus, TbRefresh, TbRotate2, TbTrash } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { btnSecondary, cardDivider, cardTitle, settingsCard } from './settingsStyles';
import { CheckAssistantRuntime, CheckAssistantAgentRuntime, CheckCustomAssistantRuntime, InspectAssistantModel, SelectAssistantRuntimeExecutable } from '../../../wailsjs/go/main/App';
import './assistantRuntime.css';

export interface RuntimeReport {
    ready: boolean;
    executable: string;
    name?: string;
    version?: string;
    protocolVersion?: number;
    extensionApiVersion?: number;
    capabilities?: { loadSession: boolean; additionalRoots: boolean; httpMcp: boolean; modelOptions: number; modeOptions: number };
    checks: { id: string; label: string; status: string; detail?: string }[];
    advice?: string;
}

export interface RuntimeHost {
    check(path: string): Promise<RuntimeReport>;
    checkAgent?(kind: string, path: string): Promise<RuntimeReport>;
    checkCustom?(path: string, args: string[]): Promise<RuntimeReport>;
    select(): Promise<string>;
    model?(source: string): Promise<{ ready: boolean; name?: string; detail: string }>;
}
type CustomAgent = { id:string; name:string; executable:string; arguments:string[] };
const host: RuntimeHost = { check: CheckAssistantRuntime, checkAgent: CheckAssistantAgentRuntime, checkCustom: CheckCustomAssistantRuntime, select: SelectAssistantRuntimeExecutable, model: InspectAssistantModel };

// 用户打开这个页面只想得到一个答案：当前 Agent 能不能用。
// 这是单一职责的紧凑状态卡：全部行都是单行布局（标签、控件、说明同一行），
// 打开页面即自动检测，结论一行给出；路径/参数/环境变量折叠进「手动配置」，
// 检测失败时自动展开露出解决入口。
export default function AssistantRuntimePanel({ value, onChange, kind = 'codeagent', onKindChange, arguments: args = [], onArgumentsChange, agentEnv = {}, onAgentEnvChange, customAgents = [], customAgentId = '', onCustomAgentsChange, disabled = false, runtime = host }: {
    value: string; onChange: (value: string) => void; kind?: string; onKindChange?: (kind:string)=>void; arguments?: string[]; onArgumentsChange?: (args:string[])=>void; agentEnv?: Record<string,string>; onAgentEnvChange?: (env: Record<string,string>)=>void; customAgents?:CustomAgent[]; customAgentId?:string; onCustomAgentsChange?: (agents:CustomAgent[], id:string)=>void; disabled?: boolean; runtime?: RuntimeHost;
}) {
    const product = kind === 'opencode' ? 'OpenCode' : kind === 'custom' ? '自定义 ACP Agent' : kind === 'codeagent' ? 'CodeAgent' : 'iCode';
    // 模型/认证来源随底座不同：跟在「选哪个 Agent」决策旁边，一句话说完。
    const sourceNote = kind === 'opencode' ? '模型与认证使用 OpenCode 本机配置（opencode auth login）。'
        : kind === 'custom' ? '模型和认证由所选 Agent 本机配置管理。'
        : kind === 'codeagent' ? '模型与认证用其本机配置；环境变量可接入「模型服务」。'
        : '模型与认证由 iCode 运行组件托管。';
    const [report, setReport] = useState<RuntimeReport | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [copied, setCopied] = useState(false);
    const [modelReport, setModelReport] = useState<{ ready: boolean; name?: string; detail: string } | null>(null);
    const generation = useRef(0);
    const hasAdvanced = args.some(arg => arg.trim()) || Object.keys(agentEnv).length > 0;
    const [advancedOpen, setAdvancedOpen] = useState(kind === 'custom' || hasAdvanced);
    useEffect(() => {
        generation.current++;
        setReport(null); setModelReport(null); setError(''); setBusy(false); setCopied(false);
        return () => { generation.current++; };
    }, [value, kind, args.join('\n')]);
    useEffect(() => { if (hasAdvanced) setAdvancedOpen(true); }, [hasAdvanced]);
    // 检测失败多半是程序没找到：主动展开手动配置，把解决入口递到用户面前。
    useEffect(() => { if (report && !report.ready) setAdvancedOpen(true); }, [report]);

    async function check() {
        const id = ++generation.current;
        setBusy(true); setError(''); setReport(null); setCopied(false);
        try {
            const [result, model] = await Promise.all([kind === 'icode' ? runtime.check(value.trim()) : kind === 'custom' ? runtime.checkCustom?.(value.trim(), args) ?? Promise.reject() : runtime.checkAgent?.(kind, value.trim()) ?? Promise.reject(), kind === 'icode' ? runtime.model?.('icode') : undefined]);
            if (id === generation.current) { setReport(result); setModelReport(model || null); }
        } catch {
            if (id === generation.current) setError('环境检测未能完成，请重试。');
        } finally { if (id === generation.current) setBusy(false); }
    }
    // 打开页面、或切换 Agent 时自动检测：用户不用学「先点检测」这一步。
    const checkRef = useRef(check);
    checkRef.current = check;
    useEffect(() => { void checkRef.current(); }, [kind]);

    function envText(): string {
        return Object.entries(agentEnv).sort(([a], [b]) => a.localeCompare(b)).map(([key, val]) => `${key}=${val}`).join('\n');
    }
    function applyEnv(text: string) {
        const next: Record<string,string> = {};
        for (const line of text.split('\n')) {
            const trimmed = line.trim();
            if (!trimmed) continue;
            const index = trimmed.indexOf('=');
            if (index <= 0) continue;
            next[trimmed.slice(0, index).trim()] = trimmed.slice(index + 1);
        }
        onAgentEnvChange?.(next);
    }
    async function select() {
        const id = ++generation.current;
        setError(''); setBusy(true);
        try {
            const selected = await runtime.select();
            if (id === generation.current && selected) onChange(selected);
        } catch {
            if (id === generation.current) setError('无法打开程序选择窗口，请直接填写完整路径。');
        } finally { if (id === generation.current) setBusy(false); }
    }
    async function copy() {
        if (!report) return;
        const text = [
            `${product} 环境检测：${report.ready ? '通过' : '未通过'}`,
            `程序：${report.executable || '未找到'}`,
            `版本：${report.name || '未知'} ${report.version || ''} · ACP ${report.protocolVersion || '未知'} · 扩展 ${report.extensionApiVersion || '未知'}`,
            ...report.checks.map(item => `${item.label}：${item.status === 'passed' ? '通过' : item.status === 'failed' ? '失败' : '未检查'}${item.detail ? ` — ${item.detail}` : ''}`),
            report.advice || '',
            modelReport ? `模型配置：${modelReport.ready ? '已读取' : '未就绪'} ${modelReport.name || ''} — ${modelReport.detail}` : '',
        ].filter(Boolean).join('\n');
        try { await navigator.clipboard.writeText(text); setCopied(true); }
        catch { setError('复制失败，请手动选择检测结果复制。'); }
    }
    const locked = disabled || busy;
    const selectedAgent = customAgents.find(agent => agent.id === customAgentId);
    const selectedValue = kind === 'custom' ? selectedAgent ? `custom:${selectedAgent.id}` : 'custom:unsaved' : kind;
    const status = busy ? '检测中' : report ? report.ready ? '可用' : '不可用' : '未检测';
    const failedChecks = report ? report.checks.filter(item => item.status === 'failed' && item.detail) : [];
    const declared = report?.capabilities ? ([
        report.capabilities.loadSession ? '会话恢复' : '',
        report.capabilities.additionalRoots ? '多项目目录' : '',
        report.capabilities.httpMcp ? 'HTTP MCP' : '',
        report.capabilities.modelOptions > 0 ? `模型选项 ×${report.capabilities.modelOptions}` : '',
        report.capabilities.modeOptions > 0 ? `模式选项 ×${report.capabilities.modeOptions}` : '',
    ] as string[]).filter(Boolean) : [];
    const advancedNote = kind === 'custom' ? 'Agent 本机操作是否会请求确认取决于其自身配置，快速检测不能证明全部操作都会经过审批。'
        : kind === 'codeagent' ? 'CodeAgent 本机操作是否会请求确认取决于其自身配置，请先核实。' : '';
    const pathField = <div className="oc-runtime-path">
        <input id="assistant-runtime-path" aria-label="程序路径" value={value} disabled={locked} placeholder={kind === 'custom' ? '填写 Agent 程序的完整路径' : `留空自动查找 ${product}`} spellCheck={false}
            onChange={event => onChange(event.target.value)} />
        <IconButton label={`选择 ${product} 程序`} disabled={locked} onClick={() => void select()}><TbFolderOpen size={18} /></IconButton>
        {kind !== 'custom' && <IconButton label="使用自动查找" disabled={locked || !value} onClick={() => onChange('')}><TbRotate2 size={18} /></IconButton>}
    </div>;
    return <section className="oc-runtime-settings" style={{...settingsCard, padding: 'var(--space-16) var(--space-20)'}} aria-labelledby="assistant-runtime-heading" aria-busy={busy}>
        <div className="oc-runtime-titlebar">
            <h3 id="assistant-runtime-heading" style={{...cardTitle, marginBottom: 0}}>助手运行环境</h3>
            <span className={`oc-runtime-status ${report ? report.ready ? 'is-ready' : 'is-failed' : ''}`}>{status}</span>
        </div>
        {onKindChange && <div className="oc-compact-row">
            <span className="oc-compact-label">当前 Agent</span>
            <select className="oc-runtime-field oc-compact-select" aria-label="当前 Agent" value={selectedValue} disabled={locked} onChange={event => {
                const next = event.target.value;
                if (next.startsWith('custom:')) {
                    if (next !== 'custom:unsaved') onCustomAgentsChange?.(customAgents, next.slice(7));
                } else onKindChange(next);
            }}><option value="codeagent">CodeAgent</option><option value="icode">iCode</option><option value="opencode">OpenCode</option>{customAgents.map(agent => <option key={agent.id} value={`custom:${agent.id}`}>{agent.name || '未命名 Agent'}</option>)}{kind === 'custom' && !selectedAgent && <option value="custom:unsaved">未命名 Agent</option>}</select>
            {onCustomAgentsChange && <IconButton label="添加 Agent" disabled={locked} onClick={() => {
                const id = crypto.randomUUID();
                const preserveDraft = kind === 'custom' && !selectedAgent;
                onCustomAgentsChange([...customAgents, {id, name:'新 Agent', executable:preserveDraft ? value : '', arguments:preserveDraft ? args : []}], id);
            }}><TbPlus size={17}/></IconButton>}
            <span className="oc-compact-note" title={kind === 'custom' && !selectedAgent ? '点击 + 添加 Agent，并填写程序信息。' : sourceNote}>{kind === 'custom' && !selectedAgent ? '点击 + 添加 Agent，并填写程序信息。' : sourceNote}</span>
        </div>}
        {kind === 'custom' && onCustomAgentsChange && selectedAgent && <div className="oc-compact-row">
            <span className="oc-compact-label">Agent 名称</span>
            <input className="oc-runtime-field oc-compact-grow" aria-label="Agent 名称" value={selectedAgent.name} disabled={locked} onChange={event => onCustomAgentsChange(customAgents.map(agent => agent.id === customAgentId ? {...agent, name:event.target.value} : agent), customAgentId)} />
            <IconButton label="移除 Agent" disabled={locked} onClick={() => onCustomAgentsChange(customAgents.filter(agent => agent.id !== customAgentId), '')}><TbTrash size={17}/></IconButton>
        </div>}
        {kind === 'custom' && <div className="oc-compact-row">
            <span className="oc-compact-label">程序路径</span>
            {pathField}
        </div>}
        <div style={cardDivider} />
        <div className="oc-compact-row">
            <button type="button" className="oc-runtime-check" style={btnSecondary} disabled={locked} onClick={() => void check()}>
                <TbRefresh size={16} className={busy ? 'is-spinning' : ''} aria-hidden="true" />{busy ? '正在检测…' : '检测环境'}
            </button>
            <div className="oc-compact-status" role="status">
                {busy ? '正在检测，最多等待约 45 秒…'
                    : report ? report.ready
                        ? <><span>运行环境可用</span>{report.version ? <> · <span className="oc-compact-meta">{`${report.name || 'iCode'} ${report.version} · ACP ${report.protocolVersion || '—'} · 扩展 ${report.extensionApiVersion || '—'}`}</span></> : null}</>
                        : <span>运行环境未就绪</span>
                    : '未检测；只验证程序启动与 ACP 握手，不调用模型。'}
            </div>
        </div>
        {report && !report.ready && <div className="oc-runtime-result" role="status">
            <div className="oc-runtime-result-heading">
                <ul className="oc-runtime-chips">{report.checks.map(item => <li key={item.id} className={`is-${item.status}`} title={item.detail || ''}>
                    {item.status === 'passed' ? <TbCheck size={13} aria-hidden="true" /> : item.status === 'failed' ? <TbCircleX size={13} aria-hidden="true" /> : <TbMinus size={13} aria-hidden="true" />}
                    <span>{item.label}</span>
                    {item.status !== 'passed' && <small>{item.status === 'failed' ? '失败' : '未检查'}</small>}
                </li>)}</ul>
                <IconButton label={copied ? '检测结果已复制' : '复制检测结果'} onClick={() => void copy()}>{copied ? <TbCheck size={16} /> : <TbCopy size={16} />}</IconButton>
            </div>
            <code className="oc-runtime-exe">{report.executable || '未找到程序'}</code>
            {!!failedChecks.length && <div className="oc-runtime-failures">{failedChecks.map(item => <p key={item.id}><span>{item.label}：</span><span>{item.detail}</span></p>)}</div>}
            {!!declared.length && <p className="oc-runtime-caps">已声明：{declared.join(' · ')}</p>}
            {report.advice && <p className="oc-runtime-advice">{report.advice}</p>}
        </div>}
        {report?.ready && report.advice && <p className="oc-runtime-advice">{report.advice}</p>}
        {error && <p className="oc-runtime-error" role="alert">{error}</p>}
        {modelReport && <div className={`oc-runtime-model-result ${modelReport.ready ? '' : 'is-failed'}`} role="status">
            <strong>{modelReport.ready ? `模型配置已读取${modelReport.name ? ` · ${modelReport.name}` : ''}` : '模型配置未就绪'}</strong>
            <p>{modelReport.detail}</p>
        </div>}
        <details className="oc-runtime-advanced" open={advancedOpen} onToggle={event => setAdvancedOpen((event.target as HTMLDetailsElement).open)}>
            <summary>{kind === 'custom' ? '启动参数与环境变量（高级）' : '手动配置：程序路径、启动参数、环境变量'}</summary>
            <div className="oc-runtime-advanced-body">
                {kind !== 'custom' && <label className="oc-runtime-advanced-label">{`程序路径（留空自动查找 ${product}）`}{pathField}</label>}
                {(kind === 'custom' || kind === 'codeagent') && <label className="oc-runtime-advanced-label">启动参数（每行一个）<textarea className="oc-runtime-field oc-runtime-textarea" aria-label="Agent 启动参数" value={args.join('\n')} disabled={locked} placeholder="例如：acp" onChange={event => onArgumentsChange?.(event.target.value.split('\n'))} /></label>}
                {kind === 'codeagent' && <label className="oc-runtime-advanced-label">{'环境变量（每行 KEY=VALUE；{{OPS_MODEL_API_KEY}} 启动时替换为「模型服务」密钥，不落盘）'}<textarea className="oc-runtime-field oc-runtime-textarea" aria-label="Agent 环境变量" value={envText()} disabled={locked} placeholder={'OPENAI_BASE_URL=https://…\nOPENAI_API_KEY={{OPS_MODEL_API_KEY}}'} onChange={event => applyEnv(event.target.value)} /></label>}
                {advancedNote && <p className="oc-runtime-advanced-note">{advancedNote}</p>}
            </div>
        </details>
    </section>;
}
