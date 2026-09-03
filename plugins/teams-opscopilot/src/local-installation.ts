import { execFile } from 'node:child_process'
import { promisify } from 'node:util'
import { createHash, randomUUID } from 'node:crypto'
import { mkdir, readFile, realpath, rename, stat, writeFile, unlink } from 'node:fs/promises'
import { dirname, extname, isAbsolute, join } from 'node:path'
import { OpsError } from './errors.js'
import type { SidecarOptions } from './sidecar-runtime.js'
import { chooseExecutable } from './executable-picker.js'
import { assertCompatible, pluginMarker } from './compatibility.js'

export { pluginMarker } from './compatibility.js'
const runFile = promisify(execFile)

/** Only installation metadata is owned by Teams. Ops resolves its own user data. */
export class LocalInstallation {
  private executable = ''
  private message = ''
  private abort = new AbortController()
  constructor(private readonly directory: string) {}
  async load() {
    try {
      const value = JSON.parse(await readFile(join(this.directory, 'local-installation.json'), 'utf8'))
      if (typeof value.executable !== 'string' || !isAbsolute(value.executable)) throw new Error()
      this.executable = value.executable
    } catch (error: any) { if (error.code !== 'ENOENT') this.message = '本地程序路径记录无效，请重新选择；原记录未被删除' }
  }
  status() { return { executable: this.executable, configured: Boolean(this.executable), message: this.message } }
  failed(error: unknown) { this.message = error instanceof OpsError ? error.message : '本地 Ops 启动失败，请检查程序版本和配置文件' }
  dispose() { this.abort.abort() }
  async choose(): Promise<{ executable: string; cancelled: boolean }> {
    return chooseExecutable(this.abort.signal)
  }
  async configure(executable: string) {
    const options = await this.inspect(executable)
    await mkdir(this.directory, { recursive: true, mode: 0o700 })
    const temp = join(this.directory, `installation-${randomUUID()}.tmp`)
    try {
      await writeFile(temp, JSON.stringify({ executable: options.executable }), { flag: 'wx', mode: 0o600 })
      this.abort.signal.throwIfAborted()
      await rename(temp, join(this.directory, 'local-installation.json'))
    } finally { await unlink(temp).catch(() => {}) }
    this.executable = options.executable; this.message = ''
    return this.status()
  }
  async resolve(): Promise<SidecarOptions> {
    if (!this.executable) throw new OpsError('RUNTIME_UNAVAILABLE', '请选择本地 OpsCopilot.exe')
    return this.inspect(this.executable)
  }
  private async inspect(value: string): Promise<SidecarOptions> {
    this.abort.signal.throwIfAborted()
    if (!isAbsolute(value) || value.startsWith('\\\\') || extname(value).toLowerCase() !== '.exe' || value.includes('\0')) throw new OpsError('INVALID_ARGUMENT', '请选择本机磁盘上的 OpsCopilot.exe')
    let executable: string, bytes: Buffer
    try {
      executable = await realpath(value)
      const info = await stat(executable)
      if (!info.isFile() || info.size > 512 * 1024 ** 2) throw new Error()
      bytes = await readFile(executable)
    } catch { throw new OpsError('NOT_FOUND', '找不到本地 Ops 程序，请重新选择') }
    if (bytes.subarray(0, 2).toString() !== 'MZ' || !bytes.includes(Buffer.from(pluginMarker))) throw new OpsError('PROTOCOL_MISMATCH', '该版本不支持 Teams 插件模式，请先升级本地 Ops；未启动此程序')
    let info: any
    try {
      const { stdout } = await runFile(executable, ['--plugin-info'], { cwd: dirname(executable), windowsHide: true, shell: false, timeout: 5000, maxBuffer: 16384, signal: this.abort.signal })
      info = JSON.parse(stdout)
    } catch { throw new OpsError('PROTOCOL_MISMATCH', '无法确认本地 Ops 的插件协议，请升级后重试') }
    assertCompatible(info)
    return { executable, sha256: createHash('sha256').update(bytes).digest('hex'), version: info.version, dataDir: join(this.directory, 'transport'), localMode: true }
  }
}
