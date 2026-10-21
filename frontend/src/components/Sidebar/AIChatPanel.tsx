import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react';
import { AIComposer, RichText } from '../AI';
import { IconButton, ResourceLibraryIcon } from '@opscopilot/shell-terminal/ui';
import { TbMessagePlus, TbPlayerStop, TbRefresh, TbX } from 'react-icons/tb';
import { LatestMessageButton } from './LatestMessageButton';
import type { AssistantRuntime, AssistantTarget } from '../../../../frontend-shell/src/assistant/runtime';
import { wailsAssistantRuntime } from '../../shell-adapter/wailsAssistantRuntime';
import { useAssistantQueue } from './useAssistantQueue';
import { AssistantInteractionCard } from '../AssistantInteraction/AssistantInteractionCard';
import { DatabaseConnectionRequest } from '../AssistantInteraction/DatabaseConnectionRequest';
import { AIChatMessage } from './AIChatMessage';
import { AssistantMessageQueue } from './AssistantMessageQueue';
import { AssistantLibrary } from '../AssistantLibrary/AssistantLibrary';
import { CommandDraftEditor } from '../AssistantLibrary/CommandDraftEditor';
import { CommandMenu } from '../AssistantLibrary/CommandMenu';
import type { AssistantCommand, CommandDraft, ConnectionChoice, LibraryHost, LibraryState } from '../AssistantLibrary/types';
import { wailsAssistantLibrary } from '../../shell-adapter/wailsAssistantLibrary';
import '../AssistantLibrary/assistantLibrary.css';
import './aiChatPanel.css';

interface AIChatPanelProps {
    runtime?: AssistantRuntime;
    activeTerminalTitle?: string;
    activeTerminalTarget?: AssistantTarget;
    terminalTabs?: {id:string;connectionId?:string;title:string}[];
    title?: string;
    onClose?: () => void;
    children?: ReactNode;
    libraryHost?: LibraryHost;
}

