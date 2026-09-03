import { createHash, randomUUID } from 'node:crypto'
import type { Context } from '@deepseek-ai/cordis'
import type { LocalInstallation } from './local-installation.js'
import { SidecarRuntime } from './sidecar-runtime.js'
import { OpsError } from './errors.js'
import { TerminalAccess, type BrowserSession } from './terminal-access.js'
import { trustedBrowserSession } from './host-context.js'
import { scriptChanges, replayValues, activeReplay, type ReplayState } from './script-contract.js'
import { EventJournal } from './events.js'
import { connectionDraft, sessionPassword } from './connections.js'
import { workspacePath, remotePath, type Transfer } from './files.js'

export const version = '0.1.0'
type ObjectValue = Record<string, unknown>
function object(value: unknown): ObjectValue {
  if (!value || typeof value !== 'object' || Array.isArray(value)) throw new OpsError('INVALID_ARGUMENT', '需要对象参数')
  return value as ObjectValue
}
function text(value: unknown, max = 512): string {
  if (typeof value !== 'string' || !value.trim() || value.length > max || value.includes('\0')) throw new OpsError('INVALID_ARGUMENT', '字符串参数无效')
  return value
}
function id(value: unknown) {
  const result = text(value, 128)
  if (!/^[a-zA-Z0-9_-]+$/.test(result)) throw new OpsError('INVALID_ARGUMENT', '资源标识无效')
  return result
}
function integer(value: unknown, fallback: number, max: number) {
  const result = value ?? fallback
  if (!Number.isInteger(result) || Number(result) < 1 || Number(result) > max) throw new OpsError('INVALID_ARGUMENT', '数值参数无效')
  return Number(result)
}
interface SavedSession { id: string; name: string; type: string; config?: Record<string, any>; children?: SavedSession[] }
function flatten(nodes: SavedSession[]): SavedSession[] { return nodes.flatMap(node => [node, ...flatten(node.children ?? [])]) }
function publicSessions(nodes: SavedSession[]): unknown[] {
  return nodes.map(node => ({
    id: node.id, name: node.name, type: node.type,
    ...(node.config ? { config: { host: node.config.host, port: node.config.port, user: node.config.user, protocol: node.config.protocol, host_key: node.config.host_key },
      hasSecret: Boolean(node.config.password || node.config.rootPassword || node.config.bastion) } : {}),
    ...(node.children ? { children: publicSessions(node.children) } : {}),
  }))
}

/** Local business boundary. Browser authentication is a separate, still-required host gate. */
export class OpsBusiness {
  readonly version = version
  private connections = new Set<string>()
  private terminals = new Map<string, string>()
  private pending = Promise.resolve<unknown>(undefined)
  private cache = new Map<string, { hash: string; result: Promise<unknown> }>()
  private queued = 0
  private choosing = false
  private disposed = false
  private draining = false
  private recordingTerminal?: string
  private replays = new Map<string, { status: ReplayState; release: () => void }>()
  private readonly unsubscribe: () => void
  private readonly terminalAccess: TerminalAccess
  private readonly events: EventJournal
  private hostChallenges = new Map<string, { host: string; port: number; key: string; expiresAt: number }>()
  private transfers = new Map<string, Transfer>()
  private uncertainTransfers = 0
  private fileStreams = 0

