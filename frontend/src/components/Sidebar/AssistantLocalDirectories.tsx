import { useEffect, useRef, useState } from 'react';
import { TbCheck, TbFolder, TbFolderPlus, TbFolderSearch, TbPencil, TbPlus, TbTrash, TbX } from 'react-icons/tb';
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
}
const wailsHost: LocalDirectoryHost = {
    list: App.AssistantLocalProjects, create: App.CreateAssistantLocalProject,
    activate: App.SelectAssistantLocalProject, rename: App.RenameAssistantLocalProject,
    delete: App.DeleteAssistantLocalProject, add: App.AddAssistantProjectDirectories,
    remove: App.RemoveAssistantProjectDirectory, select: App.SelectAssistantLocalDirectory,
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
            try {const next = await host.list(); if (live && version === generation.current) setState(next);}
            catch (e) {if (live && open) setError(String(e));}
            finally {reading = false;}
        }
        void refresh();
        const timer = busy || open ? setInterval(() => void refresh(), 1500) : undefined;
        return () => {live = false; clearInterval(timer);};
    }, [host, busy, open, revision]);
    useEffect(() => {if (open) dialog.current?.showModal();}, [open]);
    const disabled = busy || working;
    async function run(action: () => Promise<LocalProjectState | null>, done?: () => void) {
        if (busy || operation.current) return;
        operation.current = true; generation.current++;
        setWorking(true); setError('');
        try {
            const next = await action();
            if (mounted.current && next) {setState(next); setPath(''); done?.();}
        } catch (e) {if (mounted.current) setError(String(e));}
        finally {operation.current = false; if (mounted.current) setWorking(false);}
    }
    function close() {setOpen(false); setEdit(undefined); setRemoving(undefined);}
    function saveProject() {
        if (!name.trim()) return;
        void run(() => creating ? host.create(name.trim()) : host.rename(editingID, name.trim()), () => {setEdit(undefined); setEditingID(''); setName('');});
    }
    return <>
        <span className="oc-local-directory-entry">
            <IconButton label="本地项目" title={active ? `${active.name} · ${paths.length} 个目录` : '管理本地项目与目录'} onClick={() => {setError(''); setOpen(true);}}>
                <TbFolder size={17}/>{!compact && <span>{active ? `本地 · ${active.name}` : '本地项目'}</span>}
                {!!active && !compact && <small>{paths.length}</small>}
            </IconButton>
        </span>
        {open && <dialog ref={dialog} className="oc-local-directories" aria-labelledby="oc-local-directory-title" onCancel={close}>
            <header><div><h3 id="oc-local-directory-title">本地项目</h3><p>按项目保存目录，切换后供助手使用</p></div><IconButton label="关闭本地项目" onClick={close}><TbX size={18}/></IconButton></header>
            <div className="oc-local-directory-body">
                {!!state.projects.length && <div className="oc-local-project-picker">
                    <label htmlFor="oc-local-project">当前项目</label>
                    <div><select id="oc-local-project" value={state.activeId} disabled={disabled || !!edit || !!removing} onChange={event => {const id = event.target.value; void run(() => host.activate(id));}}>
                        <option value="">未选择项目</option>{state.projects.map(p => <option key={p.id} value={p.id}>{p.name} · {p.directories.length} 个目录</option>)}
                    </select>
                    <IconButton label="新建项目" disabled={disabled || !!edit || !!removing} onClick={() => {setEdit('create'); setName('');}}><TbPlus size={18}/></IconButton>
                    <IconButton label="重命名项目" disabled={disabled || !active || !!edit || !!removing} onClick={() => {setEdit('rename'); setEditingID(active!.id); setName(active!.name);}}><TbPencil size={17}/></IconButton>
                    <IconButton label="移除项目" disabled={disabled || !active || !!edit || !!removing} onClick={() => setRemoving(active)}><TbTrash size={17}/></IconButton></div>
                </div>}
                {(creating || edit === 'rename') && <form className="oc-local-project-editor" onSubmit={event => {event.preventDefault(); saveProject();}}>
                    <label htmlFor="oc-local-project-name">{creating ? '新建项目' : '重命名项目'}</label>
                    <div><input id="oc-local-project-name" aria-label="项目名称" autoFocus value={name} onChange={event => setName(event.target.value)} disabled={disabled} maxLength={64} placeholder="例如：A 项目"/>
                    <IconButton label="保存项目" disabled={disabled || !name.trim()} onClick={saveProject}><TbCheck size={18}/></IconButton>
                    {!!state.projects.length && <IconButton label="取消编辑项目" disabled={disabled} onClick={() => setEdit(undefined)}><TbX size={17}/></IconButton>}</div>
                </form>}
                {removing && <div className="oc-local-project-remove" role="alert"><p>移除项目「{removing.name}」的配置？<br/><span>磁盘上的目录和文件不会删除。</span></p><div><button disabled={disabled} onClick={() => setRemoving(undefined)}>取消</button><button disabled={disabled} onClick={() => void run(() => host.delete(removing.id), () => setRemoving(undefined))}>移除项目配置</button></div></div>}
                {!creating && !edit && !removing && <>
                    <div className="oc-local-project-heading"><span>{active ? `${active.name}的目录` : '尚未选择项目'}</span>{active && <small>{paths.length} / 16</small>}</div>
                    {paths.length ? <ul aria-label="项目目录">{paths.map(p => <li key={p}><TbFolder size={20}/><div><strong>{p.split(/[\\/]/).filter(Boolean).slice(-1)[0] || p}</strong><span title={p}>{p}</span></div><IconButton label={`移除目录 ${p}`} disabled={disabled} onClick={() => void run(() => host.remove(active!.id, p))}><TbX size={16}/></IconButton></li>)}</ul> : <div className="oc-local-directory-empty"><TbFolderPlus size={28}/><strong>{active ? '为这个项目添加工作目录' : '选择项目后管理它的目录'}</strong><p>项目代码、日志和下载目录可以分开放。</p></div>}
                    {!!active && <form onSubmit={event => {event.preventDefault(); if (path.trim()) void run(() => host.add(active.id, [path.trim()]));}}>
                        <label htmlFor="oc-local-directory-path">添加目录</label><div><input id="oc-local-directory-path" value={path} onChange={event => setPath(event.target.value)} disabled={disabled} placeholder="粘贴本地目录的完整路径" maxLength={2048}/>
                        <IconButton label="选择本地文件夹" disabled={disabled} onClick={() => void run(async () => {const selected = await host.select(); return selected ? host.add(active.id, [selected]) : null;})}><TbFolderSearch size={18}/></IconButton>
                        <IconButton label="添加本地目录" disabled={disabled || !path.trim()} onClick={() => void run(() => host.add(active.id, [path.trim()]))}><TbPlus size={18}/></IconButton></div>
                    </form>}
                </>}
                {error && <p className="oc-local-directory-error" role="alert">{error}</p>}
                {busy && <p className="oc-local-directory-note" role="status">助手运行中，可查看配置；修改请等待本轮完成或先停止。</p>}
            </div>
            <footer>项目配置自动保存，重启或新建对话后仍保留。<br/>切换项目从后续消息生效；不会自动读取内容或放开操作审批。</footer>
        </dialog>}
    </>;
}
