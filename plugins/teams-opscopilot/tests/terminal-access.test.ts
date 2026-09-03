import assert from 'node:assert/strict'
import test from 'node:test'
import { TerminalAccess } from '../src/terminal-access.js'

function setup() {
  let now = 1000, generation = 1
  const authority = new TerminalAccess(() => ({ ready: true, generation }), id => ['term-1', 'term-2'].includes(id), () => now)
  const session = { sessionId: 'a'.repeat(43), expiresAt: 100_000 }
  return { authority, session, advance: (ms: number) => { now += ms }, restart: () => { generation++ } }
}

test('script exclusivity revokes interactive input and cannot be bypassed by takeover', () => {
  const { authority, session } = setup()
  const ticket = authority.issue('term-1', 'view', session)
  let revoked = false
  const lease = authority.consume(ticket.ticket, 'term-1', session, () => { revoked = true })
  const release = authority.acquireExclusive('term-1')
  assert.equal(revoked, true)
  assert.equal(lease.canWrite(), false)
  assert.throws(() => authority.issue('term-1', 'new-view', session, true), { code: 'BUSY' })
  assert.throws(() => authority.acquireExclusive('term-1'), { code: 'BUSY' })
  release()
  const releaseNew = authority.acquireExclusive('term-1')
  release()
  assert.throws(() => authority.issue('term-1', 'new-view', session, true), { code: 'BUSY' })
  releaseNew()
  authority.issue('term-1', 'new-view', session)
  authority.invalidate()
})

test('tickets are one-use, session/terminal-bound and expire without leaking a target', () => {
  const { authority, session, advance } = setup()
  try {
    const ticket = authority.issue('term-1', 'view-1', session)
    assert(!JSON.stringify(ticket).includes('target'))
    assert.throws(() => authority.consume(ticket.ticket, 'term-2', session, () => {}), { code: 'FORBIDDEN' })
    assert.throws(() => authority.consume(ticket.ticket, 'term-1', { ...session, sessionId: 'b'.repeat(43) }, () => {}), { code: 'FORBIDDEN' })
    const lease = authority.consume(ticket.ticket, 'term-1', session, () => {})
    assert.equal(lease.canWrite(), true)
    assert.throws(() => authority.consume(ticket.ticket, 'term-1', session, () => {}), { code: 'FORBIDDEN' })
    lease.close()
    const expired = authority.issue('term-1', 'view-2', session)
    advance(30_001)
    assert.throws(() => authority.consume(expired.ticket, 'term-1', session, () => {}), { code: 'FORBIDDEN' })
    assert.doesNotThrow(() => authority.issue('term-1', 'view-3', session))
  } finally { authority.invalidate() }
})

test('takeover revokes the old writer immediately; stale close cannot release the new writer', () => {
  const { authority, session } = setup()
  let revoked = 0
  try {
    const one = authority.issue('term-1', 'same-view', session)
    const first = authority.consume(one.ticket, 'term-1', session, () => { revoked++ })
    assert.throws(() => authority.issue('term-1', 'same-view', session), { code: 'BUSY' })
    const two = authority.issue('term-1', 'next-view', session, true)
    assert.equal(revoked, 1); assert.equal(first.canWrite(), false)
    const second = authority.consume(two.ticket, 'term-1', session, () => {})
    first.close(); assert.equal(second.canWrite(), true)
  } finally { authority.invalidate() }
})

test('generation change, terminal close and browser expiry invalidate writers', () => {
  const { authority, session, restart, advance } = setup()
  let revoked = 0
  const attach = () => authority.consume(authority.issue('term-1', 'view', session).ticket, 'term-1', session, () => { revoked++ })
  try {
    const first = attach(); restart(); assert.equal(first.canWrite(), false)
    const second = attach(); authority.revokeTerminal('term-1'); assert.equal(second.canWrite(), false)
    const third = attach(); advance(100_000); assert.equal(third.canWrite(), false)
    assert.equal(revoked, 3)
  } finally { authority.invalidate() }
})

test('disconnect releases only its lease and pending tickets reserve a writer', () => {
  const { authority, session } = setup()
  try {
    const pending = authority.issue('term-1', 'view', session)
    assert.throws(() => authority.issue('term-1', 'other', session), { code: 'BUSY' })
    const attached = authority.consume(pending.ticket, 'term-1', session, () => {})
    attached.close()
    assert.doesNotThrow(() => authority.issue('term-1', 'other', session))
    assert.throws(() => authority.issue('foreign', 'view', session), { code: 'NOT_FOUND' })
  } finally { authority.invalidate() }
})
