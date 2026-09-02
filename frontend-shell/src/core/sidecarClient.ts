/**
 * SidecarClient：shell-sidecar 控制面客户端（JSON-RPC 2.0 over WebSocket）。
 * dev 模式直连 sidecar 的 /rpc 镜像；平台模式下由适配层换成平台 RPC（契约不变）。
 *
 * 数据面（PTY 字节流）不在此处：见 TerminalChannel / TerminalView。
 */

export interface SidecarOptions {
  /** 控制面 WS 地址：dev 为 ws://127.0.0.1:<port>/rpc?token=…；平台模式由适配层提供 */
  url: string;
}

interface Pending {
  resolve: (value: any) => void;
  reject: (err: Error) => void;
}

export type NotificationHandler = (params: any) => void;

export interface ConnectResult {
  connectionId: string;
}

export interface OpenTerminalResult {
  terminalId: string;
}

export interface SavedSession {
  id: string;
  name: string;
  type: string;
  children?: SavedSession[];
  config?: {
    name?: string;
    protocol?: string;
    host: string;
    port?: number;
    user: string;
    password?: string;
    rootPassword?: string;
    bastion?: {
      name?: string;
      host: string;
      port?: number;
      user: string;
      password?: string;
    };
    group?: string;
  };
}

/** 快捷命令/脚本（与终端应用 quick_commands.json 同格式）。 */
export interface QuickCommand {
  id: string;
  name: string;
  content: string;
  group?: string;
}

export interface MonitorSample {
  load1: string;
  memTotalMB: number;
  memUsedMB: number;
  memUsedPct: number;
  diskUsedPct: number;
  diskPath: string;
  sampledAt: number;
}

export class SidecarClient {
  private ws: WebSocket | null = null;
  private nextId = 1;
  private pending = new Map<number, Pending>();
  private handlers = new Map<string, Set<NotificationHandler>>();
  private closedByUser = false;

  constructor(private readonly options: SidecarOptions) {}

  /** 建立 RPC 通道；收到任何应答前 resolve。 */
  open(): Promise<void> {
    this.closedByUser = false;
    return new Promise((resolve, reject) => {
      const ws = new WebSocket(this.options.url);
      this.ws = ws;
      ws.addEventListener('open', () => resolve());
      ws.addEventListener('error', () => reject(new Error('无法连接 sidecar 控制面：' + this.options.url)));
      ws.addEventListener('message', (event) => this.handleMessage(event));
      ws.addEventListener('close', () => {
        // 未决请求全部失败
        for (const [, p] of this.pending) p.reject(new Error('sidecar 控制面连接已关闭'));
        this.pending.clear();
        if (!this.closedByUser) {
          for (const [, set] of this.handlers) set.forEach((h) => h({ __disconnected: true }));
        }
      });
    });
  }

  private handleMessage(event: MessageEvent) {
    let frame: any;
    try {
      frame = JSON.parse(typeof event.data === 'string' ? event.data : '');
    } catch {
      return;
    }
    if (frame.id !== undefined && (frame.result !== undefined || frame.error !== undefined)) {
      const pending = this.pending.get(frame.id);
      if (!pending) return;
      this.pending.delete(frame.id);
      if (frame.error) pending.reject(new Error(frame.error.message || 'sidecar 调用失败'));
      else pending.resolve(frame.result);
      return;
    }
    if (frame.method) {
      const set = this.handlers.get(frame.method);
      if (set) set.forEach((handler) => handler(frame.params));
    }
  }

  /** 订阅 sidecar 通知（terminal/exited、connection/lost…）。返回取消函数。 */
  on(method: string, handler: NotificationHandler): () => void {
    let set = this.handlers.get(method);
    if (!set) {
      set = new Set();
      this.handlers.set(method, set);
    }
    set.add(handler);
    return () => set!.delete(handler);
  }

