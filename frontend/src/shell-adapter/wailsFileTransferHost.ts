import type {
    FileTransferHost,
    FileTransferProgress,
    FileTransferDone,
    FileDropHandler,
} from '@opscopilot/shell-terminal/ui';

type WailsWindow = Window & {
    go?: { main?: { App?: Record<string, (...args: any[]) => Promise<string>> } };
    runtime?: {
        EventsOn?: (name: string, cb: (data: any) => void) => (() => void) | void;
        OnFileDrop?: (cb: FileDropHandler, useDropTarget: boolean) => void;
        OnFileDropOff?: () => void;
    };
};

const app = () => (window as WailsWindow).go?.main?.App;
const rt = () => (window as WailsWindow).runtime;

/**
 * Wails 对共享 FilesPanel 的宿主适配。
 * FT 系/Local 系/SelectSavePath 走 window.go.main.App（返回 JSON 信封字符串）；
 * 进度/完成事件走 window.runtime.EventsOn；OS 拖放走 OnFileDrop（真实路径）。
 */
export const wailsFileTransferHost: FileTransferHost = {
    FTCheck: (sessionId) => app()!.FTCheck(sessionId),
    FTList: (sessionId, remotePath) => app()!.FTList(sessionId, remotePath),
    FTStat: (sessionId, remotePath) => app()!.FTStat(sessionId, remotePath),
    FTUpload: (sessionId, localPath, remotePath) => app()!.FTUpload(sessionId, localPath, remotePath),
    FTDownload: (sessionId, remotePath, localPath) => app()!.FTDownload(sessionId, remotePath, localPath),
    FTCancel: (taskId) => app()!.FTCancel(taskId),
    FTRemoteMkdir: (sessionId, remotePath) => app()!.FTRemoteMkdir(sessionId, remotePath),
    FTRemoteRemove: (sessionId, remotePath) => app()!.FTRemoteRemove(sessionId, remotePath),
    FTRemoteRename: (sessionId, oldPath, newPath) => app()!.FTRemoteRename(sessionId, oldPath, newPath),
    FTRemoteReadFile: (sessionId, remotePath, maxBytes) => app()!.FTRemoteReadFile(sessionId, remotePath, maxBytes),
    FTRemoteWriteFile: (sessionId, remotePath, content) => app()!.FTRemoteWriteFile(sessionId, remotePath, content),
    LocalList: (localPath) => app()!.LocalList(localPath),
    LocalMkdir: (localPath) => app()!.LocalMkdir(localPath),
    LocalRemove: (localPath) => app()!.LocalRemove(localPath),
    LocalRename: (oldPath, newPath) => app()!.LocalRename(oldPath, newPath),
    LocalCopy: (src, dst) => app()!.LocalCopy(src, dst),
    LocalStat: (localPath) => app()!.LocalStat(localPath),
    SelectSavePath: (defaultName) => app()!.SelectSavePath(defaultName),
    onProgress(handler: (p: FileTransferProgress) => void): () => void {
        const off = rt()?.EventsOn?.('file-transfer-progress', handler);
        return typeof off === 'function' ? off : () => {};
    },
    onDone(handler: (d: FileTransferDone) => void): () => void {
        const off = rt()?.EventsOn?.('file-transfer-done', handler);
        return typeof off === 'function' ? off : () => {};
    },
    onFileDrop(handler: FileDropHandler): () => void {
        const runtime = rt();
        if (!runtime?.OnFileDrop) return () => {};
        // useDropTarget=false：任意 drop 都回调，命中测试由组件自己做（与迁移前行为一致）
        runtime.OnFileDrop(handler, false);
        return () => runtime.OnFileDropOff?.();
    },
};
