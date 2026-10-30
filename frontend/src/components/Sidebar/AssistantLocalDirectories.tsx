import { useEffect, useRef, useState } from 'react';
import { TbCheck, TbChevronRight, TbCircle, TbCircleCheckFilled, TbDots, TbFolder, TbFolderPlus, TbPencil, TbPlus, TbTrash, TbX } from 'react-icons/tb';
import { IconButton } from '@opscopilot/shell-terminal/ui';
import type { LocalProject, LocalProjectState } from '../../../../frontend-shell/src/assistant/localProjects';
import * as App from '../../../wailsjs/go/main/App';
import './assistantLocalDirectories.css';

export interface LocalDirectoryHost {
    list(): Promise<LocalProjectState>;
    create(name: string): Promise<LocalProjectState>;
    activate(id: string): Promise<LocalProjectState>;
    rename(id: string, name: string): Promise<LocalProjectState>;
    delete(id: string): Promise<LocalProjectState>;
    add(id: string, paths: string[]): Promise<LocalProjectState>;
    remove(id: string, path: string): Promise<LocalProjectState>;
    select(): Promise<string>;
    primary(id: string, path: string): Promise<LocalProjectState>;
    status(): Promise<{state: string}>;
}
const wailsHost: LocalDirectoryHost = {
    list: App.AssistantLocalProjects, create: App.CreateAssistantLocalProject,
    activate: App.SelectAssistantLocalProject, rename: App.RenameAssistantLocalProject,
    delete: App.DeleteAssistantLocalProject, add: App.AddAssistantProjectDirectories,
    remove: App.RemoveAssistantProjectDirectory, select: App.SelectAssistantLocalDirectory,
    primary: App.SetAssistantProjectPrimaryDirectory, status: App.AssistantProjectWorkspace,
};
const empty: LocalProjectState = {version: 1, activeId: '', projects: []};