  constructor(private readonly runtime: SidecarRuntime, private readonly identityId: string, private readonly installation?: LocalInstallation) {
    this.events = new EventJournal(() => runtime.status().generation)
    this.terminalAccess = new TerminalAccess(() => ({ generation: runtime.status().generation, ready: !this.disposed && runtime.status().state === 'ready' }), terminalId => this.terminals.has(terminalId))
    this.unsubscribe = runtime.subscribe(event => {
      if (event.method === 'runtime/exited') {
        this.hostChallenges.clear()
        this.uncertainTransfers = 0
        for (const task of this.transfers.values()) if (task.state === 'running') task.state = 'interrupted'
        this.terminalAccess.invalidate(); this.connections.clear(); this.terminals.clear(); this.cache.clear(); this.recordingTerminal = undefined
        for (const run of this.replays.values()) if (activeReplay(run.status.state)) { run.status.state = 'interrupted'; run.release() }
        this.events.emit('runtime.interrupted')
      }
      if (event.method === 'terminal/exited') {
        const terminal = (event.params as { terminalId?: string })?.terminalId
        if (terminal && this.terminals.has(terminal)) { this.terminalAccess.revokeTerminal(terminal); this.terminals.delete(terminal); this.events.emit('terminal.exited', terminal) }
      }
      if (event.method === 'shell.ft/progress' || event.method === 'shell.ft/done') {
        const value = event.params as any
        if (!value || !this.terminals.has(value.sessionId) || typeof value.taskId !== 'string') return
        const done = event.method === 'shell.ft/done', old = this.transfers.get(value.taskId)
        this.transfers.set(value.taskId, { taskId: value.taskId, sessionId: value.sessionId,
          state: done ? value.ok ? 'completed' : value.cancelled ? 'cancelled' : 'failed' : 'running',
          bytesDone: Math.max(0, Number(done ? value.bytes : value.bytesDone) || 0), bytesTotal: Math.max(0, Number(value.bytesTotal ?? old?.bytesTotal) || 0),
          ...(done ? { ok: value.ok === true, cancelled: value.cancelled === true, message: value.ok ? '传输完成' : value.cancelled ? '已取消，目标可能存在部分内容' : '传输失败，目标可能存在部分内容' } : {}),
        })
        if (this.transfers.size > 128) { const first = [...this.transfers].find(([, t]) => t.state !== 'running'); if (first) this.transfers.delete(first[0]) }
        this.events.emit(done ? 'transfer.finished' : 'transfer.progress', value.taskId)
      }
    })
  }
  async health() {
    if (this.disposed || (!this.installation && this.runtime.status().state !== 'ready')) throw new OpsError('RUNTIME_UNAVAILABLE', 'Ops 运行时不可用')
    return { healthy: true }
  }
  activeResources() { return { connections: this.connections.size, terminals: this.terminals.size } }
  activity() { return { ...this.activeResources(), recordings: this.recordingTerminal ? 1 : 0, replays: [...this.replays.values()].filter(run => activeReplay(run.status.state)).length, transfers: this.uncertainTransfers + this.fileStreams + [...this.transfers.values()].filter(t => t.state === 'running').length } }
  beginFileStream(path: string) {
    if (this.disposed || this.draining) throw new OpsError('BUSY', '插件正在停止或更新')
    if (this.fileStreams >= 4) throw new OpsError('BUSY', '文件流数量已达上限')
    const target = this.runtime.workspaceTarget(workspacePath(path))
    this.fileStreams++
    let closed = false
    return { ...target, close: () => { if (!closed) { closed = true; this.fileStreams-- } } }
  }
  /** Atomic host-only admission barrier; checking activity alone is insufficient. */
  beginDrain() {
    if (this.disposed || this.draining) throw new OpsError('BUSY', '插件正在停止或更新')
    if (this.queued || Object.values(this.activity()).some(count => count > 0)) throw new OpsError('BUSY', '请先结束连接、终端、录制和回放，再更新或禁用插件')
    this.draining = true
    return { resume: () => { if (!this.disposed) this.draining = false } }
  }
  dispose() { this.disposed = true; this.terminalAccess.invalidate(); this.unsubscribe(); this.connections.clear(); this.terminals.clear(); this.cache.clear(); this.replays.clear(); this.recordingTerminal = undefined }

  /** Used only by the versioned host terminal transport, not arbitrary call payloads. */
  issueTerminalTicket(terminalId: string, viewId: string, session: BrowserSession, takeover = false) {
    return this.terminalAccess.issue(this.ownedTerminal(terminalId), viewId, session, takeover)
  }

  /** Target contains the Sidecar secret. The trusted relay must never serialize it to UI. */
  consumeTerminalTicket(ticket: string, terminalId: string, session: BrowserSession, onRevoke: () => void) {
    const lease = this.terminalAccess.consume(ticket, terminalId, session, onRevoke)
    try { return { ...lease, target: this.runtime.terminalTarget(terminalId) } }
    catch (error) { lease.close(); throw error }
  }

