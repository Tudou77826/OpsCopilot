import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { ProductFrame, ProductToolbar, ProductNavigation } from '../../../frontend-shell/src/ui/product/ProductChrome'
import { useProductNavigation } from '../../../frontend-shell/src/ui/product/useProductNavigation'
import { useCommandQuery } from '../../../frontend-shell/src/ui/product/useCommandQuery'
import { ProductSidebar } from '../../../frontend-shell/src/ui/product/ProductSidebar'
import BottomBar, { BOTTOM_BAR_TIPS } from '../../../frontend-shell/src/ui/product/BottomBar'
import FlexLayoutAdapter from '../../../frontend-shell/src/ui/FlexLayout/FlexLayoutAdapter'
import type { TerminalRef } from '../../../frontend-shell/src/ui/Terminal/Terminal'
import SmartConnectModal from '../../../frontend-shell/src/ui/connection/SmartConnectModal'
import ScriptRecordingPanel from '../../../frontend-shell/src/ui/script/ScriptRecordingPanel'
import { useToast } from '../../../frontend-shell/src/ui/feedback/Toast'
import { SessionStatus } from '../../../frontend-shell/src/ui/types'
import { normalizeTerminalConfig } from '../../../frontend-shell/src/ui/Terminal/terminalAppearance'
import { applySkin, hostTheme, observeHostTheme, persistSkin, persistThemeFollow, readPersistedSkinChoice, readPersistedThemeFollow, type ThemeFollow } from '../../../frontend-shell/src/ui/appearance'
import type { Skin } from '../../../frontend-shell/src/ui/appearanceTypes'
import ShellSettingsModal, { type ShellSettings } from '../../../frontend-shell/src/ui/settings/ShellSettingsModal'
import ScriptListPanel from '../../../frontend-shell/src/ui/script/ScriptListPanel'
import ScriptEditorModal from '../../../frontend-shell/src/ui/script/ScriptEditorModal'
import type { ScriptRuntime } from '../../../frontend-shell/src/ui/script/types'
import QuickCommandPanel from '../../../frontend-shell/src/ui/quickcmd/QuickCommandPanel'
import SessionManager from '../../../frontend-shell/src/ui/session/SessionManager'
import type { SessionManagerRuntime, SessionNode } from '../../../frontend-shell/src/ui/ports'
import type { ConnectionConfig } from '../../../frontend-shell/src/ui/types'
import type { QuickCommandHost } from '../../../frontend-shell/src/ui/ports'
import { confirmDialog } from '../../../frontend-shell/src/ui/feedback/ConfirmDialog'
import { TeamsTerminal } from './terminal'
import type { TeamsOpsClient } from './browser-client'
import { TeamsFileHost } from './file-host'
import FilesPanel from '../../../frontend-shell/src/ui/filetransfer/FilesPanel'
import type { Transfer } from './files'
import type { AIConfigRuntime } from '../../../frontend-shell/src/ui/settings/AIConfigCard'
import CommandQueryOverlay from '../../../frontend-shell/src/ui/command/CommandQueryOverlay'

type Saved = SessionNode
type Terminal = { terminalId: string; connectionId: string }
type Snapshot = { state: string; terminals: Terminal[]; recording: { is_recording: boolean; name?: string; terminalId?: string }; replays: { id: string; scriptId: string; terminalId: string; state: string; sent: number; total: number }[] }
const flatten = (nodes: Saved[]): Saved[] => nodes.flatMap(node => [node, ...flatten(node.children ?? [])])
const initialSettings: ShellSettings = { theme: 'light', terminal: normalizeTerminalConfig(), completionDelay: 150, highlightRules: [], commandQueryShortcut: 'Ctrl+K' }
const runLabels: Record<string, string> = { running: '发送中', stopping: '正在停止', dispatched: '已发送（不代表执行成功）', stopped: '已停止发送', failed: '发送失败', unknown: '结果不明，仍保护终端', interrupted: '运行时中断' }

