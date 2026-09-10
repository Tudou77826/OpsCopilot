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
 * 会话树节点：递归树形结构（文件夹 / 会话），支持任意层级嵌套。
 * 与后端 connectionstore.Node 的 JSON 形态一致。
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

/**
 * 会话管理 UI 所需的宿主能力。具体 RPC 由 Wails 或 sidecar adapter 实现。
 *
 * 归属一律通过树位置表达：文件夹按 ID 寻址，因此重命名文件夹不会让任何引用失配，
 * 也不存在"节点位置"与某字段不一致的可能。
 */
export interface SessionManagerRuntime {
  listTree(): Promise<SessionNode[]>;
  /** 在 parentId 下新建文件夹；parentId 为空表示根。 */
  createFolder(name: string, parentId: string): Promise<void>;
  /** 保存一条新连接而不建立会话（"新建会话"入口）。 */
  createConnection(config: ConnectionConfig, parentId: string): Promise<void>;
  renameNode(id: string, newName: string): Promise<void>;
  /** 更新连接配置，节点位置不变（移动请用 moveNode）。 */
  updateConnection(id: string, config: ConnectionConfig): Promise<void>;
  /**
   * 把节点移动到 newParentId 下的第 index 位；newParentId 为空表示根。
   * index 是"摘除待移动节点之后"在目标兄弟列表中的插入位置。
   */
  moveNode(id: string, newParentId: string, index: number): Promise<void>;
  deleteNode(id: string): Promise<void>;
  /**
   * 按 orderedIds 重排某一层子节点。
   * 顺序由前端计算：浏览器的 localeCompare('zh-CN') 能正确处理中文拼音序，
   * 而后端没有等价的本地化比较，所以"按名称排序"在前端算好后交给它落库。
   */
  reorderNodes(parentId: string, orderedIds: string[]): Promise<void>;
  /**
   * 复制一条已保存连接（完整配置副本，落同一文件夹）。
   * 可选能力：宿主未提供时右键菜单不显示"复制连接"。
   */
  duplicateConnection?(id: string): Promise<void>;

  // ── Xshell 导入（可选能力，Wails 提供；sidecar 未提供时隐藏导入入口）──
  /** 探测本机 Xshell 会话目录（同机场景下最省事的导入入口）。 */
  detectXshellDirs?(): Promise<XshellSessionDir[]>;
  /** 探测本机凭据，用于在界面上说明"密码将自动解密"还是"需补充凭据"。 */
  xshellImportStatus?(): Promise<XshellCredentialStatus>;
  /** 弹出系统文件选择框，返回选中路径；用户取消时返回空串。 */
  selectImportFile?(): Promise<string>;
  /** 弹出系统目录选择框，返回选中路径；用户取消时返回空串。 */
  selectImportDirectory?(): Promise<string>;
  /** 只读分析导入影响（含密码可解密数量），不写入任何数据。 */
  analyzeXshellImport?(path: string, options: XshellImportOptions): Promise<XshellImportAnalysis>;
  /** 执行导入。 */
  applyXshellImport?(path: string, options: XshellImportOptions): Promise<XshellImportReport>;
}

/** 探测到的一个本机 Xshell 会话目录。 */
export interface XshellSessionDir {
  path: string;
  /** Xshell 版本号（取自 NetSarang Computer/<版本> 目录名）。 */
  version: number;
  /** 目录内 .xsh 文件数量，帮助用户选对目录。 */
  sessions: number;
}

/** 本机 Xshell 凭据探测结果。 */
export interface XshellCredentialStatus {
  available: boolean;
  maskedSid?: string;
  windowsUser?: string;
  /** 面向用户的一句话说明。 */
  message: string;
}

/** Xshell 导入选项。 */
export interface XshellImportOptions {
  /** 关闭时只导入主机/端口/用户名等非敏感字段。 */
  decryptPassword: boolean;
  /** 跨机器导入时由用户提供的源机器 Windows SID。 */
  sourceSid?: string;
  /** Xshell 启用了主密码时的主密码。 */
  masterPassword?: string;
}

/** 导入前的影响分析。 */
export interface XshellImportAnalysis {
  total: number;
  supported: number;
  unsupported: number;
  existing: number;
  withPassword: number;
  passwordDecrypted: number;
  passwordFailed: number;
  groups: number;
  protocols: Record<string, number>;
  warnings: string[];
}

/** 导入结果。 */
export interface XshellImportReport {
  imported: number;
  skippedExisting: number;
  skippedUnsupported: number;
  passwordDecrypted: number;
  passwordFailed: number;
  warnings: string[];
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
