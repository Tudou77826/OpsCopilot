import { useEffect, useRef, useState } from 'react';
import { TbCheck, TbCircleCheck, TbCircleX, TbCopy, TbFolderOpen, TbMinus, TbRefresh, TbRotate2, TbTerminal2 } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { CheckAssistantRuntime, InspectAssistantModel, SelectAssistantRuntimeExecutable } from '../../../wailsjs/go/main/App';
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
    select(): Promise<string>;
    model?(source: string): Promise<{ ready: boolean; name?: string; detail: string }>;
}
const host: RuntimeHost = { check: CheckAssistantRuntime, select: SelectAssistantRuntimeExecutable, model: InspectAssistantModel };

export default function AssistantRuntimePanel({ value, onChange, disabled = false, runtime = host }: {
    value: string; onChange: (value: string) => void; disabled?: boolean; runtime?: RuntimeHost;
}) {
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
    }, [value]);

    async function check() {
        const id = ++generation.current;
        setBusy(true); setError(''); setReport(null); setCopied(false);
        try {
            const [result, model] = await Promise.all([runtime.check(value.trim()), runtime.model?.('icode')]);
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
            `iCode 环境检测：${report.ready ? '通过' : '未通过'}`,
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
    return <section className="oc-runtime-settings" aria-labelledby="assistant-runtime-heading" aria-busy={busy}>
        <header className="oc-runtime-heading">
            <div><TbTerminal2 size={20} aria-hidden="true" /><h3 id="assistant-runtime-heading">内置 AI 助手</h3><span className="oc-runtime-product">iCode</span></div>
            <span className={`oc-runtime-status ${report ? report.ready ? 'is-ready' : 'is-failed' : ''}`}>
                {busy ? '检测中' : report ? report.ready ? '检测通过' : '需要处理' : '未检测'}
            </span>
        </header>
        <p className="oc-runtime-description">选择本机的 iCode 运行程序，检查它能否接入助手。</p>
        <div className="oc-runtime-model-source">
            <p>助手使用本机 iCode 已配置的模型与认证信息。请在 iCode 中管理模型；对话和操作授权仍独立。</p>
        </div>
        <label className="oc-runtime-label" htmlFor="assistant-runtime-path">程序路径</label>
        <div className="oc-runtime-path">
            <input id="assistant-runtime-path" value={value} disabled={locked} placeholder="留空自动查找 iCode" spellCheck={false}
                onChange={event => onChange(event.target.value)} />
            <IconButton label="选择 iCode 程序" disabled={locked} onClick={() => void select()}><TbFolderOpen size={18} /></IconButton>
            <IconButton label="使用自动查找" disabled={locked || !value} onClick={() => onChange('')}><TbRotate2 size={18} /></IconButton>
        </div>
        <div className="oc-runtime-actions">
            <span>{value ? '保存后优先使用此路径。' : '自动查找程序附带目录、环境变量和 PATH。'}</span>
            <button type="button" className="oc-runtime-check" disabled={locked} onClick={() => void check()}>
                <TbRefresh size={16} className={busy ? 'is-spinning' : ''} aria-hidden="true" />{busy ? '正在检测…' : '检测环境'}
            </button>
        </div>
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
    </section>;
}