export function OpsApp({ client, surface, hostSettings }: { client: TeamsOpsClient; surface: HTMLElement; hostSettings?: React.ReactNode }) {
  const [sessions, setSessions] = useState<Saved[]>([]), [snapshot, setSnapshot] = useState<Snapshot>({ state: 'starting', terminals: [], recording: { is_recording: false }, replays: [] })
  const [active, setActive] = useState<string | null>(null)
  const { sidebarOpen, setSidebarOpen, tab:section, quickOpen, setQuickOpen, settingsOpen:showSettings, setSettingsOpen:setShowSettings, toggleSidebar } = useProductNavigation({sidebarOpen:true,quickOpen:true})
  const [connectModal, setConnectModal] = useState(false), [connectSeed, setConnectSeed] = useState<ConnectionConfig[]>([])
  const [connectingSavedId, setConnectingSavedId] = useState<string>()
  const terminalRefs = useRef(new Map<string, TerminalRef>())
  const toast = useToast()
  const [settings, setSettings] = useState(initialSettings)
  // 明暗默认跟随宿主；用户手动切过之后才停在这一条上（跟随与否与"选了哪个"分开记，
  // 所以恢复跟随时不会丢掉用户原来的偏好）。
  const [themeFollow, setThemeFollow] = useState<ThemeFollow>(readPersistedThemeFollow)
  // 异步回调（settings.load）里要读当下的跟随状态，用 ref 避免读到闭包里的旧值。
  const themeFollowRef = useRef(themeFollow)
  // 皮肤是前端键（不进后端配置）：没选过就跟随宿主，选过就以选择为准。
  const [skin, setSkin] = useState<Skin>(() => readPersistedSkinChoice() ?? 'teams')
  const [error, setError] = useState(''), [busy, setBusy] = useState(false), [connected, setConnected] = useState(false)

  const [scriptId, setScriptId] = useState<string>(), scriptsRef = useRef<{ loadScripts(): void }>(null)
  const [titles, setTitles] = useState<Record<string, string>>({})
  const commandQuery = useCommandQuery({
    generate: query => client.call('ai.generate', {query}),
    type: command => send(command.replace(/[\r\n]+$/, '')),
    copy: command => navigator.clipboard.writeText(command), warn: message => toast.warning(message),
  }, active, settings.commandQueryShortcut || 'Ctrl+K')

  const senders = useRef(new Map<string, (text: string) => void>())
  const fileHost = useMemo(() => new TeamsFileHost(client), [client])
  const register = useCallback((id: string, sender?: (text: string) => void) => { if (sender) senders.current.set(id, sender); else senders.current.delete(id) }, [])
  const act = async (action: () => Promise<unknown>) => { setBusy(true); setError(''); try { await action() } catch (e) { setError((e as Error).message) } finally { setBusy(false) } }
  const refresh = useCallback(async () => {
    const [state, saved] = await Promise.all([client.call<Snapshot>('runtime.snapshot'), client.call<{ sessions: Saved[] }>('connections.list')])
    setSnapshot(state); setSessions(saved.sessions); setConnected(state.state === 'ready')
    fileHost.update((state as Snapshot & { transfers?: Transfer[] }).transfers ?? [])
    setActive(previous => state.terminals.some(t => t.terminalId === previous) ? previous : state.terminals[0]?.terminalId ?? null)
  }, [client, fileHost])
  const saveConnection = useCallback(async (config: ConnectionConfig, id?: string) => {
    const challenge = await client.call('connections.probe', { config });
    if (!await confirmDialog.show({ title: '确认主机指纹', message: `${config.user}@${config.host}:${config.port}\n${challenge.algorithm}\n${challenge.fingerprint}\n请通过可信渠道核对主机指纹。确认后主机密钥变化将拒绝连接。`, confirmText: '确认指纹' })) throw new Error('已取消保存连接');
    return await client.call<{id:string}>('connections.save', {config, id, challengeId:challenge.challengeId, confirmed:true});
  }, [client])
  // 会话树适配：main 的 SessionManager 要的是树接口，插件对外只有 connections.* 这层业务方法，
  // 这里逐个对上；插件没有的能力用等价组合实现（如"新建后归入目标文件夹"）。
  const moveSavedInto = useCallback(async (savedId: string, parentId: string) => {
    const tree = (await client.call<{ sessions: Saved[] }>('connections.list')).sessions ?? []
    const parent = flatten(tree).find(node => node.id === parentId)
    await client.call('connections.move', { id: savedId, newParentId: parentId, index: parent?.children?.length ?? 0 })
  }, [client])
  const sessionRuntime = useMemo<SessionManagerRuntime>(() => ({
    listTree: async () => (await client.call<{ sessions: SessionNode[] }>('connections.list')).sessions ?? [],
    createFolder: async (name, parentId) => { await client.call('connections.folder', { name, parentId }); await refresh() },
    // 插件保存连接必须先核对主机指纹，所以复用 saveConnection 的挑战流程，再按需归入目标文件夹。
    createConnection: async (config, parentId) => {
      const saved = await saveConnection(config)
      if (parentId) await moveSavedInto(saved.id, parentId)
      await refresh()
    },
    renameNode: async (id, newName) => { await client.call('connections.rename', { id, name: newName }); await refresh() },
    updateConnection: async (id, config) => { await saveConnection(config, id); await refresh() },
    moveNode: async (id, newParentId, index) => { await client.call('connections.move', { id, newParentId, index }); await refresh() },
    deleteNode: async id => { await client.call('connections.delete', { id }); await refresh() },
    reorderNodes: async (parentId, orderedIds) => { await client.call('connections.reorder', { parentId, orderedIds }); await refresh() },
    duplicateConnection: async id => { await client.call('connections.duplicate', { id }); await refresh() },
  }), [client, refresh, saveConnection, moveSavedInto])
  useEffect(() => {
    let disposed = false, timer: ReturnType<typeof setTimeout>
    const poll = async () => { try { await refresh() } catch (e) { if (!disposed) { setConnected(false); setError((e as Error).message) } } finally { if (!disposed) timer = setTimeout(poll, 2000) } }
    void poll()
    // settings.load 是异步的：跟随时它的返回值不能直接盖掉宿主明暗，否则持久化的旧值
    // 会在加载完成时把已对齐的宿主模式冲掉（表现就是宿主暗、插件模式层仍是亮）。
    void client.call<ShellSettings>('settings.load').then(loaded => setSettings(() => themeFollowRef.current === 'host' ? { ...loaded, theme: hostTheme() ?? loaded.theme } : loaded)).catch(e => setError(e.message))
    return () => { disposed = true; clearTimeout(timer) }
  }, [client, refresh])
  useEffect(() => { surface.dataset.theme = settings.theme }, [surface, settings.theme])
  // 跟随宿主明暗：初始对齐一次，之后监听宿主的运行期切换。
  // 宿主没有 data-theme（旧构建）时 hostTheme() 返回 undefined，此时保持插件自己的取值不变。
  useEffect(() => {
    themeFollowRef.current = themeFollow
    if (themeFollow !== 'host') return
    const sync = () => setSettings(previous => { const next = hostTheme(); return next && next !== previous.theme ? { ...previous, theme: next } : previous })
    sync()
    return observeHostTheme(sync)
  }, [themeFollow])
  useEffect(() => { if (error) toast.error(error) }, [error, toast])
  const settingsRuntime = useMemo(() => ({ load: () => client.call<ShellSettings>('settings.load'), save: async (next: ShellSettings) => { await client.call('settings.save', { settings: next }) } }), [client])
  const aiRuntime = useMemo<AIConfigRuntime>(() => ({ persistence: 'session', status: () => client.call('ai.status'), save: update => client.call('ai.configure', update) }), [client])
  const scriptRuntime = useMemo<ScriptRuntime>(() => {
    const replay = async (id: string, terminalId: string, values: Record<string, string> = {}) => {
      const script = await client.call('scripts.load', { id })
      if (!await confirmDialog.show({ title: '确认脚本目标', message: `将向终端 ${titles[terminalId] || terminalId} 发送脚本「${script.name}」。\n${Object.entries(values).map(([k, v]) => `${k} = ${v}`).join('\n')}\n回放会暂时接管输入。停止仅停止后续发送，不会撤回已执行命令。`, confirmText: '确认回放' })) return
      await client.call('scripts.replay.start', { id, terminalId, values, confirmed: true }); await refresh()
    }
    return {
      list: async () => (await client.call('scripts.list')).scripts ?? [], load: id => client.call('scripts.load', { id }),
      create: (name, description) => client.call('scripts.create', { name, description }), update: async script => { await client.call('scripts.update', { id: script.id, script }) },
      remove: async id => { await client.call('scripts.delete', { id }) }, replay, replayWithVars: replay,
      startRecording: (name, description, terminalId) => client.call('scripts.record.start', { name, description, terminalId }),
      stopRecording: () => client.call('scripts.record.stop'), recordingStatus: () => client.call('scripts.record.status'),
    }
  }, [client, refresh, titles])
  const send = (content: string) => { const writer = active && senders.current.get(active); if (!writer) throw new Error('请选择并附着一个终端'); writer(content) }
  const quickHost = useMemo<QuickCommandHost>(() => ({
    execute: content => { try { send(content.replace(/[\r\n]+$/, '') + '\r') } catch (e) { setError((e as Error).message) } },
    storage: { load: async () => (await client.call('quickCommands.list')).commands ?? [],
      add: command => { void act(() => client.call('quickCommands.save', command)) },
      update: (id, changes) => { void act(async () => { const old = (await client.call('quickCommands.list')).commands.find((v: any) => v.id === id); await client.call('quickCommands.save', { ...old, ...changes, id }) }) },
      remove: id => { void act(() => client.call('quickCommands.delete', { id })) }, reorder: ids => { void act(() => client.call('quickCommands.reorder', { ids })) },
    },
  }), [client, active])
  const openTerminal = async (connectionId: string, title: string) => {
    const { terminalId } = await client.call('terminals.open', { connectionId }); setTitles(v => ({ ...v, [terminalId]: title })); await refresh(); setActive(terminalId)
  }
  const closeTerminal = (terminalId: string) => void act(async () => {
    const current = snapshot.terminals.find(t => t.terminalId === terminalId)
    if (!current || !await confirmDialog.show({ message: '关闭该终端会终止其远端 Shell，是否继续？', danger: true })) return
    await client.call('terminals.close', { terminalId })
    if (!snapshot.terminals.some(t => t.connectionId === current.connectionId && t.terminalId !== terminalId)) await client.call('connections.disconnect', { connectionId: current.connectionId })
    await refresh()
  })

  const openSaved = (session: SessionNode) => {
    if (!session.config) return;
    setConnectingSavedId(session.id);
    setConnectSeed([{...session.config, name:session.name, password:''}]); setConnectModal(true);
  };
  const connectBatch = (configs: ConnectionConfig[]) => void act(async () => {
    for (const config of configs) {
      let id = connectingSavedId;
      const saved = id ? flatten(sessions).find(s => s.id === id)?.config : undefined;
      if (!id || !saved || !(saved as ConnectionConfig & { host_key?: string }).host_key || saved.host !== config.host || saved.port !== config.port || saved.user !== config.user) {
        id = (await saveConnection(config, id)).id;
      }
      const {connectionId} = await client.call('connections.connect', {id, password:config.password || undefined});
      try { await openTerminal(connectionId, config.name || config.host) }
      catch (e) { await client.call('connections.disconnect', {connectionId}); throw e }
    }
  });
  // 手动切明暗 = 显式覆盖：记成 manual，宿主之后再变也不改写；用户原来的选择照旧存进 sidecar。
  const toggleTheme = () => { const updated = {...settings, theme:settings.theme === 'dark' ? 'light' as const : 'dark' as const}; setThemeFollow('manual'); persistThemeFollow('manual'); setSettings(updated); void act(() => settingsRuntime.save(updated)) };
  const resumeHostTheme = () => { setThemeFollow('host'); persistThemeFollow('host') };
  // 设置页里改明暗同样是显式覆盖，否则下一次宿主切换会把它盖掉。
  const applySettings = (next: ShellSettings) => {
    if (next.theme !== settings.theme) { setThemeFollow('manual'); persistThemeFollow('manual') }
    setSettings(next)
  };
  // 皮肤与明暗是两个轴：换皮肤只改视觉语言来源，不动明暗，也不写后端配置。
  const changeSkin = (next: Skin) => { setSkin(next); persistSkin(next); applySkin(next, surface) };
  return <ProductFrame
    toolbar={<ProductToolbar status={connected ? '就绪' : '连接中…'} theme={settings.theme}
      onNewConnection={() => { setConnectingSavedId(undefined); setConnectSeed([]); setConnectModal(true) }}
      onThemeToggle={toggleTheme} onSettings={() => setShowSettings(true)}
      hostThemeFollow={{ following: themeFollow === 'host', onResume: resumeHostTheme }} />}
    terminal={<FlexLayoutAdapter
      terminals={snapshot.terminals.map(t => ({ id:t.terminalId, title:titles[t.terminalId] || `终端 ${t.terminalId.slice(0,8)}`, status:connected ? SessionStatus.CONNECTED : SessionStatus.DISCONNECTED }))}
      terminalRefs={terminalRefs} onTerminalData={(id,data) => senders.current.get(id)?.(data)}
      activeTerminalId={active} onActiveTerminalChange={setActive} onCloseTerminal={closeTerminal}
      onRenameTerminal={(id,title) => setTitles(v => ({...v,[id]:title}))}
      onDuplicateTerminal={id => { const current=snapshot.terminals.find(t => t.terminalId===id); if(current) void act(() => openTerminal(current.connectionId, titles[id] || '终端')) }}
      terminalRuntime={{resize:() => {}}} theme={settings.theme} terminalConfig={settings.terminal} completionDelay={settings.completionDelay} highlightRules={settings.highlightRules}
      renderTerminal={(id,attachRef) => <TeamsTerminal client={client} id={id} settings={settings} attachRef={attachRef} register={register}/>}
      renderFileTransfer={(activeTerminalId,terminals) => <FilesPanel host={fileHost} activeTerminalId={activeTerminalId} terminals={terminals}/>} />}
    quickCommands={<QuickCommandPanel host={quickHost} isOpen={quickOpen} onExecute={quickHost.execute}/>}
    sidebar={<ProductSidebar isOpen={sidebarOpen} activeTab={section} onToggle={() => setSidebarOpen(v => !v)}>
      <div style={{display:section === 'sessions' ? 'flex':'none',flex:1,flexDirection:'column',height:'100%',overflow:'hidden'}}>
        <SessionManager runtime={sessionRuntime} onConnect={config => {
          // main 的节点只回传连接配置：按端点找回对应的已保存节点，保住"连接的是这一条"的语义
          // （面板随后会带着它的 id 去保存，从而更新原连接而不是新建一条）。
          const match = flatten(sessions).find(node => node.config && node.config.host === config.host && node.config.port === config.port && node.config.user === config.user)
          openSaved(match ? { ...match, config } : { id: '', name: config.name ?? `${config.user}@${config.host}`, type: 'session', config })
        }} />
      </div>
      <div style={{display:section === 'script' ? 'flex':'none',flex:1,flexDirection:'column',height:'100%',overflow:'hidden'}}>
        <ScriptRecordingPanel runtime={scriptRuntime} activeSessionId={active} onRecordingComplete={() => scriptsRef.current?.loadScripts()}/>
        <ScriptListPanel ref={scriptsRef} runtime={scriptRuntime} activeSessionId={active} onEditScript={setScriptId} onReplayScript={id => {if(active) void act(() => scriptRuntime.replay(id,active))}}/>
        {snapshot.replays.length > 0 && <div role="status">{snapshot.replays.map(run => <div key={run.id}>{runLabels[run.state]} · {run.sent}/{run.total}{['running','stopping','unknown'].includes(run.state) && <button onClick={() => void act(() => client.call('scripts.replay.stop',{runId:run.id}))}>停止后续发送</button>}</div>)}</div>}
      </div>
    </ProductSidebar>}
    navigation={<ProductNavigation isSidebarOpen={sidebarOpen} sidebarTab={section} toggleSidebar={toggleSidebar}
      tabs={['sessions','script']} isQuickCommandOpen={quickOpen} onToggleQuickCommands={() => setQuickOpen(v => !v)} />}
    footer={<BottomBar tips={BOTTOM_BAR_TIPS.filter(t => !/Telnet|广播/.test(t))}/>}
  >
    <SmartConnectModal isOpen={connectModal} initialConfigs={connectSeed} onClose={() => setConnectModal(false)} onConnect={connectBatch}
      onParse={async input => (await client.call('ai.parse',{input})).configs ?? []}/>
    <ShellSettingsModal hostSettings={hostSettings} embedded isOpen={showSettings} onClose={() => setShowSettings(false)} runtime={settingsRuntime} initial={settings} onApply={applySettings} aiRuntime={aiRuntime}
      skin={{ value: skin, onChange: changeSkin, options: [
        { id: 'teams', label: '跟随 iCode Teams', hint: '颜色、圆角与字号取自宿主公开的样式契约，宿主换主题时自动跟随' },
        { id: 'default', label: 'OpsCopilot 默认', hint: '使用 OpsCopilot 自己的视觉语言，不跟随宿主' },
      ] }}/>
    <CommandQueryOverlay visible={commandQuery.visible} query={commandQuery.query} onQueryChange={commandQuery.setQuery} loading={commandQuery.loading} result={commandQuery.result} error={commandQuery.error}
      onGenerate={() => void commandQuery.generate()} onRegenerate={() => void commandQuery.generate()} onCopy={() => void commandQuery.copy()}
      onType={commandQuery.type} onClose={() => commandQuery.setVisible(false)}/>
    <ScriptEditorModal isOpen={Boolean(scriptId)} scriptId={scriptId ?? null} runtime={scriptRuntime} onClose={() => setScriptId(undefined)} onSave={() => scriptsRef.current?.loadScripts()}/>
  </ProductFrame>
}
