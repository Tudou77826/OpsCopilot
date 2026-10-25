import { useEffect, useId, useLayoutEffect, useRef, useState, type KeyboardEvent } from 'react';
import { createPortal } from 'react-dom';
import { TbCheck, TbChevronUp, TbRefresh } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { AssistantModelOption, AssistantModels, AssistantRuntime } from '../../../../frontend-shell/src/assistant/runtime';
import { usePortalRoot } from '../../../../frontend-shell/src/ui/Surface';
import './assistantModelPicker.css';

function modelLabel(model: AssistantModelOption) { return model.name.trim() || model.modelId; }

export function AssistantModelPicker({runtime, busy, revision, onChanging, onError}: {
    runtime: AssistantRuntime; busy: boolean; revision: number; onChanging: (value: boolean) => void; onError: (value: string) => void;
}) {
    const [state, setState] = useState<AssistantModels>({models: []});
    const [loading, setLoading] = useState(true);
    const [changing, setChanging] = useState(false);
    const [error, setError] = useState('');
    const generation = useRef(0);
    const operation = useRef(false);
    const mounted = useRef(false);
    const focusAfterChange = useRef(false);
    const portalRoot = usePortalRoot();
    const trigger = useRef<HTMLButtonElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const menuId = useId();
    const [open, setOpen] = useState(false);
    const [active, setActive] = useState(0);
    const [position, setPosition] = useState({left:0,bottom:0,width:160,maxHeight:300});
    const search = useRef({text:'',time:0});
    const current = state.current;
    const missing = current && !state.models.some(model => model.id === current.id);
    const options = missing ? [current, ...state.models] : state.models;
    const disabled = busy || changing || loading || !state.models.length;

    useEffect(() => { if (disabled) setOpen(false); }, [disabled]);
    useEffect(() => {
        if (changing || !focusAfterChange.current) return;
        focusAfterChange.current = false;
        if (!disabled) trigger.current?.focus();
    }, [changing, disabled]);
    useLayoutEffect(() => {
        if (!open || disabled) return;
        const update = () => {
            const rect = trigger.current?.getBoundingClientRect();
            if (!rect) return;
            const width = Math.min(220, Math.max(160, rect.width), window.innerWidth - 16);
            const top = rect.top - 6;
            setPosition({
                left:Math.max(8, Math.min(rect.right - width, window.innerWidth - width - 8)),
                bottom:window.innerHeight - top,
                width,
                maxHeight:Math.max(0, Math.min(300, top - 8)),
            });
        };
        update();
        const observer = typeof ResizeObserver === 'undefined' ? undefined : new ResizeObserver(update);
        if (trigger.current) observer?.observe(trigger.current);
        window.addEventListener('resize', update);
        window.addEventListener('scroll', update, true);
        return () => { observer?.disconnect(); window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true); };
    }, [open, disabled]);
    useEffect(() => {
        if (!open) return;
        const outside = (event: Event) => {
            const target = event.target as Node;
            if (!trigger.current?.contains(target) && !menu.current?.contains(target)) setOpen(false);
        };
        const close = () => setOpen(false);
        document.addEventListener('pointerdown', outside);
        document.addEventListener('focusin', outside);
        window.addEventListener('blur', close);
        return () => { document.removeEventListener('pointerdown', outside); document.removeEventListener('focusin', outside); window.removeEventListener('blur', close); };
    }, [open]);
    useEffect(() => { if (open) menu.current?.querySelector<HTMLElement>(`[data-index="${active}"]`)?.scrollIntoView?.({block:'nearest'}); }, [open, active]);
    useEffect(() => {mounted.current = true; return () => {mounted.current = false; generation.current++;};}, []);
    async function refresh() {
        if (!runtime.modelOptions || operation.current) return;
        const version = ++generation.current;
        setLoading(true);
        try {
            const next = await runtime.modelOptions();
            if (mounted.current && generation.current === version) {setState(next); setError('');}
        } catch (e) {
            if (mounted.current && generation.current === version) setError(String(e));
        } finally {
            if (mounted.current && generation.current === version) setLoading(false);
        }
    }
    useEffect(() => {if (!busy) void refresh();}, [runtime, busy, revision]);
    useEffect(() => {const detail = error || state.detail; if (detail) onError(detail);}, [error, state.detail, onError]);
    async function select(id: string) {
        if (busy || operation.current || !runtime.selectModel || !id) return;
        setOpen(false);
        trigger.current?.focus();
        if (id === current?.id) return;
        focusAfterChange.current = true;
        operation.current = true; generation.current++;
        setChanging(true); onChanging(true); setError('');
        try {
            const next = await runtime.selectModel(id);
            if (mounted.current) setState(next);
        } catch (e) {if (mounted.current) setError(`切换未完成：${String(e)}`);}
        finally {
            operation.current = false;
            if (mounted.current) {setChanging(false); setLoading(false); onChanging(false);}
        }
    }
    function showMenu(index = options.findIndex(model => model.id === current?.id)) {
        if (disabled) return;
        search.current = {text:'',time:0};
        setActive(Math.max(0, index));
        setOpen(true);
    }
    function onKeyDown(event: KeyboardEvent<HTMLButtonElement>) {
        if (disabled) return;
        if (event.key === 'Tab') { setOpen(false); return; }
        if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); setOpen(false); return; }
        if (event.key === 'Enter' || event.key === ' ') {
            event.preventDefault();
            if (open) { const model = options[active]; if (model) void select(model.id); }
            else showMenu();
            return;
        }
        if (['ArrowDown','ArrowUp','Home','End'].includes(event.key)) {
            event.preventDefault();
            if (event.key === 'Home') showMenu(0);
            else if (event.key === 'End') showMenu(options.length - 1);
            else if (!open) showMenu();
            else setActive(index => (index + (event.key === 'ArrowDown' ? 1 : -1) + options.length) % options.length);
            return;
        }
        if (event.key.length === 1 && !event.ctrlKey && !event.metaKey && !event.altKey) {
            const now = Date.now();
            const text = (now - search.current.time < 700 ? search.current.text : '') + event.key.toLocaleLowerCase();
            search.current = {text,time:now};
            const index = options.findIndex(model => [modelLabel(model),model.modelId].some(name => name.toLocaleLowerCase().startsWith(text)));
            if (index >= 0) { event.preventDefault(); setActive(index); setOpen(true); }
        }
    }
    if (!runtime.modelOptions || !runtime.selectModel) return null;
    const detail = error || state.detail;
    const label = changing ? '正在切换模型…' : loading ? '正在读取模型…' : current ? `当前模型：${modelLabel(current)}${modelLabel(current) !== current.modelId ? ` · ${current.modelId}` : ''}（iCode）` : '选择 iCode 中已配置的模型';
    return <div className="ai-model-picker">
        <button ref={trigger} type="button" role="combobox" className="ai-model-select" aria-label="助手模型" aria-haspopup="listbox" aria-expanded={open && !disabled} aria-controls={open && !disabled ? menuId : undefined} aria-activedescendant={open && !disabled ? `${menuId}-${active}` : undefined} disabled={disabled} title={busy ? `${label}；助手空闲后可切换` : label} onKeyDown={onKeyDown} onClick={() => open ? setOpen(false) : showMenu()}>
            <span>{changing ? '切换模型…' : current ? modelLabel(current) : loading ? '读取模型…' : '选择模型'}</span>
            <TbChevronUp size={12} aria-hidden="true"/>
        </button>
        {open && !disabled && createPortal(<div ref={menu} id={menuId} className="ai-model-menu" role="listbox" aria-label="iCode 模型" style={position}>
            {options.map((model,index) => {
                const name = modelLabel(model);
                const description = [name, name !== model.modelId ? model.modelId : '', missing && model.id === current?.id ? '当前会话' : ''].filter(Boolean).join(' · ');
                return <button key={model.id} id={`${menuId}-${index}`} type="button" role="option" tabIndex={-1} data-index={index} title={description} aria-selected={current?.id === model.id} className={active === index ? 'is-active' : undefined} onMouseEnter={() => setActive(index)} onClick={() => void select(model.id)}>
                    <span className="ai-model-option-label">{name}</span>
                    {current?.id === model.id && <TbCheck size={15} aria-hidden="true"/>}
                </button>;
            })}
        </div>, portalRoot)}
        {!!detail && <IconButton label="重新读取 iCode 模型" title={detail} disabled={busy || changing || loading} onClick={() => void refresh()}><TbRefresh size={15}/></IconButton>}
        {changing && <span className="ai-model-status" role="status">正在切换模型，保留当前对话…</span>}
    </div>;
}
