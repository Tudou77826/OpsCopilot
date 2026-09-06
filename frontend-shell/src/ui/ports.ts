import type { CompletionData } from './Terminal/CompletionOverlay';
import type { ConnectionConfig } from './types';
import type { QuickCommand } from '../core/sidecarClient';

/** Terminal UI 运行所需的宿主能力。具体传输由 Wails 或 sidecar adapter 实现。 */
export interface TerminalRuntime {
  resize(sessionId: string, cols: number, rows: number): void | Promise<void>;
  getCompletions?(input: string, cursor: number): Promise<CompletionData | null>;
}

export interface HostCapabilities {
  standaloneChrome: boolean;
  fileTransfer: boolean;
  terminalCompletion: boolean;
}

/**
 * 会话树节点：递归树形结构（文件夹 / 会话）。
 * 与后端 sessionmanager.Session 的 JSON 形态一致。
 */
export interface SessionNode {
  id: string;
  name: string;
  type: 'folder' | 'session';
  children?: SessionNode[];
  config?: ConnectionConfig;
}

/**
 * 团队共享会话条目（Wails 专有宿主能力）。
 * 无明文凭据；连接时才由宿主取回解密配置。
 */
export interface SharedSessionEntry {
  entryKey: string;
  owner: string;
  name: string;
  protocol?: string;
  host: string;
  port: number;
  user: string;
  lastLoginAt: string;
  own: boolean;
  hasSecrets: boolean;
  decryptable: boolean;
}

export interface SharedConnectResult {
  success: boolean;
  message?: string;
  config?: ConnectionConfig;
}

/** 会话管理 UI 所需的宿主能力。具体 RPC 由 Wails 或 sidecar adapter 实现。 */
export interface SessionManagerRuntime {
  listSessions(): Promise<SessionNode[]>;
  deleteSession(id: string): Promise<void>;
  renameSession(id: string, newName: string): Promise<void>;
  updateSession(id: string, config: ConnectionConfig, group: string): Promise<void>;
  createFolder(name: string): Promise<void>;
  /**
   * 复制一条已保存会话为新的连接条目（完整配置副本，落同一文件夹）。
   * 可选能力：宿主未提供时右键菜单不显示"复制连接信息"。
   */
  duplicateSession?(id: string): Promise<void>;
}

/** 团队共享会话宿主能力（Wails 专有，可选）。Sidecar 不提供时组件不渲染。 */
export interface SharedSessionRuntime {
  list?(): Promise<SharedSessionEntry[]>;
  connect?(entryKey: string): Promise<SharedConnectResult>;
  saveToLocal?(entryKey: string): Promise<void>;
  remove?(entryKey: string): Promise<void>;
  /** 共享会话别处更新时同步刷新。Wails 走 window.runtime.EventsOn('session-share:synced')；Sidecar 可空实现。 */
  onSynced?(handler: () => void): () => void;
}

/** 快捷命令存储适配器：意图化单条操作。
 * 旧的全量 save(commands) 会让多窗口互相用旧快照覆盖，已废弃；
 * 增删改各走单条接口，配合后端文件变化热加载保持多窗口一致。
 */
export interface QuickCommandStorageAdapter {
  load(): Promise<QuickCommand[]>;
  add(cmd: QuickCommand): void;
  update(id: string, updates: Partial<QuickCommand>): void;
  remove(id: string): void;
  reorder(ids: string[]): void;
}

/** 快捷命令 UI 所需宿主能力。execute → 发送到激活终端。 */
export interface QuickCommandHost {
  execute(content: string): void;
  storage: QuickCommandStorageAdapter;
  /** 多窗口变更通知订阅。Wails 走 window.runtime.EventsOn；Sidecar 走配置轮询。 */
  onExternalChange?: (handler: (cmds: QuickCommand[]) => void) => () => void;
}

// QuickCommand 类型由 core 层定义（平台无关 RPC 模型），此处供端口与组件复用。
export type { QuickCommand } from '../core/sidecarClient';

/** 命令生成（Ctrl+K）宿主能力。未注入时入口不出现（能力边界纪律）。 */
export interface CommandGeneratorRuntime {
  /** 自然语言 → Linux 命令。未配置 AI 时 reject。 */
  generate(query: string): Promise<{ command: string; explanation?: string }>;
}

/** 智能连接意图解析宿主能力（可选）。未注入时 SmartConnectModal 隐藏 AI 区。 */
export interface ConnectIntentParserRuntime {
  parse(input: string): Promise<ConnectionConfig[]>;
}
