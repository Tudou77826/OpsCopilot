import { OpsError } from './errors.js'
import type { BrowserSession } from './terminal-access.js'

/** Structural v1 contract with Teams; no imports from another repository's internals. */
export interface BrowserCallContext { protocol: 1; session: BrowserSession }

export function trustedBrowserSession(context: unknown): BrowserSession {
  const value = context as BrowserCallContext | undefined
  if (value?.protocol !== 1 || typeof value.session?.sessionId !== 'string' || !/^[A-Za-z0-9_-]{43}$/.test(value.session.sessionId) ||
    !Number.isSafeInteger(value.session.expiresAt) || value.session.expiresAt <= Date.now() || value.session.expiresAt > Date.now() + 8 * 3600_000) {
    throw new OpsError('FORBIDDEN', '需要宿主认证的浏览器上下文')
  }
  return { ...value.session }
}
