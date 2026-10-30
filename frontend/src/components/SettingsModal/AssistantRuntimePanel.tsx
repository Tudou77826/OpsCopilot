import { useEffect, useRef, useState } from 'react';
import { TbCheck, TbCircleCheck, TbCircleX, TbCopy, TbFolderOpen, TbInfoCircle, TbMinus, TbPlus, TbRefresh, TbRotate2, TbTerminal2, TbTrash } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { CheckAssistantRuntime, CheckAssistantAgentRuntime, CheckCustomAssistantRuntime, InspectAssistantModel, SelectAssistantRuntimeExecutable } from '../../../wailsjs/go/main/App';
import './assistantRuntime.css';

export interface RuntimeReport {
    ready: boolean;
    executable: string;
    name?: string;
    version?: string;
    protocolVersion?: number;
    extensionApiVersion?: number;
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

export default function AssistantRuntimePanel({ value, onChange, kind = 'icode', onKindChange, arguments: args = [], onArgumentsChange, customAgents = [], customAgentId = '', onCustomAgentsChange, disabled = false, runtime = host }: {
    value: string; onChange: (value: string) => void; kind?: string; onKindChange?: (kind:string)=>void; arguments?: string[]; onArgumentsChange?: (args:string[])=>void; customAgents?:CustomAgent[]; customAgentId?:string; onCustomAgentsChange?: (agents:CustomAgent[], id:string)=>void; disabled?: boolean; runtime?: RuntimeHost;
}) {
    const product = kind === 'opencode' ? 'OpenCode' : kind === 'custom' ? '自定义 ACP Agent' : 'iCode';
    const [report, setReport] = useState<RuntimeReport | null>(null);
    const [busy, setBusy] = useState(false);
    const [error, setError] = useState('');
    const [copied, setCopied] = useState(false);
    const [modelReport, setModelReport] = useState<{ ready: boolean; name?: string; detail: string } | null>(null);
    const generation = useRef(0);
    useEffect(() => {
        generation.current++;
        setReport(null); setModelReport(null); setError(''); setBusy(false); setCopied(false);
        return () => { generation.current++; };
    }, [value, kind, args.join('\n')]);

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
    return <section className="oc-runtime-settings" aria-labelledby="assistant-runtime-heading" aria-busy={busy}>
        <header className="oc-runtime-heading">
            <div className="oc-runtime-heading-main"><span className="oc-runtime-heading-icon"><TbTerminal2 size={20} aria-hidden="true" /></span><div><h3 id="assistant-runtime-heading">助手运行环境</h3><p>选择助手使用的 Agent，并检测连接状态</p></div></div>
            <span className={`oc-runtime-status ${report ? report.ready ? 'is-ready' : 'is-failed' : ''}`}>
                {busy ? '检测中' : report ? report.ready ? '检测通过' : '需要处理' : '未检测'}
            </span>
        </header>
        <div className="oc-runtime-layout"><div className="oc-runtime-selection">
        {onKindChange && <div className="oc-runtime-agent-picker">
            <label className="oc-runtime-label">当前 Agent<select aria-label="当前 Agent" value={selectedValue} disabled={locked} onChange={event => {
                const next = event.target.value;
                if (next.startsWith('custom:')) {
                    if (next !== 'custom:unsaved') onCustomAgentsChange?.(customAgents, next.slice(7));
                } else onKindChange(next);
            }}><option value="icode">iCode</option><option value="opencode">OpenCode</option>{customAgents.map(agent => <option key={agent.id} value={`custom:${agent.id}`}>{agent.name || '未命名 Agent'}</option>)}{kind === 'custom' && !selectedAgent && <option value="custom:unsaved">未命名 Agent</option>}</select></label>
            {onCustomAgentsChange && <IconButton label="添加 Agent" disabled={locked} onClick={() => {
                const id = crypto.randomUUID();
                const preserveDraft = kind === 'custom' && !selectedAgent;
                onCustomAgentsChange([...customAgents, {id, name:'新 Agent', executable:preserveDraft ? value : '', arguments:preserveDraft ? args : []}], id);
            }}><TbPlus size={17}/></IconButton>}
        </div>}
        {kind === 'custom' && onCustomAgentsChange && selectedAgent && <div className="oc-runtime-profile-edit"><label className="oc-runtime-label">Agent 名称<input aria-label="Agent 名称" value={selectedAgent.name} disabled={locked} onChange={event => onCustomAgentsChange(customAgents.map(agent => agent.id === customAgentId ? {...agent, name:event.target.value} : agent), customAgentId)} /></label><IconButton label="移除 Agent" disabled={locked} onClick={() => onCustomAgentsChange(customAgents.filter(agent => agent.id !== customAgentId), '')}><TbTrash size={17}/></IconButton></div>}
        {onCustomAgentsChange && <p className="oc-runtime-selection-hint">{kind === 'custom' && !selectedAgent ? '点击 + 添加 Agent，并填写程序信息。' : '点击 + 添加其他 ACP Agent。'}</p>}
        <p className="oc-runtime-selection-note">模型和认证使用 {product} 本机配置。</p>
        </div><div className="oc-runtime-configuration">
        <label className="oc-runtime-label" htmlFor="assistant-runtime-path">程序路径</label>
        <div className="oc-runtime-path">
            <input id="assistant-runtime-path" value={value} disabled={locked} placeholder={kind === 'custom' ? '填写 Agent 程序的完整路径' : `留空自动查找 ${product}`} spellCheck={false}
                onChange={event => onChange(event.target.value)} />
            <IconButton label={`选择 ${product} 程序`} disabled={locked} onClick={() => void select()}><TbFolderOpen size={18} /></IconButton>
            {kind !== 'custom' && <IconButton label="使用自动查找" disabled={locked || !value} onClick={() => onChange('')}><TbRotate2 size={18} /></IconButton>}
        </div>
        {kind === 'custom' && <label className="oc-runtime-label">启动参数（每行一个）<textarea aria-label="Agent 启动参数" value={args.join('\n')} disabled={locked} placeholder="例如：acp" onChange={event => onArgumentsChange?.(event.target.value.split('\n'))} /></label>}
        <div className="oc-runtime-actions">
            <span>{value ? '保存后优先使用此路径。' : kind === 'custom' ? '需要填写程序路径；参数请逐行填写。' : '自动查找程序附带目录、环境变量和 PATH。'}</span>
            <button type="button" className="oc-runtime-check" disabled={locked} onClick={() => void check()}>
                <TbRefresh size={16} className={busy ? 'is-spinning' : ''} aria-hidden="true" />{busy ? '正在检测…' : '检测环境'}
            </button>
        </div>
        {kind === 'opencode' && <p className="oc-runtime-inline-note"><TbInfoCircle size={16}/><span>自动审批使用「模型服务」中的快速模型；不可用时会请你确认。</span></p>}
        {kind === 'custom' && <div className="oc-runtime-notice"><TbInfoCircle size={17} aria-hidden="true"/><div><strong>使用前请了解</strong><p>Ops 操作受所选权限模式控制。Agent 的本机操作是否会请求确认取决于它自身的配置，请先核实；快速检测不能证明全部操作都会经过审批。</p><p>模型列表、上下文用量和 Agent 命令取决于其 ACP 支持情况；Ops 导入的 Skill 暂不会自动注入此 Agent。</p></div></div>}
        {busy && <p className="oc-runtime-progress" role="status">正在验证 ACP 与助手启动，最多等待约 45 秒…</p>}
        {report && <div className="oc-runtime-result" role="status">
            <div className="oc-runtime-result-heading">
                <strong>{report.ready ? '运行环境可用' : '运行环境未就绪'}</strong>
                <IconButton label={copied ? '检测结果已复制' : '复制检测结果'} onClick={() => void copy()}>{copied ? <TbCheck size={16} /> : <TbCopy size={16} />}</IconButton>
            </div>
            <div className="oc-runtime-metadata">
                {report.version && <span>{report.name || 'iCode'} {report.version} · ACP {report.protocolVersion || '—'} · 扩展 {report.extensionApiVersion || '—'}</span>}
                <code>{report.executable || '未找到程序'}</code>
            </div>
            <ul className="oc-runtime-checks">{report.checks.map(item => <li key={item.id} className={`is-${item.status}`}>
                {item.status === 'passed' ? <TbCircleCheck size={17} aria-hidden="true" /> : item.status === 'failed' ? <TbCircleX size={17} aria-hidden="true" /> : <TbMinus size={17} aria-hidden="true" />}
                <div><span>{item.label}</span>{item.detail && <p>{item.detail}</p>}</div>
                <small>{item.status === 'passed' ? '通过' : item.status === 'failed' ? '失败' : '未检查'}</small>
            </li>)}</ul>
            {report.advice && <p className="oc-runtime-advice">{report.advice}</p>}
        </div>}
        {error && <p className="oc-runtime-error" role="alert">{error}</p>}
        {modelReport && <div className={`oc-runtime-model-result ${modelReport.ready ? '' : 'is-failed'}`} role="status">
            <strong>{modelReport.ready ? `模型配置已读取${modelReport.name ? ` · ${modelReport.name}` : ''}` : '模型配置未就绪'}</strong>
            <p>{modelReport.detail}</p>
        </div>}
        <p className="oc-runtime-footnote">检测不调用模型或连接业务服务器。修改路径后请保存设置，并新建 AI 对话使用。</p>
        </div></div>
    </section>;
}
