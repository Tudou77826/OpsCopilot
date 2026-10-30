import { useEffect, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { TbCheck, TbChevronUp, TbShield } from 'react-icons/tb';
import { usePortalRoot } from '../../../../frontend-shell/src/ui/Surface';
import type { AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import './assistantApprovalMode.css';

const modes = [
    { id: 'auto', label: '自动审批', detail: '助手审查与 Ops 风险兜底，需要时询问' },
    { id: 'manual', label: '手动审核', detail: '执行操作前由你逐次确认' },
    { id: 'bypass', label: '完全访问', detail: '执行操作时不再请求审批' },
] as const;

export function AssistantApprovalMode({ runtime, busy, revision, onChanging, onError }: {
    runtime: AssistantRuntime; busy: boolean; revision: number; onChanging: (value: boolean) => void; onError: (value: string) => void;
}) {
    const [mode, setMode] = useState('auto');
    const [open, setOpen] = useState(false);
    const [confirmBypass, setConfirmBypass] = useState(false);
    const [changing, setChanging] = useState(false);
    const [loaded, setLoaded] = useState(false);
    const [unverifiedNative, setUnverifiedNative] = useState(false);
    const generation = useRef(0);
    const [position, setPosition] = useState({ left: 0, bottom: 0 });
    const trigger = useRef<HTMLButtonElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const portal = usePortalRoot();
    useEffect(() => {
        if (busy || !runtime.approvalMode) return;
        const version = ++generation.current;
        setLoaded(false);
        void Promise.resolve().then(() => runtime.approvalMode!()).then(value => { if (generation.current === version) { setMode(value); setLoaded(true); } }).catch(error => { if (generation.current === version) onError(String(error)); });
        return () => { generation.current++; };
    }, [runtime, revision, busy, onError]);
    useEffect(() => {
        let live = true;
        if (runtime.agentKind) void Promise.resolve().then(() => runtime.agentKind!()).then(kind => { if (live) setUnverifiedNative(kind === 'custom'); }).catch(() => { if (live) setUnverifiedNative(true); });
        return () => { live = false; };
    }, [runtime, revision, busy]);
    useEffect(() => { if (busy || changing) { setOpen(false); setConfirmBypass(false); } }, [busy, changing]);
    useEffect(() => {
        if (!confirmBypass) return;
        const key = (event: KeyboardEvent) => {
            if (event.key === 'Escape') { setConfirmBypass(false); trigger.current?.focus(); }
        };
        document.addEventListener('keydown', key);
        return () => document.removeEventListener('keydown', key);
    }, [confirmBypass]);
    useLayoutEffect(() => {
        if (!open) return;
        const update = () => { const rect = trigger.current?.getBoundingClientRect(); if (rect) setPosition({left: Math.max(8, Math.min(rect.left, window.innerWidth - 276)), bottom: window.innerHeight - rect.top + 6}); };
        update(); window.addEventListener('resize', update); window.addEventListener('scroll', update, true);
        return () => { window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true); };
    }, [open]);
    useEffect(() => {
        if (!open) return;
        const outside = (event: Event) => { const target = event.target as Node; if (!trigger.current?.contains(target) && !menu.current?.contains(target)) setOpen(false); };
        const key = (event: KeyboardEvent) => { if (event.key === 'Escape') { setOpen(false); trigger.current?.focus(); } };
        document.addEventListener('pointerdown', outside); document.addEventListener('keydown', key);
        return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('keydown', key); };
    }, [open]);
    if (!runtime.approvalMode || !runtime.selectApprovalMode) return null;
    const current = modes.find(option => option.id === mode);
    async function select(next: string) {
        setOpen(false);
        if (next === mode || !runtime.selectApprovalMode) return;
        generation.current++; setChanging(true); onChanging(true); onError('');
        try { setMode(await runtime.selectApprovalMode(next)); }
        catch (error) { onError(`权限模式切换未完成：${String(error)}`); }
        finally { setChanging(false); onChanging(false); }
    }
    return <div className="ai-approval-picker">
        <button ref={trigger} type="button" className={`ai-approval-trigger${mode === 'bypass' ? ' is-bypass' : ''}`} aria-label={`权限模式：${current?.label || '未知'}${unverifiedNative ? '，本机操作是否受控未验证' : ''}`} aria-haspopup="menu" aria-expanded={open} disabled={busy || changing || !loaded} onClick={() => setOpen(value => !value)} title={unverifiedNative ? 'Ops 操作受控；自定义 Agent 本机操作是否受控未验证' : '权限模式'}>
            <TbShield size={15}/><span>{current?.label || '权限模式'}</span>{unverifiedNative && <span aria-hidden="true" title="Agent 本机操作是否受控未验证">!</span>}<TbChevronUp size={11}/>
        </button>
        {open && createPortal(<div ref={menu} className="ai-approval-menu" role="menu" aria-label="权限模式" style={position}>
            <div className="ai-approval-menu-title">权限模式 <small>仅当前对话</small></div>
            {unverifiedNative && <div className="ai-approval-menu-title">Ops 操作受控；Agent 本机操作是否受控未验证</div>}
            {modes.map(option => <button key={option.id} type="button" role="menuitemradio" aria-checked={mode === option.id} onClick={() => { if (option.id === 'bypass' && mode !== 'bypass') { setOpen(false); setConfirmBypass(true); } else void select(option.id); }}>
                <span><strong>{option.label}</strong><small>{unverifiedNative && option.id !== 'bypass' ? '适用于 Ops 操作；本机操作取决于 Agent' : option.detail}</small></span>{mode === option.id && <TbCheck size={15}/>}
            </button>)}
        </div>, portal)}
        {confirmBypass && createPortal(<div className="ai-approval-confirm-backdrop" onMouseDown={event => { if (event.target === event.currentTarget) setConfirmBypass(false); }}>
            <section className="ai-approval-confirm" role="alertdialog" aria-modal="true" aria-labelledby="approval-confirm-title" aria-describedby="approval-confirm-description">
                <div className="ai-approval-confirm-icon"><TbShield size={20}/></div>
                <div><h3 id="approval-confirm-title">启用完全访问？</h3>
                    <p id="approval-confirm-description">助手将无需审批即可在本机和已连接的服务器上执行操作，包括修改或删除文件、执行命令和 SQL。</p>
                    <p>建议仅在个人开发环境中使用。此设置仅对当前对话有效。</p>
                </div>
                <div className="ai-approval-confirm-actions"><button type="button" autoFocus onClick={() => { setConfirmBypass(false); trigger.current?.focus(); }}>取消</button><button type="button" className="is-danger" onClick={() => { setConfirmBypass(false); void select('bypass'); }}>确认启用</button></div>
            </section>
        </div>, portal)}
    </div>;
}
