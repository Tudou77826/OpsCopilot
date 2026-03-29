/// <reference types="vite/client" />

interface FTPBindings {
  ListSessions: () => Promise<string>
  GetStartupSession: () => Promise<string>
  FTCheck: (sessionId: string) => Promise<string>
  FTList: (sessionId: string, remotePath: string) => Promise<string>
  FTStat: (sessionId: string, remotePath: string) => Promise<string>
  FTUpload: (sessionId: string, localPath: string, remotePath: string) => Promise<string>
  FTDownload: (sessionId: string, remotePath: string, localPath: string) => Promise<string>
  FTCancel: (taskId: string) => Promise<string>
  FTRemoteMkdir: (sessionId: string, remotePath: string) => Promise<string>
  FTRemoteRemove: (sessionId: string, remotePath: string) => Promise<string>
  FTRemoteRename: (sessionId: string, oldPath: string, newPath: string) => Promise<string>
  FTRemoteReadFile: (sessionId: string, remotePath: string, maxBytes: number) => Promise<string>
  FTRemoteWriteFile: (sessionId: string, remotePath: string, content: string) => Promise<string>
  LocalList: (localPath: string) => Promise<string>
  LocalMkdir: (localPath: string) => Promise<string>
  LocalRemove: (localPath: string) => Promise<string>
  LocalRename: (oldPath: string, newPath: string) => Promise<string>
}

interface Window {
  go: {
    main: {
      App: any
      FTPApp: FTPBindings
    }
  }
}
