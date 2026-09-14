import assert from 'node:assert/strict'
import test from 'node:test'
import { OpsBusiness } from '../src/business.js'
import type { SidecarRuntime } from '../src/sidecar-runtime.js'
import type { LocalInstallation } from '../src/local-installation.js'

function setup() {
  const calls: string[] = []
  const runtime = {
    subscribe: () => () => {}, status: () => ({ state: 'ready' }),
    async request(method: string) {
      calls.push(method)
      if (method === 'shell.configs.list') return { sessions: [{ id: 'saved', name: 'test', type: 'session', config: { host: 'test', host_key: 'fixture-host-key', password: 'secret', rootPassword: 'root-secret', bastion: { password: 'jump-secret' } } }] }
      if (method === 'shell.connect') return { connectionId: 'conn-1' }
      if (method === 'shell.openTerminal') return { terminalId: 'term-1' }
      return { id: 'script-1' }
    },
  } as unknown as SidecarRuntime
  return { business: new OpsBusiness(runtime, 'owner'), calls }
}
const request = (requestId: string, operation: string, payload = {}) => ({ schemaVersion: 1, requestId, operation, payload })

test('native picker does not block business calls, cannot duplicate, and retains drain protection', async () => {
  let finish!: (value: unknown) => void
  let starts = 0
  const installation = { choose: () => { starts++; return new Promise(resolve => { finish = resolve }) } } as unknown as LocalInstallation
  const runtime = { status: () => ({ state: 'ready', generation: 1 }), subscribe: () => () => {} } as unknown as SidecarRuntime
  const business = new OpsBusiness(runtime, 'owner', installation)
  const session = { sessionId: 'a'.repeat(43), expiresAt: Date.now() + 60000 }
  const input = request('picker', 'installation.choose')
  const picker = business.call(input, 'owner', session)
  assert.equal(business.call(input, 'owner', session), picker)
  await assert.rejects(business.call(request('another', 'installation.choose'), 'owner', session), { code: 'BUSY' })
  const status = await Promise.race([business.call(request('status', 'runtime.status'), 'owner', session), new Promise((_, reject) => { const timer=setTimeout(() => reject(new Error('dialog blocked status')), 1000); timer.unref() })])
  assert.equal((status as any).state, 'ready'); assert.equal(starts, 1)
  assert.throws(() => business.beginDrain(), { code: 'BUSY' })
  finish({ executable: '', cancelled: true }); await picker
  await new Promise(resolve => setImmediate(resolve))
  business.beginDrain().resume(); business.dispose()
})

test('deduplication is scoped to the authenticated browser session, not a caller payload', async () => {
  const { business, calls } = setup()
  const input = request('same-id', 'scripts.create', { name: 'test' })
  const session = { sessionId: 'a'.repeat(43), expiresAt: Date.now() + 60_000 }
  await business.call(input, 'owner', session)
  await business.call(input, 'owner', session)
  await business.call(input, 'owner', { ...session, sessionId: 'b'.repeat(43) })
  assert.deepEqual(calls, ['shell.script.create', 'shell.script.create'])
  await assert.rejects(business.call(input, 'owner', { ...session, expiresAt: 1 }), { code: 'FORBIDDEN' })
  business.dispose()
})

test('lifecycle admission blocks queued work, active resources and new work while draining', async () => {
  const { business } = setup()
  const connecting = business.call(request('connect', 'connections.connect', { id: 'saved' }), 'owner')
  assert.throws(() => business.beginDrain(), { code: 'BUSY' })
  await connecting
  assert.throws(() => business.beginDrain(), { code: 'BUSY' })
  await business.call(request('disconnect', 'connections.disconnect', { connectionId: 'conn-1' }), 'owner')
  // Let the queue's accounting finally callback complete.
  await new Promise(resolve => setImmediate(resolve))
  const drain = business.beginDrain()
  await assert.rejects(business.call(request('new', 'connections.connect', { id: 'saved' }), 'owner'), { code: 'BUSY' })
  assert.throws(() => business.beginDrain(), { code: 'BUSY' })
  drain.resume()
  await business.call(request('resumed', 'runtime.status'), 'owner')
  business.dispose()
})

