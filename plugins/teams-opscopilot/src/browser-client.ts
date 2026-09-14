/** Browser-side Host API v1. Only same-origin business calls and ticketed streams. */
export class TeamsOpsClient {
  readonly viewId = crypto.randomUUID()
  private session?: { csrfToken: string; expiresAt: number }
  private starting?: Promise<void>
  private abort = new AbortController()
  private sockets = new Set<WebSocket>()
  constructor(readonly bundleId: string) {
    if (!/^[a-z0-9-]+$/.test(bundleId)) throw new Error('插件标识无效')
  }
  async request(path: string, body: unknown): Promise<any> {
    if (!this.session || this.session.expiresAt < Date.now() + 10000) {
      this.starting ??= (async () => {
        const response = await fetch('/api/browser-session', { method: 'POST', credentials: 'same-origin', redirect: 'error', signal: this.abort.signal })
        const value = await response.json()
        if (!response.ok || !/^[A-Za-z0-9_-]{43}$/.test(value.csrfToken) || value.expiresAt <= Date.now()) throw new Error('无法建立浏览器会话')
        this.session = value
      })().finally(() => { this.starting = undefined })
      await this.starting
    }
    const response = await fetch(`/api/bundles/${this.bundleId}/${path}`, {
      method: 'POST', credentials: 'same-origin', redirect: 'error', signal: this.abort.signal,
      headers: { 'content-type': 'application/json', 'x-icode-csrf': this.session!.csrfToken }, body: JSON.stringify(body),
    })
    const value = await response.json()
    if (response.status === 401) this.session = undefined
    if (!response.ok) throw Object.assign(new Error(value.error || '操作失败，请核对状态后再试'), { code: value.code })
    return value.result
  }
  call<T = any>(operation: string, payload: unknown = {}): Promise<T> {
    return this.request('call', { schemaVersion: 1, requestId: crypto.randomUUID(), operation, payload })
  }
  async upload(path: string, file: File, signal: AbortSignal, progress: (percent: number) => void) {
    if (file.size > 2 * 1024 ** 3) throw new Error('单文件上限为 2 GiB')
    await this.call('runtime.status')
    signal.throwIfAborted(); this.abort.signal.throwIfAborted()
    await new Promise<void>((resolve, reject) => {
      const xhr = new XMLHttpRequest(), cancel = () => xhr.abort()
      const cleanup = () => { signal.removeEventListener('abort', cancel); this.abort.signal.removeEventListener('abort', cancel) }
      xhr.open('PUT', `/api/bundles/${this.bundleId}/files/content?path=${encodeURIComponent(path)}`)
      xhr.setRequestHeader('x-icode-csrf', this.session!.csrfToken)
      xhr.setRequestHeader('content-type', 'application/octet-stream')
      xhr.upload.onprogress = event => { if (event.lengthComputable) progress(Math.round(event.loaded / event.total * 100)) }
      xhr.onload = () => { cleanup(); if (xhr.status === 201) resolve(); else reject(new Error('导入失败：请检查目录是否存在、文件是否重名，以及可用空间')) }
      xhr.onerror = () => { cleanup(); reject(new Error('文件流中断，不会自动重试')) }
      xhr.onabort = () => { cleanup(); reject(new Error('导入已取消')) }
      signal.addEventListener('abort', cancel, { once: true }); this.abort.signal.addEventListener('abort', cancel, { once: true }); xhr.send(file)
    })
  }
  async attach(terminalId: string, receive: (text: string) => void, lost: () => void, takeover = false) {
    const { ticket } = await this.request(`terminals/${encodeURIComponent(terminalId)}/ticket`, { viewId: this.viewId, takeover })
    this.abort.signal.throwIfAborted()
    const url = new URL(`/api/bundles/${this.bundleId}/terminals/${encodeURIComponent(terminalId)}/stream`, location.href)
    url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:'
    url.searchParams.set('ticket', ticket)
    const socket = new WebSocket(url), decoder = new TextDecoder()
    this.sockets.add(socket)
    socket.binaryType = 'arraybuffer'
    let closed = false
    const close = () => { closed = true; socket.close(); this.sockets.delete(socket) }
    socket.addEventListener('message', event => {
      if (!closed && event.data instanceof ArrayBuffer) receive(decoder.decode(event.data, { stream: true }))
    })
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => { close(); reject(new Error('终端附着超时；不会自动重试')) }, 10000)
      socket.addEventListener('open', () => { clearTimeout(timer); resolve() }, { once: true })
      socket.addEventListener('error', () => { clearTimeout(timer); close(); reject(new Error('终端数据通道不可用')) }, { once: true })
      socket.addEventListener('close', () => { clearTimeout(timer); this.sockets.delete(socket); if (!closed) { lost(); reject(new Error('终端连接已断开')) } }, { once: true })
    })
    return { close, send: (text: string) => {
      const bytes = new TextEncoder().encode(text)
      if (socket.readyState !== WebSocket.OPEN || bytes.length > 65536 || socket.bufferedAmount > 262144) throw new Error('终端未附着或输入过大，未发送')
      socket.send(bytes)
    } }
  }
  dispose() { this.abort.abort(); for (const socket of this.sockets) socket.close(); this.sockets.clear() }
}
