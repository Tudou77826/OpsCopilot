import type { Readable, Writable } from 'node:stream'
import { OpsError } from './errors.js'

type Pending = { resolve(value: unknown): void; reject(error: Error): void; timer: NodeJS.Timeout }
export interface RpcEvent { method: string; params: unknown }

/** Internal transport only. Never expose call(method, params) to a browser. */
export class StdioRpc {
  private buffer = Buffer.alloc(0)
  private pending = new Map<number, Pending>()
  private sequence = 0
  private stopped = false
  private listeners = new Set<(event: RpcEvent) => void>()
  private readonly onData = (chunk: Buffer) => this.consume(chunk)
  private readonly onEnd = () => this.fail(new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 控制通道已关闭'))
  private readonly onError = () => this.fail(new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 控制通道异常'))

  constructor(private readonly input: Writable, private readonly output: Readable,
    private readonly fatal: (error: OpsError) => void = () => {},
    private readonly maxLineBytes = 1024 * 1024, private readonly maxPending = 128) {
    output.on('data', this.onData).on('end', this.onEnd).on('error', this.onError)
    input.on('error', this.onError)
  }

  subscribe(listener: (event: RpcEvent) => void) {
    this.listeners.add(listener)
    return () => { this.listeners.delete(listener) }
  }

  call<T = unknown>(method: string, params: unknown = {}, timeoutMs = 10_000): Promise<T> {
    if (this.stopped) return Promise.reject(new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 控制通道已关闭'))
    if (this.pending.size >= this.maxPending) return Promise.reject(new OpsError('BUSY', 'Sidecar 请求队列已满'))
    if (!Number.isFinite(timeoutMs) || timeoutMs <= 0) return Promise.reject(new OpsError('INVALID_ARGUMENT', '请求超时参数无效'))
    const id = ++this.sequence
    let line: string
    try { line = JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n' }
    catch { return Promise.reject(new OpsError('INVALID_ARGUMENT', '请求无法编码')) }
    if (Buffer.byteLength(line) > this.maxLineBytes) return Promise.reject(new OpsError('INVALID_ARGUMENT', '请求超过控制通道大小限制'))
    return new Promise<T>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id)
        // A timeout is not cancellation: callers must not replay a mutation.
        reject(new OpsError('TIMEOUT', 'Sidecar 响应超时，操作结果待确认'))
      }, timeoutMs)
      this.pending.set(id, { resolve: value => resolve(value as T), reject, timer })
      try { this.input.write(line, error => { if (error) this.onError() }) }
      catch { this.onError() }
    })
  }

  close() { this.fail(new OpsError('RUNTIME_UNAVAILABLE', 'Sidecar 控制通道已关闭'), false) }

  private fail(error: OpsError, notify = true) {
    if (this.stopped) return
    this.stopped = true
    this.output.off('data', this.onData).off('end', this.onEnd)
    // Keep error handlers until streams close so a late EPIPE is contained.
    for (const request of this.pending.values()) { clearTimeout(request.timer); request.reject(error) }
    this.pending.clear(); this.listeners.clear(); this.buffer = Buffer.alloc(0)
    if (notify) this.fatal(error)
  }

  private consume(chunk: Buffer) {
    if (this.stopped) return
    this.buffer = Buffer.concat([this.buffer, chunk])
    for (;;) {
      const end = this.buffer.indexOf(10)
      if (end < 0) {
        if (this.buffer.length > this.maxLineBytes) this.fail(new OpsError('PROTOCOL_MISMATCH', 'Sidecar 消息超过大小限制'))
        return
      }
      if (end > this.maxLineBytes) return this.fail(new OpsError('PROTOCOL_MISMATCH', 'Sidecar 消息超过大小限制'))
      const line = this.buffer.subarray(0, end).toString('utf8').trim()
      this.buffer = this.buffer.subarray(end + 1)
      if (!line) continue
      try {
        const value = JSON.parse(line)
        if (!value || value.jsonrpc !== '2.0' || Array.isArray(value)) throw new Error()
        if (typeof value.method === 'string' && value.id === undefined) {
          for (const listener of this.listeners) {
            try { listener({ method: value.method, params: value.params }) } catch { /* isolate observers */ }
          }
          continue
        }
        if (!Number.isSafeInteger(value.id) || ('result' in value) === ('error' in value)) throw new Error()
        const request = this.pending.get(value.id)
        if (!request) continue // a late response to a timed-out request
        this.pending.delete(value.id); clearTimeout(request.timer)
        if ('error' in value) {
          request.reject(new OpsError('OPERATION_FAILED', value.error?.code === -32009 ? '配置或脚本已被其他窗口修改，请重新加载后保存' : 'Sidecar 操作失败'))
        } else request.resolve(value.result)
      } catch { this.fail(new OpsError('PROTOCOL_MISMATCH', 'Sidecar 返回了无效协议消息')); return }
    }
  }
}