export default function AIChatPanel({ activeTerminalTitle, activeTerminalTarget, terminalTabs, runtime = wailsAssistantRuntime, title = 'AI 助手', onClose, children, libraryHost=wailsAssistantLibrary }: AIChatPanelProps) {
    const assistant = useAssistantQueue(runtime);
    const [controlError, setControlError] = useState('');
    const [input, setInput] = useState('');
    const [resetting, setResetting] = useState(false);
    const [showLatest, setShowLatest] = useState(false);
    const [libraryOpen,setLibraryOpen]=useState(false);
    const [library,setLibrary]=useState<LibraryState>({skills:[],commands:[]});
    const [draft,setDraft]=useState<CommandDraft>();
    const [connections,setConnections]=useState<ConnectionChoice[]>([]);
    const [preparing,setPreparing]=useState(false);
    const [menuDismissed,setMenuDismissed]=useState(false);
    const prepareGeneration=useRef(0);
    const preparingRef=useRef(false);
    useEffect(()=>{
        let live=true;
        void Promise.resolve().then(()=>libraryHost.read()).then(value=>{if(live)setLibrary(value);}).catch(()=>{/* A missing bridge is reported when the manager is opened. */});
        return()=>{live=false;prepareGeneration.current++;};
    },[libraryHost]);
    function chooseCommand(command:AssistantCommand){setDraft({command,template:command.template,values:{}});setInput('');setControlError('');void Promise.resolve().then(()=>libraryHost.connections()).then(setConnections).catch(error=>setControlError(String(error)));}
    const showMenu=!draft&&!menuDismissed&&/^\/[^\s]*$/.test(input);
    const history = useRef<HTMLDivElement>(null);
    const followLatest = useRef(true);
    const busy = !!assistant.current;
    const messages = assistant.messages;
    const contextTitle = activeTerminalTarget ? activeTerminalTitle : undefined;
    const actualTarget=assistant.state?.target;
    const turnTitle=actualTarget?.tabId ? terminalTabs?.find(tab=>tab.id===actualTarget.tabId && tab.connectionId===actualTarget.connectionId)?.title || (assistant.current?.target?.tabId===actualTarget.tabId ? assistant.current.title : undefined) : assistant.current?.title;

    function scrollToLatest() {
        followLatest.current = true;
        if (history.current) history.current.scrollTop = history.current.scrollHeight;
        setShowLatest(false);
    }
    useLayoutEffect(() => {
        if (followLatest.current) scrollToLatest();
        else setShowLatest(true);
    }, [messages, assistant.state?.output]);

    function send() {
        if(showMenu)return;
        if(draft){void sendCommand();return;}
        const question = input.trim();
        if (!question || assistant.transitioning) return;
        followLatest.current = true;
        setControlError('');
        try {
            assistant.enqueue(question, activeTerminalTarget, activeTerminalTitle);
            setInput('');
        } catch (error) {
            setControlError(error instanceof Error ? error.message : String(error));
        }
    }
    async function sendCommand(){
        if(!draft||assistant.transitioning||preparingRef.current)return;
        const missing=draft.command.fields.find(field=>field.required&&!draft.values[field.key]?.trim());
        if(missing){setControlError(`请填写${missing.label}`);return;}
        preparingRef.current=true;setPreparing(true);setControlError('');
        const generation=++prepareGeneration.current;
        // Capture target with the submitted draft, never after an async lookup.
        const target=activeTerminalTarget,targetTitle=activeTerminalTitle;
        try{
            const question=await libraryHost.prepare(draft.command.name,draft.template,draft.values);
            if(generation!==prepareGeneration.current)return;
            followLatest.current=true;assistant.enqueue(question,target,targetTitle);setDraft(undefined);setInput('');
        }catch(error){if(generation===prepareGeneration.current)setControlError(String(error));}
        finally{if(generation===prepareGeneration.current){preparingRef.current=false;setPreparing(false);}}
    }
    async function stop() {
        setControlError('');
        try { await assistant.stop(); }
        catch (error) { setControlError(`停止未完成：${String(error)}`); }
    }
    async function sendNow(id: number) {
        setControlError('');
        try { await assistant.sendNow(id); }
        catch (error) { setControlError(`尚未发送：${error instanceof Error ? error.message : String(error)}`); }
    }
    async function newConversation() {
        if (busy || assistant.transitioning || assistant.restoring || preparingRef.current) return;
        setResetting(true);
        setControlError('');
        try {
            await assistant.newConversation();
            followLatest.current = true;
            setInput(''); setDraft(undefined); setShowLatest(false);
        } catch (error) {
            setControlError(`无法新建对话：${error instanceof Error ? error.message : String(error)}`);
        } finally { setResetting(false); }
    }

    // Describe only observed host state; never infer reading or execution
    // from model inference or from the presence of a terminal candidate.
    const progress = resetting ? '正在新建对话…' : assistant.transitioning || assistant.state?.phase === 'stopped' ? '正在停止，等待本轮结束…' : assistant.state?.prompt ?
        assistant.state.prompt.kind === 'approval' ? '等待你确认' : '等待你回答' : assistant.state?.activity==='opening' ? '正在打开终端…' : assistant.state?.activity==='selecting' ? '正在切换终端…' : assistant.state?.output ? '正在回答…' :
        assistant.state?.activity==='database' ? '正在处理数据库任务…' : assistant.state?.activity==='local' ? '正在使用工具…' : assistant.state?.target.tabId ? '正在处理操作…' : assistant.state?.turnId ? '正在思考…' : '正在启动助手…';

    return <div className="ai-panel ai-chat-panel">
        <header className="ai-chat-header">
            <h3>{title}</h3>
            <div className="ai-chat-header-actions" role="toolbar" aria-label="对话操作">
                <IconButton label="新建对话" disabled={busy || assistant.transitioning || assistant.restoring || preparing} onClick={() => void newConversation()}><TbMessagePlus size={18}/></IconButton>
                {onClose && <IconButton label="关闭助手" onClick={onClose}><TbX size={18}/></IconButton>}
            </div>
        </header>
        {children}
        {libraryOpen&&<AssistantLibrary host={libraryHost} busy={busy||assistant.transitioning} onClose={()=>setLibraryOpen(false)} onChanged={setLibrary}/>}
        <div className="ai-chat-history">
            <div ref={history} className="ai-conversation-scroll" aria-label="对话消息" onScroll={event => {
                const element = event.currentTarget;
                followLatest.current = element.scrollHeight - element.clientHeight - element.scrollTop < 56;
                setShowLatest(!followLatest.current);
            }}>
                {messages.length ? <div className="ai-message-list">
                    {assistant.truncated && <p className="ai-history-note">仅显示最近的消息，助手仍可能记得更早的上下文。</p>}
                    {messages.map(message => <AIChatMessage key={message.id} content={message.content} phase={message.phase} user={message.role === 'user'} />)}
                    {busy && assistant.state?.output && <article className="ai-assistant-message ai-streaming-message"><RichText content={assistant.state.output} /></article>}
                </div> : <div className="ai-chat-empty">
                    <h4>{assistant.restoring ? '正在恢复对话…' : assistant.restoreError ? '对话尚未恢复' : '有什么需要一起排查？'}</h4>
                    {!assistant.restoring && !assistant.restoreError && <p>可以直接提问，需要操作时再选择终端。</p>}
                </div>}
            </div>
            <LatestMessageButton anchor={history} visible={showLatest} onClick={scrollToLatest}/>
        </div>
        <div className="ai-panel-footer">
            {!assistant.transitioning && assistant.state?.prompt && (assistant.state.prompt.kind==='database_connection' ? <DatabaseConnectionRequest key={assistant.state.prompt.id} prompt={assistant.state.prompt} onRespond={assistant.respond}/> : <AssistantInteractionCard key={assistant.state.prompt.id} prompt={assistant.state.prompt} onRespond={assistant.respond} onStop={() => void stop()} showStop={false} />)}
            {(busy || assistant.transitioning) && <div className="ai-chat-activity">
                <div className={`ai-chat-progress${assistant.state?.prompt ? ' is-waiting' : ''}`} role="status"><i aria-hidden="true" /><span>{progress}</span>
                    {turnTitle && turnTitle !== contextTitle && <small title={`本轮终端 · ${turnTitle}`}>{turnTitle}</small>}
                </div>
                {busy && <IconButton label="停止本轮" className="ai-chat-stop" onClick={() => void stop()} disabled={assistant.transitioning}><TbPlayerStop size={17}/></IconButton>}
            </div>}
            {controlError && <p className="ai-chat-error" role="alert">{controlError}</p>}
            {assistant.restoreError && <div className="ai-restore-error" role="alert"><span>{assistant.restoreError}</span><IconButton label="重试恢复对话" onClick={()=>void assistant.restore()}><TbRefresh size={18}/></IconButton></div>}
            <div className="oc-command-composer" onKeyDown={event=>{if(event.key==='Escape')setMenuDismissed(true);}}>
            {showMenu&&<CommandMenu commands={library.commands.filter(c=>!c.skill||library.skills.some(s=>s.name===c.skill&&s.enabled))} query={input.slice(1)} onChoose={chooseCommand} onManage={()=>{setMenuDismissed(true);setLibraryOpen(true);}}/>}
            <AIComposer value={draft?draft.template:input} onChange={value=>{setInput(value);setMenuDismissed(false);}} onSend={send} disabled={assistant.transitioning||assistant.restoring||!!assistant.restoreError||preparing} sendDisabled={showMenu||!!draft?.command.fields.some(field=>field.required&&!draft.values[field.key]?.trim())} busy={busy} queueWhileBusy queuePending={assistant.pending.length > 0} variant="conversation"
                toolbarLeading={<IconButton label="管理 Skills 和快捷命令" title="资源库 · Skills 与快捷命令" className="ai-composer-library" onClick={()=>setLibraryOpen(true)}><ResourceLibraryIcon/></IconButton>}
                editor={draft?<CommandDraftEditor key={draft.command.name} draft={draft} connections={connections} disabled={assistant.transitioning||preparing} onChange={setDraft} onSend={send} onCancel={()=>{setDraft(undefined);setControlError('');}}/>:undefined}
                header={<AssistantMessageQueue messages={assistant.pending} paused={assistant.paused} disabled={assistant.transitioning} resumeDisabled={busy} currentTitle={contextTitle}
                    onResume={() => { setControlError(''); assistant.resume(); }} onSendNow={id => void sendNow(id)} onRemove={assistant.remove} />}
                placeholder="输入问题…"
                hint={busy ? 'Enter 加入队列 · 立即发送会先停止本轮' : assistant.paused && assistant.pending.length ? '队列已暂停，继续后按顺序处理' : undefined}
                contexts={contextTitle ? [{ id: 'terminal', label: contextTitle, active: true }] : []} />
            </div>
        </div>
    </div>;
}
