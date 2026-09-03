import assert from 'node:assert/strict'
import { execFile, type ChildProcessWithoutNullStreams } from 'node:child_process'
import { promisify } from 'node:util'
import { mkdir, mkdtemp, readFile, readdir, writeFile, rm } from 'node:fs/promises'
import { tmpdir } from 'node:os'
import { basename, dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { after, before, test } from 'node:test'
import { LocalInstallation } from '../src/local-installation.js'
import { SidecarRuntime } from '../src/sidecar-runtime.js'
import { OpsBusiness } from '../src/business.js'
import { Context } from '@deepseek-ai/cordis'
import plugin from '../src/entry.js'

const repository = fileURLToPath(new URL('../../../', import.meta.url))
const root = await mkdtemp(join(tmpdir(), 'ops-local-install-'))
const installDir = join(root, '便携 Ops with spaces'), exe = join(installDir, 'OpsCopilot.exe')
const runtimes: SidecarRuntime[] = []
before(async () => {
  await mkdir(installDir)
  await promisify(execFile)('go', ['build', '-ldflags', '-X main.Version=local-test', '-o', exe, '.'], { cwd: repository, windowsHide: true, timeout: 120000 })
  await writeFile(join(installDir, 'config.json'), JSON.stringify({ appearance: { theme: 'light' }, scripts: { dir: 'my-scripts' }, log: { dir: 'my-logs' }, llm: { APIKey: 'fixture-key-only', BaseURL: 'https://example.invalid', FastModel: 'fixture', ComplexModel: 'fixture' } }))
  await writeFile(join(installDir, 'quick_commands.json'), JSON.stringify([{ id: 'existing', name: '桌面已有命令', content: 'pwd', group: 'default' }]))
  await writeFile(join(installDir, 'sessions.json'), JSON.stringify([{ id: 'existing', type: 'session', name: '本地已有会话', config: { host: 'fixture.invalid', port: 22, user: 'test', password: 'fixture-password-only', host_key: 'fixture-pin' } }]))
}, { timeout: 130000 })
after(async () => {
  for (const runtime of runtimes) await runtime.stop()
  assert.equal(dirname(resolve(root)), resolve(tmpdir()))
  assert(basename(root).startsWith('ops-local-install-'))
  await rm(root, { recursive: true, force: true, maxRetries: 5, retryDelay: 100 })
})

test('old and missing binaries fail before execution and do not replace the saved path', async () => {
  const installation = new LocalInstallation(join(root, 'invalid'))
  await installation.load(); assert.equal(installation.status().configured, false)
  await assert.rejects(installation.configure(join(root, 'missing.exe')), { code: 'NOT_FOUND' })
  const old = join(root, 'old.exe'); await writeFile(old, 'MZ old desktop with no plugin mode')
  await assert.rejects(installation.configure(old), { code: 'PROTOCOL_MISMATCH' })
  assert.equal(installation.status().configured, false)
  await installation.configure(exe)
  await assert.rejects(installation.configure(old), { code: 'PROTOCOL_MISMATCH' })
  assert.equal(installation.status().executable, exe)
  const remembered = new LocalInstallation(join(root, 'invalid')); await remembered.load()
  assert.equal(remembered.status().executable, exe)
  installation.dispose(); remembered.dispose()
})

test('the selected desktop exe launches distinct processes and shares exe-relative and custom paths', async () => {
  const beforeFiles = await readdir(root)
  const { stdout } = await promisify(execFile)(exe, ['--plugin-info'], { cwd: root, windowsHide: true })
  assert.equal(JSON.parse(stdout).product, 'OpsCopilot'); assert.deepEqual(await readdir(root), beforeFiles)
  const local = new LocalInstallation(join(root, 'teams'))
  await local.configure(exe)
  const first = new SidecarRuntime(() => local.resolve()), second = new SidecarRuntime(() => local.resolve())
  runtimes.push(first, second); await Promise.all([first.start(), second.start()])
  const child = (r: SidecarRuntime) => (r as unknown as { child: ChildProcessWithoutNullStreams }).child
  assert.equal(child(first).spawnfile, exe); assert(child(first).spawnargs.includes('--teams-plugin')); assert.notEqual(child(first).pid, child(second).pid)
  assert.equal(first.status().version, 'local-test', 'local product version is independent of plugin version')
  const business = new OpsBusiness(first, 'owner'), session = { sessionId: 'a'.repeat(43), expiresAt: Date.now() + 60000 }
  let i = 0; const call = (operation: string, payload = {}) => business.call({ schemaVersion: 1, requestId: `r${++i}`, operation, payload }, 'owner', session) as Promise<any>
  const saved = await call('connections.list'); assert.equal(saved.sessions[0].name, '本地已有会话'); assert(!JSON.stringify(saved).includes('fixture-password-only'))
  const commands = await call('quickCommands.list'); assert.equal(commands.commands[0].name, '桌面已有命令')
  await Promise.all([first.request('shell.quickcmds.save', { name: 'first', content: 'pwd' }), second.request('shell.quickcmds.save', { name: 'second', content: 'ls' })])
  assert.equal(JSON.parse(await readFile(join(installDir, 'quick_commands.json'), 'utf8')).length, 3)
  const settings = await call('settings.load'); assert.equal(settings.theme, 'light'); assert(settings.revision)
  await call('settings.save', { settings: { ...settings, completionDelay: 222 } })
  const config = JSON.parse(await readFile(join(installDir, 'config.json'), 'utf8'))
  assert.equal(config.completion_delay, 222); assert.equal(config.llm.APIKey, 'fixture-key-only')
  const sc = await call('scripts.create', { name: 'shared-script' })
  assert.equal(JSON.parse(await readFile(join(installDir, 'my-scripts', `script_${sc.id}.json`), 'utf8')).name, 'shared-script')
  assert((await readdir(join(installDir, 'my-logs'))).includes('recordings'))
  assert(!(await readdir(join(root, 'teams'))).some(name => /config|session|quick|script/.test(name)))
  const workspace = await call('workspace.list', { path: '/' }); assert.equal(workspace.ok, true, JSON.stringify(workspace)); assert.deepEqual(workspace.entries ?? [], [])
  const latest = await second.request<any>('shell.script.load', { id: sc.id })
  await second.request('shell.script.update', { ...latest, name: 'other-window' })
  await assert.rejects(call('scripts.update', { id: sc.id, script: { ...sc, name: 'stale-edit' } }), /其他窗口/)
  business.dispose(); await first.stop(); await second.stop(); local.dispose()
})

test('unconfigured adapter remains mountable and installation mutations require authenticated context', async () => {
  const ctx = new Context(), dataDirectory = join(root, 'first-use')
  await plugin(ctx, { host: { protocol: 1, bundleId: 'opscopilot', version: '0.1.0', artifactDirectory: root, dataDirectory } })
  try {
  const service = (ctx as any).opscopilot
  assert.deepEqual(await service.health(), { healthy: true })
  const request = { schemaVersion: 1, requestId: 'setup', operation: 'installation.configure', payload: { executable: exe, confirmed: true } }
  await assert.rejects(service.call(request), { code: 'FORBIDDEN' })
  const context = { protocol: 1, session: { sessionId: 'b'.repeat(43), expiresAt: Date.now() + 60000 } }
  const result = await service.call(request, context); assert.equal(result.state, 'ready')
  assert.equal(JSON.parse(await readFile(join(dataDirectory, 'local-installation.json'), 'utf8')).executable, exe)
  } finally { await ctx.fiber.dispose() }
})