  private call<T>(method: string, params: Record<string, unknown>): Promise<T> {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      return Promise.reject(new Error('控制面未连接'));
    }
    const id = this.nextId++;
    return new Promise<T>((resolve, reject) => {
      this.pending.set(id, { resolve: resolve as (v: any) => void, reject });
      this.ws!.send(JSON.stringify({ jsonrpc: '2.0', id, method, params }));
    });
  }

  initialize(): Promise<{ protocol: number; version: string; wsBase: string; token: string }> {
    return this.call('initialize', {});
  }

  connect(config: {
    host: string;
    port?: number;
    user: string;
    password: string;
    name?: string;
    bastion?: { host: string; port?: number; user: string; password: string } | null;
  }): Promise<ConnectResult> {
    return this.call('shell.connect', { config });
  }

  disconnect(connectionId: string): Promise<void> {
    return this.call('shell.disconnect', { connectionId });
  }

  openTerminal(connectionId: string, cols: number, rows: number): Promise<OpenTerminalResult> {
    return this.call('shell.openTerminal', { connectionId, cols, rows });
  }

  resize(terminalId: string, cols: number, rows: number): Promise<void> {
    return this.call('shell.resize', { terminalId, cols, rows });
  }

  closeTerminal(terminalId: string): Promise<void> {
    return this.call('shell.closeTerminal', { terminalId });
  }

  listConfigs(): Promise<{ sessions: SavedSession[] }> {
    return this.call('shell.configs.list', {});
  }

  saveConfig(input: { name: string; host: string; port?: number; user: string; password: string; group?: string }): Promise<{ id: string }> {
    return this.call('shell.configs.save', input);
  }

  deleteConfig(id: string): Promise<void> {
    return this.call('shell.configs.delete', { id });
  }

  renameConfig(id: string, name: string): Promise<void> {
    return this.call('shell.configs.rename', { id, name });
  }

  /** 更新已保存的连接配置（含 group 移动）。config 为完整连接配置。 */
  updateConfig(id: string, config: Record<string, unknown>, group: string): Promise<void> {
    return this.call('shell.configs.update', { id, config, group });
  }

  createFolder(name: string): Promise<void> {
    return this.call('shell.configs.createFolder', { name });
  }

  quickcmdsList(): Promise<{ commands: QuickCommand[] }> {
    return this.call('shell.quickcmds.list', {});
  }
  quickcmdsSave(cmd: QuickCommand): Promise<{ id: string }> {
    return this.call('shell.quickcmds.save', cmd as unknown as Record<string, unknown>);
  }
  quickcmdsDelete(id: string): Promise<void> {
    return this.call('shell.quickcmds.delete', { id });
  }
  quickcmdsReorder(ids: string[]): Promise<void> {
    return this.call('shell.quickcmds.reorder', { ids });
  }

  // ---- 结构化脚本（阶段 5：pkg/script 引擎，返回脚本对象） ----

  scriptList(): Promise<Record<string, unknown>[]> {
    return this.call<{ scripts?: Record<string, unknown>[] }>('shell.script.list', {}).then((r) => r.scripts ?? []);
  }
  scriptLoad(id: string): Promise<Record<string, unknown>> {
    return this.call('shell.script.load', { id });
  }
  scriptUpdate(script: Record<string, unknown>): Promise<void> {
    return this.call('shell.script.update', script);
  }
  scriptDelete(id: string): Promise<void> {
    return this.call('shell.script.delete', { id });
  }
  scriptCreate(name: string, description: string): Promise<Record<string, unknown>> {
    return this.call('shell.script.create', { name, description });
  }
  scriptReplay(id: string, terminalId: string): Promise<void> {
    return this.call('shell.script.replay', { id, terminalId });
  }
  scriptReplayVars(id: string, terminalId: string, values: Record<string, string>): Promise<void> {
    return this.call('shell.script.replayVars', { id, terminalId, values });
  }
  scriptStartRecording(name: string, description: string, terminalId: string): Promise<Record<string, unknown>> {
    return this.call('shell.script.startRecording', { name, description, terminalId });
  }
  scriptStopRecording(): Promise<Record<string, unknown>> {
    return this.call('shell.script.stopRecording', {});
  }
  scriptStatus(): Promise<Record<string, unknown>> {
    return this.call('shell.script.status', {});
  }

  // ---- 文件传输（阶段 4：对齐共享 FilesPanel 端口，返回 ftEnvelope 对象） ----

  ftCheck(terminalId: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.check', { terminalId });
  }
  ftList(terminalId: string, remotePath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.list', { terminalId, remotePath });
  }
  ftStat(terminalId: string, remotePath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.stat', { terminalId, remotePath });
  }
  ftUpload(terminalId: string, localPath: string, remotePath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.upload', { terminalId, localPath, remotePath });
  }
  ftDownload(terminalId: string, remotePath: string, localPath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.download', { terminalId, remotePath, localPath });
  }
  ftCancel(taskId: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.cancel', { taskId });
  }
  ftMkdir(terminalId: string, remotePath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.mkdir', { terminalId, remotePath });
  }
  ftRemove(terminalId: string, remotePath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.remove', { terminalId, remotePath });
  }
  ftRename(terminalId: string, oldPath: string, newPath: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.rename', { terminalId, oldPath, newPath });
  }
  ftReadFile(terminalId: string, remotePath: string, maxBytes: number): Promise<Record<string, unknown>> {
    return this.call('shell.ft.readFile', { terminalId, remotePath, maxBytes });
  }
  ftWriteFile(terminalId: string, remotePath: string, content: string): Promise<Record<string, unknown>> {
    return this.call('shell.ft.writeFile', { terminalId, remotePath, content });
  }

  // ---- 本地面板（sidecar 数据目录沙箱） ----

  fsLocalList(path: string): Promise<Record<string, unknown>> {
    return this.call('shell.fs.list', { path });
  }
  fsLocalStat(path: string): Promise<Record<string, unknown>> {
    return this.call('shell.fs.stat', { path });
  }
  fsLocalMkdir(path: string): Promise<Record<string, unknown>> {
    return this.call('shell.fs.mkdir', { path });
  }
  fsLocalRemove(path: string): Promise<Record<string, unknown>> {
    return this.call('shell.fs.remove', { path });
  }
  fsLocalRename(oldPath: string, newPath: string): Promise<Record<string, unknown>> {
    return this.call('shell.fs.rename', { oldPath, newPath });
  }

  // ---- Shell 设置（阶段 5：数据目录持久化，重启保持） ----

  settingsGet(): Promise<Record<string, unknown>> {
    return this.call('shell.settings.get', {});
  }
  settingsSave(next: Record<string, unknown>): Promise<void> {
    return this.call('shell.settings.save', next);
  }

  // ---- AI 接入配置（迭代 A：读取脱敏，密钥只在 sidecar 后台感知） ----

  aiGetConfig(): Promise<Record<string, unknown>> {
    return this.call('shell.ai.getConfig', {});
  }
  aiSaveConfig(update: Record<string, unknown>): Promise<Record<string, unknown>> {
    return this.call('shell.ai.saveConfig', update);
  }
  /** 迭代 B：AI 命令生成（单发）。未配置 AI 时后端报错。 */
  aiGenerateCommand(request: string): Promise<{ command: string; explanation?: string }> {
    return this.call('shell.ai.generateCommand', { request });
  }
  /** 迭代 B：智能连接意图解析（单发）。返回 { configs: [...] }。 */
  aiParseIntent(input: string): Promise<{ configs: Record<string, unknown>[] }> {
    return this.call('shell.ai.parseIntent', { input });
  }
  /** 迭代 C：启动诊断长任务；进度经 shell.diagnose.event 通知。terminalId/host/user 供案例绑定。 */
  diagnoseStart(problem: string, opts?: { terminalId?: string; host?: string; user?: string }): Promise<Record<string, unknown>> {
    return this.call('shell.diagnose.start', { problem, ...opts });
  }
  /** 迭代 C：取消诊断长任务（幂等）。 */
  diagnoseCancel(runId: string): Promise<void> {
    return this.call('shell.diagnose.cancel', { runId });
  }
  diagnoseStop(rootCause: string, conclusion: string): Promise<{ caseId: string; commands: number }> {
    return this.call('shell.diagnose.stop', { rootCause, conclusion });
  }
  /** 结案报告：token/concl-done 事件经 shell.diagnose.event 推送。 */
  diagnoseConclusion(rootCause: string): Promise<{ runId: string }> {
    return this.call('shell.diagnose.conclusion', { rootCause });
  }
  diagnoseArchive(input: Record<string, unknown>): Promise<{ filePath: string }> {
    return this.call('shell.diagnose.archive', input);
  }

  // ---- 命令补全（迭代 A：pkg/completion 静态库） ----

  getCompletions(input: string, cursor: number): Promise<Record<string, unknown> | null> {
    return this.call('shell.completion', { input, cursor });
  }

  monitorSample(connectionId: string): Promise<MonitorSample> {
    return this.call('shell.monitor.sample', { connectionId });
  }

  close(): void {
    this.closedByUser = true;
    this.ws?.close();
    this.ws = null;
  }
}
