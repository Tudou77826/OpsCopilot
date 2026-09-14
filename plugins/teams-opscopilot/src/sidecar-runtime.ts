import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { createHash } from 'node:crypto'
import { mkdir, readFile, stat } from 'node:fs/promises'
import { dirname, isAbsolute, parse, resolve } from 'node:path'
import { OpsError } from './errors.js'
import { StdioRpc, type RpcEvent } from './stdio-rpc.js'
import { assertCompatible } from './compatibility.js'

export interface SidecarOptions {
  executable: string
  sha256: string
  version: string
  dataDir: string
  localMode?: boolean
  startupTimeoutMs?: number
}
type State = 'stopped' | 'starting' | 'ready' | 'stopping' | 'failed'

/** One runtime belongs to one local identity. Options come from a trusted host, not UI. */
export class SidecarRuntime {
  private state: State = 'stopped'
  private child?: ChildProcessWithoutNullStreams
  private rpc?: StdioRpc
  private starting?: Promise<void>
  private stopping?: Promise<void>
  private generation = 0
  private endpoint?: { wsBase: string; token: string }
  private observers = new Set<(event: RpcEvent) => void>()
  private closed: Promise<void> = Promise.resolve()

  private activeVersion = ""
  constructor(private readonly source: SidecarOptions | (() => Promise<SidecarOptions>)) { if (typeof source !== "function") this.activeVersion=source.version }

  status() { return { state: this.state, generation: this.generation, version: this.activeVersion, protocol: 1 } }

  start(): Promise<void> {
    if (this.stopping) return Promise.reject(new OpsError('BUSY', 'Sidecar 正在停止'))
    if (this.starting) return this.starting
    if (this.state === 'ready') return Promise.resolve()
    if (this.child) return Promise.reject(new OpsError('BUSY', '旧 Sidecar 尚未退出'))
    this.state = 'starting'
    this.starting = this.launch().finally(() => { this.starting = undefined })
    return this.starting
  }

  private async launch() {
    try {
      const options = typeof this.source === "function" ? await this.source() : this.source
      const { executable, sha256, dataDir, version } = options
      this.activeVersion=version
      if (!isAbsolute(executable) || !isAbsolute(dataDir) || resolve(dataDir) === parse(resolve(dataDir)).root || !/^[a-f0-9]{64}$/i.test(sha256) || !version) {
        throw new OpsError('INVALID_ARGUMENT', 'Sidecar 宿主资源配置无效')
      }
      if (!(await stat(executable)).isFile() || createHash('sha256').update(await readFile(executable)).digest('hex') !== sha256.toLowerCase()) {
        throw new OpsError('FORBIDDEN', 'Sidecar 构件校验失败')
      }
      await mkdir(dataDir, { recursive: true, mode: 0o700 })
      if (this.state !== 'starting') throw new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 启动已取消')
      const child = spawn(executable, [...(options.localMode ? ['--teams-plugin'] : []), '--ws-addr', '127.0.0.1:0', '--data-dir', dataDir, '--workspace-files'], {
        cwd: options.localMode ? dirname(executable) : dataDir, stdio: 'pipe', windowsHide: true, shell: false,
      })
      this.child = child
      this.generation++
      // Drain stderr without retaining or forwarding possibly sensitive output.
      child.stderr.resume()
      this.closed = new Promise(resolveClose => child.once('close', () => {
        this.rpc?.close(); this.rpc = undefined; this.endpoint = undefined; this.child = undefined
        if (this.state !== 'stopping') this.state = 'failed'
        this.emit({ method: 'runtime/exited', params: { generation: this.generation } })
        resolveClose()
      }))
      const rpc = new StdioRpc(child.stdin, child.stdout, () => {
        if (this.state !== 'stopping') this.state = 'failed'
        this.endpoint = undefined; child.kill()
      })
      this.rpc = rpc
      rpc.subscribe(event => this.emit(event))
      child.once('error', () => { this.state = 'failed'; rpc.close() })
      const init = await rpc.call<{ protocol: number; version: string; wsBase: string; token: string }>('initialize', {}, options.startupTimeoutMs ?? 5000)
      if (options.localMode) assertCompatible(init)
      let url: URL
      try { url = new URL(init.wsBase) } catch { throw new OpsError('PROTOCOL_MISMATCH', 'Sidecar 数据通道无效') }
      if (init.protocol !== 1 || init.version !== version || url.protocol !== 'ws:' || url.hostname !== '127.0.0.1' || !url.port || url.username || url.password || url.search || url.hash || url.pathname !== '/' || typeof init.token !== 'string' || !init.token) {
        throw new OpsError('PROTOCOL_MISMATCH', 'Sidecar 版本或通道不兼容')
      }
      if (this.state !== 'starting') throw new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 启动已取消')
      this.endpoint = { wsBase: init.wsBase, token: init.token }
      this.state = 'ready'
    } catch (error) {
      this.rpc?.close(); this.child?.kill(); await this.closed
      if (this.state !== 'stopping') this.state = 'failed'
      throw error instanceof OpsError ? error : new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 启动失败')
    }
  }

  /** Private host transport: callers must enforce operation and resource authorization. */
  request<T = unknown>(method: string, params: unknown = {}, timeoutMs?: number): Promise<T> {
    if (this.state !== 'ready' || !this.rpc) return Promise.reject(new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 尚未就绪'))
    return this.rpc.call<T>(method, params, timeoutMs)
  }

  /** Host-only; never include this result in a public RPC response. */
  workspaceTarget(path: string) {
    if (!this.endpoint || this.state !== 'ready') throw new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 尚未就绪')
    const url = new URL('/workspace', this.endpoint.wsBase.replace('ws:', 'http:'))
    url.searchParams.set('path', path)
    return { target: url.href, headers: { authorization: `Bearer ${this.endpoint.token}` } }
  }

  /** Host-only; never include this result in a public RPC response. */
  terminalTarget(terminalId: string) {
    if (!this.endpoint || this.state !== 'ready') throw new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 尚未就绪')
    return `${this.endpoint.wsBase}/terminals/${encodeURIComponent(terminalId)}?token=${encodeURIComponent(this.endpoint.token)}`
  }

  subscribe(observer: (event: RpcEvent) => void) { this.observers.add(observer); return () => { this.observers.delete(observer) } }
  private emit(event: RpcEvent) { for (const observer of this.observers) { try { observer(event) } catch { /* isolate observers */ } } }

  stop(): Promise<void> {
    if (this.stopping) return this.stopping
    this.state = 'stopping'; this.endpoint = undefined; this.rpc?.close()
    const child = this.child
    // Close the pipe handle after pending bytes flush, so EOF does not depend
    // on platform-specific writable half-close behavior.
    child?.stdin.end(() => child.stdin.destroy())
    const timer = setTimeout(() => child?.kill('SIGKILL'), 1500)
    this.stopping = (async () => {
      await this.starting?.catch(() => {})
      await this.closed
      this.state = 'stopped'
    })().finally(() => { clearTimeout(timer); this.stopping = undefined })
    return this.stopping
  }
}