  call(input: unknown, callerIdentity: string, browserSession?: BrowserSession): Promise<unknown> {
    try {
      if (callerIdentity !== this.identityId) throw new OpsError('FORBIDDEN', '不可访问其他客户端身份的 Ops 实例')
      if (this.disposed) throw new OpsError('RUNTIME_UNAVAILABLE', 'Ops 实例已卸载')
      if (this.draining) throw new OpsError('BUSY', '插件正在停止或更新')
      const session = browserSession && trustedBrowserSession({ protocol: 1, session: browserSession })
      const request = object(input)
      if (typeof request.operation === 'string' && request.operation.startsWith('installation.') && !session) throw new OpsError('FORBIDDEN', '本地程序设置需要已认证的客户端会话')
      if (request.schemaVersion !== 1) throw new OpsError('INVALID_ARGUMENT', '不支持的插件协议版本')
      const requestId = id(request.requestId)
      const operation = text(request.operation, 80)
      const payload = object(request.payload ?? {})
      const encoded = JSON.stringify({ operation, payload })
      if (Buffer.byteLength(encoded) > 256 * 1024) throw new OpsError('INVALID_ARGUMENT', '请求过大')
      const hash = createHash('sha256').update(encoded).digest('hex')
      const cacheKey = `${session?.sessionId ?? 'local'}:${requestId}`
      const previous = this.cache.get(cacheKey)
      if (previous) {
        if (previous.hash !== hash) throw new OpsError('INVALID_ARGUMENT', '请求标识已用于不同操作')
        return previous.result
      }
      if (this.queued >= 64) throw new OpsError('BUSY', 'Ops 请求队列已满')
      this.queued++
      const generation = this.runtime.status().generation
      // Detach queued inputs from a caller's mutable object, never log them.
      const copied = JSON.parse(encoded)
      const choose = operation === 'installation.choose'
      if (choose && this.choosing) { this.queued--; throw new OpsError('BUSY', '文件选择框已打开，请先完成或取消选择') }
      if (choose) this.choosing = true
      // Native dialogs must not hold the business mutation queue. Keep their
      // admission count so unload/update still waits for the dialog to finish.
      const result = (choose ? Promise.resolve() : this.pending).then(() => {
        if (this.disposed) throw new OpsError('RUNTIME_UNAVAILABLE', 'Ops 实例已卸载')
        if (session && session.expiresAt <= Date.now()) throw new OpsError('FORBIDDEN', '浏览器会话已过期')
        if (this.runtime.status().generation !== generation) throw new OpsError('RUNTIME_UNAVAILABLE', '运行时已更换，请重新确认操作')
        return this.dispatch(copied.operation, copied.payload, generation)
      })
      const settled = result.then(() => undefined, () => undefined).finally(() => { this.queued--; if (choose) this.choosing = false })
      if (!choose) this.pending = settled
      this.cache.set(cacheKey, { hash, result })
      // Last 256 requests per process, including indeterminate failures. No automatic retry.
      if (this.cache.size > 256) this.cache.delete(this.cache.keys().next().value!)
      return result
    } catch (error) {
      return Promise.reject(error instanceof OpsError ? error : new OpsError('INVALID_ARGUMENT', '请求无法解析'))
    }
  }