test('saved connections are allowlist-redacted, including nested jump credentials', async () => {
  const { business } = setup()
  const value = await business.call(request('one', 'connections.list'), 'owner')
  assert(!JSON.stringify(value).includes('secret'))
  assert(!JSON.stringify(value).includes('password'))
  assert(JSON.stringify(value).includes('hasSecret'))
  business.dispose()
})
test('rejects identity mismatch, invalid schema, resource traversal and arbitrary methods', async () => {
  const { business, calls } = setup()
  await assert.rejects(business.call(request('one', 'runtime.status'), 'other'), { code: 'FORBIDDEN' })
  await assert.rejects(business.call({ ...request('two', 'runtime.status'), schemaVersion: 2 }, 'owner'), { code: 'INVALID_ARGUMENT' })
  await assert.rejects(business.call(request('three', 'scripts.load', { id: '../secret' }), 'owner'), { code: 'INVALID_ARGUMENT' })
  await assert.rejects(business.call(request('four', 'initialize'), 'owner'), { code: 'UNSUPPORTED_CAPABILITY' })
  await assert.rejects(business.call(request('five', 'shell.fs.list'), 'owner'), { code: 'UNSUPPORTED_CAPABILITY' })
  assert.deepEqual(calls, [])
  business.dispose()
})
test('deduplicates concurrent mutations and rejects reuse of a request ID for different content', async () => {
  const { business, calls } = setup()
  const input = request('one', 'scripts.create', { name: 'test' })
  await Promise.all([business.call(input, 'owner'), business.call(input, 'owner')])
  assert.deepEqual(calls, ['shell.script.create'])
  await assert.rejects(business.call(request('one', 'scripts.create', { name: 'changed' }), 'owner'), { code: 'INVALID_ARGUMENT' })
  business.dispose()
})
test('tracks runtime terminals and refuses references outside its own instance', async () => {
  const { business } = setup()
  await assert.rejects(business.call(request('a', 'terminals.open', { connectionId: 'foreign' }), 'owner'), { code: 'NOT_FOUND' })
  await business.call(request('b', 'connections.connect', { id: 'saved' }), 'owner')
  await business.call(request('c', 'terminals.open', { connectionId: 'conn-1' }), 'owner')
  assert.deepEqual(business.activeResources(), { connections: 1, terminals: 1 })
  await assert.rejects(business.call(request('d', 'files.list', { terminalId: 'foreign' }), 'owner'), { code: 'NOT_FOUND' })
  await business.call(request('e', 'connections.disconnect', { connectionId: 'conn-1' }), 'owner')
  assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
  business.dispose()
  await assert.rejects(business.call(request('f', 'runtime.status'), 'owner'), { code: 'RUNTIME_UNAVAILABLE' })
})

test('an in-flight connection and queued requests never continue across a runtime generation', async () => {
  let generation = 1
  let release!: (value: unknown) => void
  let entered!: () => void
  const started = new Promise<void>(resolve => { entered = resolve })
  const response = new Promise(resolve => { release = resolve })
  const calls: string[] = []
  const runtime = {
    subscribe: () => () => {}, status: () => ({ state: 'ready', generation }),
    request(method: string) { calls.push(method); entered(); return response },
  } as unknown as SidecarRuntime
  const business = new OpsBusiness(runtime, 'owner')
  const connecting = business.call(request('connect', 'connections.connect', { id: 'saved' }), 'owner')
  const queued = business.call(request('create', 'scripts.create', { name: 'must-not-run' }), 'owner')
  const rejected = Promise.all([connecting, queued].map(result => assert.rejects(result, { code: 'RUNTIME_UNAVAILABLE' })))
  await started
  generation++
  release({ sessions: [{ id: 'saved', config: { host: 'test' } }] })
  await rejected
  assert.deepEqual(calls, ['shell.configs.list'])
  assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
  business.dispose()
})

test('a late successful connection response cannot resurrect resources after disposal', async () => {
  let release!: (value: unknown) => void
  let entered!: () => void
  const started = new Promise<void>(resolve => { entered = resolve })
  const response = new Promise(resolve => { release = resolve })
  const runtime = {
    subscribe: () => () => {}, status: () => ({ state: 'ready', generation: 1 }),
    request(method: string) {
      if (method === 'shell.configs.list') return Promise.resolve({ sessions: [{ id: 'saved', config: { host: 'test', host_key: 'fixture-host-key' } }] })
      entered(); return response
    },
  } as unknown as SidecarRuntime
  const business = new OpsBusiness(runtime, 'owner')
  const connecting = business.call(request('connect', 'connections.connect', { id: 'saved' }), 'owner')
  const rejected = assert.rejects(connecting, { code: 'RUNTIME_UNAVAILABLE' })
  await started
  business.dispose()
  release({ connectionId: 'stale-connection' })
  await rejected
  assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
})
