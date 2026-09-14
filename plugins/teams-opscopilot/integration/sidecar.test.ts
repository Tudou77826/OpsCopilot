import assert from 'node:assert/strict'
import { execFile, spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { createHash } from 'node:crypto'
import { once } from 'node:events'
import { mkdir, mkdtemp, readFile, rm, writeFile } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { fileURLToPath } from 'node:url'
import { promisify } from 'node:util'
import { after, before, test } from 'node:test'
import { Context } from '@deepseek-ai/cordis'
import { SidecarRuntime } from '../src/sidecar-runtime.js'
import { createOpsPlugin, OpsBusiness } from '../src/business.js'

const root = await mkdtemp(join(tmpdir(), 'ops-teams-sidecar-'))
// integration/ -> teams-opscopilot/ -> plugins/ -> repository/
const repository = fileURLToPath(new URL('../../../', import.meta.url))
const executable = join(root, process.platform === 'win32' ? 'shellsidecar.exe' : 'shellsidecar')
const sshExecutable = join(root, process.platform === 'win32' ? 'fakessh.exe' : 'fakessh')
let sha256 = ''
let ssh: ChildProcessWithoutNullStreams | undefined
let sshPort = 0
const runtimes: SidecarRuntime[] = []

before(async () => {
  await Promise.all([
    promisify(execFile)('go', ['build', '-ldflags', '-X main.version=teams-test', '-o', executable, './cmd/shellsidecar'], { cwd: repository, timeout: 120_000 }),
    promisify(execFile)('go', ['build', '-o', sshExecutable, './cmd/fakessh'], { cwd: repository, timeout: 120_000 }),
  ])
  sha256 = createHash('sha256').update(await readFile(executable)).digest('hex')
  const remote = join(root, 'remote')
  await mkdir(remote)
  await writeFile(join(remote, 'sample.txt'), 'hello from isolated SFTP\n')
  ssh = spawn(sshExecutable, [], { stdio: 'pipe', windowsHide: true, env: { ...process.env, FAKESSH_SFTP_ROOT: remote } })
  ssh.stderr.resume()
  sshPort = await new Promise<number>((resolvePort, reject) => {
    const timer = setTimeout(() => reject(new Error('SSH fixture startup timeout')), 5000)
    ssh!.once('error', error => { clearTimeout(timer); reject(error) })
    let text = ''
    ssh!.stdout.on('data', bytes => {
      text += bytes.toString()
      const match = text.match(/127\.0\.0\.1:(\d+)/)
      if (match) { clearTimeout(timer); resolvePort(Number(match[1])) }
    })
  })
}, { timeout: 130_000 })

after(async () => {
  for (const runtime of runtimes) await runtime.stop()
  if (ssh && ssh.exitCode === null) { const closed = once(ssh, 'close'); ssh.kill(); await closed }
  await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
})

function runtime(name: string, override = {}) {
  const value = new SidecarRuntime({ executable, sha256, version: 'teams-test', dataDir: join(root, name), ...override })
  runtimes.push(value)
  return value
}
const request = (requestId: string, operation: string, payload = {}) => ({ schemaVersion: 1, requestId, operation, payload })
async function trustedHost(service: SidecarRuntime) { return (await service.request<{ key: string }>('shell.hostKey.probe', { host: '127.0.0.1', port: sshPort })).key }

test('real process verifies digest, handshakes once, hides endpoints and closes on stdio EOF', async () => {
  const service = runtime('lifecycle')
  await Promise.all([service.start(), service.start()])
  assert.equal(service.status().generation, 1)
  assert.equal(service.status().state, 'ready')
  assert(!JSON.stringify(service.status()).includes('token'))
  assert(!JSON.stringify(service.status()).includes('wsBase'))
  const { sessions } = await service.request<{ sessions: unknown[] }>('shell.configs.list')
  assert.equal(sessions.length, 0)
  const child = (service as unknown as { child: ChildProcessWithoutNullStreams }).child
  await service.stop()
  assert.equal(service.status().state, 'stopped')
  assert.equal(child.exitCode, 0, 'normal shutdown must reach Go EOF cleanup, not the force-kill fallback')
  await assert.rejects(service.request('shell.configs.list'), { code: 'RUNTIME_UNAVAILABLE' })
  await service.start(); assert.equal(service.status().generation, 2); await service.stop()
})

test('refuses tampered or mismatched binaries and can stop while startup is in flight', async () => {
  const badHash = runtime('bad-hash', { sha256: '0'.repeat(64) })
  await assert.rejects(badHash.start(), { code: 'FORBIDDEN' })
  assert.equal(badHash.status().generation, 0)
  const mismatch = runtime('bad-version', { version: 'not-this-version' })
  await assert.rejects(mismatch.start(), { code: 'PROTOCOL_MISMATCH' })
  const cancelled = runtime('cancelled')
  const starting = cancelled.start()
  const rejected = assert.rejects(starting, { code: 'RUNTIME_UNAVAILABLE' })
  await cancelled.stop(); await rejected
  assert.equal(cancelled.status().state, 'stopped')
})

test('real SSH PTY, SFTP transfer and script recording use the same isolated Sidecar', { timeout: 25_000 }, async () => {
  const service = runtime('operations')
  await service.start()
  // Internal provisioning only. The public plugin does not expose raw secret writes.
  const saved = await service.request<{ id: string }>('shell.configs.save', { name: 'fixture', host: '127.0.0.1', port: sshPort, user: 'test', password: 'test', host_key: await trustedHost(service) })
  const business = new OpsBusiness(service, 'local-test')
  let counter = 0
  const call = (op: string, payload = {}) => business.call(request(`r${++counter}`, op, payload), 'local-test') as Promise<any>
  const events: Array<{ method: string; params: any }> = []
  const unsubscribe = service.subscribe(event => events.push(event))
  let ws: WebSocket | undefined
  try {
    const listing = await call('connections.list')
    assert(!JSON.stringify(listing).includes('password'))
    const connection = await call('connections.connect', { id: saved.id })
    const terminal = await call('terminals.open', { connectionId: connection.connectionId })
    const terminalId = terminal.terminalId
    let output = ''
    ws = new WebSocket(service.terminalTarget(terminalId))
    ws.binaryType = 'arraybuffer'
    ws.onmessage = event => { output += typeof event.data === 'string' ? event.data : Buffer.from(event.data).toString() }
    await new Promise<void>((resolveOpen, reject) => {
      const timer = setTimeout(() => reject(new Error('PTY connection timeout')), 5000)
      ws!.onopen = () => { clearTimeout(timer); resolveOpen() }
      ws!.onerror = () => { clearTimeout(timer); reject(new Error('PTY connection failed')) }
    })
    await service.request('shell.script.startRecording', { name: 'recorded', terminalId })
    ws.send(Buffer.from('echo recorded-marker\n'))
    await eventually(() => output.includes('recorded-marker'))
    const recorded = await service.request<{ id: string }>('shell.script.stopRecording')
    const script = await call('scripts.load', { id: recorded.id })
    assert(JSON.stringify(script).includes('recorded-marker'))
    await service.request('shell.script.replay', { id: recorded.id, terminalId })
    await eventually(() => output.split('recorded-marker').length >= 3)
    await call('terminals.resize', { terminalId, cols: 110, rows: 35 })
    const files = await call('files.list', { terminalId, path: '/' })
    assert.equal(files.ok, true)
    assert(JSON.stringify(files).includes('sample.txt'))
    const transfer = await service.request<any>('shell.ft.download', { terminalId, remotePath: '/sample.txt', localPath: 'download.txt' })
    assert.equal(transfer.ok, true)
    await eventually(() => events.some(event => event.method === 'shell.ft/done' && event.params.taskId === transfer.taskId))
    assert.equal(await readFile(join(root, 'operations', 'files', 'download.txt'), 'utf8'), 'hello from isolated SFTP\n')
    const upload = await service.request<any>('shell.ft.upload', { terminalId, localPath: 'download.txt', remotePath: '/uploaded.txt' })
    assert.equal(upload.ok, true)
    await eventually(() => events.some(event => event.method === 'shell.ft/done' && event.params.taskId === upload.taskId))
    assert.equal(await readFile(join(root, 'remote', 'uploaded.txt'), 'utf8'), 'hello from isolated SFTP\n')
    const created = await call('scripts.create', { name: 'library-entry' })
    await call('scripts.load', { id: created.id }); await call('scripts.delete', { id: created.id })
    await call('connections.disconnect', { connectionId: connection.connectionId })
    await eventually(() => events.some(event => event.method === 'terminal/exited'))
    assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
  } finally { ws?.close(); unsubscribe(); business.dispose(); await service.stop() }
})

test('public script lifecycle records, edits, replays exclusively and stops later dispatch', { timeout: 25000 }, async () => {
  const service = runtime('managed-scripts')
  await service.start()
  const business = new OpsBusiness(service, 'owner')
  const session = { sessionId: 'm'.repeat(43), expiresAt: Date.now() + 60_000 }
  let counter = 0
  const call = (op: string, payload = {}) => business.call(request(`managed-${++counter}`, op, payload), 'owner', session) as Promise<any>
  let ws: WebSocket | undefined
  try {
    const saved = await service.request<any>('shell.configs.save', { name: 'fixture', host: '127.0.0.1', port: sshPort, user: 'test', password: 'test', host_key: await trustedHost(service) })
    const connection = await call('connections.connect', { id: saved.id })
    const { terminalId } = await call('terminals.open', connection)
    ws = new WebSocket(service.terminalTarget(terminalId))
    let output = ''
    ws.binaryType = 'arraybuffer'
    ws.onmessage = event => { output += typeof event.data === 'string' ? event.data : Buffer.from(event.data).toString() }
    await new Promise<void>((resolve, reject) => { ws!.onopen = () => resolve(); ws!.onerror = () => reject(new Error('fixture WS failed')) })
    await call('scripts.record.start', { terminalId, name: 'recorded' })
    await assert.rejects(call('terminals.close', { terminalId }), { code: 'BUSY' })
    await assert.rejects(call('connections.disconnect', connection), { code: 'BUSY' })
    ws.send(Buffer.from('echo managed-recording\n'))
    await eventually(() => output.includes('managed-recording'))
    const recorded = await call('scripts.record.stop')
    assert(JSON.stringify(await call('scripts.load', { id: recorded.id })).includes('managed-recording'))
    await call('scripts.update', { id: recorded.id, script: { ...recorded, name: 'edited', steps: [
      { command: 'echo ${MARKER}', enabled: true },
      { command: 'echo must-not-dispatch', enabled: true, delay: 60_000 },
    ], variables: [{ name: 'MARKER', required: true }] } })
    const updated = await call('scripts.load', { id: recorded.id })
    assert.equal(updated.name, 'edited')
    assert.equal(updated.steps.length, 2)
    const ticket = business.issueTerminalTicket(terminalId, 'interactive', session)
    let revoked = false
    const writer = business.consumeTerminalTicket(ticket.ticket, terminalId, session, () => { revoked = true })
    await assert.rejects(call('scripts.replay.start', { terminalId, id: recorded.id, values: { MARKER: 'marker' } }), { code: 'INVALID_ARGUMENT' })
    assert.equal(writer.canWrite(), true)
    const replay = await call('scripts.replay.start', { terminalId, id: recorded.id, values: { MARKER: 'managed-first-step' }, confirmed: true })
    assert.equal(revoked, true)
    assert.equal(writer.canWrite(), false)
    assert.throws(() => business.issueTerminalTicket(terminalId, 'second-view', session, true), { code: 'BUSY' })
    await assert.rejects(call('scripts.record.start', { terminalId, name: 'mixed' }), { code: 'BUSY' })
    await assert.rejects(call('connections.disconnect', connection), { code: 'BUSY' })
    await eventually(() => output.includes('managed-first-step'))
    const snapshot = await call('runtime.snapshot')
    assert.equal(snapshot.replays[0].id, replay.id)
    assert.equal(snapshot.replays[0].state, 'running')
    assert(!JSON.stringify(snapshot).includes('managed-first-step'))
    const stopped = await call('scripts.replay.stop', { runId: replay.id })
    assert.equal(stopped.state, 'stopped')
    assert.equal(stopped.sent, 1)
    assert.equal(business.activity().replays, 0)
    assert.equal(output.includes('must-not-dispatch'), false)
    business.issueTerminalTicket(terminalId, 'after-stop', session)
    // Empty edits must not resurrect the old commands when reloaded.
    await call('scripts.update', { id: recorded.id, script: { ...await call('scripts.load', { id: recorded.id }), name: 'empty', steps: [] } })
    assert.equal((await call('scripts.load', { id: recorded.id })).commands.length, 0)
    await call('scripts.delete', { id: recorded.id })
    await call('connections.disconnect', connection)
  } finally { ws?.close(); business.dispose(); await service.stop() }
})

test('connection setup confirms the probed host key and never persists browser passwords', async () => {
  const service = runtime('safe-connections')
  await service.start()
  const business = new OpsBusiness(service, 'owner')
  let sequence = 0
  const call = (operation: string, payload = {}) => business.call(request(`setup-${++sequence}`, operation, payload), 'owner') as Promise<any>
  const config = { name: 'safe fixture', host: '127.0.0.1', port: sshPort, user: 'test', password: 'must-not-persist' }
  try {
    const probe = await call('connections.probe', { config })
    assert(probe.fingerprint.startsWith('SHA256:'))
    await assert.rejects(call('connections.save', { config, challengeId: probe.challengeId }), { code: 'FORBIDDEN' })
    const saved = await call('connections.save', { config, challengeId: probe.challengeId, confirmed: true })
    const persisted = await readFile(join(root, 'safe-connections', 'saved-connections.json'), 'utf8')
    assert(!persisted.includes('must-not-persist'))
    assert(persisted.includes('host_key'))
    await assert.rejects(call('connections.save', { config, challengeId: probe.challengeId, confirmed: true }), { code: 'FORBIDDEN' })
    const connection = await call('connections.connect', { id: saved.id, password: 'test' })
    await call('connections.disconnect', connection)
    await call('connections.delete', { id: saved.id })
  } finally { business.dispose(); await service.stop() }
})

test('Cordis owns the Business service and disposes the real Sidecar with its Fiber', async () => {
  const service = runtime('cordis')
  const context = new Context()
  const fiber = context.plugin(createOpsPlugin(service, 'owner'))
  try {
    await fiber
    const business = context.get('opscopilot') as { health(): Promise<unknown>; call(value: unknown): Promise<any> }
    await business.health()
    assert.equal((await business.call(request('status', 'runtime.status'))).state, 'ready')
  } finally { await fiber.dispose(); await context.fiber.dispose() }
  assert.equal(service.status().state, 'stopped')
})

test('unexpected child death invalidates runtime resources without automatically replaying work', async () => {
  const service = runtime('crash')
  await service.start()
  const business = new OpsBusiness(service, 'owner')
  try {
    const saved = await service.request<{ id: string }>('shell.configs.save', { name: 'crash-fixture', host: '127.0.0.1', port: sshPort, user: 'test', password: 'test', host_key: await trustedHost(service) })
    const connection = await business.call(request('connect', 'connections.connect', { id: saved.id }), 'owner') as { connectionId: string }
    await business.call(request('terminal', 'terminals.open', connection), 'owner')
    assert.equal(business.activeResources().terminals, 1)
    // Test-only process handle: do not add a kill/PID surface to the public API.
    const child = (service as unknown as { child: ChildProcessWithoutNullStreams }).child
    const closed = once(child, 'close'); child.kill('SIGKILL'); await closed
    assert.equal(service.status().state, 'failed')
    assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
    await assert.rejects(service.request('shell.configs.list'), { code: 'RUNTIME_UNAVAILABLE' })
    await service.start()
    assert.equal(service.status().generation, 2)
    assert.deepEqual(business.activeResources(), { connections: 0, terminals: 0 })
  } finally { business.dispose(); await service.stop() }
})

async function eventually(check: () => boolean) {
  const deadline = Date.now() + 5000
  while (!check()) {
    if (Date.now() > deadline) throw new Error('Expected isolated Sidecar state was not observed')
    await new Promise(resolve => setTimeout(resolve, 20))
  }
}

test('host tickets attach real PTY streams and takeover revokes the previous writer without closing the PTY', { timeout: 15_000 }, async () => {
  const service = runtime('ticket-stream')
  await service.start()
  const business = new OpsBusiness(service, 'owner')
  const session = { sessionId: 'a'.repeat(43), expiresAt: Date.now() + 60_000 }
  const sockets: WebSocket[] = []
  try {
    const saved = await service.request<{ id: string }>('shell.configs.save', { name: 'ticket-fixture', host: '127.0.0.1', port: sshPort, user: 'test', password: 'test', host_key: await trustedHost(service) })
    const connection = await business.call(request('connect', 'connections.connect', { id: saved.id }), 'owner') as { connectionId: string }
    const { terminalId } = await business.call(request('terminal', 'terminals.open', connection), 'owner') as { terminalId: string }
    async function attach(ticket: string) {
      let socket: WebSocket | undefined
      const lease = business.consumeTerminalTicket(ticket, terminalId, session, () => socket?.close())
      socket = new WebSocket(lease.target); sockets.push(socket); socket.binaryType = 'arraybuffer'
      let output = ''
      socket.onmessage = event => { output += typeof event.data === 'string' ? event.data : Buffer.from(event.data).toString() }
      await new Promise<void>((resolve, reject) => {
        const timer = setTimeout(() => { lease.close(); reject(new Error('test relay attach timeout')) }, 3000)
        socket!.onopen = () => { clearTimeout(timer); resolve() }
        socket!.onerror = () => { clearTimeout(timer); lease.close(); reject(new Error('test relay attach failed')) }
      })
      socket.onclose = () => lease.close()
      return { socket, output: () => output, send(value: string) {
        assert(lease.canWrite(), 'revoked relay cannot forward input')
        socket!.send(Buffer.from(value))
      } }
    }
    const ticket = business.issueTerminalTicket(terminalId, 'first', session)
    assert(!JSON.stringify(ticket).includes('token'))
    const first = await attach(ticket.ticket)
    first.send('echo first-writer\n'); await eventually(() => first.output().includes('first-writer'))
    assert.throws(() => business.issueTerminalTicket(terminalId, 'second', session), { code: 'BUSY' })
    const next = business.issueTerminalTicket(terminalId, 'second', session, true)
    assert.throws(() => first.send('must-not-be-sent\n'))
    const second = await attach(next.ticket)
    second.send('echo second-writer\n'); await eventually(() => second.output().includes('second-writer'))
    await eventually(() => first.socket.readyState === WebSocket.CLOSED)
    assert.equal(business.activeResources().terminals, 1)
    await business.call(request('close', 'terminals.close', { terminalId }), 'owner')
    await eventually(() => second.socket.readyState === WebSocket.CLOSED)
  } finally { business.dispose(); for (const socket of sockets) socket.close(); await service.stop() }
})
