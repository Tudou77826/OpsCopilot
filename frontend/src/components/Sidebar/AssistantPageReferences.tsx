import { useEffect, useId, useLayoutEffect, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { TbAt, TbCheck, TbDatabase, TbFolder, TbTerminal2, TbX } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import { usePortalRoot } from '../../../../frontend-shell/src/ui/Surface';
import type { AssistantPageOption, AssistantPageReference } from '../../../../frontend-shell/src/assistant/runtime';
import './assistantPageReferences.css';

export const samePage = (a: AssistantPageReference, b: AssistantPageReference) =>
    a.kind === b.kind && a.tabId === b.tabId && a.connectionId === b.connectionId;
const labels: Record<string,string> = {terminal:'终端', database:'数据库', file:'文件'};
export function PageReferenceIcon({kind}: {kind: AssistantPageReference['kind']}) {
    const Icon = kind === 'terminal' ? TbTerminal2 : kind === 'database' ? TbDatabase : TbFolder;
    return <Icon size={14} aria-hidden="true"/>;
}

/** Resource selectors only. Choosing a page never reads it or takes ownership. */
export function AssistantPagePicker({options, selected, open, onOpenChange, onToggle, disabled, query, onQueryChange}: {
    options: AssistantPageOption[]; selected: AssistantPageReference[]; open: boolean;
    onOpenChange(open: boolean): void; onToggle(option: AssistantPageOption): void;
    disabled: boolean; query: string; onQueryChange(query: string): void;
}) {
    const trigger = useRef<HTMLButtonElement>(null);
    const menu = useRef<HTMLDivElement>(null);
    const search = useRef<HTMLInputElement>(null);
    const root = usePortalRoot();
    const id = useId();
    const [position, setPosition] = useState({left:8, bottom:0, width:320, maxHeight:300});
    const visible = options.filter(option => `${option.title} ${option.description || ''} ${labels[option.kind]}`.toLocaleLowerCase().includes(query.trim().toLocaleLowerCase()));
    useLayoutEffect(() => {
        if (!open) return;
        const update = () => {
            const rect = trigger.current?.getBoundingClientRect();
            if (!rect) return;
            const width = Math.min(320, window.innerWidth - 16);
            setPosition({left:Math.max(8, Math.min(rect.left, window.innerWidth-width-8)), bottom:window.innerHeight-rect.top+6, width, maxHeight:Math.max(100, Math.min(360, rect.top-16))});
        };
        update(); search.current?.focus();
        window.addEventListener('resize', update);
        window.addEventListener('scroll', update, true);
        return () => {window.removeEventListener('resize', update); window.removeEventListener('scroll', update, true);};
    }, [open]);
    useEffect(() => {
        if (!open) return;
        const outside = (event: Event) => {
            const target = event.target as Node;
            if (!menu.current?.contains(target) && !trigger.current?.contains(target)) onOpenChange(false);
        };
        document.addEventListener('pointerdown', outside);
        return () => document.removeEventListener('pointerdown', outside);
    }, [open, onOpenChange]);
    return <>
        <IconButton ref={trigger} label="引用页签" title="引用页签 · 也可输入 @" disabled={disabled}
            aria-haspopup="dialog" aria-expanded={open} aria-controls={open?id:undefined}
            onClick={() => onOpenChange(!open)}><TbAt size={17}/></IconButton>
        {open && !disabled && createPortal(<div ref={menu} id={id} className="ai-page-picker" role="dialog" aria-label="引用页签" style={position}
            onKeyDown={event => {
                if (event.key==='Escape') {event.preventDefault(); onOpenChange(false); trigger.current?.focus();}
                if (event.key==='ArrowDown' || event.key==='ArrowUp') {
                    const buttons = Array.from(menu.current?.querySelectorAll<HTMLButtonElement>('[role="checkbox"]:not(:disabled)') || []);
                    if (!buttons.length) return;
                    event.preventDefault();
                    const index=buttons.indexOf(document.activeElement as HTMLButtonElement);
                    buttons[index<0 ? event.key==='ArrowDown'?0:buttons.length-1 : (index+(event.key==='ArrowDown'?1:buttons.length-1))%buttons.length]?.focus();
                }
            }}>
            <header><strong>引用页签</strong><small>{selected.length}/8</small><IconButton label="完成引用" onClick={() => {onOpenChange(false); trigger.current?.focus();}}><TbCheck size={16}/></IconButton></header>
            <input ref={search} aria-label="搜索页签" placeholder="搜索名称、地址…" value={query} onChange={event=>onQueryChange(event.target.value)}/>
            <div className="ai-page-options">{visible.map(option => {
                const checked=selected.some(ref=>samePage(ref,option));
                const reason=option.unavailable || (!checked && selected.length>=8?'最多引用 8 个页签':undefined);
                return <button type="button" key={`${option.kind}:${option.tabId}:${option.connectionId}`} role="checkbox" aria-checked={checked} disabled={!!reason}
                    title={[labels[option.kind],option.title,option.description,reason].filter(Boolean).join(' · ')} onClick={()=>onToggle(option)}>
                    <PageReferenceIcon kind={option.kind}/><span><span>{option.title}</span><small>{reason || option.description || labels[option.kind]}</small></span>{checked&&<TbCheck size={15}/>}</button>;
            })}{!visible.length&&<p>{options.length?'没有匹配的页签':'暂无可引用页签，请先打开终端、数据库或文件工作区。'}</p>}</div>
            <footer>指定关注对象；内容按需读取，操作仍遵循现有授权。</footer>
        </div>,root)}
    </>;
}

export function PageReferenceTags({references, options, onRemove}: {
    references: AssistantPageReference[]; options?: AssistantPageOption[]; onRemove?: (reference: AssistantPageReference) => void;
}) {
    if (!references.length) return null;
    return <div className="ai-page-reference-tags" aria-label="引用的页签">{references.map(ref => {
        const unavailable=options && !options.some(option=>samePage(option,ref)&&!option.unavailable);
        return <span key={`${ref.kind}:${ref.tabId}:${ref.connectionId}`} className={unavailable?'is-unavailable':undefined}
            title={`${labels[ref.kind]} · ${ref.title}${unavailable?' · 已关闭、未连接或连接已更换':''}`}>
            <PageReferenceIcon kind={ref.kind}/><span>{ref.title}</span>{unavailable&&<small>不可用</small>}
            {onRemove&&<IconButton label={`移除引用：${ref.title}`} onClick={()=>onRemove(ref)}><TbX size={12}/></IconButton>}
        </span>;
    })}</div>;
}
