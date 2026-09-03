import type { FileTransferHost, FileTransferProgress, FileTransferDone } from '../../../frontend-shell/src/ui/filetransfer/FilesPanel'
import type { TeamsOpsClient } from './browser-client'
import type { Transfer } from './files'
const path = (p: string) => p.replaceAll('\\', '/') || '/'
export class TeamsFileHost implements FileTransferHost {
  readonly initialRemotePath = '.'
  private progress = new Set<(p: FileTransferProgress) => void>()
  private done = new Set<(d: FileTransferDone) => void>()
  private seen = new Map<string, string>()
  constructor(private client: TeamsOpsClient) {}
  ImportFile = async (directory: string, file: File, signal: AbortSignal, progress: (percent: number) => void) => {
    await this.client.upload(`${path(directory).replace(/\/$/, '')}/${file.name}`, file, signal, progress)
  }
  ExportFile = async (filePath: string) => {
    const value = await this.client.call('workspace.stat', {path:path(filePath)})
    if (!value.ok || value.entry?.isDir) throw new Error('请选择文件区内的一个文件')
    const link = document.createElement('a')
    link.href = `/api/bundles/${this.client.bundleId}/files/content?path=${encodeURIComponent(path(filePath))}`
    link.download = path(filePath).split('/').at(-1) || 'download'; link.click()
  }
  private async json(operation: string, payload: unknown) { return JSON.stringify(await this.client.call(operation, payload)) }
  FTCheck = (terminalId: string) => this.json('files.check', { terminalId })
  FTList = (terminalId: string, path: string) => this.json('files.list', { terminalId, path })
  FTStat = (terminalId: string, path: string) => this.json('files.stat', { terminalId, path })
  FTUpload = (terminalId: string, localPath: string, remotePath: string) => this.json('files.upload', { terminalId, localPath: path(localPath), remotePath, confirmed: true })
  FTDownload = (terminalId: string, remotePath: string, localPath: string) => this.json('files.download', { terminalId, localPath: path(localPath), remotePath, confirmed: true })
  FTCancel = (taskId: string) => this.json('files.cancel', { taskId })
  FTRemoteMkdir = (terminalId: string, path: string) => this.json('files.mkdir', { terminalId, path })
  FTRemoteRemove = (terminalId: string, path: string) => this.json('files.remove', { terminalId, path, confirmed: true })
  FTRemoteRename = (terminalId: string, oldPath: string, newPath: string) => this.json('files.rename', { terminalId, oldPath, newPath })
  FTRemoteReadFile = (terminalId: string, path: string) => this.json('files.readFile', { terminalId, path })
  FTRemoteWriteFile = (terminalId: string, path: string, content: string) => this.json('files.writeFile', { terminalId, path, content, confirmed: true })
  LocalList = async (p: string) => {
    const value = await this.client.call('workspace.list', { path: path(p) })
    // Shared desktop UI expects rooted paths; this '/' is virtual, never an OS root.
    for (const entry of value.entries ?? []) entry.path = '/' + entry.path.replace(/^\/+/, '')
    return JSON.stringify(value)
  }
  LocalMkdir = (p: string) => this.json('workspace.mkdir', { path: path(p) })
  LocalStat = (p: string) => this.json('workspace.stat', { path: path(p) })
  LocalRemove = (p: string) => this.json('workspace.remove', { path: path(p), confirmed: true })
  LocalRename = (oldPath: string, newPath: string) => this.json('workspace.rename', { oldPath: path(oldPath), newPath: path(newPath) })
  onProgress = (handler: (p: FileTransferProgress) => void) => { this.progress.add(handler); return () => { this.progress.delete(handler) } }
  onDone = (handler: (d: FileTransferDone) => void) => { this.done.add(handler); this.seen.clear(); return () => { this.done.delete(handler) } }
  update(tasks: Transfer[]) {
    for (const task of tasks) {
      const encoded = JSON.stringify(task)
      if (this.seen.get(task.taskId) === encoded) continue
      this.seen.set(task.taskId, encoded)
      for (const handler of this.progress) handler(task)
      if (task.state !== 'running') for (const handler of this.done) handler({ ...task, ok: task.state === 'completed', cancelled: task.state === 'cancelled', bytes: task.bytesDone, message: task.message || '运行时中断' })
    }
  }
}
