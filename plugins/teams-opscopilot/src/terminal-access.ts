import { randomBytes } from 'node:crypto'
import { OpsError } from './errors.js'

/** Supplied by the authenticated host, never copied from a browser request body. */
export interface BrowserSession { sessionId: string; expiresAt: number }
interface Lease {
  terminalId: string; viewId: string; sessionId: string; generation: number
  sessionExpiresAt: number; expiresAt: number; ticket?: string
  attached: boolean; revoke?: () => void; timer?: NodeJS.Timeout
}

/** Host-only authority. A relay must check canWrite on every inbound frame. */
export class TerminalAccess {
  private readonly leases = new Map<string, Lease>()
  private readonly tickets = new Map<string, Lease>()
  private readonly exclusive = new Map<string, object>()

  constructor(private readonly state: () => { generation: number; ready: boolean },
    private readonly ownsTerminal: (terminalId: string) => boolean, private readonly now = Date.now) {}

  issue(terminalId: string, viewId: string, session: BrowserSession, takeover = false) {
    this.prune()
    if (typeof viewId !== 'string' || !/^[A-Za-z0-9_-]{1,128}$/.test(viewId) || typeof takeover !== 'boolean') throw new OpsError('INVALID_ARGUMENT', '终端视图参数无效')
    if (!/^[A-Za-z0-9_-]{43}$/.test(session.sessionId) || !Number.isSafeInteger(session.expiresAt) || session.expiresAt <= this.now()) throw new OpsError('FORBIDDEN', '浏览器会话已失效')
    const state = this.state()
    if (!state.ready) throw new OpsError('RUNTIME_UNAVAILABLE', '终端运行时不可用')
    if (!this.ownsTerminal(terminalId)) throw new OpsError('NOT_FOUND', '终端不属于本实例')
    if (this.exclusive.has(terminalId)) throw new OpsError('BUSY', '脚本回放独占终端，停止或完成后才能附着')
    const old = this.leases.get(terminalId)
    // Reusing viewId never bypasses explicit takeover, even within the same session.
    if (old && !takeover) throw new OpsError('BUSY', '终端已有写入者，需要明确接管')
    if (!old && this.leases.size >= 128) throw new OpsError('BUSY', '终端附着数量已达上限')
    if (old) this.remove(old)
    const ticket = randomBytes(32).toString('base64url')
    const lease: Lease = { terminalId, viewId, sessionId: session.sessionId, generation: state.generation,
      sessionExpiresAt: session.expiresAt, expiresAt: Math.min(this.now() + 30_000, session.expiresAt), ticket, attached: false }
    this.leases.set(terminalId, lease); this.tickets.set(ticket, lease); this.arm(lease)
    return { ticket, expiresAt: lease.expiresAt, generation: lease.generation }
  }

  consume(ticket: string, terminalId: string, session: BrowserSession, onRevoke: () => void) {
    this.prune()
    const lease = this.tickets.get(ticket)
    if (!lease || lease.terminalId !== terminalId || lease.sessionId !== session.sessionId || !Number.isSafeInteger(session.expiresAt) || session.expiresAt <= this.now()) throw new OpsError('FORBIDDEN', '终端票据无效或已消费')
    this.tickets.delete(ticket); lease.ticket = undefined; lease.attached = true
    lease.expiresAt = Math.min(lease.sessionExpiresAt, session.expiresAt)
    lease.revoke = onRevoke; this.arm(lease)
    return {
      generation: lease.generation,
      canWrite: () => { this.prune(); return this.leases.get(terminalId) === lease && lease.attached },
      close: () => this.remove(lease),
    }
  }

  revokeTerminal(terminalId: string) { const lease = this.leases.get(terminalId); if (lease) this.remove(lease) }
  /** Trusted business only. Explicit replay confirmation is required by the caller. */
  acquireExclusive(terminalId: string) {
    this.prune()
    if (!this.state().ready || !this.ownsTerminal(terminalId)) throw new OpsError('RUNTIME_UNAVAILABLE', '终端不可用')
    if (this.exclusive.has(terminalId)) throw new OpsError('BUSY', '终端已有脚本回放')
    const token = {}
    this.exclusive.set(terminalId, token)
    this.revokeTerminal(terminalId)
    return () => { if (this.exclusive.get(terminalId) === token) this.exclusive.delete(terminalId) }
  }
  invalidate() { for (const lease of [...this.leases.values()]) this.remove(lease); this.exclusive.clear() }

  private prune() {
    const state = this.state()
    for (const lease of [...this.leases.values()]) {
      if (!state.ready || lease.generation !== state.generation || lease.expiresAt <= this.now() || !this.ownsTerminal(lease.terminalId)) this.remove(lease)
    }
  }
  private arm(lease: Lease) {
    clearTimeout(lease.timer)
    lease.timer = setTimeout(() => this.remove(lease), Math.min(lease.expiresAt - this.now(), 2_147_483_647))
    lease.timer.unref()
  }
  private remove(lease: Lease) {
    if (this.leases.get(lease.terminalId) !== lease) return
    this.leases.delete(lease.terminalId)
    if (lease.ticket) this.tickets.delete(lease.ticket)
    clearTimeout(lease.timer)
    try { lease.revoke?.() } catch { /* revocation must not prevent releasing other leases */ }
  }
}