  private async dispatch(operation: string, payload: ObjectValue, generation: number): Promise<unknown> {
    const ensureCurrent = () => {
      if (this.disposed || this.runtime.status().generation !== generation || this.runtime.status().state !== 'ready') {
        throw new OpsError('RUNTIME_UNAVAILABLE', '运行时已中断，请重新确认操作')
      }
    }
    const rpc = async <T = any>(method: string, params?: unknown): Promise<T> => {
      ensureCurrent()
      const result = await this.runtime.request<T>(method, params)
      // A multi-RPC operation must not continue on a replacement process or
      // repopulate cleared resource maps from an old process's late response.
      ensureCurrent()
      return result
    }
    switch (operation) {
      case 'installation.status': return { ...(this.installation?.status() ?? { configured:true, executable:'', message:'' }), state:this.runtime.status().state }
      case 'installation.choose':
        if(!this.installation) throw new OpsError('UNSUPPORTED_CAPABILITY','当前宿主不支持选择本地安装')
        return this.installation.choose()
      case 'installation.configure': case 'installation.start': {
        if(!this.installation) throw new OpsError('UNSUPPORTED_CAPABILITY','当前宿主不支持本地安装')
        if(Object.values(this.activity()).some(count=>count>0)) throw new OpsError('BUSY','请先结束活动连接、录制、回放和文件传输，再更换本地程序')
        if(operation==='installation.configure'){
          if(payload.confirmed!==true) throw new OpsError('FORBIDDEN','请确认启动所选的本地 Ops 程序')
          // Validate and save before stopping the previous idle runtime.
          await this.installation.configure(text(payload.executable,4096))
        }
        await this.runtime.stop()
        try { await this.runtime.start() } catch(error){this.installation.failed(error);throw error}
        return { ...this.installation.status(),state:this.runtime.status().state }
      }
      case 'runtime.status': return { ...this.runtime.status(), ...this.activity(), capabilities: ['connections', 'terminal-control', 'remote-file-list', 'script-library', 'script-recording', 'script-replay'] }
      case 'runtime.events': return this.events.since(payload)
      case 'runtime.snapshot': {
        for (const [runId, run] of this.replays) {
          if (activeReplay(run.status.state)) {
            try { this.updateReplay(runId, await rpc('shell.script.replayStatus', { runId })) }
            catch (error) { ensureCurrent(); if (!(error instanceof OpsError) || error.code !== 'OPERATION_FAILED') throw error }
          }
        }
        const recording = this.recordingTerminal ? { terminalId: this.recordingTerminal, ...await rpc('shell.script.status') } : { is_recording: false }
        return { schemaVersion: 1, ...this.runtime.status(), ...this.events.cursor(), ...this.activity(), terminals: [...this.terminals].map(([terminalId, connectionId]) => ({ terminalId, connectionId })), recording,
          replays: [...this.replays.values()].map(run => ({ ...run.status })), transfers: [...this.transfers.values()] }
      }
      case 'connections.list': return { sessions: publicSessions((await rpc('shell.configs.list')).sessions ?? []) }
      case 'connections.probe': {
        const draft = connectionDraft(payload.config)
        const result = await rpc<{ key: string; fingerprint: string; algorithm: string }>('shell.hostKey.probe', { host: draft.host, port: draft.port })
        for (const [key, value] of this.hostChallenges) if (value.expiresAt < Date.now()) this.hostChallenges.delete(key)
        if (this.hostChallenges.size >= 128) throw new OpsError('BUSY', '待确认主机过多')
        const challengeId = randomUUID(), expiresAt = Date.now() + 120000
        this.hostChallenges.set(challengeId, { host: draft.host, port: draft.port, key: result.key, expiresAt })
        return { challengeId, expiresAt, fingerprint: result.fingerprint, algorithm: result.algorithm }
      }
      case 'connections.save': {
        const draft = connectionDraft(payload.config), challenge = this.hostChallenges.get(id(payload.challengeId))
        if (payload.confirmed !== true || !challenge || challenge.host !== draft.host || challenge.port !== draft.port || challenge.expiresAt < Date.now()) throw new OpsError('FORBIDDEN', '请先核对并确认主机密钥')
        this.hostChallenges.delete(String(payload.challengeId))
        // Deliberately never persist a password or root password from the UI.
        const config = { ...draft, host_key: challenge.key, password: '', root_password: '', bastion: null }
        if (payload.id !== undefined) {
          await rpc('shell.configs.update', { id: id(payload.id), config, group: draft.group })
          return { id: payload.id }
        }
        return rpc('shell.configs.save', config)
      }
      case 'connections.delete': return rpc('shell.configs.delete', { id: id(payload.id) })
      case 'connections.rename': return rpc('shell.configs.rename', { id: id(payload.id), name: text(payload.name, 128) })
      case 'connections.folder': return rpc('shell.configs.createFolder', { name: text(payload.name, 128) })
      case 'connections.connect': {
        const configId = id(payload.id)
        const sessions = (await rpc('shell.configs.list')).sessions ?? []
        const saved = flatten(sessions).find(node => node.id === configId)?.config
        if (!saved) throw new OpsError('NOT_FOUND', '连接配置不存在')
        if (!saved.host_key) throw new OpsError('FORBIDDEN', '此连接尚未确认主机密钥，请编辑连接后核对指纹')
        const result = await rpc<{ connectionId: string }>('shell.connect', { config: { ...saved, password: sessionPassword(payload.password) ?? saved.password } })
        this.connections.add(result.connectionId)
        this.events.emit('connection.opened', result.connectionId)
        return result
      }
      case 'connections.disconnect': {
        const connectionId = id(payload.connectionId)
        if (!this.connections.has(connectionId)) throw new OpsError('NOT_FOUND', '连接不属于本实例')
        for (const [terminal, owner] of this.terminals) if (owner === connectionId) this.requireIdleTerminal(terminal)
        await rpc('shell.disconnect', { connectionId }); this.connections.delete(connectionId)
        for (const [terminal, owner] of this.terminals) if (owner === connectionId) { this.terminalAccess.revokeTerminal(terminal); this.terminals.delete(terminal) }
        this.events.emit('connection.closed', connectionId)
        return {}
      }
      case 'terminals.open': {
        const connectionId = id(payload.connectionId)
        if (!this.connections.has(connectionId)) throw new OpsError('NOT_FOUND', '连接不属于本实例')
        const result = await rpc<{ terminalId: string }>('shell.openTerminal', { connectionId, cols: integer(payload.cols, 80, 1000), rows: integer(payload.rows, 24, 1000) })
        this.terminals.set(result.terminalId, connectionId)
        this.events.emit('terminal.opened', result.terminalId, { connectionId })
        return result
      }
      case 'terminals.list': return { terminals: [...this.terminals].map(([terminalId, connectionId]) => ({ terminalId, connectionId, generation: this.runtime.status().generation })) }
      case 'terminals.close': {
        const terminalId = this.ownedTerminal(payload.terminalId)
        this.requireIdleTerminal(terminalId)
        await rpc('shell.closeTerminal', { terminalId }); this.terminalAccess.revokeTerminal(terminalId); this.terminals.delete(terminalId); this.events.emit('terminal.closed', terminalId); return {}
      }
      case 'terminals.resize': return rpc('shell.resize', { terminalId: this.ownedTerminal(payload.terminalId), cols: integer(payload.cols, 80, 1000), rows: integer(payload.rows, 24, 1000) })
      case 'completion': return rpc('shell.completion', { input: typeof payload.input === 'string' ? payload.input.slice(0, 8192) : '', cursor: Math.max(0, Math.min(8192, Number(payload.cursor) || 0)) })
      case 'settings.load': return rpc('shell.settings.get')
      case 'settings.save': {
        const settings = object(payload.settings), terminal = object(settings.terminal)
        if (!['light', 'dark'].includes(String(settings.theme)) || !Array.isArray(settings.highlightRules) || settings.highlightRules.length > 128) throw new OpsError('INVALID_ARGUMENT', '设置无效')
        return rpc('shell.settings.save', { theme: settings.theme, terminal: {
          scrollback: integer(terminal.scrollback, 5000, 100000), search_enabled: terminal.search_enabled === true,
          highlight_enabled: terminal.highlight_enabled === true, font_family: text(terminal.font_family, 64), font_size: integer(terminal.font_size, 14, 32),
        }, completionDelay: Math.min(2000, Math.max(0, Number(settings.completionDelay) || 0)), highlightRules: settings.highlightRules,
        revision: typeof settings.revision === 'string' ? settings.revision : undefined, commandQueryShortcut: text(settings.commandQueryShortcut || 'Ctrl+K', 64) })
      }
      case 'quickCommands.list': return rpc('shell.quickcmds.list')
      case 'quickCommands.save': return rpc('shell.quickcmds.save', { id: payload.id === undefined ? '' : id(payload.id), name: text(payload.name, 128), content: text(payload.content, 65536), group: typeof payload.group === 'string' ? payload.group.slice(0, 128) : '' })
      case 'quickCommands.delete': return rpc('shell.quickcmds.delete', { id: id(payload.id) })
      case 'quickCommands.reorder': {
        if (!Array.isArray(payload.ids) || payload.ids.length > 1000) throw new OpsError('INVALID_ARGUMENT', '排序无效')
        return rpc('shell.quickcmds.reorder', { ids: payload.ids.map(id) })
      }
      case 'ai.status': return rpc('shell.ai.getConfig')
      case 'ai.configure': {
        const baseURL = text(payload.baseURL, 2048)
        let url: URL
        try { url = new URL(baseURL) } catch { throw new OpsError('INVALID_ARGUMENT', '模型地址无效') }
        if (url.username || url.password || (url.protocol !== 'https:' && !(url.protocol === 'http:' && ['127.0.0.1', 'localhost', '[::1]'].includes(url.hostname)))) throw new OpsError('INVALID_ARGUMENT', '模型地址需要 HTTPS（回环模型允许 HTTP）')
        return rpc('shell.ai.sessionConfig', { apiKey: sessionPassword(payload.apiKey) ?? '', baseURL, fastModel: text(payload.fastModel, 128), complexModel: text(payload.complexModel || payload.fastModel, 128) })
      }
      case 'ai.generate': return rpc('shell.ai.generateCommand', { request: text(payload.query, 8192) })
      case 'ai.parse': return rpc('shell.ai.parseIntent', { input: text(payload.input, 8192) })
      case 'files.list': return rpc('shell.ft.list', { terminalId: this.ownedTerminal(payload.terminalId), remotePath: text(payload.path ?? '.', 4096) })
      case 'files.check': return rpc('shell.ft.check', { terminalId: this.ownedTerminal(payload.terminalId) })
      case 'files.stat': case 'files.mkdir': case 'files.remove': case 'files.readFile': case 'files.writeFile': {
        const method = operation.slice(6), path = remotePath(payload.path, ['remove', 'writeFile'].includes(method))
        if (method === 'remove' && payload.confirmed !== true) throw new OpsError('FORBIDDEN', '请确认删除目标')
        if (method === 'writeFile' && (payload.confirmed !== true || typeof payload.content !== 'string' || Buffer.byteLength(payload.content) > 128 * 1024)) throw new OpsError('INVALID_ARGUMENT', '文本过大或未确认覆盖')
        return rpc(`shell.ft.${method}`, { terminalId: this.ownedTerminal(payload.terminalId), remotePath: path, maxBytes: 128 * 1024, ...(method === 'writeFile' ? { content: payload.content } : {}) })
      }
      case 'files.rename': return rpc('shell.ft.rename', { terminalId: this.ownedTerminal(payload.terminalId), oldPath: remotePath(payload.oldPath, true), newPath: remotePath(payload.newPath, true) })
      case 'files.upload': case 'files.download': {
        const terminalId = this.ownedTerminal(payload.terminalId), localPath = workspacePath(payload.localPath), path = remotePath(payload.remotePath, true)
        if (payload.confirmed !== true) throw new OpsError('FORBIDDEN', '请确认传输目标与覆盖行为')
        if (this.activity().transfers >= 16) throw new OpsError('BUSY', '传输任务数量已达上限')
        this.uncertainTransfers++
        try {
          const result = await rpc(`shell.ft.${operation.slice(6)}`, { terminalId, localPath, remotePath: path })
          this.uncertainTransfers--
          if (result.ok && !this.transfers.has(result.taskId)) this.transfers.set(result.taskId, { taskId: result.taskId, sessionId: terminalId, state: 'running', bytesDone: 0, bytesTotal: 0 })
          return result
        } catch (error) { if (error instanceof OpsError && error.code !== 'TIMEOUT' && this.uncertainTransfers > 0) this.uncertainTransfers--; throw error }
      }
      case 'files.cancel': {
        const taskId = id(payload.taskId), task = this.transfers.get(taskId)
        if (!task) throw new OpsError('NOT_FOUND', '传输不属于本实例')
        return task.state === 'running' ? rpc('shell.ft.cancel', { taskId }) : { ok: true }
      }
      case 'workspace.list': case 'workspace.stat': case 'workspace.mkdir': case 'workspace.remove': {
        const path = workspacePath(payload.path), method = operation.slice(10)
        if (method === 'remove' && (payload.confirmed !== true || ['/', '.', ''].includes(path))) throw new OpsError('FORBIDDEN', '请确认删除文件，不能删除文件区本身')
        return rpc(`shell.fs.${method}`, { path })
      }
      case 'workspace.rename': return rpc('shell.fs.rename', { oldPath: workspacePath(payload.oldPath), newPath: workspacePath(payload.newPath) })
      case 'scripts.list': return rpc('shell.script.list')
      case 'scripts.create': return rpc('shell.script.create', { name: text(payload.name, 128), description: typeof payload.description === 'string' ? payload.description.slice(0, 4096) : '' })
      case 'scripts.load': return rpc('shell.script.load', { id: id(payload.id) })
      case 'scripts.update': {
        const scriptId = id(payload.id), changes = scriptChanges(payload.script)
        const saved = await rpc('shell.script.load', { id: scriptId })
        if (object(payload.script).updated_at !== saved.updated_at) throw new OpsError('OPERATION_FAILED', '脚本已被其他窗口修改，请重新打开后再保存')
        return rpc('shell.script.update', { ...saved, ...changes, id: scriptId })
      }
      case 'scripts.delete': {
        const scriptId = id(payload.id)
        if ([...this.replays.values()].some(run => run.status.scriptId === scriptId && activeReplay(run.status.state))) throw new OpsError('BUSY', '请先停止脚本回放')
        return rpc('shell.script.delete', { id: scriptId })
      }
      case 'scripts.record.start': {
        const terminalId = this.ownedTerminal(payload.terminalId)
        const name = text(payload.name, 128)
        if (this.recordingTerminal || this.activity().replays) throw new OpsError('BUSY', '请先结束当前录制或回放')
        this.recordingTerminal = terminalId
        try {
          const result = await rpc('shell.script.startRecording', { terminalId, name, description: typeof payload.description === 'string' ? payload.description.slice(0, 4096) : '' })
          this.events.emit('recording.started', terminalId)
          return result
        }
        catch (error) {
          // A timeout is indeterminate; retain activity protection until explicit stop/status.
          if (error instanceof OpsError && error.code !== 'TIMEOUT') this.recordingTerminal = undefined
          throw error
        }
      }
      case 'scripts.record.stop': {
        if (!this.recordingTerminal) throw new OpsError('NOT_FOUND', '没有活动录制')
        const saved = await rpc('shell.script.stopRecording'); this.events.emit('recording.stopped', this.recordingTerminal); this.recordingTerminal = undefined; return saved
      }
      case 'scripts.record.status': {
        const status = await rpc('shell.script.status')
        // An earlier start may still be in flight after a timeout. A negative
        // observation alone cannot release its protection.
        return status
      }
      case 'scripts.replay.start': {
        const terminalId = this.ownedTerminal(payload.terminalId), scriptId = id(payload.id), values = replayValues(payload.values)
        if (payload.confirmed !== true) throw new OpsError('INVALID_ARGUMENT', '请确认目标终端与脚本变量后回放')
        if (this.recordingTerminal) throw new OpsError('BUSY', '请先结束录制')
        const runId = randomUUID(), release = this.terminalAccess.acquireExclusive(terminalId)
        if (this.replays.size >= 128) {
          const completed = [...this.replays].find(([, run]) => !activeReplay(run.status.state))
          if (completed) this.replays.delete(completed[0])
          else { release(); throw new OpsError('BUSY', '回放任务数量已达上限') }
        }
        this.replays.set(runId, { status: { id: runId, scriptId, terminalId, state: 'unknown', sent: 0, total: 0 }, release })
        try { return this.updateReplay(runId, await rpc('shell.script.replayStart', { runId, id: scriptId, terminalId, values })) }
        catch (error) {
          if (error instanceof OpsError && error.code === 'OPERATION_FAILED') { release(); this.replays.delete(runId) }
          throw error
        }
      }
      case 'scripts.replay.status':
      case 'scripts.replay.stop': {
        const runId = id(payload.runId), run = this.replays.get(runId)
        if (!run) throw new OpsError('NOT_FOUND', '回放任务不属于本实例')
        if (!activeReplay(run.status.state)) return { ...run.status }
        return this.updateReplay(runId, await rpc(operation === 'scripts.replay.stop' ? 'shell.script.replayStop' : 'shell.script.replayStatus', { runId }))
      }
      // Config writes and local files remain closed pending secure secret storage
      // and filesystem grants. No arbitrary Sidecar RPC method is exposed.
      default: throw new OpsError('UNSUPPORTED_CAPABILITY', '该操作尚未接入 Teams 插件')
    }
  }
  private updateReplay(runId: string, status: ReplayState) {
    const run = this.replays.get(runId)
    if (!run || status.id !== runId || status.terminalId !== run.status.terminalId || status.scriptId !== run.status.scriptId || !['running', 'stopping', 'dispatched', 'stopped', 'failed'].includes(status.state) || !Number.isSafeInteger(status.sent) || !Number.isSafeInteger(status.total) || status.sent < 0 || status.total < status.sent || status.total > 1000) {
      throw new OpsError('PROTOCOL_MISMATCH', '回放状态不匹配')
    }
    if (run.status.state !== status.state || run.status.sent !== status.sent) this.events.emit('replay.changed', runId, { terminalId: status.terminalId, state: status.state, sent: status.sent, total: status.total })
    run.status = { id: runId, scriptId: status.scriptId, terminalId: status.terminalId, state: status.state, sent: status.sent, total: status.total }
    if (!activeReplay(status.state)) run.release()
    return { ...run.status }
  }
  private requireIdleTerminal(terminalId: string) {
    if (this.uncertainTransfers || [...this.transfers.values()].some(t => t.sessionId === terminalId && t.state === 'running')) throw new OpsError('BUSY', '请先等待或取消该终端上的文件传输')
    if (this.recordingTerminal === terminalId || [...this.replays.values()].some(run => run.status.terminalId === terminalId && activeReplay(run.status.state))) {
      throw new OpsError('BUSY', '请先结束该终端上的录制或脚本回放')
    }
  }
  private ownedTerminal(value: unknown) {
    const terminal = id(value)
    if (!this.terminals.has(terminal)) throw new OpsError('NOT_FOUND', '终端不属于本实例')
    return terminal
  }
}

