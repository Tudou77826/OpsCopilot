import type {
  FileTransferHost,
  FileTransferProgress,
  FileTransferDone,
} from '../../ui';
import type { SidecarClient } from '../../core/sidecarClient';

/**
 * Sidecar 对共享 FilesPanel 的宿主适配。
 * RPC 返回 ftEnvelope 对象，这里 JSON 化为端口约定的信封字符串（与 Wails 线格式一致）。
 * 能力边界：无 OS 拖放（onFileDrop 不提供）、无保存对话框（SelectSavePath 不提供）、
 * 无 LocalCopy —— 共享组件据此隐藏对应入口。
 */
export function makeSidecarFileTransferHost(client: SidecarClient): FileTransferHost {
  const env = (p: Promise<Record<string, unknown>>) => p.then((v) => JSON.stringify(v));
  return {
    FTCheck: (terminalId) => env(client.ftCheck(terminalId)),
    FTList: (terminalId, remotePath) => env(client.ftList(terminalId, remotePath)),
    FTStat: (terminalId, remotePath) => env(client.ftStat(terminalId, remotePath)),
    FTUpload: (terminalId, localPath, remotePath) => env(client.ftUpload(terminalId, localPath, remotePath)),
    FTDownload: (terminalId, remotePath, localPath) => env(client.ftDownload(terminalId, remotePath, localPath)),
    FTCancel: (taskId) => env(client.ftCancel(taskId)),
    FTRemoteMkdir: (terminalId, remotePath) => env(client.ftMkdir(terminalId, remotePath)),
    FTRemoteRemove: (terminalId, remotePath) => env(client.ftRemove(terminalId, remotePath)),
    FTRemoteRename: (terminalId, oldPath, newPath) => env(client.ftRename(terminalId, oldPath, newPath)),
    FTRemoteReadFile: (terminalId, remotePath, maxBytes) => env(client.ftReadFile(terminalId, remotePath, maxBytes)),
    FTRemoteWriteFile: (terminalId, remotePath, content) => env(client.ftWriteFile(terminalId, remotePath, content)),
    LocalList: (path) => env(client.fsLocalList(path)),
    LocalStat: (path) => env(client.fsLocalStat(path)),
    LocalMkdir: (path) => env(client.fsLocalMkdir(path)),
    LocalRemove: (path) => env(client.fsLocalRemove(path)),
    LocalRename: (oldPath, newPath) => env(client.fsLocalRename(oldPath, newPath)),
    onProgress(handler: (p: FileTransferProgress) => void): () => void {
      return client.on('shell.ft/progress', (params) => handler(params as FileTransferProgress));
    },
    onDone(handler: (d: FileTransferDone) => void): () => void {
      return client.on('shell.ft/done', (params) => handler(params as FileTransferDone));
    },
  };
}