export function AssistantLocalDirectories({busy, revision, compact=false, host = wailsHost}: {busy: boolean; revision: number; compact?: boolean; host?: LocalDirectoryHost}) {
    const [state, setState] = useState<LocalProjectState>(empty);
    const [open, setOpen] = useState(false);
    const [path, setPath] = useState('');
    const [name, setName] = useState('');
    const [edit, setEdit] = useState<'create' | 'rename'>();
    const [editingID, setEditingID] = useState('');
    const [removing, setRemoving] = useState<LocalProject>();
    const [working, setWorking] = useState(false);
    const [error, setError] = useState('');
    const [workspaceStatus, setWorkspaceStatus] = useState('pending');
    const [expanded, setExpanded] = useState<string[]>([]);
    const [manualFor, setManualFor] = useState('');
    const [menuFor, setMenuFor] = useState('');
    const initialized = useRef(false);
    const menu = useRef<HTMLDivElement>(null);
    const [loadState, setLoadState] = useState<'loading' | 'ready' | 'error'>('loading');
    const dialog = useRef<HTMLDialogElement>(null);
    const mounted = useRef(true);
    const operation = useRef(false);
    const generation = useRef(0);
    const active = state.projects.find(p => p.id === state.activeId);
    const paths = active?.directories ?? [];
    const creating = edit === 'create' || !state.projects.length;
    useEffect(() => {mounted.current = true; return () => {mounted.current = false;};}, []);
    useEffect(() => {
        let live = true, reading = false;
        async function refresh() {
            if (reading || operation.current) return;
            reading = true;
            const version = generation.current;
            try {const next = await host.list(); const status = await host.status(); if (live && version === generation.current) {setState(next); setWorkspaceStatus(status.state); setLoadState('ready');}}
            catch (e) {if (live && version === generation.current) {setLoadState('error'); if (open) setError(String(e));}}
            finally {reading = false;}
        }
        void refresh();
        const timer = busy || open ? setInterval(() => void refresh(), 1500) : undefined;
        return () => {live = false; clearInterval(timer);};
    }, [host, busy, open, revision]);
    useEffect(() => {if (open) dialog.current?.showModal();}, [open]);
    useEffect(() => {
        if (open && loadState === 'ready' && !initialized.current) {
            setExpanded(state.activeId ? [state.activeId] : []);
            initialized.current = true;
        }
    }, [open, loadState, state.activeId]);
    useEffect(() => {
        if (!menuFor) return;
        const dismiss = (event: PointerEvent) => {if (!menu.current?.contains(event.target as Node)) setMenuFor('');};
        const key = (event: KeyboardEvent) => {if (event.key === 'Escape') {event.preventDefault(); setMenuFor('');}};
        document.addEventListener('pointerdown', dismiss);
        document.addEventListener('keydown', key);
        return () => {document.removeEventListener('pointerdown', dismiss); document.removeEventListener('keydown', key);};
    }, [menuFor]);
    const disabled = busy || working;
    async function run(action: () => Promise<LocalProjectState | null>, done?: (next: LocalProjectState) => void) {
        if (busy || operation.current) return;
        operation.current = true; generation.current++;
        setWorking(true); setError('');
        try {
            const next = await action();
            if (mounted.current && next) {setState(next); setLoadState('ready'); setPath(''); try {setWorkspaceStatus((await host.status()).state);} catch {setWorkspaceStatus('error');} done?.(next);}
        } catch (e) {if (mounted.current) setError(String(e));}
        finally {operation.current = false; if (mounted.current) setWorking(false);}
    }
    function close() {setOpen(false); setEdit(undefined); setRemoving(undefined); setManualFor(''); setMenuFor(''); setPath(''); initialized.current = false;}
    function saveProject() {
        if (!name.trim()) return;
        void run(() => creating ? host.create(name.trim()) : host.rename(editingID, name.trim()), next => {if (creating && next.activeId) setExpanded(current => [...new Set([...current, next.activeId])]); setEdit(undefined); setEditingID(''); setName('');});
    }
    const statusText = !active || !paths.length ? '尚未选择工作目录' : workspaceStatus === 'applied' ? '已应用到助手' : workspaceStatus === 'error' ? '目录不可用' : workspaceStatus === 'running' ? '助手运行中' : '下次发送时应用';
    const projectStatus = loadState === 'loading' ? '正在读取项目配置' : loadState === 'error' ? '项目配置读取失败' : active ? `${active.name} · ${statusText}` : '未选择本地项目';
    return <>
        <span className={`oc-local-directory-entry${loadState === 'ready' && active && paths.length ? ' has-directories' : ''}`}>
            <IconButton label="本地项目" title={projectStatus} onClick={() => {setError(''); setOpen(true);}}>
                <TbFolder size={17}/>{active && loadState === 'ready' ? <span>{active.name}</span> : !compact && <span>本地项目</span>}
                {active && loadState === 'ready' && <small>{paths.length}</small>}
                {active && paths.length > 0 && <i className={`oc-project-status-dot is-${workspaceStatus}`} aria-hidden="true"/>}
            </IconButton>
        </span>
        {open && <dialog ref={dialog} className="oc-local-directories" aria-labelledby="oc-local-directory-title" onCancel={close}>
            <header><div><h3 id="oc-local-directory-title">本地项目</h3><p>选择项目，让助手结合对应的代码和资料工作</p></div><div className="oc-project-header-actions"><IconButton label="新建项目" title="新建项目" disabled={disabled || !!edit || !!removing} onClick={() => {setEdit('create'); setName('');}}><TbPlus size={18}/></IconButton><IconButton label="关闭本地项目" onClick={close}><TbX size={18}/></IconButton></div></header>
            <div className="oc-local-directory-body">
                {!!state.projects.length && <div className="oc-project-list" aria-label="项目列表">
                    {state.projects.map(project => {
                        const selected = project.id === active?.id;
                        const isExpanded = expanded.includes(project.id);
                        return <section className="oc-project-tree" key={project.id} aria-label={`项目 ${project.name}`}>
                            <div className="oc-project-row">
                                <button type="button" className="oc-project-choice" aria-label={`选择项目 ${project.name}`} aria-pressed={selected} title={selected ? '当前项目' : '使用此项目'} disabled={disabled || !!edit || !!removing} onClick={() => void run(() => host.activate(project.id))}>
                                    {selected ? <TbCircleCheckFilled size={17}/> : <TbCircle size={17}/>}</button>
                                <button type="button" className="oc-project-expand" aria-expanded={isExpanded} onClick={() => {setExpanded(current => isExpanded ? current.filter(id => id !== project.id) : [...current, project.id]); setMenuFor('');}}>
                                    <TbChevronRight className={isExpanded ? 'is-expanded' : ''} size={16}/><TbFolder size={17}/><strong>{project.name}</strong></button>
                                <small className="oc-project-meta">{selected ? `当前项目 · ${statusText}` : `${project.directories.length} 个目录`}</small>
                                <div className="oc-project-menu-anchor" ref={menuFor === project.id ? menu : undefined}>
                                    <IconButton label={`管理项目 ${project.name}`} title="管理项目" disabled={disabled || !!edit || !!removing} onClick={() => setMenuFor(menuFor === project.id ? '' : project.id)}><TbDots size={18}/></IconButton>
                                    {menuFor === project.id && <div className="oc-project-menu"><button type="button" onClick={() => {setMenuFor(''); setEdit('rename'); setEditingID(project.id); setName(project.name);}}><TbPencil size={15}/>重命名</button><button type="button" onClick={() => {setMenuFor(''); setRemoving(project);}}><TbTrash size={15}/>移除项目</button></div>}
                                </div>
                            </div>
                            {isExpanded && <div className="oc-project-children">
                                {project.directories.length ? <ul aria-label={`${project.name}的项目目录`}>{project.directories.map((p, index) => <li key={p}><TbFolder size={18}/><div><strong>{p.split(/[\\/]/).filter(Boolean).slice(-1)[0] || p} {index === 0 && <em>主目录</em>}</strong><span title={p}>{p}</span></div><div className="oc-directory-actions">{index > 0 && <button className="oc-project-primary" type="button" disabled={disabled} onClick={() => void run(() => host.primary(project.id, p))}>设为主目录</button>}<IconButton label={`移除目录 ${p}`} title="移除目录" disabled={disabled} onClick={() => void run(() => host.remove(project.id, p))}><TbX size={16}/></IconButton></div></li>)}</ul> : <div className="oc-project-empty">还没有工作目录</div>}
                                <div className="oc-project-add"><button type="button" disabled={disabled} onClick={() => void run(async () => {const selectedPath = await host.select(); return selectedPath ? host.add(project.id, [selectedPath]) : null;})}><TbFolderPlus size={16}/>添加目录</button><button type="button" disabled={disabled} onClick={() => {setManualFor(manualFor === project.id ? '' : project.id); setPath('');}}>输入路径</button></div>
                                {manualFor === project.id && <form className="oc-project-path" onSubmit={event => {event.preventDefault(); if (path.trim()) void run(() => host.add(project.id, [path.trim()]), () => setManualFor(''));}}>
                                    <label htmlFor={`oc-local-path-${project.id}`}>添加工作目录</label><div><input id={`oc-local-path-${project.id}`} value={path} onChange={event => setPath(event.target.value)} disabled={disabled} placeholder="输入文件夹的完整路径" maxLength={2048}/><IconButton label="添加本地目录" title="添加目录" disabled={disabled || !path.trim()} onClick={() => void run(() => host.add(project.id, [path.trim()]), () => setManualFor(''))}><TbPlus size={18}/></IconButton></div>
                                </form>}
                            </div>}
                        </section>;
                    })}
                    <button type="button" className="oc-no-project" aria-pressed={!active} disabled={disabled || !!edit || !!removing} onClick={() => void run(() => host.activate(''))}>{!active ? <TbCircleCheckFilled size={17}/> : <TbCircle size={17}/>}<span>不使用项目</span></button>
                </div>}
                {(creating || edit === 'rename') && <form className="oc-local-project-editor" onSubmit={event => {event.preventDefault(); saveProject();}}>
                    <label htmlFor="oc-local-project-name">{creating ? '新建项目' : '重命名项目'}</label>
                    <div><input id="oc-local-project-name" aria-label="项目名称" autoFocus value={name} onChange={event => setName(event.target.value)} disabled={disabled} maxLength={64} placeholder="例如：支付系统"/>
                    <IconButton label="保存项目" title="保存项目" disabled={disabled || !name.trim()} onClick={saveProject}><TbCheck size={18}/></IconButton>
                    {!!state.projects.length && <IconButton label="取消编辑项目" title="取消" disabled={disabled} onClick={() => setEdit(undefined)}><TbX size={17}/></IconButton>}</div>
                </form>}
                {removing && <div className="oc-local-project-remove" role="alert"><p>移除项目「{removing.name}」的配置？<br/><span>磁盘上的目录和文件不会删除。</span></p><div><button disabled={disabled} onClick={() => setRemoving(undefined)}>取消</button><button disabled={disabled} onClick={() => void run(() => host.delete(removing.id), () => setRemoving(undefined))}>移除项目配置</button></div></div>}
                {loadState === 'error' && !error && <p className="oc-local-directory-error" role="alert">项目配置读取失败，请重试。</p>}
                {error && <p className="oc-local-directory-error" role="alert">{error}</p>}
                {busy && <p className="oc-local-directory-note" role="status">助手运行中，可查看配置；修改请等待本轮完成或先停止。</p>}
            </div>
            <footer>主目录用于打开项目；其他目录也可供助手查阅。切换后将在下一条消息生效。</footer>
        </dialog>}
    </>;
}