/** Composition entry for a trusted host/test harness, not yet a distributable FeatureBundle. */
export function createOpsPlugin(runtime: SidecarRuntime, identityId: string, options: { terminalTransport?: boolean; installation?: LocalInstallation } = {}) {
  return async function opsPlugin(ctx: Context) {
    const business = new OpsBusiness(runtime, identityId, options.installation)
    ctx.effect(() => async () => { business.dispose(); options.installation?.dispose(); await runtime.stop() })
    if (options.installation) { try { if(options.installation.status().configured) await runtime.start() } catch(error) { options.installation.failed(error) } }
    else await runtime.start()
    ctx.provide('opscopilot', {
      version, health: () => business.health(),
      lifecycle: { protocol: 1, beginDrain: () => business.beginDrain() },
      files: { protocol: 1, open: (method: string, path: string, context: unknown) => {
        const session = trustedBrowserSession(context), generation = runtime.status().generation
        if (method !== 'GET' && method !== 'PUT') throw new OpsError('FORBIDDEN', '文件方法无效')
        return { ...business.beginFileStream(path), canUse: () => session.expiresAt > Date.now() && runtime.status().state === 'ready' && runtime.status().generation === generation }
      } },
      call: (input: unknown, context?: unknown) => business.call(input, identityId, context === undefined ? undefined : trustedBrowserSession(context)),
      // Explicit integration opt-in until distribution and update guards are complete.
      ...(options.terminalTransport ? { terminal: {
        protocol: 1,
        issue: (terminalId: string, input: unknown, context: unknown) => {
          const session = trustedBrowserSession(context), payload = object(input)
          return business.issueTerminalTicket(terminalId, text(payload.viewId, 128), session, payload.takeover === undefined ? false : payload.takeover as boolean)
        },
        consume: (ticket: string, terminalId: string, context: unknown, onRevoke: () => void) => business.consumeTerminalTicket(ticket, terminalId, trustedBrowserSession(context), onRevoke),
      } } : {}),
    })
  }
}
